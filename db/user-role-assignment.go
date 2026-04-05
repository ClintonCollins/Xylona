package db

import (
	"fmt"
	"strings"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetUserRoleAssignmentsForServer returns role assignments for a server.
func (c *Connection) GetUserRoleAssignmentsForServer(gameServerID string) ([]*models.UserRoleAssignment, error) {
	assignments, errGetAssignments := models.UserRoleAssignments.Query(
		models.SelectWhere.UserRoleAssignments.GameServerID.EQ(gameServerID),
	).All(c.ctx, c.DB)
	if errGetAssignments != nil {
		return nil, fmt.Errorf("get user role assignments for server: %w", errGetAssignments)
	}

	return assignments, nil
}

// GetUserRoleAssignmentsForUser returns role assignments for a user.
func (c *Connection) GetUserRoleAssignmentsForUser(userID string) ([]*models.UserRoleAssignment, error) {
	assignments, errGetAssignments := models.UserRoleAssignments.Query(
		models.SelectWhere.UserRoleAssignments.UserID.EQ(userID),
	).All(c.ctx, c.DB)
	if errGetAssignments != nil {
		return nil, fmt.Errorf("get user role assignments for user: %w", errGetAssignments)
	}

	return assignments, nil
}

// CreateUserRoleAssignment creates a new user-to-role assignment.
func (c *Connection) CreateUserRoleAssignment(id string, userID string, roleID string, gameServerID string, grantedBy string) error {
	gameServerIDSetter := omitnull.From(gameServerID)
	if gameServerID == "" {
		gameServerIDSetter = omitnull.FromNull[string](null.Val[string]{})
	}

	setter := &models.UserRoleAssignmentSetter{
		ID:           omit.From(id),
		UserID:       omit.From(userID),
		RoleID:       omit.From(roleID),
		GameServerID: gameServerIDSetter,
		GrantedBy:    omit.From(grantedBy),
	}

	_, errCreateAssignment := models.UserRoleAssignments.Insert(setter).One(c.ctx, c.DB)
	if errCreateAssignment != nil {
		return fmt.Errorf("create user role assignment: %w", errCreateAssignment)
	}
	return nil
}

// DeleteUserRoleAssignment deletes a role assignment by ID.
func (c *Connection) DeleteUserRoleAssignment(id string) error {
	assignment, errGetAssignment := models.FindUserRoleAssignment(c.ctx, c.DB, id)
	if errGetAssignment != nil {
		return fmt.Errorf("get user role assignment for delete: %w", errGetAssignment)
	}

	errDeleteAssignment := models.UserRoleAssignmentSlice{assignment}.DeleteAll(c.ctx, c.DB)
	if errDeleteAssignment != nil {
		return fmt.Errorf("delete user role assignment: %w", errDeleteAssignment)
	}

	return nil
}

// UserHasPermissionOnServer checks whether a user has a server-scoped permission.
func (c *Connection) UserHasPermissionOnServer(userID string, gameServerID string, permissionID string) (bool, error) {
	var count int
	errQuery := c.SQLDb.QueryRowContext(
		c.ctx,
		`SELECT COUNT(*)
		FROM user_role_assignment ura
		JOIN role_permission rp ON ura.role_id = rp.role_id
		WHERE ura.user_id = ? AND rp.permission_id = ?
		AND (ura.game_server_id = ? OR ura.game_server_id IS NULL)`,
		userID, permissionID, gameServerID,
	).Scan(&count)
	if errQuery != nil {
		return false, fmt.Errorf("check user permission on server: %w", errQuery)
	}

	return count > 0, nil
}

// GetUserRoleAssignmentByID returns a role assignment by ID.
func (c *Connection) GetUserRoleAssignmentByID(id string) (*models.UserRoleAssignment, error) {
	assignment, errGetAssignment := models.UserRoleAssignments.Query(
		models.SelectWhere.UserRoleAssignments.ID.EQ(id),
	).One(c.ctx, c.DB)
	if errGetAssignment != nil {
		return nil, fmt.Errorf("get user role assignment by ID: %w", errGetAssignment)
	}

	return assignment, nil
}

// GetUserPermissionIDsForServer returns permission IDs a user has on a server.
func (c *Connection) GetUserPermissionIDsForServer(userID string, gameServerID string) ([]string, error) {
	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`SELECT DISTINCT rp.permission_id
		FROM user_role_assignment ura
		JOIN role_permission rp ON ura.role_id = rp.role_id
		WHERE ura.user_id = ?
		AND (ura.game_server_id = ? OR ura.game_server_id IS NULL)`,
		userID, gameServerID,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("query user permission IDs for server: %w", errQuery)
	}
	defer func() {
		_ = rows.Close()
	}()

	var permissionIDs []string
	for rows.Next() {
		var permissionID string
		if errScan := rows.Scan(&permissionID); errScan != nil {
			return nil, fmt.Errorf("scan user permission ID for server: %w", errScan)
		}
		permissionIDs = append(permissionIDs, permissionID)
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate user permission IDs for server: %w", errRows)
	}

	if permissionIDs == nil {
		return []string{}, nil
	}

	return permissionIDs, nil
}

// GetUserGlobalPermissionIDs returns global permission IDs granted to a user.
func (c *Connection) GetUserGlobalPermissionIDs(userID string) ([]string, error) {
	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`SELECT DISTINCT rp.permission_id
		FROM user_role_assignment ura
		JOIN role_permission rp ON ura.role_id = rp.role_id
		WHERE ura.user_id = ?
		AND ura.game_server_id IS NULL`,
		userID,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("query user global permission IDs: %w", errQuery)
	}
	defer func() {
		_ = rows.Close()
	}()

	var permissionIDs []string
	for rows.Next() {
		var permissionID string
		if errScan := rows.Scan(&permissionID); errScan != nil {
			return nil, fmt.Errorf("scan user global permission ID: %w", errScan)
		}
		permissionIDs = append(permissionIDs, permissionID)
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate user global permission IDs: %w", errRows)
	}

	if permissionIDs == nil {
		return []string{}, nil
	}

	return permissionIDs, nil
}

// GetUserPermissionIDsForServers returns server permissions for multiple servers.
func (c *Connection) GetUserPermissionIDsForServers(userID string, gameServerIDs []string) (map[string][]string, error) {
	result := make(map[string][]string)
	if len(gameServerIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(gameServerIDs))
	for i := range gameServerIDs {
		placeholders[i] = "?"
	}
	inClause := strings.Join(placeholders, ", ")

	//nolint:gosec // only placeholder tokens are concatenated; values remain bound parameters.
	query := `SELECT ura.game_server_id, rp.permission_id
		FROM user_role_assignment ura
		JOIN role_permission rp ON ura.role_id = rp.role_id
		WHERE ura.user_id = ? AND ura.game_server_id IN (` + inClause + `)
		UNION
		SELECT gs.id AS game_server_id, rp.permission_id
		FROM user_role_assignment ura
		JOIN role_permission rp ON ura.role_id = rp.role_id
		CROSS JOIN game_server gs
		WHERE gs.id IN (` + inClause + `) AND ura.user_id = ? AND ura.game_server_id IS NULL`

	args := make([]any, 0, 1+len(gameServerIDs)+len(gameServerIDs)+1)
	args = append(args, userID)
	for _, id := range gameServerIDs {
		args = append(args, id)
	}
	for _, id := range gameServerIDs {
		args = append(args, id)
	}
	args = append(args, userID)

	rows, errQuery := c.SQLDb.QueryContext(c.ctx, query, args...)
	if errQuery != nil {
		return nil, fmt.Errorf("query user permission IDs for servers: %w", errQuery)
	}
	defer func() {
		_ = rows.Close()
	}()

	seen := make(map[string]map[string]struct{})
	for rows.Next() {
		var serverID, permissionID string
		if errScan := rows.Scan(&serverID, &permissionID); errScan != nil {
			return nil, fmt.Errorf("scan user permission ID for servers: %w", errScan)
		}
		if _, exists := seen[serverID]; !exists {
			seen[serverID] = make(map[string]struct{})
		}
		seen[serverID][permissionID] = struct{}{}
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate user permission IDs for servers: %w", errRows)
	}

	for serverID, permSet := range seen {
		perms := make([]string, 0, len(permSet))
		for perm := range permSet {
			perms = append(perms, perm)
		}
		result[serverID] = perms
	}

	return result, nil
}

// UserHasGlobalPermission checks whether the user has the given permission via
// a globally-scoped role assignment (game_server_id IS NULL).
// UserHasGlobalPermission checks whether a user has a global permission.
func (c *Connection) UserHasGlobalPermission(userID string, permissionID string) (bool, error) {
	var count int
	errQuery := c.SQLDb.QueryRowContext(
		c.ctx,
		`SELECT COUNT(*)
		FROM user_role_assignment ura
		JOIN role_permission rp ON ura.role_id = rp.role_id
		WHERE ura.user_id = ? AND rp.permission_id = ?
		AND ura.game_server_id IS NULL`,
		userID, permissionID,
	).Scan(&count)
	if errQuery != nil {
		return false, fmt.Errorf("check user global permission: %w", errQuery)
	}

	return count > 0, nil
}

// UserHasAnyGlobalPermission checks whether the user has at least one of the
// given permissions via globally-scoped role assignments (game_server_id IS NULL).
// UserHasAnyGlobalPermission checks whether a user has any of the given global permissions.
func (c *Connection) UserHasAnyGlobalPermission(userID string, permissionIDs []string) (bool, error) {
	if len(permissionIDs) == 0 {
		return false, nil
	}

	placeholders := make([]string, len(permissionIDs))
	args := make([]any, 0, len(permissionIDs)+1)
	args = append(args, userID)
	for idx, permissionID := range permissionIDs {
		placeholders[idx] = "?"
		args = append(args, permissionID)
	}

	query := `SELECT COUNT(*)
		FROM user_role_assignment ura
		JOIN role_permission rp ON ura.role_id = rp.role_id
		WHERE ura.user_id = ? AND rp.permission_id IN (` + strings.Join(placeholders, ", ") + `)
		AND ura.game_server_id IS NULL`

	var count int
	errQuery := c.SQLDb.QueryRowContext(c.ctx, query, args...).Scan(&count)
	if errQuery != nil {
		return false, fmt.Errorf("check user any global permission: %w", errQuery)
	}

	return count > 0, nil
}

// GetUserRoleAssignmentByComposite returns an assignment by user, role, and server.
func (c *Connection) GetUserRoleAssignmentByComposite(userID string, roleID string, gameServerID string) (*models.UserRoleAssignment, error) {
	if gameServerID == "" {
		assignment, errGetAssignment := models.UserRoleAssignments.Query(
			models.SelectWhere.UserRoleAssignments.UserID.EQ(userID),
			models.SelectWhere.UserRoleAssignments.RoleID.EQ(roleID),
			models.SelectWhere.UserRoleAssignments.GameServerID.IsNull(),
		).One(c.ctx, c.DB)
		if errGetAssignment != nil {
			return nil, fmt.Errorf("get user role assignment by composite: %w", errGetAssignment)
		}

		return assignment, nil
	}

	assignment, errGetAssignment := models.UserRoleAssignments.Query(
		models.SelectWhere.UserRoleAssignments.UserID.EQ(userID),
		models.SelectWhere.UserRoleAssignments.RoleID.EQ(roleID),
		models.SelectWhere.UserRoleAssignments.GameServerID.EQ(gameServerID),
	).One(c.ctx, c.DB)
	if errGetAssignment != nil {
		return nil, fmt.Errorf("get user role assignment by composite: %w", errGetAssignment)
	}

	return assignment, nil
}
