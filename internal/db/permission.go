package db

import (
	"fmt"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetAllPermissions returns all permission records.
func (c *Connection) GetAllPermissions() ([]*models.Permission, error) {
	permissions, errGetPermissions := models.Permissions.Query().All(c.ctx, c.DB)
	if errGetPermissions != nil {
		return nil, fmt.Errorf("get all permissions: %w", errGetPermissions)
	}

	return permissions, nil
}

// GetPermissionByID returns a permission by ID.
func (c *Connection) GetPermissionByID(id string) (*models.Permission, error) {
	permission, errGetPermission := models.Permissions.Query(
		models.SelectWhere.Permissions.ID.EQ(id),
	).One(c.ctx, c.DB)
	if errGetPermission != nil {
		return nil, fmt.Errorf("get permission by ID: %w", errGetPermission)
	}

	return permission, nil
}
