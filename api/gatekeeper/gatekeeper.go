package gatekeeper

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gorilla/securecookie"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	SessionIDCookieName    = "xylona_session_id"
	SessionTokenCookieName = "xylona_session_token"
)

type SessionCookies struct {
	SessionID    string
	SessionToken string
}

type Cookies map[string]string

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
		cookieParts := strings.Split(cookie, "=")
		if len(cookieParts) != 2 {
			continue
		}
		cookiesMap[cookieParts[0]] = cookieParts[1]
	}
	return cookiesMap
}

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

func GetUserFromSession(sessionID, sessionTokenEncoded string, dbConn *db.Connection, secureCookie *securecookie.SecureCookie) (*models.User, error) {
	if sessionID == "" || sessionTokenEncoded == "" {
		log.Debug().Str("sessionID", sessionID).Str("sessionToken", sessionTokenEncoded).Msg("Session ID or token not set")
		return nil, errors.New("session ID or token not set")
	}

	session, errGetSession := dbConn.GetUserSession(sessionID)
	if errGetSession != nil {
		if errors.Is(errGetSession, sql.ErrNoRows) {
			log.Debug().Str("sessionID", sessionID).Str("sessionToken", sessionTokenEncoded).Msg("Session does not exist")
			return nil, errors.New("session does not exist")
		}
		log.Error().Err(errGetSession).Msg("Error getting session")
		return nil, errors.New("internal error")
	}

	if session.ID != sessionID {
		log.Debug().Str("sessionID", sessionID).Str("sessionToken", sessionTokenEncoded).Msg("Session ID does not match")
		return nil, errors.New("session ID does not match")
	}

	decodedToken := ""
	errDecode := secureCookie.Decode(SessionTokenCookieName, sessionTokenEncoded, &decodedToken)
	if errDecode != nil {
		log.Debug().Str("sessionID", sessionID).Str("sessionToken", sessionTokenEncoded).Msg("Error decoding session token")
		return nil, errors.New("invalid session")
	}

	if session.Token != decodedToken {
		log.Debug().Str("sessionID", sessionID).Str("sessionToken", sessionTokenEncoded).Msg("Session token does not match")
		return nil, errors.New("session token does not match")
	}

	user, errGetUser := dbConn.GetUserByID(session.UserID)
	if errGetUser != nil {
		if errors.Is(errGetUser, sql.ErrNoRows) {
			log.Debug().Str("sessionID", sessionID).Str("sessionToken", sessionTokenEncoded).Msg("User does not exist")
			return nil, errors.New("user does not exist")
		}
		log.Error().Err(errGetUser).Msg("Error getting user")
		return nil, errors.New("internal error")
	}
	return user, nil
}
