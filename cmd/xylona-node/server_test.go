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
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

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
	if caps.ProtocolVersion != node.RuntimeProtocolVersion || !caps.LaunchEnv {
		t.Fatalf("runtime capabilities = %+v", caps)
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
		if !strings.Contains(chunk.Data, line) {
			t.Fatalf("chunk.Data = %q, want it to contain %q", chunk.Data, line)
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
