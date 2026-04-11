package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/stephenafamo/bob"

	"github.com/ClintonCollins/Xylona/pkg/xycrypt"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func makeNodeAPIKeySetter(id, serviceName, apiKey string) *models.NodeAPIKeySetter {
	now := time.Now().UTC()
	return &models.NodeAPIKeySetter{
		ID:          omit.From(id),
		ServiceName: omit.From(serviceName),
		APIKey:      omit.From(apiKey),
		CreatedAt:   omit.From(now),
		UpdatedAt:   omit.From(now),
	}
}

func TestInsertOrUpdateNodeAPIKeyAndGetByServiceName(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nak-insert.sqlite")

	setter := makeNodeAPIKeySetter("key-1", "modrinth", "secret-abc")

	key, errUpsert := conn.InsertOrUpdateNodeAPIKey(conn.DB, setter)
	if errUpsert != nil {
		t.Fatalf("InsertOrUpdateNodeAPIKey() error = %v", errUpsert)
	}
	if key.ServiceName != "modrinth" {
		t.Errorf("InsertOrUpdateNodeAPIKey().ServiceName = %q, want %q", key.ServiceName, "modrinth")
	}
	if key.APIKey != "secret-abc" {
		t.Errorf("InsertOrUpdateNodeAPIKey().APIKey = %q, want %q", key.APIKey, "secret-abc")
	}

	fetched, errGet := conn.GetNodeAPIKeyByServiceName("modrinth")
	if errGet != nil {
		t.Fatalf("GetNodeAPIKeyByServiceName() error = %v", errGet)
	}
	if fetched.APIKey != "secret-abc" {
		t.Errorf("GetNodeAPIKeyByServiceName().APIKey = %q, want %q", fetched.APIKey, "secret-abc")
	}
}

func TestGetNodeAPIKeys(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nak-list.sqlite")

	for _, svc := range []string{"modrinth", "hangar", "thunderstore"} {
		setter := makeNodeAPIKeySetter("key-"+svc, svc, "apikey-"+svc)
		_, errUpsert := conn.InsertOrUpdateNodeAPIKey(conn.DB, setter)
		if errUpsert != nil {
			t.Fatalf("InsertOrUpdateNodeAPIKey(%q) error = %v", svc, errUpsert)
		}
	}

	keys, errGet := conn.GetNodeAPIKeys()
	if errGet != nil {
		t.Fatalf("GetNodeAPIKeys() error = %v", errGet)
	}
	if len(keys) != 3 {
		t.Errorf("GetNodeAPIKeys() len = %d, want 3", len(keys))
	}
}

func TestNodeAPIKeyUpsertUpdatesExisting(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nak-upsert.sqlite")

	setter := makeNodeAPIKeySetter("key-steam", "steam", "old-key")
	_, errFirst := conn.InsertOrUpdateNodeAPIKey(conn.DB, setter)
	if errFirst != nil {
		t.Fatalf("InsertOrUpdateNodeAPIKey(first) error = %v", errFirst)
	}

	updatedSetter := makeNodeAPIKeySetter("key-steam-2", "steam", "new-key")
	_, errSecond := conn.InsertOrUpdateNodeAPIKey(conn.DB, updatedSetter)
	if errSecond != nil {
		t.Fatalf("InsertOrUpdateNodeAPIKey(update) error = %v", errSecond)
	}

	fetched, errGet := conn.GetNodeAPIKeyByServiceName("steam")
	if errGet != nil {
		t.Fatalf("GetNodeAPIKeyByServiceName() error = %v", errGet)
	}
	if fetched.APIKey != "new-key" {
		t.Errorf("GetNodeAPIKeyByServiceName().APIKey = %q, want %q", fetched.APIKey, "new-key")
	}
}

func TestDeleteNodeAPIKeyByServiceName(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nak-delete.sqlite")

	setter := makeNodeAPIKeySetter("key-del", "papermc", "secret")
	_, errUpsert := conn.InsertOrUpdateNodeAPIKey(conn.DB, setter)
	if errUpsert != nil {
		t.Fatalf("InsertOrUpdateNodeAPIKey() error = %v", errUpsert)
	}

	errDelete := conn.DeleteNodeAPIKeyByServiceName("papermc")
	if errDelete != nil {
		t.Fatalf("DeleteNodeAPIKeyByServiceName() error = %v", errDelete)
	}

	_, errGet := conn.GetNodeAPIKeyByServiceName("papermc")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetNodeAPIKeyByServiceName() after delete error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestGetNodeAPIKeyByServiceNameNotFound(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nak-notfound.sqlite")

	_, errGet := conn.GetNodeAPIKeyByServiceName("nonexistent")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetNodeAPIKeyByServiceName() error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestInsertOrUpdateNodeAPIKeyRespectsTransaction(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nak-tx-rollback.sqlite")

	tx, errBegin := conn.SQLDb.BeginTx(context.Background(), nil)
	if errBegin != nil {
		t.Fatalf("BeginTx() error = %v", errBegin)
	}
	bobTx := bob.NewTx(tx)

	setter := makeNodeAPIKeySetter("key-tx", "txservice", "tx-secret")

	_, errUpsert := conn.InsertOrUpdateNodeAPIKey(bobTx, setter)
	if errUpsert != nil {
		t.Fatalf("InsertOrUpdateNodeAPIKey() error = %v", errUpsert)
	}

	errRollback := tx.Rollback()
	if errRollback != nil {
		t.Fatalf("Rollback() error = %v", errRollback)
	}

	_, errGet := conn.GetNodeAPIKeyByServiceName("txservice")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetNodeAPIKeyByServiceName() after rollback error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

// --------------------------------------------------------------------------
// Encryption tests
// --------------------------------------------------------------------------

func newEncryptedConnection(t *testing.T, sqliteFileName string) *Connection {
	t.Helper()
	conn := newRBACMigratedConnection(t, sqliteFileName)

	key, errGen := xycrypt.GenerateEncryptionKey()
	if errGen != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", errGen)
	}
	conn.SetEncryptionKey(key)
	return conn
}

func TestEncryptedInsertAndGetRoundtrip(t *testing.T) {
	conn := newEncryptedConnection(t, "nak-encrypt-roundtrip.sqlite")

	setter := makeNodeAPIKeySetter("key-enc-1", "modrinth", "secret-modrinth-key-12345")

	key, errUpsert := conn.InsertOrUpdateNodeAPIKey(conn.DB, setter)
	if errUpsert != nil {
		t.Fatalf("InsertOrUpdateNodeAPIKey() error = %v", errUpsert)
	}
	// The returned key should have the decrypted (plaintext) value.
	if key.APIKey != "secret-modrinth-key-12345" {
		t.Errorf("InsertOrUpdateNodeAPIKey().APIKey = %q, want %q", key.APIKey, "secret-modrinth-key-12345")
	}

	// Fetch by service name — should also return decrypted value.
	fetched, errGet := conn.GetNodeAPIKeyByServiceName("modrinth")
	if errGet != nil {
		t.Fatalf("GetNodeAPIKeyByServiceName() error = %v", errGet)
	}
	if fetched.APIKey != "secret-modrinth-key-12345" {
		t.Errorf("GetNodeAPIKeyByServiceName().APIKey = %q, want %q", fetched.APIKey, "secret-modrinth-key-12345")
	}
}

func TestEncryptedKeyStoredDifferentFromPlaintext(t *testing.T) {
	conn := newEncryptedConnection(t, "nak-encrypt-stored.sqlite")

	plaintext := "secret-api-key-visible"
	setter := makeNodeAPIKeySetter("key-enc-stored", "steam", plaintext)

	_, errUpsert := conn.InsertOrUpdateNodeAPIKey(conn.DB, setter)
	if errUpsert != nil {
		t.Fatalf("InsertOrUpdateNodeAPIKey() error = %v", errUpsert)
	}

	// Read the raw value directly from the database to confirm it is NOT the plaintext.
	var storedValue string
	errScan := conn.SQLDb.QueryRowContext(conn.ctx, `SELECT api_key FROM node_api_key WHERE service_name = ?`, "steam").Scan(&storedValue)
	if errScan != nil {
		t.Fatalf("QueryRow() error = %v", errScan)
	}

	if storedValue == plaintext {
		t.Errorf("Stored API key matches plaintext %q — expected encrypted value", plaintext)
	}
}

func TestEncryptedGetAllKeysDecrypts(t *testing.T) {
	conn := newEncryptedConnection(t, "nak-encrypt-getall.sqlite")

	for _, svc := range []string{"svc-a", "svc-b"} {
		setter := makeNodeAPIKeySetter("key-"+svc, svc, "apikey-for-"+svc)
		_, errUpsert := conn.InsertOrUpdateNodeAPIKey(conn.DB, setter)
		if errUpsert != nil {
			t.Fatalf("InsertOrUpdateNodeAPIKey(%q) error = %v", svc, errUpsert)
		}
	}

	keys, errGet := conn.GetNodeAPIKeys()
	if errGet != nil {
		t.Fatalf("GetNodeAPIKeys() error = %v", errGet)
	}
	if len(keys) != 2 {
		t.Fatalf("GetNodeAPIKeys() len = %d, want 2", len(keys))
	}

	for _, k := range keys {
		expected := "apikey-for-" + k.ServiceName
		if k.APIKey != expected {
			t.Errorf("GetNodeAPIKeys()[%s].APIKey = %q, want %q", k.ServiceName, k.APIKey, expected)
		}
	}
}

func TestEncryptedUpsertUpdatesExisting(t *testing.T) {
	conn := newEncryptedConnection(t, "nak-encrypt-upsert.sqlite")

	setter := makeNodeAPIKeySetter("key-enc-up", "hangar", "old-encrypted-key")
	_, errFirst := conn.InsertOrUpdateNodeAPIKey(conn.DB, setter)
	if errFirst != nil {
		t.Fatalf("InsertOrUpdateNodeAPIKey(first) error = %v", errFirst)
	}

	updatedSetter := makeNodeAPIKeySetter("key-enc-up-2", "hangar", "new-encrypted-key")
	_, errSecond := conn.InsertOrUpdateNodeAPIKey(conn.DB, updatedSetter)
	if errSecond != nil {
		t.Fatalf("InsertOrUpdateNodeAPIKey(update) error = %v", errSecond)
	}

	fetched, errGet := conn.GetNodeAPIKeyByServiceName("hangar")
	if errGet != nil {
		t.Fatalf("GetNodeAPIKeyByServiceName() error = %v", errGet)
	}
	if fetched.APIKey != "new-encrypted-key" {
		t.Errorf("GetNodeAPIKeyByServiceName().APIKey = %q, want %q", fetched.APIKey, "new-encrypted-key")
	}
}

func TestLegacyNodeAPIKeyCiphertextIsNotMigrated(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nak-legacy.sqlite")

	oldKey, errOld := xycrypt.GenerateEncryptionKey()
	if errOld != nil {
		t.Fatalf("GenerateEncryptionKey(old) error = %v", errOld)
	}
	newKey, errNew := xycrypt.GenerateEncryptionKey()
	if errNew != nil {
		t.Fatalf("GenerateEncryptionKey(new) error = %v", errNew)
	}

	conn.SetEncryptionKey(oldKey)
	setter := makeNodeAPIKeySetter("key-legacy-1", "steam", "secret-steam-key")
	_, errUpsert := conn.InsertOrUpdateNodeAPIKey(conn.DB, setter)
	if errUpsert != nil {
		t.Fatalf("InsertOrUpdateNodeAPIKey() error = %v", errUpsert)
	}

	var storedBefore string
	errScanBefore := conn.SQLDb.QueryRowContext(conn.ctx, `SELECT api_key FROM node_api_key WHERE service_name = ?`, "steam").Scan(&storedBefore)
	if errScanBefore != nil {
		t.Fatalf("QueryRow(before) error = %v", errScanBefore)
	}

	conn.SetEncryptionKey(newKey)

	_, errGet := conn.GetNodeAPIKeyByServiceName("steam")
	if errGet == nil {
		t.Fatal("GetNodeAPIKeyByServiceName() expected error with wrong primary key, got nil")
	}

	_, errList := conn.GetNodeAPIKeys()
	if errList == nil {
		t.Fatal("GetNodeAPIKeys() expected error with wrong primary key, got nil")
	}

	var storedAfter string
	errScanAfter := conn.SQLDb.QueryRowContext(conn.ctx, `SELECT api_key FROM node_api_key WHERE service_name = ?`, "steam").Scan(&storedAfter)
	if errScanAfter != nil {
		t.Fatalf("QueryRow(after) error = %v", errScanAfter)
	}

	if storedAfter != storedBefore {
		t.Errorf("stored API key changed after failed read; got %q, want %q", storedAfter, storedBefore)
	}
}
