package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLiteDSNWithPragma(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		pragma string
		want   string
	}{
		{
			name:   "adds pragma to bare path",
			path:   "test.sqlite",
			pragma: "foreign_keys(1)",
			want:   "test.sqlite?_pragma=foreign_keys%281%29",
		},
		{
			name:   "adds pragma to existing query string",
			path:   "test.sqlite?cache=shared",
			pragma: "foreign_keys(1)",
			want:   "test.sqlite?cache=shared&_pragma=foreign_keys%281%29",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sqliteDSNWithPragmas(tt.path, tt.pragma)
			if got != tt.want {
				t.Errorf("sqliteDSNWithPragmas() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewConnectionEnablesForeignKeys(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "foreign-keys.sqlite")
	conn, errNewConnection := NewConnection(context.Background(), dbPath)
	if errNewConnection != nil {
		t.Fatalf("NewConnection() error = %v", errNewConnection)
	}
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close db: %v", errClose)
		}
	})

	var foreignKeysEnabled int
	errForeignKeys := conn.SQLDb.QueryRowContext(conn.ctx, `PRAGMA foreign_keys`).Scan(&foreignKeysEnabled)
	if errForeignKeys != nil {
		t.Fatalf("failed to query PRAGMA foreign_keys: %v", errForeignKeys)
	}
	if foreignKeysEnabled != 1 {
		t.Fatalf("PRAGMA foreign_keys = %d, want 1", foreignKeysEnabled)
	}

	_, errCreateParent := conn.SQLDb.ExecContext(conn.ctx, `CREATE TABLE parent (id TEXT PRIMARY KEY NOT NULL)`)
	if errCreateParent != nil {
		t.Fatalf("failed to create parent table: %v", errCreateParent)
	}
	_, errCreateChild := conn.SQLDb.ExecContext(conn.ctx, `CREATE TABLE child (parent_id TEXT REFERENCES parent (id))`)
	if errCreateChild != nil {
		t.Fatalf("failed to create child table: %v", errCreateChild)
	}

	_, errInsertChild := conn.SQLDb.ExecContext(conn.ctx, `INSERT INTO child (parent_id) VALUES ('missing-parent')`)
	if errInsertChild == nil {
		t.Fatalf("expected foreign key insert to fail, but it succeeded")
	}

	if !strings.Contains(strings.ToUpper(errInsertChild.Error()), "FOREIGN KEY") {
		t.Errorf("expected foreign key error, got: %v", errInsertChild)
	}
}

func TestNewConnectionReturnsErrorForInvalidPath(t *testing.T) {
	dbPath := t.TempDir()

	conn, errNewConnection := NewConnection(context.Background(), dbPath)
	if errNewConnection == nil {
		t.Fatal("NewConnection() error = nil, want error")
	}
	if conn != nil {
		t.Fatalf("NewConnection() connection = %+v, want nil", conn)
	}
}
