package rpc

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	migrate "github.com/rubenv/sql-migrate"

	"github.com/ClintonCollins/Xylona/db"
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

		migrationSource := &migrate.FileMigrationSource{
			Dir: filepath.Join("..", "..", "sql", "migrations"),
		}
		rpcSetMigrationTableOnce.Do(func() { migrate.SetTable("migrations") })
		_, errMigrate := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
		if errMigrate != nil {
			rpcTemplateErr = errMigrate
		}

		_, errAlterGame := conn.SQLDb.ExecContext(
			context.Background(),
			`alter table game add column binds_to_all_ips boolean not null default false`,
		)
		if errAlterGame != nil {
			isDuplicateColumnErr := strings.Contains(strings.ToLower(errAlterGame.Error()), "duplicate column name")
			if !isDuplicateColumnErr {
				rpcTemplateErr = errAlterGame
			}
		}

		errCloseConn := conn.SQLDb.Close()
		if errCloseConn != nil && rpcTemplateErr == nil {
			rpcTemplateErr = errCloseConn
		}
	})

	return rpcTemplatePath, rpcTemplateErr
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
