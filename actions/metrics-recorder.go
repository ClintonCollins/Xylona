package actions

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/noderegistry"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const (
	metricsSnapshotInterval = 60 * time.Second
	metricsCleanupInterval  = 10 * time.Minute
	minuteRetention         = 7 * 24 * time.Hour  // 7 days
	hourlyRetention         = 90 * 24 * time.Hour // 90 days
	rollupCutoff            = 24 * time.Hour      // rollup data older than 24h
)

// PlayerCountProvider returns the current player count for a game server.
type PlayerCountProvider interface {
	GetPlayerCount(gameServerID string) int
}

// MetricsRecorder periodically records node and game server metrics to the
// database. It iterates every registered NodeClient each tick so remote-node
// servers are recorded alongside the embedded ones.
type MetricsRecorder struct {
	ctx          context.Context
	db           *db.Connection
	registry     *noderegistry.Registry
	playerCounts PlayerCountProvider
}

// NewMetricsRecorder creates and starts a new metrics recorder.
func NewMetricsRecorder(ctx context.Context, dbInst *db.Connection, registry *noderegistry.Registry, playerCounts PlayerCountProvider) *MetricsRecorder {
	mr := &MetricsRecorder{
		ctx:          ctx,
		db:           dbInst,
		registry:     registry,
		playerCounts: playerCounts,
	}
	go mr.runSnapshotLoop()
	go mr.runCleanupLoop()
	return mr
}

func (mr *MetricsRecorder) runSnapshotLoop() {
	ticker := time.NewTicker(metricsSnapshotInterval)
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
	ticker := time.NewTicker(metricsCleanupInterval)
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
	if mr.registry == nil {
		return
	}
	clients := mr.registry.List()
	now := time.Now().UTC()

	userCount, errUsers := mr.db.CountUsers()
	if errUsers != nil {
		log.Error().Err(errUsers).Msg("Failed to count users for metrics history")
	}

	for _, client := range clients {
		ctx, cancel := context.WithTimeout(mr.ctx, 10*time.Second)
		snap, errSnap := client.GetNodeSnapshot(ctx)
		cancel()
		if errSnap != nil {
			log.Warn().Err(errSnap).Str("node_id", client.ID()).
				Msg("MetricsRecorder: node snapshot failed; skipping tick for this node")
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

		gameServerCount := countGameServersForNode(mr.db, client.ID())

		row := &db.NodeMetricsRow{
			ID:                     uuid.New().String(),
			NodeID:                 client.ID(),
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
			log.Error().Err(errInsert).Str("node_id", client.ID()).
				Msg("Failed to insert node metrics history")
		}

		// Per-game-server metrics rows.
		for _, ps := range snap.Processes {
			playerCount := 0
			if mr.playerCounts != nil {
				playerCount = mr.playerCounts.GetPlayerCount(ps.ID)
			}
			gsRow := &db.GameServerMetricsRow{
				ID:              uuid.New().String(),
				GameServerID:    ps.ID,
				CPUPercent:      ps.CPUPercent,
				MemoryBytes:     helpers.ClampInt64FromUint64(ps.MemoryRSS),
				MemoryPercent:   float64(ps.MemoryPercent),
				DiskUsageBytes:  helpers.ClampInt64FromUint64(ps.DiskUsageBytes),
				IOReadRate:      ps.IOReadRate,
				IOWriteRate:     ps.IOWriteRate,
				ConnectionCount: int(ps.ConnectionCount),
				PlayerCount:     playerCount,
				RecordedAt:      now,
			}
			errInsertGS := mr.db.InsertGameServerMetricsHistory(gsRow)
			if errInsertGS != nil {
				log.Error().Err(errInsertGS).Str("game_server_id", ps.ID).
					Msg("Failed to insert game server metrics history")
			}
		}
	}
}

// countGameServersForNode returns how many game servers belong to the given
// node, best-effort. Errors are logged and counted as zero so the metrics
// loop never aborts over a transient DB hiccup.
func countGameServersForNode(dbInst *db.Connection, nodeID string) int {
	all, errAll := dbInst.GetAllGameServers()
	if errAll != nil {
		log.Warn().Err(errAll).Str("node_id", nodeID).
			Msg("MetricsRecorder: failed to enumerate game servers")
		return 0
	}
	count := 0
	for _, gs := range all {
		if gs.NodeID == nodeID {
			count++
		}
	}
	return count
}

func (mr *MetricsRecorder) cleanupAndRollup() {
	now := time.Now().UTC()

	// Rollup minute-level data older than 24h to hourly aggregates.
	errRollupNode := mr.db.RollupNodeMetricsToHourly(now.Add(-rollupCutoff))
	if errRollupNode != nil {
		log.Error().Err(errRollupNode).Msg("Failed to rollup node metrics to hourly")
	}

	errRollupGS := mr.db.RollupGameServerMetricsToHourly(now.Add(-rollupCutoff))
	if errRollupGS != nil {
		log.Error().Err(errRollupGS).Msg("Failed to rollup game server metrics to hourly")
	}

	// Delete minute-level node data older than 7 days after rollup has had a chance to preserve it as hourly history.
	deletedNodeMinute, errDeleteNodeMinute := mr.db.DeleteNodeMinuteMetricsHistoryOlderThan(now.Add(-minuteRetention))
	if errDeleteNodeMinute != nil {
		log.Error().Err(errDeleteNodeMinute).Msg("Failed to delete old minute-level node metrics history")
	} else if deletedNodeMinute > 0 {
		log.Debug().Int64("deleted", deletedNodeMinute).Msg("Cleaned up old minute-level node metrics history rows")
	}

	deletedNodeHourly, errDeleteNodeHourly := mr.db.DeleteNodeHourlyMetricsHistoryOlderThan(now.Add(-hourlyRetention))
	if errDeleteNodeHourly != nil {
		log.Error().Err(errDeleteNodeHourly).Msg("Failed to delete old hourly node metrics history")
	} else if deletedNodeHourly > 0 {
		log.Debug().Int64("deleted", deletedNodeHourly).Msg("Cleaned up old hourly node metrics history rows")
	}

	deletedGS, errDeleteGS := mr.db.DeleteGameServerMetricsHistoryOlderThan(now.Add(-hourlyRetention))
	if errDeleteGS != nil {
		log.Error().Err(errDeleteGS).Msg("Failed to delete old game server metrics history")
	} else if deletedGS > 0 {
		log.Debug().Int64("deleted", deletedGS).Msg("Cleaned up old game server metrics history rows")
	}
}
