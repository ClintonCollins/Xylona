package rpc

import (
	"errors"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/controller/authz"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (xs *XylonaService) ensureLocalServerPermission(user *models.User, gameServer *models.GameServer, permissionID string) error {
	allowed, errPermission := authz.HasPermission(xs.db, user, gameServer.ID, gameServer.UserID, permissionID)
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
		return permissionDenied("insufficient permissions")
	}
	return nil
}

// computeEffectivePermissions returns the permission IDs the given user
// effectively holds on the specified game server.
// SuperUsers and owners get all permissions; others get their role-granted set.
func (xs *XylonaService) computeEffectivePermissions(user *models.User, gameServer *models.GameServer) []string {
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
