package rpc

import (
	"errors"
	"net/http"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	federationActingUserIDHeader = "X-Xylona-Acting-User-ID"
	federationOriginNodeIDHeader = "X-Xylona-Origin-Node-ID"
)

func (xs XylonaService) ensureLocalServerPermission(user *models.User, gameServer *models.GameServer, permissionID string) error {
	allowed, errPermission := helpers.HasPermission(xs.db, user, gameServer.ID, gameServer.UserID, permissionID)
	if errPermission != nil {
		log.Error().
			Err(errPermission).
			Str("server_id", gameServer.ID).
			Str("permission_id", permissionID).
			Str("user_id", user.ID).
			Msg("failed to check game server permission")
		return connect.NewError(connect.CodeInternal, errors.New("failed to check permissions"))
	}
	if !allowed {
		return connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
	}
	return nil
}

func (xs XylonaService) getLocalNodeID() (string, error) {
	localSettings, errSettings := xs.db.GetLocalSettings()
	if errSettings != nil {
		return "", errSettings
	}
	return localSettings.NodeID, nil
}

func (xs XylonaService) applyFederatedActingIdentity(header http.Header, actingUser *models.User) error {
	if actingUser == nil {
		return nil
	}

	localNodeID, errNodeID := xs.getLocalNodeID()
	if errNodeID != nil {
		return errNodeID
	}

	header.Set(federationActingUserIDHeader, actingUser.ID)
	header.Set(federationOriginNodeIDHeader, localNodeID)
	return nil
}

func getFederatedActingIdentity(header http.Header) (string, string) {
	return header.Get(federationActingUserIDHeader), header.Get(federationOriginNodeIDHeader)
}
