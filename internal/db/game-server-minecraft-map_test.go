package db

import (
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
	if settings.Enabled || settings.WorldName != "world" || settings.AcceptedAt.Valid {
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
}
