package db

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestInsertSecretKeyAndGetByID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "sk-insert.sqlite")

	now := time.Now().UTC()
	setter := &models.LocalSecretKeySetter{
		Name:          omit.From("test-key"),
		SecretKeyHash: omit.From("hash-abc-123"),
		CreatedAt:     omit.From(now),
	}

	key, errInsert := conn.InsertSecretKey(setter)
	if errInsert != nil {
		t.Fatalf("InsertSecretKey() error = %v", errInsert)
	}
	if key.Name != "test-key" {
		t.Errorf("InsertSecretKey().Name = %q, want %q", key.Name, "test-key")
	}
	if key.SecretKeyHash != "hash-abc-123" {
		t.Errorf("InsertSecretKey().SecretKeyHash = %q, want %q", key.SecretKeyHash, "hash-abc-123")
	}

	fetched, errGet := conn.GetSecretKeyByID(key.ID)
	if errGet != nil {
		t.Fatalf("GetSecretKeyByID() error = %v", errGet)
	}
	if fetched.Name != "test-key" {
		t.Errorf("GetSecretKeyByID().Name = %q, want %q", fetched.Name, "test-key")
	}
}

func TestGetSecretKeyByHash(t *testing.T) {
	conn := newRBACMigratedConnection(t, "sk-by-hash.sqlite")

	now := time.Now().UTC()
	setter := &models.LocalSecretKeySetter{
		Name:          omit.From("hash-lookup-key"),
		SecretKeyHash: omit.From("unique-hash-456"),
		CreatedAt:     omit.From(now),
	}

	_, errInsert := conn.InsertSecretKey(setter)
	if errInsert != nil {
		t.Fatalf("InsertSecretKey() error = %v", errInsert)
	}

	fetched, errGet := conn.GetSecretKeyByHash("unique-hash-456")
	if errGet != nil {
		t.Fatalf("GetSecretKeyByHash() error = %v", errGet)
	}
	if fetched.Name != "hash-lookup-key" {
		t.Errorf("GetSecretKeyByHash().Name = %q, want %q", fetched.Name, "hash-lookup-key")
	}
}

func TestGetSecretKeyByHashNotFound(t *testing.T) {
	conn := newRBACMigratedConnection(t, "sk-hash-not-found.sqlite")

	_, errGet := conn.GetSecretKeyByHash("nonexistent-hash")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetSecretKeyByHash() error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestGetSecretKeyByIDNotFound(t *testing.T) {
	conn := newRBACMigratedConnection(t, "sk-id-not-found.sqlite")

	_, errGet := conn.GetSecretKeyByID(99999)
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetSecretKeyByID() error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestGetAllSecretKeys(t *testing.T) {
	conn := newRBACMigratedConnection(t, "sk-all.sqlite")

	now := time.Now().UTC()
	for i, name := range []string{"key-a", "key-b", "key-c"} {
		setter := &models.LocalSecretKeySetter{
			Name:          omit.From(name),
			SecretKeyHash: omit.From("hash-" + name),
			CreatedAt:     omit.From(now.Add(time.Duration(i) * time.Second)),
		}
		_, errInsert := conn.InsertSecretKey(setter)
		if errInsert != nil {
			t.Fatalf("InsertSecretKey(%s) error = %v", name, errInsert)
		}
	}

	keys, errGet := conn.GetAllSecretKeys()
	if errGet != nil {
		t.Fatalf("GetAllSecretKeys() error = %v", errGet)
	}
	if len(keys) != 3 {
		t.Errorf("GetAllSecretKeys() len = %d, want 3", len(keys))
	}
}

func TestDeleteSecretKeyByID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "sk-delete.sqlite")

	now := time.Now().UTC()
	setter := &models.LocalSecretKeySetter{
		Name:          omit.From("delete-me"),
		SecretKeyHash: omit.From("hash-delete"),
		CreatedAt:     omit.From(now),
	}

	key, errInsert := conn.InsertSecretKey(setter)
	if errInsert != nil {
		t.Fatalf("InsertSecretKey() error = %v", errInsert)
	}

	errDelete := conn.DeleteSecretKeyByID(key.ID)
	if errDelete != nil {
		t.Fatalf("DeleteSecretKeyByID() error = %v", errDelete)
	}

	_, errGet := conn.GetSecretKeyByID(key.ID)
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetSecretKeyByID() after delete error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestGetAllSecretKeysEmpty(t *testing.T) {
	conn := newRBACMigratedConnection(t, "sk-empty.sqlite")

	keys, errGet := conn.GetAllSecretKeys()
	if errGet != nil {
		t.Fatalf("GetAllSecretKeys() error = %v", errGet)
	}
	if len(keys) != 0 {
		t.Errorf("GetAllSecretKeys() len = %d, want 0", len(keys))
	}
}
