package actions

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/alerts"
	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

var errRuleConditionMissing = errors.New("rule has no condition set")

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
	cpuValid      bool
	memoryPercent float64
	memoryValid   bool
	diskPercent   float64
	diskValid     bool
	online        bool
}

// nodeMetricsSnapshot holds a single node's current metrics.
type nodeMetricsSnapshot struct {
	nodeID        string
	cpuPercent    float64
	memoryPercent float64
	diskPercent   float64
}

// serverMetricsProvider abstracts listing running server metrics.
type serverMetricsProvider interface {
	ListServerMetrics() []serverMetricsSnapshot
}

// nodeMetricsProvider abstracts collecting node-level resource metrics.
type nodeMetricsProvider interface {
	ListNodeMetrics() []nodeMetricsSnapshot
}

// registryServerMetricsProvider implements serverMetricsProvider by pulling
// per-process metrics from every registered NodeClient. Replaces the older
// supervisor-only provider so remote-node servers participate in threshold
// evaluation.
type registryServerMetricsProvider struct {
	ctx      context.Context
	registry *noderegistry.Registry
}

func (r *registryServerMetricsProvider) ListServerMetrics() []serverMetricsSnapshot {
	if r.registry == nil {
		return nil
	}
	baseCtx := r.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	clients := r.registry.List()
	var out []serverMetricsSnapshot
	for _, client := range clients {
		ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
		snap, errSnap := client.GetNodeSnapshot(ctx)
		cancel()
		if errSnap != nil {
			log.Debug().Err(errSnap).Str("node_id", client.ID()).
				Msg("threshold-poller: snapshot failed; skipping server metrics for this node")
			continue
		}
		if snap == nil {
			log.Debug().Str("node_id", client.ID()).
				Msg("threshold-poller: nil snapshot; skipping server metrics for this node")
			continue
		}
		for _, ps := range snap.Processes {
			processStatusCanHaveMetrics := ps.Status == xylona.Status_ONLINE.String() ||
				ps.Status == xylona.Status_INSTALLING.String() ||
				ps.Status == xylona.Status_UPDATING.String()
			metricsValid := processStatusCanHaveMetrics && ps.MetricsValid
			out = append(out, serverMetricsSnapshot{
				serverID:      ps.ID,
				nodeID:        client.ID(),
				cpuPercent:    ps.CPUPercent,
				cpuValid:      metricsValid && ps.CPUValid,
				memoryPercent: float64(ps.MemoryPercent),
				memoryValid:   metricsValid,
				diskPercent:   ps.DiskPercent,
				diskValid:     metricsValid && ps.DiskValid && ps.DiskTotalBytes > 0,
				online:        ps.Status == "ONLINE",
			})
		}
	}
	return out
}

// registryNodeMetricsProvider implements nodeMetricsProvider by pulling host
// metrics from every registered NodeClient. Failed nodes are logged and
// skipped independently so one unavailable node does not suppress alerts for
// the rest of the registry.
type registryNodeMetricsProvider struct {
	ctx      context.Context
	registry *noderegistry.Registry
}

func (r *registryNodeMetricsProvider) ListNodeMetrics() []nodeMetricsSnapshot {
	if r.registry == nil {
		return nil
	}
	baseCtx := r.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}

	clients := r.registry.List()
	out := make([]nodeMetricsSnapshot, 0, len(clients))
	for _, client := range clients {
		ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
		snap, errSnap := client.GetNodeSnapshot(ctx)
		cancel()
		if errSnap != nil {
			log.Debug().Err(errSnap).Str("node_id", client.ID()).
				Msg("threshold-poller: snapshot failed; skipping node metrics for this node")
			continue
		}
		if snap == nil {
			log.Debug().Str("node_id", client.ID()).
				Msg("threshold-poller: nil snapshot; skipping node metrics for this node")
			continue
		}
		out = append(out, nodeMetricsSnapshot{
			nodeID:        client.ID(),
			cpuPercent:    snap.CPUPercent,
			memoryPercent: snap.MemoryPercent,
			diskPercent:   snap.DiskPercent,
		})
	}
	return out
}

// cachedRules holds one event-type's rules and when they were fetched.
type cachedRules struct {
	rules     []*models.AlertRule
	fetchedAt time.Time
}

// cachedAlertState holds an in-memory copy of an alert state row, avoiding
// repeated DB round-trips for unchanged states.
type cachedAlertState struct {
	id              string
	ruleID          string
	ruleFingerprint string
	triggered       bool
	pendingSince    time.Time
	cooldownUntil   time.Time
	lastPublishedAt time.Time
}

// cachedCondition holds a pre-parsed rule condition so JSON does not need to
// be deserialized on every tick.
type cachedCondition struct {
	condition alerts.ThresholdCondition
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
	queryMetrics GameServerQueryTelemetryProvider
	bus          *eventbus.EventBus
	now          func() time.Time

	// Rule cache — refreshed every rulesCacheTTL.
	ruleCache     map[string]cachedRules // keyed by eventType
	rulesCachedAt time.Time

	// State cache — keyed by "ruleID|entityType|entityID|entityNodeID".
	// Loaded from DB on cache miss, updated on state transitions. Runtime timing
	// survives routine refreshes only while the rule definition is unchanged.
	stateCache map[string]*cachedAlertState

	// Condition cache — keyed by rule ID. Invalidated when rules are refreshed.
	conditionCache         map[string]*cachedCondition
	unsupportedNoDataRules map[string]struct{}
}

func newThresholdPoller(
	ruleStore thresholdRuleStore,
	stateStore thresholdStateStore,
	serverProv serverMetricsProvider,
	nodeProv nodeMetricsProvider,
	playerCounts PlayerCountProvider,
	bus *eventbus.EventBus,
) *thresholdPoller {
	poller := &thresholdPoller{
		ruleStore:              ruleStore,
		stateStore:             stateStore,
		serverProv:             serverProv,
		nodeProv:               nodeProv,
		playerCounts:           playerCounts,
		bus:                    bus,
		now:                    time.Now,
		ruleCache:              make(map[string]cachedRules),
		stateCache:             make(map[string]*cachedAlertState),
		conditionCache:         make(map[string]*cachedCondition),
		unsupportedNoDataRules: make(map[string]struct{}),
	}
	queryMetrics, ok := playerCounts.(GameServerQueryTelemetryProvider)
	if ok {
		poller.queryMetrics = queryMetrics
	}
	return poller
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
		return nil, fmt.Errorf("actions: load alert state: %w", errState)
	}
	entry := &cachedAlertState{
		id:        state.ID,
		ruleID:    ruleID,
		triggered: state.Triggered != 0,
	}
	if entry.triggered {
		entry.lastPublishedAt = p.now()
	}
	p.stateCache[k] = entry
	return entry, nil
}

// getConditionCached returns the cached parsed condition for the given rule, or
// parses and caches it on first access.
func (p *thresholdPoller) getConditionCached(rule *models.AlertRule) (alerts.ThresholdCondition, error) {
	cached, ok := p.conditionCache[rule.ID]
	if ok {
		return cached.condition, cached.err
	}
	condition, errParse := parseThresholdCondition(rule.Condition)
	cached = &cachedCondition{
		condition: condition,
		err:       errParse,
	}
	p.conditionCache[rule.ID] = cached
	_, noDataAlreadyLogged := p.unsupportedNoDataRules[rule.ID]
	if errParse == nil && condition.NoDataSeconds > 0 && !noDataAlreadyLogged {
		log.Warn().Str("rule_id", rule.ID).Int64("no_data_seconds", condition.NoDataSeconds).
			Msg("Threshold poller: no-data alert delivery is not supported; invalid samples will not change triggered state")
		p.unsupportedNoDataRules[rule.ID] = struct{}{}
	}
	return condition, errParse
}

func alertRuleRuntimeFingerprint(rule *models.AlertRule) string {
	condition, conditionSet := rule.Condition.Get()
	serverID, serverIDSet := rule.ServerID.Get()
	serverNodeID, serverNodeIDSet := rule.ServerNodeID.Get()
	nodeID, nodeIDSet := rule.NodeID.Get()
	return fmt.Sprintf(
		"%q|%t:%q|%t:%q|%t:%q|%t:%q",
		rule.EventType,
		conditionSet,
		condition,
		serverIDSet,
		serverID,
		serverNodeIDSet,
		serverNodeID,
		nodeIDSet,
		nodeID,
	)
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
	cached, ok := p.ruleCache[eventType]
	if ok {
		if p.now().Sub(cached.fetchedAt) < rulesCacheTTL {
			return cached.rules, nil
		}
	}
	rules, errGet := p.ruleStore.GetEnabledAlertRulesByEventType(eventType)
	if errGet != nil {
		return nil, fmt.Errorf("failed to fetch rules for %s: %w", eventType, errGet)
	}
	p.ruleCache[eventType] = cachedRules{rules: rules, fetchedAt: p.now()}
	return rules, nil
}

// refreshRuleCache forces a re-fetch of all event types and resets the cache
// timestamp. It invalidates parsed conditions while preserving runtime timing
// only for enabled rules whose evaluated definition is unchanged.
func (p *thresholdPoller) refreshRuleCache() {
	enabledRuleFingerprints := make(map[string]string)
	refreshComplete := true
	for _, et := range thresholdEventTypes {
		rules, errGet := p.ruleStore.GetEnabledAlertRulesByEventType(et.eventType)
		if errGet != nil {
			log.Error().Err(errGet).Str("event_type", et.eventType).Msg("Threshold poller: failed to refresh rule cache")
			refreshComplete = false
			continue
		}
		p.ruleCache[et.eventType] = cachedRules{rules: rules, fetchedAt: p.now()}
		for _, rule := range rules {
			if rule.Enabled == 0 {
				continue
			}
			enabledRuleFingerprints[rule.ID] = alertRuleRuntimeFingerprint(rule)
		}
	}
	p.rulesCachedAt = p.now()

	p.conditionCache = make(map[string]*cachedCondition)
	if !refreshComplete {
		return
	}
	for key, state := range p.stateCache {
		fingerprint, enabled := enabledRuleFingerprints[state.ruleID]
		if !enabled || (state.ruleFingerprint != "" && state.ruleFingerprint != fingerprint) {
			delete(p.stateCache, key)
		}
	}
}

// runOnce executes a single evaluation cycle: fetches metrics, evaluates rules,
// manages state, and publishes events on transitions.
func (p *thresholdPoller) runOnce() {
	// Refresh rule cache if stale.
	if p.now().Sub(p.rulesCachedAt) >= rulesCacheTTL {
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
		getValue  func(snap serverMetricsSnapshot) (float64, bool)
	}{
		{
			"ALERT_EVENT_TYPE_CPU_THRESHOLD",
			eventbus.TopicGameServerCPUThreshold,
			func(snap serverMetricsSnapshot) (float64, bool) {
				return snap.cpuPercent, snap.cpuValid
			},
		},
		{
			"ALERT_EVENT_TYPE_MEMORY_THRESHOLD",
			eventbus.TopicGameServerMemoryThreshold,
			func(snap serverMetricsSnapshot) (float64, bool) {
				return snap.memoryPercent, snap.memoryValid
			},
		},
		{
			"ALERT_EVENT_TYPE_DISK_THRESHOLD",
			eventbus.TopicGameServerDiskThreshold,
			func(snap serverMetricsSnapshot) (float64, bool) {
				return snap.diskPercent, snap.diskValid
			},
		},
		{
			"ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD",
			eventbus.TopicGameServerPlayerThreshold,
			func(snap serverMetricsSnapshot) (float64, bool) {
				if !snap.online {
					return 0, false
				}
				return p.playerCountMetric(snap.serverID)
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
				currentValue, valid := et.getValue(snap)
				condition, errParse := p.getConditionCached(rule)
				if errParse != nil {
					log.Warn().Err(errParse).Str("rule_id", rule.ID).Msg("Threshold poller: failed to parse condition")
					continue
				}
				p.evaluateThreshold(rule, "server", snap.serverID, snap.nodeID, currentValue, valid, condition, func(direction eventbus.ThresholdDirection) {
					p.bus.Publish(et.topic, eventbus.ThresholdEvent{
						ServerID:     snap.serverID,
						ServerNodeID: snap.nodeID,
						CurrentValue: currentValue,
						Threshold:    condition.Value,
						Direction:    direction,
					})
				})
			}
		}
	}
}

func (p *thresholdPoller) playerCountMetric(serverID string) (float64, bool) {
	if p.queryMetrics != nil {
		telemetry := p.queryMetrics.GetGameServerQueryTelemetry(serverID)
		valid := telemetry.Status == GameServerQueryTelemetryStatusSuccess && telemetry.PlayerCountValid
		return float64(telemetry.PlayerCount), valid
	}
	if p.playerCounts == nil {
		return 0, false
	}
	return float64(p.playerCounts.GetPlayerCount(serverID)), true
}

func (p *thresholdPoller) evaluateThreshold(
	rule *models.AlertRule,
	entityType string,
	entityID string,
	entityNodeID string,
	currentValue float64,
	valid bool,
	condition alerts.ThresholdCondition,
	publish func(direction eventbus.ThresholdDirection),
) {
	key := stateKey(rule.ID, entityType, entityID, entityNodeID)
	if !valid || math.IsNaN(currentValue) || math.IsInf(currentValue, 0) {
		cached, ok := p.stateCache[key]
		if ok {
			cached.pendingSince = time.Time{}
		}
		return
	}

	breached, errEval := alerts.EvaluateThresholdOp(condition.Operator, condition.Value, currentValue)
	if errEval != nil {
		log.Warn().Err(errEval).Str("rule_id", rule.ID).Msg("Threshold poller: failed to evaluate threshold")
		return
	}

	cached, errState := p.getOrCreateStateCached(rule.ID, entityType, entityID, entityNodeID)
	if errState != nil {
		log.Error().Err(errState).Str("rule_id", rule.ID).Str("entity_type", entityType).
			Str("entity_id", entityID).Msg("Threshold poller: failed to get/create alert state")
		return
	}
	now := p.now()
	fingerprint := alertRuleRuntimeFingerprint(rule)
	if cached.ruleFingerprint != fingerprint {
		cached.ruleFingerprint = fingerprint
		cached.pendingSince = time.Time{}
		cached.cooldownUntil = time.Time{}
		cached.lastPublishedAt = time.Time{}
		if cached.triggered {
			cached.lastPublishedAt = now
		}
	}

	if cached.triggered {
		recoveryThreshold := condition.Value
		if condition.RecoveryValue != nil {
			recoveryThreshold = *condition.RecoveryValue
		}
		stillTriggered, errRecovery := alerts.EvaluateThresholdOp(condition.Operator, recoveryThreshold, currentValue)
		if errRecovery != nil {
			log.Warn().Err(errRecovery).Str("rule_id", rule.ID).Msg("Threshold poller: failed to evaluate recovery threshold")
			return
		}
		if !stillTriggered {
			errUpdate := p.stateStore.UpdateAlertStateTriggered(cached.id, false)
			if errUpdate != nil {
				log.Error().Err(errUpdate).Str("state_id", cached.id).Msg("Threshold poller: failed to update state to resolved")
				return
			}
			cached.triggered = false
			cached.pendingSince = time.Time{}
			cached.lastPublishedAt = time.Time{}
			cached.cooldownUntil = now.Add(time.Duration(condition.CooldownSeconds) * time.Second)
			publish(eventbus.ThresholdResolved)
			return
		}
		if condition.RepeatSeconds <= 0 {
			return
		}
		repeatAfter := time.Duration(condition.RepeatSeconds) * time.Second
		if now.Sub(cached.lastPublishedAt) < repeatAfter {
			return
		}
		cached.lastPublishedAt = now
		publish(eventbus.ThresholdEntered)
		return
	}

	if now.Before(cached.cooldownUntil) {
		cached.pendingSince = time.Time{}
		return
	}
	if !breached {
		cached.pendingSince = time.Time{}
		return
	}
	if cached.pendingSince.IsZero() {
		cached.pendingSince = now
	}
	forDuration := time.Duration(condition.ForSeconds) * time.Second
	if now.Sub(cached.pendingSince) < forDuration {
		return
	}

	errUpdate := p.stateStore.UpdateAlertStateTriggered(cached.id, true)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("state_id", cached.id).Msg("Threshold poller: failed to update state to triggered")
		return
	}
	cached.triggered = true
	cached.pendingSince = time.Time{}
	cached.lastPublishedAt = now
	publish(eventbus.ThresholdEntered)
}

// evaluateNodeRules evaluates all node-level threshold event types.
func (p *thresholdPoller) evaluateNodeRules() {
	if p.nodeProv == nil {
		return
	}
	snapshots := p.nodeProv.ListNodeMetrics()

	nodeEventTypes := []struct {
		eventType string
		topic     string
		getValue  func(nodeMetricsSnapshot) (float64, bool)
	}{
		{
			"ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD",
			eventbus.TopicNodeCPUThreshold,
			func(snap nodeMetricsSnapshot) (float64, bool) {
				return snap.cpuPercent, true
			},
		},
		{
			"ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD",
			eventbus.TopicNodeMemoryThreshold,
			func(snap nodeMetricsSnapshot) (float64, bool) {
				return snap.memoryPercent, true
			},
		},
		{
			"ALERT_EVENT_TYPE_NODE_DISK_THRESHOLD",
			eventbus.TopicNodeDiskThreshold,
			func(snap nodeMetricsSnapshot) (float64, bool) {
				return snap.diskPercent, true
			},
		},
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

			ruleNodeID, nodeIDSet := rule.NodeID.Get()
			for _, snap := range snapshots {
				if nodeIDSet && ruleNodeID != snap.nodeID {
					continue
				}

				currentValue, valid := et.getValue(snap)
				condition, errParse := p.getConditionCached(rule)
				if errParse != nil {
					log.Warn().Err(errParse).Str("rule_id", rule.ID).Msg("Threshold poller: failed to parse node rule condition")
					continue
				}
				p.evaluateThreshold(rule, "node", snap.nodeID, "", currentValue, valid, condition, func(direction eventbus.ThresholdDirection) {
					p.bus.Publish(et.topic, eventbus.NodeThresholdEvent{
						NodeID:       snap.nodeID,
						CurrentValue: currentValue,
						Threshold:    condition.Value,
						Direction:    direction,
					})
				})
			}
		}
	}
}

func parseThresholdCondition(condition interface{ Get() (string, bool) }) (alerts.ThresholdCondition, error) {
	condStr, isSet := condition.Get()
	if !isSet || condStr == "" {
		return alerts.ThresholdCondition{}, errRuleConditionMissing
	}
	parsed, errParse := alerts.ParseThresholdConditionJSON(condStr)
	if errParse != nil {
		return alerts.ThresholdCondition{}, fmt.Errorf("actions: parse alert rule condition: %w", errParse)
	}
	return parsed, nil
}

// backgroundJobThresholdPoller is the long-running background goroutine that
// ticks every thresholdPollInterval and evaluates threshold rules. Uses the
// registry-based providers so both embedded and remote-node servers have
// their metrics evaluated against the same rule set.
func (inst *Instance) backgroundJobThresholdPoller() {
	serverProv := &registryServerMetricsProvider{ctx: inst.ctx, registry: inst.nodeRegistry}
	nodeProv := &registryNodeMetricsProvider{ctx: inst.ctx, registry: inst.nodeRegistry}

	poller := newThresholdPoller(inst.db, inst.db, serverProv, nodeProv, inst, eventbus.Get())

	throttle := time.NewTicker(thresholdPollInterval)
	defer throttle.Stop()

	for {
		select {
		case <-inst.ctx.Done():
			return
		case <-throttle.C:
			runBackgroundTask("backgroundJobThresholdPoller", "tick", nil, func() {
				poller.runOnce()
			})
		}
	}
}
