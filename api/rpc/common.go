package rpc

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	connect_go "github.com/bufbuild/connect-go"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	SessionIDCookieName    = "xylona_session_id"
	SessionTokenCookieName = "xylona_session_token"
)

type Cookies map[string]string

func (c Cookies) Get(key string) string {
	cookie, exists := c[key]
	if !exists {
		return ""
	}
	return cookie
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

func (xs XylonaService) getUserFromHeader(header http.Header) (*models.User, error) {
	cookies := getCookiesFromHeader(header.Get("Cookie"))
	sessionID := cookies.Get(SessionIDCookieName)
	sessionTokenEncoded := cookies.Get(SessionTokenCookieName)

	if sessionID == "" || sessionTokenEncoded == "" {
		log.Debug().Str("sessionID", sessionID).Str("sessionToken", sessionTokenEncoded).Msg("Session ID or token not set")
		return nil, errors.New("session ID or token not set")
	}

	session, errGetSession := xs.db.GetUserSession(sessionID)
	if errGetSession != nil {
		if errors.Is(errGetSession, sql.ErrNoRows) {
			log.Debug().Str("sessionID", sessionID).Str("sessionToken", sessionTokenEncoded).Msg("Session does not exist")
			return nil, errors.New("session does not exist")
		}
		log.Error().Err(errGetSession).Msg("Error getting session")
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}

	if session.ID != sessionID {
		log.Debug().Str("sessionID", sessionID).Str("sessionToken", sessionTokenEncoded).Msg("Session ID does not match")
		return nil, errors.New("session ID does not match")
	}

	decodedToken := ""
	errDecode := xs.secureCookie.Decode(SessionTokenCookieName, sessionTokenEncoded, &decodedToken)
	if errDecode != nil {
		log.Debug().Str("sessionID", sessionID).Str("sessionToken", sessionTokenEncoded).Msg("Error decoding session token")
		return nil, errors.New("invalid session")
	}

	if session.Token != decodedToken {
		log.Debug().Str("sessionID", sessionID).Str("sessionToken", sessionTokenEncoded).Msg("Session token does not match")
		return nil, errors.New("session token does not match")
	}

	user, errGetUser := xs.db.GetUserByID(session.UserID)
	if errGetUser != nil {
		if errors.Is(errGetUser, sql.ErrNoRows) {
			log.Debug().Str("sessionID", sessionID).Str("sessionToken", sessionTokenEncoded).Msg("User does not exist")
			return nil, errors.New("user does not exist")
		}
		log.Error().Err(errGetUser).Msg("Error getting user")
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	return user, nil
}
