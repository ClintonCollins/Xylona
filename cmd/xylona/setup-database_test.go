package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupDatabaseRejectsEncryptionKeyBeforeCreatingDatabase(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		encryptionKey string
		wantErrPart   string
	}{
		{
			name:          "missing encryption key",
			encryptionKey: "",
			wantErrPart:   "ENCRYPTION_KEY_BASE64 must be set",
		},
		{
			name:          "malformed encryption key",
			encryptionKey: "%%%",
			wantErrPart:   "decode ENCRYPTION_KEY_BASE64",
		},
		{
			name:          "encryption key too short",
			encryptionKey: encodeSecretForTest(16),
			wantErrPart:   "ENCRYPTION_KEY_BASE64 must decode to at least 32 bytes",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dbPath := filepath.Join(t.TempDir(), "data.sqlite")
			config := validConfigurationForTest()
			config.DBFilePath = dbPath
			config.EncryptionKey = testCase.encryptionKey

			conn, errSetup := setupDatabase(context.Background(), config)
			if errSetup == nil {
				if conn != nil {
					_ = conn.SQLDb.Close()
				}
				t.Fatal("setupDatabase() error = nil, want encryption key error")
			}
			if !strings.Contains(errSetup.Error(), testCase.wantErrPart) {
				t.Fatalf("setupDatabase() error = %q, want substring %q", errSetup.Error(), testCase.wantErrPart)
			}
			if conn != nil {
				t.Fatal("setupDatabase() connection = non-nil, want nil")
			}

			_, errStat := os.Stat(dbPath)
			if errStat == nil {
				t.Fatal("setupDatabase() created the database file before encryption key validation")
			}
			if !os.IsNotExist(errStat) {
				t.Fatalf("setupDatabase() unexpected database path error = %v", errStat)
			}
		})
	}
}

func TestSetupDatabaseEncryptsWithValidatedKey(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "data.sqlite")
	config := validConfigurationForTest()
	config.DBFilePath = dbPath

	conn, errSetup := setupDatabase(context.Background(), config)
	if errSetup != nil {
		t.Fatalf("setupDatabase() error = %v", errSetup)
	}
	t.Cleanup(func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil {
			t.Errorf("close database error = %v", errClose)
		}
	})

	ciphertext, errEncrypt := conn.EncryptText("node-shared-secret")
	if errEncrypt != nil {
		t.Fatalf("EncryptText() error = %v", errEncrypt)
	}
	plaintext, errDecrypt := conn.DecryptText(ciphertext)
	if errDecrypt != nil {
		t.Fatalf("DecryptText() error = %v", errDecrypt)
	}
	if plaintext != "node-shared-secret" {
		t.Fatalf("DecryptText() = %q, want %q", plaintext, "node-shared-secret")
	}
}
