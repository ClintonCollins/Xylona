package db

import (
	"errors"
	"testing"
	"time"
)

func TestGameServerSevenDaysToDieMapPersistence(t *testing.T) {
	conn := newRBACMigratedConnection(t, "seven-days-to-die-map.sqlite")
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

	token, errShare := conn.RegenerateGameServerSevenDaysToDieMapShare(gameServerID, "user-owner")
	if errShare != nil {
		t.Fatalf("RegenerateGameServerSevenDaysToDieMapShare() error = %v", errShare)
	}
	if len(token) != sevenDaysToDieMapShareTokenByteLen*2 {
		t.Fatalf("RegenerateGameServerSevenDaysToDieMapShare() token length = %d", len(token))
	}

	resolved, errResolve := conn.GetGameServerSevenDaysToDieMapByShareToken(token)
	if errResolve != nil {
		t.Fatalf("GetGameServerSevenDaysToDieMapByShareToken() error = %v", errResolve)
	}
	if resolved.GameServerID != gameServerID || resolved.NotesJSON != `[{"id":"base"}]` || resolved.SnapshotJSON != `{"enabled":true}` {
		t.Errorf("GetGameServerSevenDaysToDieMapByShareToken() = %+v", resolved)
	}
	if !resolved.SnapshotAt.Valid || !resolved.SnapshotAt.Time.Equal(snapshotAt) {
		t.Errorf("snapshot time = %v, want %v", resolved.SnapshotAt, snapshotAt)
	}
	if resolved.ShareTokenHash.String == token {
		t.Error("stored public map credential contains the plaintext share token")
	}

	errRevoke := conn.RevokeGameServerSevenDaysToDieMapShare(gameServerID, "user-owner")
	if errRevoke != nil {
		t.Fatalf("RevokeGameServerSevenDaysToDieMapShare() error = %v", errRevoke)
	}
	_, errRevoked := conn.GetGameServerSevenDaysToDieMapByShareToken(token)
	if !errors.Is(errRevoked, ErrSevenDaysToDieMapShareNotFound) {
		t.Errorf("revoked token error = %v, want %v", errRevoked, ErrSevenDaysToDieMapShareNotFound)
	}
	_, errMalformed := conn.GetGameServerSevenDaysToDieMapByShareToken("not-a-token")
	if !errors.Is(errMalformed, ErrSevenDaysToDieMapShareNotFound) {
		t.Errorf("malformed token error = %v, want %v", errMalformed, ErrSevenDaysToDieMapShareNotFound)
	}
}
