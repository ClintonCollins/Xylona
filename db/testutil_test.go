package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stephenafamo/scan"
)

func withTxRollback(t *testing.T, db *sql.DB, fn func(t *testing.T, tx *sql.Tx)) {
	t.Helper()

	tx, errBegin := db.BeginTx(context.Background(), nil)
	if errBegin != nil {
		t.Fatalf("failed to begin transaction: %v", errBegin)
	}

	t.Cleanup(func() {
		if errRollback := tx.Rollback(); errRollback != nil && !errors.Is(errRollback, sql.ErrTxDone) {
			t.Errorf("failed to rollback transaction: %v", errRollback)
		}
	})

	fn(t, tx)
}

func withConnectionTxRollback(t *testing.T, conn *Connection, fn func(t *testing.T, tx *sql.Tx)) {
	t.Helper()

	withTxRollback(t, conn.SQLDb, func(t *testing.T, tx *sql.Tx) {
		originalExecutor := conn.DB
		conn.DB = txBobExecutor{tx: tx}

		t.Cleanup(func() {
			conn.DB = originalExecutor
		})

		fn(t, tx)
	})
}

type txBobExecutor struct {
	tx *sql.Tx
}

func (e txBobExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return e.tx.ExecContext(ctx, query, args...)
}

func (e txBobExecutor) QueryContext(ctx context.Context, query string, args ...any) (scan.Rows, error) {
	return e.tx.QueryContext(ctx, query, args...)
}
