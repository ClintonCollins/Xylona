package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func createRemoteServerCacheLookupTable(t *testing.T, conn *Connection) {
	t.Helper()

	_, errCreate := conn.SQLDb.Exec(`
		CREATE TABLE remote_server_cache (
			id TEXT PRIMARY KEY NOT NULL,
			source_node_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			remote_server_id TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'UNKNOWN',
			game_name TEXT NOT NULL DEFAULT '',
			game_id TEXT NOT NULL DEFAULT '',
			ip_address TEXT NOT NULL DEFAULT '',
			port INTEGER NOT NULL DEFAULT 0,
			query_port INTEGER NOT NULL DEFAULT 0,
			max_players INTEGER NOT NULL DEFAULT 0,
			current_players INTEGER NOT NULL DEFAULT 0,
			map_name TEXT NOT NULL DEFAULT '',
			version TEXT NOT NULL DEFAULT '',
			node_name TEXT NOT NULL DEFAULT '',
			node_host TEXT NOT NULL DEFAULT '',
			last_remote_update DATETIME,
			last_synced_at DATETIME,
			is_stale BOOLEAN NOT NULL DEFAULT FALSE,
			raw_metadata TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if errCreate != nil {
		t.Fatalf("failed to create remote_server_cache table: %v", errCreate)
	}
}

func TestGetRemoteServerCacheByRemoteServerID(t *testing.T) {
	tests := []struct {
		name          string
		insertRowsSQL string
		wantErr       error
		wantID        string
	}{
		{
			name: "returns matching row",
			insertRowsSQL: `
				INSERT INTO remote_server_cache (id, source_node_id, node_id, remote_server_id)
				VALUES ('cache-1', 'source-1', 'node-1', 'server-a')
			`,
			wantID: "cache-1",
		},
		{
			name:          "returns not found when no row exists",
			insertRowsSQL: "",
			wantErr:       sql.ErrNoRows,
		},
		{
			name: "returns ambiguity error when multiple rows match",
			insertRowsSQL: `
				INSERT INTO remote_server_cache (id, source_node_id, node_id, remote_server_id)
				VALUES
					('cache-1', 'source-1', 'node-1', 'server-a'),
					('cache-2', 'source-2', 'node-2', 'server-a')
			`,
			wantErr: ErrAmbiguousRemoteServerCache,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "remote-server-cache-lookup.sqlite")
			conn := NewConnection(context.Background(), dbPath)
			t.Cleanup(func() {
				if errClose := conn.SQLDb.Close(); errClose != nil {
					t.Errorf("failed to close db: %v", errClose)
				}
			})

			createRemoteServerCacheLookupTable(t, conn)

			if tt.insertRowsSQL != "" {
				_, errInsert := conn.SQLDb.Exec(tt.insertRowsSQL)
				if errInsert != nil {
					t.Fatalf("failed to insert test rows: %v", errInsert)
				}
			}

			cache, errGet := conn.GetRemoteServerCacheByRemoteServerID("server-a")
			if tt.wantErr != nil {
				if !errors.Is(errGet, tt.wantErr) {
					t.Fatalf("GetRemoteServerCacheByRemoteServerID() error = %v, want %v", errGet, tt.wantErr)
				}
				return
			}

			if errGet != nil {
				t.Fatalf("GetRemoteServerCacheByRemoteServerID() error = %v", errGet)
			}
			if cache.ID != tt.wantID {
				t.Errorf("cache.ID = %q, want %q", cache.ID, tt.wantID)
			}
		})
	}
}

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

func TestUpdateRemoteServerCacheStatusMarksRowFresh(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "remote-server-cache-update-status.sqlite")
	conn := NewConnection(context.Background(), dbPath)
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close db: %v", errClose)
		}
	})

	_, errCreate := conn.SQLDb.Exec(`
		CREATE TABLE remote_server_cache (
			source_node_id TEXT NOT NULL,
			remote_server_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'UNKNOWN',
			last_synced_at DATETIME,
			is_stale BOOLEAN NOT NULL DEFAULT TRUE,
			updated_at DATETIME NOT NULL
		)
	`)
	if errCreate != nil {
		t.Fatalf("failed to create table: %v", errCreate)
	}

	previousUpdate := time.Now().Add(-10 * time.Minute)
	_, errInsert := conn.SQLDb.Exec(`
		INSERT INTO remote_server_cache (source_node_id, remote_server_id, status, last_synced_at, is_stale, updated_at)
		VALUES
			('source-1', 'server-a', 'OFFLINE', NULL, 1, ?),
			('source-1', 'server-b', 'OFFLINE', NULL, 1, ?)
	`, previousUpdate, previousUpdate)
	if errInsert != nil {
		t.Fatalf("failed to insert rows: %v", errInsert)
	}

	errUpdate := conn.UpdateRemoteServerCacheStatus("source-1", "server-a", "ONLINE")
	if errUpdate != nil {
		t.Fatalf("UpdateRemoteServerCacheStatus() error = %v", errUpdate)
	}

	var (
		status       string
		lastSyncedAt sql.NullTime
		isStale      bool
		updatedAt    time.Time
	)

	errQueryUpdated := conn.SQLDb.QueryRow(`
		SELECT status, last_synced_at, is_stale, updated_at
		FROM remote_server_cache
		WHERE source_node_id = ? AND remote_server_id = ?
	`, "source-1", "server-a").Scan(&status, &lastSyncedAt, &isStale, &updatedAt)
	if errQueryUpdated != nil {
		t.Fatalf("failed to query updated row: %v", errQueryUpdated)
	}

	if status != "ONLINE" {
		t.Errorf("updated row status = %q, want %q", status, "ONLINE")
	}
	if !lastSyncedAt.Valid {
		t.Fatalf("updated row last_synced_at is NULL, want non-NULL")
	}
	if isStale {
		t.Errorf("updated row is_stale = true, want false")
	}
	if !updatedAt.After(previousUpdate) {
		t.Errorf("updated row updated_at = %v, want after %v", updatedAt, previousUpdate)
	}

	var (
		unchangedStatus       string
		unchangedLastSyncedAt sql.NullTime
		unchangedIsStale      bool
	)

	errQueryUnchanged := conn.SQLDb.QueryRow(`
		SELECT status, last_synced_at, is_stale
		FROM remote_server_cache
		WHERE source_node_id = ? AND remote_server_id = ?
	`, "source-1", "server-b").Scan(&unchangedStatus, &unchangedLastSyncedAt, &unchangedIsStale)
	if errQueryUnchanged != nil {
		t.Fatalf("failed to query unchanged row: %v", errQueryUnchanged)
	}

	if unchangedStatus != "OFFLINE" {
		t.Errorf("unchanged row status = %q, want %q", unchangedStatus, "OFFLINE")
	}
	if unchangedLastSyncedAt.Valid {
		t.Errorf("unchanged row last_synced_at = %v, want NULL", unchangedLastSyncedAt.Time)
	}
	if !unchangedIsStale {
		t.Errorf("unchanged row is_stale = false, want true")
	}
}
