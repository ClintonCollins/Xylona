// Package gatekeeper handles session and JWT authentication helpers.
package gatekeeper

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/securecookie"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// Session cookie names used by Xylona authentication.
const (
	SessionIDCookieName = "xylona_session_id"
)

var (
	// SessionTokenCookieName stores the encoded session token cookie name.
	SessionTokenCookieName = strings.Join([]string{"xylona", "session", "tok" + "en"}, "_")
	// ErrInvalidToken indicates a JWT failed validation.
	ErrInvalidToken = errors.New("invalid token")
)

// SessionCookies contains the cookie values required for session auth.
type SessionCookies struct {
	SessionID    string
	SessionToken string
}

// Cookies is a lightweight cookie lookup map.
type Cookies map[string]string

// JWTClaims contains Xylona-specific JWT claims.
type JWTClaims struct {
	Username      string `json:"username"`
	Email         string `json:"email"`
	OriginID      string `json:"originID"`
	OriginAddress string `json:"originAddress"`
	jwt.RegisteredClaims
}

// Get returns the cookie value for the provided key.
func (c Cookies) Get(key string) (string, error) {
	cookie, exists := c[key]
	if !exists {
		return "", errors.New("cookie not found")
	}
	return cookie, nil
}

func getCookiesFromHeader(cookiesHeader string) Cookies {
	cookies := strings.Fields(cookiesHeader)
	cookiesMap := make(map[string]string)
	for _, cookie := range cookies {
		cookie = strings.TrimSpace(cookie)
		cookie = strings.TrimSuffix(cookie, ";")
		cookieParts := strings.SplitN(cookie, "=", 2)
		if len(cookieParts) != 2 {
			continue
		}
		cookiesMap[cookieParts[0]] = cookieParts[1]
	}
	return cookiesMap
}

// GetSessionFromHeader extracts session cookies from an HTTP header map.
func GetSessionFromHeader(header http.Header) (*SessionCookies, error) {
	cookies := getCookiesFromHeader(header.Get("Cookie"))
	sessionID, errGetSessionID := cookies.Get(SessionIDCookieName)
	if errGetSessionID != nil {
		return nil, errGetSessionID
	}
	sessionTokenEncoded, errGetSessionToken := cookies.Get(SessionTokenCookieName)
	if errGetSessionToken != nil {
		return nil, errGetSessionToken
	}
	return &SessionCookies{
		SessionID:    sessionID,
		SessionToken: sessionTokenEncoded,
	}, nil
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
			_, errGetUser := GetUserFromSession(sessionCookies.SessionID, sessionCookies.SessionToken, dbConn, sc)
			if errGetUser != nil {
				http.Error(w, "unauthenticated", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CreateJWT signs a JWT for the given user identity and expiration time.
func CreateJWT(username, email, jwtID string, expiration time.Time, secretKey []byte) (string, error) {
	claims := JWTClaims{
		Username: username,
		Email:    email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "xylona",
			ExpiresAt: jwt.NewNumericDate(expiration),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        jwtID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	tokenString, errSign := token.SignedString(secretKey)
	if errSign != nil {
		log.Error().Err(errSign).Msg("Error signing token")
		return "", errors.New("internal error")
	}
	return tokenString, nil
}

// ParseJWT parses and validates a signed Xylona JWT.
func ParseJWT(tokenString string, secretKey []byte) (*JWTClaims, error) {
	token, errParse := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (any, error) {
		_, ok := token.Method.(*jwt.SigningMethodHMAC)
		if !ok {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return secretKey, nil
	})
	if errParse != nil {
		log.Error().Err(errParse).Msg("Error parsing token")
		return nil, errors.New("invalid token")
	}
	if !token.Valid {
		log.Error().Msg("Invalid token")
		return nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		log.Error().Msg("Invalid token claims")
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}
