package db_test

import (
	"testing"

	"github.com/ClintonCollins/Xylona/internal/db/dbtest"
)

func TestGameServerPalworldMap(t *testing.T) {
	t.Parallel()
	conn := dbtest.NewMigratedConnection(t, "palworld-map.sqlite")

	_, errUser := conn.SQLDb.ExecContext(
		t.Context(),
		`insert into user (id, user_name) values ('owner', 'palworld-owner')`,
	)
	if errUser != nil {
		t.Fatalf("insert owner: %v", errUser)
	}
	_, errNode := conn.SQLDb.ExecContext(
		t.Context(),
		`insert into node (id, name) values ('node-local', 'Local Node')`,
	)
	if errNode != nil {
		t.Fatalf("insert node: %v", errNode)
	}
	_, errIP := conn.SQLDb.ExecContext(
		t.Context(),
		`insert into ip (address, node_id) values ('127.0.0.1', 'node-local')`,
	)
	if errIP != nil {
		t.Fatalf("insert IP: %v", errIP)
	}
	_, errInsert := conn.SQLDb.ExecContext(
		t.Context(),
		`insert into game_server
			(id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, max_memory_mb, node_id, start_args_patches)
		 values ('pal-map-server', 'owner', 'Palworld Map', 'palworld', 'OFFLINE', 0, 32, '', '127.0.0.1', 8211, 8212, 'pal-map', 0, 'node-local', '[]')`,
	)
	if errInsert != nil {
		t.Fatalf("insert game server: %v", errInsert)
	}

	settings, errGet := conn.GetGameServerPalworldMap("pal-map-server")
	if errGet != nil {
		t.Fatalf("GetGameServerPalworldMap() error = %v", errGet)
	}
	if settings.LayersJSON != "[]" {
		t.Fatalf("default settings = %+v", settings)
	}

	errLayers := conn.UpdateGameServerPalworldMapLayers("pal-map-server", `[{"id":"palpagos"}]`, "owner")
	if errLayers != nil {
		t.Fatalf("UpdateGameServerPalworldMapLayers() error = %v", errLayers)
	}
	stored, errStored := conn.GetGameServerPalworldMap("pal-map-server")
	if errStored != nil {
		t.Fatalf("GetGameServerPalworldMap() after update error = %v", errStored)
	}
	if stored.LayersJSON != `[{"id":"palpagos"}]` {
		t.Fatalf("stored settings = %+v", stored)
	}
}
