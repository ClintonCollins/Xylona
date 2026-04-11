package db

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	migrate "github.com/rubenv/sql-migrate"
)

// setTableOnce guards the global migrate.SetTable call, which would race if
// multiple tests call newRBACMigratedConnection concurrently.
var setTableOnce sync.Once
var rbacTemplateOnce sync.Once
var rbacTemplatePath string
var rbacTemplateErr error

func newRBACMigratedConnection(t *testing.T, sqliteFileName string) *Connection {
	t.Helper()

	templatePath, errTemplate := ensureRBACMigratedTemplate()
	if errTemplate != nil {
		t.Fatalf("failed to create migrated sqlite template: %v", errTemplate)
	}

	dbPath := filepath.Join(t.TempDir(), sqliteFileName)
	errCopy := copyTestSQLiteDB(templatePath, dbPath)
	if errCopy != nil {
		t.Fatalf("failed to copy migrated sqlite template: %v", errCopy)
	}

	conn, errNewConnection := NewConnection(context.Background(), dbPath)
	if errNewConnection != nil {
		t.Fatalf("failed to create test database: %v", errNewConnection)
	}
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close test database: %v", errClose)
		}
	})

	return conn
}

func ensureRBACMigratedTemplate() (string, error) {
	rbacTemplateOnce.Do(func() {
		templateFile, errCreate := os.CreateTemp("", "xylona-db-rbac-template-*.sqlite")
		if errCreate != nil {
			rbacTemplateErr = errCreate
			return
		}
		rbacTemplatePath = templateFile.Name()

		errCloseTemplate := templateFile.Close()
		if errCloseTemplate != nil {
			rbacTemplateErr = errCloseTemplate
			return
		}

		conn, errNewConnection := NewConnection(context.Background(), rbacTemplatePath)
		if errNewConnection != nil {
			rbacTemplateErr = errNewConnection
			return
		}

		migrationSource := &migrate.FileMigrationSource{
			Dir: filepath.Join("..", "sql", "migrations"),
		}
		setTableOnce.Do(func() { migrate.SetTable("migrations") })
		_, errMigrate := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
		if errMigrate != nil {
			rbacTemplateErr = errMigrate
		}

		errCloseConn := conn.SQLDb.Close()
		if errCloseConn != nil && rbacTemplateErr == nil {
			rbacTemplateErr = errCloseConn
		}
	})

	return rbacTemplatePath, rbacTemplateErr
}

func copyTestSQLiteDB(sourcePath string, destinationPath string) error {
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

func seedRBACFixture(t *testing.T, conn *Connection) {
	t.Helper()

	_, errNode := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, is_local, host, port, base_url, enabled, os)
		 values (?, ?, ?, ?, ?, ?, ?, ?)`,
		"node-local", "Local Node", true, "localhost", 8080, "http://localhost:8080", true, "linux",
	)
	if errNode != nil {
		t.Fatalf("failed to insert node: %v", errNode)
	}

	_, errSettings := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into local_settings (id, node_id) values (1, ?) on conflict(id) do update set node_id = excluded.node_id`,
		"node-local",
	)
	if errSettings != nil {
		t.Fatalf("failed to insert local settings: %v", errSettings)
	}

	_, errIP := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into ip (address, usable, external) values (?, ?, ?)`,
		"127.0.0.1", true, false,
	)
	if errIP != nil {
		t.Fatalf("failed to insert ip: %v", errIP)
	}

	now := time.Now().UTC()
	_, errOwner := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"user-owner", "owner", "owner@example.com", "Owner", "User", "hash", false, now, now, now,
	)
	if errOwner != nil {
		t.Fatalf("failed to insert owner user: %v", errOwner)
	}

	_, errOther := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"user-other", "other", "other@example.com", "Other", "User", "hash", false, now, now, now,
	)
	if errOther != nil {
		t.Fatalf("failed to insert other user: %v", errOther)
	}

	_, errAdmin := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"user-admin", "admin", "admin@example.com", "Admin", "User", "hash", true, now, now, now,
	)
	if errAdmin != nil {
		t.Fatalf("failed to insert admin user: %v", errAdmin)
	}

	_, errServer := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"server-local-1", "user-owner", "Local One", "minecraft", "OFFLINE",
		20, 20, "world", "127.0.0.1", 25565, 25565, "/tmp/server-local-1", "node-local", "[]",
	)
	if errServer != nil {
		t.Fatalf("failed to insert game server: %v", errServer)
	}
}
