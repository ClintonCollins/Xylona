package db

import (
	"testing"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// insertTestGameForConfig reduces boilerplate in config schema tests.
func insertTestGameForConfig(t *testing.T, conn *Connection, id string) {
	t.Helper()
	setter := &models.GameSetter{
		ID:                omit.From(id),
		Name:              omit.From("Test Game"),
		DefaultPort:       omit.From(int64(25565)),
		DefaultQueryPort:  omit.From(int64(25565)),
		DefaultMaxPlayers: omit.From(int64(20)),
	}
	_, errInsert := conn.InsertGame(conn.DB, setter)
	if errInsert != nil {
		t.Fatalf("InsertGame() setup error = %v", errInsert)
	}
}

func TestGetGameConfigSchemas_ReturnsEmptyForNewGame(t *testing.T) {
	conn := newRBACMigratedConnection(t, "cfg-schema-read.sqlite")
	addMissingGameColumns(t, conn)
	insertTestGameForConfig(t, conn, "test-game")

	schemas, errGet := conn.GetGameConfigSchemas("test-game")
	if errGet != nil {
		t.Fatalf("GetGameConfigSchemas() error = %v", errGet)
	}
	if schemas != "" {
		t.Errorf("GetGameConfigSchemas() = %q, want empty string for null", schemas)
	}
}

func TestUpdateGameConfigSchemas_RoundTrip(t *testing.T) {
	conn := newRBACMigratedConnection(t, "cfg-schema-update.sqlite")
	addMissingGameColumns(t, conn)
	insertTestGameForConfig(t, conn, "test-game")

	schemasJSON := `[{"path":"server.properties","format":"properties","category":"Core"}]`

	errUpdate := conn.UpdateGameConfigSchemas("test-game", schemasJSON)
	if errUpdate != nil {
		t.Fatalf("UpdateGameConfigSchemas() error = %v", errUpdate)
	}

	got, errGet := conn.GetGameConfigSchemas("test-game")
	if errGet != nil {
		t.Fatalf("GetGameConfigSchemas() error = %v", errGet)
	}
	if got != schemasJSON {
		t.Errorf("GetGameConfigSchemas() = %q, want %q", got, schemasJSON)
	}
}

func TestUpdateGameConfigSchemas_ClearWithEmpty(t *testing.T) {
	conn := newRBACMigratedConnection(t, "cfg-schema-clear.sqlite")
	addMissingGameColumns(t, conn)
	insertTestGameForConfig(t, conn, "test-game")

	// Set schemas, then clear.
	errUpdate := conn.UpdateGameConfigSchemas("test-game", `[{"path":"x"}]`)
	if errUpdate != nil {
		t.Fatalf("UpdateGameConfigSchemas(set) error = %v", errUpdate)
	}
	errClear := conn.UpdateGameConfigSchemas("test-game", "")
	if errClear != nil {
		t.Fatalf("UpdateGameConfigSchemas(clear) error = %v", errClear)
	}

	got, errGet := conn.GetGameConfigSchemas("test-game")
	if errGet != nil {
		t.Fatalf("GetGameConfigSchemas() error = %v", errGet)
	}
	if got != "" {
		t.Errorf("GetGameConfigSchemas() = %q, want empty after clear", got)
	}
}
