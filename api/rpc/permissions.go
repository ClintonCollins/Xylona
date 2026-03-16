package rpc

import (
	"errors"
	"net/http"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/api/federation"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/sql/models"
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

func (xs XylonaService) applyFederatedActingIdentity(header http.Header, actingUser *models.User) error {
	// Intentionally a no-op when there is no authenticated acting user.
	return federation.ApplyActingIdentityHeadersForUser(xs.db, header, actingUser)
}
