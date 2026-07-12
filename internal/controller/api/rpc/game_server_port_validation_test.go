package rpc

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestFindAvailablePortIncrementsOnQueryPortConflict(t *testing.T) {
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
	if availablePort != 25567 {
		t.Errorf("findAvailablePort() port = %d, want %d", availablePort, 25567)
	}
	if availableQueryPort != 25566 {
		t.Errorf("findAvailablePort() queryPort = %d, want %d", availableQueryPort, 25566)
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
	insertPortValidationServer(t, fixture, "server-alt-1", "Alt One", "test-game", 62000, 62001)

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
	insertPortValidationServer(t, fixture, "server-alt-1", "Alt One", "port-normal-game", 62000, 62001)

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
	if availablePort != 62002 {
		t.Errorf("findAvailablePort() port = %d, want %d", availablePort, 62002)
	}
	if availableQueryPort != 62002 {
		t.Errorf("findAvailablePort() queryPort = %d, want %d", availableQueryPort, 62002)
	}
}

func TestFindAvailablePortIncrementsWhenExistingGameBindsToAllIPs(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)
	insertNodeScopedIPForParityTests(t, fixture, "node-local", "127.0.0.2")
	insertPortValidationGame(t, fixture, "port-normal-game", false)
	insertPortValidationGame(t, fixture, "port-bind-all-game", true)
	insertPortValidationServer(t, fixture, "server-alt-1", "Alt One", "port-bind-all-game", 62000, 62001)

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
	if availablePort != 62002 {
		t.Errorf("findAvailablePort() port = %d, want %d", availablePort, 62002)
	}
	if availableQueryPort != 62002 {
		t.Errorf("findAvailablePort() queryPort = %d, want %d", availableQueryPort, 62002)
	}
}

func TestFindAvailablePortReservesSevenDaysToDiePortRange(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)
	insertNodeScopedIPForParityTests(t, fixture, "node-local", "127.0.0.2")
	insertPortValidationServer(
		t,
		fixture,
		"server-7dtd-1",
		"7DTD One",
		"7_days_to_die",
		26900,
		26904,
	)

	game, errGetGame := fixture.conn.GetGameByID("7_days_to_die")
	if errGetGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGetGame)
	}
	availablePort, availableQueryPort, errFind := fixture.service.findAvailablePort(
		"node-local",
		"127.0.0.2",
		26900,
		26904,
		game,
		"",
	)
	if errFind != nil {
		t.Fatalf("findAvailablePort() error = %v", errFind)
	}
	if availablePort != 26905 || availableQueryPort != 26909 {
		t.Fatalf(
			"findAvailablePort() = (%d, %d), want (26905, 26909)",
			availablePort,
			availableQueryPort,
		)
	}
}

func TestGameServerPortFootprintIncludesDerivedPorts(t *testing.T) {
	tests := []struct {
		name      string
		gameID    string
		port      int64
		queryPort int64
		want      []int64
	}{
		{name: "ordinary pair", gameID: "other", port: 7000, queryPort: 7001, want: []int64{7000, 7001}},
		{name: "shared port", gameID: "other", port: 7000, queryPort: 7000, want: []int64{7000}},
		{name: "7 Days to Die range", gameID: "7_days_to_die", port: 26900, queryPort: 26904, want: []int64{26900, 26904, 26901, 26902, 26903}},
		{name: "Conan auxiliary ports", gameID: "conan_exiles", port: 7777, queryPort: 27015, want: []int64{7777, 27015, 7778, 7779}},
		{name: "Project Zomboid direct connection", gameID: "project_zomboid", port: 16261, queryPort: 16261, want: []int64{16261, 16262}},
		{name: "Palworld REST query", gameID: "palworld", port: 8211, queryPort: 8212, want: []int64{8211, 8212}},
		{name: "Sons of the Forest blob sync", gameID: "sons_of_the_forest", port: 8766, queryPort: 27016, want: []int64{8766, 27016, 27017}},
		{name: "Rust companion", gameID: "rust", port: 28015, queryPort: 28016, want: []int64{28015, 28016, 28082}},
		{name: "Source 1 auxiliary ports", gameID: "team_fortress_2", port: 27015, queryPort: 27015, want: []int64{27015, 27016, 27017}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := gameServerPortFootprint(test.gameID, test.port, test.queryPort)
			if !slices.Equal(got, test.want) {
				t.Fatalf("gameServerPortFootprint() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestFindAvailablePortReservesDerivedPorts(t *testing.T) {
	tests := []struct {
		name          string
		gameID        string
		port          int64
		queryPort     int64
		wantPort      int64
		wantQueryPort int64
	}{
		{name: "Palworld", gameID: "palworld", port: 8211, queryPort: 8212, wantPort: 8213, wantQueryPort: 8214},
		{name: "Conan Exiles", gameID: "conan_exiles", port: 7777, queryPort: 27015, wantPort: 7780, wantQueryPort: 27018},
		{name: "Sons of the Forest", gameID: "sons_of_the_forest", port: 8766, queryPort: 27016, wantPort: 8768, wantQueryPort: 27018},
		{name: "Project Zomboid", gameID: "project_zomboid", port: 16261, queryPort: 16261, wantPort: 16263, wantQueryPort: 16263},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)
			seedAlternateNodeAndIP(t, fixture)
			insertNodeScopedIPForParityTests(t, fixture, "node-local", "127.0.0.2")
			insertPortValidationServer(
				t,
				fixture,
				"server-derived-port-1",
				"First server",
				test.gameID,
				test.port,
				test.queryPort,
			)

			game, errGetGame := fixture.conn.GetGameByID(test.gameID)
			if errGetGame != nil {
				t.Fatalf("GetGameByID() error = %v", errGetGame)
			}
			gotPort, gotQueryPort, errFind := fixture.service.findAvailablePort(
				"node-local",
				"127.0.0.2",
				test.port,
				test.queryPort,
				game,
				"",
			)
			if errFind != nil {
				t.Fatalf("findAvailablePort() error = %v", errFind)
			}
			if gotPort != test.wantPort || gotQueryPort != test.wantQueryPort {
				t.Fatalf(
					"findAvailablePort() = (%d, %d), want (%d, %d)",
					gotPort,
					gotQueryPort,
					test.wantPort,
					test.wantQueryPort,
				)
			}
		})
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

func insertPortValidationServer(t *testing.T, fixture *rbacRPCFixture, id string, name string, gameID string, port int64, queryPort int64) {
	t.Helper()

	_, errInsert := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "user-owner", name, gameID, "OFFLINE",
		20, 20, "world", "127.0.0.2", port, queryPort, "/tmp/"+id, "node-local", "[]",
	)
	if errInsert != nil {
		t.Fatalf("insert game server error = %v", errInsert)
	}
}
