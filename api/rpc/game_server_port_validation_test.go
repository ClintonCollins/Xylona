package rpc

import (
	"context"
	"testing"
)

func TestFindAvailablePortIgnoresQueryPortConflicts(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)
	insertNodeScopedIPForParityTests(t, fixture, "node-local", "127.0.0.2")
	seedTestGame(t, fixture)

	_, errInsert := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"server-alt-1", "user-owner", "Alt One", "test-game", "OFFLINE",
		20, 20, "world", "127.0.0.2", 25565, 25565, "/tmp/server-alt-1", "node-local", "[]",
	)
	if errInsert != nil {
		t.Fatalf("insert alt game server error = %v", errInsert)
	}

	game, errGetGame := fixture.conn.GetGameByID("test-game")
	if errGetGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGetGame)
	}

	availablePort, availableQueryPort, errFind := fixture.service.findAvailablePort(
		"node-local",
		"127.0.0.2",
		25566,
		25565,
		game,
		"",
	)
	if errFind != nil {
		t.Fatalf("findAvailablePort() error = %v", errFind)
	}
	if availablePort != 25566 {
		t.Errorf("findAvailablePort() port = %d, want %d", availablePort, 25566)
	}
	if availableQueryPort != 25565 {
		t.Errorf("findAvailablePort() queryPort = %d, want %d", availableQueryPort, 25565)
	}
}

func TestFindAvailablePortAllowsSamePortAndQueryPort(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)
	insertNodeScopedIPForParityTests(t, fixture, "node-local", "127.0.0.2")
	seedTestGame(t, fixture)

	game, errGetGame := fixture.conn.GetGameByID("test-game")
	if errGetGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGetGame)
	}

	availablePort, availableQueryPort, errFind := fixture.service.findAvailablePort(
		"node-local",
		"127.0.0.2",
		25566,
		25566,
		game,
		"",
	)
	if errFind != nil {
		t.Fatalf("findAvailablePort() error = %v", errFind)
	}
	if availablePort != 25566 {
		t.Errorf("findAvailablePort() port = %d, want %d", availablePort, 25566)
	}
	if availableQueryPort != 25566 {
		t.Errorf("findAvailablePort() queryPort = %d, want %d", availableQueryPort, 25566)
	}
}

func TestFindAvailablePortAllowsSameIPAndPortOnDifferentNode(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)
	seedTestGame(t, fixture)

	game, errGetGame := fixture.conn.GetGameByID("test-game")
	if errGetGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGetGame)
	}

	availablePort, availableQueryPort, errFind := fixture.service.findAvailablePort(
		"node-alt",
		"127.0.0.1",
		25565,
		25565,
		game,
		"",
	)
	if errFind != nil {
		t.Fatalf("findAvailablePort() error = %v", errFind)
	}
	if availablePort != 25565 {
		t.Errorf("findAvailablePort() port = %d, want %d", availablePort, 25565)
	}
	if availableQueryPort != 25565 {
		t.Errorf("findAvailablePort() queryPort = %d, want %d", availableQueryPort, 25565)
	}
}
