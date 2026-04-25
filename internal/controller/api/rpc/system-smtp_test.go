package rpc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/gorilla/securecookie"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/pkg/mailer"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

type smtpFixture struct {
	conn         *db.Connection
	service      *XylonaService
	secureCookie *securecookie.SecureCookie
}

func newSMTPFixture(t *testing.T) *smtpFixture {
	t.Helper()

	conn := newRPCFixtureConnection(t, "smtp-rpc.sqlite")

	seedSMTPFixture(t, conn)

	secureCookieInst := securecookie.New(
		[]byte("0123456789abcdef0123456789abcdef"),
		[]byte("0123456789abcdef"),
	)

	service := &XylonaService{
		ctx:          context.Background(),
		db:           conn,
		secureCookie: secureCookieInst,
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
// - "user-regular": non-super user.
func seedSMTPFixture(t *testing.T, conn *db.Connection) {
	t.Helper()

	ctx := context.Background()

	_, errNode := conn.SQLDb.ExecContext(ctx,
		`INSERT INTO node (id, name, listen_url, enabled) VALUES (?, ?, ?, ?)`,
		"node-local", "Local Node", "http://localhost:8080", true,
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
		`INSERT INTO ip (address, usable, external, node_id) VALUES (?, ?, ?, ?)`,
		"127.0.0.1", true, false, "node-local",
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

func TestSystemSMTPRPC_Unauthenticated(t *testing.T) {
	t.Parallel()

	fixture := newSMTPFixture(t)

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "get config",
			call: func() error {
				req := connect.NewRequest(&xylona.GetSystemSMTPConfigRequest{})
				_, errGet := fixture.service.GetSystemSMTPConfig(context.Background(), req)
				return errGet
			},
		},
		{
			name: "set config",
			call: func() error {
				req := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
					Config: &xylona.SystemSMTPConfig{
						Host:        "mail.example.com",
						Port:        587,
						FromAddress: "noreply@example.com",
					},
				})
				_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), req)
				return errSet
			},
		},
		{
			name: "test smtp",
			call: func() error {
				req := connect.NewRequest(&xylona.TestSystemSMTPRequest{
					ToAddress: "test@example.com",
				})
				_, errTest := fixture.service.TestSystemSMTP(context.Background(), req)
				return errTest
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errCall := tt.call()
			if errCall == nil {
				t.Fatalf("%s expected error, got nil", tt.name)
			}

			code := connect.CodeOf(errCall)
			if code != connect.CodeUnauthenticated {
				t.Errorf("%s code = %v, want %v", tt.name, code, connect.CodeUnauthenticated)
			}
		})
	}
}

func TestSystemSMTPRPC_NonSuperuser(t *testing.T) {
	t.Parallel()

	fixture := newSMTPFixture(t)

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "get config",
			call: func() error {
				req := connect.NewRequest(&xylona.GetSystemSMTPConfigRequest{})
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-regular")
				_, errGet := fixture.service.GetSystemSMTPConfig(context.Background(), req)
				return errGet
			},
		},
		{
			name: "set config",
			call: func() error {
				req := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
					Config: &xylona.SystemSMTPConfig{
						Host:        "mail.example.com",
						Port:        587,
						FromAddress: "noreply@example.com",
					},
				})
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-regular")
				_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), req)
				return errSet
			},
		},
		{
			name: "test smtp",
			call: func() error {
				req := connect.NewRequest(&xylona.TestSystemSMTPRequest{
					ToAddress: "test@example.com",
				})
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-regular")
				_, errTest := fixture.service.TestSystemSMTP(context.Background(), req)
				return errTest
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errCall := tt.call()
			if errCall == nil {
				t.Fatalf("%s expected error, got nil", tt.name)
			}

			code := connect.CodeOf(errCall)
			if code != connect.CodePermissionDenied {
				t.Errorf("%s code = %v, want %v", tt.name, code, connect.CodePermissionDenied)
			}
		})
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
	if resp.Msg.GetConfigured() {
		t.Errorf("configured = true, want false")
	}
	if resp.Msg.GetConfig() != nil {
		t.Errorf("config = %v, want nil", resp.Msg.GetConfig())
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
	if !resp.Msg.GetConfigured() {
		t.Fatalf("configured = false, want true")
	}

	got := resp.Msg.GetConfig()
	if got == nil {
		t.Fatalf("config = nil, want non-nil")
	}
	if got.GetHost() != "smtp.example.com" {
		t.Errorf("host = %q, want %q", got.GetHost(), "smtp.example.com")
	}
	if got.GetPort() != 465 {
		t.Errorf("port = %d, want %d", got.GetPort(), 465)
	}
	if got.GetUser() != "smtpuser" {
		t.Errorf("user = %q, want %q", got.GetUser(), "smtpuser")
	}
	if got.GetPassword() != "" {
		t.Errorf("password = %q, want empty string", got.GetPassword())
	}
	if got.GetFromAddress() != "noreply@example.com" {
		t.Errorf("from_address = %q, want %q", got.GetFromAddress(), "noreply@example.com")
	}
	if !got.GetTlsEnabled() {
		t.Errorf("tls_enabled = false, want true")
	}
	if !resp.Msg.GetPasswordConfigured() {
		t.Errorf("password_configured = false, want true")
	}
}

// ---------------------------------------------------------------------------
// SetSystemSMTPConfig — empty password preservation
// ---------------------------------------------------------------------------

func TestSetSystemSMTPConfig_EmptyPasswordPreservesOriginal(t *testing.T) {
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

	// Step 2: Re-save with an empty password (simulating the write-only UI
	// behavior where the user does not change the password).
	updateReq := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "smtp.example.com",
			Port:        587,
			User:        "smtpuser",
			Password:    "",
			FromAddress: "noreply@example.com",
			TlsEnabled:  true,
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-super")

	_, errUpdate := fixture.service.SetSystemSMTPConfig(context.Background(), updateReq)
	if errUpdate != nil {
		t.Fatalf("SetSystemSMTPConfig(empty password) error = %v", errUpdate)
	}

	// Step 3: Read back from DB directly to verify the real password is preserved.
	jsonStr, errGet := fixture.conn.GetSystemConfig(systemSMTPConfigKey)
	if errGet != nil {
		t.Fatalf("GetSystemConfig() error = %v", errGet)
	}

	if !strings.Contains(jsonStr, "realPassword123") {
		t.Errorf("stored config should contain original password, got: %s", jsonStr)
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

func TestSetSystemSMTPConfig_MissingUser(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "mail.example.com",
			Port:        587,
			User:        "",
			Password:    "secret123",
			FromAddress: "noreply@example.com",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), req)
	if errSet == nil {
		t.Fatal("SetSystemSMTPConfig(missing user) expected error, got nil")
	}
	if connect.CodeOf(errSet) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want %v", connect.CodeOf(errSet), connect.CodeInvalidArgument)
	}
}

func TestSetSystemSMTPConfig_MissingPasswordOnInitialSetup(t *testing.T) {
	fixture := newSMTPFixture(t)

	req := connect.NewRequest(&xylona.SetSystemSMTPConfigRequest{
		Config: &xylona.SystemSMTPConfig{
			Host:        "mail.example.com",
			Port:        587,
			User:        "mailer",
			Password:    "",
			FromAddress: "noreply@example.com",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-super")

	_, errSet := fixture.service.SetSystemSMTPConfig(context.Background(), req)
	if errSet == nil {
		t.Fatal("SetSystemSMTPConfig(missing password on initial setup) expected error, got nil")
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
	if resp.Msg.GetSuccess() {
		t.Errorf("success = true, want false")
	}
	if resp.Msg.GetError() != "SMTP is not configured" {
		t.Errorf("error = %q, want %q", resp.Msg.GetError(), "SMTP is not configured")
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
	if !resp.Msg.GetSuccess() {
		t.Errorf("success = false, want true")
	}
	if resp.Msg.GetError() != "" {
		t.Errorf("error = %q, want empty", resp.Msg.GetError())
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
			User:        "smtpuser",
			Password:    "secret",
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

	_, errTest := fixture.service.TestSystemSMTP(context.Background(), req)
	if errTest == nil {
		t.Fatal("TestSystemSMTP() error = nil, want non-nil")
	}
	if connect.CodeOf(errTest) != connect.CodeUnavailable {
		t.Fatalf("TestSystemSMTP() code = %v, want %v", connect.CodeOf(errTest), connect.CodeUnavailable)
	}
	if !strings.Contains(errTest.Error(), "connection refused") {
		t.Errorf("error = %q, want to contain %q", errTest.Error(), "connection refused")
	}
}
