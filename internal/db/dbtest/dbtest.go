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

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/gamedefinitions"
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

	return openMigratedConnection(t, sqliteFileName, true)
}

// NewMigratedSchemaConnection creates a temporary SQLite database with only
// migrations applied. Use it for tests that specifically exercise startup data
// synchronization.
func NewMigratedSchemaConnection(t *testing.T, sqliteFileName string) *db.Connection {
	t.Helper()

	return openMigratedConnection(t, sqliteFileName, false)
}

func openMigratedConnection(t *testing.T, sqliteFileName string, syncOfficialDefinitions bool) *db.Connection {
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

	conn, errNewConnection := db.NewConnection(context.Background(), dbPath)
	if errNewConnection != nil {
		t.Fatalf("failed to create test database: %v", errNewConnection)
	}
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close test database: %v", errClose)
		}
	})

	if syncOfficialDefinitions {
		_, errSyncDefinitions := gamedefinitions.SyncOfficialDefinitions(conn)
		if errSyncDefinitions != nil {
			t.Fatalf("failed to sync official game definitions: %v", errSyncDefinitions)
		}
	}

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

		conn, errNewConnection := db.NewConnection(context.Background(), migratedTemplatePath)
		if errNewConnection != nil {
			migratedTemplateErr = fmt.Errorf("open template db connection: %w", errNewConnection)
			return
		}
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

// migrationsDir locates the sql/migrations directory by walking up from the
// test package working directory.
func migrationsDir(t *testing.T) string {
	t.Helper()

	workingDir, errWorkingDir := os.Getwd()
	if errWorkingDir != nil {
		t.Fatalf("get working directory: %v", errWorkingDir)
	}

	for dir := workingDir; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "sql", "migrations")
		matches, errGlob := filepath.Glob(filepath.Join(candidate, "*.sql"))
		if errGlob == nil && len(matches) > 0 {
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	t.Fatalf("could not locate sql/migrations directory")
	return ""
}
