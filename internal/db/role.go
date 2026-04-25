package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// ErrRoleIsSystem indicates a system role cannot be deleted or mutated.
var ErrRoleIsSystem = errors.New("role is a system role")

// GetAllRoles returns all role records.
func (c *Connection) GetAllRoles() ([]*models.Role, error) {
	roles, errGetRoles := models.Roles.Query().All(c.ctx, c.DB)
	if errGetRoles != nil {
		return nil, fmt.Errorf("get all roles: %w", errGetRoles)
	}

	return roles, nil
}

// GetRoleByID returns a role by ID.
func (c *Connection) GetRoleByID(id string) (*models.Role, error) {
	role, errGetRole := models.Roles.Query(
		models.SelectWhere.Roles.ID.EQ(id),
	).One(c.ctx, c.DB)
	if errGetRole != nil {
		return nil, fmt.Errorf("get role by ID: %w", errGetRole)
	}

	return role, nil
}

// CreateRole inserts a role without assigning permissions.
func (c *Connection) CreateRole(id string, name string, description string) error {
	setter := &models.RoleSetter{
		ID:          omit.From(id),
		Name:        omit.From(name),
		Description: omit.From(description),
		IsSystem:    omit.From(false),
	}

	_, errCreateRole := models.Roles.Insert(setter).One(c.ctx, c.DB)
	if errCreateRole != nil {
		return fmt.Errorf("create role: %w", errCreateRole)
	}
	return nil
}

// CreateRoleWithPermissions creates a custom role and assigns permissions atomically.
// If any permission ID is invalid, the entire operation is rolled back.
func (c *Connection) CreateRoleWithPermissions(id string, name string, description string, permissionIDs []string) error {
	tx, errBegin := c.SQLDb.BeginTx(c.ctx, nil)
	if errBegin != nil {
		return fmt.Errorf("begin create role with permissions transaction: %w", errBegin)
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
		return fmt.Errorf("insert role in create role with permissions: %w", errInsertRole)
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
			return fmt.Errorf("insert role permission in create role with permissions: %w", errInsertPermission)
		}
	}

	errCommit := tx.Commit()
	if errCommit != nil && !errors.Is(errCommit, sql.ErrTxDone) {
		return fmt.Errorf("commit create role with permissions transaction: %w", errCommit)
	}

	return nil
}

// DeleteRole deletes a non-system role by ID.
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
		return fmt.Errorf("delete role: %w", errDeleteRole)
	}

	return nil
}

// GetAllRolesWithPermissions returns all roles and a map of role ID to permission IDs
// using a single JOIN query, avoiding the N+1 problem of querying permissions per role.
// GetAllRolesWithPermissions returns roles plus their assigned permission IDs.
func (c *Connection) GetAllRolesWithPermissions() ([]*models.Role, map[string][]string, error) {
	roles, errGetRoles := models.Roles.Query().All(c.ctx, c.DB)
	if errGetRoles != nil {
		return nil, nil, fmt.Errorf("get roles for permissions lookup: %w", errGetRoles)
	}

	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`SELECT role_id, permission_id FROM role_permission ORDER BY role_id, permission_id`,
	)
	if errQuery != nil {
		return nil, nil, fmt.Errorf("query role permissions: %w", errQuery)
	}
	defer func() {
		_ = rows.Close()
	}()

	permissionsByRole := make(map[string][]string, len(roles))
	for rows.Next() {
		var roleID, permissionID string
		errScan := rows.Scan(&roleID, &permissionID)
		if errScan != nil {
			return nil, nil, fmt.Errorf("scan role permission: %w", errScan)
		}
		permissionsByRole[roleID] = append(permissionsByRole[roleID], permissionID)
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, nil, fmt.Errorf("iterate role permissions: %w", errRows)
	}

	return roles, permissionsByRole, nil
}

// GetPermissionsForRole returns permission IDs granted to a role.
func (c *Connection) GetPermissionsForRole(roleID string) ([]string, error) {
	rolePermissions, errGetRolePermissions := models.RolePermissions.Query(
		models.SelectWhere.RolePermissions.RoleID.EQ(roleID),
	).All(c.ctx, c.DB)
	if errGetRolePermissions != nil {
		return nil, fmt.Errorf("get permissions for role: %w", errGetRolePermissions)
	}

	permissionIDs := make([]string, len(rolePermissions))
	for i, rolePermission := range rolePermissions {
		permissionIDs[i] = rolePermission.PermissionID
	}

	return permissionIDs, nil
}

// SetRolePermissions replaces the permission set assigned to a role.
func (c *Connection) SetRolePermissions(roleID string, permissionIDs []string) error {
	tx, errBegin := c.SQLDb.BeginTx(c.ctx, nil)
	if errBegin != nil {
		return fmt.Errorf("begin set role permissions transaction: %w", errBegin)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, errDelete := tx.ExecContext(c.ctx, `DELETE FROM role_permission WHERE role_id = ?`, roleID)
	if errDelete != nil {
		return fmt.Errorf("delete existing role permissions: %w", errDelete)
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
			return fmt.Errorf("insert role permission: %w", errInsert)
		}
	}

	errCommit := tx.Commit()
	if errCommit != nil && !errors.Is(errCommit, sql.ErrTxDone) {
		return fmt.Errorf("commit set role permissions transaction: %w", errCommit)
	}

	return nil
}
