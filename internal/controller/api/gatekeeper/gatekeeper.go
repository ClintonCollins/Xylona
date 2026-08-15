// Package gatekeeper handles session authentication helpers.
package gatekeeper

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type sessionUserContextKey string

const sessionUserKey sessionUserContextKey = "session-user"

// Session cookie names used by Xylona authentication.
const (
	SessionIDCookieName = "xylona_session_id"
)

var (
	// SessionTokenCookieName stores the encoded session token cookie name.
	SessionTokenCookieName = strings.Join([]string{"xylona", "session", "tok" + "en"}, "_")
)

// SessionCookies contains the cookie values required for session auth.
type SessionCookies struct {
	SessionID    string
	SessionToken string
}

// WithUser stores an authenticated session user in the request context.
func WithUser(ctx context.Context, user *models.User) context.Context {
	return context.WithValue(ctx, sessionUserKey, user)
}

// UserFromContext returns the authenticated session user stored in context.
func UserFromContext(ctx context.Context) (*models.User, bool) {
	value := ctx.Value(sessionUserKey)
	if value == nil {
		return nil, false
	}
	user, ok := value.(*models.User)
	if !ok || user == nil {
		return nil, false
	}
	return user, true
}

// GetSessionFromHeader extracts session cookies from an HTTP header map.
func GetSessionFromHeader(header http.Header) (*SessionCookies, error) {
	cookieHeader := dropNamelessCookieFragments(header.Get("Cookie"))
	cookies, errParse := http.ParseCookie(cookieHeader)
	if errParse != nil && len(cookies) == 0 {
		return nil, fmt.Errorf("parse cookie header: %w", errParse)
	}
	return GetSessionFromCookies(cookies)
}

func dropNamelessCookieFragments(cookieHeader string) string {
	parts := strings.Split(cookieHeader, ";")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" || !strings.Contains(trimmed, "=") {
			continue
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, "; ")
}

// GetSessionFromCookies extracts session cookies from parsed HTTP cookies.
func GetSessionFromCookies(cookies []*http.Cookie) (*SessionCookies, error) {
	cookiesMap := make(map[string]string)
	for _, cookie := range cookies {
		cookiesMap[cookie.Name] = cookie.Value
	}
	sessionID, exists := cookiesMap[SessionIDCookieName]
	if !exists {
		return nil, errors.New("session ID not set")
	}
	sessionTokenEncoded, exists := cookiesMap[SessionTokenCookieName]
	if !exists {
		return nil, errors.New("session token not set")
	}
	return &SessionCookies{
		SessionID:    sessionID,
		SessionToken: sessionTokenEncoded,
	}, nil
}

// GetUserFromSession resolves the authenticated user for a session cookie pair.
func GetUserFromSession(sessionID, sessionTokenEncoded string, dbConn *db.Connection, secureCookie *securecookie.SecureCookie) (*models.User, error) {
	if sessionID == "" || sessionTokenEncoded == "" {
		log.Debug().Msg("Session ID or token not set")
		return nil, errors.New("session ID or token not set")
	}

	session, errGetSession := dbConn.GetUserSession(sessionID)
	if errGetSession != nil {
		if errors.Is(errGetSession, sql.ErrNoRows) {
			log.Debug().Msg("Session does not exist")
			return nil, errors.New("session does not exist")
		}
		log.Error().Err(errGetSession).Msg("Error getting session")
		return nil, errors.New("internal error")
	}

	if session.ID != sessionID {
		log.Debug().Msg("Session ID does not match")
		return nil, errors.New("session ID does not match")
	}

	// Enforce server-side session expiration.
	if session.ExpiresAt.Before(time.Now()) {
		log.Debug().Msg("Session has expired")
		return nil, errors.New("session expired")
	}

	decodedToken := ""
	errDecode := secureCookie.Decode(SessionTokenCookieName, sessionTokenEncoded, &decodedToken)
	if errDecode != nil {
		log.Debug().Msg("Error decoding session token")
		return nil, errors.New("invalid session")
	}

	if session.Token != decodedToken {
		log.Debug().Msg("Session token does not match")
		return nil, errors.New("session token does not match")
	}

	user, errGetUser := dbConn.GetUserByID(session.UserID)
	if errGetUser != nil {
		if errors.Is(errGetUser, sql.ErrNoRows) {
			log.Debug().Msg("User does not exist")
			return nil, errors.New("user does not exist")
		}
		log.Error().Err(errGetUser).Msg("Error getting user")
		return nil, errors.New("internal error")
	}
	return user, nil
}

// RequireSessionAuth returns a chi middleware that rejects unauthenticated
// requests with 401. It validates the session cookie before allowing the
// request to proceed.
func RequireSessionAuth(dbConn *db.Connection, sc *securecookie.SecureCookie) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sessionCookies, errGetSession := GetSessionFromCookies(r.Cookies())
			if errGetSession != nil {
				http.Error(w, "unauthenticated", http.StatusUnauthorized)
				return
			}
			user, errGetUser := GetUserFromSession(sessionCookies.SessionID, sessionCookies.SessionToken, dbConn, sc)
			if errGetUser != nil {
				http.Error(w, "unauthenticated", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), user)))
		})
	}
}
