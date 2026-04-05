// Package dbtest provides migrated SQLite helpers for database tests.
package dbtest

import (
	"context"
	"path/filepath"
	"testing"

	migrate "github.com/rubenv/sql-migrate"

	"github.com/ClintonCollins/Xylona/db"
)

// NewMigratedConnection creates a temporary SQLite database with all migrations
// applied. The database is automatically closed when the test completes.
// This is intended for use by test packages outside db/ that need a fully
// migrated schema without manually creating tables.
func NewMigratedConnection(t *testing.T, sqliteFileName string) *db.Connection {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), sqliteFileName)
	conn := db.NewConnection(context.Background(), dbPath)
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close test database: %v", errClose)
		}
	})

	migrationSource := &migrate.FileMigrationSource{
		Dir: migrationsDir(t),
	}
	migrate.SetTable("migrations")
	_, errMigrate := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
	if errMigrate != nil {
		t.Fatalf("failed to apply migrations: %v", errMigrate)
	}

	return conn
}

// migrationsDir locates the sql/migrations directory relative to the test's
// working directory. Tests in different packages run from different directories,
// so we walk up looking for the canonical path.
func migrationsDir(t *testing.T) string {
	t.Helper()

	// All packages in this repo are at most 2 levels deep from the root.
	candidates := []string{
		filepath.Join("..", "sql", "migrations"),
		filepath.Join("..", "..", "sql", "migrations"),
		filepath.Join("sql", "migrations"),
	}
	for _, candidate := range candidates {
		abs, errAbs := filepath.Abs(candidate)
		if errAbs != nil {
			continue
		}
		matches, errGlob := filepath.Glob(filepath.Join(abs, "*.sql"))
		if errGlob != nil {
			continue
		}
		if len(matches) > 0 {
			return abs
		}
	}

	t.Fatalf("could not locate sql/migrations directory")
	return ""
}
