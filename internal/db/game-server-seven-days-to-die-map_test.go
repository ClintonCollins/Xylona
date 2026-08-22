package db

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	migrate "github.com/rubenv/sql-migrate"
)

func TestGameServerSevenDaysToDieMapPersistence(t *testing.T) {
	conn := newEncryptedConnection(t, "seven-days-to-die-map.sqlite")
	seedRBACFixture(t, conn)
	const gameServerID = "server-local-1"

	errNotes := conn.UpdateGameServerSevenDaysToDieMapNotes(gameServerID, `[{"id":"base"}]`, "user-owner")
	if errNotes != nil {
		t.Fatalf("UpdateGameServerSevenDaysToDieMapNotes() error = %v", errNotes)
	}
	snapshotAt := time.Date(2026, time.July, 19, 15, 30, 0, 0, time.UTC)
	errSnapshot := conn.StoreGameServerSevenDaysToDieMapSnapshot(gameServerID, `{"enabled":true}`, snapshotAt)
	if errSnapshot != nil {
		t.Fatalf("StoreGameServerSevenDaysToDieMapSnapshot() error = %v", errSnapshot)
	}

	settings, errSettings := conn.GetGameServerSevenDaysToDieMap(gameServerID)
	if errSettings != nil {
		t.Fatalf("GetGameServerSevenDaysToDieMap() error = %v", errSettings)
	}
	if settings.GameServerID != gameServerID || settings.NotesJSON != `[{"id":"base"}]` || settings.SnapshotJSON != `{"enabled":true}` {
		t.Errorf("GetGameServerSevenDaysToDieMap() = %+v", settings)
	}
	if !settings.SnapshotAt.Valid || !settings.SnapshotAt.Time.Equal(snapshotAt) {
		t.Errorf("snapshot time = %v, want %v", settings.SnapshotAt, snapshotAt)
	}
}

func TestGameServerMapShareMigrationInvalidatesLegacyLinks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "seven-days-to-die-map-legacy.sqlite")
	conn, errConnection := NewConnection(t.Context(), dbPath)
	if errConnection != nil {
		t.Fatalf("NewConnection() error = %v", errConnection)
	}
	t.Cleanup(func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil {
			t.Errorf("close test database: %v", errClose)
		}
	})
	migrationsDir, errMigrationsDir := rbacMigrationsDir()
	if errMigrationsDir != nil {
		t.Fatalf("locate migrations: %v", errMigrationsDir)
	}
	migrationSource := &migrate.FileMigrationSource{Dir: migrationsDir}
	setTableOnce.Do(func() { migrate.SetTable("migrations") })
	_, errPreviousMigrations := migrate.ExecVersion(conn.SQLDb, "sqlite3", migrationSource, migrate.Up, 20260815000000)
	if errPreviousMigrations != nil {
		t.Fatalf("migrate to previous version: %v", errPreviousMigrations)
	}
	seedRBACFixture(t, conn)
	const gameServerID = "server-local-1"
	const legacyHash = "legacy-token-hash"
	_, errLegacySevenDays := conn.SQLDb.ExecContext(
		t.Context(),
		`insert into game_server_seven_days_to_die_map (game_server_id, share_token_hash)
		 values (?, ?)`,
		gameServerID,
		legacyHash,
	)
	if errLegacySevenDays != nil {
		t.Fatalf("insert legacy 7 Days to Die map share: %v", errLegacySevenDays)
	}
	_, errLegacyPalworld := conn.SQLDb.ExecContext(
		t.Context(),
		`insert into game_server_palworld_map (game_server_id, share_token_hash) values (?, ?)`,
		gameServerID,
		legacyHash,
	)
	if errLegacyPalworld != nil {
		t.Fatalf("insert legacy Palworld map share: %v", errLegacyPalworld)
	}
	_, errLegacyMinecraft := conn.SQLDb.ExecContext(
		t.Context(),
		`insert into game_server_minecraft_map (game_server_id, share_token_hash, created_at, updated_at)
		 values (?, ?, current_timestamp, current_timestamp)`,
		gameServerID,
		legacyHash,
	)
	if errLegacyMinecraft != nil {
		t.Fatalf("insert legacy Minecraft map share: %v", errLegacyMinecraft)
	}
	_, errLatestMigration := migrate.ExecVersion(conn.SQLDb, "sqlite3", migrationSource, migrate.Up, 20260822000000)
	if errLatestMigration != nil {
		t.Fatalf("apply canonical map share migration: %v", errLatestMigration)
	}

	var legacyShareCount int
	errCount := conn.SQLDb.QueryRowContext(
		t.Context(),
		"select count(*) from game_server_seven_days_to_die_map_share where game_server_id = ?",
		gameServerID,
	).Scan(&legacyShareCount)
	if errCount != nil {
		t.Fatalf("count legacy 7 Days to Die shares: %v", errCount)
	}
	if legacyShareCount != 0 {
		t.Fatalf("legacy 7 Days to Die share count = %d, want 0", legacyShareCount)
	}
	for _, table := range []string{"game_server_seven_days_to_die_map", "game_server_palworld_map", "game_server_minecraft_map"} {
		var tokenHash sql.NullString
		errHash := conn.SQLDb.QueryRowContext(
			t.Context(),
			"select share_token_hash from "+table+" where game_server_id = ?",
			gameServerID,
		).Scan(&tokenHash)
		if errHash != nil {
			t.Fatalf("read %s legacy token hash: %v", table, errHash)
		}
		if tokenHash.Valid {
			t.Errorf("%s legacy token hash was not cleared", table)
		}
	}
}
