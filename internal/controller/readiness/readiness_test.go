package readiness

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestCheckStartSteamGSLTBlocksUntilSecretConfigured(t *testing.T) {
	conn := newReadinessSecretConnection(t)
	gameServer := &models.GameServer{ID: "server-1"}
	gameServer.R.Game = &models.Game{RequiresSteamGameServerLoginToken: true}

	errMissing := CheckStart(context.Background(), conn, gameServer, nil)
	if errMissing == nil {
		t.Fatal("CheckStart() error = nil, want Steam GSLT required")
	}
	if !strings.Contains(errMissing.Error(), "Steam GSLT required") {
		t.Fatalf("CheckStart() error = %v, want Steam GSLT required", errMissing)
	}

	errSet := SetSteamGSLT(conn, gameServer.ID, "steam-token", "user-admin")
	if errSet != nil {
		t.Fatalf("SetSteamGSLT() error = %v", errSet)
	}

	errReady := CheckStart(context.Background(), conn, gameServer, nil)
	if errReady != nil {
		t.Fatalf("CheckStart() after SetSteamGSLT error = %v", errReady)
	}
}

func newReadinessSecretConnection(t *testing.T) *db.Connection {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "readiness.sqlite")
	conn, errNew := db.NewConnection(context.Background(), dbPath)
	if errNew != nil {
		t.Fatalf("NewConnection() error = %v", errNew)
	}
	t.Cleanup(func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil {
			t.Errorf("failed to close test db: %v", errClose)
		}
	})
	conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))

	_, errCreate := conn.SQLDb.ExecContext(
		context.Background(),
		`create table game_server_secret (
			game_server_id text not null,
			kind text not null,
			name text not null,
			value_encrypted text not null,
			updated_by_user_id text,
			created_at timestamp not null default current_timestamp,
			updated_at timestamp not null default current_timestamp,
			primary key (game_server_id, kind, name)
		)`,
	)
	if errCreate != nil {
		t.Fatalf("create game_server_secret table error = %v", errCreate)
	}

	_, errIndex := conn.SQLDb.ExecContext(
		context.Background(),
		`create unique index game_server_secret_lower_name_idx
		 on game_server_secret (game_server_id, kind, lower(name))`,
	)
	if errIndex != nil {
		t.Fatalf("create game_server_secret lower-name index error = %v", errIndex)
	}

	return conn
}
