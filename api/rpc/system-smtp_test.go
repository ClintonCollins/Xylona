package rpc

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/gorilla/securecookie"
	migrate "github.com/rubenv/sql-migrate"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/pkg/mailer"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

type smtpFixture struct {
	conn         *db.Connection
	service      XylonaService
	secureCookie *securecookie.SecureCookie
}

func newSMTPFixture(t *testing.T) *smtpFixture {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "smtp-rpc.sqlite")
	conn := db.NewConnection(context.Background(), dbPath)
	t.Cleanup(func() {
		errClose := conn.SQLDb.Close()
		if errClose != nil {
			t.Errorf("failed to close test db: %v", errClose)
		}
	})

	migrationSource := &migrate.FileMigrationSource{
		Dir: filepath.Join("..", "..", "sql", "migrations"),
	}
	migrate.SetTable("migrations")
	_, errMigrate := migrate.Exec(conn.SQLDb, "sqlite3", migrationSource, migrate.Up)
	if errMigrate != nil {
		t.Fatalf("failed to apply migrations: %v", errMigrate)
	}
	_, errAlterGame := conn.SQLDb.ExecContext(
		context.Background(),
		`alter table game add column binds_to_all_ips boolean not null default false`,
	)
	if errAlterGame != nil && !strings.Contains(strings.ToLower(errAlterGame.Error()), "duplicate column name") {
		t.Fatalf("failed to ensure game.binds_to_all_ips column: %v", errAlterGame)
	}

	seedSMTPFixture(t, conn)

	secureCookieInst := securecookie.New(
		[]byte("0123456789abcdef0123456789abcdef"),
		[]byte("0123456789abcdef"),
	)

	service := XylonaService{
		ctx:          context.Background(),
		db:           conn,
		secureCookie: secureCookieInst,
		listCache:    newRemoteServerListCache(remoteServerListCacheTTL),
	}

	return &smtpFixture{
		conn:         conn,
		service:      service,
		secureCookie: secureCookieInst,
	}
}

// seedSMTPFixture creates:
// - "node-local": local node
// - local_settings pointing to node-local
// - "user-super": superuser
// - "user-regular": non-super user
func seedSMTPFixture(t *testing.T, conn *db.Connection) {
	t.Helper()

	ctx := context.Background()

	_, errNode := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO node (id, name, is_local, host, port, base_url, enabled) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"node-local", "Local Node", true, "localhost", 8080, "http://localhost:8080", true,
	)
	if errNode != nil {
		t.Fatalf("failed to insert node: %v", errNode)
	}

	_, errSettings := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO local_settings (id, node_id) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET node_id = excluded.node_id`,
		"node-local",
	)
	if errSettings != nil {
		t.Fatalf("failed to insert local_settings: %v", errSettings)
	}

	_, errIP := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO ip (address, usable, external) VALUES (?, ?, ?)`,
		"127.0.0.1", true, false,
	)
	if errIP != nil {
		t.Fatalf("failed to insert ip: %v", errIP)
	}

	// Superuser
	_, errSuper := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'), datetime('now'))`,
		"user-super", "superadmin", "super@example.com", "Super", "User", "hash", true,
	)
	if errSuper != nil {
		t.Fatalf("failed to insert superuser: %v", errSuper)
	}

	// Regular (non-super) user
	_, errRegular := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'), datetime('now'))`,
		"user-regular", "regular", "regular@example.com", "Regular", "User", "hash", false,
	)
	if errRegular != nil {
		t.Fatalf("failed to insert regular user: %v", errRegular)
	}
}

// ---------------------------------------------------------------------------
// GetSystemSMTPConfig — auth tests
// ---------------------------------------------------------------------------

func TestGetSystemSMTPConfig_Unauthenticated(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.GetSystemSMTPConfigRequest{})

	_, errGet := fixture.service.GetSystemSMTPConfig(context.Background(), req)
	if errGet == nil {
		t.Fatalf("GetSystemSMTPConfig(unauthenticated) expected error, got nil")
	}
	if connect.CodeOf(errGet) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v", connect.CodeOf(errGet), connect.CodeUnauthenticated)
	}
}

func TestGetSystemSMTPConfig_NonSuperuser(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.GetSystemSMTPConfigRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-regular")

	_, errGet := fixture.service.GetSystemSMTPConfig(context.Background(), req)
	if errGet == nil {
		t.Fatalf("GetSystemSMTPConfig(non-super) expected error, got nil")
	}
	if connect.CodeOf(errGet) != connect.CodePermissionDenied {
		t.Errorf("code = %v, want %v", connect.CodeOf(errGet), connect.CodePermissionDenied)
	}
}

// ---------------------------------------------------------------------------
// SetSystemSMTPConfig — auth tests
// ---------------------------------------------------------------------------

func TestSetSystemSMTPConfig_Unauthenticated(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "mail.example.com",
			Port:        587,
			FromAddress: "noreply@example.com",
		},
	})

	_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), req)
	if errSet == nil {
		t.Fatalf("SetSystemSMTPConfig(unauthenticated) expected error, got nil")
	}
	if connect.CodeOf(errSet) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v", connect.CodeOf(errSet), connect.CodeUnauthenticated)
	}
}

func TestSetSystemSMTPConfig_NonSuperuser(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "mail.example.com",
			Port:        587,
			FromAddress: "noreply@example.com",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-regular")

	_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), req)
	if errSet == nil {
		t.Fatalf("SetSystemSMTPConfig(non-super) expected error, got nil")
	}
	if connect.CodeOf(errSet) != connect.CodePermissionDenied {
		t.Errorf("code = %v, want %v", connect.CodeOf(errSet), connect.CodePermissionDenied)
	}
}

// ---------------------------------------------------------------------------
// TestSystemSMTP — auth tests
// ---------------------------------------------------------------------------

func TestTestSystemSMTP_Unauthenticated(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.TestSystemSMTPRequest{
		ToAddress: "test@example.com",
	})

	_, errTest := fixture.service.TestSystemSMTP(context.Background(), req)
	if errTest == nil {
		t.Fatalf("TestSystemSMTP(unauthenticated) expected error, got nil")
	}
	if connect.CodeOf(errTest) != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want %v", connect.CodeOf(errTest), connect.CodeUnauthenticated)
	}
}

func TestTestSystemSMTP_NonSuperuser(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.TestSystemSMTPRequest{
		ToAddress: "test@example.com",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-regular")

	_, errTest := fixture.service.TestSystemSMTP(context.Background(), req)
	if errTest == nil {
		t.Fatalf("TestSystemSMTP(non-super) expected error, got nil")
	}
	if connect.CodeOf(errTest) != connect.CodePermissionDenied {
		t.Errorf("code = %v, want %v", connect.CodeOf(errTest), connect.CodePermissionDenied)
	}
}

// ---------------------------------------------------------------------------
// GetSystemSMTPConfig — not configured
// ---------------------------------------------------------------------------

func TestGetSystemSMTPConfig_NotConfigured(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.GetSystemSMTPConfigRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	resp, errGet := fixture.service.GetSystemSMTPConfig(context.Background(), req)
	if errGet != nil {
		t.Fatalf("GetSystemSMTPConfig(not configured) error = %v", errGet)
	}
	if resp.Msg.Configured {
		t.Errorf("configured = true, want false")
	}
	if resp.Msg.Config != nil {
		t.Errorf("config = %v, want nil", resp.Msg.Config)
	}
}

// ---------------------------------------------------------------------------
// SetSystemSMTPConfig + GetSystemSMTPConfig — round-trip
// ---------------------------------------------------------------------------

func TestSetAndGetSystemSMTPConfig_RoundTrip(t *testing.T) {
	fixture := newSMTPFixture(t)

	setReq := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "smtp.example.com",
			Port:        465,
			User:        "smtpuser",
			Password:    "secret123",
			FromAddress: "noreply@example.com",
			TlsEnabled:  true,
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, setReq, "user-super")

	_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), setReq)
	if errSet != nil {
		t.Fatalf("SetSystemSMTPConfig() error = %v", errSet)
	}

	getReq := connect.NewRequest(&xylona.GetSystemSMTPConfigRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getReq, "user-super")

	resp, errGet := fixture.service.GetSystemSMTPConfig(context.Background(), getReq)
	if errGet != nil {
		t.Fatalf("GetSystemSMTPConfig() error = %v", errGet)
	}
	if !resp.Msg.Configured {
		t.Fatalf("configured = false, want true")
	}

	got := resp.Msg.Config
	if got == nil {
		t.Fatalf("config = nil, want non-nil")
	}
	if got.Host != "smtp.example.com" {
		t.Errorf("host = %q, want %q", got.Host, "smtp.example.com")
	}
	if got.Port != 465 {
		t.Errorf("port = %d, want %d", got.Port, 465)
	}
	if got.User != "smtpuser" {
		t.Errorf("user = %q, want %q", got.User, "smtpuser")
	}
	if got.Password != "********" {
		t.Errorf("password = %q, want %q", got.Password, "********")
	}
	if got.FromAddress != "noreply@example.com" {
		t.Errorf("from_address = %q, want %q", got.FromAddress, "noreply@example.com")
	}
	if !got.TlsEnabled {
		t.Errorf("tls_enabled = false, want true")
	}
}

// ---------------------------------------------------------------------------
// SetSystemSMTPConfig — masked password preservation
// ---------------------------------------------------------------------------

func TestSetSystemSMTPConfig_MaskedPasswordPreservesOriginal(t *testing.T) {
	fixture := newSMTPFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))

	// Step 1: Save a config with a real password.
	setReq := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "smtp.example.com",
			Port:        587,
			User:        "smtpuser",
			Password:    "realPassword123",
			FromAddress: "noreply@example.com",
			TlsEnabled:  true,
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, setReq, "user-super")

	_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), setReq)
	if errSet != nil {
		t.Fatalf("SetSystemSMTPConfig(initial) error = %v", errSet)
	}

	// Step 2: Re-save with the masked placeholder (simulating the frontend
	// round-trip where the user does not change the password).
	updateReq := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "smtp.example.com",
			Port:        587,
			User:        "smtpuser",
			Password:    "********",
			FromAddress: "noreply@example.com",
			TlsEnabled:  true,
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-super")

	_, errUpdate := fixture.service.SetSystemSMTPConfig(context.Background(), updateReq)
	if errUpdate != nil {
		t.Fatalf("SetSystemSMTPConfig(masked) error = %v", errUpdate)
	}

	// Step 3: Read back from DB directly to verify the real password is preserved.
	jsonStr, errGet := fixture.conn.GetSystemConfig(systemSMTPConfigKey)
	if errGet != nil {
		t.Fatalf("GetSystemConfig() error = %v", errGet)
	}

	if !strings.Contains(jsonStr, "realPassword123") {
		t.Errorf("stored config should contain original password, got: %s", jsonStr)
	}
	if strings.Contains(jsonStr, "********") {
		t.Errorf("stored config should not contain masked password, got: %s", jsonStr)
	}
}

func TestSetSystemSMTPConfig_NewPasswordOverwritesExisting(t *testing.T) {
	fixture := newSMTPFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))

	// Step 1: Save with initial password.
	setReq := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "smtp.example.com",
			Port:        587,
			User:        "smtpuser",
			Password:    "oldPassword",
			FromAddress: "noreply@example.com",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, setReq, "user-super")

	_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), setReq)
	if errSet != nil {
		t.Fatalf("SetSystemSMTPConfig(initial) error = %v", errSet)
	}

	// Step 2: Save with a new real password (not the mask).
	updateReq := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "smtp.example.com",
			Port:        587,
			User:        "smtpuser",
			Password:    "brandNewPassword",
			FromAddress: "noreply@example.com",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-super")

	_, errUpdate := fixture.service.SetSystemSMTPConfig(context.Background(), updateReq)
	if errUpdate != nil {
		t.Fatalf("SetSystemSMTPConfig(new password) error = %v", errUpdate)
	}

	// Step 3: Verify the new password is stored.
	jsonStr, errGet := fixture.conn.GetSystemConfig(systemSMTPConfigKey)
	if errGet != nil {
		t.Fatalf("GetSystemConfig() error = %v", errGet)
	}

	if !strings.Contains(jsonStr, "brandNewPassword") {
		t.Errorf("stored config should contain new password, got: %s", jsonStr)
	}
	if strings.Contains(jsonStr, "oldPassword") {
		t.Errorf("stored config should not contain old password, got: %s", jsonStr)
	}
}

// ---------------------------------------------------------------------------
// SetSystemSMTPConfig — validation
// ---------------------------------------------------------------------------

func TestSetSystemSMTPConfig_MissingHost(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "",
			Port:        587,
			FromAddress: "noreply@example.com",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), req)
	if errSet == nil {
		t.Fatalf("SetSystemSMTPConfig(missing host) expected error, got nil")
	}
	if connect.CodeOf(errSet) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errSet), connect.CodeInvalidArgument)
	}
}

func TestSetSystemSMTPConfig_MissingPort(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "mail.example.com",
			Port:        0,
			FromAddress: "noreply@example.com",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), req)
	if errSet == nil {
		t.Fatalf("SetSystemSMTPConfig(missing port) expected error, got nil")
	}
	if connect.CodeOf(errSet) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errSet), connect.CodeInvalidArgument)
	}
}

func TestSetSystemSMTPConfig_MissingFromAddress(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "mail.example.com",
			Port:        587,
			FromAddress: "",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), req)
	if errSet == nil {
		t.Fatalf("SetSystemSMTPConfig(missing from_address) expected error, got nil")
	}
	if connect.CodeOf(errSet) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errSet), connect.CodeInvalidArgument)
	}
}

// ---------------------------------------------------------------------------
// TestSystemSMTP — validation and stub
// ---------------------------------------------------------------------------

func TestTestSystemSMTP_EmptyToAddress(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.TestSystemSMTPRequest{
		ToAddress: "",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errTest := fixture.service.TestSystemSMTP(context.Background(), req)
	if errTest == nil {
		t.Fatalf("TestSystemSMTP(empty to_address) expected error, got nil")
	}
	if connect.CodeOf(errTest) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errTest), connect.CodeInvalidArgument)
	}
}

func TestSetSystemSMTPConfig_NilConfig(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), req)
	if errSet == nil {
		t.Fatalf("SetSystemSMTPConfig(nil config) expected error, got nil")
	}
	if connect.CodeOf(errSet) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errSet), connect.CodeInvalidArgument)
	}
}

func TestSetSystemSMTPConfig_InvalidPort(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "smtp.example.com",
			Port:        99999,
			FromAddress: "test@example.com",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), req)
	if errSet == nil {
		t.Fatalf("SetSystemSMTPConfig(invalid port) expected error, got nil")
	}
	if connect.CodeOf(errSet) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errSet), connect.CodeInvalidArgument)
	}
}

func TestTestSystemSMTP_NotConfigured(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.TestSystemSMTPRequest{
		ToAddress: "recipient@example.com",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	resp, errTest := fixture.service.TestSystemSMTP(context.Background(), req)
	if errTest != nil {
		t.Fatalf("TestSystemSMTP(not configured) error = %v", errTest)
	}
	if resp.Msg.Success {
		t.Errorf("success = true, want false")
	}
	if resp.Msg.Error != "SMTP is not configured" {
		t.Errorf("error = %q, want %q", resp.Msg.Error, "SMTP is not configured")
	}
}

func TestTestSystemSMTP_SendSuccess(t *testing.T) {
	fixture := newSMTPFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))

	// Store an SMTP config first.
	setReq := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "smtp.example.com",
			Port:        587,
			User:        "smtpuser",
			Password:    "secret",
			FromAddress: "noreply@example.com",
			TlsEnabled:  true,
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, setReq, "user-super")

	_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), setReq)
	if errSet != nil {
		t.Fatalf("SetSystemSMTPConfig() error = %v", errSet)
	}

	// Inject a fake send function that succeeds.
	fixture.service.testEmailSendFunc = func(_ context.Context, cfg *mailer.SMTPConfig, to string, subject string, _ string) error {
		if cfg.Host != "smtp.example.com" {
			t.Errorf("send config host = %q, want %q", cfg.Host, "smtp.example.com")
		}
		if to != "recipient@example.com" {
			t.Errorf("send to = %q, want %q", to, "recipient@example.com")
		}
		if subject != "Xylona SMTP Test" {
			t.Errorf("send subject = %q, want %q", subject, "Xylona SMTP Test")
		}
		return nil
	}

	req := connect.NewRequest(&xylona.TestSystemSMTPRequest{
		ToAddress: "recipient@example.com",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	resp, errTest := fixture.service.TestSystemSMTP(context.Background(), req)
	if errTest != nil {
		t.Fatalf("TestSystemSMTP() error = %v", errTest)
	}
	if !resp.Msg.Success {
		t.Errorf("success = false, want true")
	}
	if resp.Msg.Error != "" {
		t.Errorf("error = %q, want empty", resp.Msg.Error)
	}
}

func TestTestSystemSMTP_SendFailure(t *testing.T) {
	fixture := newSMTPFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))

	// Store an SMTP config first.
	setReq := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "smtp.example.com",
			Port:        587,
			FromAddress: "noreply@example.com",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, setReq, "user-super")

	_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), setReq)
	if errSet != nil {
		t.Fatalf("SetSystemSMTPConfig() error = %v", errSet)
	}

	// Inject a fake send function that fails.
	fixture.service.testEmailSendFunc = func(_ context.Context, _ *mailer.SMTPConfig, _ string, _ string, _ string) error {
		return errors.New("connection refused")
	}

	req := connect.NewRequest(&xylona.TestSystemSMTPRequest{
		ToAddress: "recipient@example.com",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	resp, errTest := fixture.service.TestSystemSMTP(context.Background(), req)
	if errTest != nil {
		t.Fatalf("TestSystemSMTP() error = %v", errTest)
	}
	if resp.Msg.Success {
		t.Errorf("success = true, want false")
	}
	if resp.Msg.Error != "connection refused" {
		t.Errorf("error = %q, want %q", resp.Msg.Error, "connection refused")
	}
}
