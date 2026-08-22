package db

import (
	"database/sql"
	"errors"
	"testing"
)

func TestGameServerMapSharePersistence(t *testing.T) {
	conn := newRBACMigratedConnection(t, "game-server-map-share.sqlite")
	seedGameServerFixture(t, conn)
	insertMapShareTestServer(t, conn, "server-local-3", "Local Three", 25567)

	t.Run("lazily creates disabled settings once", func(t *testing.T) {
		share, errCreate := conn.GetOrCreateGameServerMapShare("server-local-1", "FirstSlug")
		if errCreate != nil {
			t.Fatalf("GetOrCreateGameServerMapShare() error = %v", errCreate)
		}
		if share.GameServerID != "server-local-1" || share.PublicIdentifier != "FirstSlug" || share.Enabled {
			t.Fatalf("GetOrCreateGameServerMapShare() = %+v", share)
		}

		existing, errExisting := conn.GetOrCreateGameServerMapShare("server-local-1", "IgnoredSlug")
		if errExisting != nil {
			t.Fatalf("GetOrCreateGameServerMapShare(existing) error = %v", errExisting)
		}
		if existing.PublicIdentifier != "FirstSlug" {
			t.Fatalf("existing public identifier = %q, want FirstSlug", existing.PublicIdentifier)
		}
	})

	t.Run("keeps identifiers case-sensitive and supports caller collision retry", func(t *testing.T) {
		caseVariant, errCaseVariant := conn.GetOrCreateGameServerMapShare("server-local-2", "firstslug")
		if errCaseVariant != nil {
			t.Fatalf("case-variant create error = %v", errCaseVariant)
		}
		if caseVariant.PublicIdentifier != "firstslug" {
			t.Fatalf("case-variant public identifier = %q", caseVariant.PublicIdentifier)
		}

		_, errConflict := conn.GetOrCreateGameServerMapShare("server-local-3", "FirstSlug")
		if !errors.Is(errConflict, ErrGameServerMapShareIdentifierConflict) {
			t.Fatalf("duplicate identifier error = %v, want conflict", errConflict)
		}
		retried, errRetry := conn.GetOrCreateGameServerMapShare("server-local-3", "RetrySlug")
		if errRetry != nil {
			t.Fatalf("collision retry error = %v", errRetry)
		}
		if retried.PublicIdentifier != "RetrySlug" {
			t.Fatalf("collision retry identifier = %q", retried.PublicIdentifier)
		}
	})

	t.Run("updates atomically and releases renamed identifiers", func(t *testing.T) {
		_, errConflict := conn.UpdateGameServerMapShare("server-local-1", "RetrySlug", true)
		if !errors.Is(errConflict, ErrGameServerMapShareIdentifierConflict) {
			t.Fatalf("conflicting update error = %v, want conflict", errConflict)
		}
		unchanged, errUnchanged := conn.GetGameServerMapShareByGameServerID("server-local-1")
		if errUnchanged != nil {
			t.Fatalf("GetGameServerMapShareByGameServerID() error = %v", errUnchanged)
		}
		if unchanged.PublicIdentifier != "FirstSlug" || unchanged.Enabled {
			t.Fatalf("share after conflicting update = %+v", unchanged)
		}

		renamed, errRename := conn.UpdateGameServerMapShare("server-local-1", "Renamed_Slug", true)
		if errRename != nil {
			t.Fatalf("UpdateGameServerMapShare(rename) error = %v", errRename)
		}
		if renamed.PublicIdentifier != "Renamed_Slug" || !renamed.Enabled {
			t.Fatalf("renamed share = %+v", renamed)
		}
		resolved, errResolve := conn.GetEnabledGameServerMapShareByIdentifier("Renamed_Slug")
		if errResolve != nil || resolved.GameServerID != "server-local-1" {
			t.Fatalf("enabled lookup = %+v, %v", resolved, errResolve)
		}
		_, errOldLookup := conn.GetEnabledGameServerMapShareByIdentifier("FirstSlug")
		if !errors.Is(errOldLookup, sql.ErrNoRows) {
			t.Fatalf("old identifier lookup error = %v, want sql.ErrNoRows", errOldLookup)
		}

		reused, errReuse := conn.UpdateGameServerMapShare("server-local-2", "FirstSlug", true)
		if errReuse != nil {
			t.Fatalf("reuse old identifier error = %v", errReuse)
		}
		if reused.PublicIdentifier != "FirstSlug" || !reused.Enabled {
			t.Fatalf("reused share = %+v", reused)
		}
	})

	t.Run("disable retains identifier and server delete cascades", func(t *testing.T) {
		disabled, errDisable := conn.UpdateGameServerMapShare("server-local-1", "Renamed_Slug", false)
		if errDisable != nil {
			t.Fatalf("UpdateGameServerMapShare(disable) error = %v", errDisable)
		}
		if disabled.PublicIdentifier != "Renamed_Slug" || disabled.Enabled {
			t.Fatalf("disabled share = %+v", disabled)
		}
		_, errDisabledLookup := conn.GetEnabledGameServerMapShareByIdentifier("Renamed_Slug")
		if !errors.Is(errDisabledLookup, sql.ErrNoRows) {
			t.Fatalf("disabled identifier lookup error = %v, want sql.ErrNoRows", errDisabledLookup)
		}

		errDelete := conn.DeleteGameServer("server-local-1")
		if errDelete != nil {
			t.Fatalf("DeleteGameServer() error = %v", errDelete)
		}
		_, errDeleted := conn.GetGameServerMapShareByGameServerID("server-local-1")
		if !errors.Is(errDeleted, sql.ErrNoRows) {
			t.Fatalf("deleted server share lookup error = %v, want sql.ErrNoRows", errDeleted)
		}
	})
}

func insertMapShareTestServer(t *testing.T, conn *Connection, id string, name string, port int) {
	t.Helper()
	_, errInsert := conn.SQLDb.ExecContext(
		t.Context(),
		`insert into game_server
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "user-other", name, "minecraft", "OFFLINE", 10, 10, "world", "127.0.0.1", port, port,
		"/tmp/"+id, "node-local", "[]",
	)
	if errInsert != nil {
		t.Fatalf("failed to insert map share test server: %v", errInsert)
	}
}
