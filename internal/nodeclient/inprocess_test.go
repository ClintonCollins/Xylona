package nodeclient

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
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
