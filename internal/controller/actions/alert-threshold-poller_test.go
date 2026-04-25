package actions

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// subscribeReliable subscribes to the given topic with a buffered channel so
// that messages published synchronously during runOnce() are not silently
// dropped by the eventbus non-blocking send.
func subscribeReliable(bus *eventbus.EventBus, topic string) chan any {
	return bus.SubscribeReliable(topic)
}

// --- Fake implementations ---

// fakeAlertRuleStore implements thresholdRuleStore.
type fakeAlertRuleStore struct {
	mu    sync.Mutex
	rules map[string][]*models.AlertRule // keyed by eventType
	calls []string                       // records which eventTypes were queried
}

func newFakeAlertRuleStore() *fakeAlertRuleStore {
	return &fakeAlertRuleStore{
		rules: make(map[string][]*models.AlertRule),
	}
}

func (f *fakeAlertRuleStore) addRule(rule *models.AlertRule) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules[rule.EventType] = append(f.rules[rule.EventType], rule)
}

func (f *fakeAlertRuleStore) GetEnabledAlertRulesByEventType(eventType string) ([]*models.AlertRule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, eventType)
	return f.rules[eventType], nil
}

// fakeAlertStateStore implements thresholdStateStore.
type fakeAlertStateStore struct {
	mu     sync.Mutex
	states map[string]*models.AlertState // keyed by ruleID+entityType+entityID+entityNodeID
	// Track UpdateAlertStateTriggered calls
	updateCalls []fakeStateUpdate
	// Track GetOrCreateAlertState calls
	getOrCreateCalls int
}

type fakeStateUpdate struct {
	id        string
	triggered bool
}

func newFakeAlertStateStore() *fakeAlertStateStore {
	return &fakeAlertStateStore{
		states: make(map[string]*models.AlertState),
	}
}

func (f *fakeAlertStateStore) key(ruleID, entityType, entityID, entityNodeID string) string {
	return ruleID + "|" + entityType + "|" + entityID + "|" + entityNodeID
}

func (f *fakeAlertStateStore) GetOrCreateAlertState(ruleID, entityType, entityID, entityNodeID string) (*models.AlertState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getOrCreateCalls++
	k := f.key(ruleID, entityType, entityID, entityNodeID)
	if s, ok := f.states[k]; ok {
		return s, nil
	}
	state := &models.AlertState{
		ID:           k,
		AlertRuleID:  ruleID,
		EntityType:   entityType,
		EntityID:     entityID,
		EntityNodeID: entityNodeID,
		Triggered:    0,
	}
	f.states[k] = state
	return state, nil
}

func (f *fakeAlertStateStore) UpdateAlertStateTriggered(id string, triggered bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateCalls = append(f.updateCalls, fakeStateUpdate{id: id, triggered: triggered})
	// Update the in-memory state too.
	for _, s := range f.states {
		if s.ID == id {
			if triggered {
				s.Triggered = 1
			} else {
				s.Triggered = 0
			}
			break
		}
	}
	return nil
}

// fakeServerMetricsProvider implements serverMetricsProvider.
type fakeServerMetricsProvider struct {
	mu       sync.Mutex
	commands []fakeCommandMetrics
}

type fakeCommandMetrics struct {
	id            string
	nodeID        string
	cpuPercent    float64
	memoryPercent float32
	diskBytes     uint64
}

func (f *fakeServerMetricsProvider) ListServerMetrics() []serverMetricsSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []serverMetricsSnapshot
	for _, cmd := range f.commands {
		out = append(out, serverMetricsSnapshot{
			serverID:      cmd.id,
			nodeID:        cmd.nodeID,
			cpuPercent:    cmd.cpuPercent,
			memoryPercent: float64(cmd.memoryPercent),
			diskBytes:     cmd.diskBytes,
		})
	}
	return out
}

// fakeNodeMetricsProvider implements nodeMetricsProvider.
type fakeNodeMetricsProvider struct {
	mu            sync.Mutex
	nodes         []nodeMetricsSnapshot
	nodeID        string
	cpuPercent    float64
	memoryPercent float64
	diskPercent   float64
	shouldErr     bool
}

func (f *fakeNodeMetricsProvider) ListNodeMetrics() []nodeMetricsSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.shouldErr {
		return nil
	}
	if f.nodes != nil {
		return append([]nodeMetricsSnapshot(nil), f.nodes...)
	}
	if f.nodeID == "" {
		return nil
	}
	return []nodeMetricsSnapshot{
		{
			nodeID:        f.nodeID,
			cpuPercent:    f.cpuPercent,
			memoryPercent: f.memoryPercent,
			diskPercent:   f.diskPercent,
		},
	}
}

// fakePlayerCountProvider implements PlayerCountProvider.
type fakePlayerCountProvider struct {
	mu     sync.Mutex
	counts map[string]int
}

func (f *fakePlayerCountProvider) GetPlayerCount(gameServerID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counts[gameServerID]
}

// --- Helper constructors ---

func makeCondition(value float64) null.Val[string] {
	type cond struct {
		Operator string  `json:"operator"`
		Value    float64 `json:"value"`
	}
	b, _ := json.Marshal(cond{Operator: ">=", Value: value})
	return null.From(string(b))
}

func makeRule(id, eventType string, condition null.Val[string], serverID null.Val[string]) *models.AlertRule {
	return &models.AlertRule{
		ID:                    id,
		UserID:                "user-1",
		ServerID:              serverID,
		EventType:             eventType,
		Condition:             condition,
		NotificationChannelID: "chan-1",
		Enabled:               1,
	}
}

// --- Tests ---

// TestThresholdPollerFetchesEnabledRules verifies that runOnce queries
// the DB for each threshold event type.
func TestThresholdPollerFetchesEnabledRules(t *testing.T) {
	ruleStore := newFakeAlertRuleStore()
	stateStore := newFakeAlertStateStore()
	serverProv := &fakeServerMetricsProvider{}
	nodeProv := &fakeNodeMetricsProvider{nodeID: "node-1"}
	playerProv := &fakePlayerCountProvider{counts: map[string]int{}}

	bus := eventbus.Get()
	poller := newThresholdPoller(ruleStore, stateStore, serverProv, nodeProv, playerProv, bus)
	poller.runOnce()

	// All seven threshold event types should have been queried.
	expectedTypes := []string{
		"ALERT_EVENT_TYPE_CPU_THRESHOLD",
		"ALERT_EVENT_TYPE_MEMORY_THRESHOLD",
		"ALERT_EVENT_TYPE_DISK_THRESHOLD",
		"ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD",
		"ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD",
		"ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD",
		"ALERT_EVENT_TYPE_NODE_DISK_THRESHOLD",
	}

	ruleStore.mu.Lock()
	calls := make(map[string]bool, len(ruleStore.calls))
	for _, c := range ruleStore.calls {
		calls[c] = true
	}
	ruleStore.mu.Unlock()

	for _, et := range expectedTypes {
		if !calls[et] {
			t.Errorf("expected event type %q to be queried, but it was not", et)
		}
	}
}

// TestThresholdPollerOKToTriggered verifies a state transition from not
// triggered to triggered publishes a ThresholdEvent with direction=entered.
func TestThresholdPollerOKToTriggered(t *testing.T) {
	ruleStore := newFakeAlertRuleStore()
	stateStore := newFakeAlertStateStore()

	rule := makeRule("rule-cpu-1", "ALERT_EVENT_TYPE_CPU_THRESHOLD",
		makeCondition(80), null.From("server-1"))
	ruleStore.addRule(rule)

	serverProv := &fakeServerMetricsProvider{
		commands: []fakeCommandMetrics{
			{id: "server-1", nodeID: "node-1", cpuPercent: 90.0},
		},
	}
	nodeProv := &fakeNodeMetricsProvider{nodeID: "node-1"}
	playerProv := &fakePlayerCountProvider{counts: map[string]int{}}

	bus := eventbus.Get()
	sub := subscribeReliable(bus, eventbus.TopicGameServerCPUThreshold)
	defer bus.Unsubscribe(eventbus.TopicGameServerCPUThreshold, sub)

	poller := newThresholdPoller(ruleStore, stateStore, serverProv, nodeProv, playerProv, bus)
	poller.runOnce()

	// Expect one update: triggered=true.
	stateStore.mu.Lock()
	updates := stateStore.updateCalls
	stateStore.mu.Unlock()

	if len(updates) != 1 {
		t.Fatalf("expected 1 state update, got %d", len(updates))
	}
	if !updates[0].triggered {
		t.Errorf("expected triggered=true, got false")
	}

	// Expect a ThresholdEvent published.
	var msg any
	select {
	case msg = <-sub:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for ThresholdEvent")
	}

	ev, ok := msg.(eventbus.ThresholdEvent)
	if !ok {
		t.Fatalf("expected ThresholdEvent, got %T", msg)
	}
	if ev.Direction != eventbus.ThresholdEntered {
		t.Errorf("direction = %q, want %q", ev.Direction, eventbus.ThresholdEntered)
	}
	if ev.ServerID != "server-1" {
		t.Errorf("serverID = %q, want %q", ev.ServerID, "server-1")
	}
	if ev.Threshold != 80 {
		t.Errorf("threshold = %v, want 80", ev.Threshold)
	}
	if ev.CurrentValue != 90.0 {
		t.Errorf("currentValue = %v, want 90.0", ev.CurrentValue)
	}
}

// TestThresholdPollerTriggeredToOK verifies that when a threshold is no longer
// breached and was previously triggered, a resolved event is published.
func TestThresholdPollerTriggeredToOK(t *testing.T) {
	ruleStore := newFakeAlertRuleStore()
	stateStore := newFakeAlertStateStore()

	rule := makeRule("rule-mem-1", "ALERT_EVENT_TYPE_MEMORY_THRESHOLD",
		makeCondition(70), null.From("server-2"))
	ruleStore.addRule(rule)

	// Pre-seed state as already triggered.
	k := "rule-mem-1|server|server-2|node-2"
	stateStore.states[k] = &models.AlertState{
		ID:           k,
		AlertRuleID:  "rule-mem-1",
		EntityType:   "server",
		EntityID:     "server-2",
		EntityNodeID: "node-2",
		Triggered:    1,
	}

	serverProv := &fakeServerMetricsProvider{
		commands: []fakeCommandMetrics{
			{id: "server-2", nodeID: "node-2", memoryPercent: 50}, // below threshold
		},
	}
	nodeProv := &fakeNodeMetricsProvider{nodeID: "node-2"}
	playerProv := &fakePlayerCountProvider{counts: map[string]int{}}

	bus := eventbus.Get()
	sub := subscribeReliable(bus, eventbus.TopicGameServerMemoryThreshold)
	defer bus.Unsubscribe(eventbus.TopicGameServerMemoryThreshold, sub)

	poller := newThresholdPoller(ruleStore, stateStore, serverProv, nodeProv, playerProv, bus)
	poller.runOnce()

	stateStore.mu.Lock()
	updates := stateStore.updateCalls
	stateStore.mu.Unlock()

	if len(updates) != 1 {
		t.Fatalf("expected 1 state update, got %d", len(updates))
	}
	if updates[0].triggered {
		t.Errorf("expected triggered=false (resolved), got true")
	}

	var msg any
	select {
	case msg = <-sub:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for ThresholdEvent")
	}

	ev, ok := msg.(eventbus.ThresholdEvent)
	if !ok {
		t.Fatalf("expected ThresholdEvent, got %T", msg)
	}
	if ev.Direction != eventbus.ThresholdResolved {
		t.Errorf("direction = %q, want %q", ev.Direction, eventbus.ThresholdResolved)
	}
}

// TestThresholdPollerNoStateChangeNoEvent verifies that when the threshold is
// already triggered and still breached, no event is published and no state
// update is made.
func TestThresholdPollerNoStateChangeNoEvent(t *testing.T) {
	ruleStore := newFakeAlertRuleStore()
	stateStore := newFakeAlertStateStore()

	rule := makeRule("rule-cpu-2", "ALERT_EVENT_TYPE_CPU_THRESHOLD",
		makeCondition(50), null.From("server-3"))
	ruleStore.addRule(rule)

	// Pre-seed as already triggered.
	k := "rule-cpu-2|server|server-3|node-3"
	stateStore.states[k] = &models.AlertState{
		ID:           k,
		AlertRuleID:  "rule-cpu-2",
		EntityType:   "server",
		EntityID:     "server-3",
		EntityNodeID: "node-3",
		Triggered:    1,
	}

	serverProv := &fakeServerMetricsProvider{
		commands: []fakeCommandMetrics{
			{id: "server-3", nodeID: "node-3", cpuPercent: 75.0}, // still above 50
		},
	}
	nodeProv := &fakeNodeMetricsProvider{nodeID: "node-3"}
	playerProv := &fakePlayerCountProvider{counts: map[string]int{}}

	bus := eventbus.Get()
	sub := subscribeReliable(bus, eventbus.TopicGameServerCPUThreshold)
	defer bus.Unsubscribe(eventbus.TopicGameServerCPUThreshold, sub)

	poller := newThresholdPoller(ruleStore, stateStore, serverProv, nodeProv, playerProv, bus)
	poller.runOnce()

	stateStore.mu.Lock()
	updates := stateStore.updateCalls
	stateStore.mu.Unlock()

	if len(updates) != 0 {
		t.Errorf("expected 0 state updates (no transition), got %d", len(updates))
	}

	select {
	case <-sub:
		t.Error("expected no event published for no state change")
	case <-time.After(50 * time.Millisecond):
		// Good — no event.
	}
}

// TestThresholdPollerAllServersRule verifies that a rule with NULL ServerID is
// evaluated against each running server independently, creating separate state rows.
func TestThresholdPollerAllServersRule(t *testing.T) {
	ruleStore := newFakeAlertRuleStore()
	stateStore := newFakeAlertStateStore()

	// NULL ServerID = all servers.
	rule := makeRule("rule-all-cpu", "ALERT_EVENT_TYPE_CPU_THRESHOLD",
		makeCondition(60), null.Val[string]{})
	ruleStore.addRule(rule)

	serverProv := &fakeServerMetricsProvider{
		commands: []fakeCommandMetrics{
			{id: "srv-a", nodeID: "node-1", cpuPercent: 80.0}, // triggers
			{id: "srv-b", nodeID: "node-1", cpuPercent: 40.0}, // does not trigger
			{id: "srv-c", nodeID: "node-1", cpuPercent: 70.0}, // triggers
		},
	}
	nodeProv := &fakeNodeMetricsProvider{nodeID: "node-1"}
	playerProv := &fakePlayerCountProvider{counts: map[string]int{}}

	bus := eventbus.Get()
	sub := subscribeReliable(bus, eventbus.TopicGameServerCPUThreshold)
	defer bus.Unsubscribe(eventbus.TopicGameServerCPUThreshold, sub)

	poller := newThresholdPoller(ruleStore, stateStore, serverProv, nodeProv, playerProv, bus)
	poller.runOnce()

	stateStore.mu.Lock()
	updates := stateStore.updateCalls
	stateStore.mu.Unlock()

	// Expect 2 state updates (srv-a and srv-c triggered).
	triggered := 0
	for _, u := range updates {
		if u.triggered {
			triggered++
		}
	}
	if triggered != 2 {
		t.Errorf("expected 2 triggered state updates, got %d (total updates: %d)", triggered, len(updates))
	}

	// Expect 2 events published.
	events := 0
	deadline := time.After(200 * time.Millisecond)
loop:
	for {
		select {
		case <-sub:
			events++
		case <-deadline:
			break loop
		}
	}
	if events != 2 {
		t.Errorf("expected 2 ThresholdEvents published, got %d", events)
	}
}

// TestThresholdPollerNodeRuleEvaluated verifies that node-level rules are
// evaluated against node metrics.
func TestThresholdPollerNodeRuleEvaluated(t *testing.T) {
	ruleStore := newFakeAlertRuleStore()
	stateStore := newFakeAlertStateStore()

	rule := makeRule("rule-node-cpu", "ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD",
		makeCondition(50), null.Val[string]{})
	rule.NodeID = null.From("node-x")
	ruleStore.addRule(rule)

	serverProv := &fakeServerMetricsProvider{}
	nodeProv := &fakeNodeMetricsProvider{
		nodeID:     "node-x",
		cpuPercent: 75.0,
	}
	playerProv := &fakePlayerCountProvider{counts: map[string]int{}}

	bus := eventbus.Get()
	sub := subscribeReliable(bus, eventbus.TopicNodeCPUThreshold)
	defer bus.Unsubscribe(eventbus.TopicNodeCPUThreshold, sub)

	poller := newThresholdPoller(ruleStore, stateStore, serverProv, nodeProv, playerProv, bus)
	poller.runOnce()

	stateStore.mu.Lock()
	updates := stateStore.updateCalls
	stateStore.mu.Unlock()

	if len(updates) != 1 {
		t.Fatalf("expected 1 state update, got %d", len(updates))
	}
	if !updates[0].triggered {
		t.Errorf("expected triggered=true for node rule")
	}

	var msg any
	select {
	case msg = <-sub:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for NodeThresholdEvent")
	}

	ev, ok := msg.(eventbus.NodeThresholdEvent)
	if !ok {
		t.Fatalf("expected NodeThresholdEvent, got %T", msg)
	}
	if ev.Direction != eventbus.ThresholdEntered {
		t.Errorf("direction = %q, want %q", ev.Direction, eventbus.ThresholdEntered)
	}
	if ev.NodeID != "node-x" {
		t.Errorf("nodeID = %q, want %q", ev.NodeID, "node-x")
	}
}

func TestThresholdPollerAllNodesRuleEvaluatesEveryNode(t *testing.T) {
	ruleStore := newFakeAlertRuleStore()
	stateStore := newFakeAlertStateStore()

	rule := makeRule("rule-all-node-memory", "ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD",
		makeCondition(70), null.Val[string]{})
	ruleStore.addRule(rule)

	serverProv := &fakeServerMetricsProvider{}
	nodeProv := &fakeNodeMetricsProvider{
		nodes: []nodeMetricsSnapshot{
			{nodeID: "node-a", memoryPercent: 75.0},
			{nodeID: "node-b", memoryPercent: 71.0},
			{nodeID: "node-c", memoryPercent: 55.0},
		},
	}
	playerProv := &fakePlayerCountProvider{counts: map[string]int{}}

	bus := eventbus.Get()
	sub := subscribeReliable(bus, eventbus.TopicNodeMemoryThreshold)
	defer bus.Unsubscribe(eventbus.TopicNodeMemoryThreshold, sub)

	poller := newThresholdPoller(ruleStore, stateStore, serverProv, nodeProv, playerProv, bus)
	poller.runOnce()

	stateStore.mu.Lock()
	updates := append([]fakeStateUpdate(nil), stateStore.updateCalls...)
	stateStore.mu.Unlock()

	triggered := 0
	for _, update := range updates {
		if update.triggered {
			triggered++
		}
	}
	if triggered != 2 {
		t.Fatalf("triggered updates = %d, want 2", triggered)
	}

	seen := map[string]bool{}
	deadline := time.After(200 * time.Millisecond)
loop:
	for {
		select {
		case msg := <-sub:
			ev, ok := msg.(eventbus.NodeThresholdEvent)
			if !ok {
				t.Fatalf("expected NodeThresholdEvent, got %T", msg)
			}
			seen[ev.NodeID] = true
		case <-deadline:
			break loop
		}
	}

	if !seen["node-a"] || !seen["node-b"] {
		t.Fatalf("node events = %+v, want node-a and node-b", seen)
	}
	if seen["node-c"] {
		t.Fatalf("node events = %+v, did not expect node-c", seen)
	}
}

func TestRegistryNodeMetricsProviderListsEveryRegisteredNodeAndSkipsFailures(t *testing.T) {
	registry := noderegistry.New("node-a", &nodeclient.FakeNodeClient{
		NodeID: "node-a",
		SnapshotResult: &node.NodeSnapshot{
			CPUPercent:    61,
			MemoryPercent: 62,
			DiskPercent:   63,
		},
	})
	registry.Register(&nodeclient.FakeNodeClient{
		NodeID: "node-b",
		SnapshotResult: &node.NodeSnapshot{
			CPUPercent:    71,
			MemoryPercent: 72,
			DiskPercent:   73,
		},
	})
	registry.Register(&nodeclient.FakeNodeClient{
		NodeID:      "node-failing",
		SnapshotErr: errors.New("snapshot failed"),
	})

	provider := &registryNodeMetricsProvider{
		ctx:      context.Background(),
		registry: registry,
	}

	snapshots := provider.ListNodeMetrics()
	if len(snapshots) != 2 {
		t.Fatalf("ListNodeMetrics() len = %d, want 2", len(snapshots))
	}

	seen := map[string]nodeMetricsSnapshot{}
	for _, snapshot := range snapshots {
		seen[snapshot.nodeID] = snapshot
	}

	nodeA, ok := seen["node-a"]
	if !ok {
		t.Fatal("missing node-a metrics")
	}
	if nodeA.cpuPercent != 61 || nodeA.memoryPercent != 62 || nodeA.diskPercent != 63 {
		t.Fatalf("node-a metrics = %+v, want cpu=61 memory=62 disk=63", nodeA)
	}

	nodeB, ok := seen["node-b"]
	if !ok {
		t.Fatal("missing node-b metrics")
	}
	if nodeB.cpuPercent != 71 || nodeB.memoryPercent != 72 || nodeB.diskPercent != 73 {
		t.Fatalf("node-b metrics = %+v, want cpu=71 memory=72 disk=73", nodeB)
	}

	_, ok = seen["node-failing"]
	if ok {
		t.Fatal("failing node should have been skipped")
	}
}

// TestThresholdPollerRuleCacheRefresh verifies that rules are refreshed from the
// DB when the cache is stale (older than the refresh interval).
func TestThresholdPollerRuleCacheRefresh(t *testing.T) {
	ruleStore := newFakeAlertRuleStore()
	stateStore := newFakeAlertStateStore()
	serverProv := &fakeServerMetricsProvider{}
	nodeProv := &fakeNodeMetricsProvider{nodeID: "node-1"}
	playerProv := &fakePlayerCountProvider{counts: map[string]int{}}

	bus := eventbus.Get()
	poller := newThresholdPoller(ruleStore, stateStore, serverProv, nodeProv, playerProv, bus)

	// Force the cache to appear stale.
	poller.rulesCachedAt = time.Time{}

	// Run once — should populate the cache.
	poller.runOnce()

	ruleStore.mu.Lock()
	firstCallCount := len(ruleStore.calls)
	ruleStore.mu.Unlock()

	if firstCallCount == 0 {
		t.Fatal("expected DB calls on first runOnce with stale cache")
	}

	// Run again immediately — cache should be fresh so no new DB calls.
	ruleStore.mu.Lock()
	ruleStore.calls = nil
	ruleStore.mu.Unlock()

	poller.runOnce()

	ruleStore.mu.Lock()
	secondCallCount := len(ruleStore.calls)
	ruleStore.mu.Unlock()

	if secondCallCount != 0 {
		t.Errorf("expected 0 DB calls on second runOnce (cache fresh), got %d", secondCallCount)
	}

	// Force stale again — should re-query.
	poller.rulesCachedAt = time.Now().Add(-rulesCacheTTL - time.Second)

	ruleStore.mu.Lock()
	ruleStore.calls = nil
	ruleStore.mu.Unlock()

	poller.runOnce()

	ruleStore.mu.Lock()
	thirdCallCount := len(ruleStore.calls)
	ruleStore.mu.Unlock()

	if thirdCallCount == 0 {
		t.Error("expected DB calls on third runOnce with stale cache")
	}
}

// TestThresholdPollerPlayerCountRule verifies player count threshold evaluation.
func TestThresholdPollerPlayerCountRule(t *testing.T) {
	ruleStore := newFakeAlertRuleStore()
	stateStore := newFakeAlertStateStore()

	rule := makeRule("rule-players-1", "ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD",
		makeCondition(10), null.From("server-4"))
	ruleStore.addRule(rule)

	serverProv := &fakeServerMetricsProvider{
		commands: []fakeCommandMetrics{
			{id: "server-4", nodeID: "node-1"},
		},
	}
	nodeProv := &fakeNodeMetricsProvider{nodeID: "node-1"}
	playerProv := &fakePlayerCountProvider{counts: map[string]int{"server-4": 15}}

	bus := eventbus.Get()
	sub := subscribeReliable(bus, eventbus.TopicGameServerPlayerThreshold)
	defer bus.Unsubscribe(eventbus.TopicGameServerPlayerThreshold, sub)

	poller := newThresholdPoller(ruleStore, stateStore, serverProv, nodeProv, playerProv, bus)
	poller.runOnce()

	stateStore.mu.Lock()
	updates := stateStore.updateCalls
	stateStore.mu.Unlock()

	if len(updates) != 1 {
		t.Fatalf("expected 1 state update, got %d", len(updates))
	}
	if !updates[0].triggered {
		t.Errorf("expected triggered=true for player count rule")
	}

	var msg any
	select {
	case msg = <-sub:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for ThresholdEvent")
	}

	ev, ok := msg.(eventbus.ThresholdEvent)
	if !ok {
		t.Fatalf("expected ThresholdEvent, got %T", msg)
	}
	if ev.CurrentValue != 15.0 {
		t.Errorf("currentValue = %v, want 15.0", ev.CurrentValue)
	}
}

// TestThresholdPollerDisabledRuleSkipped verifies that disabled rules are not
// evaluated.
func TestThresholdPollerDisabledRuleSkipped(t *testing.T) {
	ruleStore := newFakeAlertRuleStore()
	stateStore := newFakeAlertStateStore()

	rule := makeRule("rule-disabled", "ALERT_EVENT_TYPE_CPU_THRESHOLD",
		makeCondition(1), null.From("server-5"))
	rule.Enabled = 0 // disabled
	ruleStore.addRule(rule)

	serverProv := &fakeServerMetricsProvider{
		commands: []fakeCommandMetrics{
			{id: "server-5", nodeID: "node-1", cpuPercent: 99.0},
		},
	}
	nodeProv := &fakeNodeMetricsProvider{nodeID: "node-1"}
	playerProv := &fakePlayerCountProvider{counts: map[string]int{}}

	bus := eventbus.Get()
	sub := subscribeReliable(bus, eventbus.TopicGameServerCPUThreshold)
	defer bus.Unsubscribe(eventbus.TopicGameServerCPUThreshold, sub)

	poller := newThresholdPoller(ruleStore, stateStore, serverProv, nodeProv, playerProv, bus)
	poller.runOnce()

	stateStore.mu.Lock()
	updates := stateStore.updateCalls
	stateStore.mu.Unlock()

	if len(updates) != 0 {
		t.Errorf("expected 0 state updates for disabled rule, got %d", len(updates))
	}

	select {
	case <-sub:
		t.Error("expected no event for disabled rule")
	case <-time.After(50 * time.Millisecond):
		// Good.
	}
}

// TestEvaluateThresholdOp has been moved to pkg/alerts/threshold-event-data_test.go
// where the shared EvaluateThresholdOp function is defined and tested.

// TestThresholdPollerStateCaching verifies that the poller caches alert state
// in memory, avoiding redundant DB queries on subsequent ticks when no state
// transition has occurred.
func TestThresholdPollerStateCaching(t *testing.T) {
	ruleStore := newFakeAlertRuleStore()
	stateStore := newFakeAlertStateStore()

	rule := makeRule("rule-cache-test", "ALERT_EVENT_TYPE_CPU_THRESHOLD",
		makeCondition(80), null.From("server-cache"))
	ruleStore.addRule(rule)

	serverProv := &fakeServerMetricsProvider{
		commands: []fakeCommandMetrics{
			{id: "server-cache", nodeID: "node-1", cpuPercent: 90.0},
		},
	}
	nodeProv := &fakeNodeMetricsProvider{nodeID: "node-1"}
	playerProv := &fakePlayerCountProvider{counts: map[string]int{}}

	bus := eventbus.Get()
	sub := subscribeReliable(bus, eventbus.TopicGameServerCPUThreshold)
	defer bus.Unsubscribe(eventbus.TopicGameServerCPUThreshold, sub)

	poller := newThresholdPoller(ruleStore, stateStore, serverProv, nodeProv, playerProv, bus)

	// First tick: should query the DB for state (cache miss).
	poller.runOnce()

	stateStore.mu.Lock()
	firstGetCalls := stateStore.getOrCreateCalls
	stateStore.mu.Unlock()

	if firstGetCalls != 1 {
		t.Fatalf("expected 1 DB state call on first tick, got %d", firstGetCalls)
	}

	// Drain the event from first tick.
	select {
	case <-sub:
	case <-time.After(100 * time.Millisecond):
	}

	// Second tick: same state (still triggered, still breached) — should NOT
	// query the DB for state (cache hit).
	stateStore.mu.Lock()
	stateStore.getOrCreateCalls = 0
	stateStore.mu.Unlock()

	poller.runOnce()

	stateStore.mu.Lock()
	secondGetCalls := stateStore.getOrCreateCalls
	stateStore.mu.Unlock()

	if secondGetCalls != 0 {
		t.Errorf("expected 0 DB state calls on second tick (cached), got %d", secondGetCalls)
	}
}

// TestThresholdPollerStateCacheInvalidatedOnRuleRefresh verifies that the
// state cache is cleared when rules are refreshed (cache TTL expiry).
func TestThresholdPollerStateCacheInvalidatedOnRuleRefresh(t *testing.T) {
	ruleStore := newFakeAlertRuleStore()
	stateStore := newFakeAlertStateStore()

	rule := makeRule("rule-invalidate", "ALERT_EVENT_TYPE_CPU_THRESHOLD",
		makeCondition(80), null.From("server-inv"))
	ruleStore.addRule(rule)

	serverProv := &fakeServerMetricsProvider{
		commands: []fakeCommandMetrics{
			{id: "server-inv", nodeID: "node-1", cpuPercent: 90.0},
		},
	}
	nodeProv := &fakeNodeMetricsProvider{nodeID: "node-1"}
	playerProv := &fakePlayerCountProvider{counts: map[string]int{}}

	bus := eventbus.Get()
	sub := subscribeReliable(bus, eventbus.TopicGameServerCPUThreshold)
	defer bus.Unsubscribe(eventbus.TopicGameServerCPUThreshold, sub)

	poller := newThresholdPoller(ruleStore, stateStore, serverProv, nodeProv, playerProv, bus)

	// First tick populates state cache.
	poller.runOnce()

	// Drain event.
	select {
	case <-sub:
	case <-time.After(100 * time.Millisecond):
	}

	// Force rule cache to appear stale so next runOnce refreshes rules.
	poller.rulesCachedAt = time.Now().Add(-rulesCacheTTL - time.Second)

	stateStore.mu.Lock()
	stateStore.getOrCreateCalls = 0
	stateStore.mu.Unlock()

	// This tick should refresh rules and invalidate the state cache, causing
	// a fresh DB lookup for state.
	poller.runOnce()

	stateStore.mu.Lock()
	callsAfterRefresh := stateStore.getOrCreateCalls
	stateStore.mu.Unlock()

	if callsAfterRefresh == 0 {
		t.Error("expected DB state call after rule cache refresh, got 0")
	}
}
