package rpc

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestLogout(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	xs := &XylonaService{
		db:           fixture.conn,
		secureCookie: fixture.secureCookie,
	}

	// 1. Create a user and a session
	user := createUserForRPCUserTests(t, fixture, "logoutuser", false)

	req := connect.NewRequest(&xylona.LogoutRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, user.Id)

	// Get session ID from header to verify deletion later
	cookies := getCookiesFromHeader(req.Header().Get("Cookie"))
	sessionID := cookies[gatekeeper.SessionIDCookieName]
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
}

func getCookiesFromHeader(cookiesHeader string) map[string]string {
	cookies := strings.Split(cookiesHeader, ";")
	cookiesMap := make(map[string]string)
	for _, cookie := range cookies {
		cookie = strings.TrimSpace(cookie)
		cookieParts := strings.Split(cookie, "=")
		if len(cookieParts) != 2 {
			continue
		}
		cookiesMap[cookieParts[0]] = cookieParts[1]
	}
	return cookiesMap
}
