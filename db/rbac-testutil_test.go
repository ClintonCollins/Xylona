package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	migrate "github.com/rubenv/sql-migrate"
)

func newRBACMigratedConnection(t *testing.T, sqliteFileName string) *Connection {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), sqliteFileName)
	conn := NewConnection(context.Background(), dbPath)
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

	return conn
}

func seedRBACFixture(t *testing.T, conn *Connection) {
	t.Helper()

	_, errNode := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, is_local, host, port, base_url, enabled)
		 values (?, ?, ?, ?, ?, ?, ?)`,
		"node-local", "Local Node", true, "localhost", 8080, "http://localhost:8080", true,
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
		 (id, user_id, name, game_id, start_command, status, set_players, max_players, map, ip, port, query_port, directory, node_id)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"server-local-1", "user-owner", "Local One", "minecraft", "java -jar server.jar", "OFFLINE",
		20, 20, "world", "127.0.0.1", 25565, 25565, "/tmp/server-local-1", "node-local",
	)
	if errServer != nil {
		t.Fatalf("failed to insert game server: %v", errServer)
	}
}
