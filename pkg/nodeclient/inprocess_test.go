package nodeclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/supervisor"
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

func TestInProcessClientIDReturnsConfiguredID(t *testing.T) {
	client, _ := newTestClient(t)
	if got := client.ID(); got != "node-A" {
		t.Fatalf("ID() = %q, want %q", got, "node-A")
	}
}

func TestInProcessClientPingSucceedsWithLiveContext(t *testing.T) {
	client, _ := newTestClient(t)
	errPing := client.Ping(t.Context())
	if errPing != nil {
		t.Fatalf("Ping() unexpected error: %v", errPing)
	}
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

	_, errStart := client.StartProcess(t.Context(), node.ProcessConfig{ID: "srv-1", BaseCommand: "echo"}, 0)
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

func TestInProcessClientStreamEventsMirrorsStatusEventBusUpdates(t *testing.T) {
	client, _ := newTestClient(t)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	stream, errStream := client.StreamEvents(ctx)
	if errStream != nil {
		t.Fatalf("StreamEvents: %v", errStream)
	}

	eventbus.Get().Publish(eventbus.TopicGameServerStatusChanged, eventbus.StatusChangedEvent{
		ServerID:  "srv-bridge",
		NewStatus: xylona.Status_ONLINE.String(),
	})

	select {
	case got, ok := <-stream:
		if !ok {
			t.Fatal("stream closed before status event arrived")
		}
		if got.Type != node.EventTypeProcessStatus {
			t.Fatalf("got.Type = %v, want %v", got.Type, node.EventTypeProcessStatus)
		}
		if got.ProcessID != "srv-bridge" {
			t.Fatalf("got.ProcessID = %q, want %q", got.ProcessID, "srv-bridge")
		}
		if got.Status != xylona.Status_ONLINE.String() {
			t.Fatalf("got.Status = %q, want %q", got.Status, xylona.Status_ONLINE.String())
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bridged status event")
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
