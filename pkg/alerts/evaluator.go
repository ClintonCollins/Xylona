package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/aarondl/opt/null"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// RuleStore abstracts the DB methods the evaluator needs for fetching rules.
type RuleStore interface {
	GetEnabledAlertRulesByEventType(eventType string) ([]*models.AlertRule, error)
}

// DeliveryJob represents a unit of work for the delivery workers. It contains
// all the information needed to deliver a notification for a matched alert rule.
type DeliveryJob struct {
	RuleID       string
	ChannelID    string
	UserID       string
	EventType    string
	EventData    string // JSON-encoded event data
	ServerID     string
	ServerNodeID string
	NodeID       string
}

// eventData is the normalized representation of event payload fields used for
// condition evaluation. Only the fields relevant to the event type will be set.
type eventData struct {
	CurrentValue float64 `json:"current_value"`
	NewStatus    string  `json:"new_status,omitempty"`
	OldStatus    string  `json:"old_status,omitempty"`
	ExitCode     int     `json:"exit_code,omitempty"`
	Threshold    float64 `json:"threshold"`
	Direction    string  `json:"direction,omitempty"`
}

// topicEventTypeMap maps event bus topics to the proto enum string names stored
// in the DB.
var topicEventTypeMap = map[string]string{
	eventbus.TopicGameServerCrashed:         "ALERT_EVENT_TYPE_CRASH",
	eventbus.TopicGameServerStatusChanged:   "ALERT_EVENT_TYPE_STATUS_CHANGE",
	eventbus.TopicGameServerCPUThreshold:    "ALERT_EVENT_TYPE_CPU_THRESHOLD",
	eventbus.TopicGameServerMemoryThreshold: "ALERT_EVENT_TYPE_MEMORY_THRESHOLD",
	eventbus.TopicGameServerDiskThreshold:   "ALERT_EVENT_TYPE_DISK_THRESHOLD",
	eventbus.TopicGameServerPlayerThreshold: "ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD",
	eventbus.TopicNodeCPUThreshold:          "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD",
	eventbus.TopicNodeMemoryThreshold:       "ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD",
	eventbus.TopicNodeDiskThreshold:         "ALERT_EVENT_TYPE_NODE_DISK_THRESHOLD",
}

// nodeTopics is the set of topics that represent node-level events.
var nodeTopics = map[string]bool{
	eventbus.TopicNodeCPUThreshold:    true,
	eventbus.TopicNodeMemoryThreshold: true,
	eventbus.TopicNodeDiskThreshold:   true,
}

// topicToEventType returns the DB event type string for the given event bus
// topic. Returns ("", false) for unknown topics.
func topicToEventType(topic string) (string, bool) {
	et, ok := topicEventTypeMap[topic]
	return et, ok
}

// DropRecorder can record a failed history entry when a delivery job is dropped
// because the job channel is full. This is an optional subset of HistoryStore.
type DropRecorder interface {
	InsertAlertHistory(ruleID, userID, serverID, serverNodeID, nodeID, eventType, eventData, channelType, deliveryStatus string) (*models.AlertHistory, error)
}

// Evaluator subscribes to event bus topics, matches rules from the DB, and
// produces DeliveryJob values for the delivery workers.
type Evaluator struct {
	store        RuleStore
	bus          *eventbus.EventBus
	jobChan      chan DeliveryJob
	dropRecorder DropRecorder
}

// NewEvaluator creates an Evaluator that reads rules from store and publishes
// delivery jobs to the returned channel. The channel is buffered. The
// dropRecorder is called when a job is dropped due to a full channel; pass nil
// to only log drops without recording them.
func NewEvaluator(store RuleStore, bus *eventbus.EventBus, dropRecorder DropRecorder) (*Evaluator, chan DeliveryJob) {
	jobChan := make(chan DeliveryJob, 256)
	return &Evaluator{
		store:        store,
		bus:          bus,
		jobChan:      jobChan,
		dropRecorder: dropRecorder,
	}, jobChan
}

// Start subscribes to all alert event topics and processes events until the
// context is cancelled. It spawns one goroutine per topic.
func (e *Evaluator) Start(ctx context.Context) {
	allTopics := []string{
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

	for _, topic := range allTopics {
		ch := e.bus.SubscribeReliable(topic)
		go e.listen(ctx, topic, ch)
	}

	log.Info().Int("topics", len(allTopics)).Msg("Alert evaluator started")
}

// listen reads events from the subscription channel for a single topic.
func (e *Evaluator) listen(ctx context.Context, topic string, ch chan any) {
	for {
		select {
		case <-ctx.Done():
			e.bus.Unsubscribe(topic, ch)
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			e.handleEvent(topic, msg)
		}
	}
}

// handleEvent dispatches a raw event bus message to the appropriate processing
// function based on whether it is a server or node event.
func (e *Evaluator) handleEvent(topic string, msg any) {
	eventType, ok := topicToEventType(topic)
	if !ok {
		log.Warn().Str("topic", topic).Msg("Alert evaluator received event for unknown topic")
		return
	}

	if nodeTopics[topic] {
		e.handleNodeEvent(topic, eventType, msg)
		return
	}

	e.handleServerEvent(topic, eventType, msg)
}

// handleServerEvent extracts fields from a server-level event and evaluates rules.
func (e *Evaluator) handleServerEvent(topic, eventType string, msg any) {
	serverID, serverNodeID, data, ok := extractServerEventData(msg)
	if !ok {
		return
	}

	dataJSON, errMarshal := marshalEvaluatorEventData(eventType, data)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Str("topic", topic).Msg("Failed to marshal event data")
		return
	}

	jobs, errProcess := processServerEvent(e.store, eventType, serverID, serverNodeID, data, string(dataJSON))
	if errProcess != nil {
		log.Error().Err(errProcess).Str("topic", topic).Msg("Failed to process server event")
		return
	}

	for _, job := range jobs {
		select {
		case e.jobChan <- job:
		default:
			log.Warn().Str("rule_id", job.RuleID).Str("event_type", job.EventType).Msg("Delivery job channel full, dropping job")
			e.recordDroppedJob(job)
		}
	}
}

// handleNodeEvent extracts fields from a node-level event and evaluates rules.
func (e *Evaluator) handleNodeEvent(topic, eventType string, msg any) {
	nodeID, data, ok := extractNodeEventData(msg)
	if !ok {
		return
	}

	dataJSON, errMarshal := marshalEvaluatorEventData(eventType, data)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Str("topic", topic).Msg("Failed to marshal event data")
		return
	}

	jobs, errProcess := processNodeEvent(e.store, eventType, nodeID, data, string(dataJSON))
	if errProcess != nil {
		log.Error().Err(errProcess).Str("topic", topic).Msg("Failed to process node event")
		return
	}

	for _, job := range jobs {
		select {
		case e.jobChan <- job:
		default:
			log.Warn().Str("rule_id", job.RuleID).Str("event_type", job.EventType).Msg("Delivery job channel full, dropping job")
			e.recordDroppedJob(job)
		}
	}
}

// recordDroppedJob inserts a failed alert history record for a job that was
// dropped because the delivery channel was full. This ensures the user can see
// in their alert history that a notification was lost.
func (e *Evaluator) recordDroppedJob(job DeliveryJob) {
	if e.dropRecorder == nil {
		return
	}
	_, errInsert := e.dropRecorder.InsertAlertHistory(
		job.RuleID,
		job.UserID,
		job.ServerID,
		job.ServerNodeID,
		job.NodeID,
		job.EventType,
		job.EventData,
		"", // channel type unknown — we never resolved it
		"DELIVERY_STATUS_FAILED",
	)
	if errInsert != nil {
		log.Error().Err(errInsert).Str("rule_id", job.RuleID).Msg("Failed to record dropped delivery job in alert history")
	}
}

// extractServerEventData converts a typed event bus message into the serverID,
// serverNodeID, and normalized eventData. Returns ok=false for unknown types.
func extractServerEventData(msg any) (serverID, serverNodeID string, data eventData, ok bool) {
	switch ev := msg.(type) {
	case eventbus.ServerCrashedEvent:
		return ev.ServerID, ev.ServerNodeID, eventData{
			ExitCode: ev.ExitCode,
		}, true
	case eventbus.StatusChangedEvent:
		return ev.ServerID, ev.ServerNodeID, eventData{
			OldStatus: ev.OldStatus,
			NewStatus: ev.NewStatus,
		}, true
	case eventbus.ThresholdEvent:
		return ev.ServerID, ev.ServerNodeID, eventData{
			CurrentValue: ev.CurrentValue,
			Threshold:    ev.Threshold,
			Direction:    string(ev.Direction),
		}, true
	default:
		log.Warn().Str("type", fmt.Sprintf("%T", msg)).Msg("Alert evaluator received unknown server event type")
		return "", "", eventData{}, false
	}
}

// extractNodeEventData converts a typed node event bus message into the nodeID
// and normalized eventData. Returns ok=false for unknown types.
func extractNodeEventData(msg any) (nodeID string, data eventData, ok bool) {
	ev, isNode := msg.(eventbus.NodeThresholdEvent)
	if !isNode {
		log.Warn().Str("type", fmt.Sprintf("%T", msg)).Msg("Alert evaluator received unknown node event type")
		return "", eventData{}, false
	}
	return ev.NodeID, eventData{
		CurrentValue: ev.CurrentValue,
		Threshold:    ev.Threshold,
		Direction:    string(ev.Direction),
	}, true
}

func marshalEvaluatorEventData(eventType string, data eventData) ([]byte, error) {
	if isThresholdAlertEventType(eventType) {
		return marshalAlertEventData("threshold", ThresholdEventData{
			CurrentValue: data.CurrentValue,
			Threshold:    data.Threshold,
			Direction:    data.Direction,
		})
	}

	switch eventType {
	case "ALERT_EVENT_TYPE_CRASH":
		return marshalAlertEventData("crash", struct {
			ExitCode int `json:"exit_code"`
		}{
			ExitCode: data.ExitCode,
		})
	case "ALERT_EVENT_TYPE_STATUS_CHANGE":
		return marshalAlertEventData("status change", struct {
			NewStatus string `json:"new_status,omitempty"`
			OldStatus string `json:"old_status,omitempty"`
		}{
			NewStatus: data.NewStatus,
			OldStatus: data.OldStatus,
		})
	default:
		return marshalAlertEventData("generic", data)
	}
}

func marshalAlertEventData(eventName string, payload any) ([]byte, error) {
	jsonBytes, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		return nil, fmt.Errorf("marshal %s alert event data: %w", eventName, errMarshal)
	}
	return jsonBytes, nil
}

func isThresholdAlertEventType(eventType string) bool {
	switch eventType {
	case "ALERT_EVENT_TYPE_CPU_THRESHOLD",
		"ALERT_EVENT_TYPE_MEMORY_THRESHOLD",
		"ALERT_EVENT_TYPE_DISK_THRESHOLD",
		"ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD",
		"ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD",
		"ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD",
		"ALERT_EVENT_TYPE_NODE_DISK_THRESHOLD":
		return true
	default:
		return false
	}
}

// processServerEvent is the core rule-matching logic for server events. It is a
// pure function (given an interface for the store) to enable unit testing.
func processServerEvent(store RuleStore, eventType, serverID, serverNodeID string, data eventData, dataJSON string) ([]DeliveryJob, error) {
	rules, errGet := store.GetEnabledAlertRulesByEventType(eventType)
	if errGet != nil {
		return nil, fmt.Errorf("failed to fetch rules for event type %s: %w", eventType, errGet)
	}

	var jobs []DeliveryJob
	for _, rule := range rules {
		if !isEnabled(rule) {
			continue
		}
		if !matchesServer(rule, serverID, serverNodeID) {
			continue
		}
		matches, errCond := evaluateCondition(rule.Condition, data)
		if errCond != nil {
			log.Warn().Err(errCond).Str("rule_id", rule.ID).Msg("Failed to evaluate condition, skipping rule")
			continue
		}
		if !matches {
			continue
		}
		jobs = append(jobs, DeliveryJob{
			RuleID:       rule.ID,
			ChannelID:    rule.NotificationChannelID,
			UserID:       rule.UserID,
			EventType:    eventType,
			EventData:    dataJSON,
			ServerID:     serverID,
			ServerNodeID: serverNodeID,
		})
	}

	return jobs, nil
}

// processNodeEvent is the core rule-matching logic for node events.
func processNodeEvent(store RuleStore, eventType, nodeID string, data eventData, dataJSON string) ([]DeliveryJob, error) {
	rules, errGet := store.GetEnabledAlertRulesByEventType(eventType)
	if errGet != nil {
		return nil, fmt.Errorf("failed to fetch rules for event type %s: %w", eventType, errGet)
	}

	var jobs []DeliveryJob
	for _, rule := range rules {
		if !isEnabled(rule) {
			continue
		}
		if !matchesNode(rule, nodeID) {
			continue
		}
		matches, errCond := evaluateCondition(rule.Condition, data)
		if errCond != nil {
			log.Warn().Err(errCond).Str("rule_id", rule.ID).Msg("Failed to evaluate condition, skipping rule")
			continue
		}
		if !matches {
			continue
		}
		jobs = append(jobs, DeliveryJob{
			RuleID:    rule.ID,
			ChannelID: rule.NotificationChannelID,
			UserID:    rule.UserID,
			EventType: eventType,
			EventData: dataJSON,
			NodeID:    nodeID,
		})
	}

	return jobs, nil
}

// isEnabled returns true if the rule's Enabled field is non-zero.
func isEnabled(rule *models.AlertRule) bool {
	return rule.Enabled != 0
}

// matchesServer returns true if the rule applies to the given server.
// A NULL ServerID in the rule matches all servers. If ServerID is set, it must
// match exactly. Similarly, a NULL ServerNodeID matches any node for that server.
func matchesServer(rule *models.AlertRule, serverID, serverNodeID string) bool {
	ruleServerID, serverIDSet := rule.ServerID.Get()
	if serverIDSet && ruleServerID != serverID {
		return false
	}

	ruleNodeID, nodeIDSet := rule.ServerNodeID.Get()
	if nodeIDSet && ruleNodeID != serverNodeID {
		return false
	}

	return true
}

// matchesNode returns true if the rule applies to the given node.
// A NULL NodeID in the rule matches all nodes.
func matchesNode(rule *models.AlertRule, nodeID string) bool {
	ruleNodeID, nodeIDSet := rule.NodeID.Get()
	if nodeIDSet && ruleNodeID != nodeID {
		return false
	}
	return true
}

// statusConditionJSON is the internal representation of a rule's condition
// field for status-list conditions.
type statusConditionJSON struct {
	Operator string   `json:"operator,omitempty"`
	Statuses []string `json:"statuses,omitempty"`
}

// evaluateCondition checks whether the event data satisfies the rule's
// condition. A NULL or empty condition always matches (used for crash events
// and other unconditional alerts).
func evaluateCondition(condition null.Val[string], data eventData) (bool, error) {
	condStr, isSet := condition.Get()
	if !isSet || condStr == "" {
		return true, nil
	}

	var cond statusConditionJSON
	errUnmarshal := json.Unmarshal([]byte(condStr), &cond)
	if errUnmarshal != nil {
		return false, fmt.Errorf("invalid condition JSON: %w", errUnmarshal)
	}

	// Status list condition.
	if len(cond.Statuses) > 0 {
		return slices.Contains(cond.Statuses, data.NewStatus), nil
	}

	// Threshold condition — delegate to the shared implementation.
	if cond.Operator != "" {
		op, threshold, errParse := ParseConditionJSON(condStr)
		if errParse != nil {
			return false, errParse
		}
		return EvaluateThresholdOp(op, threshold, data.CurrentValue)
	}

	// No recognized condition fields — treat as unconditional match.
	return true, nil
}
