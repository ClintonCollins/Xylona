// Package dbtest provides migrated SQLite helpers for database tests.
package dbtest

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	migrate "github.com/rubenv/sql-migrate"

	"github.com/ClintonCollins/Xylona/db"
)

var migratedTemplateOnce sync.Once
var migratedTemplatePath string
var migratedTemplateErr error
var setMigrationTableOnce sync.Once

// NewMigratedConnection creates a temporary SQLite database with all migrations
// applied. The database is automatically closed when the test completes.
// This is intended for use by test packages outside db/ that need a fully
// migrated schema without manually creating tables.
func NewMigratedConnection(t *testing.T, sqliteFileName string) *db.Connection {
	t.Helper()

	templatePath, errTemplate := ensureMigratedTemplate(t)
	if errTemplate != nil {
		t.Fatalf("failed to create migrated sqlite template: %v", errTemplate)
	}

	dbPath := filepath.Join(t.TempDir(), sqliteFileName)
	errCopy := copySQLiteTemplate(templatePath, dbPath)
	if errCopy != nil {
		t.Fatalf("failed to copy migrated sqlite template: %v", errCopy)
	}

	conn := db.NewConnection(context.Background(), dbPath)
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close test database: %v", errClose)
		}
	})

	return conn
}

func ensureMigratedTemplate(t *testing.T) (string, error) {
	t.Helper()

	migratedTemplateOnce.Do(func() {
		//nolint:usetesting // shared template must outlive any individual test TempDir cleanup.
		templateFile, errCreate := os.CreateTemp("", "xylona-dbtest-template-*.sqlite")
		if errCreate != nil {
			migratedTemplateErr = fmt.Errorf("create template file: %w", errCreate)
			return
		}
		migratedTemplatePath = templateFile.Name()

		errCloseTemplate := templateFile.Close()
		if errCloseTemplate != nil {
			migratedTemplateErr = fmt.Errorf("close template file: %w", errCloseTemplate)
			return
		}

		conn := db.NewConnection(context.Background(), migratedTemplatePath)
		migrationSource := &migrate.FileMigrationSource{
			Dir: migrationsDir(t),
		}
		setMigrationTableOnce.Do(func() { migrate.SetTable("migrations") })

		_, errMigrate := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
		if errMigrate != nil {
			migratedTemplateErr = fmt.Errorf("apply migrations to template db: %w", errMigrate)
		}

		errCloseConn := conn.SQLDb.Close()
		if errCloseConn != nil && migratedTemplateErr == nil {
			migratedTemplateErr = fmt.Errorf("close template db connection: %w", errCloseConn)
		}
	})

	return migratedTemplatePath, migratedTemplateErr
}

func copySQLiteTemplate(sourcePath string, destinationPath string) error {
	sourceFile, errSource := os.Open(sourcePath)
	if errSource != nil {
		return fmt.Errorf("open source sqlite template: %w", errSource)
	}
	defer func() {
		_ = sourceFile.Close()
	}()

	destinationFile, errDestination := os.Create(destinationPath)
	if errDestination != nil {
		return fmt.Errorf("create destination sqlite db: %w", errDestination)
	}
	defer func() {
		_ = destinationFile.Close()
	}()

	_, errCopy := io.Copy(destinationFile, sourceFile)
	if errCopy != nil {
		return fmt.Errorf("copy sqlite template: %w", errCopy)
	}

	errSync := destinationFile.Sync()
	if errSync != nil {
		return fmt.Errorf("sync destination sqlite db: %w", errSync)
	}

	return nil
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
