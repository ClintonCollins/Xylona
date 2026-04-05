package alerts

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

type alertTestCrashEventData struct {
	ExitCode int `json:"exit_code,omitempty"`
}

type alertTestStatusEventData struct {
	OldStatus string `json:"old_status,omitempty"`
	NewStatus string `json:"new_status,omitempty"`
}

type alertTestThresholdEventData struct {
	CurrentValue float64 `json:"current_value"`
	Threshold    float64 `json:"threshold"`
	Direction    string  `json:"direction"`
}

func TestSerializeFederationAlertEventServerCrashed(t *testing.T) {
	ts := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	msg := eventbus.ServerCrashedEvent{
		ServerID:     "srv-1",
		ServerNodeID: "node-1",
		ExitCode:     42,
		Timestamp:    ts,
	}

	evt, ok := SerializeFederationAlertEvent(eventbus.TopicGameServerCrashed, msg)
	if !ok {
		t.Fatal("SerializeFederationAlertEvent returned false for valid crash event")
	}

	if evt.GetEventType() != xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH {
		t.Errorf("EventType = %v, want ALERT_EVENT_TYPE_CRASH", evt.GetEventType())
	}
	if evt.GetServerId() != "srv-1" {
		t.Errorf("ServerId = %q, want %q", evt.GetServerId(), "srv-1")
	}
	if evt.GetServerNodeId() != "node-1" {
		t.Errorf("ServerNodeId = %q, want %q", evt.GetServerNodeId(), "node-1")
	}
	if evt.GetNodeId() != "" {
		t.Errorf("NodeId = %q, want empty for server event", evt.GetNodeId())
	}

	var data alertTestCrashEventData
	errUnmarshal := json.Unmarshal([]byte(evt.GetEventData()), &data)
	if errUnmarshal != nil {
		t.Fatalf("Failed to unmarshal event_data: %v", errUnmarshal)
	}
	if data.ExitCode != 42 {
		t.Errorf("event_data exit_code = %d, want 42", data.ExitCode)
	}

	if evt.GetTimestamp() == nil {
		t.Fatal("Timestamp is nil")
	}
	if !evt.GetTimestamp().AsTime().Equal(ts) {
		t.Errorf("Timestamp = %v, want %v", evt.GetTimestamp().AsTime(), ts)
	}
}

func TestSerializeFederationAlertEventStatusChanged(t *testing.T) {
	msg := eventbus.StatusChangedEvent{
		ServerID:     "srv-2",
		ServerNodeID: "node-2",
		OldStatus:    "ONLINE",
		NewStatus:    "OFFLINE",
	}

	evt, ok := SerializeFederationAlertEvent(eventbus.TopicGameServerStatusChanged, msg)
	if !ok {
		t.Fatal("SerializeFederationAlertEvent returned false for valid status event")
	}

	if evt.GetEventType() != xylona.AlertEventType_ALERT_EVENT_TYPE_STATUS_CHANGE {
		t.Errorf("EventType = %v, want ALERT_EVENT_TYPE_STATUS_CHANGE", evt.GetEventType())
	}
	if evt.GetServerId() != "srv-2" {
		t.Errorf("ServerId = %q, want %q", evt.GetServerId(), "srv-2")
	}

	var data alertTestStatusEventData
	errUnmarshal := json.Unmarshal([]byte(evt.GetEventData()), &data)
	if errUnmarshal != nil {
		t.Fatalf("Failed to unmarshal event_data: %v", errUnmarshal)
	}
	if data.OldStatus != "ONLINE" {
		t.Errorf("event_data old_status = %q, want %q", data.OldStatus, "ONLINE")
	}
	if data.NewStatus != "OFFLINE" {
		t.Errorf("event_data new_status = %q, want %q", data.NewStatus, "OFFLINE")
	}
}

func TestSerializeFederationAlertEventServerThreshold(t *testing.T) {
	cases := []struct {
		name      string
		topic     string
		wantType  xylona.AlertEventType
		direction eventbus.ThresholdDirection
	}{
		{
			name:      "CPU threshold",
			topic:     eventbus.TopicGameServerCPUThreshold,
			wantType:  xylona.AlertEventType_ALERT_EVENT_TYPE_CPU_THRESHOLD,
			direction: eventbus.ThresholdEntered,
		},
		{
			name:      "Memory threshold",
			topic:     eventbus.TopicGameServerMemoryThreshold,
			wantType:  xylona.AlertEventType_ALERT_EVENT_TYPE_MEMORY_THRESHOLD,
			direction: eventbus.ThresholdResolved,
		},
		{
			name:      "Disk threshold",
			topic:     eventbus.TopicGameServerDiskThreshold,
			wantType:  xylona.AlertEventType_ALERT_EVENT_TYPE_DISK_THRESHOLD,
			direction: eventbus.ThresholdEntered,
		},
		{
			name:      "Player threshold",
			topic:     eventbus.TopicGameServerPlayerThreshold,
			wantType:  xylona.AlertEventType_ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD,
			direction: eventbus.ThresholdResolved,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := eventbus.ThresholdEvent{
				ServerID:     "srv-thr",
				ServerNodeID: "node-thr",
				CurrentValue: 85.5,
				Threshold:    80.0,
				Direction:    tc.direction,
			}

			evt, ok := SerializeFederationAlertEvent(tc.topic, msg)
			if !ok {
				t.Fatal("SerializeFederationAlertEvent returned false for valid threshold event")
			}

			if evt.GetEventType() != tc.wantType {
				t.Errorf("EventType = %v, want %v", evt.GetEventType(), tc.wantType)
			}
			if evt.GetServerId() != "srv-thr" {
				t.Errorf("ServerId = %q, want %q", evt.GetServerId(), "srv-thr")
			}
			if evt.GetNodeId() != "" {
				t.Errorf("NodeId = %q, want empty for server event", evt.GetNodeId())
			}

			var data alertTestThresholdEventData
			errUnmarshal := json.Unmarshal([]byte(evt.GetEventData()), &data)
			if errUnmarshal != nil {
				t.Fatalf("Failed to unmarshal event_data: %v", errUnmarshal)
			}
			if math.Abs(data.CurrentValue-85.5) > 0.01 {
				t.Errorf("event_data current_value = %v, want 85.5", data.CurrentValue)
			}
			if math.Abs(data.Threshold-80.0) > 0.01 {
				t.Errorf("event_data threshold = %v, want 80.0", data.Threshold)
			}
			if data.Direction != string(tc.direction) {
				t.Errorf("event_data direction = %q, want %q", data.Direction, tc.direction)
			}
		})
	}
}

func TestSerializeFederationAlertEventNodeThreshold(t *testing.T) {
	cases := []struct {
		name     string
		topic    string
		wantType xylona.AlertEventType
	}{
		{
			name:     "Node CPU threshold",
			topic:    eventbus.TopicNodeCPUThreshold,
			wantType: xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD,
		},
		{
			name:     "Node memory threshold",
			topic:    eventbus.TopicNodeMemoryThreshold,
			wantType: xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD,
		},
		{
			name:     "Node disk threshold",
			topic:    eventbus.TopicNodeDiskThreshold,
			wantType: xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_DISK_THRESHOLD,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := eventbus.NodeThresholdEvent{
				NodeID:       "node-42",
				CurrentValue: 92.1,
				Threshold:    90.0,
				Direction:    eventbus.ThresholdEntered,
			}

			evt, ok := SerializeFederationAlertEvent(tc.topic, msg)
			if !ok {
				t.Fatal("SerializeFederationAlertEvent returned false for valid node threshold event")
			}

			if evt.GetEventType() != tc.wantType {
				t.Errorf("EventType = %v, want %v", evt.GetEventType(), tc.wantType)
			}
			if evt.GetNodeId() != "node-42" {
				t.Errorf("NodeId = %q, want %q", evt.GetNodeId(), "node-42")
			}
			if evt.GetServerId() != "" {
				t.Errorf("ServerId = %q, want empty for node event", evt.GetServerId())
			}
			if evt.GetServerNodeId() != "" {
				t.Errorf("ServerNodeId = %q, want empty for node event", evt.GetServerNodeId())
			}

			var data alertTestThresholdEventData
			errUnmarshal := json.Unmarshal([]byte(evt.GetEventData()), &data)
			if errUnmarshal != nil {
				t.Fatalf("Failed to unmarshal event_data: %v", errUnmarshal)
			}
			if math.Abs(data.CurrentValue-92.1) > 0.01 {
				t.Errorf("event_data current_value = %v, want 92.1", data.CurrentValue)
			}
		})
	}
}

func TestSerializeFederationAlertEventThresholdZeroValuesPreserved(t *testing.T) {
	msg := eventbus.ThresholdEvent{
		ServerID:     "srv-zero",
		ServerNodeID: "node-zero",
		CurrentValue: 0,
		Threshold:    0,
		Direction:    eventbus.ThresholdEntered,
	}

	evt, ok := SerializeFederationAlertEvent(eventbus.TopicGameServerCPUThreshold, msg)
	if !ok {
		t.Fatal("SerializeFederationAlertEvent returned false for zero-valued threshold event")
	}

	for _, fragment := range []string{`"current_value":0`, `"threshold":0`} {
		if !strings.Contains(evt.GetEventData(), fragment) {
			t.Fatalf("EventData = %q, want fragment %q", evt.GetEventData(), fragment)
		}
	}
}

func TestSerializeFederationAlertEventUnknownTopic(t *testing.T) {
	_, ok := SerializeFederationAlertEvent("unknown.topic", "anything")
	if ok {
		t.Error("SerializeFederationAlertEvent returned true for unknown topic, want false")
	}
}

func TestSerializeFederationAlertEventWrongMessageType(t *testing.T) {
	_, ok := SerializeFederationAlertEvent(eventbus.TopicGameServerCrashed, "not-a-real-event")
	if ok {
		t.Error("SerializeFederationAlertEvent returned true for wrong message type, want false")
	}
}

func TestSerializeFederationAlertEventWrongNodeMessageType(t *testing.T) {
	_, ok := SerializeFederationAlertEvent(eventbus.TopicNodeCPUThreshold, eventbus.ThresholdEvent{})
	if ok {
		t.Error("SerializeFederationAlertEvent returned true for wrong node message type, want false")
	}
}

func TestRoundTripServerCrashed(t *testing.T) {
	localBus := eventbus.Get()
	ch := localBus.SubscribeReliable(eventbus.TopicGameServerCrashed)
	defer localBus.Unsubscribe(eventbus.TopicGameServerCrashed, ch)

	ts := time.Date(2026, 3, 24, 15, 30, 0, 0, time.UTC)
	original := eventbus.ServerCrashedEvent{
		ServerID:     "srv-rt-1",
		ServerNodeID: "node-rt-1",
		ExitCode:     137,
		Timestamp:    ts,
	}

	protoEvt, ok := SerializeFederationAlertEvent(eventbus.TopicGameServerCrashed, original)
	if !ok {
		t.Fatal("SerializeFederationAlertEvent returned false")
	}

	RepublishFederationAlertEvent(localBus, protoEvt)

	select {
	case received := <-ch:
		evt, isType := received.(eventbus.ServerCrashedEvent)
		if !isType {
			t.Fatalf("received event type = %T, want ServerCrashedEvent", received)
		}
		if evt.ServerID != original.ServerID {
			t.Errorf("ServerID = %q, want %q", evt.ServerID, original.ServerID)
		}
		if evt.ServerNodeID != original.ServerNodeID {
			t.Errorf("ServerNodeID = %q, want %q", evt.ServerNodeID, original.ServerNodeID)
		}
		if evt.ExitCode != original.ExitCode {
			t.Errorf("ExitCode = %d, want %d", evt.ExitCode, original.ExitCode)
		}
		if !evt.Timestamp.Equal(original.Timestamp) {
			t.Errorf("Timestamp = %v, want %v", evt.Timestamp, original.Timestamp)
		}
	default:
		t.Fatal("no event received on the event bus after republish")
	}
}

func TestRoundTripStatusChanged(t *testing.T) {
	localBus := eventbus.Get()
	ch := localBus.SubscribeReliable(eventbus.TopicGameServerStatusChanged)
	defer localBus.Unsubscribe(eventbus.TopicGameServerStatusChanged, ch)

	original := eventbus.StatusChangedEvent{
		ServerID:     "srv-rt-2",
		ServerNodeID: "node-rt-2",
		OldStatus:    "ONLINE",
		NewStatus:    "CRASHED",
	}

	protoEvt, ok := SerializeFederationAlertEvent(eventbus.TopicGameServerStatusChanged, original)
	if !ok {
		t.Fatal("SerializeFederationAlertEvent returned false")
	}

	RepublishFederationAlertEvent(localBus, protoEvt)

	select {
	case received := <-ch:
		evt, isType := received.(eventbus.StatusChangedEvent)
		if !isType {
			t.Fatalf("received event type = %T, want StatusChangedEvent", received)
		}
		if evt.ServerID != original.ServerID {
			t.Errorf("ServerID = %q, want %q", evt.ServerID, original.ServerID)
		}
		if evt.ServerNodeID != original.ServerNodeID {
			t.Errorf("ServerNodeID = %q, want %q", evt.ServerNodeID, original.ServerNodeID)
		}
		if evt.OldStatus != original.OldStatus {
			t.Errorf("OldStatus = %q, want %q", evt.OldStatus, original.OldStatus)
		}
		if evt.NewStatus != original.NewStatus {
			t.Errorf("NewStatus = %q, want %q", evt.NewStatus, original.NewStatus)
		}
	default:
		t.Fatal("no event received on the event bus after republish")
	}
}

func TestRoundTripServerThreshold(t *testing.T) {
	localBus := eventbus.Get()
	ch := localBus.SubscribeReliable(eventbus.TopicGameServerCPUThreshold)
	defer localBus.Unsubscribe(eventbus.TopicGameServerCPUThreshold, ch)

	original := eventbus.ThresholdEvent{
		ServerID:     "srv-rt-3",
		ServerNodeID: "node-rt-3",
		CurrentValue: 95.5,
		Threshold:    90.0,
		Direction:    eventbus.ThresholdEntered,
	}

	protoEvt, ok := SerializeFederationAlertEvent(eventbus.TopicGameServerCPUThreshold, original)
	if !ok {
		t.Fatal("SerializeFederationAlertEvent returned false")
	}

	RepublishFederationAlertEvent(localBus, protoEvt)

	select {
	case received := <-ch:
		evt, isType := received.(eventbus.ThresholdEvent)
		if !isType {
			t.Fatalf("received event type = %T, want ThresholdEvent", received)
		}
		if evt.ServerID != original.ServerID {
			t.Errorf("ServerID = %q, want %q", evt.ServerID, original.ServerID)
		}
		if evt.ServerNodeID != original.ServerNodeID {
			t.Errorf("ServerNodeID = %q, want %q", evt.ServerNodeID, original.ServerNodeID)
		}
		if math.Abs(evt.CurrentValue-original.CurrentValue) > 0.01 {
			t.Errorf("CurrentValue = %v, want %v", evt.CurrentValue, original.CurrentValue)
		}
		if math.Abs(evt.Threshold-original.Threshold) > 0.01 {
			t.Errorf("Threshold = %v, want %v", evt.Threshold, original.Threshold)
		}
		if evt.Direction != original.Direction {
			t.Errorf("Direction = %q, want %q", evt.Direction, original.Direction)
		}
	default:
		t.Fatal("no event received on the event bus after republish")
	}
}

func TestRoundTripNodeThreshold(t *testing.T) {
	localBus := eventbus.Get()
	ch := localBus.SubscribeReliable(eventbus.TopicNodeMemoryThreshold)
	defer localBus.Unsubscribe(eventbus.TopicNodeMemoryThreshold, ch)

	original := eventbus.NodeThresholdEvent{
		NodeID:       "node-rt-4",
		CurrentValue: 88.2,
		Threshold:    85.0,
		Direction:    eventbus.ThresholdResolved,
	}

	protoEvt, ok := SerializeFederationAlertEvent(eventbus.TopicNodeMemoryThreshold, original)
	if !ok {
		t.Fatal("SerializeFederationAlertEvent returned false")
	}

	RepublishFederationAlertEvent(localBus, protoEvt)

	select {
	case received := <-ch:
		evt, isType := received.(eventbus.NodeThresholdEvent)
		if !isType {
			t.Fatalf("received event type = %T, want NodeThresholdEvent", received)
		}
		if evt.NodeID != original.NodeID {
			t.Errorf("NodeID = %q, want %q", evt.NodeID, original.NodeID)
		}
		if math.Abs(evt.CurrentValue-original.CurrentValue) > 0.01 {
			t.Errorf("CurrentValue = %v, want %v", evt.CurrentValue, original.CurrentValue)
		}
		if math.Abs(evt.Threshold-original.Threshold) > 0.01 {
			t.Errorf("Threshold = %v, want %v", evt.Threshold, original.Threshold)
		}
		if evt.Direction != original.Direction {
			t.Errorf("Direction = %q, want %q", evt.Direction, original.Direction)
		}
	default:
		t.Fatal("no event received on the event bus after republish")
	}
}

func TestRepublishUnknownEventType(_ *testing.T) {
	localBus := eventbus.Get()
	evt := &xylona.FederationAlertEvent{
		EventType: xylona.AlertEventType_ALERT_EVENT_TYPE_UNSPECIFIED,
		EventData: "{}",
	}
	RepublishFederationAlertEvent(localBus, evt)
}

func TestAllAlertTopicsCovered(t *testing.T) {
	for _, topic := range AllFederationAlertTopics {
		protoType, ok := AlertTopicToProtoType[topic]
		if !ok {
			t.Errorf("topic %q has no entry in AlertTopicToProtoType", topic)
			continue
		}
		reverseTopic, reverseOK := AlertProtoTypeToTopic[protoType]
		if !reverseOK {
			t.Errorf("proto type %v has no entry in AlertProtoTypeToTopic", protoType)
			continue
		}
		if reverseTopic != topic {
			t.Errorf("round-trip for topic %q: AlertProtoTypeToTopic[%v] = %q, want %q",
				topic, protoType, reverseTopic, topic)
		}
	}
}
