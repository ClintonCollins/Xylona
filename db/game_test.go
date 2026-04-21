package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/aarondl/opt/omit"
	"github.com/stephenafamo/bob"

	"github.com/ClintonCollins/Xylona/cfgschema"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestInsertGameAndGetGameByID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "game-insert.sqlite")

	setter := &models.GameSetter{
		ID:                omit.From("test-game"),
		Name:              omit.From("Test Game"),
		DefaultPort:       omit.From(int64(27015)),
		DefaultQueryPort:  omit.From(int64(27015)),
		DefaultMaxPlayers: omit.From(int64(32)),
		WindowsSupport:    omit.From(true),
	}

	game, errInsert := conn.InsertGame(conn.DB, setter)
	if errInsert != nil {
		t.Fatalf("InsertGame() error = %v", errInsert)
	}
	if game.ID != "test-game" {
		t.Errorf("InsertGame().ID = %q, want %q", game.ID, "test-game")
	}
	if game.Name != "Test Game" {
		t.Errorf("InsertGame().Name = %q, want %q", game.Name, "Test Game")
	}

	fetched, errGet := conn.GetGameByID("test-game")
	if errGet != nil {
		t.Fatalf("GetGameByID() error = %v", errGet)
	}
	if fetched.Name != "Test Game" {
		t.Errorf("GetGameByID().Name = %q, want %q", fetched.Name, "Test Game")
	}
	if fetched.DefaultPort != 27015 {
		t.Errorf("GetGameByID().DefaultPort = %d, want %d", fetched.DefaultPort, 27015)
	}
}

func TestGetGameByIDNotFound(t *testing.T) {
	conn := newRBACMigratedConnection(t, "game-not-found.sqlite")

	_, errGet := conn.GetGameByID("nonexistent")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetGameByID() error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestGetGames(t *testing.T) {
	conn := newRBACMigratedConnection(t, "game-list.sqlite")

	// The migration seeds at least the "minecraft" game.
	games, errGet := conn.GetGames()
	if errGet != nil {
		t.Fatalf("GetGames() error = %v", errGet)
	}
	if len(games) == 0 {
		t.Fatalf("GetGames() returned 0 games, want at least 1")
	}

	found := false
	for _, g := range games {
		if g.ID == "minecraft" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("GetGames() missing seeded game %q", "minecraft")
	}
}

func TestSeededWindroseGame(t *testing.T) {
	conn := newRBACMigratedConnection(t, "game-windrose.sqlite")

	game, errGet := conn.GetGameByID("windrose")
	if errGet != nil {
		t.Fatalf("GetGameByID(windrose) error = %v", errGet)
	}
	if game.Name != "Windrose" {
		t.Errorf("Name = %q, want %q", game.Name, "Windrose")
	}
	if game.DefaultPort != 7777 {
		t.Errorf("DefaultPort = %d, want 7777", game.DefaultPort)
	}
	if game.DefaultQueryPort != 7778 {
		t.Errorf("DefaultQueryPort = %d, want 7778", game.DefaultQueryPort)
	}
	if game.DefaultMaxPlayers != 8 {
		t.Errorf("DefaultMaxPlayers = %d, want 8", game.DefaultMaxPlayers)
	}
	if !game.UsesSteamcmd {
		t.Error("UsesSteamcmd = false, want true")
	}
	if game.SteamAppID != "4129620" {
		t.Errorf("SteamAppID = %q, want %q", game.SteamAppID, "4129620")
	}
	if game.LinuxSupport {
		t.Error("LinuxSupport = true, want false")
	}
	if !game.WindowsSupport {
		t.Error("WindowsSupport = false, want true")
	}
	if game.WindowsBaseCommand != "WindroseServer.exe" {
		t.Errorf("WindowsBaseCommand = %q, want %q", game.WindowsBaseCommand, "WindroseServer.exe")
	}
	if game.WindowsStartArgsTemplate.GetOr("") == "" {
		t.Error("WindowsStartArgsTemplate is empty, want -log template")
	}

	schemasJSON := game.ConfigSchemas.GetOr("")
	if schemasJSON == "" {
		t.Fatal("ConfigSchemas is empty, want ServerDescription.json schema")
	}
	validationErrors := cfgschema.ValidateConfigSchemas(schemasJSON)
	if len(validationErrors) > 0 {
		t.Fatalf("ValidateConfigSchemas() errors = %v", validationErrors)
	}

	entries, errParse := cfgschema.ParseConfigSchemas(schemasJSON)
	if errParse != nil {
		t.Fatalf("ParseConfigSchemas() error = %v", errParse)
	}
	if len(entries) != 1 {
		t.Fatalf("config schema entry count = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Path != "ServerDescription.json" {
		t.Errorf("schema path = %q, want %q", entry.Path, "ServerDescription.json")
	}
	if entry.Format != "json" {
		t.Errorf("schema format = %q, want %q", entry.Format, "json")
	}
	managedSource := entry.ManagedFields["ServerDescription_Persistent.DirectConnectionServerPort"]
	if managedSource != "game_server.port" {
		t.Errorf("managed direct port source = %q, want %q", managedSource, "game_server.port")
	}
}

func TestUpdateGame(t *testing.T) {
	conn := newRBACMigratedConnection(t, "game-update.sqlite")

	setter := &models.GameSetter{
		ID:                omit.From("update-game"),
		Name:              omit.From("Original Name"),
		DefaultPort:       omit.From(int64(7777)),
		DefaultQueryPort:  omit.From(int64(7777)),
		DefaultMaxPlayers: omit.From(int64(16)),
	}

	game, errInsert := conn.InsertGame(conn.DB, setter)
	if errInsert != nil {
		t.Fatalf("InsertGame() error = %v", errInsert)
	}

	updateSetter := &models.GameSetter{
		ID:                omit.From("update-game"),
		Name:              omit.From("Updated Name"),
		DefaultPort:       omit.From(int64(8888)),
		DefaultQueryPort:  omit.From(int64(8888)),
		DefaultMaxPlayers: omit.From(int64(16)),
	}

	updated, errUpdate := conn.UpdateGame(conn.DB, game, updateSetter)
	if errUpdate != nil {
		t.Fatalf("UpdateGame() error = %v", errUpdate)
	}
	if updated.Name != "Updated Name" {
		t.Errorf("UpdateGame().Name = %q, want %q", updated.Name, "Updated Name")
	}
	if updated.DefaultPort != 8888 {
		t.Errorf("UpdateGame().DefaultPort = %d, want %d", updated.DefaultPort, 8888)
	}
}

func TestDeleteGameByID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "game-delete.sqlite")

	setter := &models.GameSetter{
		ID:                omit.From("delete-game"),
		Name:              omit.From("Delete Me"),
		DefaultPort:       omit.From(int64(9999)),
		DefaultQueryPort:  omit.From(int64(9999)),
		DefaultMaxPlayers: omit.From(int64(8)),
	}

	_, errInsert := conn.InsertGame(conn.DB, setter)
	if errInsert != nil {
		t.Fatalf("InsertGame() error = %v", errInsert)
	}

	errDelete := conn.DeleteGameByID("delete-game")
	if errDelete != nil {
		t.Fatalf("DeleteGameByID() error = %v", errDelete)
	}

	_, errGet := conn.GetGameByID("delete-game")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetGameByID() after delete error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestInsertGameDuplicateID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "game-dup.sqlite")

	setter := &models.GameSetter{
		ID:                omit.From("dup-game"),
		Name:              omit.From("First"),
		DefaultPort:       omit.From(int64(1000)),
		DefaultQueryPort:  omit.From(int64(1000)),
		DefaultMaxPlayers: omit.From(int64(8)),
	}
	_, errFirst := conn.InsertGame(conn.DB, setter)
	if errFirst != nil {
		t.Fatalf("InsertGame(first) error = %v", errFirst)
	}

	setter2 := &models.GameSetter{
		ID:                omit.From("dup-game"),
		Name:              omit.From("Second"),
		DefaultPort:       omit.From(int64(2000)),
		DefaultQueryPort:  omit.From(int64(2000)),
		DefaultMaxPlayers: omit.From(int64(8)),
	}
	_, errSecond := conn.InsertGame(conn.DB, setter2)
	if errSecond == nil {
		t.Fatalf("InsertGame(duplicate) expected error, got nil")
	}
}

func TestInsertGameRespectsTransaction(t *testing.T) {
	conn := newRBACMigratedConnection(t, "game-tx-rollback.sqlite")

	tx, errBegin := conn.SQLDb.BeginTx(context.Background(), nil)
	if errBegin != nil {
		t.Fatalf("BeginTx() error = %v", errBegin)
	}
	bobTx := bob.NewTx(tx)

	setter := &models.GameSetter{
		ID:                omit.From("tx-game"),
		Name:              omit.From("Tx Game"),
		DefaultPort:       omit.From(int64(5555)),
		DefaultQueryPort:  omit.From(int64(5555)),
		DefaultMaxPlayers: omit.From(int64(10)),
	}

	_, errInsert := conn.InsertGame(bobTx, setter)
	if errInsert != nil {
		t.Fatalf("InsertGame() error = %v", errInsert)
	}

	errRollback := tx.Rollback()
	if errRollback != nil {
		t.Fatalf("Rollback() error = %v", errRollback)
	}

	_, errGet := conn.GetGameByID("tx-game")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetGameByID() after rollback error = %v, want %v", errGet, sql.ErrNoRows)
	}
}
