package rpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func newTestGame(id, name string) *xylona.Game {
	return &xylona.Game{
		Id:               id,
		Name:             name,
		DefaultPort:      25565,
		DefaultQueryPort: 25565,
		DefaultMaxPlayers: 20,
		LinuxSupport:     true,
		LinuxStartCommand: "java -jar server.jar",
	}
}

func addGameForTests(t *testing.T, fixture *rbacRPCFixture, id, name string) *xylona.Game {
	t.Helper()

	req := connect.NewRequest(&xylona.AddGameRequest{
		Game: newTestGame(id, name),
	})

	resp, errAdd := fixture.service.AddGame(context.Background(), req)
	if errAdd != nil {
		t.Fatalf("AddGame() setup error = %v", errAdd)
	}
	if resp.Msg == nil || resp.Msg.Game == nil {
		t.Fatalf("AddGame() returned empty response")
	}

	return resp.Msg.Game
}

func TestGetGameValidID(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	game := addGameForTests(t, fixture, "test-game-get", "Test Game Get")

	req := connect.NewRequest(&xylona.GetGameRequest{
		Id: game.Id,
	})

	resp, errGet := fixture.service.GetGame(context.Background(), req)
	if errGet != nil {
		t.Fatalf("GetGame() error = %v", errGet)
	}
	if resp.Msg == nil || resp.Msg.Game == nil {
		t.Fatalf("GetGame() returned empty response")
	}
	if resp.Msg.Game.Id != game.Id {
		t.Errorf("GetGame().Game.Id = %q, want %q", resp.Msg.Game.Id, game.Id)
	}
	if resp.Msg.Game.Name != "Test Game Get" {
		t.Errorf("GetGame().Game.Name = %q, want %q", resp.Msg.Game.Name, "Test Game Get")
	}
}

func TestGetGameNonExistentID(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.GetGameRequest{
		Id: "does-not-exist",
	})

	_, errGet := fixture.service.GetGame(context.Background(), req)
	if errGet == nil {
		t.Fatalf("GetGame() expected error, got nil")
	}
	if connect.CodeOf(errGet) != connect.CodeNotFound {
		t.Errorf("GetGame() code = %v, want %v", connect.CodeOf(errGet), connect.CodeNotFound)
	}
}

func TestListGamesReturnsSeededGames(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.ListGamesRequest{})

	resp, errList := fixture.service.ListGames(context.Background(), req)
	if errList != nil {
		t.Fatalf("ListGames() error = %v", errList)
	}
	if resp.Msg == nil {
		t.Fatalf("ListGames() returned nil message")
	}
	// The initial migration seeds games, so there should be at least some.
	baselineCount := len(resp.Msg.Games)
	if baselineCount == 0 {
		t.Errorf("ListGames() returned 0 games, expected seeded games from migration")
	}
}

func TestListGamesIncludesAdded(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// Get baseline count from migration-seeded games.
	baselineReq := connect.NewRequest(&xylona.ListGamesRequest{})
	baselineResp, errBaseline := fixture.service.ListGames(context.Background(), baselineReq)
	if errBaseline != nil {
		t.Fatalf("ListGames() baseline error = %v", errBaseline)
	}
	baselineCount := len(baselineResp.Msg.Games)

	_ = addGameForTests(t, fixture, "list-game-1", "List Game 1")
	_ = addGameForTests(t, fixture, "list-game-2", "List Game 2")
	_ = addGameForTests(t, fixture, "list-game-3", "List Game 3")

	req := connect.NewRequest(&xylona.ListGamesRequest{})

	resp, errList := fixture.service.ListGames(context.Background(), req)
	if errList != nil {
		t.Fatalf("ListGames() error = %v", errList)
	}
	if resp.Msg == nil {
		t.Fatalf("ListGames() returned nil message")
	}
	wantCount := baselineCount + 3
	if len(resp.Msg.Games) != wantCount {
		t.Errorf("ListGames() returned %d games, want %d", len(resp.Msg.Games), wantCount)
	}
}

func TestAddGameValidData(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.AddGameRequest{
		Game: newTestGame("add-game-valid", "Add Game Valid"),
	})

	resp, errAdd := fixture.service.AddGame(context.Background(), req)
	if errAdd != nil {
		t.Fatalf("AddGame() error = %v", errAdd)
	}
	if resp.Msg == nil || resp.Msg.Game == nil {
		t.Fatalf("AddGame() returned empty response")
	}
	if resp.Msg.Game.Name != "Add Game Valid" {
		t.Errorf("AddGame().Game.Name = %q, want %q", resp.Msg.Game.Name, "Add Game Valid")
	}
	if resp.Msg.Game.DefaultPort != 25565 {
		t.Errorf("AddGame().Game.DefaultPort = %d, want %d", resp.Msg.Game.DefaultPort, 25565)
	}

	// Verify game is persisted via GetGame.
	getReq := connect.NewRequest(&xylona.GetGameRequest{
		Id: resp.Msg.Game.Id,
	})
	getResp, errGet := fixture.service.GetGame(context.Background(), getReq)
	if errGet != nil {
		t.Fatalf("GetGame() after AddGame error = %v", errGet)
	}
	if getResp.Msg.Game.Name != "Add Game Valid" {
		t.Errorf("GetGame().Game.Name = %q, want %q", getResp.Msg.Game.Name, "Add Game Valid")
	}
}

func TestAddGameDuplicateID(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	_ = addGameForTests(t, fixture, "dup-game", "Duplicate Game")

	req := connect.NewRequest(&xylona.AddGameRequest{
		Game: newTestGame("dup-game", "Duplicate Game 2"),
	})

	_, errAdd := fixture.service.AddGame(context.Background(), req)
	if errAdd == nil {
		t.Fatalf("AddGame() with duplicate ID expected error, got nil")
	}
	if connect.CodeOf(errAdd) != connect.CodeInternal {
		t.Errorf("AddGame() code = %v, want %v", connect.CodeOf(errAdd), connect.CodeInternal)
	}
}

func TestEditGameValidData(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	game := addGameForTests(t, fixture, "edit-game", "Edit Game Original")

	updatedGame := newTestGame(game.Id, "Edit Game Updated")
	updatedGame.DefaultPort = 27015

	req := connect.NewRequest(&xylona.EditGameRequest{
		GameId: game.Id,
		Game:   updatedGame,
	})

	resp, errEdit := fixture.service.EditGame(context.Background(), req)
	if errEdit != nil {
		t.Fatalf("EditGame() error = %v", errEdit)
	}
	if resp.Msg == nil || resp.Msg.Game == nil {
		t.Fatalf("EditGame() returned empty response")
	}
	if resp.Msg.Game.Name != "Edit Game Updated" {
		t.Errorf("EditGame().Game.Name = %q, want %q", resp.Msg.Game.Name, "Edit Game Updated")
	}
	if resp.Msg.Game.DefaultPort != 27015 {
		t.Errorf("EditGame().Game.DefaultPort = %d, want %d", resp.Msg.Game.DefaultPort, 27015)
	}
}

func TestEditGameNonExistentID(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.EditGameRequest{
		GameId: "does-not-exist",
		Game:   newTestGame("does-not-exist", "No Such Game"),
	})

	_, errEdit := fixture.service.EditGame(context.Background(), req)
	if errEdit == nil {
		t.Fatalf("EditGame() expected error, got nil")
	}
	if connect.CodeOf(errEdit) != connect.CodeNotFound {
		t.Errorf("EditGame() code = %v, want %v", connect.CodeOf(errEdit), connect.CodeNotFound)
	}
}

func TestRemoveGameValidID(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	game := addGameForTests(t, fixture, "remove-game", "Remove Game")

	req := connect.NewRequest(&xylona.RemoveGameRequest{
		GameId: game.Id,
	})

	_, errRemove := fixture.service.RemoveGame(context.Background(), req)
	if errRemove != nil {
		t.Fatalf("RemoveGame() error = %v", errRemove)
	}

	// Verify game is actually deleted.
	getReq := connect.NewRequest(&xylona.GetGameRequest{
		Id: game.Id,
	})
	_, errGet := fixture.service.GetGame(context.Background(), getReq)
	if errGet == nil {
		t.Fatalf("GetGame() after RemoveGame expected error, got nil")
	}
	if connect.CodeOf(errGet) != connect.CodeNotFound {
		t.Errorf("GetGame() after RemoveGame code = %v, want %v", connect.CodeOf(errGet), connect.CodeNotFound)
	}
}

func TestRemoveGameNonExistentID(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.RemoveGameRequest{
		GameId: "does-not-exist",
	})

	_, errRemove := fixture.service.RemoveGame(context.Background(), req)
	if errRemove == nil {
		t.Fatalf("RemoveGame() expected error, got nil")
	}
	if connect.CodeOf(errRemove) != connect.CodeNotFound {
		t.Errorf("RemoveGame() code = %v, want %v", connect.CodeOf(errRemove), connect.CodeNotFound)
	}
}

func TestRemoveGameUsedByGameServer(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// The seed fixture creates a game_server with game_id "minecraft",
	// and the initial migration seeds a "minecraft" game row.
	// Trying to remove it should fail because servers reference it.
	req := connect.NewRequest(&xylona.RemoveGameRequest{
		GameId: "minecraft",
	})

	_, errRemove := fixture.service.RemoveGame(context.Background(), req)
	if errRemove == nil {
		t.Fatalf("RemoveGame() expected error for game with active servers, got nil")
	}
	if connect.CodeOf(errRemove) != connect.CodeFailedPrecondition {
		t.Errorf("RemoveGame() code = %v, want %v", connect.CodeOf(errRemove), connect.CodeFailedPrecondition)
	}
}
