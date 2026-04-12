package db

import (
	"strings"
	"testing"
	"time"
)

type metricsHistorySnapshot struct {
	recordedAt string
	cpuPercent float64
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
		CPUPercent:             40,
		MemoryPercent:          20,
		MemoryUsedBytes:        4096,
		MemoryTotalBytes:       8192,
		DiskPercent:            10,
		DiskUsedBytes:          1000,
		DiskTotalBytes:         2000,
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
		CPUPercent:             60,
		MemoryPercent:          30,
		MemoryUsedBytes:        6144,
		MemoryTotalBytes:       8192,
		DiskPercent:            20,
		DiskUsedBytes:          1200,
		DiskTotalBytes:         2000,
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
