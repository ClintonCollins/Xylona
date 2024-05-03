package rpc

import (
	"database/sql"
	"errors"
	"net/http"

	connect_go "github.com/bufbuild/connect-go"
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

func (xs XylonaService) getGameServerFromID(gameServerID string) (*models.GameServer, error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(gameServerID)
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	return gameServer, nil
}
