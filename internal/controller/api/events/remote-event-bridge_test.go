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
	crashEvents := bus.SubscribeReliable(eventbus.TopicGameServerCrashed)
	defer bus.Unsubscribe(eventbus.TopicGameServerCrashed, crashEvents)

	bridge := &RemoteEventBridge{bus: bus}
	previous := make(map[string]string)
	cursors := make(map[string]processLifecycleCursor)
	occurredAt := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
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
		Timestamp:          occurredAt,
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
	if !got.OccurredAt.Equal(occurredAt) {
		t.Fatalf("lifecycle occurrence time = %v, want %v", got.OccurredAt, occurredAt)
	}
	rawCrash := <-crashEvents
	crash, ok := rawCrash.(eventbus.ServerCrashedEvent)
	if !ok {
		t.Fatalf("crash event type = %T", rawCrash)
	}
	if crash.ServerID != "server-1" || crash.ServerNodeID != "node-1" || crash.ExitCode != 17 {
		t.Fatalf("crash event = %+v", crash)
	}

	bridge.republish("node-1", event, previous, cursors)
	select {
	case duplicate := <-statusEvents:
		t.Fatalf("duplicate replay was republished: %+v", duplicate)
	case <-time.After(50 * time.Millisecond):
	}
	select {
	case duplicate := <-crashEvents:
		t.Fatalf("duplicate crash was republished: %+v", duplicate)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRemoteEventBridgeRepublishesUnknownExitCrashOnce(t *testing.T) {
	bus := eventbus.Get()
	crashEvents := bus.SubscribeReliable(eventbus.TopicGameServerCrashed)
	defer bus.Unsubscribe(eventbus.TopicGameServerCrashed, crashEvents)
	statusEvents := bus.SubscribeReliable(eventbus.TopicGameServerStatusChanged)
	defer bus.Unsubscribe(eventbus.TopicGameServerStatusChanged, statusEvents)
	bridge := &RemoteEventBridge{bus: bus}
	previous := make(map[string]string)
	cursors := make(map[string]processLifecycleCursor)
	event := node.Event{
		Type:               node.EventTypeProcessStatus,
		ProcessID:          "server-signal-exit",
		OldStatus:          xylona.Status_ONLINE.String(),
		Status:             xylona.Status_OFFLINE.String(),
		ExecutionID:        "execution-signal-exit",
		TransitionSequence: 2,
		ExitCode:           -1,
		ExitCodeKnown:      true,
	}

	bridge.republish("node-remote", event, previous, cursors)
	<-statusEvents
	rawCrash := <-crashEvents
	crash, ok := rawCrash.(eventbus.ServerCrashedEvent)
	if !ok || crash.ServerID != event.ProcessID || crash.ServerNodeID != "node-remote" || crash.ExitCode != -1 {
		t.Fatalf("unknown-exit crash = %+v", rawCrash)
	}

	bridge.republish("node-remote", event, previous, cursors)
	select {
	case duplicate := <-crashEvents:
		t.Fatalf("duplicate unknown-exit crash was republished: %+v", duplicate)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRemoteEventBridgeDoesNotTreatOperationFailureAsGameServerCrash(t *testing.T) {
	bus := eventbus.Get()
	crashEvents := bus.SubscribeReliable(eventbus.TopicGameServerCrashed)
	defer bus.Unsubscribe(eventbus.TopicGameServerCrashed, crashEvents)
	statusEvents := bus.SubscribeReliable(eventbus.TopicGameServerStatusChanged)
	defer bus.Unsubscribe(eventbus.TopicGameServerStatusChanged, statusEvents)
	bridge := &RemoteEventBridge{bus: bus}
	bridge.republish("node-remote", node.Event{
		Type:               node.EventTypeProcessStatus,
		ProcessID:          "server-update-failure",
		OldStatus:          xylona.Status_UPDATING.String(),
		Status:             xylona.Status_OFFLINE.String(),
		ExecutionID:        "execution-update-failure",
		TransitionSequence: 2,
		ExitCode:           1,
		ExitCodeKnown:      true,
	}, make(map[string]string), make(map[string]processLifecycleCursor))
	<-statusEvents
	select {
	case crash := <-crashEvents:
		t.Fatalf("operation failure was republished as a crash: %+v", crash)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRemoteEventBridgeRetainsLegacyPreviousStatus(t *testing.T) {
	bus := eventbus.Get()
	statusEvents := bus.SubscribeReliable(eventbus.TopicGameServerStatusChanged)
	defer bus.Unsubscribe(eventbus.TopicGameServerStatusChanged, statusEvents)
	crashEvents := bus.SubscribeReliable(eventbus.TopicGameServerCrashed)
	defer bus.Unsubscribe(eventbus.TopicGameServerCrashed, crashEvents)

	bridge := &RemoteEventBridge{bus: bus}
	previous := make(map[string]string)
	cursors := make(map[string]processLifecycleCursor)

	bridge.republish("node-1", node.Event{
		Type:      node.EventTypeProcessStatus,
		ProcessID: "server-legacy",
		Status:    xylona.Status_ONLINE.String(),
	}, previous, cursors)
	<-statusEvents

	offlineEvent := node.Event{
		Type:          node.EventTypeProcessStatus,
		ProcessID:     "server-legacy",
		Status:        xylona.Status_OFFLINE.String(),
		ExitCode:      9,
		ExitCodeKnown: true,
	}
	bridge.republish("node-1", offlineEvent, previous, cursors)
	raw := <-statusEvents
	got, ok := raw.(eventbus.StatusChangedEvent)
	if !ok {
		t.Fatalf("event type = %T", raw)
	}
	if got.OldStatus != xylona.Status_ONLINE.String() {
		t.Fatalf("OldStatus = %q, want ONLINE", got.OldStatus)
	}
	rawCrash := <-crashEvents
	crash, ok := rawCrash.(eventbus.ServerCrashedEvent)
	if !ok || crash.ServerID != "server-legacy" || crash.ExitCode != 9 {
		t.Fatalf("legacy crash event = %+v", rawCrash)
	}

	bridge.republish("node-1", offlineEvent, previous, cursors)
	<-statusEvents
	select {
	case duplicate := <-crashEvents:
		t.Fatalf("duplicate legacy crash was republished: %+v", duplicate)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCurrentNodeOwnsProcess(t *testing.T) {
	tests := []struct {
		name         string
		serverNodeID string
		eventNodeID  string
		want         bool
	}{
		{name: "current owner", serverNodeID: "node-a", eventNodeID: "node-a", want: true},
		{name: "former node after reassignment", serverNodeID: "node-b", eventNodeID: "node-a", want: false},
		{name: "missing server", serverNodeID: "", eventNodeID: "node-a", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := currentNodeOwnsProcess(test.serverNodeID, test.eventNodeID)
			if got != test.want {
				t.Fatalf("currentNodeOwnsProcess(%q, %q) = %v, want %v", test.serverNodeID, test.eventNodeID, got, test.want)
			}
		})
	}
}
