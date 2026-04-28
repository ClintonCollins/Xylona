package rpc

import (
	"context"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
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

func TestFindAvailablePortAllowsSamePortOnDifferentIPsForNormalGames(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)
	insertNodeScopedIPForParityTests(t, fixture, "node-local", "127.0.0.2")
	seedTestGame(t, fixture)
	insertPortValidationServer(t, fixture, "server-alt-1", "Alt One", "test-game", "127.0.0.2", 62000, 62001)

	game, errGetGame := fixture.conn.GetGameByID("test-game")
	if errGetGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGetGame)
	}

	availablePort, availableQueryPort, errFind := fixture.service.findAvailablePort(
		"node-local",
		"127.0.0.1",
		62000,
		62002,
		game,
		"",
	)
	if errFind != nil {
		t.Fatalf("findAvailablePort() error = %v", errFind)
	}
	if availablePort != 62000 {
		t.Errorf("findAvailablePort() port = %d, want %d", availablePort, 62000)
	}
	if availableQueryPort != 62002 {
		t.Errorf("findAvailablePort() queryPort = %d, want %d", availableQueryPort, 62002)
	}
}

func TestFindAvailablePortIncrementsWhenSelectedGameBindsToAllIPs(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)
	insertNodeScopedIPForParityTests(t, fixture, "node-local", "127.0.0.2")
	insertPortValidationGame(t, fixture, "port-normal-game", false)
	insertPortValidationGame(t, fixture, "port-bind-all-game", true)
	insertPortValidationServer(t, fixture, "server-alt-1", "Alt One", "port-normal-game", "127.0.0.2", 62000, 62001)

	game, errGetGame := fixture.conn.GetGameByID("port-bind-all-game")
	if errGetGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGetGame)
	}

	availablePort, availableQueryPort, errFind := fixture.service.findAvailablePort(
		"node-local",
		"127.0.0.1",
		62000,
		62000,
		game,
		"",
	)
	if errFind != nil {
		t.Fatalf("findAvailablePort() error = %v", errFind)
	}
	if availablePort != 62001 {
		t.Errorf("findAvailablePort() port = %d, want %d", availablePort, 62001)
	}
	if availableQueryPort != 62001 {
		t.Errorf("findAvailablePort() queryPort = %d, want %d", availableQueryPort, 62001)
	}
}

func TestFindAvailablePortIncrementsWhenExistingGameBindsToAllIPs(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)
	insertNodeScopedIPForParityTests(t, fixture, "node-local", "127.0.0.2")
	insertPortValidationGame(t, fixture, "port-normal-game", false)
	insertPortValidationGame(t, fixture, "port-bind-all-game", true)
	insertPortValidationServer(t, fixture, "server-alt-1", "Alt One", "port-bind-all-game", "127.0.0.2", 62000, 62001)

	game, errGetGame := fixture.conn.GetGameByID("port-normal-game")
	if errGetGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGetGame)
	}

	availablePort, availableQueryPort, errFind := fixture.service.findAvailablePort(
		"node-local",
		"127.0.0.1",
		62000,
		62000,
		game,
		"",
	)
	if errFind != nil {
		t.Fatalf("findAvailablePort() error = %v", errFind)
	}
	if availablePort != 62001 {
		t.Errorf("findAvailablePort() port = %d, want %d", availablePort, 62001)
	}
	if availableQueryPort != 62001 {
		t.Errorf("findAvailablePort() queryPort = %d, want %d", availableQueryPort, 62001)
	}
}

func insertPortValidationGame(t *testing.T, fixture *rbacRPCFixture, id string, bindsToAllIPs bool) {
	t.Helper()

	now := time.Now().UTC()
	_, errInsert := fixture.conn.InsertGame(fixture.conn.DB, &models.GameSetter{
		ID:                omit.From(id),
		Name:              omit.From(id),
		DefaultPort:       omit.From(int64(28000)),
		DefaultQueryPort:  omit.From(int64(28001)),
		DefaultMaxPlayers: omit.From(int64(48)),
		BindsToAllIps:     omit.From(bindsToAllIPs),
		WindowsSupport:    omit.From(true),
		CreatedAt:         omit.From(now),
		UpdatedAt:         omit.From(now),
	})
	if errInsert != nil {
		t.Fatalf("InsertGame() error = %v", errInsert)
	}
}

func insertPortValidationServer(t *testing.T, fixture *rbacRPCFixture, id string, name string, gameID string, ip string, port int64, queryPort int64) {
	t.Helper()

	_, errInsert := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "user-owner", name, gameID, "OFFLINE",
		20, 20, "world", ip, port, queryPort, "/tmp/"+id, "node-local", "[]",
	)
	if errInsert != nil {
		t.Fatalf("insert game server error = %v", errInsert)
	}
}
