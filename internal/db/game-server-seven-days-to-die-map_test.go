package db

import (
	"errors"
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

	firstShare, errFirstShare := conn.CreateGameServerSevenDaysToDieMapShare(gameServerID, "user-owner")
	if errFirstShare != nil {
		t.Fatalf("CreateGameServerSevenDaysToDieMapShare(first) error = %v", errFirstShare)
	}
	secondShare, errSecondShare := conn.CreateGameServerSevenDaysToDieMapShare(gameServerID, "user-owner")
	if errSecondShare != nil {
		t.Fatalf("CreateGameServerSevenDaysToDieMapShare(second) error = %v", errSecondShare)
	}
	if len(firstShare.Token) != sevenDaysToDieMapShareTokenByteLen*2 || firstShare.ID == secondShare.ID {
		t.Fatalf("created shares = %+v, %+v", firstShare, secondShare)
	}

	shares, errList := conn.ListGameServerSevenDaysToDieMapShares(gameServerID)
	if errList != nil {
		t.Fatalf("ListGameServerSevenDaysToDieMapShares() error = %v", errList)
	}
	if len(shares) != 2 {
		t.Fatalf("ListGameServerSevenDaysToDieMapShares() count = %d, want 2", len(shares))
	}
	listedTokens := map[string]bool{}
	for _, share := range shares {
		listedTokens[share.Token] = true
	}
	if !listedTokens[firstShare.Token] || !listedTokens[secondShare.Token] {
		t.Errorf("listed share tokens = %v", listedTokens)
	}
	var encryptedToken string
	errEncrypted := conn.SQLDb.QueryRowContext(
		t.Context(),
		"select token_encrypted from game_server_seven_days_to_die_map_share where id = ?",
		firstShare.ID,
	).Scan(&encryptedToken)
	if errEncrypted != nil {
		t.Fatalf("read encrypted share token: %v", errEncrypted)
	}
	if encryptedToken == firstShare.Token {
		t.Error("stored public map credential contains the plaintext share token")
	}

	resolved, errResolve := conn.GetGameServerSevenDaysToDieMapByShareToken(firstShare.Token)
	if errResolve != nil {
		t.Fatalf("GetGameServerSevenDaysToDieMapByShareToken() error = %v", errResolve)
	}
	if resolved.GameServerID != gameServerID || resolved.NotesJSON != `[{"id":"base"}]` || resolved.SnapshotJSON != `{"enabled":true}` {
		t.Errorf("GetGameServerSevenDaysToDieMapByShareToken() = %+v", resolved)
	}
	if !resolved.SnapshotAt.Valid || !resolved.SnapshotAt.Time.Equal(snapshotAt) {
		t.Errorf("snapshot time = %v, want %v", resolved.SnapshotAt, snapshotAt)
	}
	if !resolved.ShareEnabled {
		t.Error("resolved map share_enabled = false, want true")
	}

	errRevoke := conn.RevokeGameServerSevenDaysToDieMapShare(gameServerID, firstShare.ID)
	if errRevoke != nil {
		t.Fatalf("RevokeGameServerSevenDaysToDieMapShare() error = %v", errRevoke)
	}
	_, errRevoked := conn.GetGameServerSevenDaysToDieMapByShareToken(firstShare.Token)
	if !errors.Is(errRevoked, ErrSevenDaysToDieMapShareNotFound) {
		t.Errorf("revoked token error = %v, want %v", errRevoked, ErrSevenDaysToDieMapShareNotFound)
	}
	_, errSecondResolve := conn.GetGameServerSevenDaysToDieMapByShareToken(secondShare.Token)
	if errSecondResolve != nil {
		t.Errorf("remaining token error = %v", errSecondResolve)
	}
	errRevokeAll := conn.RevokeGameServerSevenDaysToDieMapShare(gameServerID, "")
	if errRevokeAll != nil {
		t.Fatalf("RevokeGameServerSevenDaysToDieMapShare(all) error = %v", errRevokeAll)
	}
	settings, errSettings := conn.GetGameServerSevenDaysToDieMap(gameServerID)
	if errSettings != nil {
		t.Fatalf("GetGameServerSevenDaysToDieMap() error = %v", errSettings)
	}
	if settings.ShareEnabled {
		t.Error("map share_enabled = true after revoking all shares")
	}
	_, errMalformed := conn.GetGameServerSevenDaysToDieMapByShareToken("not-a-token")
	if !errors.Is(errMalformed, ErrSevenDaysToDieMapShareNotFound) {
		t.Errorf("malformed token error = %v, want %v", errMalformed, ErrSevenDaysToDieMapShareNotFound)
	}
}

func TestGameServerSevenDaysToDieMapShareMigrationPreservesLegacyLink(t *testing.T) {
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
	const legacyToken = "0000000000000000000000000000000000000000000000000000000000000000"
	_, errLegacy := conn.SQLDb.ExecContext(
		t.Context(),
		`insert into game_server_seven_days_to_die_map (game_server_id, share_token_hash)
		 values (?, ?)`,
		gameServerID,
		hashSevenDaysToDieMapShareToken(legacyToken),
	)
	if errLegacy != nil {
		t.Fatalf("insert legacy map share: %v", errLegacy)
	}
	_, errLatestMigration := migrate.ExecVersion(conn.SQLDb, "sqlite3", migrationSource, migrate.Up, 20260819000000)
	if errLatestMigration != nil {
		t.Fatalf("apply map shares migration: %v", errLatestMigration)
	}

	shares, errList := conn.ListGameServerSevenDaysToDieMapShares(gameServerID)
	if errList != nil {
		t.Fatalf("ListGameServerSevenDaysToDieMapShares() error = %v", errList)
	}
	if len(shares) != 1 || shares[0].Token != "" {
		t.Fatalf("migrated shares = %+v, want one legacy entry without a recoverable token", shares)
	}
	_, errResolve := conn.GetGameServerSevenDaysToDieMapByShareToken(legacyToken)
	if errResolve != nil {
		t.Fatalf("GetGameServerSevenDaysToDieMapByShareToken(legacy) error = %v", errResolve)
	}
	errRevoke := conn.RevokeGameServerSevenDaysToDieMapShare(gameServerID, shares[0].ID)
	if errRevoke != nil {
		t.Fatalf("RevokeGameServerSevenDaysToDieMapShare(legacy) error = %v", errRevoke)
	}
	_, errRevoked := conn.GetGameServerSevenDaysToDieMapByShareToken(legacyToken)
	if !errors.Is(errRevoked, ErrSevenDaysToDieMapShareNotFound) {
		t.Errorf("revoked legacy token error = %v, want %v", errRevoked, ErrSevenDaysToDieMapShareNotFound)
	}
}
