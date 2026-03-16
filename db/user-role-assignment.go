package db

import (
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
