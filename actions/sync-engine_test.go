package actions

import (
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/db/dbtest"
)

func TestNormalizeNodeSyncInterval(t *testing.T) {
	tests := []struct {
		name      string
		seconds   int32
		wantValue time.Duration
	}{
		{name: "uses default for zero", seconds: 0, wantValue: defaultNodeSyncInterval},
		{name: "uses default for negative", seconds: -30, wantValue: defaultNodeSyncInterval},
		{name: "clamps to minimum", seconds: 30, wantValue: minNodeSyncInterval},
		{name: "uses configured interval in range", seconds: 120, wantValue: 120 * time.Second},
		{name: "clamps to maximum", seconds: int32((maxNodeSyncInterval / time.Second) + 1), wantValue: maxNodeSyncInterval},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeNodeSyncInterval(tt.seconds)
			if got != tt.wantValue {
				t.Errorf("normalizeNodeSyncInterval(%d) = %v, want %v", tt.seconds, got, tt.wantValue)
			}
		})
	}
}

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		name       string
		retryCount int64
		maxBackoff time.Duration
	}{
		{name: "first retry", retryCount: 0, maxBackoff: 6 * time.Second},
		{name: "second retry", retryCount: 1, maxBackoff: 7 * time.Second},
		{name: "third retry", retryCount: 2, maxBackoff: 9 * time.Second},
		{name: "high retry count caps at max", retryCount: 20, maxBackoff: maxRetryBackoff + 5*time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateBackoff(tt.retryCount)
			if got < 0 {
				t.Errorf("calculateBackoff(%d) returned negative duration: %v", tt.retryCount, got)
			}
			if got > tt.maxBackoff {
				t.Errorf("calculateBackoff(%d) = %v, exceeds expected max %v", tt.retryCount, got, tt.maxBackoff)
			}
		})
	}
}

func TestCalculateBackoffIncreases(t *testing.T) {
	// Run multiple times to average out jitter.
	sumLow := time.Duration(0)
	sumHigh := time.Duration(0)
	iterations := 100

	for range iterations {
		sumLow += calculateBackoff(0)
		sumHigh += calculateBackoff(5)
	}

	avgLow := sumLow / time.Duration(iterations)
	avgHigh := sumHigh / time.Duration(iterations)

	if avgHigh <= avgLow {
		t.Errorf("Expected higher retry count to produce longer backoff: avg(retry=0)=%v, avg(retry=5)=%v", avgLow, avgHigh)
	}
}

func TestCalculateBackoffCapsAtMax(t *testing.T) {
	for range 50 {
		got := calculateBackoff(100)
		// Should never exceed maxRetryBackoff + max jitter (5s).
		if got > maxRetryBackoff+5*time.Second {
			t.Errorf("calculateBackoff(100) = %v, exceeds max + jitter", got)
		}
	}
}

func TestCleanupRemoteServerCacheRemovesOrphanedAndStaleRows(t *testing.T) {
	conn := dbtest.NewMigratedConnection(t, "sync-engine-cleanup.sqlite")

	// Disable foreign keys temporarily so we can insert orphaned rows for testing cleanup behavior.
	_, errDisableFK := conn.SQLDb.Exec(`PRAGMA foreign_keys = OFF`)
	if errDisableFK != nil {
		t.Fatalf("failed to disable foreign keys: %v", errDisableFK)
	}
	t.Cleanup(func() {
		_, _ = conn.SQLDb.Exec(`PRAGMA foreign_keys = ON`)
	})

	_, errInsertNodes := conn.SQLDb.Exec(`
		INSERT INTO node (id, name, is_local, host, port) VALUES
			('remote-1', 'Remote Node', 0, '', 0),
			('local-1', 'Local Node', 1, '', 0)
	`)
	if errInsertNodes != nil {
		t.Fatalf("failed to insert node rows: %v", errInsertNodes)
	}

	oldUpdatedAt := time.Now().Add(-10 * time.Minute)
	freshUpdatedAt := time.Now()

	_, errInsertCache := conn.SQLDb.Exec(`
		INSERT INTO remote_server_cache (id, source_node_id, node_id, remote_server_id, is_stale, updated_at) VALUES
			('cache-1', 'remote-1', 'remote-1', 'server-stale-old', 1, ?),
			('cache-2', 'remote-1', 'remote-1', 'server-stale-fresh', 1, ?),
			('cache-3', 'remote-1', 'remote-1', 'server-not-stale', 0, ?),
			('cache-4', 'missing-node', 'remote-1', 'server-orphan-source', 0, ?),
			('cache-5', 'remote-1', 'missing-node', 'server-orphan-node', 0, ?),
			('cache-6', 'local-1', 'local-1', 'server-local-ref', 0, ?)
	`, oldUpdatedAt, freshUpdatedAt, oldUpdatedAt, freshUpdatedAt, freshUpdatedAt, freshUpdatedAt)
	if errInsertCache != nil {
		t.Fatalf("failed to insert remote_server_cache rows: %v", errInsertCache)
	}

	engine := &FederationSyncEngine{db: conn}
	engine.cleanupRemoteServerCache()

	rows, errRows := conn.SQLDb.Query(`SELECT remote_server_id FROM remote_server_cache ORDER BY remote_server_id`)
	if errRows != nil {
		t.Fatalf("failed to query remaining cache rows: %v", errRows)
	}
	t.Cleanup(func() {
		if errCloseRows := rows.Close(); errCloseRows != nil {
			t.Errorf("failed to close rows: %v", errCloseRows)
		}
	})

	remaining := make([]string, 0)
	for rows.Next() {
		var serverID string
		errScan := rows.Scan(&serverID)
		if errScan != nil {
			t.Fatalf("failed to scan row: %v", errScan)
		}
		remaining = append(remaining, serverID)
	}
	if errRowsNext := rows.Err(); errRowsNext != nil {
		t.Fatalf("failed iterating rows: %v", errRowsNext)
	}

	if len(remaining) != 2 {
		t.Fatalf("remaining row count = %d, want %d (rows: %v)", len(remaining), 2, remaining)
	}
	if remaining[0] != "server-not-stale" {
		t.Errorf("remaining[0] = %q, want %q", remaining[0], "server-not-stale")
	}
	if remaining[1] != "server-stale-fresh" {
		t.Errorf("remaining[1] = %q, want %q", remaining[1], "server-stale-fresh")
	}
}
