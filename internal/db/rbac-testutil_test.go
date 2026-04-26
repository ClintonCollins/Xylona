package db

import (
	"context"
	"errors"
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

		migrationsDir, errMigrationsDir := rbacMigrationsDir()
		if errMigrationsDir != nil {
			rbacTemplateErr = errMigrationsDir
			return
		}
		migrationSource := &migrate.FileMigrationSource{
			Dir: migrationsDir,
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

func rbacMigrationsDir() (string, error) {
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
		`insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)`,
		"node-local", "Local Node", "http://localhost:8080", true,
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
		`insert into ip (address, usable, external, node_id) values (?, ?, ?, ?)`,
		"127.0.0.1", true, false, "node-local",
	)
	if errIP != nil {
		t.Fatalf("failed to insert ip: %v", errIP)
	}

	_, errGame := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game (id, name, default_port, default_query_port, default_max_players, windows_support)
		 values (?, ?, ?, ?, ?, ?)`,
		"minecraft", "Minecraft", 25565, 25565, 20, true,
	)
	if errGame != nil {
		t.Fatalf("failed to insert game: %v", errGame)
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
