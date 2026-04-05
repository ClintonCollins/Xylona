package alerts

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// --- Test helpers ---

// makeRule builds an AlertRule with the given parameters. Empty strings for
// serverID, serverNodeID, nodeID, and condition produce NULL (unset) fields.
func makeRule(id, userID, serverID, serverNodeID, nodeID, eventType, condition, channelID string, enabled bool) *models.AlertRule {
	enabledInt := int64(0)
	if enabled {
		enabledInt = 1
	}

	rule := &models.AlertRule{
		ID:                    id,
		UserID:                userID,
		EventType:             eventType,
		NotificationChannelID: channelID,
		Enabled:               enabledInt,
	}

	if serverID != "" {
		rule.ServerID = null.From(serverID)
	}
	if serverNodeID != "" {
		rule.ServerNodeID = null.From(serverNodeID)
	}
	if nodeID != "" {
		rule.NodeID = null.From(nodeID)
	}
	if condition != "" {
		rule.Condition = null.From(condition)
	}

	return rule
}

// --- matchesServer tests ---

func TestMatchesServer(t *testing.T) {
	tests := []struct {
		name         string
		rule         *models.AlertRule
		serverID     string
		serverNodeID string
		want         bool
	}{
		{
			name:         "exact server match",
			rule:         makeRule("r1", "u1", "srv-1", "node-a", "", "ALERT_EVENT_TYPE_CRASH", "", "ch1", true),
			serverID:     "srv-1",
			serverNodeID: "node-a",
			want:         true,
		},
		{
			name:         "all-servers match (NULL server_id)",
			rule:         makeRule("r2", "u1", "", "", "", "ALERT_EVENT_TYPE_CRASH", "", "ch1", true),
			serverID:     "srv-1",
			serverNodeID: "node-a",
			want:         true,
		},
		{
			name:         "wrong node",
			rule:         makeRule("r3", "u1", "srv-1", "node-b", "", "ALERT_EVENT_TYPE_CRASH", "", "ch1", true),
			serverID:     "srv-1",
			serverNodeID: "node-a",
			want:         false,
		},
		{
			name:         "wrong server",
			rule:         makeRule("r4", "u1", "srv-1", "node-a", "", "ALERT_EVENT_TYPE_CRASH", "", "ch1", true),
			serverID:     "srv-2",
			serverNodeID: "node-a",
			want:         false,
		},
		{
			name:         "rule has server_id but NULL server_node_id matches any node",
			rule:         makeRule("r5", "u1", "srv-1", "", "", "ALERT_EVENT_TYPE_CRASH", "", "ch1", true),
			serverID:     "srv-1",
			serverNodeID: "node-a",
			want:         true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesServer(tc.rule, tc.serverID, tc.serverNodeID)
			if got != tc.want {
				t.Errorf("matchesServer() = %v, want %v", got, tc.want)
			}
		})
	}
}

// --- matchesNode tests ---

func TestMatchesNode(t *testing.T) {
	tests := []struct {
		name   string
		rule   *models.AlertRule
		nodeID string
		want   bool
	}{
		{
			name:   "exact node match",
			rule:   makeRule("r1", "u1", "", "", "node-a", "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD", "", "ch1", true),
			nodeID: "node-a",
			want:   true,
		},
		{
			name:   "all-nodes match (NULL node_id)",
			rule:   makeRule("r2", "u1", "", "", "", "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD", "", "ch1", true),
			nodeID: "node-a",
			want:   true,
		},
		{
			name:   "wrong node",
			rule:   makeRule("r3", "u1", "", "", "node-b", "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD", "", "ch1", true),
			nodeID: "node-a",
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesNode(tc.rule, tc.nodeID)
			if got != tc.want {
				t.Errorf("matchesNode() = %v, want %v", got, tc.want)
			}
		})
	}
}

// --- evaluateCondition tests ---

func TestEvaluateCondition(t *testing.T) {
	tests := []struct {
		name      string
		condition null.Val[string]
		eventData eventData
		want      bool
		wantErr   bool
	}{
		{
			name:      "NULL condition always matches (crash event)",
			condition: null.Val[string]{},
			eventData: eventData{},
			want:      true,
		},
		{
			name:      "threshold >= matches value above",
			condition: null.From(`{"operator":">=","value":90}`),
			eventData: eventData{CurrentValue: 95},
			want:      true,
		},
		{
			name:      "threshold >= does not match value below",
			condition: null.From(`{"operator":">=","value":90}`),
			eventData: eventData{CurrentValue: 85},
			want:      false,
		},
		{
			name:      "threshold > matches value strictly above",
			condition: null.From(`{"operator":">","value":90}`),
			eventData: eventData{CurrentValue: 91},
			want:      true,
		},
		{
			name:      "threshold > does not match equal value",
			condition: null.From(`{"operator":">","value":90}`),
			eventData: eventData{CurrentValue: 90},
			want:      false,
		},
		{
			name:      "threshold <= matches value below",
			condition: null.From(`{"operator":"<=","value":10}`),
			eventData: eventData{CurrentValue: 5},
			want:      true,
		},
		{
			name:      "threshold <= does not match value above",
			condition: null.From(`{"operator":"<=","value":10}`),
			eventData: eventData{CurrentValue: 15},
			want:      false,
		},
		{
			name:      "threshold < matches value strictly below",
			condition: null.From(`{"operator":"<","value":10}`),
			eventData: eventData{CurrentValue: 9},
			want:      true,
		},
		{
			name:      "threshold < does not match equal value",
			condition: null.From(`{"operator":"<","value":10}`),
			eventData: eventData{CurrentValue: 10},
			want:      false,
		},
		{
			name:      "threshold == matches equal value",
			condition: null.From(`{"operator":"==","value":50}`),
			eventData: eventData{CurrentValue: 50},
			want:      true,
		},
		{
			name:      "threshold == does not match different value",
			condition: null.From(`{"operator":"==","value":50}`),
			eventData: eventData{CurrentValue: 51},
			want:      false,
		},
		{
			name:      "threshold == matches nearly equal floating point value",
			condition: null.From(`{"operator":"==","value":50}`),
			eventData: eventData{CurrentValue: 50.0000004},
			want:      true,
		},
		{
			name:      "threshold = alias matches equal value",
			condition: null.From(`{"operator":"=","value":50}`),
			eventData: eventData{CurrentValue: 50},
			want:      true,
		},
		{
			name:      "status change matches listed status",
			condition: null.From(`{"statuses":["offline"]}`),
			eventData: eventData{NewStatus: "offline"},
			want:      true,
		},
		{
			name:      "status change does not match unlisted status",
			condition: null.From(`{"statuses":["offline"]}`),
			eventData: eventData{NewStatus: "online"},
			want:      false,
		},
		{
			name:      "status change matches one of multiple statuses",
			condition: null.From(`{"statuses":["offline","crashed"]}`),
			eventData: eventData{NewStatus: "crashed"},
			want:      true,
		},
		{
			name:      "invalid JSON condition returns error",
			condition: null.From(`{bad json`),
			eventData: eventData{},
			wantErr:   true,
		},
		{
			name:      "unknown operator returns error",
			condition: null.From(`{"operator":"!=","value":50}`),
			eventData: eventData{CurrentValue: 50},
			wantErr:   true,
		},
		{
			name:      "empty condition string treated as no condition",
			condition: null.From(""),
			eventData: eventData{},
			want:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, errEval := evaluateCondition(tc.condition, tc.eventData)
			if tc.wantErr {
				if errEval == nil {
					t.Errorf("evaluateCondition() error = nil, want error")
				}
				return
			}
			if errEval != nil {
				t.Fatalf("evaluateCondition() unexpected error = %v", errEval)
			}
			if got != tc.want {
				t.Errorf("evaluateCondition() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestThresholdEventDataJSONPreservesZeroValues(t *testing.T) {
	data := eventData{
		CurrentValue: 0,
		Threshold:    0,
		Direction:    string(eventbus.ThresholdEntered),
	}

	payload, errMarshal := json.Marshal(data)
	if errMarshal != nil {
		t.Fatalf("json.Marshal(eventData) error = %v", errMarshal)
	}

	for _, fragment := range []string{`"current_value":0`, `"threshold":0`} {
		if !strings.Contains(string(payload), fragment) {
			t.Fatalf("payload = %q, want fragment %q", string(payload), fragment)
		}
	}
}

func TestMarshalEvaluatorEventData_PreservesCrashExitCodeZero(t *testing.T) {
	payload, errMarshal := marshalEvaluatorEventData("ALERT_EVENT_TYPE_CRASH", eventData{ExitCode: 0})
	if errMarshal != nil {
		t.Fatalf("marshalEvaluatorEventData(crash) error = %v", errMarshal)
	}
	if !strings.Contains(string(payload), `"exit_code":0`) {
		t.Fatalf("payload = %q, want %q", string(payload), `"exit_code":0`)
	}
}

// --- isEnabled tests ---

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name string
		rule *models.AlertRule
		want bool
	}{
		{
			name: "enabled rule",
			rule: makeRule("r1", "u1", "", "", "", "ALERT_EVENT_TYPE_CRASH", "", "ch1", true),
			want: true,
		},
		{
			name: "disabled rule",
			rule: makeRule("r2", "u1", "", "", "", "ALERT_EVENT_TYPE_CRASH", "", "ch1", false),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isEnabled(tc.rule)
			if got != tc.want {
				t.Errorf("isEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

// --- processEvent integration test with mock store ---

// mockRuleStore is a test double that returns a pre-configured slice of rules.
type mockRuleStore struct {
	rules map[string][]*models.AlertRule
	err   error
}

func (m *mockRuleStore) GetEnabledAlertRulesByEventType(eventType string) ([]*models.AlertRule, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.rules[eventType], nil
}

func TestProcessServerEvent(t *testing.T) {
	tests := []struct {
		name      string
		rules     []*models.AlertRule
		eventType string
		serverID  string
		nodeID    string
		data      eventData
		wantJobs  int
	}{
		{
			name: "crash event matches rule with exact server",
			rules: []*models.AlertRule{
				makeRule("r1", "u1", "srv-1", "node-a", "", "ALERT_EVENT_TYPE_CRASH", "", "ch1", true),
			},
			eventType: "ALERT_EVENT_TYPE_CRASH",
			serverID:  "srv-1",
			nodeID:    "node-a",
			data:      eventData{},
			wantJobs:  1,
		},
		{
			name: "crash event matches wildcard rule",
			rules: []*models.AlertRule{
				makeRule("r1", "u1", "", "", "", "ALERT_EVENT_TYPE_CRASH", "", "ch1", true),
			},
			eventType: "ALERT_EVENT_TYPE_CRASH",
			serverID:  "srv-1",
			nodeID:    "node-a",
			data:      eventData{},
			wantJobs:  1,
		},
		{
			name: "threshold event with matching condition produces job",
			rules: []*models.AlertRule{
				makeRule("r1", "u1", "srv-1", "node-a", "", "ALERT_EVENT_TYPE_CPU_THRESHOLD", `{"operator":">=","value":90}`, "ch1", true),
			},
			eventType: "ALERT_EVENT_TYPE_CPU_THRESHOLD",
			serverID:  "srv-1",
			nodeID:    "node-a",
			data:      eventData{CurrentValue: 95},
			wantJobs:  1,
		},
		{
			name: "threshold event with non-matching condition produces no job",
			rules: []*models.AlertRule{
				makeRule("r1", "u1", "srv-1", "node-a", "", "ALERT_EVENT_TYPE_CPU_THRESHOLD", `{"operator":">=","value":90}`, "ch1", true),
			},
			eventType: "ALERT_EVENT_TYPE_CPU_THRESHOLD",
			serverID:  "srv-1",
			nodeID:    "node-a",
			data:      eventData{CurrentValue: 85},
			wantJobs:  0,
		},
		{
			name: "disabled rule never produces a job",
			rules: []*models.AlertRule{
				makeRule("r1", "u1", "srv-1", "node-a", "", "ALERT_EVENT_TYPE_CRASH", "", "ch1", false),
			},
			eventType: "ALERT_EVENT_TYPE_CRASH",
			serverID:  "srv-1",
			nodeID:    "node-a",
			data:      eventData{},
			wantJobs:  0,
		},
		{
			name: "wrong server does not match",
			rules: []*models.AlertRule{
				makeRule("r1", "u1", "srv-1", "node-a", "", "ALERT_EVENT_TYPE_CRASH", "", "ch1", true),
			},
			eventType: "ALERT_EVENT_TYPE_CRASH",
			serverID:  "srv-2",
			nodeID:    "node-a",
			data:      eventData{},
			wantJobs:  0,
		},
		{
			name: "multiple rules some match some do not",
			rules: []*models.AlertRule{
				makeRule("r1", "u1", "srv-1", "node-a", "", "ALERT_EVENT_TYPE_CRASH", "", "ch1", true),
				makeRule("r2", "u2", "srv-2", "node-a", "", "ALERT_EVENT_TYPE_CRASH", "", "ch2", true),
				makeRule("r3", "u3", "", "", "", "ALERT_EVENT_TYPE_CRASH", "", "ch3", true),
			},
			eventType: "ALERT_EVENT_TYPE_CRASH",
			serverID:  "srv-1",
			nodeID:    "node-a",
			data:      eventData{},
			wantJobs:  2, // r1 (exact) and r3 (wildcard), not r2 (wrong server)
		},
		{
			name: "status change with matching condition",
			rules: []*models.AlertRule{
				makeRule("r1", "u1", "srv-1", "node-a", "", "ALERT_EVENT_TYPE_STATUS_CHANGE", `{"statuses":["offline","crashed"]}`, "ch1", true),
			},
			eventType: "ALERT_EVENT_TYPE_STATUS_CHANGE",
			serverID:  "srv-1",
			nodeID:    "node-a",
			data:      eventData{NewStatus: "offline"},
			wantJobs:  1,
		},
		{
			name: "status change with non-matching status",
			rules: []*models.AlertRule{
				makeRule("r1", "u1", "srv-1", "node-a", "", "ALERT_EVENT_TYPE_STATUS_CHANGE", `{"statuses":["offline"]}`, "ch1", true),
			},
			eventType: "ALERT_EVENT_TYPE_STATUS_CHANGE",
			serverID:  "srv-1",
			nodeID:    "node-a",
			data:      eventData{NewStatus: "online"},
			wantJobs:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockRuleStore{
				rules: map[string][]*models.AlertRule{
					tc.eventType: tc.rules,
				},
			}

			dataJSON, errMarshal := json.Marshal(tc.data)
			if errMarshal != nil {
				t.Fatalf("failed to marshal event data: %v", errMarshal)
			}

			jobs, errProcess := processServerEvent(store, tc.eventType, tc.serverID, tc.nodeID, tc.data, string(dataJSON))
			if errProcess != nil {
				t.Fatalf("processServerEvent() unexpected error = %v", errProcess)
			}
			if len(jobs) != tc.wantJobs {
				t.Errorf("processServerEvent() produced %d jobs, want %d", len(jobs), tc.wantJobs)
			}

			// Verify job fields for matching rules.
			for _, job := range jobs {
				if job.EventType != tc.eventType {
					t.Errorf("job.EventType = %q, want %q", job.EventType, tc.eventType)
				}
				if job.ServerID != tc.serverID {
					t.Errorf("job.ServerID = %q, want %q", job.ServerID, tc.serverID)
				}
				if job.ServerNodeID != tc.nodeID {
					t.Errorf("job.ServerNodeID = %q, want %q", job.ServerNodeID, tc.nodeID)
				}
			}
		})
	}
}

func TestProcessNodeEvent(t *testing.T) {
	tests := []struct {
		name      string
		rules     []*models.AlertRule
		eventType string
		nodeID    string
		data      eventData
		wantJobs  int
	}{
		{
			name: "node threshold matches exact node",
			rules: []*models.AlertRule{
				makeRule("r1", "u1", "", "", "node-a", "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD", `{"operator":">=","value":90}`, "ch1", true),
			},
			eventType: "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD",
			nodeID:    "node-a",
			data:      eventData{CurrentValue: 95},
			wantJobs:  1,
		},
		{
			name: "node threshold matches wildcard node",
			rules: []*models.AlertRule{
				makeRule("r1", "u1", "", "", "", "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD", `{"operator":">=","value":90}`, "ch1", true),
			},
			eventType: "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD",
			nodeID:    "node-a",
			data:      eventData{CurrentValue: 95},
			wantJobs:  1,
		},
		{
			name: "node threshold wrong node",
			rules: []*models.AlertRule{
				makeRule("r1", "u1", "", "", "node-b", "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD", `{"operator":">=","value":90}`, "ch1", true),
			},
			eventType: "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD",
			nodeID:    "node-a",
			data:      eventData{CurrentValue: 95},
			wantJobs:  0,
		},
		{
			name: "node threshold condition not met",
			rules: []*models.AlertRule{
				makeRule("r1", "u1", "", "", "node-a", "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD", `{"operator":">=","value":90}`, "ch1", true),
			},
			eventType: "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD",
			nodeID:    "node-a",
			data:      eventData{CurrentValue: 85},
			wantJobs:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockRuleStore{
				rules: map[string][]*models.AlertRule{
					tc.eventType: tc.rules,
				},
			}

			dataJSON, errMarshal := json.Marshal(tc.data)
			if errMarshal != nil {
				t.Fatalf("failed to marshal event data: %v", errMarshal)
			}

			jobs, errProcess := processNodeEvent(store, tc.eventType, tc.nodeID, tc.data, string(dataJSON))
			if errProcess != nil {
				t.Fatalf("processNodeEvent() unexpected error = %v", errProcess)
			}
			if len(jobs) != tc.wantJobs {
				t.Errorf("processNodeEvent() produced %d jobs, want %d", len(jobs), tc.wantJobs)
			}

			for _, job := range jobs {
				if job.EventType != tc.eventType {
					t.Errorf("job.EventType = %q, want %q", job.EventType, tc.eventType)
				}
				if job.NodeID != tc.nodeID {
					t.Errorf("job.NodeID = %q, want %q", job.NodeID, tc.nodeID)
				}
			}
		})
	}
}

func TestProcessServerEvent_StoreError(t *testing.T) {
	store := &mockRuleStore{
		err: errors.New("db connection failed"),
	}
	_, errProcess := processServerEvent(store, "ALERT_EVENT_TYPE_CRASH", "srv-1", "node-a", eventData{}, "{}")
	if errProcess == nil {
		t.Fatal("processServerEvent() error = nil, want error from store")
	}
}

func TestProcessNodeEvent_StoreError(t *testing.T) {
	store := &mockRuleStore{
		err: errors.New("db connection failed"),
	}
	_, errProcess := processNodeEvent(store, "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD", "node-a", eventData{}, "{}")
	if errProcess == nil {
		t.Fatal("processNodeEvent() error = nil, want error from store")
	}
}

// --- extractServerEventData tests ---

func TestExtractServerEventData_CrashedEvent(t *testing.T) {
	msg := eventbus.ServerCrashedEvent{
		ServerID:     "srv-1",
		ServerNodeID: "node-a",
		ExitCode:     137,
		Timestamp:    time.Now(),
	}
	serverID, serverNodeID, data, ok := extractServerEventData(msg)
	if !ok {
		t.Fatal("extractServerEventData returned ok=false for ServerCrashedEvent")
	}
	if serverID != "srv-1" {
		t.Errorf("serverID = %q, want %q", serverID, "srv-1")
	}
	if serverNodeID != "node-a" {
		t.Errorf("serverNodeID = %q, want %q", serverNodeID, "node-a")
	}
	if data.ExitCode != 137 {
		t.Errorf("data.ExitCode = %d, want %d", data.ExitCode, 137)
	}
}

func TestExtractServerEventData_StatusChangedEvent(t *testing.T) {
	msg := eventbus.StatusChangedEvent{
		ServerID:     "srv-2",
		ServerNodeID: "node-b",
		OldStatus:    "online",
		NewStatus:    "offline",
	}
	serverID, serverNodeID, data, ok := extractServerEventData(msg)
	if !ok {
		t.Fatal("extractServerEventData returned ok=false for StatusChangedEvent")
	}
	if serverID != "srv-2" {
		t.Errorf("serverID = %q, want %q", serverID, "srv-2")
	}
	if serverNodeID != "node-b" {
		t.Errorf("serverNodeID = %q, want %q", serverNodeID, "node-b")
	}
	if data.NewStatus != "offline" {
		t.Errorf("data.NewStatus = %q, want %q", data.NewStatus, "offline")
	}
	if data.OldStatus != "online" {
		t.Errorf("data.OldStatus = %q, want %q", data.OldStatus, "online")
	}
}

func TestExtractServerEventData_ThresholdEvent(t *testing.T) {
	msg := eventbus.ThresholdEvent{
		ServerID:     "srv-3",
		ServerNodeID: "node-c",
		CurrentValue: 95.5,
		Threshold:    90.0,
		Direction:    eventbus.ThresholdEntered,
	}
	serverID, serverNodeID, data, ok := extractServerEventData(msg)
	if !ok {
		t.Fatal("extractServerEventData returned ok=false for ThresholdEvent")
	}
	if serverID != "srv-3" {
		t.Errorf("serverID = %q, want %q", serverID, "srv-3")
	}
	if serverNodeID != "node-c" {
		t.Errorf("serverNodeID = %q, want %q", serverNodeID, "node-c")
	}
	if data.CurrentValue != 95.5 {
		t.Errorf("data.CurrentValue = %f, want %f", data.CurrentValue, 95.5)
	}
	if data.Direction != "entered" {
		t.Errorf("data.Direction = %q, want %q", data.Direction, "entered")
	}
}

func TestExtractServerEventData_UnknownType(t *testing.T) {
	_, _, _, ok := extractServerEventData("not a real event")
	if ok {
		t.Error("extractServerEventData returned ok=true for unknown type")
	}
}

func TestExtractNodeEventData_NodeThresholdEvent(t *testing.T) {
	msg := eventbus.NodeThresholdEvent{
		NodeID:       "node-1",
		CurrentValue: 88.0,
		Threshold:    80.0,
		Direction:    eventbus.ThresholdResolved,
	}
	nodeID, data, ok := extractNodeEventData(msg)
	if !ok {
		t.Fatal("extractNodeEventData returned ok=false for NodeThresholdEvent")
	}
	if nodeID != "node-1" {
		t.Errorf("nodeID = %q, want %q", nodeID, "node-1")
	}
	if data.CurrentValue != 88.0 {
		t.Errorf("data.CurrentValue = %f, want %f", data.CurrentValue, 88.0)
	}
	if data.Direction != "resolved" {
		t.Errorf("data.Direction = %q, want %q", data.Direction, "resolved")
	}
}

func TestExtractNodeEventData_UnknownType(t *testing.T) {
	_, _, ok := extractNodeEventData("not a node event")
	if ok {
		t.Error("extractNodeEventData returned ok=true for unknown type")
	}
}

// --- Integration: Evaluator end-to-end with event bus ---

func TestEvaluator_EndToEnd(t *testing.T) {
	bus := eventbus.Get()

	store := &mockRuleStore{
		rules: map[string][]*models.AlertRule{
			"ALERT_EVENT_TYPE_CRASH": {
				makeRule("r1", "u1", "srv-1", "node-a", "", "ALERT_EVENT_TYPE_CRASH", "", "ch1", true),
			},
		},
	}

	eval, jobChan := NewEvaluator(store, bus, nil)

	ctx := t.Context()

	eval.Start(ctx)

	// Publish a crash event to the bus.
	bus.Publish(eventbus.TopicGameServerCrashed, eventbus.ServerCrashedEvent{
		ServerID:     "srv-1",
		ServerNodeID: "node-a",
		ExitCode:     1,
		Timestamp:    time.Now(),
	})

	// Read the delivery job from the channel with a timeout.
	select {
	case job := <-jobChan:
		if job.RuleID != "r1" {
			t.Errorf("job.RuleID = %q, want %q", job.RuleID, "r1")
		}
		if job.ChannelID != "ch1" {
			t.Errorf("job.ChannelID = %q, want %q", job.ChannelID, "ch1")
		}
		if job.ServerID != "srv-1" {
			t.Errorf("job.ServerID = %q, want %q", job.ServerID, "srv-1")
		}
		if job.EventType != "ALERT_EVENT_TYPE_CRASH" {
			t.Errorf("job.EventType = %q, want %q", job.EventType, "ALERT_EVENT_TYPE_CRASH")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for delivery job")
	}
}

func TestEvaluator_UnknownEventTypeDropped(t *testing.T) {
	bus := eventbus.Get()

	store := &mockRuleStore{
		rules: map[string][]*models.AlertRule{
			"ALERT_EVENT_TYPE_CRASH": {
				// Wildcard rule that would match anything if the unknown event
				// was incorrectly processed.
				makeRule("r1", "u1", "", "", "", "ALERT_EVENT_TYPE_CRASH", "", "ch1", true),
			},
		},
	}

	eval, jobChan := NewEvaluator(store, bus, nil)

	ctx := t.Context()

	eval.Start(ctx)

	// Publish a crash event with the wrong type (string instead of struct).
	// This should be dropped by extractServerEventData returning ok=false.
	bus.Publish(eventbus.TopicGameServerCrashed, "not a real event")

	// Verify no job appears.
	select {
	case job := <-jobChan:
		t.Fatalf("expected no delivery job, got %+v", job)
	case <-time.After(200 * time.Millisecond):
		// Good — no job was produced.
	}
}

// --- Dropped job recording tests ---

// dropRecordingStore records InsertAlertHistory calls for verifying dropped job behavior.
type dropRecordingStore struct {
	mu      sync.Mutex
	records []droppedRecord
}

type droppedRecord struct {
	ruleID         string
	eventType      string
	deliveryStatus string
}

func (d *dropRecordingStore) InsertAlertHistory(ruleID, _ string, _, _, _, eventType, _, _, deliveryStatus string) (*models.AlertHistory, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.records = append(d.records, droppedRecord{
		ruleID:         ruleID,
		eventType:      eventType,
		deliveryStatus: deliveryStatus,
	})
	return &models.AlertHistory{ID: "drop-hist-1"}, nil
}

func TestRecordDroppedJob(t *testing.T) {
	recorder := &dropRecordingStore{}
	eval := &Evaluator{
		dropRecorder: recorder,
		jobChan:      make(chan DeliveryJob), // unbuffered, will not be used
	}

	job := DeliveryJob{
		RuleID:    "rule-drop-1",
		ChannelID: "ch-1",
		UserID:    "user-1",
		EventType: "ALERT_EVENT_TYPE_CRASH",
		EventData: "{}",
		ServerID:  "srv-1",
	}

	eval.recordDroppedJob(job)

	recorder.mu.Lock()
	records := recorder.records
	recorder.mu.Unlock()

	if len(records) != 1 {
		t.Fatalf("expected 1 dropped record, got %d", len(records))
	}
	if records[0].ruleID != "rule-drop-1" {
		t.Errorf("ruleID = %q, want %q", records[0].ruleID, "rule-drop-1")
	}
	if records[0].deliveryStatus != "DELIVERY_STATUS_FAILED" {
		t.Errorf("deliveryStatus = %q, want %q", records[0].deliveryStatus, "DELIVERY_STATUS_FAILED")
	}
}

func TestRecordDroppedJob_NilRecorder(_ *testing.T) {
	eval := &Evaluator{
		dropRecorder: nil,
		jobChan:      make(chan DeliveryJob),
	}

	// Should not panic.
	eval.recordDroppedJob(DeliveryJob{RuleID: "r1"})
}

// --- topicToEventType tests ---

func TestTopicToEventType(t *testing.T) {
	tests := []struct {
		topic string
		want  string
		found bool
	}{
		{"game_server.crashed", "ALERT_EVENT_TYPE_CRASH", true},
		{"game_server.status_changed", "ALERT_EVENT_TYPE_STATUS_CHANGE", true},
		{"game_server.cpu_threshold", "ALERT_EVENT_TYPE_CPU_THRESHOLD", true},
		{"game_server.memory_threshold", "ALERT_EVENT_TYPE_MEMORY_THRESHOLD", true},
		{"game_server.disk_threshold", "ALERT_EVENT_TYPE_DISK_THRESHOLD", true},
		{"game_server.player_threshold", "ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD", true},
		{"node.cpu_threshold", "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD", true},
		{"node.memory_threshold", "ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD", true},
		{"node.disk_threshold", "ALERT_EVENT_TYPE_NODE_DISK_THRESHOLD", true},
		{"unknown.topic", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.topic, func(t *testing.T) {
			got, ok := topicToEventType(tc.topic)
			if ok != tc.found {
				t.Errorf("topicToEventType(%q) ok = %v, want %v", tc.topic, ok, tc.found)
			}
			if got != tc.want {
				t.Errorf("topicToEventType(%q) = %q, want %q", tc.topic, got, tc.want)
			}
		})
	}
}
