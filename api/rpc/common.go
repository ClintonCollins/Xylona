package rpc

import (
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (xs XylonaService) getUserFromHeader(header http.Header) (*models.User, error) {
	sessionCookies, errGetSession := gatekeeper.GetSessionFromHeader(header)
	if errGetSession != nil {
		log.Debug().Err(errGetSession).Msg("Error getting session")
		return nil, errGetSession
	}

	user, errGetUser := gatekeeper.GetUserFromSession(sessionCookies.SessionID, sessionCookies.SessionToken, xs.db, xs.secureCookie)
	if errGetUser != nil {
		log.Debug().Err(errGetUser).Msg("Error getting user")
		return nil, errGetUser
	}
	return user, nil
}
