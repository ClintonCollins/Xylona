package node

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestNodeEmitsAuthoritativeProcessLifecycle(t *testing.T) {
	supervisorInst, errNew := supervisor.New(context.Background())
	if errNew != nil {
		t.Fatalf("supervisor.New() error = %v", errNew)
	}
	nodeInst := New(t.Context(), supervisorInst, nil)
	subscription := nodeInst.Events().Subscribe()
	defer nodeInst.Events().Unsubscribe(subscription)

	executable, errExecutable := os.Executable()
	if errExecutable != nil {
		t.Fatalf("os.Executable() error = %v", errExecutable)
	}
	_, errStart := nodeInst.StartProcess(ProcessConfig{
		ID:               "lifecycle-test",
		Name:             "Lifecycle Test",
		BaseCommand:      executable,
		Args:             []string{"-test.run=^$"},
		WorkingDirectory: t.TempDir(),
	}, xylona.Status_ONLINE)
	if errStart != nil {
		t.Fatalf("StartProcess() error = %v", errStart)
	}

	started := receiveLifecycleEvent(t, subscription)
	if started.OldStatus != xylona.Status_OFFLINE.String() || started.Status != xylona.Status_ONLINE.String() {
		t.Fatalf("started statuses = %s -> %s", started.OldStatus, started.Status)
	}
	if started.ExecutionID == "" || started.TransitionSequence != 1 || started.ExitCodeKnown {
		t.Fatalf("started lifecycle = %+v", started)
	}

	stopped := receiveLifecycleEvent(t, subscription)
	if stopped.OldStatus != xylona.Status_ONLINE.String() || stopped.Status != xylona.Status_OFFLINE.String() {
		t.Fatalf("stopped statuses = %s -> %s", stopped.OldStatus, stopped.Status)
	}
	if stopped.ExecutionID != started.ExecutionID || stopped.TransitionSequence != 2 || !stopped.ExitCodeKnown {
		t.Fatalf("stopped lifecycle = %+v", stopped)
	}

	snapshot, found, errSnapshot := nodeInst.GetProcessSnapshot("lifecycle-test")
	if errSnapshot != nil || !found || snapshot == nil {
		t.Fatalf("GetProcessSnapshot() = (%+v, %t, %v)", snapshot, found, errSnapshot)
	}
	if snapshot.ExecutionID != stopped.ExecutionID || snapshot.PreviousStatus != xylona.Status_ONLINE.String() ||
		snapshot.TransitionSequence != 2 || !snapshot.ExitCodeKnown {
		t.Fatalf("terminal snapshot = %+v", snapshot)
	}
}

func receiveLifecycleEvent(t *testing.T, events <-chan Event) Event {
	t.Helper()
	select {
	case event, open := <-events:
		if !open {
			t.Fatal("lifecycle event stream closed")
		}
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for lifecycle event")
		return Event{}
	}
}
