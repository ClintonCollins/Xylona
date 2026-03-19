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

// computeEffectivePermissions returns the permission IDs the given user
// effectively holds on the specified game server.
// SuperUsers and owners get all permissions; others get their role-granted set.
func (xs XylonaService) computeEffectivePermissions(user *models.User, gameServer *models.GameServer) []string {
	if user.SuperUser || user.ID == gameServer.UserID {
		return xs.allPermissionIDs
	}
	perms, errPerms := xs.db.GetUserPermissionIDsForServer(user.ID, gameServer.ID)
	if errPerms != nil {
		log.Error().Err(errPerms).
			Str("user_id", user.ID).
			Str("server_id", gameServer.ID).
			Msg("Failed to get effective permissions")
		// Return nil — frontend treats empty as "unknown, show everything" (backend enforces)
		return nil
	}
	return perms
}
