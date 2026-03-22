package db

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"

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
