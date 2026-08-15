package actions

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	defaultMetricsSnapshotInterval = 60 * time.Second
	defaultMetricsCleanupInterval  = 10 * time.Minute
	defaultMetricsHistoryRetention = 90 * 24 * time.Hour
	defaultMetricsRollupAfter      = 24 * time.Hour
	metricsSnapshotFetchTimeout    = 10 * time.Second
	minMetricsSnapshotInterval     = 15 * time.Second
	maxMetricsSnapshotInterval     = 15 * time.Minute
	minMetricsCleanupInterval      = time.Minute
	maxMetricsCleanupInterval      = 24 * time.Hour
	minMetricsHistoryRetention     = 24 * time.Hour
	maxMetricsHistoryRetention     = 365 * 24 * time.Hour
	minMetricsRollupAfter          = time.Hour
)

// MetricsRecorderConfig controls the cost and retention of persisted metrics.
// The node collector has separate node-local collection settings.
type MetricsRecorderConfig struct {
	SnapshotInterval time.Duration
	CleanupInterval  time.Duration
	HistoryRetention time.Duration
	RollupAfter      time.Duration
}

// DefaultMetricsRecorderConfig returns the production history defaults.
func DefaultMetricsRecorderConfig() MetricsRecorderConfig {
	return MetricsRecorderConfig{
		SnapshotInterval: defaultMetricsSnapshotInterval,
		CleanupInterval:  defaultMetricsCleanupInterval,
		HistoryRetention: defaultMetricsHistoryRetention,
		RollupAfter:      defaultMetricsRollupAfter,
	}
}

// PlayerCountProvider returns the current player count for a game server.
type PlayerCountProvider interface {
	GetPlayerCount(gameServerID string) int
}

// MetricsRecorder periodically records node and game server metrics to the
// database. It iterates every registered NodeClient each tick so remote-node
// servers are recorded alongside the embedded ones.
type MetricsRecorder struct {
	ctx            context.Context
	db             *db.Connection
	registry       *noderegistry.Registry
	playerCounts   PlayerCountProvider
	queryTelemetry GameServerQueryTelemetryProvider
	config         MetricsRecorderConfig
}

// NewMetricsRecorderWithConfig creates and starts a metrics recorder with
// validated collection and retention settings.
func NewMetricsRecorderWithConfig(ctx context.Context, dbInst *db.Connection, registry *noderegistry.Registry, playerCounts PlayerCountProvider, config MetricsRecorderConfig) *MetricsRecorder {
	config = normalizeMetricsRecorderConfig(config)
	queryTelemetry, _ := playerCounts.(GameServerQueryTelemetryProvider)
	mr := &MetricsRecorder{
		ctx:            ctx,
		db:             dbInst,
		registry:       registry,
		playerCounts:   playerCounts,
		queryTelemetry: queryTelemetry,
		config:         config,
	}
	go mr.runSnapshotLoop()
	go mr.runCleanupLoop()
	return mr
}

func normalizeMetricsRecorderConfig(config MetricsRecorderConfig) MetricsRecorderConfig {
	defaults := DefaultMetricsRecorderConfig()
	if config.SnapshotInterval <= 0 {
		config.SnapshotInterval = defaults.SnapshotInterval
	}
	config.SnapshotInterval = min(max(config.SnapshotInterval, minMetricsSnapshotInterval), maxMetricsSnapshotInterval)
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = defaults.CleanupInterval
	}
	config.CleanupInterval = min(max(config.CleanupInterval, minMetricsCleanupInterval), maxMetricsCleanupInterval)
	if config.HistoryRetention <= 0 {
		config.HistoryRetention = defaults.HistoryRetention
	}
	config.HistoryRetention = min(max(config.HistoryRetention, minMetricsHistoryRetention), maxMetricsHistoryRetention)
	if config.RollupAfter <= 0 {
		config.RollupAfter = defaults.RollupAfter
	}
	config.RollupAfter = max(config.RollupAfter, minMetricsRollupAfter)
	if config.RollupAfter >= config.HistoryRetention {
		config.RollupAfter = defaults.RollupAfter
		if config.RollupAfter >= config.HistoryRetention {
			config.RollupAfter = config.HistoryRetention / 2
		}
	}
	return config
}

func (mr *MetricsRecorder) runSnapshotLoop() {
	mr.recordAllNodeMetrics()
	ticker := time.NewTicker(mr.config.SnapshotInterval)
	defer ticker.Stop()

	for {
		select {
		case <-mr.ctx.Done():
			return
		case <-ticker.C:
			runBackgroundTask("MetricsRecorder.runSnapshotLoop", "tick", nil, func() {
				mr.recordAllNodeMetrics()
			})
		}
	}
}

func (mr *MetricsRecorder) runCleanupLoop() {
	ticker := time.NewTicker(mr.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-mr.ctx.Done():
			return
		case <-ticker.C:
			runBackgroundTask("MetricsRecorder.runCleanupLoop", "tick", nil, func() {
				mr.cleanupAndRollup()
			})
		}
	}
}

// recordAllNodeMetrics iterates every registered NodeClient and records a
// point-in-time metrics row for each node + each process the node is
// tracking. For embedded and remote nodes the flow is identical.
func (mr *MetricsRecorder) recordAllNodeMetrics() {
	if mr.registry == nil || mr.db == nil {
		return
	}
	clients := mr.registry.List()
	now := time.Now().UTC()
	allServers, errServers := mr.db.GetAllGameServers()
	if errServers != nil {
		log.Error().Err(errServers).Msg("MetricsRecorder: failed to enumerate game servers")
		return
	}
	serversByNode := make(map[string][]*models.GameServer)
	for _, gameServer := range allServers {
		serversByNode[gameServer.NodeID] = append(serversByNode[gameServer.NodeID], gameServer)
	}

	userCount, errUsers := mr.db.CountUsers()
	if errUsers != nil {
		log.Error().Err(errUsers).Msg("Failed to count users for metrics history")
	}

	visitedNodes := make(map[string]struct{}, len(clients))
	for _, client := range clients {
		nodeID := client.ID()
		visitedNodes[nodeID] = struct{}{}
		ctx, cancel := context.WithTimeout(mr.ctx, metricsSnapshotFetchTimeout)
		snap, errSnap := client.GetNodeSnapshot(ctx)
		cancel()
		if errSnap != nil || snap == nil {
			log.Warn().Err(errSnap).Str("node_id", nodeID).
				Msg("MetricsRecorder: node snapshot failed; recording an availability gap")
			for _, gameServer := range serversByNode[nodeID] {
				mr.insertGameServerMetricsRow(mr.unavailableGameServerMetricsRow(gameServer, nodeID, now, "node_unavailable"))
			}
			continue
		}

		// Node-level metrics row.
		runningCount := 0
		for _, ps := range snap.Processes {
			if ps.Status == xylona.Status_ONLINE.String() ||
				ps.Status == xylona.Status_INSTALLING.String() ||
				ps.Status == xylona.Status_UPDATING.String() {
				runningCount++
			}
		}

		gameServerCount := len(serversByNode[nodeID])

		row := &db.NodeMetricsRow{
			ID:                     uuid.New().String(),
			NodeID:                 nodeID,
			CPUPercent:             snap.CPUPercent,
			MemoryPercent:          snap.MemoryPercent,
			MemoryUsedBytes:        helpers.ClampInt64FromUint64(snap.MemoryUsed),
			MemoryTotalBytes:       helpers.ClampInt64FromUint64(snap.TotalMemory),
			DiskPercent:            snap.DiskPercent,
			DiskUsedBytes:          helpers.ClampInt64FromUint64(snap.DiskUsed),
			DiskTotalBytes:         helpers.ClampInt64FromUint64(snap.DiskTotal),
			GameServerCount:        gameServerCount,
			RunningGameServerCount: runningCount,
			UserCount:              userCount,
			RecordedAt:             now,
		}

		errInsert := mr.db.InsertNodeMetricsHistory(row)
		if errInsert != nil {
			log.Error().Err(errInsert).Str("node_id", nodeID).
				Msg("Failed to insert node metrics history")
		}

		processesByID := make(map[string]*node.ProcessSnapshot, len(snap.Processes))
		for processIndex := range snap.Processes {
			processSnapshot := &snap.Processes[processIndex]
			processesByID[processSnapshot.ID] = processSnapshot
		}
		for _, gameServer := range serversByNode[nodeID] {
			processSnapshot, found := processesByID[gameServer.ID]
			if !found {
				mr.insertGameServerMetricsRow(mr.unavailableGameServerMetricsRow(gameServer, nodeID, now, "server_offline"))
				continue
			}
			row := mr.gameServerMetricsRow(gameServer, nodeID, snap, processSnapshot, now)
			mr.insertGameServerMetricsRow(row)
		}
	}

	for nodeID, gameServers := range serversByNode {
		_, visited := visitedNodes[nodeID]
		if visited {
			continue
		}
		for _, gameServer := range gameServers {
			mr.insertGameServerMetricsRow(mr.unavailableGameServerMetricsRow(gameServer, nodeID, now, "node_unavailable"))
		}
	}
}

func (mr *MetricsRecorder) insertGameServerMetricsRow(row *db.GameServerMetricsRow) {
	errInsert := mr.db.InsertGameServerMetricsHistory(row)
	if errInsert != nil {
		log.Error().Err(errInsert).Str("game_server_id", row.GameServerID).
			Msg("Failed to insert game server metrics history")
	}
}

func (mr *MetricsRecorder) unavailableGameServerMetricsRow(gameServer *models.GameServer, nodeID string, recordedAt time.Time, collectionStatus string) *db.GameServerMetricsRow {
	row := &db.GameServerMetricsRow{
		ID:                            uuid.New().String(),
		GameServerID:                  gameServer.ID,
		NodeID:                        sql.NullString{String: nodeID, Valid: nodeID != ""},
		RecordedAt:                    recordedAt,
		GranularitySeconds:            int64(mr.config.SnapshotInterval / time.Second),
		SampleCount:                   1,
		AvailableSampleCount:          0,
		AvailabilityRatio:             0,
		CollectionStatus:              collectionStatus,
		CPUValid:                      false,
		CPUValidSet:                   true,
		IOValidSampleCountSet:         true,
		ConnectionValidSampleCountSet: true,
	}
	if collectionStatus == "server_offline" {
		row.ProcessStatus = sql.NullString{String: xylona.Status_OFFLINE.String(), Valid: true}
	}
	mr.applyConfiguredMemory(row, gameServer)
	return row
}

func (mr *MetricsRecorder) gameServerMetricsRow(gameServer *models.GameServer, nodeID string, snapshot *node.NodeSnapshot, process *node.ProcessSnapshot, recordedAt time.Time) *db.GameServerMetricsRow {
	collectionStatus := "available"
	availableSampleCount := int64(1)
	availabilityRatio := 1.0
	if process.Status == xylona.Status_OFFLINE.String() {
		collectionStatus = "server_offline"
		availableSampleCount = 0
		availabilityRatio = 0
	} else if !process.MetricsValid {
		collectionStatus = "warming_up"
		availableSampleCount = 0
		availabilityRatio = 0
	}
	processCollectedAt := snapshot.Collected.UTC()
	if processCollectedAt.IsZero() {
		processCollectedAt = recordedAt
	}
	row := &db.GameServerMetricsRow{
		ID:                            uuid.New().String(),
		GameServerID:                  gameServer.ID,
		NodeID:                        sql.NullString{String: nodeID, Valid: nodeID != ""},
		CPUPercent:                    process.CPUPercent,
		MemoryBytes:                   helpers.ClampInt64FromUint64(process.MemoryRSS),
		MemoryPercent:                 float64(process.MemoryPercent),
		DiskUsageBytes:                helpers.ClampInt64FromUint64(process.DiskUsageBytes),
		IOReadRate:                    process.IOReadRate,
		IOWriteRate:                   process.IOWriteRate,
		ConnectionCount:               int64(process.ConnectionCount),
		RecordedAt:                    recordedAt,
		GranularitySeconds:            int64(mr.config.SnapshotInterval / time.Second),
		SampleCount:                   1,
		AvailableSampleCount:          availableSampleCount,
		AvailabilityRatio:             availabilityRatio,
		CollectionStatus:              collectionStatus,
		ProcessCollectedAt:            &processCollectedAt,
		CPUValid:                      process.CPUValid,
		CPUValidSet:                   true,
		IOValidSampleCountSet:         true,
		ConnectionValidSampleCountSet: true,
		VolumeValid:                   process.DiskValid,
		ProcessStatus:                 sql.NullString{String: process.Status, Valid: process.Status != ""},
		ExecutionID:                   sql.NullString{String: process.ExecutionID, Valid: process.ExecutionID != ""},
	}
	if process.CPUValid {
		row.CPUPercentMin = sql.NullFloat64{Float64: process.CPUPercent, Valid: true}
		row.CPUPercentMax = sql.NullFloat64{Float64: process.CPUPercent, Valid: true}
	}
	if process.CPUCores > 0 {
		row.NodeCPUCores = sql.NullInt64{Int64: int64(process.CPUCores), Valid: true}
	}
	if process.MetricsValid {
		row.MemoryBytesMin = sql.NullInt64{Int64: row.MemoryBytes, Valid: true}
		row.MemoryBytesMax = sql.NullInt64{Int64: row.MemoryBytes, Valid: true}
		row.MemoryPercentMin = sql.NullFloat64{Float64: row.MemoryPercent, Valid: true}
		row.MemoryPercentMax = sql.NullFloat64{Float64: row.MemoryPercent, Valid: true}
	}
	if process.IOValid {
		row.IOValidSampleCount = 1
		row.IOReadRateMin = sql.NullFloat64{Float64: row.IOReadRate, Valid: true}
		row.IOReadRateMax = sql.NullFloat64{Float64: row.IOReadRate, Valid: true}
		row.IOWriteRateMin = sql.NullFloat64{Float64: row.IOWriteRate, Valid: true}
		row.IOWriteRateMax = sql.NullFloat64{Float64: row.IOWriteRate, Valid: true}
	}
	if process.ConnectionCountValid {
		row.ConnectionValidSampleCount = 1
		row.ConnectionCountMin = sql.NullInt64{Int64: row.ConnectionCount, Valid: true}
		row.ConnectionCountMax = sql.NullInt64{Int64: row.ConnectionCount, Valid: true}
	}
	if snapshot.TotalMemory > 0 {
		row.NodeMemoryUsedBytes = sql.NullInt64{Int64: helpers.ClampInt64FromUint64(snapshot.MemoryUsed), Valid: true}
		row.NodeMemoryTotalBytes = sql.NullInt64{Int64: helpers.ClampInt64FromUint64(snapshot.TotalMemory), Valid: true}
	}
	if !process.DiskMeasuredAt.IsZero() {
		diskMeasuredAt := process.DiskMeasuredAt.UTC()
		row.DiskMeasuredAt = &diskMeasuredAt
		row.DiskUsageBytesMin = sql.NullInt64{Int64: row.DiskUsageBytes, Valid: true}
		row.DiskUsageBytesMax = sql.NullInt64{Int64: row.DiskUsageBytes, Valid: true}
		row.VolumeTotalBytes = sql.NullInt64{Int64: helpers.ClampInt64FromUint64(process.DiskTotalBytes), Valid: true}
		row.VolumeFreeBytes = sql.NullInt64{Int64: helpers.ClampInt64FromUint64(process.DiskFreeBytes), Valid: true}
		row.VolumePercent = sql.NullFloat64{Float64: process.DiskPercent, Valid: true}
	}
	if process.UnixStartedAt > 0 && recordedAt.Unix() >= process.UnixStartedAt {
		row.ServerUptimeSeconds = sql.NullInt64{Int64: recordedAt.Unix() - process.UnixStartedAt, Valid: true}
	}
	mr.applyConfiguredMemory(row, gameServer)
	if collectionStatus == "available" {
		mr.applyQueryTelemetry(row, gameServer.ID)
	}
	return row
}

func (mr *MetricsRecorder) applyConfiguredMemory(row *db.GameServerMetricsRow, gameServer *models.GameServer) {
	if gameServer.MaxMemoryMB <= 0 {
		return
	}
	const bytesPerMiB = int64(1024 * 1024)
	row.ConfiguredMemoryBytes = sql.NullInt64{Int64: gameServer.MaxMemoryMB * bytesPerMiB, Valid: true}
}

func (mr *MetricsRecorder) applyQueryTelemetry(row *db.GameServerMetricsRow, gameServerID string) {
	if mr.queryTelemetry == nil {
		return
	}
	telemetry := mr.queryTelemetry.GetGameServerQueryTelemetry(gameServerID)
	switch telemetry.Status {
	case GameServerQueryTelemetryStatusUnsupported:
		row.QuerySupported = sql.NullBool{Bool: false, Valid: true}
	case GameServerQueryTelemetryStatusSuccess:
		row.QuerySupported = sql.NullBool{Bool: true, Valid: true}
		row.QuerySuccess = sql.NullBool{Bool: true, Valid: true}
	case GameServerQueryTelemetryStatusFailure:
		row.QuerySupported = sql.NullBool{Bool: true, Valid: true}
		row.QuerySuccess = sql.NullBool{Bool: false, Valid: true}
	case GameServerQueryTelemetryStatusUnavailable:
		row.QuerySupported = sql.NullBool{Bool: true, Valid: true}
		row.QuerySuccess = sql.NullBool{Bool: false, Valid: true}
	case GameServerQueryTelemetryStatusNotYetQueried:
		return
	}
	if !telemetry.CheckedAt.IsZero() {
		queryCheckedAt := telemetry.CheckedAt.UTC()
		row.QueryCheckedAt = &queryCheckedAt
	}
	if telemetry.Status != GameServerQueryTelemetryStatusSuccess {
		return
	}
	if telemetry.DurationValid {
		durationMS := float64(telemetry.Duration) / float64(time.Millisecond)
		row.QueryDurationMS = sql.NullFloat64{Float64: durationMS, Valid: true}
		row.QueryDurationMSMin = sql.NullFloat64{Float64: durationMS, Valid: true}
		row.QueryDurationMSMax = sql.NullFloat64{Float64: durationMS, Valid: true}
	}
	if telemetry.PlayerCountValid {
		playerCount := int64(telemetry.PlayerCount)
		row.PlayerCount = playerCount
		row.PlayerCountMin = sql.NullInt64{Int64: playerCount, Valid: true}
		row.PlayerCountMax = sql.NullInt64{Int64: playerCount, Valid: true}
	}
	if telemetry.PlayerCapacityValid {
		row.PlayerCapacity = sql.NullInt64{Int64: int64(telemetry.PlayerCapacity), Valid: true}
	}
	if telemetry.PalworldServerFPSValid {
		row.ServerFPS = sql.NullFloat64{Float64: telemetry.PalworldServerFPS, Valid: true}
		row.ServerFPSMin = row.ServerFPS
		row.ServerFPSMax = row.ServerFPS
	}
	if telemetry.PalworldFrameTimeMSValid {
		row.ServerFrameTimeMS = sql.NullFloat64{Float64: telemetry.PalworldFrameTimeMS, Valid: true}
		row.ServerFrameTimeMSMin = row.ServerFrameTimeMS
		row.ServerFrameTimeMSMax = row.ServerFrameTimeMS
	}
	if telemetry.PalworldUptimeSecondsValid {
		row.ServerUptimeSeconds = sql.NullInt64{Int64: helpers.ClampInt64FromUint64(telemetry.PalworldUptimeSeconds), Valid: true}
	}
}

func (mr *MetricsRecorder) cleanupAndRollup() {
	mr.cleanupAndRollupAt(time.Now().UTC())
}

func (mr *MetricsRecorder) cleanupAndRollupAt(now time.Time) {
	config := normalizeMetricsRecorderConfig(mr.config)
	rollupCutoff := now.Add(-config.RollupAfter).Truncate(time.Hour)

	// Only roll up complete hours. Cleanup runs more frequently than hourly, so
	// using an unaligned cutoff would repeatedly replace one hour with partial
	// slices and permanently discard the samples from earlier cleanup passes.
	errRollupNode := mr.db.RollupNodeMetricsToHourly(rollupCutoff)
	if errRollupNode != nil {
		log.Error().Err(errRollupNode).Msg("Failed to rollup node metrics to hourly")
	}

	errRollupGS := mr.db.RollupGameServerMetricsToHourly(rollupCutoff)
	if errRollupGS != nil {
		log.Error().Err(errRollupGS).Msg("Failed to rollup game server metrics to hourly")
	}

	// Remove any fine-grained node rows that could not be rolled up.
	deletedNodeMinute, errDeleteNodeMinute := mr.db.DeleteNodeMinuteMetricsHistoryOlderThan(rollupCutoff)
	if errDeleteNodeMinute != nil {
		log.Error().Err(errDeleteNodeMinute).Msg("Failed to delete old minute-level node metrics history")
	} else if deletedNodeMinute > 0 {
		log.Debug().Int64("deleted", deletedNodeMinute).Msg("Cleaned up old minute-level node metrics history rows")
	}

	deletedNodeHourly, errDeleteNodeHourly := mr.db.DeleteNodeHourlyMetricsHistoryOlderThan(now.Add(-config.HistoryRetention))
	if errDeleteNodeHourly != nil {
		log.Error().Err(errDeleteNodeHourly).Msg("Failed to delete old hourly node metrics history")
	} else if deletedNodeHourly > 0 {
		log.Debug().Int64("deleted", deletedNodeHourly).Msg("Cleaned up old hourly node metrics history rows")
	}

	deletedGS, errDeleteGS := mr.db.DeleteGameServerMetricsHistoryOlderThan(now.Add(-config.HistoryRetention))
	if errDeleteGS != nil {
		log.Error().Err(errDeleteGS).Msg("Failed to delete old game server metrics history")
	} else if deletedGS > 0 {
		log.Debug().Int64("deleted", deletedGS).Msg("Cleaned up old game server metrics history rows")
	}

	deletedLifecycle, errDeleteLifecycle := mr.db.DeleteGameServerLifecycleEventsOlderThan(now.Add(-config.HistoryRetention))
	if errDeleteLifecycle != nil {
		log.Error().Err(errDeleteLifecycle).Msg("Failed to delete old game server lifecycle events")
	} else if deletedLifecycle > 0 {
		log.Debug().Int64("deleted", deletedLifecycle).Msg("Cleaned up old game server lifecycle events")
	}

	deletedOperations, errDeleteOperations := mr.db.DeleteGameServerOperationEventsOlderThan(now.Add(-config.HistoryRetention))
	if errDeleteOperations != nil {
		log.Error().Err(errDeleteOperations).Msg("Failed to delete old game server operation events")
	} else if deletedOperations > 0 {
		log.Debug().Int64("deleted", deletedOperations).Msg("Cleaned up old game server operation events")
	}
}
