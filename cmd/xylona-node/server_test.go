package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/launchenv"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/nodetls"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1/nodeprotoconnect"
)

// newTestServer wires up a pinned-TLS httptest server with a nodeServiceServer
// wrapping a real *internal/node.Node (backed by a fresh supervisor, no DB). Returns
// the server URL and the cert fingerprint.
func newTestServer(t *testing.T, sharedSecret string) (string, string) {
	t.Helper()

	certPEM, keyPEM, fingerprint, errGen := nodetls.GenerateSelfSigned(context.Background(), "test-node")
	if errGen != nil {
		t.Fatalf("GenerateSelfSigned: %v", errGen)
	}
	tlsConfig, errTLS := nodetls.NewServerTLSConfig(certPEM, keyPEM)
	if errTLS != nil {
		t.Fatalf("NewServerTLSConfig: %v", errTLS)
	}

	supInst, errSup := supervisor.New(t.Context())
	if errSup != nil {
		t.Fatalf("supervisor.New: %v", errSup)
	}
	n := node.New(t.Context(), supInst, nil)

	svc := newNodeServiceServer(n, sharedSecret)
	mux := http.NewServeMux()
	path, handler := nodeprotoconnect.NewNodeServiceHandler(svc)
	mux.Handle(path, handler)

	server := httptest.NewUnstartedServer(mux)
	server.TLS = tlsConfig
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	return server.URL, fingerprint
}

// TestServerAuthorization verifies that requests without a bearer token or
// with the wrong token are rejected, and a matching token is accepted.
func TestServerAuthorization(t *testing.T) {
	t.Parallel()

	const secret = "correct-horse-battery-staple"

	t.Run("valid secret is accepted", func(t *testing.T) {
		t.Parallel()
		url, fingerprint := newTestServer(t, secret)
		client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, secret)
		if errNew != nil {
			t.Fatalf("NewGRPCClient: %v", errNew)
		}
		errPing := client.Ping(t.Context())
		if errPing != nil {
			t.Fatalf("Ping with valid secret failed: %v", errPing)
		}
	})

	t.Run("wrong secret is rejected", func(t *testing.T) {
		t.Parallel()
		url, fingerprint := newTestServer(t, secret)
		client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, "bad-secret")
		if errNew != nil {
			t.Fatalf("NewGRPCClient: %v", errNew)
		}
		errPing := client.Ping(t.Context())
		if errPing == nil {
			t.Fatal("expected rejection with wrong secret")
		}
		if !strings.Contains(errPing.Error(), "unauthenticated") {
			t.Fatalf("expected unauthenticated error, got %v", errPing)
		}
	})

	t.Run("missing header is rejected via direct connect client", func(t *testing.T) {
		t.Parallel()
		url, fingerprint := newTestServer(t, secret)

		httpClient, errClient := nodetls.NewPinnedTLSClient(fingerprint)
		if errClient != nil {
			t.Fatalf("NewPinnedTLSClient: %v", errClient)
		}
		connectClient := nodeprotoconnect.NewNodeServiceClient(httpClient, url)

		_, errPing := connectClient.Ping(t.Context(), connect.NewRequest(&nodeprotov1.PingRequest{}))
		if errPing == nil {
			t.Fatal("expected error for missing auth header")
		}
	})
}

// TestNodeServiceServerListFiles drives the full client->server round trip for
// ListFiles, using a directory materialized in the test filesystem.
func TestNodeServiceServerListFiles(t *testing.T) {
	t.Parallel()

	const secret = "test-secret"
	url, fingerprint := newTestServer(t, secret)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, secret)
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	// Materialize a single file so ListFiles has something to return.
	dir := t.TempDir()
	contents := []byte("hello from listfiles test")
	errWrite := os.WriteFile(filepath.Join(dir, "sample.txt"), contents, 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile: %v", errWrite)
	}

	entries, errList := client.ListFiles(t.Context(), dir, "")
	if errList != nil {
		t.Fatalf("ListFiles: %v", errList)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "sample.txt" || entries[0].Size != int64(len(contents)) {
		t.Fatalf("entry mismatch: %+v", entries[0])
	}
}

func TestNodeServiceServerRuntimeCapabilities(t *testing.T) {
	t.Parallel()

	const secret = "test-secret"
	url, fingerprint := newTestServer(t, secret)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, secret)
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	caps, errCaps := client.GetRuntimeCapabilities(t.Context())
	if errCaps != nil {
		t.Fatalf("GetRuntimeCapabilities: %v", errCaps)
	}
	if caps.ProtocolVersion != node.RuntimeProtocolVersion || !caps.LaunchEnv ||
		!caps.ReliableProcessLifecycle || !caps.TelnetInput || !caps.RCONInput ||
		!caps.RESTInput || !caps.PlayerActions || !caps.PalworldMap ||
		!caps.SevenDaysToDieMap || !caps.MinecraftMap {
		t.Fatalf("runtime capabilities = %+v", caps)
	}
}

func TestNodeServiceServerQuerySevenDaysToDieWebAPIStatus(t *testing.T) {
	t.Parallel()

	const secret = "test-secret"
	url, fingerprint := newTestServer(t, secret)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, secret)
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}
	directory := t.TempDir()
	config := `<ServerSettings>
		<property name="WebDashboardEnabled" value="false" />
		<property name="WebDashboardPort" value="8082" />
	</ServerSettings>`
	errWrite := os.WriteFile(filepath.Join(directory, "serverconfig.xml"), []byte(config), 0o600)
	if errWrite != nil {
		t.Fatalf("write server config: %v", errWrite)
	}

	status, errQuery := client.QuerySevenDaysToDieWebAPIStatus(t.Context(), node.SevenDaysToDieWebAPIStatusQueryRequest{
		WorkingDirectory: directory,
		TokenName:        "controller",
		TokenSecret:      "web-api-secret",
	})
	if errQuery != nil {
		t.Fatalf("QuerySevenDaysToDieWebAPIStatus: %v", errQuery)
	}
	if status == nil || status.ConnectionState != node.SevenDaysToDieWebAPIConnectionStateDashboardDisabled {
		t.Fatalf("status = %+v, want dashboard disabled", status)
	}
}

func TestSevenDaysToDieWebAPIStateToProto(t *testing.T) {
	t.Parallel()

	t.Run("connection state", func(t *testing.T) {
		tests := []struct {
			name  string
			input node.SevenDaysToDieWebAPIConnectionState
			want  nodeprotov1.SevenDaysToDieWebAPIConnectionState
		}{
			{name: "unspecified", input: node.SevenDaysToDieWebAPIConnectionStateUnspecified, want: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_UNSPECIFIED},
			{name: "available", input: node.SevenDaysToDieWebAPIConnectionStateAvailable, want: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE},
			{name: "server offline", input: node.SevenDaysToDieWebAPIConnectionStateServerOffline, want: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE},
			{name: "dashboard disabled", input: node.SevenDaysToDieWebAPIConnectionStateDashboardDisabled, want: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DASHBOARD_DISABLED},
			{name: "misconfigured", input: node.SevenDaysToDieWebAPIConnectionStateMisconfigured, want: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_MISCONFIGURED},
			{name: "node unavailable", input: node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable, want: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_NODE_UNAVAILABLE},
			{name: "unreachable", input: node.SevenDaysToDieWebAPIConnectionStateUnreachable, want: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_WEB_API_UNREACHABLE},
			{name: "discovery unsupported", input: node.SevenDaysToDieWebAPIConnectionStateDiscoveryUnsupported, want: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DISCOVERY_UNSUPPORTED},
			{name: "authentication denied", input: node.SevenDaysToDieWebAPIConnectionStateAuthenticationDenied, want: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AUTHENTICATION_DENIED},
			{name: "invalid response", input: node.SevenDaysToDieWebAPIConnectionStateInvalidResponse, want: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_INVALID_RESPONSE},
			{name: "unknown", input: node.SevenDaysToDieWebAPIConnectionState(99), want: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_UNSPECIFIED},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got := sevenDaysToDieWebAPIConnectionStateToProto(test.input)
				if got != test.want {
					t.Fatalf("sevenDaysToDieWebAPIConnectionStateToProto(%v) = %v, want %v", test.input, got, test.want)
				}
			})
		}
	})

	t.Run("value state", func(t *testing.T) {
		tests := []struct {
			name  string
			input node.SevenDaysToDieWebAPIValueState
			want  nodeprotov1.SevenDaysToDieWebAPIValueState
		}{
			{name: "unspecified", input: node.SevenDaysToDieWebAPIValueStateUnspecified, want: nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED},
			{name: "available", input: node.SevenDaysToDieWebAPIValueStateAvailable, want: nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE},
			{name: "unsupported", input: node.SevenDaysToDieWebAPIValueStateUnsupported, want: nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED},
			{name: "permission denied", input: node.SevenDaysToDieWebAPIValueStatePermissionDenied, want: nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED},
			{name: "unavailable", input: node.SevenDaysToDieWebAPIValueStateUnavailable, want: nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE},
			{name: "unknown", input: node.SevenDaysToDieWebAPIValueState(99), want: nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got := sevenDaysToDieWebAPIValueStateToProto(test.input)
				if got != test.want {
					t.Fatalf("sevenDaysToDieWebAPIValueStateToProto(%v) = %v, want %v", test.input, got, test.want)
				}
			})
		}
	})
}

func TestSevenDaysToDieWebAPIStatusToProto(t *testing.T) {
	t.Parallel()

	active := false
	observedAt := time.Unix(1_700_000_000, 0).UTC()
	tests := []struct {
		name           string
		observedAt     time.Time
		wantObservedAt bool
	}{
		{name: "maps complete status", observedAt: observedAt, wantObservedAt: true},
		{name: "omits invalid timestamp", observedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			status := sevenDaysToDieWebAPIStatusToProto(&node.SevenDaysToDieWebAPIStatus{
				ConnectionState: node.SevenDaysToDieWebAPIConnectionStateAvailable,
				APIVersion:      "3.1.0",
				Capabilities: node.SevenDaysToDieWebAPICapabilities{
					PlayerData: true, RuntimeSettings: true, NativeLog: true, WorldPopulation: true,
					HostileAndAnimalPositions: true, AccessControl: true, GamePermissions: true, ReportedMods: true,
				},
				WorldTimeState:   node.SevenDaysToDieWebAPIValueStateAvailable,
				WorldTime:        &node.SevenDaysToDieGameTime{Day: 12, Hour: 5, Minute: 30},
				BloodMoonState:   node.SevenDaysToDieWebAPIValueStateAvailable,
				BloodMoonActive:  &active,
				NextBloodMoon:    &node.SevenDaysToDieGameTime{Day: 14, Hour: 22},
				NextBloodMoonEnd: &node.SevenDaysToDieGameTime{Day: 15, Hour: 4},
				ObservedAt:       test.observedAt,
			})
			if status.GetConnectionState() != nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE || status.GetApiVersion() != "3.1.0" {
				t.Fatalf("status = %+v", status)
			}
			if status.GetWorldTime().GetDay() != 12 || status.GetNextBloodMoon().GetDay() != 14 || status.GetNextBloodMoonEnd().GetDay() != 15 {
				t.Fatalf("game times = world %+v next %+v end %+v", status.GetWorldTime(), status.GetNextBloodMoon(), status.GetNextBloodMoonEnd())
			}
			if status.BloodMoonActive == nil || status.GetBloodMoonActive() {
				t.Fatalf("blood moon active = %v, want present false", status.BloodMoonActive)
			}
			capabilities := status.GetCapabilities()
			if !capabilities.GetPlayerData() || !capabilities.GetRuntimeSettings() || !capabilities.GetNativeLog() ||
				!capabilities.GetWorldPopulation() || !capabilities.GetHostileAndAnimalPositions() || !capabilities.GetAccessControl() ||
				!capabilities.GetGamePermissions() || !capabilities.GetReportedMods() {
				t.Fatalf("capabilities = %+v", capabilities)
			}
			if (status.GetObservedAt() != nil) != test.wantObservedAt {
				t.Fatalf("observed at = %v, want present %t", status.GetObservedAt(), test.wantObservedAt)
			}
		})
	}
}

func TestNodeRESTInputKindFromProto(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   nodeprotov1.RESTInputKind
		want    node.RESTInputKind
		wantErr bool
	}{
		{
			name:  "Satisfactory",
			input: nodeprotov1.RESTInputKind_REST_INPUT_KIND_SATISFACTORY,
			want:  node.RESTInputKindSatisfactory,
		},
		{
			name:  "Palworld",
			input: nodeprotov1.RESTInputKind_REST_INPUT_KIND_PALWORLD,
			want:  node.RESTInputKindPalworld,
		},
		{
			name:    "unspecified",
			input:   nodeprotov1.RESTInputKind_REST_INPUT_KIND_UNSPECIFIED,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, errKind := nodeRESTInputKindFromProto(tc.input)
			if tc.wantErr {
				if errKind == nil {
					t.Fatalf("nodeRESTInputKindFromProto(%v) error = nil", tc.input)
				}
				return
			}
			if errKind != nil {
				t.Fatalf("nodeRESTInputKindFromProto(%v) error = %v", tc.input, errKind)
			}
			if got != tc.want {
				t.Fatalf("nodeRESTInputKindFromProto(%v) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestTranslateConsoleInputErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       error
		wantCode    connect.Code
		wantMessage string
	}{
		{
			name:        "command rejection is invalid argument",
			input:       node.NewConsoleInputRejectedError("Palworld API returned 401 Unauthorized"),
			wantCode:    connect.CodeInvalidArgument,
			wantMessage: "Palworld API returned 401 Unauthorized",
		},
		{
			name:        "transport failure remains failed precondition",
			input:       node.ErrConsoleInputUnavailable,
			wantCode:    connect.CodeFailedPrecondition,
			wantMessage: node.ErrConsoleInputUnavailable.Error(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			errTranslated := translate(tc.input)
			if connect.CodeOf(errTranslated) != tc.wantCode {
				t.Fatalf("translate() code = %v, want %v", connect.CodeOf(errTranslated), tc.wantCode)
			}
			if !strings.Contains(errTranslated.Error(), tc.wantMessage) {
				t.Fatalf("translate() error = %v, want containing %q", errTranslated, tc.wantMessage)
			}
		})
	}
}

func TestNodeServiceServerReceivesTelnetInput(t *testing.T) {
	t.Parallel()

	const secret = "test-secret"
	url, fingerprint := newTestServer(t, secret)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, secret)
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	errStart := client.StartProcess(t.Context(), node.ProcessConfig{
		ID:          "remote-telnet-input",
		BaseCommand: "unused",
		InputTelnet: &node.TelnetInput{Port: 0},
	}, xylona.Status_ONLINE)
	if errStart == nil || !strings.Contains(errStart.Error(), supervisor.ErrTelnetPortRequired.Error()) {
		t.Fatalf("StartProcess error = %v, want node-side telnet validation", errStart)
	}
}

func TestNodeServiceServerLaunchEnvReachesChild(t *testing.T) {
	t.Parallel()

	const secret = "test-secret"
	const processID = "srv-launch-env"
	const envKey = "XYLONA_REMOTE_LAUNCH_ENV_TEST"
	const envValue = "remote-launch-value"
	url, fingerprint := newTestServer(t, secret)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, secret)
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	baseCommand, args := launchEnvEchoCommand(envKey)
	errStart := client.StartProcess(t.Context(), node.ProcessConfig{
		ID:               processID,
		Name:             "Launch Env",
		BaseCommand:      baseCommand,
		Args:             args,
		WorkingDirectory: t.TempDir(),
		LaunchEnv: map[string]string{
			envKey: envValue,
		},
	}, xylona.Status_ONLINE)
	if errStart != nil {
		t.Fatalf("StartProcess: %v", errStart)
	}
	defer func() {
		errStop := client.StopProcess(context.Background(), processID, "")
		if errStop != nil && !errors.Is(errStop, node.ErrProcessNotFound) {
			t.Logf("StopProcess after launch-env test: %v", errStop)
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		chunk, errRead := client.ReadConsoleBuffer(t.Context(), processID)
		if errRead != nil {
			t.Fatalf("ReadConsoleBuffer: %v", errRead)
		}
		if strings.Contains(chunk.Data, envValue) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	chunk, errRead := client.ReadConsoleBuffer(t.Context(), processID)
	if errRead != nil {
		t.Fatalf("ReadConsoleBuffer after timeout: %v", errRead)
	}
	t.Fatalf("console output = %q, want launch env value", chunk.Data)
}

func TestTranslateLaunchEnvironmentValidationError(t *testing.T) {
	errValidation := launchenv.NewValidationError([]launchenv.ValidationIssue{{
		Name:    "JDK_JAVA_OPTIONS",
		Message: "environment variable is reserved",
	}})
	errTranslated := translate(errValidation)
	if connect.CodeOf(errTranslated) != connect.CodeInvalidArgument {
		t.Fatalf("translate() code = %v, want %v", connect.CodeOf(errTranslated), connect.CodeInvalidArgument)
	}
}

func launchEnvEchoCommand(key string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "echo %" + key + "% & ping -n 3 127.0.0.1 >NUL"}
	}
	return "sh", []string{"-c", "printf '%s\n' \"$" + key + "\"; sleep 2"}
}

func TestNodeServiceServerStreamConsoleOutput(t *testing.T) {
	t.Parallel()

	const secret = "test-secret"
	const processID = "srv-console"
	const line = "hello from node stream"

	url, fingerprint := newTestServer(t, secret)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, secret)
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	streamResultCh := make(chan struct {
		chunks <-chan node.ConsoleChunk
		err    error
	}, 1)
	go func() {
		chunks, errStream := client.StreamConsoleOutput(ctx, processID)
		streamResultCh <- struct {
			chunks <-chan node.ConsoleChunk
			err    error
		}{chunks: chunks, err: errStream}
	}()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	var chunks <-chan node.ConsoleChunk
	for chunks == nil {
		select {
		case result := <-streamResultCh:
			if result.err != nil {
				t.Fatalf("StreamConsoleOutput: %v", result.err)
			}
			chunks = result.chunks
		case <-ticker.C:
			errSend := client.SendConsoleOutput(ctx, processID, line)
			if errSend != nil {
				t.Fatalf("SendConsoleOutput: %v", errSend)
			}
		case <-ctx.Done():
			t.Fatalf("timed out opening console stream: %v", ctx.Err())
		}
	}

	select {
	case chunk, ok := <-chunks:
		if !ok {
			t.Fatal("console stream closed before chunk arrived")
		}
		if chunk.ProcessID != processID {
			t.Fatalf("chunk.ProcessID = %q, want %q", chunk.ProcessID, processID)
		}
		if !chunk.ResetBuffer {
			t.Fatalf("initial chunk = %+v, want reset replay", chunk)
		}
		if strings.Contains(chunk.Data, line) {
			return
		}
		errSend := client.SendConsoleOutput(ctx, processID, line)
		if errSend != nil {
			t.Fatalf("SendConsoleOutput after replay: %v", errSend)
		}
		select {
		case live, open := <-chunks:
			if !open {
				t.Fatal("console stream closed before live chunk arrived")
			}
			if live.ResetBuffer || live.Sequence <= chunk.Sequence || !strings.Contains(live.Data, line) {
				t.Fatalf("live chunk = %+v, want sequenced console line %q", live, line)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for live console chunk: %v", ctx.Err())
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for console chunk: %v", ctx.Err())
	}
}

func TestNodeServiceServerStatAndStreamFile(t *testing.T) {
	t.Parallel()

	const secret = "test-secret"
	url, fingerprint := newTestServer(t, secret)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, secret)
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	dir := t.TempDir()
	contents := []byte("stream me from the node")
	errWrite := os.WriteFile(filepath.Join(dir, "archive.zip"), contents, 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile: %v", errWrite)
	}

	entry, errStat := client.StatFile(t.Context(), dir, "archive.zip")
	if errStat != nil {
		t.Fatalf("StatFile: %v", errStat)
	}
	if entry.Name != "archive.zip" || entry.Size != int64(len(contents)) || entry.IsDirectory {
		t.Fatalf("StatFile entry mismatch: %+v", entry)
	}

	stream, errStream := client.StreamFile(t.Context(), dir, "archive.zip")
	if errStream != nil {
		t.Fatalf("StreamFile: %v", errStream)
	}
	data, errRead := io.ReadAll(stream)
	errClose := stream.Close()
	if errRead != nil {
		t.Fatalf("ReadAll stream: %v", errRead)
	}
	if errClose != nil {
		t.Fatalf("Close stream: %v", errClose)
	}
	if string(data) != string(contents) {
		t.Fatalf("StreamFile data = %q, want %q", string(data), string(contents))
	}
}

func TestNodeServiceServerStreamWriteCopyAndProbe(t *testing.T) {
	t.Parallel()

	const secret = "test-secret"
	url, fingerprint := newTestServer(t, secret)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, secret)
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	dir := t.TempDir()
	result, errWrite := client.StreamWriteFile(t.Context(), dir, "payload.txt", strings.NewReader("remote payload"), node.ProtectionPolicy{})
	if errWrite != nil {
		t.Fatalf("StreamWriteFile: %v", errWrite)
	}
	wantSHA := fmt.Sprintf("%x", sha256.Sum256([]byte("remote payload")))
	if result.BytesWritten != int64(len("remote payload")) || result.SHA256 != wantSHA {
		t.Fatalf("StreamWriteFile result = %+v, want bytes and sha %q", result, wantSHA)
	}

	copied, errCopy := client.CopyFiles(t.Context(), dir, []node.CopyFileOperation{
		{SourceRelativePath: "payload.txt", DestinationRelativePath: "copies/payload.txt"},
	}, node.ProtectionPolicy{})
	if errCopy != nil {
		t.Fatalf("CopyFiles: %v", errCopy)
	}
	if len(copied) != 1 || copied[0] != "copies/payload.txt" {
		t.Fatalf("CopyFiles copied = %v, want [copies/payload.txt]", copied)
	}

	errMkdir := os.Mkdir(filepath.Join(dir, "steamapps"), 0o750)
	if errMkdir != nil {
		t.Fatalf("Mkdir steamapps: %v", errMkdir)
	}
	errManifest := os.WriteFile(filepath.Join(dir, "steamapps", "appmanifest_90.acf"), []byte(`"buildid" "98765"`), 0o600)
	if errManifest != nil {
		t.Fatalf("WriteFile manifest: %v", errManifest)
	}
	probe, errProbe := client.ProbeInstalledVersion(t.Context(), node.InstalledVersionProbeRequest{
		Directory:           dir,
		Kind:                node.InstalledVersionProbeKindSteamManifest,
		PreferredSteamAppID: "90",
	})
	if errProbe != nil {
		t.Fatalf("ProbeInstalledVersion: %v", errProbe)
	}
	if !probe.Found || probe.Version != "98765" || probe.SourcePath != "steamapps/appmanifest_90.acf" {
		t.Fatalf("ProbeInstalledVersion = %+v, want Steam manifest hit", probe)
	}
}

func TestNodeServiceServerQueryGameServer(t *testing.T) {
	t.Parallel()

	const secret = "test-secret"
	url, fingerprint := newTestServer(t, secret)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, secret)
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	result, errQuery := client.QueryGameServer(t.Context(), node.GameServerQueryRequest{
		Kind:       node.GameServerQueryKindMinecraft,
		IP:         "127.0.0.1",
		QueryPort:  1,
		MaxPlayers: 24,
	})
	if errQuery != nil {
		t.Fatalf("QueryGameServer: %v", errQuery)
	}
	if result.Kind != node.GameServerQueryKindMinecraft {
		t.Fatalf("kind = %v, want Minecraft", result.Kind)
	}
	if result.Minecraft == nil {
		t.Fatal("Minecraft result is nil")
	}
	if result.Minecraft.MaxPlayers != 24 {
		t.Fatalf("max players = %d, want 24", result.Minecraft.MaxPlayers)
	}
}

func TestSourceQueryToProtoPreservesPlayerList(t *testing.T) {
	t.Parallel()

	result := sourceQueryToProto(&node.SourceQueryInfo{
		Players:             2,
		MaxPlayers:          24,
		PlayerList:          []string{"Alyx", "Gordon"},
		PlayerListSupported: true,
	})

	if result == nil || !result.GetPlayerListSupported() || !slices.Equal(result.GetPlayerList(), []string{"Alyx", "Gordon"}) {
		t.Fatalf("Source proto player data = %+v, want supported [Alyx Gordon]", result)
	}
}

func TestNodeServiceServerFileErrorsAreDistinct(t *testing.T) {
	t.Parallel()

	const secret = "test-secret"
	url, fingerprint := newTestServer(t, secret)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, secret)
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	dir := t.TempDir()

	_, errMissing := client.ReadFile(t.Context(), dir, "missing.txt")
	if !errors.Is(errMissing, os.ErrNotExist) {
		t.Fatalf("ReadFile missing error = %v, want os.ErrNotExist", errMissing)
	}
	if errors.Is(errMissing, node.ErrInvalidPath) {
		t.Fatalf("ReadFile missing error = %v, must not be ErrInvalidPath", errMissing)
	}

	_, errInvalid := client.ReadFile(t.Context(), dir, "../escape.txt")
	if !errors.Is(errInvalid, node.ErrInvalidPath) {
		t.Fatalf("ReadFile invalid path error = %v, want ErrInvalidPath", errInvalid)
	}
	if errors.Is(errInvalid, os.ErrNotExist) {
		t.Fatalf("ReadFile invalid path error = %v, must not be os.ErrNotExist", errInvalid)
	}
}

func TestNodeServiceServerListBindableIPs(t *testing.T) {
	t.Parallel()

	const secret = "test-secret"
	url, fingerprint := newTestServer(t, secret)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, secret)
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	ips, errList := client.ListBindableIPs(t.Context())
	if errList != nil {
		t.Fatalf("ListBindableIPs: %v", errList)
	}
	if len(ips) == 0 {
		t.Fatalf("expected at least one bindable IP")
	}
}

func TestPalworldMapSnapshotToProtoPreservesSafeIntelligenceData(t *testing.T) {
	t.Parallel()

	snapshot := palworldMapSnapshotToProto(&node.PalworldMapSnapshot{
		CollectedAt: time.Now().UTC(),
		Actors: []node.PalworldMapActor{
			{
				Key:      "actor-key",
				Kind:     node.PalworldMapActorKindBase,
				Name:     "Skyforge",
				GuildKey: "guild-key",
			},
		},
		Health: &node.PalworldMapHealth{
			ServerFPS:         60,
			ServerFrameTimeMS: 16.67,
			CurrentPlayers:    4,
			MaxPlayers:        32,
			UptimeSeconds:     3600,
			BaseCampCount:     3,
			Days:              99,
		},
	})

	if snapshot == nil || len(snapshot.GetActors()) != 1 {
		t.Fatalf("palworldMapSnapshotToProto() = %+v", snapshot)
	}
	if snapshot.GetActors()[0].GetGuildKey() != "guild-key" {
		t.Fatalf("actor = %+v, want guild key", snapshot.GetActors()[0])
	}
	health := snapshot.GetHealth()
	if health == nil ||
		health.GetServerFps() != 60 ||
		health.GetServerFrameTimeMs() != 16.67 ||
		health.GetCurrentPlayers() != 4 ||
		health.GetMaxPlayers() != 32 ||
		health.GetUptimeSeconds() != 3600 ||
		health.GetBaseCampCount() != 3 ||
		health.GetDays() != 99 {
		t.Fatalf("health = %+v, want safe metrics", health)
	}
}
