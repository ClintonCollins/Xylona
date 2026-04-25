package gatekeeper

import (
	"context"
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

func TestGetCookiesFromHeaderIgnoresMalformedFragments(t *testing.T) {
	cookies := getCookiesFromHeader(`xylona_session_id=session-123; malformed-cookie; xylona_session_token=encoded-token`)

	if got := cookies[SessionIDCookieName]; got != "session-123" {
		t.Fatalf("session ID cookie = %q, want %q", got, "session-123")
	}
	if got := cookies[SessionTokenCookieName]; got != "encoded-token" {
		t.Fatalf("session token cookie = %q, want %q", got, "encoded-token")
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
		name          string
		origin        string
		referer       string
		forwardedHost string
		wantStatus    int
		wantCalled    bool
		requestURL    string
		requestHost   string
		requestProto  string
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
			name:         "foreign origin is rejected",
			origin:       "https://evil.test",
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
			name:          "forwarded host is used behind a reverse proxy",
			origin:        "https://xylona.test",
			forwardedHost: "xylona.test",
			wantStatus:    http.StatusNoContent,
			wantCalled:    true,
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
			if tt.requestProto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.requestProto)
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

func TestCreateJWTAndParseJWTRoundTrip(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	expiration := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)

	tokenString, errCreate := CreateJWT("gatekeeper-user", "gatekeeper@example.com", "jwt-123", expiration, secret)
	if errCreate != nil {
		t.Fatalf("CreateJWT() error = %v", errCreate)
	}

	claims, errParse := ParseJWT(tokenString, secret)
	if errParse != nil {
		t.Fatalf("ParseJWT() error = %v", errParse)
	}

	if claims.Username != "gatekeeper-user" {
		t.Fatalf("claims.Username = %q, want %q", claims.Username, "gatekeeper-user")
	}
	if claims.Email != "gatekeeper@example.com" {
		t.Fatalf("claims.Email = %q, want %q", claims.Email, "gatekeeper@example.com")
	}
	if claims.ID != "jwt-123" {
		t.Fatalf("claims.ID = %q, want %q", claims.ID, "jwt-123")
	}
	if claims.Issuer != "xylona" {
		t.Fatalf("claims.Issuer = %q, want %q", claims.Issuer, "xylona")
	}

	gotExpiresAt := claims.ExpiresAt.Time.UTC().Truncate(time.Second)
	if !gotExpiresAt.Equal(expiration) {
		t.Fatalf("claims.ExpiresAt = %s, want %s", gotExpiresAt, expiration)
	}

	tamperedToken := tokenString + "-tampered"
	_, errParseTampered := ParseJWT(tamperedToken, secret)
	if errParseTampered == nil {
		t.Fatal("ParseJWT() tampered token error = nil, want invalid token")
	}
}
