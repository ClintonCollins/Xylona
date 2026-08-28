package rpc

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/controller/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestLogout(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	xs := &XylonaService{
		db:           fixture.conn,
		secureCookie: fixture.secureCookie,
	}
	closedSessionID := ""
	xs.closeSession = func(sessionID string) {
		closedSessionID = sessionID
	}

	// 1. Create a user and a session
	user := createUserForRPCUserTests(t, fixture, "logoutuser", false)

	req := connect.NewRequest(&xylona.LogoutRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, user.GetId())

	sessionCookies, errSession := gatekeeper.GetSessionFromHeader(req.Header())
	if errSession != nil {
		t.Fatalf("GetSessionFromHeader() error = %v", errSession)
	}
	sessionID := sessionCookies.SessionID
	if sessionID == "" {
		t.Fatal("session ID not set in request")
	}

	// 2. Call Logout
	resp, err := xs.Logout(context.Background(), req)
	if err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	// 3. Verify cookies are cleared in response
	setCookies := resp.Header().Values("Set-Cookie")
	if len(setCookies) < 2 {
		t.Errorf("Expected at least 2 Set-Cookie headers, got %d", len(setCookies))
	}

	foundIDClear := false
	foundTokenClear := false
	for _, cookie := range setCookies {
		if strings.Contains(cookie, gatekeeper.SessionIDCookieName+"=;") || strings.Contains(cookie, gatekeeper.SessionIDCookieName+"= ") {
			if strings.Contains(cookie, "Max-Age=-1") || strings.Contains(cookie, "Expires=Thu, 01 Jan 1970 00:00:00 GMT") {
				foundIDClear = true
			}
		}
		if strings.Contains(cookie, gatekeeper.SessionTokenCookieName+"=;") || strings.Contains(cookie, gatekeeper.SessionTokenCookieName+"= ") {
			if strings.Contains(cookie, "Max-Age=-1") || strings.Contains(cookie, "Expires=Thu, 01 Jan 1970 00:00:00 GMT") {
				foundTokenClear = true
			}
		}
	}

	if !foundIDClear {
		t.Error("Session ID cookie not cleared in response")
	}
	if !foundTokenClear {
		t.Error("Session token cookie not cleared in response")
	}

	// 4. Verify session is deleted from DB
	_, err = fixture.conn.GetUserSession(sessionID)
	if err == nil {
		t.Error("Session still exists in DB after logout")
	}
	if closedSessionID != sessionID {
		t.Errorf("closed session ID = %q, want %q", closedSessionID, sessionID)
	}
}
