package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
)

func TestFederationLocalIdentityKeyPEMFormatDownMigrationFailsFast(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "rollback-guard.sqlite")

	conn, errNewConnection := NewConnection(context.Background(), dbPath)
	if errNewConnection != nil {
		t.Fatalf("NewConnection() error = %v", errNewConnection)
	}
	t.Cleanup(func() {
		_ = conn.SQLDb.Close()
	})

	migrationSource := &migrate.FileMigrationSource{
		Dir: filepath.Join("..", "sql", "migrations"),
	}
	migrate.SetTable("migrations")

	_, errMigrateUp := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
	if errMigrateUp != nil {
		t.Fatalf("migrate.Exec(Up) error = %v", errMigrateUp)
	}

	_, errMigrateDown := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Down)
	if errMigrateDown == nil {
		t.Fatal("migrate.Exec(Down) error = nil, want unsupported rollback failure")
	}
	if !strings.Contains(errMigrateDown.Error(), "down migration unsupported") {
		t.Fatalf("migrate.Exec(Down) error = %v, want unsupported rollback message", errMigrateDown)
	}
}
