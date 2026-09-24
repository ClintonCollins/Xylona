package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	migrate "github.com/rubenv/sql-migrate"
)

type metricsHistorySnapshot struct {
	recordedAt string
	cpuPercent float64
}

func TestEnhanceGameServerMetricsMigrationMarksLegacyValidityUnknown(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "metrics-history-legacy-migration.sqlite")
	conn, errConnection := NewConnection(context.Background(), dbPath)
	if errConnection != nil {
		t.Fatalf("NewConnection() error = %v", errConnection)
	}
	t.Cleanup(func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil {
			t.Errorf("close test database: %v", errClose)
		}
	})

	migrationsDir, errMigrationsDir := rbacMigrationsDir()
	if errMigrationsDir != nil {
		t.Fatalf("locate migrations: %v", errMigrationsDir)
	}
	migrationSource := &migrate.FileMigrationSource{Dir: migrationsDir}
	setTableOnce.Do(func() { migrate.SetTable("migrations") })
	_, errPreviousMigrations := migrate.ExecVersion(conn.SQLDb, "sqlite3", migrationSource, migrate.Up, 20260716000000)
	if errPreviousMigrations != nil {
		t.Fatalf("migrate to previous version: %v", errPreviousMigrations)
	}

	seedRBACFixture(t, conn)
	legacyRecordedAt := time.Date(2026, time.April, 10, 18, 0, 0, 0, time.UTC)
	insertLegacyGameServerMetricsHistoryRow(
		t,
		conn,
		"pre-migration-metrics",
		"server-local-1",
		25,
		512,
		fmtTime(legacyRecordedAt),
	)

	_, errLatestMigration := migrate.ExecVersion(conn.SQLDb, "sqlite3", migrationSource, migrate.Up, 20260717000000)
	if errLatestMigration != nil {
		t.Fatalf("apply enhanced metrics migration: %v", errLatestMigration)
	}

	rows, errHistory := conn.GetGameServerMetricsHistory(
		"server-local-1",
		legacyRecordedAt,
		legacyRecordedAt.Add(time.Second),
	)
	if errHistory != nil {
		t.Fatalf("GetGameServerMetricsHistory() error = %v", errHistory)
	}
	if len(rows) != 1 {
		t.Fatalf("GetGameServerMetricsHistory() len = %d, want 1", len(rows))
	}
	legacy := rows[0]
	if legacy.CPUPercent != 25 || legacy.MemoryBytes != 512 {
		t.Fatalf("legacy numeric values = (CPU %v, memory %d), want (25, 512)", legacy.CPUPercent, legacy.MemoryBytes)
	}
	if legacy.AvailableSampleCount != 0 || legacy.AvailabilityRatio != 0 || legacy.CollectionStatus != "unknown" {
		t.Fatalf(
			"legacy availability = (%d samples, %v ratio, %q status), want (0, 0, unknown)",
			legacy.AvailableSampleCount,
			legacy.AvailabilityRatio,
			legacy.CollectionStatus,
		)
	}
	if legacy.CPUValid || legacy.CPUValidSampleCount != 0 || legacy.IOValidSampleCount != 0 ||
		legacy.ConnectionValidSampleCount != 0 || legacy.VolumeValid || legacy.VolumeValidSampleCount != 0 {
		t.Fatalf(
			"legacy validity = (CPU %t/%d, I/O %d, connection %d, volume %t/%d), want all unknown",
			legacy.CPUValid,
			legacy.CPUValidSampleCount,
			legacy.IOValidSampleCount,
			legacy.ConnectionValidSampleCount,
			legacy.VolumeValid,
			legacy.VolumeValidSampleCount,
		)
	}
}

func TestRollupGameServerMetricsToHourlyHandlesLegacyRecordedAt(t *testing.T) {
	conn := newRBACMigratedConnection(t, "metrics-history-game-server.sqlite")
	seedRBACFixture(t, conn)

	cutoff := time.Date(2026, time.April, 10, 20, 30, 0, 0, time.UTC)

	insertLegacyGameServerMetricsHistoryRow(
		t,
		conn,
		"gs-legacy-a",
		"server-local-1",
		10,
		256,
		time.Date(2026, time.April, 10, 18, 10, 0, 0, time.UTC).String(),
	)
	insertLegacyGameServerMetricsHistoryRow(
		t,
		conn,
		"gs-legacy-b",
		"server-local-1",
		30,
		512,
		time.Date(2026, time.April, 10, 18, 40, 0, 0, time.UTC).String(),
	)

	errInsertCurrentA := conn.InsertGameServerMetricsHistory(&GameServerMetricsRow{
		ID:              "gs-current-a",
		GameServerID:    "server-local-1",
		CPUPercent:      40,
		MemoryBytes:     1024,
		MemoryPercent:   20,
		DiskUsageBytes:  2048,
		IOReadRate:      4,
		IOWriteRate:     6,
		ConnectionCount: 2,
		PlayerCount:     3,
		RecordedAt:      time.Date(2026, time.April, 10, 19, 5, 0, 0, time.UTC),
	})
	if errInsertCurrentA != nil {
		t.Fatalf("InsertGameServerMetricsHistory(current A) error = %v", errInsertCurrentA)
	}

	errInsertCurrentB := conn.InsertGameServerMetricsHistory(&GameServerMetricsRow{
		ID:              "gs-current-b",
		GameServerID:    "server-local-1",
		CPUPercent:      60,
		MemoryBytes:     2048,
		MemoryPercent:   30,
		DiskUsageBytes:  4096,
		IOReadRate:      8,
		IOWriteRate:     10,
		ConnectionCount: 4,
		PlayerCount:     5,
		RecordedAt:      time.Date(2026, time.April, 10, 19, 35, 0, 0, time.UTC),
	})
	if errInsertCurrentB != nil {
		t.Fatalf("InsertGameServerMetricsHistory(current B) error = %v", errInsertCurrentB)
	}

	errRollup := conn.RollupGameServerMetricsToHourly(cutoff)
	if errRollup != nil {
		t.Fatalf("RollupGameServerMetricsToHourly() error = %v", errRollup)
	}

	rows := loadGameServerMetricsSnapshots(t, conn, "server-local-1")
	if len(rows) != 2 {
		t.Fatalf("loadGameServerMetricsSnapshots() len = %d, want 2", len(rows))
	}

	if rows[0].recordedAt != `2026-04-10 18:00:00` {
		t.Fatalf("legacy bucket recorded_at = %q, want %q", rows[0].recordedAt, `2026-04-10 18:00:00`)
	}
	if rows[0].cpuPercent != 20 {
		t.Fatalf("legacy bucket CPUPercent = %v, want 20", rows[0].cpuPercent)
	}

	if rows[1].recordedAt != `2026-04-10 19:00:00` {
		t.Fatalf("current bucket recorded_at = %q, want %q", rows[1].recordedAt, `2026-04-10 19:00:00`)
	}
	if rows[1].cpuPercent != 50 {
		t.Fatalf("current bucket CPUPercent = %v, want 50", rows[1].cpuPercent)
	}

	history, errHistory := conn.GetGameServerMetricsHistory(
		"server-local-1",
		time.Date(2026, time.April, 10, 18, 0, 0, 0, time.UTC),
		time.Date(2026, time.April, 10, 20, 0, 0, 0, time.UTC),
	)
	if errHistory != nil {
		t.Fatalf("GetGameServerMetricsHistory() error = %v", errHistory)
	}
	if len(history) != 2 {
		t.Fatalf("GetGameServerMetricsHistory() len = %d, want 2", len(history))
	}
	legacy := history[0]
	if legacy.AvailableSampleCount != 0 || legacy.AvailabilityRatio != 0 || legacy.CollectionStatus != "unknown" {
		t.Fatalf(
			"legacy availability = (%d samples, %v ratio, %q status), want (0, 0, unknown)",
			legacy.AvailableSampleCount,
			legacy.AvailabilityRatio,
			legacy.CollectionStatus,
		)
	}
	if legacy.CPUValid || legacy.CPUValidSampleCount != 0 {
		t.Fatalf("legacy CPU validity = (%t, %d samples), want (false, 0)", legacy.CPUValid, legacy.CPUValidSampleCount)
	}
	if legacy.IOValidSampleCount != 0 || legacy.ConnectionValidSampleCount != 0 ||
		legacy.VolumeValid || legacy.VolumeValidSampleCount != 0 {
		t.Fatalf(
			"legacy I/O, connection, and volume validity = (%d, %d, %t/%d), want unknown",
			legacy.IOValidSampleCount,
			legacy.ConnectionValidSampleCount,
			legacy.VolumeValid,
			legacy.VolumeValidSampleCount,
		)
	}
}

func TestRollupGameServerMetricsToHourlyUsesLatestLifecycleMetadata(t *testing.T) {
	conn := newRBACMigratedConnection(t, "metrics-history-game-server-latest-metadata.sqlite")
	seedRBACFixture(t, conn)

	bucketStart := time.Date(2026, time.July, 17, 14, 0, 0, 0, time.UTC)
	rows := []*GameServerMetricsRow{
		{
			ID:                    "a-latest-row",
			GameServerID:          "server-local-1",
			NodeID:                sql.NullString{String: "a-latest-node", Valid: true},
			RecordedAt:            bucketStart.Add(50 * time.Minute),
			GranularitySeconds:    60,
			SampleCount:           1,
			AvailableSampleCount:  1,
			CollectionStatus:      "available",
			CPUValid:              true,
			CPUValidSet:           true,
			NodeCPUCores:          sql.NullInt64{Int64: 4, Valid: true},
			NodeMemoryTotalBytes:  sql.NullInt64{Int64: 8_000, Valid: true},
			ConfiguredMemoryBytes: sql.NullInt64{Int64: 1_024, Valid: true},
			VolumeTotalBytes:      sql.NullInt64{Int64: 500, Valid: true},
			PlayerCapacity:        sql.NullInt64{Int64: 10, Valid: true},
			QuerySupported:        sql.NullBool{Bool: false, Valid: true},
			QuerySuccess:          sql.NullBool{Bool: false, Valid: true},
			ServerUptimeSeconds:   sql.NullInt64{Int64: 10, Valid: true},
			ProcessStatus:         sql.NullString{String: "a_latest", Valid: true},
			ExecutionID:           sql.NullString{String: "a-latest-execution", Valid: true},
		},
		{
			ID:                    "z-older-row",
			GameServerID:          "server-local-1",
			NodeID:                sql.NullString{String: "z-older-node", Valid: true},
			RecordedAt:            bucketStart.Add(10 * time.Minute),
			GranularitySeconds:    60,
			SampleCount:           1,
			AvailableSampleCount:  1,
			CollectionStatus:      "available",
			CPUValid:              true,
			CPUValidSet:           true,
			NodeCPUCores:          sql.NullInt64{Int64: 16, Valid: true},
			NodeMemoryTotalBytes:  sql.NullInt64{Int64: 32_000, Valid: true},
			ConfiguredMemoryBytes: sql.NullInt64{Int64: 4_096, Valid: true},
			VolumeTotalBytes:      sql.NullInt64{Int64: 1_000, Valid: true},
			PlayerCapacity:        sql.NullInt64{Int64: 100, Valid: true},
			QuerySupported:        sql.NullBool{Bool: true, Valid: true},
			QuerySuccess:          sql.NullBool{Bool: true, Valid: true},
			ServerUptimeSeconds:   sql.NullInt64{Int64: 900, Valid: true},
			ProcessStatus:         sql.NullString{String: "z_old", Valid: true},
			ExecutionID:           sql.NullString{String: "z-older-execution", Valid: true},
		},
	}
	for _, row := range rows {
		errInsert := conn.InsertGameServerMetricsHistory(row)
		if errInsert != nil {
			t.Fatalf("InsertGameServerMetricsHistory(%s) error = %v", row.ID, errInsert)
		}
	}

	errRollup := conn.RollupGameServerMetricsToHourly(bucketStart.Add(time.Hour))
	if errRollup != nil {
		t.Fatalf("RollupGameServerMetricsToHourly() error = %v", errRollup)
	}

	hourlyRows, errQuery := conn.GetGameServerMetricsHistory(
		"server-local-1",
		bucketStart,
		bucketStart.Add(time.Hour),
	)
	if errQuery != nil {
		t.Fatalf("GetGameServerMetricsHistory() error = %v", errQuery)
	}
	if len(hourlyRows) != 1 {
		t.Fatalf("hourly row count = %d, want 1", len(hourlyRows))
	}
	hourly := hourlyRows[0]
	if !hourly.NodeID.Valid || hourly.NodeID.String != "a-latest-node" {
		t.Fatalf("node ID = %+v, want latest node", hourly.NodeID)
	}
	if !hourly.ProcessStatus.Valid || hourly.ProcessStatus.String != "a_latest" {
		t.Fatalf("process status = %+v, want latest status", hourly.ProcessStatus)
	}
	if !hourly.ExecutionID.Valid || hourly.ExecutionID.String != "a-latest-execution" {
		t.Fatalf("execution ID = %+v, want latest execution", hourly.ExecutionID)
	}
	if !hourly.ServerUptimeSeconds.Valid || hourly.ServerUptimeSeconds.Int64 != 10 {
		t.Fatalf("server uptime = %+v, want latest uptime 10", hourly.ServerUptimeSeconds)
	}
	if !hourly.NodeCPUCores.Valid || hourly.NodeCPUCores.Int64 != 4 {
		t.Fatalf("node CPU cores = %+v, want latest value 4", hourly.NodeCPUCores)
	}
	if !hourly.NodeMemoryTotalBytes.Valid || hourly.NodeMemoryTotalBytes.Int64 != 8_000 {
		t.Fatalf("node memory total = %+v, want latest value 8000", hourly.NodeMemoryTotalBytes)
	}
	if !hourly.ConfiguredMemoryBytes.Valid || hourly.ConfiguredMemoryBytes.Int64 != 1_024 {
		t.Fatalf("configured memory = %+v, want latest value 1024", hourly.ConfiguredMemoryBytes)
	}
	if !hourly.VolumeTotalBytes.Valid || hourly.VolumeTotalBytes.Int64 != 500 {
		t.Fatalf("volume total = %+v, want latest value 500", hourly.VolumeTotalBytes)
	}
	if !hourly.PlayerCapacity.Valid || hourly.PlayerCapacity.Int64 != 10 {
		t.Fatalf("player capacity = %+v, want latest value 10", hourly.PlayerCapacity)
	}
	if !hourly.QuerySupported.Valid || hourly.QuerySupported.Bool {
		t.Fatalf("query supported = %+v, want latest false value", hourly.QuerySupported)
	}
	if !hourly.QuerySuccess.Valid || hourly.QuerySuccess.Bool {
		t.Fatalf("query success = %+v, want latest false value", hourly.QuerySuccess)
	}
}

func TestRollupGameServerMetricsToHourlyIsAtomicAndIdempotent(t *testing.T) {
	t.Run("replaces one rollup per server hour and preserves metric weights", func(t *testing.T) {
		conn := newRBACMigratedConnection(t, "metrics-history-game-server-idempotent.sqlite")
		seedRBACFixture(t, conn)

		bucketStart := time.Date(2026, time.July, 17, 8, 0, 0, 0, time.UTC)
		for index := range 60 {
			cpuValid := index == 0
			querySuccessful := index == 0
			row := &GameServerMetricsRow{
				ID:                   fmt.Sprintf("weighted-raw-%02d", index),
				GameServerID:         "server-local-1",
				CPUPercent:           999,
				MemoryBytes:          100,
				DiskUsageBytes:       999,
				PlayerCount:          999,
				RecordedAt:           bucketStart.Add(time.Duration(index) * time.Minute),
				GranularitySeconds:   60,
				SampleCount:          1,
				AvailableSampleCount: 1,
				CollectionStatus:     "available",
				CPUValid:             cpuValid,
				CPUValidSet:          true,
				VolumeValid:          cpuValid,
				QuerySuccess:         sql.NullBool{Bool: querySuccessful, Valid: true},
				QueryDurationMS:      sql.NullFloat64{Float64: 999, Valid: true},
				QueryDurationMSMin:   sql.NullFloat64{Float64: 999, Valid: true},
				QueryDurationMSMax:   sql.NullFloat64{Float64: 999, Valid: true},
				ServerFPS:            sql.NullFloat64{Float64: 999, Valid: true},
				ServerFPSMin:         sql.NullFloat64{Float64: 999, Valid: true},
				ServerFPSMax:         sql.NullFloat64{Float64: 999, Valid: true},
				ServerFrameTimeMS:    sql.NullFloat64{Float64: 999, Valid: true},
				ServerFrameTimeMSMin: sql.NullFloat64{Float64: 999, Valid: true},
				ServerFrameTimeMSMax: sql.NullFloat64{Float64: 999, Valid: true},
			}
			if cpuValid {
				row.CPUPercent = 60
			}
			if querySuccessful {
				row.PlayerCount = 12
				row.QueryDurationMS = sql.NullFloat64{Float64: 60, Valid: true}
				row.QueryDurationMSMin = row.QueryDurationMS
				row.QueryDurationMSMax = row.QueryDurationMS
				row.ServerFPS = sql.NullFloat64{Float64: 60, Valid: true}
				row.ServerFPSMin = row.ServerFPS
				row.ServerFPSMax = row.ServerFPS
				row.ServerFrameTimeMS = sql.NullFloat64{Float64: 10, Valid: true}
				row.ServerFrameTimeMSMin = row.ServerFrameTimeMS
				row.ServerFrameTimeMSMax = row.ServerFrameTimeMS
			}
			if cpuValid {
				row.DiskUsageBytes = 600
			}
			errInsert := conn.InsertGameServerMetricsHistory(row)
			if errInsert != nil {
				t.Fatalf("InsertGameServerMetricsHistory(raw %d) error = %v", index, errInsert)
			}
		}

		_, errSeedRollup := conn.SQLDb.ExecContext(
			conn.ctx,
			`INSERT INTO game_server_metrics_history (
				id, game_server_id, cpu_percent, player_count, recorded_at,
				granularity_seconds, sample_count, available_sample_count,
				cpu_valid_sample_count, query_successful_sample_count,
				collection_status, cpu_valid, query_success, rollup_hour
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"stale-rollup",
			"server-local-1",
			1,
			1,
			fmtTime(bucketStart),
			3600,
			1,
			1,
			1,
			1,
			"available",
			true,
			true,
			fmtTime(bucketStart),
		)
		if errSeedRollup != nil {
			t.Fatalf("insert stale hourly rollup: %v", errSeedRollup)
		}

		cutoff := bucketStart.Add(2 * time.Hour)
		for attempt := range 2 {
			errRollup := conn.RollupGameServerMetricsToHourly(cutoff)
			if errRollup != nil {
				t.Fatalf("RollupGameServerMetricsToHourly(attempt %d) error = %v", attempt+1, errRollup)
			}
		}

		rows, errQuery := conn.GetGameServerMetricsHistory(
			"server-local-1",
			bucketStart,
			bucketStart.Add(time.Hour),
		)
		if errQuery != nil {
			t.Fatalf("GetGameServerMetricsHistory() error = %v", errQuery)
		}
		if len(rows) != 1 {
			t.Fatalf("hourly row count = %d, want 1", len(rows))
		}
		row := rows[0]
		if row.SampleCount != 60 || row.CPUValidSampleCount != 1 || row.QuerySuccessfulSampleCount != 1 ||
			row.QueryDurationValidSampleCount != 1 || row.ServerFPSValidSampleCount != 1 ||
			row.ServerFrameTimeValidSampleCount != 1 || row.VolumeValidSampleCount != 1 {
			t.Fatalf(
				"sample weights = (%d total, %d CPU, %d query, %d duration, %d FPS, %d frame time, %d volume), want (60, 1, 1, 1, 1, 1, 1)",
				row.SampleCount,
				row.CPUValidSampleCount,
				row.QuerySuccessfulSampleCount,
				row.QueryDurationValidSampleCount,
				row.ServerFPSValidSampleCount,
				row.ServerFrameTimeValidSampleCount,
				row.VolumeValidSampleCount,
			)
		}
		if row.CPUPercent != 60 || row.PlayerCount != 12 || row.DiskUsageBytes != 600 {
			t.Fatalf(
				"weighted values = (CPU %v, players %d, disk %d), want (60, 12, 600)",
				row.CPUPercent,
				row.PlayerCount,
				row.DiskUsageBytes,
			)
		}
		if !row.QueryDurationMS.Valid || row.QueryDurationMS.Float64 != 60 ||
			!row.ServerFPS.Valid || row.ServerFPS.Float64 != 60 ||
			!row.ServerFrameTimeMS.Valid || row.ServerFrameTimeMS.Float64 != 10 {
			t.Fatalf(
				"query performance values = (duration %+v, FPS %+v, frame time %+v), want (60, 60, 10)",
				row.QueryDurationMS,
				row.ServerFPS,
				row.ServerFrameTimeMS,
			)
		}
	})

	t.Run("delete failure rolls back the hourly insert", func(t *testing.T) {
		conn := newRBACMigratedConnection(t, "metrics-history-game-server-atomic.sqlite")
		seedRBACFixture(t, conn)

		bucketStart := time.Date(2026, time.July, 17, 9, 0, 0, 0, time.UTC)
		for index := range 2 {
			errInsert := conn.InsertGameServerMetricsHistory(&GameServerMetricsRow{
				ID:                   fmt.Sprintf("atomic-raw-%d", index),
				GameServerID:         "server-local-1",
				CPUPercent:           float64(10 + index*20),
				RecordedAt:           bucketStart.Add(time.Duration(index) * time.Minute),
				GranularitySeconds:   60,
				SampleCount:          1,
				AvailableSampleCount: 1,
				CollectionStatus:     "available",
				CPUValid:             true,
				CPUValidSet:          true,
			})
			if errInsert != nil {
				t.Fatalf("InsertGameServerMetricsHistory(raw %d) error = %v", index, errInsert)
			}
		}

		_, errTrigger := conn.SQLDb.ExecContext(
			conn.ctx,
			`CREATE TRIGGER fail_game_server_metrics_rollup_delete
			BEFORE DELETE ON game_server_metrics_history
			WHEN OLD.granularity_seconds < 3600
			BEGIN
				SELECT RAISE(ABORT, 'forced rollup delete failure');
			END`,
		)
		if errTrigger != nil {
			t.Fatalf("create delete failure trigger: %v", errTrigger)
		}

		errRollup := conn.RollupGameServerMetricsToHourly(bucketStart.Add(time.Hour))
		if errRollup == nil {
			t.Fatal("RollupGameServerMetricsToHourly() error = nil, want forced delete failure")
		}

		var rawCount int
		var hourlyCount int
		errCounts := conn.SQLDb.QueryRowContext(
			conn.ctx,
			`SELECT
				SUM(CASE WHEN granularity_seconds < 3600 THEN 1 ELSE 0 END),
				SUM(CASE WHEN rollup_hour IS NOT NULL THEN 1 ELSE 0 END)
			FROM game_server_metrics_history
			WHERE game_server_id = ?`,
			"server-local-1",
		).Scan(&rawCount, &hourlyCount)
		if errCounts != nil {
			t.Fatalf("query post-rollback row counts: %v", errCounts)
		}
		if rawCount != 2 || hourlyCount != 0 {
			t.Fatalf("post-rollback counts = (%d raw, %d hourly), want (2, 0)", rawCount, hourlyCount)
		}
	})
}

func TestInsertGameServerMetricsHistoryValidityCountDefaults(t *testing.T) {
	tests := []struct {
		name                string
		ioCountSet          bool
		connectionCountSet  bool
		wantIOCount         int64
		wantConnectionCount int64
	}{
		{
			name:                "explicit zero counts remain invalid",
			ioCountSet:          true,
			connectionCountSet:  true,
			wantIOCount:         0,
			wantConnectionCount: 0,
		},
		{
			name:                "omitted counts retain legacy availability default",
			wantIOCount:         1,
			wantConnectionCount: 1,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn := newRBACMigratedConnection(t, fmt.Sprintf("metrics-history-validity-count-%d.sqlite", index))
			seedRBACFixture(t, conn)

			errInsert := conn.InsertGameServerMetricsHistory(&GameServerMetricsRow{
				ID:                            fmt.Sprintf("validity-count-%d", index),
				GameServerID:                  "server-local-1",
				RecordedAt:                    time.Date(2026, time.July, 17, 12, index, 0, 0, time.UTC),
				GranularitySeconds:            60,
				SampleCount:                   1,
				AvailableSampleCount:          1,
				CollectionStatus:              "available",
				IOValidSampleCountSet:         test.ioCountSet,
				ConnectionValidSampleCountSet: test.connectionCountSet,
			})
			if errInsert != nil {
				t.Fatalf("InsertGameServerMetricsHistory() error = %v", errInsert)
			}

			var ioCount int64
			var connectionCount int64
			errQuery := conn.SQLDb.QueryRowContext(
				conn.ctx,
				`SELECT io_valid_sample_count, connection_valid_sample_count
				FROM game_server_metrics_history
				WHERE id = ?`,
				fmt.Sprintf("validity-count-%d", index),
			).Scan(&ioCount, &connectionCount)
			if errQuery != nil {
				t.Fatalf("query persisted validity counts: %v", errQuery)
			}
			if ioCount != test.wantIOCount || connectionCount != test.wantConnectionCount {
				t.Fatalf(
					"persisted validity counts = (%d IO, %d connection), want (%d, %d)",
					ioCount,
					connectionCount,
					test.wantIOCount,
					test.wantConnectionCount,
				)
			}
		})
	}
}

func TestRollupNodeMetricsToHourlyHandlesLegacyRecordedAt(t *testing.T) {
	conn := newRBACMigratedConnection(t, "metrics-history-node.sqlite")
	seedRBACFixture(t, conn)

	cutoff := time.Date(2026, time.April, 10, 20, 30, 0, 0, time.UTC)

	insertLegacyNodeMetricsHistoryRow(
		t,
		conn,
		"node-legacy-a",
		"node-local",
		10,
		1024,
		time.Date(2026, time.April, 10, 18, 10, 0, 0, time.UTC).String(),
	)
	insertLegacyNodeMetricsHistoryRow(
		t,
		conn,
		"node-legacy-b",
		"node-local",
		30,
		2048,
		time.Date(2026, time.April, 10, 18, 40, 0, 0, time.UTC).String(),
	)

	errInsertCurrentA := conn.InsertNodeMetricsHistory(&NodeMetricsRow{
		ID:                     "node-current-a",
		NodeID:                 "node-local",
		CPUPercent:             validFloat64(40),
		MemoryPercent:          validFloat64(20),
		MemoryUsedBytes:        validInt64(4096),
		MemoryTotalBytes:       validInt64(8192),
		DiskPercent:            validFloat64(10),
		DiskUsedBytes:          validInt64(1000),
		DiskTotalBytes:         validInt64(2000),
		GameServerCount:        2,
		RunningGameServerCount: 1,
		UserCount:              3,
		RecordedAt:             time.Date(2026, time.April, 10, 19, 5, 0, 0, time.UTC),
	})
	if errInsertCurrentA != nil {
		t.Fatalf("InsertNodeMetricsHistory(current A) error = %v", errInsertCurrentA)
	}

	errInsertCurrentB := conn.InsertNodeMetricsHistory(&NodeMetricsRow{
		ID:                     "node-current-b",
		NodeID:                 "node-local",
		CPUPercent:             validFloat64(60),
		MemoryPercent:          validFloat64(30),
		MemoryUsedBytes:        validInt64(6144),
		MemoryTotalBytes:       validInt64(8192),
		DiskPercent:            validFloat64(20),
		DiskUsedBytes:          validInt64(1200),
		DiskTotalBytes:         validInt64(2000),
		GameServerCount:        4,
		RunningGameServerCount: 3,
		UserCount:              5,
		RecordedAt:             time.Date(2026, time.April, 10, 19, 35, 0, 0, time.UTC),
	})
	if errInsertCurrentB != nil {
		t.Fatalf("InsertNodeMetricsHistory(current B) error = %v", errInsertCurrentB)
	}

	errRollup := conn.RollupNodeMetricsToHourly(cutoff)
	if errRollup != nil {
		t.Fatalf("RollupNodeMetricsToHourly() error = %v", errRollup)
	}

	rows := loadNodeMetricsSnapshots(t, conn, "node-local")
	if len(rows) != 2 {
		t.Fatalf("loadNodeMetricsSnapshots() len = %d, want 2", len(rows))
	}

	if rows[0].recordedAt != `2026-04-10 18:00:00` {
		t.Fatalf("legacy bucket recorded_at = %q, want %q", rows[0].recordedAt, `2026-04-10 18:00:00`)
	}
	if rows[0].cpuPercent != 20 {
		t.Fatalf("legacy bucket CPUPercent = %v, want 20", rows[0].cpuPercent)
	}

	if rows[1].recordedAt != `2026-04-10 19:00:00` {
		t.Fatalf("current bucket recorded_at = %q, want %q", rows[1].recordedAt, `2026-04-10 19:00:00`)
	}
	if rows[1].cpuPercent != 50 {
		t.Fatalf("current bucket CPUPercent = %v, want 50", rows[1].cpuPercent)
	}
}

func TestRollupNodeMetricsToHourlyStoresIntegerByteAverages(t *testing.T) {
	conn := newRBACMigratedConnection(t, "metrics-history-node-bytes.sqlite")
	seedRBACFixture(t, conn)

	for index, sample := range []struct{ memory, disk int64 }{{1000, 7}, {1001, 8}} {
		errInsert := conn.InsertNodeMetricsHistory(&NodeMetricsRow{
			ID:               fmt.Sprintf("node-bytes-%d", index),
			NodeID:           "node-local",
			MemoryUsedBytes:  validInt64(sample.memory),
			MemoryTotalBytes: validInt64(2048),
			DiskUsedBytes:    validInt64(sample.disk),
			DiskTotalBytes:   validInt64(16),
			RecordedAt:       time.Date(2026, time.April, 10, 18, 10+index*20, 0, 0, time.UTC),
		})
		if errInsert != nil {
			t.Fatalf("InsertNodeMetricsHistory(%d) error = %v", index, errInsert)
		}
	}

	errRollup := conn.RollupNodeMetricsToHourly(time.Date(2026, time.April, 10, 20, 0, 0, 0, time.UTC))
	if errRollup != nil {
		t.Fatalf("RollupNodeMetricsToHourly() error = %v", errRollup)
	}

	rows, errHistory := conn.GetNodeMetricsHistory(
		"node-local",
		time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.April, 11, 0, 0, 0, 0, time.UTC),
	)
	if errHistory != nil {
		t.Fatalf("GetNodeMetricsHistory() error = %v", errHistory)
	}
	if len(rows) != 1 {
		t.Fatalf("GetNodeMetricsHistory() len = %d, want 1", len(rows))
	}
	if rows[0].MemoryUsedBytes != validInt64(1001) || rows[0].DiskUsedBytes != validInt64(8) {
		t.Fatalf("rolled-up bytes = (%v memory, %v disk), want (1001, 8)", rows[0].MemoryUsedBytes, rows[0].DiskUsedBytes)
	}
}

// A reading the node could not take is stored as NULL and read back as
// invalid, never as a 0% that charts would plot.
func TestNodeMetricsHistoryStoresUnavailableReadingsAsNull(t *testing.T) {
	conn := newRBACMigratedConnection(t, "metrics-history-node-null.sqlite")
	seedRBACFixture(t, conn)

	recordedAt := time.Date(2026, time.April, 10, 18, 10, 0, 0, time.UTC)
	errInsert := conn.InsertNodeMetricsHistory(&NodeMetricsRow{
		ID:               "node-partial",
		NodeID:           "node-local",
		MemoryPercent:    validFloat64(40),
		MemoryUsedBytes:  validInt64(400),
		MemoryTotalBytes: validInt64(1000),
		RecordedAt:       recordedAt,
	})
	if errInsert != nil {
		t.Fatalf("InsertNodeMetricsHistory() error = %v", errInsert)
	}

	var cpuNull, memoryNull, diskNull, diskUsedNull, diskTotalNull bool
	errScan := conn.SQLDb.QueryRowContext(conn.ctx,
		`SELECT cpu_percent IS NULL, memory_percent IS NULL, disk_percent IS NULL,
			disk_used_bytes IS NULL, disk_total_bytes IS NULL
		FROM node_metrics_history WHERE id = ?`, "node-partial",
	).Scan(&cpuNull, &memoryNull, &diskNull, &diskUsedNull, &diskTotalNull)
	if errScan != nil {
		t.Fatalf("read stored row: %v", errScan)
	}
	if !cpuNull || memoryNull || !diskNull || !diskUsedNull || !diskTotalNull {
		t.Fatalf("stored NULLs = (cpu %t, memory %t, disk %t/%t/%t), want (true, false, true/true/true)",
			cpuNull, memoryNull, diskNull, diskUsedNull, diskTotalNull)
	}

	rows, errHistory := conn.GetNodeMetricsHistory("node-local", recordedAt, recordedAt.Add(time.Second))
	if errHistory != nil {
		t.Fatalf("GetNodeMetricsHistory() error = %v", errHistory)
	}
	if len(rows) != 1 {
		t.Fatalf("GetNodeMetricsHistory() len = %d, want 1", len(rows))
	}
	if rows[0].CPUPercent.Valid || rows[0].DiskUsedBytes.Valid || rows[0].MemoryPercent != validFloat64(40) {
		t.Fatalf("read back cpu %v, disk used %v, memory %v; want invalid, invalid, 40",
			rows[0].CPUPercent, rows[0].DiskUsedBytes, rows[0].MemoryPercent)
	}
}

func TestRollupNodeMetricsToHourlyIgnoresUnavailableReadings(t *testing.T) {
	tests := []struct {
		name    string
		samples []sql.NullFloat64
		want    sql.NullFloat64
	}{
		{name: "all valid", samples: []sql.NullFloat64{validFloat64(20), validFloat64(40)}, want: validFloat64(30)},
		{name: "unavailable samples are skipped", samples: []sql.NullFloat64{validFloat64(20), {}, {}}, want: validFloat64(20)},
		{name: "no valid sample stays unavailable", samples: []sql.NullFloat64{{}, {}}, want: sql.NullFloat64{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn := newRBACMigratedConnection(t, "metrics-history-node-rollup-null.sqlite")
			seedRBACFixture(t, conn)
			for index, sample := range test.samples {
				errInsert := conn.InsertNodeMetricsHistory(&NodeMetricsRow{
					ID:          fmt.Sprintf("node-sample-%d", index),
					NodeID:      "node-local",
					CPUPercent:  sample,
					DiskPercent: sample,
					RecordedAt:  time.Date(2026, time.April, 10, 18, 10+index*10, 0, 0, time.UTC),
				})
				if errInsert != nil {
					t.Fatalf("InsertNodeMetricsHistory(%d) error = %v", index, errInsert)
				}
			}

			errRollup := conn.RollupNodeMetricsToHourly(time.Date(2026, time.April, 10, 20, 0, 0, 0, time.UTC))
			if errRollup != nil {
				t.Fatalf("RollupNodeMetricsToHourly() error = %v", errRollup)
			}

			rows, errHistory := conn.GetNodeMetricsHistory(
				"node-local",
				time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC),
				time.Date(2026, time.April, 11, 0, 0, 0, 0, time.UTC),
			)
			if errHistory != nil {
				t.Fatalf("GetNodeMetricsHistory() error = %v", errHistory)
			}
			if len(rows) != 1 {
				t.Fatalf("GetNodeMetricsHistory() len = %d, want 1 hourly row", len(rows))
			}
			if rows[0].CPUPercent != test.want || rows[0].DiskPercent != test.want {
				t.Fatalf("hourly (cpu, disk) = (%v, %v), want %v", rows[0].CPUPercent, rows[0].DiskPercent, test.want)
			}
		})
	}
}

// The rebuild drops NOT NULL without losing rows or the history index, and its
// Down restores NOT NULL by turning unavailable readings back into 0.
func TestNodeMetricReadingsNullableMigrationRoundTrip(t *testing.T) {
	const previousVersion = 20260923041416
	dbPath := filepath.Join(t.TempDir(), "metrics-history-node-nullable-migration.sqlite")
	conn, errConnection := NewConnection(context.Background(), dbPath)
	if errConnection != nil {
		t.Fatalf("NewConnection() error = %v", errConnection)
	}
	t.Cleanup(func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil {
			t.Errorf("close test database: %v", errClose)
		}
	})
	migrationsDir, errMigrationsDir := rbacMigrationsDir()
	if errMigrationsDir != nil {
		t.Fatalf("locate migrations: %v", errMigrationsDir)
	}
	source := &migrate.FileMigrationSource{Dir: migrationsDir}
	setTableOnce.Do(func() { migrate.SetTable("migrations") })
	_, errPrevious := migrate.ExecVersion(conn.SQLDb, "sqlite3", source, migrate.Up, previousVersion)
	if errPrevious != nil {
		t.Fatalf("migrate to previous version: %v", errPrevious)
	}
	seedRBACFixture(t, conn)
	insertLegacyNodeMetricsHistoryRow(t, conn, "legacy", "node-local", 12, 1024, "2026-04-10 18:10:00")

	_, errUp := migrate.ExecMax(conn.SQLDb, "sqlite3", source, migrate.Up, 1)
	if errUp != nil {
		t.Fatalf("apply nullable migration: %v", errUp)
	}
	assertNodeMetricsSchema(t, conn, false)
	errInsert := conn.InsertNodeMetricsHistory(&NodeMetricsRow{
		ID:         "unavailable",
		NodeID:     "node-local",
		RecordedAt: time.Date(2026, time.April, 10, 18, 20, 0, 0, time.UTC),
	})
	if errInsert != nil {
		t.Fatalf("insert unavailable row: %v", errInsert)
	}

	_, errDown := migrate.ExecMax(conn.SQLDb, "sqlite3", source, migrate.Down, 1)
	if errDown != nil {
		t.Fatalf("roll back nullable migration: %v", errDown)
	}
	assertNodeMetricsSchema(t, conn, true)
	rows := loadNodeMetricsSnapshots(t, conn, "node-local")
	if len(rows) != 2 || rows[0].cpuPercent != 12 || rows[1].cpuPercent != 0 {
		t.Fatalf("rows after Down = %#v, want legacy 12 and unavailable mapped to 0", rows)
	}
}

func assertNodeMetricsSchema(t *testing.T, conn *Connection, wantNotNull bool) {
	t.Helper()
	var notNull bool
	errColumn := conn.SQLDb.QueryRowContext(conn.ctx,
		`SELECT "notnull" FROM pragma_table_info('node_metrics_history') WHERE name = 'cpu_percent'`,
	).Scan(&notNull)
	if errColumn != nil {
		t.Fatalf("read cpu_percent column info: %v", errColumn)
	}
	if notNull != wantNotNull {
		t.Fatalf("cpu_percent NOT NULL = %t, want %t", notNull, wantNotNull)
	}
	var indexCount, foreignKeyCount int
	errIndex := conn.SQLDb.QueryRowContext(conn.ctx,
		`SELECT count(*) FROM pragma_index_list('node_metrics_history') WHERE name = 'idx_node_metrics_node_time'`,
	).Scan(&indexCount)
	if errIndex != nil {
		t.Fatalf("read node metrics indexes: %v", errIndex)
	}
	errForeignKey := conn.SQLDb.QueryRowContext(conn.ctx,
		`SELECT count(*) FROM pragma_foreign_key_list('node_metrics_history')
		WHERE "table" = 'node' AND "from" = 'node_id' AND on_delete = 'CASCADE'`,
	).Scan(&foreignKeyCount)
	if errForeignKey != nil {
		t.Fatalf("read node metrics foreign keys: %v", errForeignKey)
	}
	if indexCount != 1 || foreignKeyCount != 1 {
		t.Fatalf("history index = %d, node cascade foreign key = %d; want 1 and 1", indexCount, foreignKeyCount)
	}
}

func validFloat64(value float64) sql.NullFloat64 {
	return sql.NullFloat64{Float64: value, Valid: true}
}

func validInt64(value int64) sql.NullInt64 {
	return sql.NullInt64{Int64: value, Valid: true}
}

func insertLegacyGameServerMetricsHistoryRow(
	t *testing.T,
	conn *Connection,
	id string,
	gameServerID string,
	cpuPercent float64,
	memoryBytes int64,
	recordedAt string,
) {
	t.Helper()

	_, errExec := conn.SQLDb.ExecContext(
		conn.ctx,
		`INSERT INTO game_server_metrics_history (
			id,
			game_server_id,
			cpu_percent,
			memory_bytes,
			memory_percent,
			disk_usage_bytes,
			io_read_rate,
			io_write_rate,
			connection_count,
			player_count,
			recorded_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id,
		gameServerID,
		cpuPercent,
		memoryBytes,
		10,
		2048,
		1,
		2,
		3,
		4,
		recordedAt,
	)
	if errExec != nil {
		t.Fatalf("insert legacy game server metrics history row: %v", errExec)
	}
}

func insertLegacyNodeMetricsHistoryRow(
	t *testing.T,
	conn *Connection,
	id string,
	nodeID string,
	cpuPercent float64,
	memoryUsedBytes int64,
	recordedAt string,
) {
	t.Helper()

	_, errExec := conn.SQLDb.ExecContext(
		conn.ctx,
		`INSERT INTO node_metrics_history (
			id,
			node_id,
			cpu_percent,
			memory_percent,
			memory_used_bytes,
			memory_total_bytes,
			disk_percent,
			disk_used_bytes,
			disk_total_bytes,
			game_server_count,
			running_game_server_count,
			user_count,
			recorded_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id,
		nodeID,
		cpuPercent,
		10,
		memoryUsedBytes,
		4096,
		20,
		2048,
		8192,
		2,
		1,
		3,
		recordedAt,
	)
	if errExec != nil {
		t.Fatalf("insert legacy node metrics history row: %v", errExec)
	}
}

func loadGameServerMetricsSnapshots(t *testing.T, conn *Connection, gameServerID string) []metricsHistorySnapshot {
	t.Helper()

	rows, errQuery := conn.SQLDb.QueryContext(
		conn.ctx,
		`SELECT recorded_at, cpu_percent
		FROM game_server_metrics_history
		WHERE game_server_id = ?
		ORDER BY recorded_at ASC`,
		gameServerID,
	)
	if errQuery != nil {
		t.Fatalf("query game server metrics history snapshots: %v", errQuery)
	}
	defer func() {
		_ = rows.Close()
	}()

	var snapshots []metricsHistorySnapshot
	for rows.Next() {
		var snapshot metricsHistorySnapshot
		errScan := rows.Scan(&snapshot.recordedAt, &snapshot.cpuPercent)
		if errScan != nil {
			t.Fatalf("scan game server metrics history snapshot: %v", errScan)
		}
		snapshot.recordedAt = normalizeMetricsRecordedAtForAssertion(snapshot.recordedAt)
		snapshots = append(snapshots, snapshot)
	}

	errRows := rows.Err()
	if errRows != nil {
		t.Fatalf("iterate game server metrics history snapshots: %v", errRows)
	}

	return snapshots
}

func loadNodeMetricsSnapshots(t *testing.T, conn *Connection, nodeID string) []metricsHistorySnapshot {
	t.Helper()

	rows, errQuery := conn.SQLDb.QueryContext(
		conn.ctx,
		`SELECT recorded_at, cpu_percent
		FROM node_metrics_history
		WHERE node_id = ?
		ORDER BY recorded_at ASC`,
		nodeID,
	)
	if errQuery != nil {
		t.Fatalf("query node metrics history snapshots: %v", errQuery)
	}
	defer func() {
		_ = rows.Close()
	}()

	var snapshots []metricsHistorySnapshot
	for rows.Next() {
		var snapshot metricsHistorySnapshot
		errScan := rows.Scan(&snapshot.recordedAt, &snapshot.cpuPercent)
		if errScan != nil {
			t.Fatalf("scan node metrics history snapshot: %v", errScan)
		}
		snapshot.recordedAt = normalizeMetricsRecordedAtForAssertion(snapshot.recordedAt)
		snapshots = append(snapshots, snapshot)
	}

	errRows := rows.Err()
	if errRows != nil {
		t.Fatalf("iterate node metrics history snapshots: %v", errRows)
	}

	return snapshots
}

func normalizeMetricsRecordedAtForAssertion(value string) string {
	if len(value) > len(`2006-01-02 15:04:05`) {
		value = value[:len(`2006-01-02 15:04:05`)]
	}

	return strings.ReplaceAll(value, "T", " ")
}
