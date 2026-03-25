package actions

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/alerts"
	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/pkg/sysinfo"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

const (
	// thresholdPollInterval is how often the poller ticks.
	thresholdPollInterval = 5 * time.Second

	// rulesCacheTTL is how long cached rules are considered fresh.
	rulesCacheTTL = 30 * time.Second
)

// thresholdRuleStore is the subset of db.Connection methods the poller needs
// for fetching rules.
type thresholdRuleStore interface {
	GetEnabledAlertRulesByEventType(eventType string) ([]*models.AlertRule, error)
}

// thresholdStateStore is the subset of db.Connection methods the poller needs
// for managing alert state.
type thresholdStateStore interface {
	GetOrCreateAlertState(ruleID, entityType, entityID, entityNodeID string) (*models.AlertState, error)
	UpdateAlertStateTriggered(id string, triggered bool) error
}

// serverMetricsSnapshot holds a single server's current metrics.
type serverMetricsSnapshot struct {
	serverID      string
	nodeID        string
	cpuPercent    float64
	memoryPercent float64
	diskBytes     uint64
}

// serverMetricsProvider abstracts listing running server metrics.
type serverMetricsProvider interface {
	ListServerMetrics() []serverMetricsSnapshot
}

// nodeMetricsProvider abstracts collecting node-level resource metrics.
type nodeMetricsProvider interface {
	CollectNodeMetrics() (nodeID string, cpuPercent, memoryPercent, diskPercent float64, err error)
}

// supervisorMetricsProvider wraps *supervisor.Instance to implement
// serverMetricsProvider using live process metrics.
type supervisorMetricsProvider struct {
	inst *supervisor.Instance
}

func (s *supervisorMetricsProvider) ListServerMetrics() []serverMetricsSnapshot {
	if s.inst == nil {
		return nil
	}
	commands := s.inst.ListCommands()
	out := make([]serverMetricsSnapshot, 0, len(commands))
	for _, cmd := range commands {
		ptr, errGet := s.inst.GetCommandByID(cmd.ID)
		if errGet != nil {
			continue
		}
		cpuPercent, _, _, memoryPercent, _, _, diskBytes, _, _, _ := ptr.Metrics()
		out = append(out, serverMetricsSnapshot{
			serverID:      cmd.ID,
			nodeID:        ptr.NodeID(),
			cpuPercent:    cpuPercent,
			memoryPercent: float64(memoryPercent),
			diskBytes:     diskBytes,
		})
	}
	return out
}

// sysinfoNodeMetricsProvider wraps sysinfo.CollectResourceSnapshot and a static
// nodeID to implement nodeMetricsProvider.
type sysinfoNodeMetricsProvider struct {
	nodeID string
}

func (s *sysinfoNodeMetricsProvider) CollectNodeMetrics() (nodeID string, cpuPercent, memoryPercent, diskPercent float64, err error) {
	snap, errSnap := sysinfo.CollectResourceSnapshot()
	if errSnap != nil {
		return "", 0, 0, 0, errSnap
	}
	return s.nodeID, snap.CPUPercent, snap.MemoryPercent, snap.DiskPercent, nil
}

// cachedRules holds one event-type's rules and when they were fetched.
type cachedRules struct {
	rules     []*models.AlertRule
	fetchedAt time.Time
}

// cachedAlertState holds an in-memory copy of an alert state row, avoiding
// repeated DB round-trips for unchanged states.
type cachedAlertState struct {
	id        string
	triggered bool
}

// cachedCondition holds a pre-parsed rule condition so parseCondition does not
// need to deserialize JSON on every tick.
type cachedCondition struct {
	operator  string
	threshold float64
	err       error
}

// thresholdPoller is the background task that periodically checks metrics
// against threshold alert rules and publishes events on state transitions.
type thresholdPoller struct {
	ruleStore    thresholdRuleStore
	stateStore   thresholdStateStore
	serverProv   serverMetricsProvider
	nodeProv     nodeMetricsProvider
	playerCounts PlayerCountProvider
	bus          *eventbus.EventBus

	// Rule cache — refreshed every rulesCacheTTL.
	ruleCache     map[string]cachedRules // keyed by eventType
	rulesCachedAt time.Time

	// State cache — keyed by "ruleID|entityType|entityID|entityNodeID".
	// Loaded from DB on cache miss, updated on state transitions.
	// Invalidated when rules are refreshed.
	stateCache map[string]*cachedAlertState

	// Condition cache — keyed by rule ID. Invalidated when rules are refreshed.
	conditionCache map[string]*cachedCondition
}

func newThresholdPoller(
	ruleStore thresholdRuleStore,
	stateStore thresholdStateStore,
	serverProv serverMetricsProvider,
	nodeProv nodeMetricsProvider,
	playerCounts PlayerCountProvider,
	bus *eventbus.EventBus,
) *thresholdPoller {
	return &thresholdPoller{
		ruleStore:      ruleStore,
		stateStore:     stateStore,
		serverProv:     serverProv,
		nodeProv:       nodeProv,
		playerCounts:   playerCounts,
		bus:            bus,
		ruleCache:      make(map[string]cachedRules),
		stateCache:     make(map[string]*cachedAlertState),
		conditionCache: make(map[string]*cachedCondition),
	}
}

// stateKey builds the cache key for alert state lookups.
func stateKey(ruleID, entityType, entityID, entityNodeID string) string {
	return ruleID + "|" + entityType + "|" + entityID + "|" + entityNodeID
}

// getOrCreateStateCached returns the cached alert state or loads it from the DB
// on a cache miss.
func (p *thresholdPoller) getOrCreateStateCached(ruleID, entityType, entityID, entityNodeID string) (*cachedAlertState, error) {
	k := stateKey(ruleID, entityType, entityID, entityNodeID)
	cached, ok := p.stateCache[k]
	if ok {
		return cached, nil
	}
	state, errState := p.stateStore.GetOrCreateAlertState(ruleID, entityType, entityID, entityNodeID)
	if errState != nil {
		return nil, errState
	}
	entry := &cachedAlertState{
		id:        state.ID,
		triggered: state.Triggered != 0,
	}
	p.stateCache[k] = entry
	return entry, nil
}

// getConditionCached returns the cached parsed condition for the given rule, or
// parses and caches it on first access.
func (p *thresholdPoller) getConditionCached(rule *models.AlertRule) (operator string, threshold float64, err error) {
	cached, ok := p.conditionCache[rule.ID]
	if ok {
		return cached.operator, cached.threshold, cached.err
	}
	threshold, operator, errParse := parseCondition(rule.Condition)
	p.conditionCache[rule.ID] = &cachedCondition{
		operator:  operator,
		threshold: threshold,
		err:       errParse,
	}
	return operator, threshold, errParse
}

// thresholdEventTypes lists all DB event type strings for threshold rules,
// paired with their event bus topics.
var thresholdEventTypes = []struct {
	eventType string
	topic     string
	isNode    bool
}{
	{"ALERT_EVENT_TYPE_CPU_THRESHOLD", eventbus.TopicGameServerCPUThreshold, false},
	{"ALERT_EVENT_TYPE_MEMORY_THRESHOLD", eventbus.TopicGameServerMemoryThreshold, false},
	{"ALERT_EVENT_TYPE_DISK_THRESHOLD", eventbus.TopicGameServerDiskThreshold, false},
	{"ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD", eventbus.TopicGameServerPlayerThreshold, false},
	{"ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD", eventbus.TopicNodeCPUThreshold, true},
	{"ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD", eventbus.TopicNodeMemoryThreshold, true},
	{"ALERT_EVENT_TYPE_NODE_DISK_THRESHOLD", eventbus.TopicNodeDiskThreshold, true},
}

// getRules returns rules for the given eventType, using the cache when fresh.
func (p *thresholdPoller) getRules(eventType string) ([]*models.AlertRule, error) {
	if cached, ok := p.ruleCache[eventType]; ok {
		if time.Since(cached.fetchedAt) < rulesCacheTTL {
			return cached.rules, nil
		}
	}
	rules, errGet := p.ruleStore.GetEnabledAlertRulesByEventType(eventType)
	if errGet != nil {
		return nil, fmt.Errorf("failed to fetch rules for %s: %w", eventType, errGet)
	}
	p.ruleCache[eventType] = cachedRules{rules: rules, fetchedAt: time.Now()}
	return rules, nil
}

// refreshRuleCache forces a re-fetch of all event types and resets the cache
// timestamp. It also invalidates the state and condition caches since rule
// changes may affect which states and conditions are relevant.
func (p *thresholdPoller) refreshRuleCache() {
	for _, et := range thresholdEventTypes {
		rules, errGet := p.ruleStore.GetEnabledAlertRulesByEventType(et.eventType)
		if errGet != nil {
			log.Error().Err(errGet).Str("event_type", et.eventType).Msg("Threshold poller: failed to refresh rule cache")
			continue
		}
		p.ruleCache[et.eventType] = cachedRules{rules: rules, fetchedAt: time.Now()}
	}
	p.rulesCachedAt = time.Now()

	// Invalidate dependent caches — rules may have changed conditions or been
	// removed, so stale state entries could cause incorrect suppression.
	p.stateCache = make(map[string]*cachedAlertState)
	p.conditionCache = make(map[string]*cachedCondition)
}

// runOnce executes a single evaluation cycle: fetches metrics, evaluates rules,
// manages state, and publishes events on transitions.
func (p *thresholdPoller) runOnce() {
	// Refresh rule cache if stale.
	if time.Since(p.rulesCachedAt) >= rulesCacheTTL {
		p.refreshRuleCache()
	}

	// Evaluate server-level threshold rules.
	serverSnapshots := p.serverProv.ListServerMetrics()
	p.evaluateServerRules(serverSnapshots)

	// Evaluate node-level threshold rules.
	p.evaluateNodeRules()
}

// evaluateServerRules evaluates all server-level threshold event types.
func (p *thresholdPoller) evaluateServerRules(snapshots []serverMetricsSnapshot) {
	serverEventTypes := []struct {
		eventType string
		topic     string
		getValue  func(snap serverMetricsSnapshot, pc PlayerCountProvider) float64
	}{
		{
			"ALERT_EVENT_TYPE_CPU_THRESHOLD",
			eventbus.TopicGameServerCPUThreshold,
			func(snap serverMetricsSnapshot, _ PlayerCountProvider) float64 {
				return snap.cpuPercent
			},
		},
		{
			"ALERT_EVENT_TYPE_MEMORY_THRESHOLD",
			eventbus.TopicGameServerMemoryThreshold,
			func(snap serverMetricsSnapshot, _ PlayerCountProvider) float64 {
				return snap.memoryPercent
			},
		},
		{
			"ALERT_EVENT_TYPE_DISK_THRESHOLD",
			eventbus.TopicGameServerDiskThreshold,
			func(snap serverMetricsSnapshot, _ PlayerCountProvider) float64 {
				return float64(snap.diskBytes)
			},
		},
		{
			"ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD",
			eventbus.TopicGameServerPlayerThreshold,
			func(snap serverMetricsSnapshot, pc PlayerCountProvider) float64 {
				if pc == nil {
					return 0
				}
				return float64(pc.GetPlayerCount(snap.serverID))
			},
		},
	}

	for _, et := range serverEventTypes {
		rules, errGet := p.getRules(et.eventType)
		if errGet != nil {
			log.Error().Err(errGet).Str("event_type", et.eventType).Msg("Threshold poller: failed to get rules")
			continue
		}

		for _, rule := range rules {
			if rule.Enabled == 0 {
				continue
			}

			// Determine which servers to evaluate this rule against.
			var targets []serverMetricsSnapshot
			ruleServerID, serverIDSet := rule.ServerID.Get()
			if serverIDSet {
				// Specific server rule.
				for _, snap := range snapshots {
					if snap.serverID == ruleServerID {
						targets = append(targets, snap)
						break
					}
				}
			} else {
				// All-servers rule — evaluate against every running server.
				targets = snapshots
			}

			for _, snap := range targets {
				currentValue := et.getValue(snap, p.playerCounts)
				op, threshold, errParse := p.getConditionCached(rule)
				if errParse != nil {
					log.Warn().Err(errParse).Str("rule_id", rule.ID).Msg("Threshold poller: failed to parse condition")
					continue
				}

				breached, errEval := alerts.EvaluateThresholdOp(op, threshold, currentValue)
				if errEval != nil {
					log.Warn().Err(errEval).Str("rule_id", rule.ID).Msg("Threshold poller: failed to evaluate threshold")
					continue
				}

				cached, errState := p.getOrCreateStateCached(rule.ID, "server", snap.serverID, snap.nodeID)
				if errState != nil {
					log.Error().Err(errState).Str("rule_id", rule.ID).Str("server_id", snap.serverID).Msg("Threshold poller: failed to get/create alert state")
					continue
				}

				if breached && !cached.triggered {
					// Transition: OK → triggered.
					errUpdate := p.stateStore.UpdateAlertStateTriggered(cached.id, true)
					if errUpdate != nil {
						log.Error().Err(errUpdate).Str("state_id", cached.id).Msg("Threshold poller: failed to update state to triggered")
						continue
					}
					cached.triggered = true
					p.bus.Publish(et.topic, eventbus.ThresholdEvent{
						ServerID:     snap.serverID,
						ServerNodeID: snap.nodeID,
						CurrentValue: currentValue,
						Threshold:    threshold,
						Direction:    eventbus.ThresholdEntered,
					})
				} else if !breached && cached.triggered {
					// Transition: triggered → OK.
					errUpdate := p.stateStore.UpdateAlertStateTriggered(cached.id, false)
					if errUpdate != nil {
						log.Error().Err(errUpdate).Str("state_id", cached.id).Msg("Threshold poller: failed to update state to resolved")
						continue
					}
					cached.triggered = false
					p.bus.Publish(et.topic, eventbus.ThresholdEvent{
						ServerID:     snap.serverID,
						ServerNodeID: snap.nodeID,
						CurrentValue: currentValue,
						Threshold:    threshold,
						Direction:    eventbus.ThresholdResolved,
					})
				}
				// No state change: no event published.
			}
		}
	}
}

// evaluateNodeRules evaluates all node-level threshold event types.
func (p *thresholdPoller) evaluateNodeRules() {
	nodeID, cpuPercent, memoryPercent, diskPercent, errCollect := p.nodeProv.CollectNodeMetrics()
	if errCollect != nil {
		log.Error().Err(errCollect).Msg("Threshold poller: failed to collect node metrics")
		return
	}

	nodeEventTypes := []struct {
		eventType string
		topic     string
		value     float64
	}{
		{"ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD", eventbus.TopicNodeCPUThreshold, cpuPercent},
		{"ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD", eventbus.TopicNodeMemoryThreshold, memoryPercent},
		{"ALERT_EVENT_TYPE_NODE_DISK_THRESHOLD", eventbus.TopicNodeDiskThreshold, diskPercent},
	}

	for _, et := range nodeEventTypes {
		rules, errGet := p.getRules(et.eventType)
		if errGet != nil {
			log.Error().Err(errGet).Str("event_type", et.eventType).Msg("Threshold poller: failed to get node rules")
			continue
		}

		for _, rule := range rules {
			if rule.Enabled == 0 {
				continue
			}

			// Check if this rule targets our node or all nodes.
			ruleNodeID, nodeIDSet := rule.NodeID.Get()
			if nodeIDSet && ruleNodeID != nodeID {
				continue
			}

			op, threshold, errParse := p.getConditionCached(rule)
			if errParse != nil {
				log.Warn().Err(errParse).Str("rule_id", rule.ID).Msg("Threshold poller: failed to parse node rule condition")
				continue
			}

			breached, errEval := alerts.EvaluateThresholdOp(op, threshold, et.value)
			if errEval != nil {
				log.Warn().Err(errEval).Str("rule_id", rule.ID).Msg("Threshold poller: failed to evaluate node threshold")
				continue
			}

			cached, errState := p.getOrCreateStateCached(rule.ID, "node", nodeID, "")
			if errState != nil {
				log.Error().Err(errState).Str("rule_id", rule.ID).Str("node_id", nodeID).Msg("Threshold poller: failed to get/create node alert state")
				continue
			}

			if breached && !cached.triggered {
				errUpdate := p.stateStore.UpdateAlertStateTriggered(cached.id, true)
				if errUpdate != nil {
					log.Error().Err(errUpdate).Str("state_id", cached.id).Msg("Threshold poller: failed to update node state to triggered")
					continue
				}
				cached.triggered = true
				p.bus.Publish(et.topic, eventbus.NodeThresholdEvent{
					NodeID:       nodeID,
					CurrentValue: et.value,
					Threshold:    threshold,
					Direction:    eventbus.ThresholdEntered,
				})
			} else if !breached && cached.triggered {
				errUpdate := p.stateStore.UpdateAlertStateTriggered(cached.id, false)
				if errUpdate != nil {
					log.Error().Err(errUpdate).Str("state_id", cached.id).Msg("Threshold poller: failed to update node state to resolved")
					continue
				}
				cached.triggered = false
				p.bus.Publish(et.topic, eventbus.NodeThresholdEvent{
					NodeID:       nodeID,
					CurrentValue: et.value,
					Threshold:    threshold,
					Direction:    eventbus.ThresholdResolved,
				})
			}
		}
	}
}

// parseCondition extracts the operator and threshold value from a rule's
// condition field. Returns an error if the condition is missing or malformed.
func parseCondition(condition interface{ Get() (string, bool) }) (threshold float64, operator string, err error) {
	condStr, isSet := condition.Get()
	if !isSet || condStr == "" {
		return 0, "", fmt.Errorf("rule has no condition set")
	}
	op, thresh, errParse := alerts.ParseConditionJSON(condStr)
	if errParse != nil {
		return 0, "", errParse
	}
	return thresh, op, nil
}

// backgroundJobThresholdPoller is the long-running background goroutine that
// ticks every thresholdPollInterval and evaluates threshold rules.
func (inst *Instance) backgroundJobThresholdPoller(localNodeID string) {
	serverProv := &supervisorMetricsProvider{inst: inst.supervisorInstance}
	nodeProv := &sysinfoNodeMetricsProvider{nodeID: localNodeID}

	poller := newThresholdPoller(inst.db, inst.db, serverProv, nodeProv, inst, eventbus.Get())

	throttle := time.NewTicker(thresholdPollInterval)
	defer throttle.Stop()

	for {
		select {
		case <-inst.ctx.Done():
			return
		case <-throttle.C:
			poller.runOnce()
		}
	}
}
