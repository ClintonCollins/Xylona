package actions

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/db/dbtest"
)

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
