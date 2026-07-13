package events

import (
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestRemoteEventBridgeRepublishReliableLifecycle(t *testing.T) {
	bus := eventbus.Get()
	statusEvents := bus.SubscribeReliable(eventbus.TopicGameServerStatusChanged)
	defer bus.Unsubscribe(eventbus.TopicGameServerStatusChanged, statusEvents)

	bridge := &RemoteEventBridge{bus: bus}
	previous := make(map[string]string)
	cursors := make(map[string]processLifecycleCursor)
	event := node.Event{
		Type:               node.EventTypeProcessStatus,
		ProcessID:          "server-1",
		OldStatus:          xylona.Status_ONLINE.String(),
		Status:             xylona.Status_OFFLINE.String(),
		ExecutionID:        "execution-1",
		TransitionSequence: 2,
		ExitCode:           17,
		ExitCodeKnown:      true,
		Replayed:           true,
	}

	bridge.republish("node-1", event, previous, cursors)
	raw := <-statusEvents
	got, ok := raw.(eventbus.StatusChangedEvent)
	if !ok {
		t.Fatalf("event type = %T", raw)
	}
	if got.OldStatus != xylona.Status_ONLINE.String() || got.NewStatus != xylona.Status_OFFLINE.String() {
		t.Fatalf("statuses = %s -> %s", got.OldStatus, got.NewStatus)
	}
	if got.ExecutionID != "execution-1" || got.TransitionSequence != 2 || !got.ExitCodeKnown || got.ExitCode != 17 || !got.Replayed {
		t.Fatalf("lifecycle metadata = %+v", got)
	}

	bridge.republish("node-1", event, previous, cursors)
	select {
	case duplicate := <-statusEvents:
		t.Fatalf("duplicate replay was republished: %+v", duplicate)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRemoteEventBridgeRetainsLegacyPreviousStatus(t *testing.T) {
	bus := eventbus.Get()
	statusEvents := bus.SubscribeReliable(eventbus.TopicGameServerStatusChanged)
	defer bus.Unsubscribe(eventbus.TopicGameServerStatusChanged, statusEvents)

	bridge := &RemoteEventBridge{bus: bus}
	previous := make(map[string]string)
	cursors := make(map[string]processLifecycleCursor)

	bridge.republish("node-1", node.Event{
		Type:      node.EventTypeProcessStatus,
		ProcessID: "server-legacy",
		Status:    xylona.Status_ONLINE.String(),
	}, previous, cursors)
	<-statusEvents

	bridge.republish("node-1", node.Event{
		Type:      node.EventTypeProcessStatus,
		ProcessID: "server-legacy",
		Status:    xylona.Status_OFFLINE.String(),
	}, previous, cursors)
	raw := <-statusEvents
	got, ok := raw.(eventbus.StatusChangedEvent)
	if !ok {
		t.Fatalf("event type = %T", raw)
	}
	if got.OldStatus != xylona.Status_ONLINE.String() {
		t.Fatalf("OldStatus = %q, want ONLINE", got.OldStatus)
	}
}
