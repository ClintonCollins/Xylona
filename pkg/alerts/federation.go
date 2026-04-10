package alerts

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// AlertTopicToProtoType maps event bus alert topics to their corresponding
// proto AlertEventType enum values.
var AlertTopicToProtoType = map[string]xylona.AlertEventType{
	eventbus.TopicGameServerCrashed:         xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH,
	eventbus.TopicGameServerStatusChanged:   xylona.AlertEventType_ALERT_EVENT_TYPE_STATUS_CHANGE,
	eventbus.TopicGameServerCPUThreshold:    xylona.AlertEventType_ALERT_EVENT_TYPE_CPU_THRESHOLD,
	eventbus.TopicGameServerMemoryThreshold: xylona.AlertEventType_ALERT_EVENT_TYPE_MEMORY_THRESHOLD,
	eventbus.TopicGameServerDiskThreshold:   xylona.AlertEventType_ALERT_EVENT_TYPE_DISK_THRESHOLD,
	eventbus.TopicGameServerPlayerThreshold: xylona.AlertEventType_ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD,
	eventbus.TopicNodeCPUThreshold:          xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD,
	eventbus.TopicNodeMemoryThreshold:       xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD,
	eventbus.TopicNodeDiskThreshold:         xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_DISK_THRESHOLD,
}

// AlertProtoTypeToTopic is the reverse mapping from proto AlertEventType to
// event bus topic. It is used on the receiver side to republish events.
var AlertProtoTypeToTopic = map[xylona.AlertEventType]string{
	xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH:                  eventbus.TopicGameServerCrashed,
	xylona.AlertEventType_ALERT_EVENT_TYPE_STATUS_CHANGE:          eventbus.TopicGameServerStatusChanged,
	xylona.AlertEventType_ALERT_EVENT_TYPE_CPU_THRESHOLD:          eventbus.TopicGameServerCPUThreshold,
	xylona.AlertEventType_ALERT_EVENT_TYPE_MEMORY_THRESHOLD:       eventbus.TopicGameServerMemoryThreshold,
	xylona.AlertEventType_ALERT_EVENT_TYPE_DISK_THRESHOLD:         eventbus.TopicGameServerDiskThreshold,
	xylona.AlertEventType_ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD: eventbus.TopicGameServerPlayerThreshold,
	xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD:     eventbus.TopicNodeCPUThreshold,
	xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD:  eventbus.TopicNodeMemoryThreshold,
	xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_DISK_THRESHOLD:    eventbus.TopicNodeDiskThreshold,
}

// AllFederationAlertTopics is the ordered list of all event bus topics that
// carry alert events and should be forwarded over the federation stream.
var AllFederationAlertTopics = []string{
	eventbus.TopicGameServerCrashed,
	eventbus.TopicGameServerStatusChanged,
	eventbus.TopicGameServerCPUThreshold,
	eventbus.TopicGameServerMemoryThreshold,
	eventbus.TopicGameServerDiskThreshold,
	eventbus.TopicGameServerPlayerThreshold,
	eventbus.TopicNodeCPUThreshold,
	eventbus.TopicNodeMemoryThreshold,
	eventbus.TopicNodeDiskThreshold,
}

var nodeAlertTopics = map[string]bool{
	eventbus.TopicNodeCPUThreshold:    true,
	eventbus.TopicNodeMemoryThreshold: true,
	eventbus.TopicNodeDiskThreshold:   true,
}

type crashEventData struct {
	ExitCode int `json:"exit_code,omitempty"`
}

type statusEventData struct {
	ServerName string `json:"server_name,omitempty"`
	OldStatus  string `json:"old_status,omitempty"`
	NewStatus  string `json:"new_status,omitempty"`
}

// IsFederatedEvent returns true if the event was received from a remote peer
// and should not be re-forwarded over federation.
func IsFederatedEvent(msg any) bool {
	switch ev := msg.(type) {
	case eventbus.ServerCrashedEvent:
		return ev.Federated
	case eventbus.StatusChangedEvent:
		return ev.Federated
	case eventbus.ThresholdEvent:
		return ev.Federated
	case eventbus.NodeThresholdEvent:
		return ev.Federated
	default:
		return false
	}
}

// SerializeFederationAlertEvent converts a typed event bus message into a
// federation alert event. It returns false for unknown topics, unrecognized
// types, or events that already originated from federation.
func SerializeFederationAlertEvent(topic string, msg any) (*xylona.FederationAlertEvent, bool) {
	protoType, ok := AlertTopicToProtoType[topic]
	if !ok {
		return nil, false
	}

	if IsFederatedEvent(msg) {
		return nil, false
	}

	evt := &xylona.FederationAlertEvent{
		EventType: protoType,
		Timestamp: timestamppb.Now(),
	}

	if nodeAlertTopics[topic] {
		ok = populateNodeAlertEvent(evt, msg)
	} else {
		ok = populateServerAlertEvent(evt, topic, msg)
	}
	if !ok {
		return nil, false
	}

	return evt, true
}

func populateServerAlertEvent(evt *xylona.FederationAlertEvent, topic string, msg any) bool {
	switch ev := msg.(type) {
	case eventbus.ServerCrashedEvent:
		evt.ServerId = &ev.ServerID
		evt.ServerNodeId = &ev.ServerNodeID
		evt.Timestamp = timestamppb.New(ev.Timestamp)
		dataJSON, errMarshal := json.Marshal(crashEventData{ExitCode: ev.ExitCode})
		if errMarshal != nil {
			log.Error().Err(errMarshal).Str("topic", topic).Msg("Failed to marshal crash event data for federation")
			return false
		}
		evt.EventData = string(dataJSON)
		return true

	case eventbus.StatusChangedEvent:
		evt.ServerId = &ev.ServerID
		evt.ServerNodeId = &ev.ServerNodeID
		dataJSON, errMarshal := json.Marshal(statusEventData{
			ServerName: ev.ServerName,
			OldStatus:  ev.OldStatus,
			NewStatus:  ev.NewStatus,
		})
		if errMarshal != nil {
			log.Error().Err(errMarshal).Str("topic", topic).Msg("Failed to marshal status event data for federation")
			return false
		}
		evt.EventData = string(dataJSON)
		return true

	case eventbus.ThresholdEvent:
		evt.ServerId = &ev.ServerID
		evt.ServerNodeId = &ev.ServerNodeID
		dataJSON, errMarshal := json.Marshal(ThresholdEventData{
			CurrentValue: ev.CurrentValue,
			Threshold:    ev.Threshold,
			Direction:    string(ev.Direction),
		})
		if errMarshal != nil {
			log.Error().Err(errMarshal).Str("topic", topic).Msg("Failed to marshal threshold event data for federation")
			return false
		}
		evt.EventData = string(dataJSON)
		return true

	default:
		log.Warn().Str("type", fmt.Sprintf("%T", msg)).Str("topic", topic).
			Msg("Unrecognized server event type for federation alert serialization")
		return false
	}
}

func populateNodeAlertEvent(evt *xylona.FederationAlertEvent, msg any) bool {
	ev, ok := msg.(eventbus.NodeThresholdEvent)
	if !ok {
		log.Warn().Str("type", fmt.Sprintf("%T", msg)).
			Msg("Unrecognized node event type for federation alert serialization")
		return false
	}

	evt.NodeId = &ev.NodeID
	dataJSON, errMarshal := json.Marshal(ThresholdEventData{
		CurrentValue: ev.CurrentValue,
		Threshold:    ev.Threshold,
		Direction:    string(ev.Direction),
	})
	if errMarshal != nil {
		log.Error().Err(errMarshal).Msg("Failed to marshal node threshold event data for federation")
		return false
	}
	evt.EventData = string(dataJSON)
	return true
}

// RepublishFederationAlertEvent republishes a federation alert event on the
// local event bus so alert evaluation can process it without creating loops.
func RepublishFederationAlertEvent(bus *eventbus.EventBus, evt *xylona.FederationAlertEvent) {
	topic, ok := AlertProtoTypeToTopic[evt.GetEventType()]
	if !ok {
		log.Warn().Str("event_type", evt.GetEventType().String()).
			Msg("Received federation alert event with unknown event type, skipping republish")
		return
	}

	if nodeAlertTopics[topic] {
		republishNodeAlertEvent(bus, topic, evt)
		return
	}

	republishServerAlertEvent(bus, topic, evt)
}

func republishServerAlertEvent(bus *eventbus.EventBus, topic string, evt *xylona.FederationAlertEvent) {
	serverID := evt.GetServerId()
	serverNodeID := evt.GetServerNodeId()

	switch topic {
	case eventbus.TopicGameServerCrashed:
		var data crashEventData
		errUnmarshal := json.Unmarshal([]byte(evt.GetEventData()), &data)
		if errUnmarshal != nil {
			log.Error().Err(errUnmarshal).Str("topic", topic).Msg("Failed to unmarshal federated crash event data")
			return
		}
		ts := time.Now()
		if evt.GetTimestamp() != nil {
			ts = evt.GetTimestamp().AsTime()
		}
		bus.Publish(topic, eventbus.ServerCrashedEvent{
			ServerID:     serverID,
			ServerNodeID: serverNodeID,
			ExitCode:     data.ExitCode,
			Timestamp:    ts,
			Federated:    true,
		})

	case eventbus.TopicGameServerStatusChanged:
		var data statusEventData
		errUnmarshal := json.Unmarshal([]byte(evt.GetEventData()), &data)
		if errUnmarshal != nil {
			log.Error().Err(errUnmarshal).Str("topic", topic).Msg("Failed to unmarshal federated status event data")
			return
		}
		bus.Publish(topic, eventbus.StatusChangedEvent{
			ServerID:     serverID,
			ServerName:   data.ServerName,
			ServerNodeID: serverNodeID,
			OldStatus:    data.OldStatus,
			NewStatus:    data.NewStatus,
			Federated:    true,
		})

	case eventbus.TopicGameServerCPUThreshold,
		eventbus.TopicGameServerMemoryThreshold,
		eventbus.TopicGameServerDiskThreshold,
		eventbus.TopicGameServerPlayerThreshold:
		var data ThresholdEventData
		errUnmarshal := json.Unmarshal([]byte(evt.GetEventData()), &data)
		if errUnmarshal != nil {
			log.Error().Err(errUnmarshal).Str("topic", topic).Msg("Failed to unmarshal federated threshold event data")
			return
		}
		bus.Publish(topic, eventbus.ThresholdEvent{
			ServerID:     serverID,
			ServerNodeID: serverNodeID,
			CurrentValue: data.CurrentValue,
			Threshold:    data.Threshold,
			Direction:    eventbus.ThresholdDirection(data.Direction),
			Federated:    true,
		})

	default:
		log.Warn().Str("topic", topic).Msg("No republish handler for federated server alert topic")
	}
}

func republishNodeAlertEvent(bus *eventbus.EventBus, topic string, evt *xylona.FederationAlertEvent) {
	var data ThresholdEventData
	errUnmarshal := json.Unmarshal([]byte(evt.GetEventData()), &data)
	if errUnmarshal != nil {
		log.Error().Err(errUnmarshal).Str("topic", topic).Msg("Failed to unmarshal federated node threshold event data")
		return
	}

	bus.Publish(topic, eventbus.NodeThresholdEvent{
		NodeID:       evt.GetNodeId(),
		CurrentValue: data.CurrentValue,
		Threshold:    data.Threshold,
		Direction:    eventbus.ThresholdDirection(data.Direction),
		Federated:    true,
	})
}
