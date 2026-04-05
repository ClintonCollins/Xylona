package db

import (
	"database/sql"
	"errors"
	"fmt"

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
		log.Error().Err(err).Str("session_id", id).Msg("Error deleting user session")
		return fmt.Errorf("delete user session: %w", err)
	}
	return nil
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
