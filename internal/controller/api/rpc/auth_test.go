package rpc

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"golang.org/x/crypto/bcrypt"

	"github.com/ClintonCollins/Xylona/internal/controller/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestLoginCookieSecureAttribute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		secureCookies bool
		wantSecure    bool
	}{
		{
			name:          "secure cookies enabled",
			secureCookies: true,
			wantSecure:    true,
		},
		{
			name:          "secure cookies disabled",
			secureCookies: false,
			wantSecure:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)
			xs := &XylonaService{
				db:            fixture.conn,
				secureCookie:  fixture.secureCookie,
				secureCookies: tt.secureCookies,
			}

			// Create a user via the fixture's service so the password hash is set properly.
			user := createUserForRPCUserTests(t, fixture, "securecookieuser", false)
			_ = user

			req := connect.NewRequest(&xylona.LoginRequest{
				UserName: "securecookieuser",
				Password: "password123",
			})

			resp, errLogin := xs.Login(context.Background(), req)
			if errLogin != nil {
				t.Fatalf("Login() error = %v", errLogin)
			}

			setCookies := resp.Header().Values("Set-Cookie")
			if len(setCookies) < 2 {
				t.Fatalf("Expected at least 2 Set-Cookie headers, got %d", len(setCookies))
			}

			for _, cookie := range setCookies {
				isToken := strings.Contains(cookie, gatekeeper.SessionTokenCookieName+"=")
				isID := strings.Contains(cookie, gatekeeper.SessionIDCookieName+"=")
				if !isToken && !isID {
					continue
				}

				hasSecure := strings.Contains(cookie, "Secure")
				if tt.wantSecure && !hasSecure {
					t.Errorf("Cookie %q: expected Secure flag to be set, but it was not", cookie)
				}
				if !tt.wantSecure && hasSecure {
					t.Errorf("Cookie %q: expected Secure flag to be absent, but it was set", cookie)
				}
			}
		})
	}
}

func TestLogoutCookieSecureAttribute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		secureCookies bool
		wantSecure    bool
	}{
		{
			name:          "secure cookies enabled on logout",
			secureCookies: true,
			wantSecure:    true,
		},
		{
			name:          "secure cookies disabled on logout",
			secureCookies: false,
			wantSecure:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)
			xs := &XylonaService{
				db:            fixture.conn,
				secureCookie:  fixture.secureCookie,
				secureCookies: tt.secureCookies,
			}

			user := createUserForRPCUserTests(t, fixture, "logoutsecure", false)

			req := connect.NewRequest(&xylona.LogoutRequest{})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, user.GetId())

			resp, errLogout := xs.Logout(context.Background(), req)
			if errLogout != nil {
				t.Fatalf("Logout() error = %v", errLogout)
			}

			setCookies := resp.Header().Values("Set-Cookie")
			if len(setCookies) < 2 {
				t.Fatalf("Expected at least 2 Set-Cookie headers, got %d", len(setCookies))
			}

			for _, cookie := range setCookies {
				isToken := strings.Contains(cookie, gatekeeper.SessionTokenCookieName+"=")
				isID := strings.Contains(cookie, gatekeeper.SessionIDCookieName+"=")
				if !isToken && !isID {
					continue
				}

				hasSecure := strings.Contains(cookie, "Secure")
				if tt.wantSecure && !hasSecure {
					t.Errorf("Cookie %q: expected Secure flag to be set, but it was not", cookie)
				}
				if !tt.wantSecure && hasSecure {
					t.Errorf("Cookie %q: expected Secure flag to be absent, but it was set", cookie)
				}
			}
		})
	}
}

func TestLoginValidCredentials(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	user := createUserForRPCUserTests(t, fixture, "login-valid", false)

	req := connect.NewRequest(&xylona.LoginRequest{
		UserName: "login-valid",
		Password: "password123",
	})

	resp, errLogin := fixture.service.Login(context.Background(), req)
	if errLogin != nil {
		t.Fatalf("Login() error = %v", errLogin)
	}
	if resp.Msg == nil || resp.Msg.GetUser() == nil {
		t.Fatalf("Login() returned empty response")
	}
	if resp.Msg.GetUser().GetId() != user.GetId() {
		t.Errorf("Login().User.Id = %q, want %q", resp.Msg.GetUser().GetId(), user.GetId())
	}
	if resp.Msg.GetUser().GetUserName() != "login-valid" {
		t.Errorf("Login().User.UserName = %q, want %q", resp.Msg.GetUser().GetUserName(), "login-valid")
	}

	setCookies := resp.Header().Values("Set-Cookie")
	if len(setCookies) < 2 {
		t.Fatalf("expected at least 2 Set-Cookie headers, got %d", len(setCookies))
	}

	foundToken := false
	foundID := false
	for _, cookie := range setCookies {
		if strings.Contains(cookie, gatekeeper.SessionTokenCookieName+"=") {
			foundToken = true
		}
		if strings.Contains(cookie, gatekeeper.SessionIDCookieName+"=") {
			foundID = true
		}
	}
	if !foundToken {
		t.Errorf("missing %s Set-Cookie header", gatekeeper.SessionTokenCookieName)
	}
	if !foundID {
		t.Errorf("missing %s Set-Cookie header", gatekeeper.SessionIDCookieName)
	}
}

func TestLoginInvalidPassword(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	_ = createUserForRPCUserTests(t, fixture, "login-badpass", false)

	req := connect.NewRequest(&xylona.LoginRequest{
		UserName: "login-badpass",
		Password: "wrong-password",
	})

	_, errLogin := fixture.service.Login(context.Background(), req)
	if errLogin == nil {
		t.Fatalf("Login() expected error, got nil")
	}
	if connect.CodeOf(errLogin) != connect.CodeUnauthenticated {
		t.Errorf("Login() code = %v, want %v", connect.CodeOf(errLogin), connect.CodeUnauthenticated)
	}
}

func TestLoginNonExistentUser(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.LoginRequest{
		UserName: "does-not-exist",
		Password: "password123",
	})

	_, errLogin := fixture.service.Login(context.Background(), req)
	if errLogin == nil {
		t.Fatalf("Login() expected error, got nil")
	}
	if connect.CodeOf(errLogin) != connect.CodeUnauthenticated {
		t.Errorf("Login() code = %v, want %v", connect.CodeOf(errLogin), connect.CodeUnauthenticated)
	}
}

func TestLoginRejectsLegacyBcryptHash(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	legacyHash, errLegacyHash := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	if errLegacyHash != nil {
		t.Fatalf("GenerateFromPassword() error = %v", errLegacyHash)
	}

	now := time.Now().UTC()
	_, errCreateUser := fixture.conn.CreateUser(&models.UserSetter{
		ID:           omit.From("user-legacy-bcrypt"),
		UserName:     omit.From("legacy-bcrypt"),
		Email:        omit.From("legacy-bcrypt@example.com"),
		FirstName:    omit.From("Legacy"),
		LastName:     omit.From("User"),
		PasswordHash: omit.From(string(legacyHash)),
		SuperUser:    omit.From(false),
		LastLoginAt:  omit.From(now),
		CreatedAt:    omit.From(now),
		UpdatedAt:    omit.From(now),
	})
	if errCreateUser != nil {
		t.Fatalf("CreateUser() error = %v", errCreateUser)
	}

	req := connect.NewRequest(&xylona.LoginRequest{
		UserName: "legacy-bcrypt",
		Password: "password123",
	})

	_, errLogin := fixture.service.Login(context.Background(), req)
	if errLogin == nil {
		t.Fatal("Login() expected error for legacy bcrypt hash, got nil")
	}
	if connect.CodeOf(errLogin) != connect.CodeUnauthenticated {
		t.Errorf("Login() code = %v, want %v", connect.CodeOf(errLogin), connect.CodeUnauthenticated)
	}
}

func TestLogoutWithValidSession(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	user := createUserForRPCUserTests(t, fixture, "logout-valid", false)

	req := connect.NewRequest(&xylona.LogoutRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, user.GetId())

	cookies := getCookiesFromHeader(req.Header().Get("Cookie"))
	sessionID := cookies[gatekeeper.SessionIDCookieName]
	if sessionID == "" {
		t.Fatal("session ID not set in request")
	}

	resp, errLogout := fixture.service.Logout(context.Background(), req)
	if errLogout != nil {
		t.Fatalf("Logout() error = %v", errLogout)
	}

	setCookies := resp.Header().Values("Set-Cookie")
	if len(setCookies) < 2 {
		t.Fatalf("expected at least 2 Set-Cookie headers, got %d", len(setCookies))
	}

	_, errGetSession := fixture.conn.GetUserSession(sessionID)
	if !errors.Is(errGetSession, sql.ErrNoRows) {
		t.Errorf("session still exists after logout: error = %v, want %v", errGetSession, sql.ErrNoRows)
	}
}

func TestLogoutWithoutSessionCookies(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.LogoutRequest{})

	resp, errLogout := fixture.service.Logout(context.Background(), req)
	if errLogout != nil {
		t.Fatalf("Logout() error = %v", errLogout)
	}

	setCookies := resp.Header().Values("Set-Cookie")
	if len(setCookies) < 2 {
		t.Fatalf("expected at least 2 Set-Cookie headers, got %d", len(setCookies))
	}
}

func TestCheckUserAuthenticatedWithValidSession(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	user := createUserForRPCUserTests(t, fixture, "check-auth-valid", false)

	req := connect.NewRequest(&xylona.CheckUserAuthenticatedRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, user.GetId())

	resp, errCheck := fixture.service.CheckUserAuthenticated(context.Background(), req)
	if errCheck != nil {
		t.Fatalf("CheckUserAuthenticated() error = %v", errCheck)
	}
	if resp.Msg == nil {
		t.Fatalf("CheckUserAuthenticated() returned nil message")
	}
	if !resp.Msg.GetAuthenticated() {
		t.Errorf("CheckUserAuthenticated().Authenticated = false, want true")
	}
	if resp.Msg.GetUser() == nil {
		t.Fatalf("CheckUserAuthenticated().User is nil")
	}
	if resp.Msg.GetUser().GetId() != user.GetId() {
		t.Errorf("CheckUserAuthenticated().User.Id = %q, want %q", resp.Msg.GetUser().GetId(), user.GetId())
	}
	if resp.Msg.GetUser().GetUserName() != "check-auth-valid" {
		t.Errorf("CheckUserAuthenticated().User.UserName = %q, want %q", resp.Msg.GetUser().GetUserName(), "check-auth-valid")
	}
}

func TestCheckUserAuthenticatedIncludesGlobalPermissionIDs(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	user := createUserForRPCUserTests(t, fixture, "check-auth-permissions", false)

	_, errRole := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`INSERT INTO role (id, name, description, is_system) VALUES (?, ?, ?, ?)`,
		"role-alert-viewer", "Alert Viewer", "Can view alert history", false,
	)
	if errRole != nil {
		t.Fatalf("failed to insert role: %v", errRole)
	}

	_, errManagePerm := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`INSERT INTO role_permission (role_id, permission_id) VALUES (?, ?)`,
		"role-alert-viewer", "alerts.manage",
	)
	if errManagePerm != nil {
		t.Fatalf("failed to insert alerts.manage role_permission: %v", errManagePerm)
	}

	_, errHistoryPerm := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`INSERT INTO role_permission (role_id, permission_id) VALUES (?, ?)`,
		"role-alert-viewer", permissionAlertsViewHistory,
	)
	if errHistoryPerm != nil {
		t.Fatalf("failed to insert alerts.view_history role_permission: %v", errHistoryPerm)
	}

	errAssign := fixture.conn.CreateUserRoleAssignment(
		"assignment-check-auth-permissions",
		user.GetId(),
		"role-alert-viewer",
		"",
		"user-admin",
	)
	if errAssign != nil {
		t.Fatalf("CreateUserRoleAssignment() error = %v", errAssign)
	}

	req := connect.NewRequest(&xylona.CheckUserAuthenticatedRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, user.GetId())

	resp, errCheck := fixture.service.CheckUserAuthenticated(context.Background(), req)
	if errCheck != nil {
		t.Fatalf("CheckUserAuthenticated() error = %v", errCheck)
	}
	if resp.Msg == nil {
		t.Fatalf("CheckUserAuthenticated() returned nil message")
	}

	gotPerms := make(map[string]struct{}, len(resp.Msg.GetPermissionIds()))
	for _, permissionID := range resp.Msg.GetPermissionIds() {
		gotPerms[permissionID] = struct{}{}
	}

	for _, permissionID := range []string{"alerts.manage", permissionAlertsViewHistory} {
		if _, ok := gotPerms[permissionID]; !ok {
			t.Errorf("CheckUserAuthenticated().PermissionIds missing %q; got %v", permissionID, resp.Msg.GetPermissionIds())
		}
	}
}

func TestCheckUserAuthenticatedReturnsInternalErrorWhenPermissionLookupFails(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	user := createUserForRPCUserTests(t, fixture, "check-auth-permission-error", false)
	fixture.service.permissionIDsForUserFn = func(_ *models.User) ([]string, error) {
		return nil, errors.New("permission lookup failed")
	}

	req := connect.NewRequest(&xylona.CheckUserAuthenticatedRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, user.GetId())

	_, errCheck := fixture.service.CheckUserAuthenticated(context.Background(), req)
	if errCheck == nil {
		t.Fatal("CheckUserAuthenticated() error = nil, want internal error")
	}
	if connect.CodeOf(errCheck) != connect.CodeInternal {
		t.Fatalf("CheckUserAuthenticated() code = %v, want %v", connect.CodeOf(errCheck), connect.CodeInternal)
	}
}

func TestCheckUserAuthenticatedWithoutSession(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.CheckUserAuthenticatedRequest{})

	resp, errCheck := fixture.service.CheckUserAuthenticated(context.Background(), req)
	if errCheck != nil {
		t.Fatalf("CheckUserAuthenticated() error = %v", errCheck)
	}
	if resp.Msg == nil {
		t.Fatalf("CheckUserAuthenticated() returned nil message")
	}
	if resp.Msg.GetAuthenticated() {
		t.Errorf("CheckUserAuthenticated().Authenticated = true, want false")
	}
	if resp.Msg.GetUser() != nil {
		t.Errorf("CheckUserAuthenticated().User should be nil for unauthenticated request")
	}
}
