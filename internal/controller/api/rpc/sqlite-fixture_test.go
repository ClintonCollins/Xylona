package rpc

import (
	"context"
	"errors"
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

var rpcTemplateOnce sync.Once
var rpcTemplatePath string
var rpcTemplateErr error
var rpcSetMigrationTableOnce sync.Once

func newRPCFixtureConnection(t *testing.T, sqliteFileName string) *db.Connection {
	t.Helper()

	templatePath, errTemplate := ensureRPCMigratedTemplate()
	if errTemplate != nil {
		t.Fatalf("failed to create migrated rpc sqlite template: %v", errTemplate)
	}

	dbPath := filepath.Join(t.TempDir(), sqliteFileName)
	errCopy := copyRPCSQLiteDB(templatePath, dbPath)
	if errCopy != nil {
		t.Fatalf("failed to copy rpc sqlite template: %v", errCopy)
	}

	conn, errNewConnection := db.NewConnection(context.Background(), dbPath)
	if errNewConnection != nil {
		t.Fatalf("failed to create test database: %v", errNewConnection)
	}
	t.Cleanup(func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil {
			t.Errorf("failed to close test db: %v", errClose)
		}
	})

	return conn
}

func ensureRPCMigratedTemplate() (string, error) {
	rpcTemplateOnce.Do(func() {
		templateFile, errCreate := os.CreateTemp("", "xylona-rpc-template-*.sqlite")
		if errCreate != nil {
			rpcTemplateErr = errCreate
			return
		}
		rpcTemplatePath = templateFile.Name()

		errCloseTemplate := templateFile.Close()
		if errCloseTemplate != nil {
			rpcTemplateErr = errCloseTemplate
			return
		}

		conn, errNewConnection := db.NewConnection(context.Background(), rpcTemplatePath)
		if errNewConnection != nil {
			rpcTemplateErr = errNewConnection
			return
		}

		migrationsDir, errMigrationsDir := rpcMigrationsDir()
		if errMigrationsDir != nil {
			rpcTemplateErr = errMigrationsDir
			return
		}
		migrationSource := &migrate.FileMigrationSource{
			Dir: migrationsDir,
		}
		rpcSetMigrationTableOnce.Do(func() { migrate.SetTable("migrations") })
		_, errMigrate := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
		if errMigrate != nil {
			rpcTemplateErr = errMigrate
		}
		if rpcTemplateErr == nil {
			_, errSyncDefinitions := gamedefinitions.SyncOfficialDefinitions(conn)
			if errSyncDefinitions != nil {
				rpcTemplateErr = errSyncDefinitions
			}
		}

		errCloseConn := conn.SQLDb.Close()
		if errCloseConn != nil && rpcTemplateErr == nil {
			rpcTemplateErr = errCloseConn
		}
	})

	return rpcTemplatePath, rpcTemplateErr
}

func rpcMigrationsDir() (string, error) {
	workingDir, errWorkingDir := os.Getwd()
	if errWorkingDir != nil {
		return "", fmt.Errorf("get working directory: %w", errWorkingDir)
	}

	for dir := workingDir; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "sql", "migrations")
		matches, errGlob := filepath.Glob(filepath.Join(candidate, "*.sql"))
		if errGlob == nil && len(matches) > 0 {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	return "", errors.New("locate sql/migrations directory")
}

func copyRPCSQLiteDB(sourcePath string, destinationPath string) error {
	sourceFile, errSource := os.Open(sourcePath)
	if errSource != nil {
		return fmt.Errorf("open source sqlite file: %w", errSource)
	}
	defer func() {
		_ = sourceFile.Close()
	}()

	destinationFile, errDestination := os.Create(destinationPath)
	if errDestination != nil {
		return fmt.Errorf("create destination sqlite file: %w", errDestination)
	}
	defer func() {
		_ = destinationFile.Close()
	}()

	_, errCopy := io.Copy(destinationFile, sourceFile)
	if errCopy != nil {
		return fmt.Errorf("copy sqlite template file: %w", errCopy)
	}

	errSync := destinationFile.Sync()
	if errSync != nil {
		return fmt.Errorf("sync destination sqlite file: %w", errSync)
	}

	return nil
}
