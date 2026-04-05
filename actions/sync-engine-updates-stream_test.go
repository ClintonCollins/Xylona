package actions

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/db/dbtest"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// mockStatusBroadcaster records BroadcastRemoteServerStatus calls for test assertions.
type mockStatusBroadcaster struct {
	mu    sync.Mutex
	calls []statusBroadcastCall
}

type statusBroadcastCall struct {
	serverID string
	status   xylona.Status
}

func (m *mockStatusBroadcaster) BroadcastRemoteServerStatus(serverID string, status xylona.Status) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, statusBroadcastCall{serverID: serverID, status: status})
}

// mockMetricsBroadcaster records BroadcastRemoteServerMetrics calls for test assertions.
type mockMetricsBroadcaster struct {
	mu    sync.Mutex
	calls []metricsBroadcastCall
}

type metricsBroadcastCall struct {
	serverID string
	metrics  *xylona.GameServerMetrics
}

func (m *mockMetricsBroadcaster) BroadcastRemoteServerMetrics(serverID string, metrics *xylona.GameServerMetrics) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, metricsBroadcastCall{serverID: serverID, metrics: metrics})
}

// newSyncEngineTestDB creates a test SQLite DB with all migrations applied for sync engine tests.
func newSyncEngineTestDB(t *testing.T) *db.Connection {
	t.Helper()
	return dbtest.NewMigratedConnection(t, "sync-engine-updates-stream.sqlite")
}

// insertRemoteNode inserts a remote (non-local) node row for testing.
func insertRemoteNode(t *testing.T, conn *db.Connection, id string, name string) {
	t.Helper()

	_, errInsert := conn.SQLDb.ExecContext(
		context.Background(),
		`INSERT INTO node (id, name, is_local, host, port) VALUES (?, ?, 0, '', 0)`,
		id, name,
	)
	if errInsert != nil {
		t.Fatalf("failed to insert remote node %q: %v", id, errInsert)
	}
}

// insertRemoteServerCache inserts a remote server cache row for testing.
func insertRemoteServerCache(t *testing.T, conn *db.Connection, id, sourceNodeID, nodeID, remoteServerID, status string) {
	t.Helper()

	now := time.Now()
	_, errInsert := conn.SQLDb.ExecContext(
		context.Background(),
		`INSERT INTO remote_server_cache (id, source_node_id, node_id, remote_server_id, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, sourceNodeID, nodeID, remoteServerID, status, now, now,
	)
	if errInsert != nil {
		t.Fatalf("failed to insert remote server cache %q: %v", id, errInsert)
	}
}

func TestHandleSnapshot_UpsertsServers(t *testing.T) {
	conn := newSyncEngineTestDB(t)

	// Seed a remote node that the snapshot is "from".
	insertRemoteNode(t, conn, "remote-node-1", "Remote Node 1")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	engine := &FederationSyncEngine{
		ctx:       ctx,
		db:        conn,
		peerStops: make(map[string]context.CancelFunc),
	}

	node := &models.Node{
		ID:      "remote-node-1",
		Name:    "Remote Node 1",
		BaseURL: "https://remote-node-1.example.com",
	}

	snapshot := &xylona.FederationServerSnapshot{
		Servers: []*xylona.FederationServerState{
			{
				ServerId:       "server-aaa",
				Status:         xylona.Status_ONLINE,
				DisplayName:    "Minecraft Survival",
				GameName:       "Minecraft",
				GameId:         "game-mc",
				IpAddress:      "10.0.0.1",
				Port:           25565,
				QueryPort:      25566,
				MaxPlayers:     20,
				CurrentPlayers: 5,
				MapName:        "world",
				Version:        "1.20.4",
			},
			{
				ServerId:       "server-bbb",
				Status:         xylona.Status_OFFLINE,
				DisplayName:    "Factorio Server",
				GameName:       "Factorio",
				GameId:         "game-fact",
				IpAddress:      "10.0.0.2",
				Port:           34197,
				QueryPort:      34198,
				MaxPlayers:     10,
				CurrentPlayers: 0,
				MapName:        "nauvis",
				Version:        "1.1.104",
			},
		},
	}

	engine.handleSnapshot(node, snapshot)

	// Query the remote_server_cache table and verify both servers are present.
	rows, errRows := conn.SQLDb.QueryContext(
		context.Background(),
		`SELECT remote_server_id, display_name, status, game_name FROM remote_server_cache ORDER BY remote_server_id`,
	)
	if errRows != nil {
		t.Fatalf("failed to query remote_server_cache: %v", errRows)
	}
	t.Cleanup(func() {
		if errCloseRows := rows.Close(); errCloseRows != nil {
			t.Errorf("failed to close rows: %v", errCloseRows)
		}
	})

	type cacheRow struct {
		remoteServerID string
		displayName    string
		status         string
		gameName       string
	}

	var results []cacheRow
	for rows.Next() {
		var row cacheRow
		errScan := rows.Scan(&row.remoteServerID, &row.displayName, &row.status, &row.gameName)
		if errScan != nil {
			t.Fatalf("failed to scan row: %v", errScan)
		}
		results = append(results, row)
	}
	if errNext := rows.Err(); errNext != nil {
		t.Fatalf("rows iteration error: %v", errNext)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 cached servers, got %d: %+v", len(results), results)
	}

	// Results are ordered by remote_server_id.
	if results[0].remoteServerID != "server-aaa" {
		t.Errorf("results[0].remoteServerID = %q, want %q", results[0].remoteServerID, "server-aaa")
	}
	if results[0].displayName != "Minecraft Survival" {
		t.Errorf("results[0].displayName = %q, want %q", results[0].displayName, "Minecraft Survival")
	}
	if results[0].status != xylona.Status_ONLINE.String() {
		t.Errorf("results[0].status = %q, want %q", results[0].status, xylona.Status_ONLINE.String())
	}
	if results[0].gameName != "Minecraft" {
		t.Errorf("results[0].gameName = %q, want %q", results[0].gameName, "Minecraft")
	}

	if results[1].remoteServerID != "server-bbb" {
		t.Errorf("results[1].remoteServerID = %q, want %q", results[1].remoteServerID, "server-bbb")
	}
	if results[1].displayName != "Factorio Server" {
		t.Errorf("results[1].displayName = %q, want %q", results[1].displayName, "Factorio Server")
	}
	if results[1].status != xylona.Status_OFFLINE.String() {
		t.Errorf("results[1].status = %q, want %q", results[1].status, xylona.Status_OFFLINE.String())
	}
	if results[1].gameName != "Factorio" {
		t.Errorf("results[1].gameName = %q, want %q", results[1].gameName, "Factorio")
	}
}

func TestHandleStatusChange_UpdatesCacheAndBroadcasts(t *testing.T) {
	conn := newSyncEngineTestDB(t)

	// Seed a remote node and a cached server entry.
	insertRemoteNode(t, conn, "remote-node-1", "Remote Node 1")
	insertRemoteServerCache(t, conn, "cache-1", "remote-node-1", "remote-node-1", "server-aaa", "OFFLINE")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	broadcaster := &mockStatusBroadcaster{}

	engine := &FederationSyncEngine{
		ctx:               ctx,
		db:                conn,
		peerStops:         make(map[string]context.CancelFunc),
		statusBroadcaster: broadcaster,
	}

	node := &models.Node{
		ID:   "remote-node-1",
		Name: "Remote Node 1",
	}

	statusChange := &xylona.FederationServerStatusChange{
		ServerId: "server-aaa",
		Status:   xylona.Status_ONLINE,
	}

	engine.handleStatusChange(node, statusChange)

	// Verify the DB cache was updated.
	var updatedStatus string
	errScan := conn.SQLDb.QueryRowContext(
		context.Background(),
		`SELECT status FROM remote_server_cache WHERE source_node_id = ? AND remote_server_id = ?`,
		"remote-node-1", "server-aaa",
	).Scan(&updatedStatus)
	if errScan != nil {
		t.Fatalf("failed to query updated status: %v", errScan)
	}
	if updatedStatus != xylona.Status_ONLINE.String() {
		t.Errorf("updated status = %q, want %q", updatedStatus, xylona.Status_ONLINE.String())
	}

	// Verify the broadcaster was called.
	broadcaster.mu.Lock()
	defer broadcaster.mu.Unlock()
	if len(broadcaster.calls) != 1 {
		t.Fatalf("expected 1 broadcast call, got %d", len(broadcaster.calls))
	}
	if broadcaster.calls[0].serverID != "server-aaa" {
		t.Errorf("broadcast serverID = %q, want %q", broadcaster.calls[0].serverID, "server-aaa")
	}
	if broadcaster.calls[0].status != xylona.Status_ONLINE {
		t.Errorf("broadcast status = %v, want %v", broadcaster.calls[0].status, xylona.Status_ONLINE)
	}
}

func TestHandleMetricsUpdate_CallsBroadcaster(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	broadcaster := &mockMetricsBroadcaster{}

	engine := &FederationSyncEngine{
		ctx:                ctx,
		peerStops:          make(map[string]context.CancelFunc),
		metricsBroadcaster: broadcaster,
	}

	metricsUpdate := &xylona.FederationServerMetricsUpdate{
		ServerId: "server-aaa",
		Metrics: &xylona.GameServerMetrics{
			CpuPercent:  45.2,
			MemoryBytes: 1024 * 1024 * 512,
		},
	}

	engine.handleMetricsUpdate(metricsUpdate)

	// Verify the broadcaster was called.
	broadcaster.mu.Lock()
	defer broadcaster.mu.Unlock()
	if len(broadcaster.calls) != 1 {
		t.Fatalf("expected 1 metrics broadcast call, got %d", len(broadcaster.calls))
	}
	if broadcaster.calls[0].serverID != "server-aaa" {
		t.Errorf("broadcast serverID = %q, want %q", broadcaster.calls[0].serverID, "server-aaa")
	}
	if broadcaster.calls[0].metrics == nil {
		t.Fatalf("broadcast metrics is nil, want non-nil")
	}
	if broadcaster.calls[0].metrics.GetCpuPercent() != 45.2 {
		t.Errorf("broadcast CpuPercent = %f, want %f", broadcaster.calls[0].metrics.GetCpuPercent(), 45.2)
	}
	if broadcaster.calls[0].metrics.GetMemoryBytes() != 1024*1024*512 {
		t.Errorf("broadcast MemoryBytes = %d, want %d", broadcaster.calls[0].metrics.GetMemoryBytes(), 1024*1024*512)
	}
}
