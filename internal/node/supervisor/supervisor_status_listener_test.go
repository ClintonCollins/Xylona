package supervisor

import (
	"sync"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func newTestCommand(t *testing.T) *Command {
	t.Helper()
	ctx := t.Context()
	return &Command{
		ID:                  "test-id",
		RWMutex:             &sync.RWMutex{},
		outputListeners:     make(map[string]chan *xylona.Message),
		outputListenersLock: &sync.RWMutex{},
		statusListeners:     make(map[string]chan *xylona.GameServerStatusUpdate),
		statusListenersLock: &sync.RWMutex{},
		instanceCtx:         ctx,
		status:              xylona.Status_OFFLINE,
		toggleOutputType:    make(chan struct{}),
	}
}

func TestAddStatusListener_ReceivesStatusChanges(t *testing.T) {
	cmd := newTestCommand(t)
	cmd.gameServerName = "Alpha"
	ch := make(chan *xylona.GameServerStatusUpdate, 8)
	cmd.AddStatusListener("test-listener", ch)

	cmd.sendJobStatusNotification(xylona.Status_OFFLINE, xylona.Status_ONLINE)

	select {
	case update := <-ch:
		if update.GetGameServerId() != "test-id" {
			t.Errorf("expected server ID %q, got %q", "test-id", update.GetGameServerId())
		}
		if update.GetStatus() != xylona.Status_ONLINE {
			t.Errorf("expected status ONLINE, got %v", update.GetStatus())
		}
		if update.GetGameServerName() != "Alpha" {
			t.Errorf("expected game server name %q, got %q", "Alpha", update.GetGameServerName())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for status update")
	}
}

func TestStatusListener_DoesNotReceiveConsoleOutput(t *testing.T) {
	cmd := newTestCommand(t)
	ch := make(chan *xylona.GameServerStatusUpdate, 8)
	cmd.AddStatusListener("test-listener", ch)

	cmd.handleOutputListeners(&xylona.Message{
		Type: xylona.Message_GameServerConsole,
		GameServerConsoleOutput: &xylona.GameServerConsoleOutput{
			GameServerId: "test-id",
			Output:       "some console output",
		},
	})

	select {
	case <-ch:
		t.Fatal("status listener should not receive console output")
	case <-time.After(200 * time.Millisecond):
		// Expected — no message received
	}
}

func TestRemoveStatusListener_StopsDelivery(t *testing.T) {
	cmd := newTestCommand(t)
	ch := make(chan *xylona.GameServerStatusUpdate, 8)
	cmd.AddStatusListener("test-listener", ch)
	cmd.RemoveStatusListener("test-listener")

	cmd.sendJobStatusNotification(xylona.Status_ONLINE, xylona.Status_OFFLINE)

	select {
	case <-ch:
		t.Fatal("removed listener should not receive updates")
	case <-time.After(200 * time.Millisecond):
		// Expected — no message received
	}
}

func TestStatusNotificationRetainsAuthoritativeLifecycleMetadata(t *testing.T) {
	cmd := newTestCommand(t)
	cmd.executionID = "execution-1"
	cmd.intentionalStop.Store(true)
	hookEvents := make(chan eventbus.StatusChangedEvent, 2)
	cmd.statusEventHook = func(event eventbus.StatusChangedEvent) {
		hookEvents <- event
	}

	cmd.sendJobStatusNotification(xylona.Status_OFFLINE, xylona.Status_ONLINE)
	started := cmd.Lifecycle()
	if started.ExecutionID != "execution-1" || started.PreviousStatus != xylona.Status_OFFLINE || started.TransitionSequence != 1 {
		t.Fatalf("started lifecycle = %+v", started)
	}
	if started.ExitCodeKnown {
		t.Fatal("start transition unexpectedly retained an exit code")
	}

	cmd.sendJobStatusNotificationWithExit(xylona.Status_ONLINE, 23)
	stopped := cmd.Lifecycle()
	if stopped.PreviousStatus != xylona.Status_ONLINE || stopped.TransitionSequence != 2 {
		t.Fatalf("stopped lifecycle = %+v", stopped)
	}
	if !stopped.IntentionalStop || !stopped.ExitCodeKnown || stopped.ExitCode != 23 {
		t.Fatalf("stopped terminal metadata = %+v", stopped)
	}
	<-hookEvents
	terminalEvent := <-hookEvents
	if terminalEvent.ExecutionID != "execution-1" || terminalEvent.TransitionSequence != 2 ||
		!terminalEvent.ExitCodeKnown || terminalEvent.ExitCode != 23 {
		t.Fatalf("direct lifecycle hook event = %+v", terminalEvent)
	}
}
