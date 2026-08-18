package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetUser returns a user by username.
func (c *Connection) GetUser(username string) (*models.User, error) {
	user, err := models.Users.Query(models.SelectWhere.Users.UserName.EQ(username)).One(c.ctx, c.DB)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Err(err).Msg("Error querying user")
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}

// GetUserByID returns a user by ID.
func (c *Connection) GetUserByID(id string) (*models.User, error) {
	user, err := models.Users.Query(models.SelectWhere.Users.ID.EQ(id)).One(c.ctx, c.DB)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Err(err).Msg("Error querying user")
		}
		return nil, fmt.Errorf("get user by ID: %w", err)
	}
	return user, nil
}

// CreateUser inserts a new user record.
func (c *Connection) CreateUser(userSetter *models.UserSetter) (*models.User, error) {
	user, err := models.Users.Insert(userSetter).One(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Msg("Error creating user")
		return nil, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

// UpdateUser updates an existing user record.
func (c *Connection) UpdateUser(userSetter *models.UserSetter) error {
	_, err := models.Users.Update(
		userSetter.UpdateMod(),
		models.UpdateWhere.Users.ID.EQ(userSetter.ID.MustGet()),
	).Exec(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Msg("Error updating user")
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

// CreateUserSession inserts a new user session record.
func (c *Connection) CreateUserSession(userSessionSetter *models.UserSessionSetter) (*models.UserSession, error) {
	userSession, err := models.UserSessions.Insert(userSessionSetter).One(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Msg("Error creating user session")
		return nil, fmt.Errorf("create user session: %w", err)
	}
	return userSession, nil
}

// GetUserSession returns a user session by ID.
func (c *Connection) GetUserSession(id string) (*models.UserSession, error) {
	userSession, err := models.UserSessions.Query(models.SelectWhere.UserSessions.ID.EQ(id)).One(c.ctx, c.DB)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Err(err).Msg("Error querying user session")
		}
		return nil, fmt.Errorf("get user session: %w", err)
	}
	return userSession, nil
}

// DeleteUserSession deletes a user session by ID.
func (c *Connection) DeleteUserSession(id string) error {
	_, err := models.UserSessions.Delete(
		models.DeleteWhere.UserSessions.ID.EQ(id),
	).Exec(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Msg("Error deleting user session")
		return fmt.Errorf("delete user session: %w", err)
	}
	return nil
}

// DeleteUserSessionsByUserID deletes every session belonging to userID.
func (c *Connection) DeleteUserSessionsByUserID(userID string) (int64, error) {
	result, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`DELETE FROM user_session WHERE user_id = ?`,
		userID,
	)
	if errExec != nil {
		log.Error().Err(errExec).Str("user_id", userID).Msg("Error deleting user sessions")
		return 0, fmt.Errorf("delete user sessions by user ID: %w", errExec)
	}

	rowsAffected, errRowsAffected := result.RowsAffected()
	if errRowsAffected != nil {
		return 0, fmt.Errorf("delete user sessions by user ID rows affected: %w", errRowsAffected)
	}

	return rowsAffected, nil
}

// TouchUserSession records recent activity on a session.
func (c *Connection) TouchUserSession(id string, at time.Time) error {
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`UPDATE user_session SET updated_at = ? WHERE id = ?`,
		at.UTC().Format("2006-01-02 15:04:05"),
		id,
	)
	if errExec != nil {
		log.Error().Err(errExec).Str("session_id", id).Msg("Error touching user session")
		return fmt.Errorf("touch user session: %w", errExec)
	}
	return nil
}

// PruneExpiredUserSessions deletes user sessions that expired before the given time.
func (c *Connection) PruneExpiredUserSessions(olderThan time.Time) (int64, error) {
	result, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`DELETE FROM user_session WHERE expires_at < ?`,
		olderThan.UTC().Format("2006-01-02 15:04:05"),
	)
	if errExec != nil {
		log.Error().Err(errExec).Msg("Error pruning expired user sessions")
		return 0, fmt.Errorf("prune expired user sessions: %w", errExec)
	}

	rowsAffected, errRowsAffected := result.RowsAffected()
	if errRowsAffected != nil {
		return 0, fmt.Errorf("prune expired user sessions rows affected: %w", errRowsAffected)
	}

	return rowsAffected, nil
}

// GetAllUsers returns all user records.
func (c *Connection) GetAllUsers() ([]*models.User, error) {
	users, err := models.Users.Query().All(c.ctx, c.DB)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Err(err).Msg("Error querying users")
		}
		return nil, fmt.Errorf("get all users: %w", err)
	}
	return users, nil
}

// CountSuperUsers returns the number of superuser accounts.
func (c *Connection) CountSuperUsers() (int, error) {
	var count int
	errQuery := c.SQLDb.QueryRowContext(c.ctx, `SELECT COUNT(*) FROM user WHERE super_user = 1`).Scan(&count)
	if errQuery != nil {
		log.Error().Err(errQuery).Msg("Error counting super users")
		return 0, fmt.Errorf("count super users: %w", errQuery)
	}
	return count, nil
}

// DeleteUser deletes a user by ID.
func (c *Connection) DeleteUser(id string) error {
	user, errGetUser := models.Users.Query(models.SelectWhere.Users.ID.EQ(id)).One(c.ctx, c.DB)
	if errGetUser != nil {
		if !errors.Is(errGetUser, sql.ErrNoRows) {
			log.Error().Err(errGetUser).Str("user_id", id).Msg("Error querying user for delete")
		}
		return fmt.Errorf("get user for delete: %w", errGetUser)
	}

	errDeleteUser := models.UserSlice{user}.DeleteAll(c.ctx, c.DB)
	if errDeleteUser != nil {
		log.Error().Err(errDeleteUser).Str("user_id", id).Msg("Error deleting user")
		return fmt.Errorf("delete user: %w", errDeleteUser)
	}

	return nil
}
