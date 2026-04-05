package actions

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestHandleAlertEventRepublishesFederatedEvents(t *testing.T) {
	engine := &FederationSyncEngine{}
	bus := eventbus.Get()
	ts := time.Date(2026, 3, 24, 20, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		topic     string
		event     *xylona.FederationAlertEvent
		assertion func(t *testing.T, got any)
	}{
		{
			name:  "crash event",
			topic: eventbus.TopicGameServerCrashed,
			event: &xylona.FederationAlertEvent{
				EventType:    xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
				ServerId:     new("srv-1"),
				ServerNodeId: new("node-1"),
				EventData:    `{"exit_code":137}`,
				Timestamp:    timestamppb.New(ts),
			},
			assertion: func(t *testing.T, got any) {
				t.Helper()
				event, ok := got.(eventbus.ServerCrashedEvent)
				if !ok {
					t.Fatalf("received event type = %T, want ServerCrashedEvent", got)
				}
				if !event.Federated {
					t.Fatal("ServerCrashedEvent.Federated = false, want true")
				}
				if !event.Timestamp.Equal(ts) {
					t.Fatalf("ServerCrashedEvent.Timestamp = %v, want %v", event.Timestamp, ts)
				}
			},
		},
		{
			name:  "status event",
			topic: eventbus.TopicGameServerStatusChanged,
			event: &xylona.FederationAlertEvent{
				EventType:    xylona.AlertEventType_ALERT_EVENT_TYPE_STATUS_CHANGE,
				ServerId:     new("srv-2"),
				ServerNodeId: new("node-2"),
				EventData:    `{"old_status":"ONLINE","new_status":"OFFLINE"}`,
			},
			assertion: func(t *testing.T, got any) {
				t.Helper()
				event, ok := got.(eventbus.StatusChangedEvent)
				if !ok {
					t.Fatalf("received event type = %T, want StatusChangedEvent", got)
				}
				if !event.Federated {
					t.Fatal("StatusChangedEvent.Federated = false, want true")
				}
			},
		},
		{
			name:  "server threshold event",
			topic: eventbus.TopicGameServerCPUThreshold,
			event: &xylona.FederationAlertEvent{
				EventType:    xylona.AlertEventType_ALERT_EVENT_TYPE_CPU_THRESHOLD,
				ServerId:     new("srv-3"),
				ServerNodeId: new("node-3"),
				EventData:    `{"current_value":95.5,"threshold":90,"direction":"entered"}`,
			},
			assertion: func(t *testing.T, got any) {
				t.Helper()
				event, ok := got.(eventbus.ThresholdEvent)
				if !ok {
					t.Fatalf("received event type = %T, want ThresholdEvent", got)
				}
				if !event.Federated {
					t.Fatal("ThresholdEvent.Federated = false, want true")
				}
			},
		},
		{
			name:  "node threshold event",
			topic: eventbus.TopicNodeCPUThreshold,
			event: &xylona.FederationAlertEvent{
				EventType: xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD,
				NodeId:    new("node-4"),
				EventData: `{"current_value":91.5,"threshold":90,"direction":"entered"}`,
			},
			assertion: func(t *testing.T, got any) {
				t.Helper()
				event, ok := got.(eventbus.NodeThresholdEvent)
				if !ok {
					t.Fatalf("received event type = %T, want NodeThresholdEvent", got)
				}
				if !event.Federated {
					t.Fatal("NodeThresholdEvent.Federated = false, want true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := bus.SubscribeReliable(tt.topic)
			defer bus.Unsubscribe(tt.topic, ch)

			engine.handleAlertEvent("peer-1", tt.event)

			select {
			case got := <-ch:
				tt.assertion(t, got)
			case <-time.After(2 * time.Second):
				t.Fatalf("timed out waiting for republished event on topic %q", tt.topic)
			}
		})
	}
}
