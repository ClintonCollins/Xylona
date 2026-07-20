package db

import (
	"errors"
	"testing"
)

func TestGameServerMinecraftMapPersistence(t *testing.T) {
	conn := newRBACMigratedConnection(t, "minecraft-map.sqlite")
	seedRBACFixture(t, conn)
	const gameServerID = "server-local-1"

	settings, errDefaults := conn.GetGameServerMinecraftMap(gameServerID)
	if errDefaults != nil {
		t.Fatalf("GetGameServerMinecraftMap() error = %v", errDefaults)
	}
	if settings.Enabled || settings.WorldName != "world" || settings.AcceptedAt.Valid || settings.ShareTokenHash.Valid {
		t.Fatalf("default Minecraft map settings = %+v", settings)
	}

	errEnable := conn.UpdateGameServerMinecraftMapConfig(gameServerID, true, "survival", true, "user-owner")
	if errEnable != nil {
		t.Fatalf("UpdateGameServerMinecraftMapConfig(enable) error = %v", errEnable)
	}
	errDisable := conn.UpdateGameServerMinecraftMapConfig(gameServerID, false, "survival", false, "user-owner")
	if errDisable != nil {
		t.Fatalf("UpdateGameServerMinecraftMapConfig(disable) error = %v", errDisable)
	}
	settings, errStored := conn.GetGameServerMinecraftMap(gameServerID)
	if errStored != nil {
		t.Fatalf("GetGameServerMinecraftMap() after update error = %v", errStored)
	}
	if settings.Enabled || settings.WorldName != "survival" || !settings.AcceptedAt.Valid {
		t.Fatalf("stored Minecraft map settings = %+v", settings)
	}

	token, errShare := conn.RegenerateGameServerMinecraftMapShare(gameServerID, "user-owner")
	if errShare != nil {
		t.Fatalf("RegenerateGameServerMinecraftMapShare() error = %v", errShare)
	}
	if len(token) != minecraftMapShareTokenByteLen*2 {
		t.Fatalf("Minecraft map token length = %d", len(token))
	}
	resolved, errResolve := conn.GetGameServerMinecraftMapByShareToken(token)
	if errResolve != nil || resolved.GameServerID != gameServerID {
		t.Fatalf("GetGameServerMinecraftMapByShareToken() = %+v, %v", resolved, errResolve)
	}
	if resolved.ShareTokenHash.String == token {
		t.Fatal("stored Minecraft map share credential contains plaintext")
	}

	rotated, errRotate := conn.RegenerateGameServerMinecraftMapShare(gameServerID, "user-owner")
	if errRotate != nil {
		t.Fatalf("rotate Minecraft map share token: %v", errRotate)
	}
	_, errOld := conn.GetGameServerMinecraftMapByShareToken(token)
	if !errors.Is(errOld, ErrMinecraftMapShareNotFound) {
		t.Fatalf("old Minecraft map token error = %v", errOld)
	}
	errRevoke := conn.RevokeGameServerMinecraftMapShare(gameServerID, "user-owner")
	if errRevoke != nil {
		t.Fatalf("RevokeGameServerMinecraftMapShare() error = %v", errRevoke)
	}
	_, errRevoked := conn.GetGameServerMinecraftMapByShareToken(rotated)
	if !errors.Is(errRevoked, ErrMinecraftMapShareNotFound) {
		t.Fatalf("revoked Minecraft map token error = %v", errRevoked)
	}
	_, errMalformed := conn.GetGameServerMinecraftMapByShareToken("not-a-token")
	if !errors.Is(errMalformed, ErrMinecraftMapShareNotFound) {
		t.Fatalf("malformed Minecraft map token error = %v", errMalformed)
	}
}
