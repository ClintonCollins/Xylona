package rpc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/gorilla/securecookie"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/mailer"
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
		{
			name: "begin Google OAuth",
			call: func() error {
				req := connect.NewRequest(&xylona.BeginGoogleMailOAuthRequest{})
				_, errBegin := fixture.service.BeginGoogleMailOAuth(context.Background(), req)
				return errBegin
			},
		},
		{
			name: "disconnect Google",
			call: func() error {
				req := connect.NewRequest(&xylona.DisconnectGoogleMailRequest{})
				_, errDisconnect := fixture.service.DisconnectGoogleMail(context.Background(), req)
				return errDisconnect
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
		{
			name: "begin Google OAuth",
			call: func() error {
				req := connect.NewRequest(&xylona.BeginGoogleMailOAuthRequest{})
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-regular")
				_, errBegin := fixture.service.BeginGoogleMailOAuth(context.Background(), req)
				return errBegin
			},
		},
		{
			name: "disconnect Google",
			call: func() error {
				req := connect.NewRequest(&xylona.DisconnectGoogleMailRequest{})
				addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-regular")
				_, errDisconnect := fixture.service.DisconnectGoogleMail(context.Background(), req)
				return errDisconnect
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
	if resp.Msg.GetError() != "Controller email delivery is not configured" {
		t.Errorf("error = %q, want %q", resp.Msg.GetError(), "Controller email delivery is not configured")
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
		if subject != "Xylona Email Delivery Test" {
			t.Errorf("send subject = %q, want %q", subject, "Xylona Email Delivery Test")
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

func TestBeginGoogleMailOAuth(t *testing.T) {
	fixture := newSMTPFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))

	request := connect.NewRequest(&xylona.BeginGoogleMailOAuthRequest{
		ClientId:     "client-id.apps.googleusercontent.com",
		ClientSecret: "client-secret",
		RedirectUri:  "https://xylona.example.com" + GoogleMailOAuthCallbackPath,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-super")

	response, errBegin := fixture.service.BeginGoogleMailOAuth(t.Context(), request)
	if errBegin != nil {
		t.Fatalf("BeginGoogleMailOAuth() error = %v", errBegin)
	}
	parsedURL, errParse := url.Parse(response.Msg.GetAuthorizationUrl())
	if errParse != nil {
		t.Fatalf("url.Parse() error = %v", errParse)
	}
	stateValue := parsedURL.Query().Get("state")
	if stateValue == "" {
		t.Fatal("authorization URL state is empty")
	}

	state, exists := fixture.service.googleMailOAuthStates[stateValue]
	if !exists {
		t.Fatal("OAuth state was not stored")
	}
	if state.userID != "user-super" {
		t.Errorf("state userID = %q, want user-super", state.userID)
	}
	if state.clientSecret != "client-secret" {
		t.Errorf("state clientSecret = %q, want client-secret", state.clientSecret)
	}

	_, stored, errRead := fixture.service.readStoredSystemSMTPConfig()
	if errRead != nil {
		t.Fatalf("readStoredSystemSMTPConfig() error = %v", errRead)
	}
	if stored {
		t.Error("OAuth client credentials were persisted before authorization completed")
	}
}

func TestGoogleMailOAuthCallback(t *testing.T) {
	fixture := newSMTPFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))

	fixture.service.googleMailExchangeFunc = func(
		_ context.Context,
		clientID string,
		clientSecret string,
		redirectURI string,
		code string,
		verifier string,
	) (*mailer.GoogleAuthorization, error) {
		if clientID != "client-id.apps.googleusercontent.com" {
			t.Errorf("clientID = %q, want client-id.apps.googleusercontent.com", clientID)
		}
		if clientSecret != "client-secret" {
			t.Errorf("clientSecret = %q, want client-secret", clientSecret)
		}
		if redirectURI != "https://xylona.example.com"+GoogleMailOAuthCallbackPath {
			t.Errorf("redirectURI = %q", redirectURI)
		}
		if code != "authorization-code" {
			t.Errorf("code = %q, want authorization-code", code)
		}
		if verifier == "" {
			t.Error("verifier is empty")
		}
		return &mailer.GoogleAuthorization{
			RefreshToken: "refresh-token",
			Email:        "sender@example.com",
		}, nil
	}

	beginRequest := connect.NewRequest(&xylona.BeginGoogleMailOAuthRequest{
		ClientId:     "client-id.apps.googleusercontent.com",
		ClientSecret: "client-secret",
		RedirectUri:  "https://xylona.example.com" + GoogleMailOAuthCallbackPath,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, beginRequest, "user-super")
	beginResponse, errBegin := fixture.service.BeginGoogleMailOAuth(t.Context(), beginRequest)
	if errBegin != nil {
		t.Fatalf("BeginGoogleMailOAuth() error = %v", errBegin)
	}
	authorizationURL, errParse := url.Parse(beginResponse.Msg.GetAuthorizationUrl())
	if errParse != nil {
		t.Fatalf("url.Parse() error = %v", errParse)
	}
	stateValue := authorizationURL.Query().Get("state")

	callbackURL := GoogleMailOAuthCallbackPath + "?code=authorization-code&state=" + url.QueryEscape(stateValue)
	httpRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, callbackURL, nil)
	httpResponse := httptest.NewRecorder()

	fixture.service.GoogleMailOAuthCallback(httpResponse, httpRequest)
	if httpResponse.Code != http.StatusOK {
		t.Fatalf("callback status = %d, want %d", httpResponse.Code, http.StatusOK)
	}
	if !strings.Contains(httpResponse.Body.String(), "/admin/settings?google=connected") {
		t.Errorf("callback body = %q, want connected settings transition", httpResponse.Body.String())
	}
	if httpResponse.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", httpResponse.Header().Get("Cache-Control"))
	}
	if httpResponse.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("Referrer-Policy = %q, want no-referrer", httpResponse.Header().Get("Referrer-Policy"))
	}

	storedConfig, stored, errStored := fixture.service.readStoredSystemSMTPConfig()
	if errStored != nil {
		t.Fatalf("readStoredSystemSMTPConfig() error = %v", errStored)
	}
	if !stored {
		t.Fatal("Google mail config was not stored")
	}
	if storedConfig.GetProvider() != xylona.SystemEmailProvider_SYSTEM_EMAIL_PROVIDER_GOOGLE {
		t.Errorf("provider = %v, want Google", storedConfig.GetProvider())
	}
	if storedConfig.GetGoogleRefreshToken() != "refresh-token" {
		t.Errorf("refresh token = %q, want refresh-token", storedConfig.GetGoogleRefreshToken())
	}
	if storedConfig.GetGoogleEmail() != "sender@example.com" {
		t.Errorf("Google email = %q, want sender@example.com", storedConfig.GetGoogleEmail())
	}

	getRequest := connect.NewRequest(&xylona.GetSystemSMTPConfigRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getRequest, "user-super")
	getResponse, errGet := fixture.service.GetSystemSMTPConfig(t.Context(), getRequest)
	if errGet != nil {
		t.Fatalf("GetSystemSMTPConfig() error = %v", errGet)
	}
	if !getResponse.Msg.GetConfigured() || !getResponse.Msg.GetGoogleConnected() {
		t.Errorf("configured = %v, google_connected = %v, want both true", getResponse.Msg.GetConfigured(), getResponse.Msg.GetGoogleConnected())
	}
	if getResponse.Msg.GetConfig().GetGoogleRefreshToken() != "" {
		t.Error("GetSystemSMTPConfig() exposed the Google refresh token")
	}
	if getResponse.Msg.GetConfig().GetGoogleClientSecret() != "" {
		t.Error("GetSystemSMTPConfig() exposed the Google client secret")
	}

	replayRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, callbackURL, nil)
	replayResponse := httptest.NewRecorder()
	fixture.service.GoogleMailOAuthCallback(replayResponse, replayRequest)
	if !strings.Contains(replayResponse.Body.String(), "/admin/settings?google=invalid_state") {
		t.Errorf("replay body = %q, want invalid state transition", replayResponse.Body.String())
	}
}

func TestDisconnectGoogleMail(t *testing.T) {
	fixture := newSMTPFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))

	storedConfig := &xylona.SystemSMTPConfig{
		Provider:           xylona.SystemEmailProvider_SYSTEM_EMAIL_PROVIDER_GOOGLE,
		Host:               "smtp.example.com",
		Port:               587,
		User:               "smtp-user",
		Password:           "smtp-password",
		FromAddress:        "sender@example.com",
		TlsEnabled:         true,
		GoogleClientId:     "client-id",
		GoogleClientSecret: "client-secret",
		GoogleRefreshToken: "refresh-token",
		GoogleEmail:        "sender@gmail.com",
	}
	errStore := fixture.service.writeStoredSystemSMTPConfig(storedConfig)
	if errStore != nil {
		t.Fatalf("writeStoredSystemSMTPConfig() error = %v", errStore)
	}

	var revokedToken string
	fixture.service.googleMailRevokeFunc = func(_ context.Context, refreshToken string) error {
		revokedToken = refreshToken
		return nil
	}

	request := connect.NewRequest(&xylona.DisconnectGoogleMailRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-super")
	_, errDisconnect := fixture.service.DisconnectGoogleMail(t.Context(), request)
	if errDisconnect != nil {
		t.Fatalf("DisconnectGoogleMail() error = %v", errDisconnect)
	}
	if revokedToken != "refresh-token" {
		t.Errorf("revoked token = %q, want refresh-token", revokedToken)
	}

	config, _, errRead := fixture.service.readStoredSystemSMTPConfig()
	if errRead != nil {
		t.Fatalf("readStoredSystemSMTPConfig() error = %v", errRead)
	}
	if config.GetGoogleRefreshToken() != "" || config.GetGoogleEmail() != "" {
		t.Error("disconnect retained Google authorization data")
	}
	if config.GetProvider() != xylona.SystemEmailProvider_SYSTEM_EMAIL_PROVIDER_SMTP {
		t.Errorf("provider = %v, want SMTP fallback", config.GetProvider())
	}
}

func TestValidateGoogleMailRedirectURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		uri     string
		wantErr bool
	}{
		{name: "HTTPS deployment", uri: "https://xylona.example.com" + GoogleMailOAuthCallbackPath},
		{name: "localhost HTTP", uri: "http://localhost:9002" + GoogleMailOAuthCallbackPath},
		{name: "loopback HTTP", uri: "http://127.0.0.1:9002" + GoogleMailOAuthCallbackPath},
		{name: "non-loopback HTTP", uri: "http://xylona.lan" + GoogleMailOAuthCallbackPath, wantErr: true},
		{name: "wrong path", uri: "https://xylona.example.com/admin/settings", wantErr: true},
		{name: "query forbidden", uri: "https://xylona.example.com" + GoogleMailOAuthCallbackPath + "?next=evil", wantErr: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, errValidate := validateGoogleMailRedirectURI(testCase.uri)
			if testCase.wantErr && errValidate == nil {
				t.Fatal("validateGoogleMailRedirectURI() error = nil, want non-nil")
			}
			if !testCase.wantErr && errValidate != nil {
				t.Fatalf("validateGoogleMailRedirectURI() error = %v", errValidate)
			}
		})
	}
}
