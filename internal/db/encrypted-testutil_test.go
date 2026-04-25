package db

import (
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/xycrypt"
)

// newEncryptedConnection builds a migrated test connection with a fresh
// encryption key configured. Used by the notification-channel and
// system-config tests that round-trip encrypted payloads.
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
