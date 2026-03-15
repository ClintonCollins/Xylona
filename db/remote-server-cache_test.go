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
