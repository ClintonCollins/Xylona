package main

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	dbpkg "github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers/federation"
	"github.com/ClintonCollins/Xylona/pkg/xycrypt"
)

func newStartupTestDatabasePath(t *testing.T, fileName string) string {
	t.Helper()

	return filepath.Join(t.TempDir(), fileName)
}

func newStartupTestConnection(t *testing.T, dbPath string) *dbpkg.Connection {
	t.Helper()

	conn, errNewConnection := dbpkg.NewConnection(context.Background(), dbPath)
	if errNewConnection != nil {
		t.Fatalf("NewConnection() error = %v", errNewConnection)
	}
	t.Cleanup(func() {
		_ = conn.SQLDb.Close()
	})

	errMigrate := dbpkg.RunMigrations(conn.SQLDb, EmbeddedMigrations, "sql/migrations")
	if errMigrate != nil {
		t.Fatalf("RunMigrations() error = %v", errMigrate)
	}

	return conn
}

func makeStartupTestConfig(dbPath string, key []byte) Configuration {
	return Configuration{
		DBFilePath:     dbPath,
		EncryptionKey:  base64.StdEncoding.EncodeToString(key),
		FederationPort: 8443,
	}
}

func makeStartupTestConfigWithJWT(dbPath string, encryptionKey []byte, jwtKey []byte) Configuration {
	cfg := makeStartupTestConfig(dbPath, encryptionKey)
	cfg.JWTSecretKey = base64.StdEncoding.EncodeToString(jwtKey)
	return cfg
}

func TestSetupDatabaseRejectsShortEncryptionKey(t *testing.T) {
	testCases := []struct {
		name string
		size int
	}{
		{name: "too short", size: 31},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			dbPath := newStartupTestDatabasePath(t, "setup-database-length.sqlite")
			cfg := makeStartupTestConfig(dbPath, []byte(strings.Repeat("a", tt.size)))

			_, errSetup := setupDatabase(context.Background(), cfg)
			if errSetup == nil {
				t.Fatal("setupDatabase() error = nil, want error")
			}
			if !strings.Contains(errSetup.Error(), "at least 32 bytes") {
				t.Fatalf("setupDatabase() error = %v, want minimum length error", errSetup)
			}
		})
	}
}

func TestSetupDatabaseAcceptsLongEncryptionKeyByTruncatingTo32Bytes(t *testing.T) {
	dbPath := newStartupTestDatabasePath(t, "setup-database-long-key.sqlite")
	longKey := []byte(strings.Repeat("a", 33))

	dbInstFirst, errSetupFirst := setupDatabase(context.Background(), makeStartupTestConfig(dbPath, longKey))
	if errSetupFirst != nil {
		t.Fatalf("setupDatabase(first) error = %v", errSetupFirst)
	}
	errSetConfig := dbInstFirst.SetSystemConfig("smtp_config", `{"host":"smtp.example.com"}`)
	if errSetConfig != nil {
		t.Fatalf("SetSystemConfig() error = %v", errSetConfig)
	}

	errCloseFirst := dbInstFirst.SQLDb.Close()
	if errCloseFirst != nil {
		t.Fatalf("Close(first connection) error = %v", errCloseFirst)
	}

	dbInstSecond, errSetupSecond := setupDatabase(context.Background(), makeStartupTestConfig(dbPath, longKey[:xycrypt.EncryptionKeySize]))
	if errSetupSecond != nil {
		t.Fatalf("setupDatabase(second) error = %v", errSetupSecond)
	}
	t.Cleanup(func() {
		_ = dbInstSecond.SQLDb.Close()
	})

	gotConfig, errGetConfig := dbInstSecond.GetSystemConfig("smtp_config")
	if errGetConfig != nil {
		t.Fatalf("GetSystemConfig() error = %v", errGetConfig)
	}
	if gotConfig != `{"host":"smtp.example.com"}` {
		t.Fatalf("GetSystemConfig() = %q, want original config", gotConfig)
	}
}

func TestSetupDatabaseMigratesLegacyJWTEncryptedSystemConfig(t *testing.T) {
	dbPath := newStartupTestDatabasePath(t, "setup-database-jwt-fallback.sqlite")
	conn := newStartupTestConnection(t, dbPath)

	legacyJWTKey, errLegacyJWTKey := xycrypt.GenerateEncryptionKey()
	if errLegacyJWTKey != nil {
		t.Fatalf("GenerateEncryptionKey(legacy JWT) error = %v", errLegacyJWTKey)
	}
	newEncryptionKey, errNewEncryptionKey := xycrypt.GenerateEncryptionKey()
	if errNewEncryptionKey != nil {
		t.Fatalf("GenerateEncryptionKey(new encryption) error = %v", errNewEncryptionKey)
	}
	wrongJWTKey, errWrongJWTKey := xycrypt.GenerateEncryptionKey()
	if errWrongJWTKey != nil {
		t.Fatalf("GenerateEncryptionKey(wrong JWT) error = %v", errWrongJWTKey)
	}

	conn.SetEncryptionKey(legacyJWTKey)
	errSetConfig := conn.SetSystemConfig("smtp_config", `{"host":"smtp.example.com"}`)
	if errSetConfig != nil {
		t.Fatalf("SetSystemConfig() error = %v", errSetConfig)
	}

	errCloseSeed := conn.SQLDb.Close()
	if errCloseSeed != nil {
		t.Fatalf("Close(seed connection) error = %v", errCloseSeed)
	}

	dbInstFirst, errSetupFirst := setupDatabase(context.Background(), makeStartupTestConfigWithJWT(dbPath, newEncryptionKey, legacyJWTKey))
	if errSetupFirst != nil {
		t.Fatalf("setupDatabase(first) error = %v", errSetupFirst)
	}
	errCloseFirst := dbInstFirst.SQLDb.Close()
	if errCloseFirst != nil {
		t.Fatalf("Close(first connection) error = %v", errCloseFirst)
	}

	dbInstSecond, errSetupSecond := setupDatabase(context.Background(), makeStartupTestConfigWithJWT(dbPath, newEncryptionKey, wrongJWTKey))
	if errSetupSecond != nil {
		t.Fatalf("setupDatabase(second) error = %v", errSetupSecond)
	}
	t.Cleanup(func() {
		_ = dbInstSecond.SQLDb.Close()
	})

	gotConfig, errGetConfig := dbInstSecond.GetSystemConfig("smtp_config")
	if errGetConfig != nil {
		t.Fatalf("GetSystemConfig() error = %v", errGetConfig)
	}
	if gotConfig != `{"host":"smtp.example.com"}` {
		t.Fatalf("GetSystemConfig() = %q, want original config", gotConfig)
	}
}

func TestSetupDatabaseMigratesLegacyFederationLocalIdentityKeyPEM(t *testing.T) {
	dbPath := newStartupTestDatabasePath(t, "setup-database-migrate.sqlite")
	conn := newStartupTestConnection(t, dbPath)

	_, errInsert := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into federation_local_identity
			(id, node_id, cert_path, key_path, cert_pem, key_pem, key_pem_format, cert_fingerprint)
		 values (1, ?, '', '', ?, ?, ?, ?)`,
		"legacy-node",
		"legacy-cert",
		"legacy-key",
		"plaintext",
		"legacy-fp",
	)
	if errInsert != nil {
		t.Fatalf("insert legacy federation identity error = %v", errInsert)
	}

	errCloseSeed := conn.SQLDb.Close()
	if errCloseSeed != nil {
		t.Fatalf("Close(seed connection) error = %v", errCloseSeed)
	}

	key, errGenerate := xycrypt.GenerateEncryptionKey()
	if errGenerate != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", errGenerate)
	}

	dbInst, errSetup := setupDatabase(context.Background(), makeStartupTestConfig(dbPath, key))
	if errSetup != nil {
		t.Fatalf("setupDatabase() error = %v", errSetup)
	}
	t.Cleanup(func() {
		_ = dbInst.SQLDb.Close()
	})

	var storedCertPEM string
	var storedKeyPEM string
	var storedKeyPEMFormat string
	errScan := dbInst.SQLDb.QueryRowContext(
		context.Background(),
		`select cert_pem, key_pem, key_pem_format from federation_local_identity where id = 1`,
	).Scan(&storedCertPEM, &storedKeyPEM, &storedKeyPEMFormat)
	if errScan != nil {
		t.Fatalf("QueryRow() error = %v", errScan)
	}
	if storedCertPEM != "legacy-cert" {
		t.Errorf("stored cert_pem = %q, want %q", storedCertPEM, "legacy-cert")
	}
	if storedKeyPEM == "legacy-key" {
		t.Errorf("stored key_pem matches plaintext after startup migration")
	}
	if storedKeyPEMFormat != "aes256gcm-v1" {
		t.Errorf("stored key_pem_format = %q, want %q", storedKeyPEMFormat, "aes256gcm-v1")
	}
}

func TestSetupDatabaseFailsWhenEncryptedFederationIdentityIsUnreadable(t *testing.T) {
	dbPath := newStartupTestDatabasePath(t, "setup-database-wrong-key.sqlite")

	goodKey, errGoodKey := xycrypt.GenerateEncryptionKey()
	if errGoodKey != nil {
		t.Fatalf("GenerateEncryptionKey(good) error = %v", errGoodKey)
	}

	dbInst, errSetup := setupDatabase(context.Background(), makeStartupTestConfig(dbPath, goodKey))
	if errSetup != nil {
		t.Fatalf("setupDatabase() with good key error = %v", errSetup)
	}

	errUpsert := dbInst.UpsertFederationLocalIdentity("node-1", "cert-pem", "key-pem", "fp-1")
	if errUpsert != nil {
		t.Fatalf("UpsertFederationLocalIdentity() error = %v", errUpsert)
	}

	errClose := dbInst.SQLDb.Close()
	if errClose != nil {
		t.Fatalf("Close() error = %v", errClose)
	}

	wrongKey, errWrongKey := xycrypt.GenerateEncryptionKey()
	if errWrongKey != nil {
		t.Fatalf("GenerateEncryptionKey(wrong) error = %v", errWrongKey)
	}

	_, errSetupWrongKey := setupDatabase(context.Background(), makeStartupTestConfig(dbPath, wrongKey))
	if errSetupWrongKey == nil {
		t.Fatal("setupDatabase() with wrong key error = nil, want error")
	}
	if !strings.Contains(errSetupWrongKey.Error(), "federation local identity") {
		t.Fatalf("setupDatabase() error = %v, want federation identity failure", errSetupWrongKey)
	}
}

func TestSetupDatabaseDoesNotMigratePlaintextFederationIdentityBeforeExistingSecretValidation(t *testing.T) {
	dbPath := newStartupTestDatabasePath(t, "setup-database-prevalidate.sqlite")
	conn := newStartupTestConnection(t, dbPath)

	goodKey, errGoodKey := xycrypt.GenerateEncryptionKey()
	if errGoodKey != nil {
		t.Fatalf("GenerateEncryptionKey(good) error = %v", errGoodKey)
	}
	conn.SetEncryptionKey(goodKey)

	errSetConfig := conn.SetSystemConfig("smtp_config", `{"host":"smtp.example.com"}`)
	if errSetConfig != nil {
		t.Fatalf("SetSystemConfig() error = %v", errSetConfig)
	}

	_, errInsert := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into federation_local_identity
			(id, node_id, cert_path, key_path, cert_pem, key_pem, key_pem_format, cert_fingerprint)
		 values (1, ?, '', '', ?, ?, ?, ?)`,
		"legacy-node",
		"legacy-cert",
		"legacy-key",
		"plaintext",
		"legacy-fp",
	)
	if errInsert != nil {
		t.Fatalf("insert legacy federation identity error = %v", errInsert)
	}

	errCloseSeed := conn.SQLDb.Close()
	if errCloseSeed != nil {
		t.Fatalf("Close(seed connection) error = %v", errCloseSeed)
	}

	wrongKey, errWrongKey := xycrypt.GenerateEncryptionKey()
	if errWrongKey != nil {
		t.Fatalf("GenerateEncryptionKey(wrong) error = %v", errWrongKey)
	}

	_, errSetupWrongKey := setupDatabase(context.Background(), makeStartupTestConfig(dbPath, wrongKey))
	if errSetupWrongKey == nil {
		t.Fatal("setupDatabase() with wrong key error = nil, want error")
	}
	if !strings.Contains(errSetupWrongKey.Error(), "existing encrypted secret storage") {
		t.Fatalf("setupDatabase() error = %v, want existing encrypted secret storage failure", errSetupWrongKey)
	}

	inspectConn, errInspectConnection := dbpkg.NewConnection(context.Background(), dbPath)
	if errInspectConnection != nil {
		t.Fatalf("NewConnection(inspect) error = %v", errInspectConnection)
	}
	t.Cleanup(func() {
		_ = inspectConn.SQLDb.Close()
	})

	var storedKeyPEM string
	var storedKeyPEMFormat string
	errScan := inspectConn.SQLDb.QueryRowContext(
		context.Background(),
		`select key_pem, key_pem_format from federation_local_identity where id = 1`,
	).Scan(&storedKeyPEM, &storedKeyPEMFormat)
	if errScan != nil {
		t.Fatalf("QueryRow(inspect) error = %v", errScan)
	}
	if storedKeyPEM != "legacy-key" {
		t.Fatalf("stored key_pem = %q, want plaintext value preserved after failed startup", storedKeyPEM)
	}
	if storedKeyPEMFormat != "plaintext" {
		t.Fatalf("stored key_pem_format = %q, want %q", storedKeyPEMFormat, "plaintext")
	}

	dbInst, errSetup := setupDatabase(context.Background(), makeStartupTestConfig(dbPath, goodKey))
	if errSetup != nil {
		t.Fatalf("setupDatabase() with good key error = %v", errSetup)
	}
	t.Cleanup(func() {
		_ = dbInst.SQLDb.Close()
	})

	var migratedKeyPEM string
	var migratedKeyPEMFormat string
	errScanMigrated := dbInst.SQLDb.QueryRowContext(
		context.Background(),
		`select key_pem, key_pem_format from federation_local_identity where id = 1`,
	).Scan(&migratedKeyPEM, &migratedKeyPEMFormat)
	if errScanMigrated != nil {
		t.Fatalf("QueryRow(migrated) error = %v", errScanMigrated)
	}
	if migratedKeyPEM == "legacy-key" {
		t.Fatalf("stored key_pem = %q, want encrypted value after successful startup", migratedKeyPEM)
	}
	if migratedKeyPEMFormat != "aes256gcm-v1" {
		t.Fatalf("stored key_pem_format = %q, want %q", migratedKeyPEMFormat, "aes256gcm-v1")
	}
}

func TestSetupFederationIdentityMigratesPlaintextAndIsStableOnRestart(t *testing.T) {
	dbPath := newStartupTestDatabasePath(t, "setup-federation-identity-stable.sqlite")
	seedConn := newStartupTestConnection(t, dbPath)

	certPEM, keyPEM, errGenerate := federation.GenerateCertificatePEM("legacy-node")
	if errGenerate != nil {
		t.Fatalf("GenerateCertificatePEM() error = %v", errGenerate)
	}

	_, errInsert := seedConn.SQLDb.ExecContext(
		context.Background(),
		`insert into federation_local_identity
			(id, node_id, cert_path, key_path, cert_pem, key_pem, key_pem_format, cert_fingerprint)
		 values (1, ?, '', '', ?, ?, ?, ?)`,
		"legacy-node",
		string(certPEM),
		string(keyPEM),
		"plaintext",
		"legacy-fp",
	)
	if errInsert != nil {
		t.Fatalf("insert plaintext federation identity error = %v", errInsert)
	}

	errCloseSeed := seedConn.SQLDb.Close()
	if errCloseSeed != nil {
		t.Fatalf("Close(seed connection) error = %v", errCloseSeed)
	}

	key, errKey := xycrypt.GenerateEncryptionKey()
	if errKey != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", errKey)
	}
	cfg := makeStartupTestConfig(dbPath, key)

	dbInstFirst, errSetupFirst := setupDatabase(context.Background(), cfg)
	if errSetupFirst != nil {
		t.Fatalf("setupDatabase(first) error = %v", errSetupFirst)
	}
	_, _, errFederationFirst := setupFederationIdentity(context.Background(), dbInstFirst, cfg)
	if errFederationFirst != nil {
		t.Fatalf("setupFederationIdentity(first) error = %v", errFederationFirst)
	}

	var storedAfterFirst string
	errScanFirst := dbInstFirst.SQLDb.QueryRowContext(
		context.Background(),
		`select key_pem from federation_local_identity where id = 1`,
	).Scan(&storedAfterFirst)
	if errScanFirst != nil {
		t.Fatalf("QueryRow(first) error = %v", errScanFirst)
	}

	errCloseFirst := dbInstFirst.SQLDb.Close()
	if errCloseFirst != nil {
		t.Fatalf("Close(first connection) error = %v", errCloseFirst)
	}

	dbInstSecond, errSetupSecond := setupDatabase(context.Background(), cfg)
	if errSetupSecond != nil {
		t.Fatalf("setupDatabase(second) error = %v", errSetupSecond)
	}
	t.Cleanup(func() {
		_ = dbInstSecond.SQLDb.Close()
	})

	_, _, errFederationSecond := setupFederationIdentity(context.Background(), dbInstSecond, cfg)
	if errFederationSecond != nil {
		t.Fatalf("setupFederationIdentity(second) error = %v", errFederationSecond)
	}

	var storedAfterSecond string
	errScanSecond := dbInstSecond.SQLDb.QueryRowContext(
		context.Background(),
		`select key_pem from federation_local_identity where id = 1`,
	).Scan(&storedAfterSecond)
	if errScanSecond != nil {
		t.Fatalf("QueryRow(second) error = %v", errScanSecond)
	}
	if storedAfterSecond != storedAfterFirst {
		t.Errorf("stored key_pem changed across restart")
	}
}

func TestSetupFederationIdentityFailsForStoredInvalidIdentity(t *testing.T) {
	dbPath := newStartupTestDatabasePath(t, "setup-federation-identity-invalid.sqlite")

	key, errGenerate := xycrypt.GenerateEncryptionKey()
	if errGenerate != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", errGenerate)
	}

	dbInst, errSetup := setupDatabase(context.Background(), makeStartupTestConfig(dbPath, key))
	if errSetup != nil {
		t.Fatalf("setupDatabase() error = %v", errSetup)
	}
	t.Cleanup(func() {
		_ = dbInst.SQLDb.Close()
	})

	errUpsert := dbInst.UpsertFederationLocalIdentity("stored-node", "not-a-cert", "not-a-key", "fp-invalid")
	if errUpsert != nil {
		t.Fatalf("UpsertFederationLocalIdentity() error = %v", errUpsert)
	}

	_, _, errFederation := setupFederationIdentity(context.Background(), dbInst, Configuration{FederationPort: 8443})
	if errFederation == nil {
		t.Fatal("setupFederationIdentity() error = nil, want error")
	}
	if !strings.Contains(errFederation.Error(), "stored cert") {
		t.Fatalf("setupFederationIdentity() error = %v, want stored identity failure", errFederation)
	}
}
