package actions

import (
	"context"
	"path/filepath"
	"testing"

	migrate "github.com/rubenv/sql-migrate"

	"github.com/ClintonCollins/Xylona/db"
)

func newTestInstance(t *testing.T) *Instance {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test-actions.sqlite")
	conn := db.NewConnection(context.Background(), dbPath)
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close test database: %v", errClose)
		}
	})

	migrationSource := &migrate.FileMigrationSource{
		Dir: filepath.Join("..", "sql", "migrations"),
	}
	migrate.SetTable("migrations")
	_, errMigrate := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
	if errMigrate != nil {
		t.Fatalf("failed to apply migrations: %v", errMigrate)
	}

	return NewInstance(context.Background(), conn, nil, nil, nil)
}
