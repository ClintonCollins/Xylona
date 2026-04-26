package db

import (
	"context"
	"testing"
	"time"
)

func TestGameServerSecretEnvRoundTrip(t *testing.T) {
	conn := newEncryptedConnection(t, "game-server-secret.sqlite")
	seedRBACFixture(t, conn)

	errSet := conn.SetGameServerSecretEnv("server-local-1", "TOKEN", "secret-value", "user-admin")
	if errSet != nil {
		t.Fatalf("SetGameServerSecretEnv() error = %v", errSet)
	}

	var encryptedValue string
	var createdAt time.Time
	var updatedAt time.Time
	errScan := conn.SQLDb.QueryRowContext(
		context.Background(),
		`select value_encrypted, created_at, updated_at
		 from game_server_secret
		 where game_server_id = ? and kind = ? and name = ?`,
		"server-local-1",
		GameServerSecretKindEnv,
		"TOKEN",
	).Scan(&encryptedValue, &createdAt, &updatedAt)
	if errScan != nil {
		t.Fatalf("query raw secret error = %v", errScan)
	}
	if encryptedValue == "secret-value" {
		t.Fatal("stored value is plaintext, want encrypted ciphertext")
	}

	states, errStates := conn.ListGameServerSecretEnvStates("server-local-1")
	if errStates != nil {
		t.Fatalf("ListGameServerSecretEnvStates() error = %v", errStates)
	}
	if len(states) != 1 {
		t.Fatalf("ListGameServerSecretEnvStates() length = %d, want 1", len(states))
	}
	if states[0].Name != "TOKEN" || !states[0].Configured {
		t.Fatalf("ListGameServerSecretEnvStates()[0] = %+v, want configured TOKEN", states[0])
	}

	decrypted, errDecrypt := conn.DecryptGameServerSecretEnv("server-local-1")
	if errDecrypt != nil {
		t.Fatalf("DecryptGameServerSecretEnv() error = %v", errDecrypt)
	}
	if decrypted["TOKEN"] != "secret-value" {
		t.Fatalf("DecryptGameServerSecretEnv()[TOKEN] = %q, want %q", decrypted["TOKEN"], "secret-value")
	}

	time.Sleep(time.Millisecond)
	errReplace := conn.SetGameServerSecretEnv("server-local-1", "TOKEN", "replacement", "user-admin")
	if errReplace != nil {
		t.Fatalf("SetGameServerSecretEnv(replace) error = %v", errReplace)
	}

	var replacedCreatedAt time.Time
	var replacedUpdatedAt time.Time
	errReplaceScan := conn.SQLDb.QueryRowContext(
		context.Background(),
		`select created_at, updated_at
		 from game_server_secret
		 where game_server_id = ? and kind = ? and name = ?`,
		"server-local-1",
		GameServerSecretKindEnv,
		"TOKEN",
	).Scan(&replacedCreatedAt, &replacedUpdatedAt)
	if errReplaceScan != nil {
		t.Fatalf("query replaced raw secret error = %v", errReplaceScan)
	}
	if !replacedCreatedAt.Equal(createdAt) {
		t.Fatalf("created_at after replace = %s, want preserved %s", replacedCreatedAt, createdAt)
	}
	if !replacedUpdatedAt.After(updatedAt) {
		t.Fatalf("updated_at after replace = %s, want after %s", replacedUpdatedAt, updatedAt)
	}

	replaced, errReplacedDecrypt := conn.DecryptGameServerSecretEnv("server-local-1")
	if errReplacedDecrypt != nil {
		t.Fatalf("DecryptGameServerSecretEnv(replace) error = %v", errReplacedDecrypt)
	}
	if replaced["TOKEN"] != "replacement" {
		t.Fatalf("DecryptGameServerSecretEnv()[TOKEN] after replace = %q, want %q", replaced["TOKEN"], "replacement")
	}

	errClear := conn.ClearGameServerSecretEnv("server-local-1", "TOKEN")
	if errClear != nil {
		t.Fatalf("ClearGameServerSecretEnv() error = %v", errClear)
	}
	clearedStates, errClearedStates := conn.ListGameServerSecretEnvStates("server-local-1")
	if errClearedStates != nil {
		t.Fatalf("ListGameServerSecretEnvStates(after clear) error = %v", errClearedStates)
	}
	if len(clearedStates) != 0 {
		t.Fatalf("ListGameServerSecretEnvStates(after clear) length = %d, want 0", len(clearedStates))
	}
}

func TestGameServerSecretEnvCascadeDelete(t *testing.T) {
	conn := newEncryptedConnection(t, "game-server-secret-cascade.sqlite")
	seedRBACFixture(t, conn)

	errSet := conn.SetGameServerSecretEnv("server-local-1", "TOKEN", "secret-value", "user-admin")
	if errSet != nil {
		t.Fatalf("SetGameServerSecretEnv() error = %v", errSet)
	}

	errDelete := conn.DeleteGameServer("server-local-1")
	if errDelete != nil {
		t.Fatalf("DeleteGameServer() error = %v", errDelete)
	}

	states, errStates := conn.ListGameServerSecretEnvStates("server-local-1")
	if errStates != nil {
		t.Fatalf("ListGameServerSecretEnvStates() error = %v", errStates)
	}
	if len(states) != 0 {
		t.Fatalf("ListGameServerSecretEnvStates() length = %d, want cascade delete", len(states))
	}
}
