package db

import (
	"fmt"

	"github.com/ClintonCollins/Xylona/sql/models"
)

type permissionLookup interface {
	UserHasPermissionOnServer(userID string, gameServerID string, permissionID string) (bool, error)
}

// HasPermission checks local authorization for a user on a local game server.
// Check order: super user -> owner -> explicit role assignment.
func HasPermission(lookup permissionLookup, user *models.User, gameServerID string, gameServerOwnerUserID string, permissionID string) (bool, error) {
	if user == nil {
		return false, nil
	}
	if user.SuperUser {
		return true, nil
	}
	if user.ID == gameServerOwnerUserID {
		return true, nil
	}
	allowed, errPermission := lookup.UserHasPermissionOnServer(user.ID, gameServerID, permissionID)
	if errPermission != nil {
		return false, fmt.Errorf("check permission %s for user %s on server %s: %w", permissionID, user.ID, gameServerID, errPermission)
	}
	return allowed, nil
}
