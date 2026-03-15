package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDeleteRemoteServerCacheByCompositeKey(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "remote-server-cache.sqlite")
	conn := NewConnection(context.Background(), dbPath)
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close db: %v", errClose)
		}
	})

	_, errCreate := conn.SQLDb.Exec(`
		CREATE TABLE remote_server_cache (
			source_node_id TEXT NOT NULL,
			remote_server_id TEXT NOT NULL
		)
	`)
	if errCreate != nil {
		t.Fatalf("failed to create table: %v", errCreate)
	}

	_, errInsert := conn.SQLDb.Exec(`
		INSERT INTO remote_server_cache (source_node_id, remote_server_id)
		VALUES
			('node-1', 'server-a'),
			('node-1', 'server-b')
	`)
	if errInsert != nil {
		t.Fatalf("failed to insert rows: %v", errInsert)
	}

	errDelete := conn.DeleteRemoteServerCacheByCompositeKey("node-1", "server-a")
	if errDelete != nil {
		t.Fatalf("DeleteRemoteServerCacheByCompositeKey() error = %v", errDelete)
	}

	var count int
	errCount := conn.SQLDb.QueryRow(`SELECT COUNT(*) FROM remote_server_cache`).Scan(&count)
	if errCount != nil {
		t.Fatalf("failed to count rows: %v", errCount)
	}
	if count != 1 {
		t.Errorf("remaining row count = %d, want %d", count, 1)
	}

	var remainingRemoteServerID string
	errRemaining := conn.SQLDb.QueryRow(`SELECT remote_server_id FROM remote_server_cache LIMIT 1`).Scan(&remainingRemoteServerID)
	if errRemaining != nil {
		t.Fatalf("failed to fetch remaining row: %v", errRemaining)
	}
	if remainingRemoteServerID != "server-b" {
		t.Errorf("remaining remote_server_id = %q, want %q", remainingRemoteServerID, "server-b")
	}
}

func TestDeleteOrphanedRemoteServerCacheByNodeReferences(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "remote-server-cache-orphan-cleanup.sqlite")
	conn := NewConnection(context.Background(), dbPath)
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close db: %v", errClose)
		}
	})

	_, errCreateNode := conn.SQLDb.Exec(`
		CREATE TABLE node (
			id TEXT PRIMARY KEY NOT NULL,
			is_local BOOLEAN NOT NULL DEFAULT FALSE
		)
	`)
	if errCreateNode != nil {
		t.Fatalf("failed to create node table: %v", errCreateNode)
	}

	_, errCreateCache := conn.SQLDb.Exec(`
		CREATE TABLE remote_server_cache (
			id TEXT PRIMARY KEY NOT NULL,
			source_node_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			remote_server_id TEXT NOT NULL
		)
	`)
	if errCreateCache != nil {
		t.Fatalf("failed to create remote_server_cache table: %v", errCreateCache)
	}

	_, errInsertNodes := conn.SQLDb.Exec(`
		INSERT INTO node (id, is_local) VALUES
			('remote-1', 0),
			('remote-2', 0),
			('local-1', 1)
	`)
	if errInsertNodes != nil {
		t.Fatalf("failed to insert node rows: %v", errInsertNodes)
	}

	_, errInsertCache := conn.SQLDb.Exec(`
		INSERT INTO remote_server_cache (id, source_node_id, node_id, remote_server_id) VALUES
			('cache-1', 'remote-1', 'remote-1', 'server-valid'),
			('cache-2', 'remote-1', 'missing-node', 'server-orphan-node'),
			('cache-3', 'missing-node', 'remote-1', 'server-orphan-source'),
			('cache-4', 'local-1', 'local-1', 'server-local-ref')
	`)
	if errInsertCache != nil {
		t.Fatalf("failed to insert remote server cache rows: %v", errInsertCache)
	}

	errDelete := conn.DeleteOrphanedRemoteServerCacheByNodeReferences()
	if errDelete != nil {
		t.Fatalf("DeleteOrphanedRemoteServerCacheByNodeReferences() error = %v", errDelete)
	}

	rows, errRows := conn.SQLDb.Query(`SELECT remote_server_id FROM remote_server_cache ORDER BY remote_server_id`)
	if errRows != nil {
		t.Fatalf("failed to query remaining remote_server_cache rows: %v", errRows)
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
			t.Fatalf("failed to scan remaining row: %v", errScan)
		}
		remaining = append(remaining, serverID)
	}
	if errRowsNext := rows.Err(); errRowsNext != nil {
		t.Fatalf("failed iterating rows: %v", errRowsNext)
	}

	if len(remaining) != 1 {
		t.Fatalf("remaining row count = %d, want %d (rows: %v)", len(remaining), 1, remaining)
	}
	if remaining[0] != "server-valid" {
		t.Errorf("remaining remote_server_id = %q, want %q", remaining[0], "server-valid")
	}
}
