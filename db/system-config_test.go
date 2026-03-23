package db

import (
	"database/sql"
	"errors"
	"testing"
)

func TestSetAndGetSystemConfig(t *testing.T) {
	conn := newRBACMigratedConnection(t, "sc-roundtrip.sqlite")

	errSet := conn.SetSystemConfig("smtp_host", "mail.example.com")
	if errSet != nil {
		t.Fatalf("SetSystemConfig() error = %v", errSet)
	}

	value, errGet := conn.GetSystemConfig("smtp_host")
	if errGet != nil {
		t.Fatalf("GetSystemConfig() error = %v", errGet)
	}
	if value != "mail.example.com" {
		t.Errorf("GetSystemConfig() = %q, want %q", value, "mail.example.com")
	}
}

func TestSetSystemConfig_Upsert(t *testing.T) {
	conn := newRBACMigratedConnection(t, "sc-upsert.sqlite")

	errFirst := conn.SetSystemConfig("smtp_host", "old.example.com")
	if errFirst != nil {
		t.Fatalf("SetSystemConfig(first) error = %v", errFirst)
	}

	errSecond := conn.SetSystemConfig("smtp_host", "new.example.com")
	if errSecond != nil {
		t.Fatalf("SetSystemConfig(second) error = %v", errSecond)
	}

	value, errGet := conn.GetSystemConfig("smtp_host")
	if errGet != nil {
		t.Fatalf("GetSystemConfig() error = %v", errGet)
	}
	if value != "new.example.com" {
		t.Errorf("GetSystemConfig() = %q, want %q", value, "new.example.com")
	}
}

func TestGetSystemConfig_NotFound(t *testing.T) {
	conn := newRBACMigratedConnection(t, "sc-notfound.sqlite")

	_, errGet := conn.GetSystemConfig("nonexistent")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetSystemConfig(nonexistent) error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestSystemConfig_Encryption(t *testing.T) {
	conn := newEncryptedConnection(t, "sc-encrypt.sqlite")

	plaintext := `{"api_key":"super-secret-12345"}`

	errSet := conn.SetSystemConfig("smtp_password", plaintext)
	if errSet != nil {
		t.Fatalf("SetSystemConfig() error = %v", errSet)
	}

	// Read the raw stored value — must NOT equal plaintext.
	var storedValue string
	errScan := conn.SQLDb.QueryRow(`SELECT value FROM system_config WHERE key = ?`, "smtp_password").Scan(&storedValue)
	if errScan != nil {
		t.Fatalf("QueryRow() error = %v", errScan)
	}
	if storedValue == plaintext {
		t.Error("Stored value matches plaintext — expected encrypted value to be stored")
	}

	// Fetch via GetSystemConfig — must return decrypted plaintext.
	decrypted, errGet := conn.GetSystemConfig("smtp_password")
	if errGet != nil {
		t.Fatalf("GetSystemConfig() error = %v", errGet)
	}
	if decrypted != plaintext {
		t.Errorf("GetSystemConfig() = %q, want plaintext %q", decrypted, plaintext)
	}

	// Upsert with new value and verify round-trip.
	newPlaintext := `{"api_key":"new-secret-67890"}`
	errUpdate := conn.SetSystemConfig("smtp_password", newPlaintext)
	if errUpdate != nil {
		t.Fatalf("SetSystemConfig(update) error = %v", errUpdate)
	}

	updated, errGetUpdated := conn.GetSystemConfig("smtp_password")
	if errGetUpdated != nil {
		t.Fatalf("GetSystemConfig() after update error = %v", errGetUpdated)
	}
	if updated != newPlaintext {
		t.Errorf("GetSystemConfig() after update = %q, want %q", updated, newPlaintext)
	}
}
