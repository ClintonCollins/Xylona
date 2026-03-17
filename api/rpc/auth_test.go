package rpc

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestLoginCookieSecureAttribute(t *testing.T) {
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
		tt := tt
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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)
			xs := &XylonaService{
				db:            fixture.conn,
				secureCookie:  fixture.secureCookie,
				secureCookies: tt.secureCookies,
			}

			user := createUserForRPCUserTests(t, fixture, "logoutsecure", false)

			req := connect.NewRequest(&xylona.LogoutRequest{})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, user.Id)

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
	if resp.Msg == nil || resp.Msg.User == nil {
		t.Fatalf("Login() returned empty response")
	}
	if resp.Msg.User.Id != user.Id {
		t.Errorf("Login().User.Id = %q, want %q", resp.Msg.User.Id, user.Id)
	}
	if resp.Msg.User.UserName != "login-valid" {
		t.Errorf("Login().User.UserName = %q, want %q", resp.Msg.User.UserName, "login-valid")
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

func TestLogoutWithValidSession(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	user := createUserForRPCUserTests(t, fixture, "logout-valid", false)

	req := connect.NewRequest(&xylona.LogoutRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, user.Id)

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
	fixture := newRBACRPCFixture(t)

	user := createUserForRPCUserTests(t, fixture, "check-auth-valid", false)

	req := connect.NewRequest(&xylona.CheckUserAuthenticatedRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, user.Id)

	resp, errCheck := fixture.service.CheckUserAuthenticated(context.Background(), req)
	if errCheck != nil {
		t.Fatalf("CheckUserAuthenticated() error = %v", errCheck)
	}
	if resp.Msg == nil {
		t.Fatalf("CheckUserAuthenticated() returned nil message")
	}
	if !resp.Msg.Authenticated {
		t.Errorf("CheckUserAuthenticated().Authenticated = false, want true")
	}
	if resp.Msg.User == nil {
		t.Fatalf("CheckUserAuthenticated().User is nil")
	}
	if resp.Msg.User.Id != user.Id {
		t.Errorf("CheckUserAuthenticated().User.Id = %q, want %q", resp.Msg.User.Id, user.Id)
	}
	if resp.Msg.User.UserName != "check-auth-valid" {
		t.Errorf("CheckUserAuthenticated().User.UserName = %q, want %q", resp.Msg.User.UserName, "check-auth-valid")
	}
}

func TestCheckUserAuthenticatedWithoutSession(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.CheckUserAuthenticatedRequest{})

	resp, errCheck := fixture.service.CheckUserAuthenticated(context.Background(), req)
	if errCheck != nil {
		t.Fatalf("CheckUserAuthenticated() error = %v", errCheck)
	}
	if resp.Msg == nil {
		t.Fatalf("CheckUserAuthenticated() returned nil message")
	}
	if resp.Msg.Authenticated {
		t.Errorf("CheckUserAuthenticated().Authenticated = true, want false")
	}
	if resp.Msg.User != nil {
		t.Errorf("CheckUserAuthenticated().User should be nil for unauthenticated request")
	}
}
