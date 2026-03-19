package db

import (
	"fmt"
	"strings"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func (c *Connection) GetUserRoleAssignmentsForServer(gameServerID string) ([]*models.UserRoleAssignment, error) {
	assignments, errGetAssignments := models.UserRoleAssignments.Query(
		models.SelectWhere.UserRoleAssignments.GameServerID.EQ(gameServerID),
	).All(c.ctx, c.DB)
	if errGetAssignments != nil {
		return nil, errGetAssignments
	}

	return assignments, nil
}

func (c *Connection) GetUserRoleAssignmentsForUser(userID string) ([]*models.UserRoleAssignment, error) {
	assignments, errGetAssignments := models.UserRoleAssignments.Query(
		models.SelectWhere.UserRoleAssignments.UserID.EQ(userID),
	).All(c.ctx, c.DB)
	if errGetAssignments != nil {
		return nil, errGetAssignments
	}

	return assignments, nil
}

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
	return errCreateAssignment
}

func (c *Connection) DeleteUserRoleAssignment(id string) error {
	assignment, errGetAssignment := models.FindUserRoleAssignment(c.ctx, c.DB, id)
	if errGetAssignment != nil {
		return errGetAssignment
	}

	errDeleteAssignment := models.UserRoleAssignmentSlice{assignment}.DeleteAll(c.ctx, c.DB)
	if errDeleteAssignment != nil {
		return errDeleteAssignment
	}

	return nil
}

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
		return false, errQuery
	}

	return count > 0, nil
}

func (c *Connection) GetUserRoleAssignmentByID(id string) (*models.UserRoleAssignment, error) {
	assignment, errGetAssignment := models.UserRoleAssignments.Query(
		models.SelectWhere.UserRoleAssignments.ID.EQ(id),
	).One(c.ctx, c.DB)
	if errGetAssignment != nil {
		return nil, errGetAssignment
	}

	return assignment, nil
}

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
		return nil, errQuery
	}
	defer func() {
		_ = rows.Close()
	}()

	var permissionIDs []string
	for rows.Next() {
		var permissionID string
		if errScan := rows.Scan(&permissionID); errScan != nil {
			return nil, errScan
		}
		permissionIDs = append(permissionIDs, permissionID)
	}
	if errRows := rows.Err(); errRows != nil {
		return nil, errRows
	}

	if permissionIDs == nil {
		return []string{}, nil
	}

	return permissionIDs, nil
}

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

	query := fmt.Sprintf(
		`SELECT ura.game_server_id, rp.permission_id
		FROM user_role_assignment ura
		JOIN role_permission rp ON ura.role_id = rp.role_id
		WHERE ura.user_id = ? AND ura.game_server_id IN (%s)
		UNION
		SELECT gs.id AS game_server_id, rp.permission_id
		FROM user_role_assignment ura
		JOIN role_permission rp ON ura.role_id = rp.role_id
		CROSS JOIN game_server gs
		WHERE gs.id IN (%s) AND ura.user_id = ? AND ura.game_server_id IS NULL`,
		inClause, inClause,
	)

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
		return nil, errQuery
	}
	defer func() {
		_ = rows.Close()
	}()

	seen := make(map[string]map[string]struct{})
	for rows.Next() {
		var serverID, permissionID string
		if errScan := rows.Scan(&serverID, &permissionID); errScan != nil {
			return nil, errScan
		}
		if _, exists := seen[serverID]; !exists {
			seen[serverID] = make(map[string]struct{})
		}
		seen[serverID][permissionID] = struct{}{}
	}
	if errRows := rows.Err(); errRows != nil {
		return nil, errRows
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

func (c *Connection) GetUserRoleAssignmentByComposite(userID string, roleID string, gameServerID string) (*models.UserRoleAssignment, error) {
	if gameServerID == "" {
		return models.UserRoleAssignments.Query(
			models.SelectWhere.UserRoleAssignments.UserID.EQ(userID),
			models.SelectWhere.UserRoleAssignments.RoleID.EQ(roleID),
			models.SelectWhere.UserRoleAssignments.GameServerID.IsNull(),
		).One(c.ctx, c.DB)
	}

	return models.UserRoleAssignments.Query(
		models.SelectWhere.UserRoleAssignments.UserID.EQ(userID),
		models.SelectWhere.UserRoleAssignments.RoleID.EQ(roleID),
		models.SelectWhere.UserRoleAssignments.GameServerID.EQ(gameServerID),
	).One(c.ctx, c.DB)
}
