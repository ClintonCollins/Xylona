package gatekeeper

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/gorilla/securecookie"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/db/dbtest"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func newGatekeeperTestConnection(t *testing.T) (*db.Connection, *securecookie.SecureCookie) {
	t.Helper()

	conn := dbtest.NewMigratedConnection(t, "gatekeeper-auth.sqlite")
	secureCookieInst := securecookie.New(
		[]byte("0123456789abcdef0123456789abcdef"),
		[]byte("0123456789abcdef"),
	)

	return conn, secureCookieInst
}

func createGatekeeperTestUser(t *testing.T, conn *db.Connection, userID string, username string) *models.User {
	t.Helper()

	now := time.Now().UTC()
	user, errCreate := conn.CreateUser(&models.UserSetter{
		ID:           omit.From(userID),
		UserName:     omit.From(username),
		Email:        omit.From(username + "@example.com"),
		FirstName:    omit.From("Gate"),
		LastName:     omit.From("Keeper"),
		PasswordHash: omit.From("hash"),
		SuperUser:    omit.From(false),
		LastLoginAt:  omit.From(now),
		CreatedAt:    omit.From(now),
		UpdatedAt:    omit.From(now),
	})
	if errCreate != nil {
		t.Fatalf("CreateUser() error = %v", errCreate)
	}

	return user
}

func createGatekeeperTestSession(
	t *testing.T,
	conn *db.Connection,
	secureCookieInst *securecookie.SecureCookie,
	userID string,
	sessionID string,
	sessionToken string,
	expiresAt time.Time,
) string {
	t.Helper()

	now := time.Now().UTC()
	_, errCreateSession := conn.CreateUserSession(&models.UserSessionSetter{
		ID:        omit.From(sessionID),
		UserID:    omit.From(userID),
		Token:     omit.From(sessionToken),
		CreatedAt: omit.From(now),
		UpdatedAt: omit.From(now),
		ExpiresAt: omit.From(expiresAt),
	})
	if errCreateSession != nil {
		t.Fatalf("CreateUserSession() error = %v", errCreateSession)
	}

	encodedToken, errEncode := secureCookieInst.Encode(SessionTokenCookieName, sessionToken)
	if errEncode != nil {
		t.Fatalf("securecookie.Encode() error = %v", errEncode)
	}

	return encodedToken
}

func cookieHeader(sessionID string, sessionToken string) string {
	return SessionIDCookieName + "=" + sessionID + "; " + SessionTokenCookieName + "=" + sessionToken
}

func TestGetSessionFromHeaderIgnoresMalformedFragments(t *testing.T) {
	header := http.Header{}
	header.Set("Cookie", `xylona_session_id=session-123; malformed-cookie; xylona_session_token=encoded-token`)

	cookies, errGetSession := GetSessionFromHeader(header)
	if errGetSession != nil {
		t.Fatalf("GetSessionFromHeader() error = %v", errGetSession)
	}
	if cookies.SessionID != "session-123" {
		t.Fatalf("GetSessionFromHeader().SessionID = %q, want %q", cookies.SessionID, "session-123")
	}
	if cookies.SessionToken != "encoded-token" {
		t.Fatalf("GetSessionFromHeader().SessionToken = %q, want %q", cookies.SessionToken, "encoded-token")
	}
}

func TestGetSessionFromHeaderParsesNoSpaceAfterSemicolon(t *testing.T) {
	header := http.Header{}
	header.Set("Cookie", SessionIDCookieName+"=session-123;"+SessionTokenCookieName+"=encoded-token")

	cookies, errGetSession := GetSessionFromHeader(header)
	if errGetSession != nil {
		t.Fatalf("GetSessionFromHeader() error = %v", errGetSession)
	}
	if cookies.SessionID != "session-123" {
		t.Fatalf("GetSessionFromHeader().SessionID = %q, want %q", cookies.SessionID, "session-123")
	}
	if cookies.SessionToken != "encoded-token" {
		t.Fatalf("GetSessionFromHeader().SessionToken = %q, want %q", cookies.SessionToken, "encoded-token")
	}
}

func TestGetUserFromSession(t *testing.T) {
	conn, secureCookieInst := newGatekeeperTestConnection(t)
	user := createGatekeeperTestUser(t, conn, "user-gatekeeper", "gatekeeper")

	tests := []struct {
		name          string
		sessionID     string
		sessionToken  string
		expiresAt     time.Time
		tamperToken   bool
		tokenMismatch bool
		wantErr       string
		wantUserID    string
	}{
		{
			name:         "valid session returns user",
			sessionID:    "session-valid",
			sessionToken: "token-valid",
			expiresAt:    time.Now().UTC().Add(time.Hour),
			wantUserID:   user.ID,
		},
		{
			name:         "expired session is rejected",
			sessionID:    "session-expired",
			sessionToken: "token-expired",
			expiresAt:    time.Now().UTC().Add(-time.Hour),
			wantErr:      "session expired",
		},
		{
			name:         "tampered encoded token is rejected",
			sessionID:    "session-tampered",
			sessionToken: "token-tampered",
			expiresAt:    time.Now().UTC().Add(time.Hour),
			tamperToken:  true,
			wantErr:      "invalid session",
		},
		{
			name:          "decoded token mismatch is rejected",
			sessionID:     "session-mismatch",
			sessionToken:  "token-stored",
			expiresAt:     time.Now().UTC().Add(time.Hour),
			tokenMismatch: true,
			wantErr:       "session token does not match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encodedToken := createGatekeeperTestSession(
				t,
				conn,
				secureCookieInst,
				user.ID,
				tt.sessionID,
				tt.sessionToken,
				tt.expiresAt,
			)

			switch {
			case tt.tamperToken:
				encodedToken = encodedToken + "-tampered"
			case tt.tokenMismatch:
				mismatchedEncodedToken, errEncode := secureCookieInst.Encode(SessionTokenCookieName, "different-token")
				if errEncode != nil {
					t.Fatalf("securecookie.Encode() mismatch error = %v", errEncode)
				}
				encodedToken = mismatchedEncodedToken
			}

			gotUser, errGetUser := GetUserFromSession(tt.sessionID, encodedToken, conn, secureCookieInst)
			if tt.wantErr != "" {
				if errGetUser == nil {
					t.Fatalf("GetUserFromSession() error = nil, want %q", tt.wantErr)
				}
				if errGetUser.Error() != tt.wantErr {
					t.Fatalf("GetUserFromSession() error = %q, want %q", errGetUser.Error(), tt.wantErr)
				}
				return
			}

			if errGetUser != nil {
				t.Fatalf("GetUserFromSession() error = %v", errGetUser)
			}
			if gotUser == nil {
				t.Fatal("GetUserFromSession() returned nil user")
			}
			if gotUser.ID != tt.wantUserID {
				t.Fatalf("GetUserFromSession().ID = %q, want %q", gotUser.ID, tt.wantUserID)
			}
		})
	}
}

func TestGetUserFromSessionRejectsIdleTimeout(t *testing.T) {
	conn, secureCookieInst := newGatekeeperTestConnection(t)
	user := createGatekeeperTestUser(t, conn, "user-idle", "idle")
	encodedToken := createGatekeeperTestSession(
		t,
		conn,
		secureCookieInst,
		user.ID,
		"session-idle",
		"token-idle",
		time.Now().UTC().Add(30*24*time.Hour),
	)

	staleAt := time.Now().UTC().Add(-SessionIdleTimeout - time.Hour).Format("2006-01-02 15:04:05")
	_, errStale := conn.SQLDb.ExecContext(
		context.Background(),
		`update user_session set updated_at = ? where id = ?`,
		staleAt,
		"session-idle",
	)
	if errStale != nil {
		t.Fatalf("update stale session error = %v", errStale)
	}

	_, errGetUser := GetUserFromSession("session-idle", encodedToken, conn, secureCookieInst)
	if errGetUser == nil {
		t.Fatal("GetUserFromSession() error = nil, want idle timeout")
	}
	if errGetUser.Error() != "session expired" {
		t.Fatalf("GetUserFromSession() error = %q, want %q", errGetUser.Error(), "session expired")
	}
}

func TestValidateUserFromSessionDoesNotRecordActivity(t *testing.T) {
	conn, secureCookieInst := newGatekeeperTestConnection(t)
	user := createGatekeeperTestUser(t, conn, "user-passive", "passive")
	encodedToken := createGatekeeperTestSession(
		t,
		conn,
		secureCookieInst,
		user.ID,
		"session-passive",
		"token-passive",
		time.Now().UTC().Add(time.Hour),
	)

	staleAt := time.Now().UTC().Add(-2 * sessionActivityTouchInterval).Format("2006-01-02 15:04:05")
	_, errStale := conn.SQLDb.ExecContext(
		context.Background(),
		`update user_session set updated_at = ? where id = ?`,
		staleAt,
		"session-passive",
	)
	if errStale != nil {
		t.Fatalf("update passive session error = %v", errStale)
	}

	_, errValidate := ValidateUserFromSession("session-passive", encodedToken, conn, secureCookieInst)
	if errValidate != nil {
		t.Fatalf("ValidateUserFromSession() error = %v", errValidate)
	}

	session, errSession := conn.GetUserSession("session-passive")
	if errSession != nil {
		t.Fatalf("GetUserSession() error = %v", errSession)
	}
	if session.UpdatedAt.Format("2006-01-02 15:04:05") != staleAt {
		t.Fatalf("ValidateUserFromSession() updated activity to %s, want %s", session.UpdatedAt, staleAt)
	}
}

func TestRequireSessionAuth(t *testing.T) {
	conn, secureCookieInst := newGatekeeperTestConnection(t)
	user := createGatekeeperTestUser(t, conn, "user-session-middleware", "middleware")
	encodedToken := createGatekeeperTestSession(
		t,
		conn,
		secureCookieInst,
		user.ID,
		"session-middleware",
		"middleware-token",
		time.Now().UTC().Add(time.Hour),
	)

	handlerCalled := false
	handler := RequireSessionAuth(conn, secureCookieInst)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true

		gotUser, ok := UserFromContext(r.Context())
		if !ok {
			t.Fatal("expected authenticated user in request context")
		}
		if gotUser.ID != user.ID {
			t.Fatalf("UserFromContext().ID = %q, want %q", gotUser.ID, user.ID)
		}

		w.WriteHeader(http.StatusNoContent)
	}))

	t.Run("valid session reaches handler", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/protected", nil)
		req.Header.Set("Cookie", cookieHeader("session-middleware", encodedToken))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
		if !handlerCalled {
			t.Fatal("expected handler to be called for valid session")
		}
	})

	t.Run("missing cookies are rejected", func(t *testing.T) {
		handlerCalled = false

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/protected", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if handlerCalled {
			t.Fatal("handler should not be called for missing cookies")
		}
	})
}

func TestRequireSameOriginFormRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		origin         string
		referer        string
		forwardedHost  string
		forwardedProto string
		secFetchSite   string
		wantStatus     int
		wantCalled     bool
		requestURL     string
		requestHost    string
		requestProto   string
	}{
		{
			name:         "same origin is allowed",
			origin:       "https://xylona.test",
			wantStatus:   http.StatusNoContent,
			wantCalled:   true,
			requestURL:   "http://xylona.test/api/file/upload",
			requestHost:  "xylona.test",
			requestProto: "https",
		},
		{
			name:           "same-origin fetch metadata is allowed through HTTPS proxy",
			origin:         "https://xylona.test",
			forwardedHost:  "xylona.test",
			forwardedProto: "https",
			secFetchSite:   "same-origin",
			wantStatus:     http.StatusNoContent,
			wantCalled:     true,
			requestURL:     "http://internal.proxy.local/api/file/upload",
			requestHost:    "internal.proxy.local",
		},
		{
			name:           "same host and forwarded HTTPS are allowed without fetch metadata",
			origin:         "https://xylona.test",
			forwardedProto: "https",
			wantStatus:     http.StatusNoContent,
			wantCalled:     true,
			requestURL:     "http://xylona.test/api/file/upload",
			requestHost:    "xylona.test",
		},
		{
			name:         "foreign origin is rejected",
			origin:       "https://evil.test",
			secFetchSite: "cross-site",
			wantStatus:   http.StatusForbidden,
			wantCalled:   false,
			requestURL:   "http://xylona.test/api/file/upload",
			requestHost:  "xylona.test",
			requestProto: "https",
		},
		{
			name:         "foreign referer is rejected",
			referer:      "https://evil.test/upload",
			wantStatus:   http.StatusForbidden,
			wantCalled:   false,
			requestURL:   "http://xylona.test/api/file/upload",
			requestHost:  "xylona.test",
			requestProto: "https",
		},
		{
			name:         "missing origin and referer are allowed",
			wantStatus:   http.StatusNoContent,
			wantCalled:   true,
			requestURL:   "http://xylona.test/api/file/upload",
			requestHost:  "xylona.test",
			requestProto: "https",
		},
		{
			name:          "untrusted forwarded host is ignored",
			origin:        "https://xylona.test",
			forwardedHost: "xylona.test",
			wantStatus:    http.StatusForbidden,
			wantCalled:    false,
			requestURL:    "http://internal.proxy.local/api/file/upload",
			requestHost:   "internal.proxy.local",
			requestProto:  "https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handlerCalled := false
			handler := RequireSameOriginFormRequests()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				handlerCalled = true
				w.WriteHeader(http.StatusNoContent)
			}))

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, tt.requestURL, nil)
			req.Host = tt.requestHost
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.referer != "" {
				req.Header.Set("Referer", tt.referer)
			}
			if tt.forwardedHost != "" {
				req.Header.Set("X-Forwarded-Host", tt.forwardedHost)
			}
			if tt.forwardedProto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.forwardedProto)
			}
			if tt.secFetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tt.secFetchSite)
			}
			if tt.requestProto == "https" {
				req.TLS = &tls.ConnectionState{}
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if handlerCalled != tt.wantCalled {
				t.Fatalf("handler called = %t, want %t", handlerCalled, tt.wantCalled)
			}
		})
	}
}

func TestRequireSameOriginFormRequestsTrustedProxy(t *testing.T) {
	t.Parallel()

	trust, errTrust := ParseTrustedProxies("127.0.0.1")
	if errTrust != nil {
		t.Fatalf("ParseTrustedProxies() error = %v", errTrust)
	}

	handlerCalled := false
	handler := RequireSameOriginFormRequestsForProxies(trust)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://internal.proxy.local/api/file/upload", nil)
	req.Host = "internal.proxy.local"
	req.RemoteAddr = "127.0.0.1:443"
	req.Header.Set("Origin", "https://xylona.test")
	req.Header.Set("X-Forwarded-Host", "xylona.test")
	req.Header.Set("X-Forwarded-Proto", "https")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if !handlerCalled {
		t.Fatal("expected handler to be called behind a trusted proxy")
	}
}
