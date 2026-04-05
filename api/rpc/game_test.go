package rpc

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func newTestGame(id, name string) *xylona.Game {
	return &xylona.Game{
		Id:                       id,
		Name:                     name,
		DefaultPort:              25565,
		DefaultQueryPort:         25565,
		DefaultMaxPlayers:        20,
		LinuxSupport:             true,
		LinuxStartArgsTemplate:   `[{"id":"jar","order":0,"ownership":"system","tokens":["-jar","server.jar"],"label":"Jar"}]`,
		LinuxBaseCommand:         "java",
		WindowsSupport:           true,
		WindowsStartArgsTemplate: `[{"id":"jar","order":0,"ownership":"system","tokens":["-jar","server.jar"],"label":"Jar"}]`,
		WindowsBaseCommand:       "java",
	}
}

func addGameForTests(t *testing.T, fixture *rbacRPCFixture, id, name string) *xylona.Game {
	t.Helper()

	req := connect.NewRequest(&xylona.AddGameRequest{
		Game: newTestGame(id, name),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errAdd := fixture.service.AddGame(context.Background(), req)
	if errAdd != nil {
		t.Fatalf("AddGame() setup error = %v", errAdd)
	}
	if resp.Msg == nil || resp.Msg.GetGame() == nil {
		t.Fatalf("AddGame() returned empty response")
	}

	return resp.Msg.GetGame()
}

func TestGetGameValidID(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	game := addGameForTests(t, fixture, "test-game-get", "Test Game Get")

	req := connect.NewRequest(&xylona.GetGameRequest{
		Id: game.GetId(),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errGet := fixture.service.GetGame(context.Background(), req)
	if errGet != nil {
		t.Fatalf("GetGame() error = %v", errGet)
	}
	if resp.Msg == nil || resp.Msg.GetGame() == nil {
		t.Fatalf("GetGame() returned empty response")
	}
	if resp.Msg.GetGame().GetId() != game.GetId() {
		t.Errorf("GetGame().Game.Id = %q, want %q", resp.Msg.GetGame().GetId(), game.GetId())
	}
	if resp.Msg.GetGame().GetName() != "Test Game Get" {
		t.Errorf("GetGame().Game.Name = %q, want %q", resp.Msg.GetGame().GetName(), "Test Game Get")
	}
}

func TestGetGameNonExistentID(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.GetGameRequest{
		Id: "does-not-exist",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

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
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errList := fixture.service.ListGames(context.Background(), req)
	if errList != nil {
		t.Fatalf("ListGames() error = %v", errList)
	}
	if resp.Msg == nil {
		t.Fatalf("ListGames() returned nil message")
	}
	// The initial migration seeds games, so there should be at least some.
	baselineCount := len(resp.Msg.GetGames())
	if baselineCount == 0 {
		t.Errorf("ListGames() returned 0 games, expected seeded games from migration")
	}
}

func TestListGamesIncludesAdded(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// Get baseline count from migration-seeded games.
	baselineReq := connect.NewRequest(&xylona.ListGamesRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, baselineReq, "user-admin")
	baselineResp, errBaseline := fixture.service.ListGames(context.Background(), baselineReq)
	if errBaseline != nil {
		t.Fatalf("ListGames() baseline error = %v", errBaseline)
	}
	baselineCount := len(baselineResp.Msg.GetGames())

	_ = addGameForTests(t, fixture, "list-game-1", "List Game 1")
	_ = addGameForTests(t, fixture, "list-game-2", "List Game 2")
	_ = addGameForTests(t, fixture, "list-game-3", "List Game 3")

	req := connect.NewRequest(&xylona.ListGamesRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errList := fixture.service.ListGames(context.Background(), req)
	if errList != nil {
		t.Fatalf("ListGames() error = %v", errList)
	}
	if resp.Msg == nil {
		t.Fatalf("ListGames() returned nil message")
	}
	wantCount := baselineCount + 3
	if len(resp.Msg.GetGames()) != wantCount {
		t.Errorf("ListGames() returned %d games, want %d", len(resp.Msg.GetGames()), wantCount)
	}
}

func TestAddGameValidData(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.AddGameRequest{
		Game: newTestGame("add-game-valid", "Add Game Valid"),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errAdd := fixture.service.AddGame(context.Background(), req)
	if errAdd != nil {
		t.Fatalf("AddGame() error = %v", errAdd)
	}
	if resp.Msg == nil || resp.Msg.GetGame() == nil {
		t.Fatalf("AddGame() returned empty response")
	}
	if resp.Msg.GetGame().GetName() != "Add Game Valid" {
		t.Errorf("AddGame().Game.Name = %q, want %q", resp.Msg.GetGame().GetName(), "Add Game Valid")
	}
	if resp.Msg.GetGame().GetDefaultPort() != 25565 {
		t.Errorf("AddGame().Game.DefaultPort = %d, want %d", resp.Msg.GetGame().GetDefaultPort(), 25565)
	}

	// Verify game is persisted via GetGame.
	getReq := connect.NewRequest(&xylona.GetGameRequest{
		Id: resp.Msg.GetGame().GetId(),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getReq, "user-admin")
	getResp, errGet := fixture.service.GetGame(context.Background(), getReq)
	if errGet != nil {
		t.Fatalf("GetGame() after AddGame error = %v", errGet)
	}
	if getResp.Msg.GetGame().GetName() != "Add Game Valid" {
		t.Errorf("GetGame().Game.Name = %q, want %q", getResp.Msg.GetGame().GetName(), "Add Game Valid")
	}
}

func TestAddGameRejectsInvalidStructuredStartArgs(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	game := newTestGame("add-game-invalid-start-args", "Add Game Invalid Start Args")
	game.StartArgBlocklist = `[{"pattern":"[","reason":"broken regex"}]`

	req := connect.NewRequest(&xylona.AddGameRequest{
		Game: game,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	_, errAdd := fixture.service.AddGame(context.Background(), req)
	if errAdd == nil {
		t.Fatalf("AddGame() error = nil, want invalid argument")
	}
	if connect.CodeOf(errAdd) != connect.CodeInvalidArgument {
		t.Errorf("AddGame() code = %v, want %v", connect.CodeOf(errAdd), connect.CodeInvalidArgument)
	}
}

func TestAddGameDuplicateID(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	_ = addGameForTests(t, fixture, "dup-game", "Duplicate Game")

	req := connect.NewRequest(&xylona.AddGameRequest{
		Game: newTestGame("dup-game", "Duplicate Game 2"),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

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

	updatedGame := newTestGame(game.GetId(), "Edit Game Updated")
	updatedGame.DefaultPort = 27015

	req := connect.NewRequest(&xylona.EditGameRequest{
		GameId: game.GetId(),
		Game:   updatedGame,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errEdit := fixture.service.EditGame(context.Background(), req)
	if errEdit != nil {
		t.Fatalf("EditGame() error = %v", errEdit)
	}
	if resp.Msg == nil || resp.Msg.GetGame() == nil {
		t.Fatalf("EditGame() returned empty response")
	}
	if resp.Msg.GetGame().GetName() != "Edit Game Updated" {
		t.Errorf("EditGame().Game.Name = %q, want %q", resp.Msg.GetGame().GetName(), "Edit Game Updated")
	}
	if resp.Msg.GetGame().GetDefaultPort() != 27015 {
		t.Errorf("EditGame().Game.DefaultPort = %d, want %d", resp.Msg.GetGame().GetDefaultPort(), 27015)
	}
}

func TestEditGameRejectsInvalidStructuredStartArgs(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	game := addGameForTests(t, fixture, "edit-game-invalid-start-args", "Edit Game Invalid Start Args")

	updatedGame := newTestGame(game.GetId(), "Edit Game Invalid Start Args")
	updatedGame.LinuxBaseCommand = ""

	req := connect.NewRequest(&xylona.EditGameRequest{
		GameId: game.GetId(),
		Game:   updatedGame,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	_, errEdit := fixture.service.EditGame(context.Background(), req)
	if errEdit == nil {
		t.Fatalf("EditGame() error = nil, want invalid argument")
	}
	if connect.CodeOf(errEdit) != connect.CodeInvalidArgument {
		t.Errorf("EditGame() code = %v, want %v", connect.CodeOf(errEdit), connect.CodeInvalidArgument)
	}
}

func TestEditGameNonExistentID(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.EditGameRequest{
		GameId: "does-not-exist",
		Game:   newTestGame("does-not-exist", "No Such Game"),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	_, errEdit := fixture.service.EditGame(context.Background(), req)
	if errEdit == nil {
		t.Fatalf("EditGame() expected error, got nil")
	}
	if connect.CodeOf(errEdit) != connect.CodeNotFound {
		t.Errorf("EditGame() code = %v, want %v", connect.CodeOf(errEdit), connect.CodeNotFound)
	}
}

func TestGetGameValheimSeedDoesNotUseKnownDefaultPassword(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.GetGameRequest{
		Id: "valheim",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errGet := fixture.service.GetGame(context.Background(), req)
	if errGet != nil {
		t.Fatalf("GetGame(valheim) error = %v", errGet)
	}

	template := resp.Msg.GetGame().GetLinuxStartArgsTemplate()
	if template == "" {
		t.Fatalf("GetGame(valheim).Game.LinuxStartArgsTemplate = empty, want seeded template")
	}
	if containsKnownDefault := strings.Contains(template, `"changeme"`); containsKnownDefault {
		t.Fatalf("GetGame(valheim).Game.LinuxStartArgsTemplate leaked known default password: %s", template)
	}
	if !strings.Contains(template, `{{SERVER_ID}}`) {
		t.Fatalf("GetGame(valheim).Game.LinuxStartArgsTemplate = %s, want unique SERVER_ID placeholder", template)
	}
}

func TestRemoveGameValidID(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	game := addGameForTests(t, fixture, "remove-game", "Remove Game")

	req := connect.NewRequest(&xylona.RemoveGameRequest{
		GameId: game.GetId(),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	_, errRemove := fixture.service.RemoveGame(context.Background(), req)
	if errRemove != nil {
		t.Fatalf("RemoveGame() error = %v", errRemove)
	}

	// Verify game is actually deleted.
	getReq := connect.NewRequest(&xylona.GetGameRequest{
		Id: game.GetId(),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getReq, "user-admin")
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
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

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
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	_, errRemove := fixture.service.RemoveGame(context.Background(), req)
	if errRemove == nil {
		t.Fatalf("RemoveGame() expected error for game with active servers, got nil")
	}
	if connect.CodeOf(errRemove) != connect.CodeFailedPrecondition {
		t.Errorf("RemoveGame() code = %v, want %v", connect.CodeOf(errRemove), connect.CodeFailedPrecondition)
	}
}
