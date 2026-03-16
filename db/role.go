package db

import (
	"database/sql"
	"errors"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
)

var ErrRoleIsSystem = errors.New("role is a system role")

func (c *Connection) GetAllRoles() ([]*models.Role, error) {
	roles, errGetRoles := models.Roles.Query().All(c.ctx, c.DB)
	if errGetRoles != nil {
		return nil, errGetRoles
	}

	return roles, nil
}

func (c *Connection) GetRoleByID(id string) (*models.Role, error) {
	role, errGetRole := models.Roles.Query(
		models.SelectWhere.Roles.ID.EQ(id),
	).One(c.ctx, c.DB)
	if errGetRole != nil {
		return nil, errGetRole
	}

	return role, nil
}

func (c *Connection) CreateRole(id string, name string, description string) error {
	setter := &models.RoleSetter{
		ID:          omit.From(id),
		Name:        omit.From(name),
		Description: omit.From(description),
		IsSystem:    omit.From(false),
	}

	_, errCreateRole := models.Roles.Insert(setter).One(c.ctx, c.DB)
	return errCreateRole
}

// CreateRoleWithPermissions creates a custom role and assigns the given permissions atomically.
// If any permission ID is invalid, the entire operation is rolled back.
func (c *Connection) CreateRoleWithPermissions(id string, name string, description string, permissionIDs []string) error {
	tx, errBegin := c.SQLDb.BeginTx(c.ctx, nil)
	if errBegin != nil {
		return errBegin
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, errInsertRole := tx.ExecContext(
		c.ctx,
		`INSERT INTO role (id, name, description, is_system) VALUES (?, ?, ?, false)`,
		id, name, description,
	)
	if errInsertRole != nil {
		return errInsertRole
	}

	seenPermissionIDs := make(map[string]struct{}, len(permissionIDs))
	for _, permissionID := range permissionIDs {
		if _, seen := seenPermissionIDs[permissionID]; seen {
			continue
		}
		seenPermissionIDs[permissionID] = struct{}{}

		_, errInsertPermission := tx.ExecContext(
			c.ctx,
			`INSERT INTO role_permission (role_id, permission_id) VALUES (?, ?)`,
			id, permissionID,
		)
		if errInsertPermission != nil {
			return errInsertPermission
		}
	}

	errCommit := tx.Commit()
	if errCommit != nil && !errors.Is(errCommit, sql.ErrTxDone) {
		return errCommit
	}

	return nil
}

func (c *Connection) DeleteRole(id string) error {
	role, errGetRole := c.GetRoleByID(id)
	if errGetRole != nil {
		return errGetRole
	}
	if role.IsSystem {
		return ErrRoleIsSystem
	}

	errDeleteRole := models.RoleSlice{role}.DeleteAll(c.ctx, c.DB)
	if errDeleteRole != nil {
		return errDeleteRole
	}

	return nil
}

func (c *Connection) GetPermissionsForRole(roleID string) ([]string, error) {
	rolePermissions, errGetRolePermissions := models.RolePermissions.Query(
		models.SelectWhere.RolePermissions.RoleID.EQ(roleID),
	).All(c.ctx, c.DB)
	if errGetRolePermissions != nil {
		return nil, errGetRolePermissions
	}

	permissionIDs := make([]string, len(rolePermissions))
	for i, rolePermission := range rolePermissions {
		permissionIDs[i] = rolePermission.PermissionID
	}

	return permissionIDs, nil
}

func (c *Connection) SetRolePermissions(roleID string, permissionIDs []string) error {
	tx, errBegin := c.SQLDb.BeginTx(c.ctx, nil)
	if errBegin != nil {
		return errBegin
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, errDelete := tx.ExecContext(c.ctx, `DELETE FROM role_permission WHERE role_id = ?`, roleID)
	if errDelete != nil {
		return errDelete
	}

	seenPermissionIDs := make(map[string]struct{}, len(permissionIDs))
	for _, permissionID := range permissionIDs {
		if _, seen := seenPermissionIDs[permissionID]; seen {
			continue
		}
		seenPermissionIDs[permissionID] = struct{}{}

		_, errInsert := tx.ExecContext(
			c.ctx,
			`INSERT INTO role_permission (role_id, permission_id) VALUES (?, ?)`,
			roleID,
			permissionID,
		)
		if errInsert != nil {
			return errInsert
		}
	}

	errCommit := tx.Commit()
	if errCommit != nil && !errors.Is(errCommit, sql.ErrTxDone) {
		return errCommit
	}

	return nil
}
