package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	migrate "github.com/rubenv/sql-migrate"
)

func TestGameServerSecretEnvRoundTrip(t *testing.T) {
	conn := newEncryptedConnection(t, "game-server-secret.sqlite")
	seedRBACFixture(t, conn)

	errSet := conn.SetGameServerSecretEnv("server-local-1", "TOKEN", "secret-value", "user-admin")
	if errSet != nil {
		t.Fatalf("SetGameServerSecretEnv() error = %v", errSet)
	}

	var encryptedValue string
	var createdAt time.Time
	var updatedAt time.Time
	errScan := conn.SQLDb.QueryRowContext(
		context.Background(),
		`select value_encrypted, created_at, updated_at
		 from game_server_secret
		 where game_server_id = ? and kind = ? and name = ?`,
		"server-local-1",
		GameServerSecretKindEnv,
		"TOKEN",
	).Scan(&encryptedValue, &createdAt, &updatedAt)
	if errScan != nil {
		t.Fatalf("query raw secret error = %v", errScan)
	}
	if encryptedValue == "secret-value" {
		t.Fatal("stored value is plaintext, want encrypted ciphertext")
	}

	states, errStates := conn.ListGameServerSecretEnvStates("server-local-1")
	if errStates != nil {
		t.Fatalf("ListGameServerSecretEnvStates() error = %v", errStates)
	}
	if len(states) != 1 {
		t.Fatalf("ListGameServerSecretEnvStates() length = %d, want 1", len(states))
	}
	if states[0].Name != "TOKEN" || !states[0].Configured {
		t.Fatalf("ListGameServerSecretEnvStates()[0] = %+v, want configured TOKEN", states[0])
	}

	decrypted, errDecrypt := conn.DecryptGameServerSecretEnv("server-local-1")
	if errDecrypt != nil {
		t.Fatalf("DecryptGameServerSecretEnv() error = %v", errDecrypt)
	}
	if decrypted["TOKEN"] != "secret-value" {
		t.Fatalf("DecryptGameServerSecretEnv()[TOKEN] = %q, want %q", decrypted["TOKEN"], "secret-value")
	}

	time.Sleep(time.Millisecond)
	errReplace := conn.SetGameServerSecretEnv("server-local-1", "TOKEN", "replacement", "user-admin")
	if errReplace != nil {
		t.Fatalf("SetGameServerSecretEnv(replace) error = %v", errReplace)
	}

	var replacedCreatedAt time.Time
	var replacedUpdatedAt time.Time
	errReplaceScan := conn.SQLDb.QueryRowContext(
		context.Background(),
		`select created_at, updated_at
		 from game_server_secret
		 where game_server_id = ? and kind = ? and name = ?`,
		"server-local-1",
		GameServerSecretKindEnv,
		"TOKEN",
	).Scan(&replacedCreatedAt, &replacedUpdatedAt)
	if errReplaceScan != nil {
		t.Fatalf("query replaced raw secret error = %v", errReplaceScan)
	}
	if !replacedCreatedAt.Equal(createdAt) {
		t.Fatalf("created_at after replace = %s, want preserved %s", replacedCreatedAt, createdAt)
	}
	if !replacedUpdatedAt.After(updatedAt) {
		t.Fatalf("updated_at after replace = %s, want after %s", replacedUpdatedAt, updatedAt)
	}

	replaced, errReplacedDecrypt := conn.DecryptGameServerSecretEnv("server-local-1")
	if errReplacedDecrypt != nil {
		t.Fatalf("DecryptGameServerSecretEnv(replace) error = %v", errReplacedDecrypt)
	}
	if replaced["TOKEN"] != "replacement" {
		t.Fatalf("DecryptGameServerSecretEnv()[TOKEN] after replace = %q, want %q", replaced["TOKEN"], "replacement")
	}

	errCaseReplace := conn.SetGameServerSecretEnv("server-local-1", "token", "case-replacement", "user-admin")
	if errCaseReplace != nil {
		t.Fatalf("SetGameServerSecretEnv(case replace) error = %v", errCaseReplace)
	}
	caseStates, errCaseStates := conn.ListGameServerSecretEnvStates("server-local-1")
	if errCaseStates != nil {
		t.Fatalf("ListGameServerSecretEnvStates(case replace) error = %v", errCaseStates)
	}
	if len(caseStates) != 1 {
		t.Fatalf("ListGameServerSecretEnvStates(case replace) length = %d, want 1", len(caseStates))
	}
	if caseStates[0].Name != "TOKEN" {
		t.Fatalf("ListGameServerSecretEnvStates(case replace)[0].Name = %q, want canonical TOKEN", caseStates[0].Name)
	}
	caseReplaced, errCaseDecrypt := conn.DecryptGameServerSecretEnv("server-local-1")
	if errCaseDecrypt != nil {
		t.Fatalf("DecryptGameServerSecretEnv(case replace) error = %v", errCaseDecrypt)
	}
	if caseReplaced["TOKEN"] != "case-replacement" {
		t.Fatalf("DecryptGameServerSecretEnv()[TOKEN] after case replace = %q, want %q", caseReplaced["TOKEN"], "case-replacement")
	}
	if _, ok := caseReplaced["token"]; ok {
		t.Fatal("DecryptGameServerSecretEnv() returned duplicate lowercase token entry")
	}

	_, errDuplicate := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server_secret
			(game_server_id, kind, name, value_encrypted, updated_by_user_id, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?)`,
		"server-local-1",
		GameServerSecretKindEnv,
		"token",
		"encrypted",
		nil,
		time.Now().UTC(),
		time.Now().UTC(),
	)
	if errDuplicate == nil {
		t.Fatal("direct duplicate lower-case secret insert error = nil, want unique constraint failure")
	}

	errClear := conn.ClearGameServerSecretEnv("server-local-1", "token")
	if errClear != nil {
		t.Fatalf("ClearGameServerSecretEnv() error = %v", errClear)
	}
	clearedStates, errClearedStates := conn.ListGameServerSecretEnvStates("server-local-1")
	if errClearedStates != nil {
		t.Fatalf("ListGameServerSecretEnvStates(after clear) error = %v", errClearedStates)
	}
	if len(clearedStates) != 0 {
		t.Fatalf("ListGameServerSecretEnvStates(after clear) length = %d, want 0", len(clearedStates))
	}
}

func TestGameServerSecretEnvCascadeDelete(t *testing.T) {
	conn := newEncryptedConnection(t, "game-server-secret-cascade.sqlite")
	seedRBACFixture(t, conn)

	errSet := conn.SetGameServerSecretEnv("server-local-1", "TOKEN", "secret-value", "user-admin")
	if errSet != nil {
		t.Fatalf("SetGameServerSecretEnv() error = %v", errSet)
	}

	errDelete := conn.DeleteGameServer("server-local-1")
	if errDelete != nil {
		t.Fatalf("DeleteGameServer() error = %v", errDelete)
	}

	states, errStates := conn.ListGameServerSecretEnvStates("server-local-1")
	if errStates != nil {
		t.Fatalf("ListGameServerSecretEnvStates() error = %v", errStates)
	}
	if len(states) != 0 {
		t.Fatalf("ListGameServerSecretEnvStates() length = %d, want cascade delete", len(states))
	}
}

func TestGameServerSecretEnvCaseDuplicateMigrationKeepsNewest(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "secret-duplicate-migration.sqlite")
	conn, errNew := NewConnection(context.Background(), dbPath)
	if errNew != nil {
		t.Fatalf("NewConnection() error = %v", errNew)
	}
	t.Cleanup(func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil {
			t.Errorf("close test database: %v", errClose)
		}
	})

	_, errCreate := conn.SQLDb.ExecContext(
		context.Background(),
		`create table game_server_secret (
			game_server_id text not null,
			kind text not null,
			name text not null,
			value_encrypted text not null,
			updated_by_user_id text,
			created_at datetime not null,
			updated_at datetime not null,
			primary key (game_server_id, kind, name)
		)`,
	)
	if errCreate != nil {
		t.Fatalf("create game_server_secret table: %v", errCreate)
	}

	older := time.Now().UTC().Add(-time.Hour)
	newer := time.Now().UTC()
	_, errInsert := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server_secret
			(game_server_id, kind, name, value_encrypted, updated_by_user_id, created_at, updated_at)
		 values
			(?, ?, ?, ?, ?, ?, ?),
			(?, ?, ?, ?, ?, ?, ?)`,
		"server-local-1",
		GameServerSecretKindEnv,
		"TOKEN",
		"older",
		nil,
		older,
		older,
		"server-local-1",
		GameServerSecretKindEnv,
		"token",
		"newer",
		nil,
		newer,
		newer,
	)
	if errInsert != nil {
		t.Fatalf("insert duplicate secret rows: %v", errInsert)
	}

	duplicateMigration := findTestMigration(t, "20260426000002")
	migrationSource := &migrate.MemoryMigrationSource{
		Migrations: []*migrate.Migration{duplicateMigration},
	}
	_, errMigrate := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
	if errMigrate != nil {
		t.Fatalf("apply duplicate cleanup migration: %v", errMigrate)
	}

	var count int
	var keptName string
	var keptValue string
	errScan := conn.SQLDb.QueryRowContext(
		context.Background(),
		`select count(*), name, value_encrypted from game_server_secret`,
	).Scan(&count, &keptName, &keptValue)
	if errScan != nil {
		t.Fatalf("query migrated duplicate rows: %v", errScan)
	}
	if count != 1 {
		t.Fatalf("migrated duplicate row count = %d, want 1", count)
	}
	if keptName != "token" {
		t.Fatalf("migrated duplicate kept name = %q, want newest token", keptName)
	}
	if keptValue != "newer" {
		t.Fatalf("migrated duplicate kept value = %q, want newest value", keptValue)
	}

	_, errDuplicate := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server_secret
			(game_server_id, kind, name, value_encrypted, updated_by_user_id, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?)`,
		"server-local-1",
		GameServerSecretKindEnv,
		"TOKEN",
		"duplicate",
		nil,
		time.Now().UTC(),
		time.Now().UTC(),
	)
	if errDuplicate == nil {
		t.Fatal("insert post-migration case duplicate error = nil, want unique constraint failure")
	}
}

func findTestMigration(t *testing.T, idPrefix string) *migrate.Migration {
	t.Helper()

	migrationsDir, errDir := rbacMigrationsDir()
	if errDir != nil {
		t.Fatalf("locate migrations dir: %v", errDir)
	}

	migrationSource := &migrate.FileMigrationSource{Dir: migrationsDir}
	migrations, errFind := migrationSource.FindMigrations()
	if errFind != nil {
		t.Fatalf("find migrations: %v", errFind)
	}
	for _, migration := range migrations {
		if strings.Contains(migration.Id, idPrefix) {
			return migration
		}
	}

	t.Fatalf("migration with prefix %q not found", idPrefix)
	return nil
}
