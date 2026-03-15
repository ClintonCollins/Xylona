package db

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"
)

type sqlExecContext interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func createPeerSyncStateTestSchema(t *testing.T, exec sqlExecContext) {
	t.Helper()

	_, errCreateNode := exec.ExecContext(
		context.Background(),
		`CREATE TABLE node (
			id TEXT PRIMARY KEY NOT NULL
		)`,
	)
	if errCreateNode != nil {
		t.Fatalf("failed to create node table: %v", errCreateNode)
	}

	_, errCreateSyncState := exec.ExecContext(
		context.Background(),
		`CREATE TABLE peer_sync_state (
			id TEXT PRIMARY KEY NOT NULL,
			node_id TEXT NOT NULL REFERENCES node (id) ON DELETE CASCADE,
			last_cursor TEXT NOT NULL DEFAULT '',
			last_full_sync_at DATETIME,
			last_delta_sync_at DATETIME,
			last_error TEXT NOT NULL DEFAULT '',
			retry_count INTEGER NOT NULL DEFAULT 0,
			next_retry_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	)
	if errCreateSyncState != nil {
		t.Fatalf("failed to create peer_sync_state table: %v", errCreateSyncState)
	}

	_, errUniqueIndex := exec.ExecContext(
		context.Background(),
		`CREATE UNIQUE INDEX peer_sync_state_node_id_unique ON peer_sync_state (node_id)`,
	)
	if errUniqueIndex != nil {
		t.Fatalf("failed to create unique index: %v", errUniqueIndex)
	}
}

func TestGetOrCreatePeerSyncStateCreatesAndReusesRow(t *testing.T) {
	t.Parallel()

	conn := newTestConnection(t, "peer-sync-state-create.sqlite")

	withConnectionTxRollback(t, conn, func(t *testing.T, tx *sql.Tx) {
		createPeerSyncStateTestSchema(t, tx)

		_, errInsertNode := tx.ExecContext(context.Background(), `INSERT INTO node (id) VALUES (?)`, "node-1")
		if errInsertNode != nil {
			t.Fatalf("failed to insert node row: %v", errInsertNode)
		}

		first, errFirst := conn.GetOrCreatePeerSyncState("node-1")
		if errFirst != nil {
			t.Fatalf("GetOrCreatePeerSyncState() first call error = %v", errFirst)
		}
		if first.NodeID != "node-1" {
			t.Fatalf("first.NodeID = %q, want %q", first.NodeID, "node-1")
		}

		second, errSecond := conn.GetOrCreatePeerSyncState("node-1")
		if errSecond != nil {
			t.Fatalf("GetOrCreatePeerSyncState() second call error = %v", errSecond)
		}

		if first.ID != second.ID {
			t.Fatalf("expected the same sync-state row ID, got first=%q second=%q", first.ID, second.ID)
		}

		var count int
		errCount := tx.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM peer_sync_state WHERE node_id = ?`, "node-1").Scan(&count)
		if errCount != nil {
			t.Fatalf("failed to count peer_sync_state rows: %v", errCount)
		}
		if count != 1 {
			t.Fatalf("peer_sync_state rows = %d, want 1", count)
		}
	})
}

func TestGetOrCreatePeerSyncStateConcurrentCallsReuseSingleRow(t *testing.T) {
	t.Parallel()

	conn := newTestConnection(t, "peer-sync-state-concurrency.sqlite")
	conn.SQLDb.SetMaxOpenConns(1)
	conn.SQLDb.SetMaxIdleConns(1)

	createPeerSyncStateTestSchema(t, conn.SQLDb)
	_, errBusyTimeout := conn.SQLDb.ExecContext(context.Background(), `PRAGMA busy_timeout = 5000`)
	if errBusyTimeout != nil {
		t.Fatalf("failed to set busy timeout pragma: %v", errBusyTimeout)
	}

	_, errInsertNode := conn.SQLDb.ExecContext(context.Background(), `INSERT INTO node (id) VALUES (?)`, "node-1")
	if errInsertNode != nil {
		t.Fatalf("failed to insert node row: %v", errInsertNode)
	}

	const workers = 32

	var wg sync.WaitGroup
	wg.Add(workers)

	start := make(chan struct{})
	errCh := make(chan error, workers)
	idCh := make(chan string, workers)

	for range workers {
		go func() {
			defer wg.Done()
			<-start

			syncState, errGetOrCreate := conn.GetOrCreatePeerSyncState("node-1")
			if errGetOrCreate != nil {
				errCh <- errGetOrCreate
				return
			}
			idCh <- syncState.ID
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)
	close(idCh)

	for errGetOrCreate := range errCh {
		if errGetOrCreate != nil {
			t.Fatalf("GetOrCreatePeerSyncState() concurrent call returned error: %v", errGetOrCreate)
		}
	}

	var firstID string
	firstIDSet := false
	for id := range idCh {
		if !firstIDSet {
			firstID = id
			firstIDSet = true
			continue
		}
		if id != firstID {
			t.Fatalf("expected all concurrent calls to return the same row ID; got %q and %q", firstID, id)
		}
	}

	var count int
	errCount := conn.SQLDb.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM peer_sync_state WHERE node_id = ?`, "node-1").Scan(&count)
	if errCount != nil {
		t.Fatalf("failed to count peer_sync_state rows: %v", errCount)
	}
	if count != 1 {
		t.Fatalf("peer_sync_state rows = %d, want 1", count)
	}
}

func TestUpdatePeerSyncStateErrorAndSuccess(t *testing.T) {
	t.Parallel()

	conn := newTestConnection(t, "peer-sync-state-update.sqlite")

	withConnectionTxRollback(t, conn, func(t *testing.T, tx *sql.Tx) {
		createPeerSyncStateTestSchema(t, tx)

		_, errInsertNode := tx.ExecContext(context.Background(), `INSERT INTO node (id) VALUES (?)`, "node-1")
		if errInsertNode != nil {
			t.Fatalf("failed to insert node row: %v", errInsertNode)
		}

		_, errInsertSyncState := tx.ExecContext(
			context.Background(),
			`INSERT INTO peer_sync_state (id, node_id, last_error, retry_count) VALUES (?, ?, ?, ?)`,
			"sync-1",
			"node-1",
			"",
			0,
		)
		if errInsertSyncState != nil {
			t.Fatalf("failed to insert peer_sync_state row: %v", errInsertSyncState)
		}

		nextRetryAt := time.Now().UTC().Add(2 * time.Minute)
		errUpdateError := conn.UpdatePeerSyncStateError("node-1", "sync failed", 3, nextRetryAt)
		if errUpdateError != nil {
			t.Fatalf("UpdatePeerSyncStateError() error = %v", errUpdateError)
		}

		var (
			gotLastError string
			gotRetry     int
			gotNextRetry sql.NullTime
		)
		errQueryAfterError := tx.QueryRowContext(
			context.Background(),
			`SELECT last_error, retry_count, next_retry_at FROM peer_sync_state WHERE node_id = ?`,
			"node-1",
		).Scan(&gotLastError, &gotRetry, &gotNextRetry)
		if errQueryAfterError != nil {
			t.Fatalf("failed to query peer_sync_state after error update: %v", errQueryAfterError)
		}

		if gotLastError != "sync failed" {
			t.Fatalf("last_error = %q, want %q", gotLastError, "sync failed")
		}
		if gotRetry != 3 {
			t.Fatalf("retry_count = %d, want %d", gotRetry, 3)
		}
		if !gotNextRetry.Valid {
			t.Fatalf("next_retry_at should be set after UpdatePeerSyncStateError()")
		}

		errUpdateSuccess := conn.UpdatePeerSyncStateSuccess("node-1", "cursor-123")
		if errUpdateSuccess != nil {
			t.Fatalf("UpdatePeerSyncStateSuccess() error = %v", errUpdateSuccess)
		}

		var (
			gotCursor       string
			gotLastError2   string
			gotRetry2       int
			gotNextRetry2   sql.NullTime
			gotLastFullSync sql.NullTime
		)
		errQueryAfterSuccess := tx.QueryRowContext(
			context.Background(),
			`SELECT last_cursor, last_error, retry_count, next_retry_at, last_full_sync_at
			 FROM peer_sync_state
			 WHERE node_id = ?`,
			"node-1",
		).Scan(&gotCursor, &gotLastError2, &gotRetry2, &gotNextRetry2, &gotLastFullSync)
		if errQueryAfterSuccess != nil {
			t.Fatalf("failed to query peer_sync_state after success update: %v", errQueryAfterSuccess)
		}

		if gotCursor != "cursor-123" {
			t.Fatalf("last_cursor = %q, want %q", gotCursor, "cursor-123")
		}
		if gotLastError2 != "" {
			t.Fatalf("last_error after success = %q, want empty string", gotLastError2)
		}
		if gotRetry2 != 0 {
			t.Fatalf("retry_count after success = %d, want 0", gotRetry2)
		}
		if gotNextRetry2.Valid {
			t.Fatalf("next_retry_at should be NULL after UpdatePeerSyncStateSuccess()")
		}
		if !gotLastFullSync.Valid {
			t.Fatalf("last_full_sync_at should be set after UpdatePeerSyncStateSuccess()")
		}
	})
}
