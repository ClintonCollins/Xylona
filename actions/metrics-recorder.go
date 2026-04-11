package actions

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/sysinfo"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/supervisor"
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

// MetricsRecorder periodically records node and game server metrics to the database.
type MetricsRecorder struct {
	ctx            context.Context
	db             *db.Connection
	supervisorInst *supervisor.Instance
	localNodeID    string
	playerCounts   PlayerCountProvider
}

// NewMetricsRecorder creates and starts a new metrics recorder.
func NewMetricsRecorder(ctx context.Context, dbInst *db.Connection, supervisorInst *supervisor.Instance, localNodeID string, playerCounts PlayerCountProvider) *MetricsRecorder {
	mr := &MetricsRecorder{
		ctx:            ctx,
		db:             dbInst,
		supervisorInst: supervisorInst,
		localNodeID:    localNodeID,
		playerCounts:   playerCounts,
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
				mr.recordNodeMetrics()
				mr.recordGameServerMetrics()
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

func (mr *MetricsRecorder) recordNodeMetrics() {
	now := time.Now().UTC()
	snapshot, errSnapshot := sysinfo.CollectResourceSnapshot()
	if errSnapshot != nil {
		log.Error().Err(errSnapshot).Msg("Failed to collect node resource snapshot for metrics history")
		return
	}

	gameServerCount, errGSCount := mr.db.CountGameServers()
	if errGSCount != nil {
		log.Error().Err(errGSCount).Msg("Failed to count game servers for metrics history")
	}

	runningCount := 0
	if mr.supervisorInst != nil {
		for _, cmd := range mr.supervisorInst.ListCommands() {
			if cmd.Status() == xylona.Status_ONLINE || cmd.Status() == xylona.Status_INSTALLING || cmd.Status() == xylona.Status_UPDATING {
				runningCount++
			}
		}
	}

	userCount, errUsers := mr.db.CountUsers()
	if errUsers != nil {
		log.Error().Err(errUsers).Msg("Failed to count users for metrics history")
	}

	row := &db.NodeMetricsRow{
		ID:                     uuid.New().String(),
		NodeID:                 mr.localNodeID,
		CPUPercent:             snapshot.CPUPercent,
		MemoryPercent:          snapshot.MemoryPercent,
		MemoryUsedBytes:        helpers.ClampInt64FromUint64(snapshot.MemoryUsed),
		MemoryTotalBytes:       helpers.ClampInt64FromUint64(snapshot.MemoryTotal),
		DiskPercent:            snapshot.DiskPercent,
		DiskUsedBytes:          helpers.ClampInt64FromUint64(snapshot.DiskUsed),
		DiskTotalBytes:         helpers.ClampInt64FromUint64(snapshot.DiskTotal),
		GameServerCount:        gameServerCount,
		RunningGameServerCount: runningCount,
		UserCount:              userCount,
		RecordedAt:             now,
	}

	errInsert := mr.db.InsertNodeMetricsHistory(row)
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Failed to insert node metrics history")
	}
}

func (mr *MetricsRecorder) recordGameServerMetrics() {
	if mr.supervisorInst == nil {
		return
	}

	now := time.Now().UTC()
	commands := mr.supervisorInst.ListCommands()

	for _, cmd := range commands {
		cpuPercent, memoryRSS, _, memoryPercent, _, _, diskUsageBytes, ioReadRate, ioWriteRate, connectionCount := cmd.Metrics()

		playerCount := 0
		if mr.playerCounts != nil {
			playerCount = mr.playerCounts.GetPlayerCount(cmd.ID)
		}

		row := &db.GameServerMetricsRow{
			ID:              uuid.New().String(),
			GameServerID:    cmd.ID,
			CPUPercent:      cpuPercent,
			MemoryBytes:     helpers.ClampInt64FromUint64(memoryRSS),
			MemoryPercent:   float64(memoryPercent),
			DiskUsageBytes:  helpers.ClampInt64FromUint64(diskUsageBytes),
			IOReadRate:      ioReadRate,
			IOWriteRate:     ioWriteRate,
			ConnectionCount: int(connectionCount),
			PlayerCount:     playerCount,
			RecordedAt:      now,
		}

		errInsert := mr.db.InsertGameServerMetricsHistory(row)
		if errInsert != nil {
			log.Error().Err(errInsert).Str("game_server_id", cmd.ID).Msg("Failed to insert game server metrics history")
		}
	}
}

func (mr *MetricsRecorder) cleanupAndRollup() {
	now := time.Now().UTC()

	// Delete minute-level data older than 7 days.
	deletedNode, errDeleteNode := mr.db.DeleteNodeMetricsHistoryOlderThan(now.Add(-minuteRetention))
	if errDeleteNode != nil {
		log.Error().Err(errDeleteNode).Msg("Failed to delete old node metrics history")
	} else if deletedNode > 0 {
		log.Debug().Int64("deleted", deletedNode).Msg("Cleaned up old node metrics history rows")
	}

	deletedGS, errDeleteGS := mr.db.DeleteGameServerMetricsHistoryOlderThan(now.Add(-hourlyRetention))
	if errDeleteGS != nil {
		log.Error().Err(errDeleteGS).Msg("Failed to delete old game server metrics history")
	} else if deletedGS > 0 {
		log.Debug().Int64("deleted", deletedGS).Msg("Cleaned up old game server metrics history rows")
	}

	// Rollup minute-level data older than 24h to hourly aggregates.
	errRollupNode := mr.db.RollupNodeMetricsToHourly(now.Add(-rollupCutoff))
	if errRollupNode != nil {
		log.Error().Err(errRollupNode).Msg("Failed to rollup node metrics to hourly")
	}

	errRollupGS := mr.db.RollupGameServerMetricsToHourly(now.Add(-rollupCutoff))
	if errRollupGS != nil {
		log.Error().Err(errRollupGS).Msg("Failed to rollup game server metrics to hourly")
	}
}
