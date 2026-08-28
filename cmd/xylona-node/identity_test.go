package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIdentityRoundTrip covers save -> load across disk with the full set of
// required fields, plus rejection of partial files.
func TestIdentityRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("save then load returns identical identity", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "node-data")

		// #nosec G101 -- static test fixture placeholders, not real credentials.
		original := &nodeIdentity{
			NodeID:        "node-abc",
			CertPEM:       "-----BEGIN CERTIFICATE-----\nMII...\n-----END CERTIFICATE-----\n",
			KeyPEM:        "-----BEGIN EC PRIVATE KEY-----\nMHc...\n-----END EC PRIVATE KEY-----\n",
			Fingerprint:   "abc123",
			ControllerURL: "https://xylona.test",
			SharedSecret:  "supersecret",
		}
		errSave := saveIdentity(dir, original)
		if errSave != nil {
			t.Fatalf("saveIdentity: %v", errSave)
		}

		loaded, errLoad := loadIdentity(dir)
		if errLoad != nil {
			t.Fatalf("loadIdentity: %v", errLoad)
		}
		if loaded.NodeID != "node-abc" || loaded.SharedSecret != "supersecret" {
			t.Fatalf("roundtrip lost fields: %+v", loaded)
		}
		if loaded.SchemaVersion != currentIdentitySchemaVersion {
			t.Fatalf("expected schema version %d, got %d", currentIdentitySchemaVersion, loaded.SchemaVersion)
		}
	})

	t.Run("loadIdentity returns errIdentityMissing when file absent", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		_, errLoad := loadIdentity(dir)
		if !errors.Is(errLoad, errIdentityMissing) {
			t.Fatalf("expected errIdentityMissing, got %v", errLoad)
		}
	})

	t.Run("saveIdentity rejects incomplete identity", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		incomplete := &nodeIdentity{NodeID: "only-the-id"}
		errSave := saveIdentity(dir, incomplete)
		if errSave == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("loadIdentity rejects corrupt JSON", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "node-data")
		errDataDir := ensureIdentityDataDir(dir)
		if errDataDir != nil {
			t.Fatalf("ensureIdentityDataDir: %v", errDataDir)
		}

		identityPath := filepath.Join(dir, identityFileName)
		errWrite := os.WriteFile(identityPath, []byte("not json"), 0o600)
		if errWrite != nil {
			t.Fatalf("WriteFile: %v", errWrite)
		}
		errProtect := protectIdentityPathSecurity(identityPath, false)
		if errProtect != nil {
			t.Fatalf("protectIdentityPathSecurity: %v", errProtect)
		}
		_, errLoad := loadIdentity(dir)
		if errLoad == nil {
			t.Fatal("expected parse error")
		}
		if !strings.Contains(errLoad.Error(), "parse node identity") {
			t.Fatalf("unexpected error message: %v", errLoad)
		}
	})

	t.Run("saveIdentity creates missing parent directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		dir := filepath.Join(root, "nested", "deep")

		errSave := saveIdentity(dir, &nodeIdentity{
			NodeID:       "n",
			CertPEM:      "cert",
			KeyPEM:       "key",
			Fingerprint:  "fp",
			SharedSecret: "s",
		})
		if errSave != nil {
			t.Fatalf("saveIdentity: %v", errSave)
		}
		_, errStat := os.Stat(filepath.Join(dir, identityFileName))
		if errStat != nil {
			t.Fatalf("identity file not written: %v", errStat)
		}
	})
}
