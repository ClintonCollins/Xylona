package helpers

import (
	"github.com/ClintonCollins/Xylona/sql/models"
)

type PermissionLookup interface {
	UserHasPermissionOnServer(userID string, gameServerID string, permissionID string) (bool, error)
}

// HasPermission checks local authorization for a user on a local game server.
// Check order: super user -> owner -> explicit role assignment.
func HasPermission(permissionLookup PermissionLookup, user *models.User, gameServerID string, gameServerOwnerUserID string, permissionID string) (bool, error) {
	if user == nil {
		return false, nil
	}
	if user.SuperUser {
		return true, nil
	}
	if user.ID == gameServerOwnerUserID {
		return true, nil
	}
	return permissionLookup.UserHasPermissionOnServer(user.ID, gameServerID, permissionID)
}
