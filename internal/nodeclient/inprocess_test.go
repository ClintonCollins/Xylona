package nodeclient

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
)

// newTestClient constructs an in-process client backed by a Node with no
// supervisor and no database. Suitable for file-ops and event tests that do
// not need process supervision.
func newTestClient(t *testing.T) (NodeClient, *node.Node) {
	t.Helper()
	n := node.New(t.Context(), nil, nil)
	client := NewInProcessClient("node-A", n)
	return client, n
}

func newSupervisorBackedTestClient(t *testing.T) (NodeClient, *node.Node) {
	t.Helper()
	supervisorInst, errSupervisor := supervisor.New(t.Context())
	if errSupervisor != nil {
		t.Fatalf("supervisor.New: %v", errSupervisor)
	}
	n := node.New(t.Context(), supervisorInst, nil)
	client := NewInProcessClient("node-A", n)
	return client, n
}

func TestInProcessClientPingHonorsCanceledContext(t *testing.T) {
	client, _ := newTestClient(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	errPing := client.Ping(ctx)
	if !errors.Is(errPing, context.Canceled) {
		t.Fatalf("Ping() err = %v, want context.Canceled", errPing)
	}
}

func TestInProcessClientQuerySevenDaysToDieMapHonorsTacticalFlag(t *testing.T) {
	const markerID = "f4c2d4ea-7e4d-46b0-aaf2-26ea769951d4"
	tacticalRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-SDTD-API-TOKENNAME") != "controller" || request.Header.Get("X-SDTD-API-SECRET") != "web-api-secret" {
			http.Error(response, "missing credentials", http.StatusUnauthorized)
			return
		}
		var body string
		switch request.URL.Path {
		case "/api/map/config":
			body = `{"data":{"enabled":true,"mapBlockSize":128,"maxZoom":4,"mapSize":{"x":6144,"y":255,"z":6144}}}`
		case "/api/player":
			body = `{"data":{"players":[{"entityId":7,"name":"Alex","position":{"x":1,"y":2,"z":3}}]}}`
		case "/api/openapi/openapi.yaml":
			tacticalRequests++
			body = "openapi: 3.1.0\ninfo:\n  version: \"3.0\"\npaths:\n  /api/markers:\n    get: {}\n"
		case "/api/markers":
			tacticalRequests++
			body = `{"data":[{"id":"` + markerID + `","name":"Trader","x":10,"y":20}]}`
		default:
			http.NotFound(response, request)
			return
		}
		_, errWrite := response.Write([]byte(body))
		if errWrite != nil {
			t.Errorf("write upstream response: %v", errWrite)
		}
	}))
	t.Cleanup(server.Close)
	serverURL, errURL := url.Parse(server.URL)
	if errURL != nil {
		t.Fatalf("parse upstream URL: %v", errURL)
	}
	_, port, found := strings.Cut(serverURL.Host, ":")
	if !found {
		t.Fatalf("upstream host %q has no port", serverURL.Host)
	}
	directory := t.TempDir()
	config := fmt.Sprintf(`<ServerSettings><property name="WebDashboardEnabled" value="true"/><property name="WebDashboardPort" value="%s" /></ServerSettings>`, port)
	errWrite := os.WriteFile(filepath.Join(directory, "serverconfig.xml"), []byte(config), 0o600)
	if errWrite != nil {
		t.Fatalf("write server config: %v", errWrite)
	}
	client, _ := newTestClient(t)
	request := node.SevenDaysToDieMapQueryRequest{
		WorkingDirectory: directory, TokenName: "controller", TokenSecret: "web-api-secret",
	}

	base, errBase := client.QuerySevenDaysToDieMap(t.Context(), request)
	if errBase != nil || base == nil || len(base.Players) != 1 || tacticalRequests != 0 ||
		base.NativeMarkerState != node.SevenDaysToDieWebAPIValueStateUnspecified {
		t.Fatalf("base map = %+v, requests = %d, error = %v", base, tacticalRequests, errBase)
	}
	request.IncludeTactical = true
	tactical, errTactical := client.QuerySevenDaysToDieMap(t.Context(), request)
	if errTactical != nil || tactical == nil || len(tactical.NativeMarkers) != 1 ||
		tactical.NativeMarkerState != node.SevenDaysToDieWebAPIValueStateAvailable ||
		tactical.ClaimsState != node.SevenDaysToDieWebAPIValueStateUnsupported || tacticalRequests != 2 {
		t.Fatalf("tactical map = %+v, requests = %d, error = %v", tactical, tacticalRequests, errTactical)
	}
}

func TestInProcessClientQuerySevenDaysToDieWebAPIStatus(t *testing.T) {
	bloodMoonRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var body string
		switch request.URL.Path {
		case "/api/openapi/openapi.yaml":
			body = "openapi: 3.1.0\ninfo:\n  version: V2.2\npaths:\n  /api/bloodmoon:\n    get: {}\n  /api/serverstats:\n    get: {}\n"
		case "/api/bloodmoon":
			bloodMoonRequests++
			body = `{"data":{"gameTime":{"days":7,"hours":22,"minutes":0},"bloodmoonActive":true,"nextBloodmoon":{"days":14,"hours":22,"minutes":0},"nextBloodmoonEnd":{"days":15,"hours":4,"minutes":0}}}`
		case "/api/serverstats":
			body = `{"data":{"gameTime":{"days":8,"hours":13,"minutes":37}}}`
		default:
			http.NotFound(response, request)
			return
		}
		_, errWrite := response.Write([]byte(body))
		if errWrite != nil {
			t.Errorf("write upstream response: %v", errWrite)
		}
	}))
	t.Cleanup(server.Close)
	serverURL, errURL := url.Parse(server.URL)
	if errURL != nil {
		t.Fatalf("parse upstream URL: %v", errURL)
	}
	_, port, found := strings.Cut(serverURL.Host, ":")
	if !found {
		t.Fatalf("upstream host %q has no port", serverURL.Host)
	}
	client, _ := newTestClient(t)
	directory := t.TempDir()
	config := fmt.Sprintf(`<ServerSettings><property name="WebDashboardEnabled" value="true"/><property name="WebDashboardPort" value="%s" /></ServerSettings>`, port)
	errWrite := os.WriteFile(filepath.Join(directory, "serverconfig.xml"), []byte(config), 0o600)
	if errWrite != nil {
		t.Fatalf("write server config: %v", errWrite)
	}

	request := node.SevenDaysToDieWebAPIStatusQueryRequest{
		WorkingDirectory: directory,
		TokenName:        "controller",
		TokenSecret:      "web-api-secret",
	}
	status, errQuery := client.QuerySevenDaysToDieWebAPIStatus(t.Context(), request)
	if errQuery != nil {
		t.Fatalf("QuerySevenDaysToDieWebAPIStatus: %v", errQuery)
	}
	if status == nil || status.WorldTime == nil || status.WorldTime.Day != 8 ||
		status.BloodMoonState != node.SevenDaysToDieWebAPIValueStateUnspecified || bloodMoonRequests != 0 {
		t.Fatalf("viewer status = %+v, Blood Moon requests = %d", status, bloodMoonRequests)
	}
	request.IncludeTactical = true
	tacticalStatus, errTactical := client.QuerySevenDaysToDieWebAPIStatus(t.Context(), request)
	if errTactical != nil {
		t.Fatalf("QuerySevenDaysToDieWebAPIStatus(tactical): %v", errTactical)
	}
	if tacticalStatus == nil || tacticalStatus.BloodMoonState != node.SevenDaysToDieWebAPIValueStateAvailable || bloodMoonRequests != 1 {
		t.Fatalf("tactical status = %+v, Blood Moon requests = %d", tacticalStatus, bloodMoonRequests)
	}
}

func TestInProcessClientQuerySevenDaysToDiePlayers(t *testing.T) {
	client, _ := newTestClient(t)
	directory := t.TempDir()
	config := `<ServerSettings>
		<property name="WebDashboardEnabled" value="false" />
		<property name="WebDashboardPort" value="8082" />
	</ServerSettings>`
	errWrite := os.WriteFile(filepath.Join(directory, "serverconfig.xml"), []byte(config), 0o600)
	if errWrite != nil {
		t.Fatalf("write server config: %v", errWrite)
	}

	result, errQuery := client.QuerySevenDaysToDiePlayers(t.Context(), node.SevenDaysToDiePlayersQueryRequest{
		WorkingDirectory: directory,
		TokenName:        "controller",
		TokenSecret:      "web-api-secret",
	})
	if errQuery != nil {
		t.Fatalf("QuerySevenDaysToDiePlayers: %v", errQuery)
	}
	if result == nil || result.ConnectionState != node.SevenDaysToDieWebAPIConnectionStateDashboardDisabled ||
		result.State != node.SevenDaysToDieWebAPIValueStateUnavailable {
		t.Fatalf("result = %+v, want dashboard disabled", result)
	}
}

func TestInProcessClientQuerySevenDaysToDieReportedMods(t *testing.T) {
	client, _ := newTestClient(t)
	directory := t.TempDir()
	config := `<ServerSettings>
		<property name="WebDashboardEnabled" value="false" />
		<property name="WebDashboardPort" value="8082" />
	</ServerSettings>`
	errWrite := os.WriteFile(filepath.Join(directory, "serverconfig.xml"), []byte(config), 0o600)
	if errWrite != nil {
		t.Fatalf("write server config: %v", errWrite)
	}

	result, errQuery := client.QuerySevenDaysToDieReportedMods(t.Context(), node.SevenDaysToDieReportedModsQueryRequest{
		WorkingDirectory: directory,
		TokenName:        "controller",
		TokenSecret:      "web-api-secret",
	})
	if errQuery != nil {
		t.Fatalf("QuerySevenDaysToDieReportedMods: %v", errQuery)
	}
	if result == nil || result.ConnectionState != node.SevenDaysToDieWebAPIConnectionStateDashboardDisabled ||
		result.State != node.SevenDaysToDieWebAPIValueStateUnavailable {
		t.Fatalf("result = %+v, want dashboard disabled", result)
	}
}

func TestInProcessClientQuerySevenDaysToDieSandboxSettings(t *testing.T) {
	client, _ := newTestClient(t)
	directory := t.TempDir()
	config := `<ServerSettings>
		<property name="WebDashboardEnabled" value="false" />
		<property name="WebDashboardPort" value="8082" />
		<property name="SandboxCode" value="ABC" />
	</ServerSettings>`
	errWrite := os.WriteFile(filepath.Join(directory, "serverconfig.xml"), []byte(config), 0o600)
	if errWrite != nil {
		t.Fatalf("write server config: %v", errWrite)
	}

	result, errQuery := client.QuerySevenDaysToDieSandboxSettings(t.Context(), node.SevenDaysToDieSandboxSettingsQueryRequest{
		WorkingDirectory: directory,
		TokenName:        "controller",
		TokenSecret:      "web-api-secret",
	})
	if errQuery != nil {
		t.Fatalf("QuerySevenDaysToDieSandboxSettings: %v", errQuery)
	}
	if result == nil || result.ConnectionState != node.SevenDaysToDieWebAPIConnectionStateDashboardDisabled ||
		result.State != node.SevenDaysToDieWebAPIValueStateUnavailable {
		t.Fatalf("result = %+v, want dashboard disabled", result)
	}
}

func TestInProcessClientFileRoundTrip(t *testing.T) {
	client, _ := newTestClient(t)
	dir := t.TempDir()

	const relativePath = "hello.txt"
	errCreate := client.CreateFileOrDirectory(t.Context(), dir, relativePath, "hi", false, node.ProtectionPolicy{})
	if errCreate != nil {
		t.Fatalf("CreateFileOrDirectory: %v", errCreate)
	}

	data, errRead := client.ReadFile(t.Context(), dir, relativePath)
	if errRead != nil {
		t.Fatalf("ReadFile: %v", errRead)
	}
	if string(data) != "hi" {
		t.Fatalf("ReadFile = %q, want %q", string(data), "hi")
	}

	errWrite := client.WriteFile(t.Context(), dir, relativePath, []byte("bye"), node.ProtectionPolicy{})
	if errWrite != nil {
		t.Fatalf("WriteFile: %v", errWrite)
	}

	entries, errList := client.ListFiles(t.Context(), dir, "")
	if errList != nil {
		t.Fatalf("ListFiles: %v", errList)
	}
	if len(entries) != 1 {
		t.Fatalf("ListFiles len = %d, want 1", len(entries))
	}
	if entries[0].Name != relativePath {
		t.Fatalf("ListFiles[0].Name = %q, want %q", entries[0].Name, relativePath)
	}

	// Confirm the file content made it to disk via the client write.
	on, errReadDisk := os.ReadFile(filepath.Join(dir, relativePath))
	if errReadDisk != nil {
		t.Fatalf("os.ReadFile: %v", errReadDisk)
	}
	if string(on) != "bye" {
		t.Fatalf("on disk = %q, want %q", string(on), "bye")
	}
}

func TestInProcessClientStreamWriteCopyAndProbe(t *testing.T) {
	client, _ := newTestClient(t)
	dir := t.TempDir()

	result, errStreamWrite := client.StreamWriteFile(t.Context(), dir, "payload.bin", strings.NewReader("streamed payload"), node.ProtectionPolicy{})
	if errStreamWrite != nil {
		t.Fatalf("StreamWriteFile: %v", errStreamWrite)
	}
	wantSHA := fmt.Sprintf("%x", sha256.Sum256([]byte("streamed payload")))
	if result.BytesWritten != int64(len("streamed payload")) || result.SHA256 != wantSHA {
		t.Fatalf("StreamWriteFile result = %+v, want bytes and sha %q", result, wantSHA)
	}

	copied, errCopy := client.CopyFiles(t.Context(), dir, []node.CopyFileOperation{
		{SourceRelativePath: "payload.bin", DestinationRelativePath: "copies/payload.bin"},
	}, node.ProtectionPolicy{})
	if errCopy != nil {
		t.Fatalf("CopyFiles: %v", errCopy)
	}
	if len(copied) != 1 || copied[0] != "copies/payload.bin" {
		t.Fatalf("CopyFiles copied = %v, want [copies/payload.bin]", copied)
	}

	errMkdir := os.Mkdir(filepath.Join(dir, "steamapps"), 0o750)
	if errMkdir != nil {
		t.Fatalf("Mkdir steamapps: %v", errMkdir)
	}
	errManifest := os.WriteFile(filepath.Join(dir, "steamapps", "appmanifest_90.acf"), []byte(`"buildid" "222"`), 0o600)
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
	if !probe.Found || probe.Version != "222" || probe.SourcePath != "steamapps/appmanifest_90.acf" {
		t.Fatalf("ProbeInstalledVersion result = %+v, want Steam manifest hit", probe)
	}
}

func TestInProcessClientReadConsoleBufferHandlesUnknownProcess(t *testing.T) {
	client, _ := newTestClient(t)

	chunk, errChunk := client.ReadConsoleBuffer(t.Context(), "missing")
	if errChunk != nil {
		t.Fatalf("ReadConsoleBuffer unexpected error: %v", errChunk)
	}
	if chunk.ProcessID != "missing" {
		t.Fatalf("chunk.ProcessID = %q, want %q", chunk.ProcessID, "missing")
	}
	if chunk.Data != "" {
		t.Fatalf("chunk.Data = %q, want empty string", chunk.Data)
	}
}

func TestInProcessClientStartProcessWithoutSupervisorReturnsError(t *testing.T) {
	client, _ := newTestClient(t)

	errStart := client.StartProcess(t.Context(), node.ProcessConfig{ID: "srv-1", BaseCommand: "echo"}, 0)
	if errStart == nil {
		t.Fatalf("StartProcess expected error when supervisor is nil")
	}
}

func TestInProcessClientStreamEventsDeliversNodeEvents(t *testing.T) {
	client, n := newTestClient(t)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	stream, errStream := client.StreamEvents(ctx)
	if errStream != nil {
		t.Fatalf("StreamEvents: %v", errStream)
	}

	want := node.Event{
		Type:      node.EventTypeProcessStatus,
		ProcessID: "srv-42",
		Timestamp: time.Now(),
	}

	// Publish after subscribing; the emitter's goroutine-safe Publish will
	// deliver to our bridged channel.
	go n.Events().Publish(want)

	select {
	case got, ok := <-stream:
		if !ok {
			t.Fatalf("stream closed before event arrived")
		}
		if got.ProcessID != want.ProcessID {
			t.Fatalf("got.ProcessID = %q, want %q", got.ProcessID, want.ProcessID)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for bridged event")
	}
}

func TestInProcessClientStreamEventsClosesOnContextCancel(t *testing.T) {
	client, _ := newTestClient(t)

	ctx, cancel := context.WithCancel(t.Context())
	stream, errStream := client.StreamEvents(ctx)
	if errStream != nil {
		t.Fatalf("StreamEvents: %v", errStream)
	}
	cancel()

	select {
	case _, ok := <-stream:
		if ok {
			// Drain one possible spurious value; next recv must be closed.
			select {
			case _, stillOpen := <-stream:
				if stillOpen {
					t.Fatalf("stream not closed after cancel")
				}
			case <-time.After(time.Second):
				t.Fatalf("stream not closed after cancel")
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("stream not closed after cancel")
	}
}

func TestInProcessClientStreamEventsReplaysRetainedStatus(t *testing.T) {
	client, nodeInst := newTestClient(t)
	nodeInst.Events().Publish(node.Event{
		Type:               node.EventTypeProcessStatus,
		ProcessID:          "srv-replay",
		OldStatus:          "OFFLINE",
		Status:             "ONLINE",
		ExecutionID:        "execution-1",
		TransitionSequence: 1,
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	stream, errStream := client.StreamEvents(ctx)
	if errStream != nil {
		t.Fatalf("StreamEvents: %v", errStream)
	}

	select {
	case got, ok := <-stream:
		if !ok {
			t.Fatal("stream closed before status event arrived")
		}
		if got.Type != node.EventTypeProcessStatus {
			t.Fatalf("got.Type = %v, want %v", got.Type, node.EventTypeProcessStatus)
		}
		if got.ProcessID != "srv-replay" {
			t.Fatalf("got.ProcessID = %q, want %q", got.ProcessID, "srv-replay")
		}
		if got.Status != "ONLINE" || !got.Replayed || got.ExecutionID != "execution-1" {
			t.Fatalf("replayed status event = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replayed status event")
	}
}

func TestInProcessClientStreamConsoleOutputDeliversInjectedConsoleLines(t *testing.T) {
	client, n := newSupervisorBackedTestClient(t)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	stream, errStream := client.StreamConsoleOutput(ctx, "srv-console")
	if errStream != nil {
		t.Fatalf("StreamConsoleOutput: %v", errStream)
	}
	replay := <-stream
	if !replay.ResetBuffer || replay.ProcessID != "srv-console" {
		t.Fatalf("initial console chunk = %+v, want reset replay for srv-console", replay)
	}
	select {
	case chunk, open := <-stream:
		if !open {
			t.Fatal("healthy offline console stream closed after its reset replay")
		}
		t.Fatalf("unexpected offline console chunk = %+v", chunk)
	case <-time.After(50 * time.Millisecond):
	}

	errSend := n.SendConsoleOutput("srv-console", "hello remote console")
	if errSend != nil {
		t.Fatalf("SendConsoleOutput: %v", errSend)
	}

	select {
	case chunk, ok := <-stream:
		if !ok {
			t.Fatal("stream closed before console chunk arrived")
		}
		if chunk.ProcessID != "srv-console" {
			t.Fatalf("chunk.ProcessID = %q, want %q", chunk.ProcessID, "srv-console")
		}
		if chunk.Data == "" {
			t.Fatal("chunk.Data should not be empty")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for console chunk")
	}
}

func TestNewInProcessClientWithNilNodeReturnsErrorsFromMethods(t *testing.T) {
	client := NewInProcessClient("node-nil", nil)
	if client.ID() != "node-nil" {
		t.Fatalf("ID() = %q, want %q", client.ID(), "node-nil")
	}

	errPing := client.Ping(t.Context())
	if !errors.Is(errPing, ErrNodeNil) {
		t.Fatalf("Ping err = %v, want ErrNodeNil", errPing)
	}

	_, errList := client.ListFiles(t.Context(), "/tmp", "")
	if !errors.Is(errList, ErrNodeNil) {
		t.Fatalf("ListFiles err = %v, want ErrNodeNil", errList)
	}
}
