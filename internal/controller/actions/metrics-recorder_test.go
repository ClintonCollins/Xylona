package actions

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/db/dbtest"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestNormalizeMetricsRecorderConfig(t *testing.T) {
	t.Parallel()

	defaults := DefaultMetricsRecorderConfig()
	tests := []struct {
		name  string
		input MetricsRecorderConfig
		want  MetricsRecorderConfig
	}{
		{
			name: "zero values use defaults",
			want: defaults,
		},
		{
			name: "custom values are preserved",
			input: MetricsRecorderConfig{
				SnapshotInterval: 30 * time.Second,
				CleanupInterval:  2 * time.Minute,
				HistoryRetention: 7 * 24 * time.Hour,
				RollupAfter:      6 * time.Hour,
			},
			want: MetricsRecorderConfig{
				SnapshotInterval: 30 * time.Second,
				CleanupInterval:  2 * time.Minute,
				HistoryRetention: 7 * 24 * time.Hour,
				RollupAfter:      6 * time.Hour,
			},
		},
		{
			name: "collection and retention costs are bounded",
			input: MetricsRecorderConfig{
				SnapshotInterval: time.Second,
				CleanupInterval:  48 * time.Hour,
				HistoryRetention: time.Hour,
				RollupAfter:      time.Minute,
			},
			want: MetricsRecorderConfig{
				SnapshotInterval: minMetricsSnapshotInterval,
				CleanupInterval:  maxMetricsCleanupInterval,
				HistoryRetention: minMetricsHistoryRetention,
				RollupAfter:      minMetricsRollupAfter,
			},
		},
		{
			name: "rollup remains inside retention window",
			input: MetricsRecorderConfig{
				SnapshotInterval: defaults.SnapshotInterval,
				CleanupInterval:  defaults.CleanupInterval,
				HistoryRetention: minMetricsHistoryRetention,
				RollupAfter:      48 * time.Hour,
			},
			want: MetricsRecorderConfig{
				SnapshotInterval: defaults.SnapshotInterval,
				CleanupInterval:  defaults.CleanupInterval,
				HistoryRetention: minMetricsHistoryRetention,
				RollupAfter:      minMetricsHistoryRetention / 2,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeMetricsRecorderConfig(test.input)
			if got != test.want {
				t.Errorf("normalizeMetricsRecorderConfig() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestMetricsRecorderCleanupAndRollupPreservesHourlyNodeHistory(t *testing.T) {
	ctx := context.Background()
	conn := dbtest.NewMigratedConnection(t, "metrics-recorder.sqlite")
	seedMetricsRecorderNodeFixture(t, conn)

	now := time.Now().UTC().Truncate(time.Hour)
	oldMinuteA := now.Add(-(8 * 24 * time.Hour)).Add(10 * time.Minute)
	oldMinuteB := now.Add(-(8 * 24 * time.Hour)).Add(40 * time.Minute)
	survivingHourly := now.Add(-(30 * 24 * time.Hour))
	expiredHourly := now.Add(-(100 * 24 * time.Hour))

	errInsertMinuteA := conn.InsertNodeMetricsHistory(&db.NodeMetricsRow{
		ID:                     "minute-a",
		NodeID:                 "node-local",
		CPUPercent:             10,
		MemoryPercent:          20,
		MemoryUsedBytes:        1000,
		MemoryTotalBytes:       2000,
		DiskPercent:            30,
		DiskUsedBytes:          3000,
		DiskTotalBytes:         4000,
		GameServerCount:        1,
		RunningGameServerCount: 1,
		UserCount:              2,
		RecordedAt:             oldMinuteA,
	})
	if errInsertMinuteA != nil {
		t.Fatalf("InsertNodeMetricsHistory(minute A) error = %v", errInsertMinuteA)
	}

	errInsertMinuteB := conn.InsertNodeMetricsHistory(&db.NodeMetricsRow{
		ID:                     "minute-b",
		NodeID:                 "node-local",
		CPUPercent:             30,
		MemoryPercent:          40,
		MemoryUsedBytes:        2000,
		MemoryTotalBytes:       2000,
		DiskPercent:            50,
		DiskUsedBytes:          3500,
		DiskTotalBytes:         4000,
		GameServerCount:        3,
		RunningGameServerCount: 2,
		UserCount:              4,
		RecordedAt:             oldMinuteB,
	})
	if errInsertMinuteB != nil {
		t.Fatalf("InsertNodeMetricsHistory(minute B) error = %v", errInsertMinuteB)
	}

	errInsertSurvivingHourly := conn.InsertNodeMetricsHistory(&db.NodeMetricsRow{
		ID:                     "hourly-surviving",
		NodeID:                 "node-local",
		CPUPercent:             50,
		MemoryPercent:          60,
		MemoryUsedBytes:        2500,
		MemoryTotalBytes:       3000,
		DiskPercent:            70,
		DiskUsedBytes:          3600,
		DiskTotalBytes:         4000,
		GameServerCount:        5,
		RunningGameServerCount: 4,
		UserCount:              6,
		RecordedAt:             survivingHourly,
	})
	if errInsertSurvivingHourly != nil {
		t.Fatalf("InsertNodeMetricsHistory(surviving hourly) error = %v", errInsertSurvivingHourly)
	}

	errInsertExpiredHourly := conn.InsertNodeMetricsHistory(&db.NodeMetricsRow{
		ID:                     "hourly-expired",
		NodeID:                 "node-local",
		CPUPercent:             70,
		MemoryPercent:          80,
		MemoryUsedBytes:        2600,
		MemoryTotalBytes:       3000,
		DiskPercent:            90,
		DiskUsedBytes:          3700,
		DiskTotalBytes:         4000,
		GameServerCount:        7,
		RunningGameServerCount: 6,
		UserCount:              8,
		RecordedAt:             expiredHourly,
	})
	if errInsertExpiredHourly != nil {
		t.Fatalf("InsertNodeMetricsHistory(expired hourly) error = %v", errInsertExpiredHourly)
	}

	recorder := &MetricsRecorder{
		ctx: ctx,
		db:  conn,
	}

	recorder.cleanupAndRollup()

	rows := loadNodeMetricSnapshots(t, conn)
	if len(rows) != 2 {
		t.Fatalf("loadNodeMetricSnapshots() len = %d, want 2", len(rows))
	}

	gotByHour := map[string]nodeMetricSnapshot{}
	for _, row := range rows {
		gotByHour[row.recordedAt] = row
	}

	aggregatedHour := now.Add(-(8 * 24 * time.Hour)).Format(`2006-01-02 15:04:05`)
	survivingHourlyRecordedAt := survivingHourly.Format(`2006-01-02 15:04:05`)
	expiredHourlyRecordedAt := expiredHourly.Format(`2006-01-02 15:04:05`)

	if _, ok := gotByHour[survivingHourlyRecordedAt]; !ok {
		t.Fatalf("missing surviving hourly row for %v", survivingHourlyRecordedAt)
	}
	if _, ok := gotByHour[aggregatedHour]; !ok {
		t.Fatalf("missing rolled-up hourly row for %v", aggregatedHour)
	}
	if _, ok := gotByHour[expiredHourlyRecordedAt]; ok {
		t.Fatalf("found expired hourly row for %v, want deleted", expiredHourlyRecordedAt)
	}

	if gotByHour[aggregatedHour].cpuPercent != 20 {
		t.Fatalf("rolled-up hourly CPUPercent = %v, want 20", gotByHour[aggregatedHour].cpuPercent)
	}
}

func TestMetricsRecorderCleanupAndRollupWaitsForCompleteHours(t *testing.T) {
	t.Parallel()

	conn := dbtest.NewMigratedConnection(t, "metrics-recorder-complete-hours.sqlite")
	seedMetricsRecorderNodeFixture(t, conn)
	now := time.Date(2026, time.July, 17, 12, 35, 0, 0, time.UTC)
	completeHourSample := now.Add(-2 * time.Hour).Add(5 * time.Minute)
	boundaryHourSample := now.Add(-time.Hour).Add(5 * time.Minute)
	for id, recordedAt := range map[string]time.Time{
		"complete-hour": completeHourSample,
		"boundary-hour": boundaryHourSample,
	} {
		errInsert := conn.InsertNodeMetricsHistory(&db.NodeMetricsRow{
			ID:               id,
			NodeID:           "node-local",
			CPUPercent:       25,
			MemoryPercent:    30,
			MemoryUsedBytes:  1000,
			MemoryTotalBytes: 2000,
			RecordedAt:       recordedAt,
		})
		if errInsert != nil {
			t.Fatalf("InsertNodeMetricsHistory(%s) error = %v", id, errInsert)
		}
	}

	recorder := &MetricsRecorder{
		ctx: context.Background(),
		db:  conn,
		config: MetricsRecorderConfig{
			RollupAfter:      time.Hour,
			HistoryRetention: 24 * time.Hour,
		},
	}
	recorder.cleanupAndRollupAt(now)

	rows := loadNodeMetricSnapshots(t, conn)
	if len(rows) != 2 {
		t.Fatalf("loadNodeMetricSnapshots() len = %d, want 2", len(rows))
	}
	recordedAt := map[string]struct{}{}
	for _, row := range rows {
		recordedAt[row.recordedAt] = struct{}{}
	}
	wantRolledUp := completeHourSample.Truncate(time.Hour).Format("2006-01-02 15:04:05")
	wantBoundaryRaw := boundaryHourSample.Format("2006-01-02 15:04:05")
	_, rolledUp := recordedAt[wantRolledUp]
	if !rolledUp {
		t.Fatalf("missing complete-hour rollup at %s; rows = %#v", wantRolledUp, rows)
	}
	_, boundaryRaw := recordedAt[wantBoundaryRaw]
	if !boundaryRaw {
		t.Fatalf("boundary-hour raw sample at %s was rolled up too early; rows = %#v", wantBoundaryRaw, rows)
	}
}

func TestMetricsRecorderDoesNotAttachStaleQueryDataToUnavailableSamples(t *testing.T) {
	t.Parallel()

	telemetry := &Instance{}
	telemetry.recordSuccessfulGameServerQuery("server-1", xylona.ServerQuery_Palworld, time.Now().Add(-time.Millisecond), &xylona.ServerQuery{
		Type: xylona.ServerQuery_Palworld,
		Palworld: &xylona.PalworldQueryInfo{
			Players:           4,
			MaxPlayers:        16,
			ServerFps:         60,
			ServerFrameTimeMs: 16.7,
		},
	})
	recorder := &MetricsRecorder{
		config:         DefaultMetricsRecorderConfig(),
		queryTelemetry: telemetry,
	}
	gameServer := &models.GameServer{ID: "server-1", NodeID: "node-1"}
	now := time.Now().UTC()

	unavailable := recorder.unavailableGameServerMetricsRow(gameServer, "node-1", now, "node_unavailable")
	assertQueryMetricsUnavailable(t, unavailable)

	offline := recorder.gameServerMetricsRow(gameServer, "node-1", &node.NodeSnapshot{Collected: now}, &node.ProcessSnapshot{
		ID:           "server-1",
		Status:       xylona.Status_OFFLINE.String(),
		MetricsValid: true,
	}, now)
	assertQueryMetricsUnavailable(t, offline)
	if offline.CollectionStatus != "server_offline" || offline.AvailableSampleCount != 0 || offline.AvailabilityRatio != 0 {
		t.Fatalf("offline collection state = (%q, %d, %v), want server_offline, 0, 0", offline.CollectionStatus, offline.AvailableSampleCount, offline.AvailabilityRatio)
	}
}

func TestMetricsRecorderOnlyAttachesSuccessfulQueryPerformanceTelemetry(t *testing.T) {
	t.Parallel()

	checkedAt := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		status      GameServerQueryTelemetryStatus
		wantSamples bool
	}{
		{
			name:   "failed query omits performance samples",
			status: GameServerQueryTelemetryStatusFailure,
		},
		{
			name:        "successful query includes valid performance samples",
			status:      GameServerQueryTelemetryStatusSuccess,
			wantSamples: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := staticGameServerQueryTelemetryProvider{snapshot: GameServerQueryTelemetrySnapshot{
				Status:                   test.status,
				CheckedAt:                checkedAt,
				Duration:                 25 * time.Millisecond,
				DurationValid:            true,
				PalworldServerFPS:        60,
				PalworldServerFPSValid:   true,
				PalworldFrameTimeMS:      16.7,
				PalworldFrameTimeMSValid: true,
			}}
			recorder := &MetricsRecorder{queryTelemetry: provider}
			row := &db.GameServerMetricsRow{}
			recorder.applyQueryTelemetry(row, "server-1")

			if row.QueryCheckedAt == nil || !row.QueryCheckedAt.Equal(checkedAt) {
				t.Fatalf("query checked at = %v, want %v", row.QueryCheckedAt, checkedAt)
			}
			if row.QueryDurationMS.Valid != test.wantSamples || row.ServerFPS.Valid != test.wantSamples || row.ServerFrameTimeMS.Valid != test.wantSamples {
				t.Fatalf(
					"performance sample validity = (duration %t, FPS %t, frame time %t), want %t",
					row.QueryDurationMS.Valid,
					row.ServerFPS.Valid,
					row.ServerFrameTimeMS.Valid,
					test.wantSamples,
				)
			}
		})
	}
}

func TestMetricsRecorderPreservesMetricSpecificValidity(t *testing.T) {
	t.Parallel()

	recorder := &MetricsRecorder{config: DefaultMetricsRecorderConfig()}
	gameServer := &models.GameServer{ID: "server-1", NodeID: "node-1"}
	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name                       string
		process                    *node.ProcessSnapshot
		wantIOValidCount           int64
		wantConnectionValidCount   int64
		wantIOExtrema              bool
		wantConnectionCountExtrema bool
	}{
		{
			name: "valid zero IO is distinct from unavailable connections",
			process: &node.ProcessSnapshot{
				ID:                   "server-1",
				Status:               xylona.Status_ONLINE.String(),
				MetricsValid:         true,
				IOValid:              true,
				ConnectionCountValid: false,
			},
			wantIOValidCount: 1,
			wantIOExtrema:    true,
		},
		{
			name: "valid zero connections are distinct from unavailable IO",
			process: &node.ProcessSnapshot{
				ID:                   "server-1",
				Status:               xylona.Status_ONLINE.String(),
				MetricsValid:         true,
				IOValid:              false,
				ConnectionCountValid: true,
			},
			wantConnectionValidCount:   1,
			wantConnectionCountExtrema: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := recorder.gameServerMetricsRow(
				gameServer,
				"node-1",
				&node.NodeSnapshot{Collected: now},
				tt.process,
				now,
			)
			if !row.IOValidSampleCountSet || !row.ConnectionValidSampleCountSet {
				t.Fatal("metric-specific validity counts must be explicitly set")
			}
			if row.IOValidSampleCount != tt.wantIOValidCount {
				t.Errorf("IO valid sample count = %d, want %d", row.IOValidSampleCount, tt.wantIOValidCount)
			}
			if row.ConnectionValidSampleCount != tt.wantConnectionValidCount {
				t.Errorf("connection valid sample count = %d, want %d", row.ConnectionValidSampleCount, tt.wantConnectionValidCount)
			}
			if row.IOReadRateMin.Valid != tt.wantIOExtrema || row.IOWriteRateMax.Valid != tt.wantIOExtrema {
				t.Errorf("IO extrema validity = (%t, %t), want %t", row.IOReadRateMin.Valid, row.IOWriteRateMax.Valid, tt.wantIOExtrema)
			}
			if row.ConnectionCountMin.Valid != tt.wantConnectionCountExtrema || row.ConnectionCountMax.Valid != tt.wantConnectionCountExtrema {
				t.Errorf("connection extrema validity = (%t, %t), want %t", row.ConnectionCountMin.Valid, row.ConnectionCountMax.Valid, tt.wantConnectionCountExtrema)
			}
		})
	}
}

func assertQueryMetricsUnavailable(t *testing.T, row *db.GameServerMetricsRow) {
	t.Helper()
	if row.QuerySuccess.Valid || row.PlayerCountMin.Valid || row.ServerFPS.Valid || row.ServerFrameTimeMS.Valid {
		t.Fatalf("unavailable row contains query telemetry: success=%v players=%v fps=%v frame_time=%v", row.QuerySuccess, row.PlayerCountMin, row.ServerFPS, row.ServerFrameTimeMS)
	}
}

type staticGameServerQueryTelemetryProvider struct {
	snapshot GameServerQueryTelemetrySnapshot
}

func (provider staticGameServerQueryTelemetryProvider) GetGameServerQueryTelemetry(string) GameServerQueryTelemetrySnapshot {
	return provider.snapshot
}

func seedMetricsRecorderNodeFixture(t *testing.T, conn *db.Connection) {
	t.Helper()

	_, errExec := conn.SQLDb.ExecContext(
		context.Background(),
		`INSERT INTO node (id, name, listen_url, enabled) VALUES (?, ?, ?, ?)`,
		"node-local",
		"Local Node",
		"http://localhost:8080",
		true,
	)
	if errExec != nil {
		t.Fatalf("insert node fixture: %v", errExec)
	}
}

type nodeMetricSnapshot struct {
	recordedAt string
	cpuPercent float64
}

func loadNodeMetricSnapshots(t *testing.T, conn *db.Connection) []nodeMetricSnapshot {
	t.Helper()

	rows, errQuery := conn.SQLDb.QueryContext(
		context.Background(),
		`SELECT recorded_at, cpu_percent
		FROM node_metrics_history
		WHERE node_id = ?
		ORDER BY recorded_at ASC`,
		"node-local",
	)
	if errQuery != nil {
		t.Fatalf("query node metrics snapshots: %v", errQuery)
	}
	defer func() {
		_ = rows.Close()
	}()

	var snapshots []nodeMetricSnapshot
	for rows.Next() {
		var snapshot nodeMetricSnapshot
		errScan := rows.Scan(&snapshot.recordedAt, &snapshot.cpuPercent)
		if errScan != nil {
			t.Fatalf("scan node metrics snapshot: %v", errScan)
		}
		snapshot.recordedAt = normalizeRecordedAtForAssertion(snapshot.recordedAt)
		snapshots = append(snapshots, snapshot)
	}

	errRows := rows.Err()
	if errRows != nil {
		t.Fatalf("iterate node metrics snapshots: %v", errRows)
	}

	sort.Slice(snapshots, func(i int, j int) bool {
		return snapshots[i].recordedAt < snapshots[j].recordedAt
	})

	return snapshots
}

func normalizeRecordedAtForAssertion(value string) string {
	if len(value) > len(`2006-01-02 15:04:05`) {
		value = value[:len(`2006-01-02 15:04:05`)]
	}

	return strings.ReplaceAll(value, "T", " ")
}
