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

func TestInsertOrUpdateNodeApiKeyAndGetByServiceName(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nak-insert.sqlite")

	setter := makeNodeAPIKeySetter("key-1", "modrinth", "secret-abc")

	key, errUpsert := conn.InsertOrUpdateNodeApiKey(conn.DB, setter)
	if errUpsert != nil {
		t.Fatalf("InsertOrUpdateNodeApiKey() error = %v", errUpsert)
	}
	if key.ServiceName != "modrinth" {
		t.Errorf("InsertOrUpdateNodeApiKey().ServiceName = %q, want %q", key.ServiceName, "modrinth")
	}
	if key.APIKey != "secret-abc" {
		t.Errorf("InsertOrUpdateNodeApiKey().APIKey = %q, want %q", key.APIKey, "secret-abc")
	}

	fetched, errGet := conn.GetNodeApiKeyByServiceName("modrinth")
	if errGet != nil {
		t.Fatalf("GetNodeApiKeyByServiceName() error = %v", errGet)
	}
	if fetched.APIKey != "secret-abc" {
		t.Errorf("GetNodeApiKeyByServiceName().APIKey = %q, want %q", fetched.APIKey, "secret-abc")
	}
}

func TestGetNodeApiKeys(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nak-list.sqlite")

	for _, svc := range []string{"modrinth", "hangar", "thunderstore"} {
		setter := makeNodeAPIKeySetter("key-"+svc, svc, "apikey-"+svc)
		_, errUpsert := conn.InsertOrUpdateNodeApiKey(conn.DB, setter)
		if errUpsert != nil {
			t.Fatalf("InsertOrUpdateNodeApiKey(%q) error = %v", svc, errUpsert)
		}
	}

	keys, errGet := conn.GetNodeApiKeys()
	if errGet != nil {
		t.Fatalf("GetNodeApiKeys() error = %v", errGet)
	}
	if len(keys) != 3 {
		t.Errorf("GetNodeApiKeys() len = %d, want 3", len(keys))
	}
}

func TestNodeApiKeyUpsertUpdatesExisting(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nak-upsert.sqlite")

	setter := makeNodeAPIKeySetter("key-steam", "steam", "old-key")
	_, errFirst := conn.InsertOrUpdateNodeApiKey(conn.DB, setter)
	if errFirst != nil {
		t.Fatalf("InsertOrUpdateNodeApiKey(first) error = %v", errFirst)
	}

	updatedSetter := makeNodeAPIKeySetter("key-steam-2", "steam", "new-key")
	_, errSecond := conn.InsertOrUpdateNodeApiKey(conn.DB, updatedSetter)
	if errSecond != nil {
		t.Fatalf("InsertOrUpdateNodeApiKey(update) error = %v", errSecond)
	}

	fetched, errGet := conn.GetNodeApiKeyByServiceName("steam")
	if errGet != nil {
		t.Fatalf("GetNodeApiKeyByServiceName() error = %v", errGet)
	}
	if fetched.APIKey != "new-key" {
		t.Errorf("GetNodeApiKeyByServiceName().APIKey = %q, want %q", fetched.APIKey, "new-key")
	}
}

func TestDeleteNodeApiKeyByServiceName(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nak-delete.sqlite")

	setter := makeNodeAPIKeySetter("key-del", "papermc", "secret")
	_, errUpsert := conn.InsertOrUpdateNodeApiKey(conn.DB, setter)
	if errUpsert != nil {
		t.Fatalf("InsertOrUpdateNodeApiKey() error = %v", errUpsert)
	}

	errDelete := conn.DeleteNodeApiKeyByServiceName("papermc")
	if errDelete != nil {
		t.Fatalf("DeleteNodeApiKeyByServiceName() error = %v", errDelete)
	}

	_, errGet := conn.GetNodeApiKeyByServiceName("papermc")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetNodeApiKeyByServiceName() after delete error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestGetNodeApiKeyByServiceNameNotFound(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nak-notfound.sqlite")

	_, errGet := conn.GetNodeApiKeyByServiceName("nonexistent")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetNodeApiKeyByServiceName() error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestInsertOrUpdateNodeApiKeyRespectsTransaction(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nak-tx-rollback.sqlite")

	tx, errBegin := conn.SQLDb.BeginTx(context.Background(), nil)
	if errBegin != nil {
		t.Fatalf("BeginTx() error = %v", errBegin)
	}
	bobTx := bob.NewTx(tx)

	setter := makeNodeAPIKeySetter("key-tx", "txservice", "tx-secret")

	_, errUpsert := conn.InsertOrUpdateNodeApiKey(bobTx, setter)
	if errUpsert != nil {
		t.Fatalf("InsertOrUpdateNodeApiKey() error = %v", errUpsert)
	}

	errRollback := tx.Rollback()
	if errRollback != nil {
		t.Fatalf("Rollback() error = %v", errRollback)
	}

	_, errGet := conn.GetNodeApiKeyByServiceName("txservice")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetNodeApiKeyByServiceName() after rollback error = %v, want %v", errGet, sql.ErrNoRows)
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

	key, errUpsert := conn.InsertOrUpdateNodeApiKey(conn.DB, setter)
	if errUpsert != nil {
		t.Fatalf("InsertOrUpdateNodeApiKey() error = %v", errUpsert)
	}
	// The returned key should have the decrypted (plaintext) value.
	if key.APIKey != "secret-modrinth-key-12345" {
		t.Errorf("InsertOrUpdateNodeApiKey().APIKey = %q, want %q", key.APIKey, "secret-modrinth-key-12345")
	}

	// Fetch by service name — should also return decrypted value.
	fetched, errGet := conn.GetNodeApiKeyByServiceName("modrinth")
	if errGet != nil {
		t.Fatalf("GetNodeApiKeyByServiceName() error = %v", errGet)
	}
	if fetched.APIKey != "secret-modrinth-key-12345" {
		t.Errorf("GetNodeApiKeyByServiceName().APIKey = %q, want %q", fetched.APIKey, "secret-modrinth-key-12345")
	}
}

func TestEncryptedKeyStoredDifferentFromPlaintext(t *testing.T) {
	conn := newEncryptedConnection(t, "nak-encrypt-stored.sqlite")

	plaintext := "secret-api-key-visible"
	setter := makeNodeAPIKeySetter("key-enc-stored", "steam", plaintext)

	_, errUpsert := conn.InsertOrUpdateNodeApiKey(conn.DB, setter)
	if errUpsert != nil {
		t.Fatalf("InsertOrUpdateNodeApiKey() error = %v", errUpsert)
	}

	// Read the raw value directly from the database to confirm it is NOT the plaintext.
	var storedValue string
	errScan := conn.SQLDb.QueryRow(`SELECT api_key FROM node_api_key WHERE service_name = ?`, "steam").Scan(&storedValue)
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
		_, errUpsert := conn.InsertOrUpdateNodeApiKey(conn.DB, setter)
		if errUpsert != nil {
			t.Fatalf("InsertOrUpdateNodeApiKey(%q) error = %v", svc, errUpsert)
		}
	}

	keys, errGet := conn.GetNodeApiKeys()
	if errGet != nil {
		t.Fatalf("GetNodeApiKeys() error = %v", errGet)
	}
	if len(keys) != 2 {
		t.Fatalf("GetNodeApiKeys() len = %d, want 2", len(keys))
	}

	for _, k := range keys {
		expected := "apikey-for-" + k.ServiceName
		if k.APIKey != expected {
			t.Errorf("GetNodeApiKeys()[%s].APIKey = %q, want %q", k.ServiceName, k.APIKey, expected)
		}
	}
}

func TestEncryptedUpsertUpdatesExisting(t *testing.T) {
	conn := newEncryptedConnection(t, "nak-encrypt-upsert.sqlite")

	setter := makeNodeAPIKeySetter("key-enc-up", "hangar", "old-encrypted-key")
	_, errFirst := conn.InsertOrUpdateNodeApiKey(conn.DB, setter)
	if errFirst != nil {
		t.Fatalf("InsertOrUpdateNodeApiKey(first) error = %v", errFirst)
	}

	updatedSetter := makeNodeAPIKeySetter("key-enc-up-2", "hangar", "new-encrypted-key")
	_, errSecond := conn.InsertOrUpdateNodeApiKey(conn.DB, updatedSetter)
	if errSecond != nil {
		t.Fatalf("InsertOrUpdateNodeApiKey(update) error = %v", errSecond)
	}

	fetched, errGet := conn.GetNodeApiKeyByServiceName("hangar")
	if errGet != nil {
		t.Fatalf("GetNodeApiKeyByServiceName() error = %v", errGet)
	}
	if fetched.APIKey != "new-encrypted-key" {
		t.Errorf("GetNodeApiKeyByServiceName().APIKey = %q, want %q", fetched.APIKey, "new-encrypted-key")
	}
}

func TestDecryptAPIKey_FallbackKey(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nak-fallback.sqlite")

	oldKey, errOld := xycrypt.GenerateEncryptionKey()
	if errOld != nil {
		t.Fatalf("GenerateEncryptionKey(old) error = %v", errOld)
	}
	newKey, errNew := xycrypt.GenerateEncryptionKey()
	if errNew != nil {
		t.Fatalf("GenerateEncryptionKey(new) error = %v", errNew)
	}

	// Insert with old key.
	conn.SetEncryptionKey(oldKey)
	setter := makeNodeAPIKeySetter("key-fb-1", "steam", "secret-steam-key")
	_, errUpsert := conn.InsertOrUpdateNodeApiKey(conn.DB, setter)
	if errUpsert != nil {
		t.Fatalf("InsertOrUpdateNodeApiKey() error = %v", errUpsert)
	}

	// Switch to new key with old key as fallback.
	conn.SetEncryptionKey(newKey)
	conn.SetFallbackEncryptionKey(oldKey)

	// First read should succeed via fallback and re-encrypt under the new key.
	fetched, errGet := conn.GetNodeApiKeyByServiceName("steam")
	if errGet != nil {
		t.Fatalf("GetNodeApiKeyByServiceName() with fallback error = %v", errGet)
	}
	if fetched.APIKey != "secret-steam-key" {
		t.Errorf("APIKey = %q, want %q", fetched.APIKey, "secret-steam-key")
	}

	// Remove the fallback key. The value should now be readable with only the
	// primary key, proving the re-encryption happened.
	conn.SetFallbackEncryptionKey(nil)

	fetchedAgain, errGetAgain := conn.GetNodeApiKeyByServiceName("steam")
	if errGetAgain != nil {
		t.Fatalf("GetNodeApiKeyByServiceName() after re-encrypt (no fallback) error = %v — re-encryption did not happen", errGetAgain)
	}
	if fetchedAgain.APIKey != "secret-steam-key" {
		t.Errorf("APIKey after re-encrypt = %q, want %q", fetchedAgain.APIKey, "secret-steam-key")
	}
}
