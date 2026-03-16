package db

import "github.com/ClintonCollins/Xylona/sql/models"

func (c *Connection) GetAllPermissions() ([]*models.Permission, error) {
	permissions, errGetPermissions := models.Permissions.Query().All(c.ctx, c.DB)
	if errGetPermissions != nil {
		return nil, errGetPermissions
	}

	return permissions, nil
}

func (c *Connection) GetPermissionByID(id string) (*models.Permission, error) {
	permission, errGetPermission := models.Permissions.Query(
		models.SelectWhere.Permissions.ID.EQ(id),
	).One(c.ctx, c.DB)
	if errGetPermission != nil {
		return nil, errGetPermission
	}

	return permission, nil
}
