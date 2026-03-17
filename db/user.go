package db

import (
	"database/sql"
	"errors"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func (c *Connection) GetUser(username string) (*models.User, error) {
	user, err := models.Users.Query(models.SelectWhere.Users.UserName.EQ(username)).One(c.ctx, c.DB)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Err(err).Msg("Error querying user")
		}
		return nil, err
	}
	return user, err
}

func (c *Connection) GetUserByID(id string) (*models.User, error) {
	user, err := models.Users.Query(models.SelectWhere.Users.ID.EQ(id)).One(c.ctx, c.DB)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Err(err).Msg("Error querying user")
		}
		return nil, err
	}
	return user, err
}

func (c *Connection) CreateUser(userSetter *models.UserSetter) (*models.User, error) {
	user, err := models.Users.Insert(userSetter).One(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Msg("Error creating user")
		return nil, err
	}
	return user, err
}

func (c *Connection) UpdateUser(userSetter *models.UserSetter) error {
	_, err := models.Users.Update(
		userSetter.UpdateMod(),
		models.UpdateWhere.Users.ID.EQ(userSetter.ID.MustGet()),
	).Exec(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Msg("Error updating user")
		return err
	}
	return err
}

func (c *Connection) CreateUserSession(userSessionSetter *models.UserSessionSetter) (*models.UserSession, error) {
	userSession, err := models.UserSessions.Insert(userSessionSetter).One(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Msg("Error creating user session")
		return nil, err
	}
	return userSession, err
}

func (c *Connection) GetUserSession(id string) (*models.UserSession, error) {
	userSession, err := models.UserSessions.Query(models.SelectWhere.UserSessions.ID.EQ(id)).One(c.ctx, c.DB)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Err(err).Msg("Error querying user session")
		}
		return nil, err
	}
	return userSession, err
}

func (c *Connection) DeleteUserSession(id string) error {
	_, err := models.UserSessions.Delete(
		models.DeleteWhere.UserSessions.ID.EQ(id),
	).Exec(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Str("session_id", id).Msg("Error deleting user session")
		return err
	}
	return nil
}

func (c *Connection) GetAllUsers() ([]*models.User, error) {
	users, err := models.Users.Query().All(c.ctx, c.DB)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Err(err).Msg("Error querying users")
		}
		return nil, err
	}
	return users, err
}

func (c *Connection) CountSuperUsers() (int, error) {
	var count int
	errQuery := c.SQLDb.QueryRowContext(c.ctx, `SELECT COUNT(*) FROM user WHERE super_user = 1`).Scan(&count)
	if errQuery != nil {
		log.Error().Err(errQuery).Msg("Error counting super users")
		return 0, errQuery
	}
	return count, nil
}

func (c *Connection) DeleteUser(id string) error {
	user, errGetUser := models.Users.Query(models.SelectWhere.Users.ID.EQ(id)).One(c.ctx, c.DB)
	if errGetUser != nil {
		if !errors.Is(errGetUser, sql.ErrNoRows) {
			log.Error().Err(errGetUser).Str("user_id", id).Msg("Error querying user for delete")
		}
		return errGetUser
	}

	errDeleteUser := models.UserSlice{user}.DeleteAll(c.ctx, c.DB)
	if errDeleteUser != nil {
		log.Error().Err(errDeleteUser).Str("user_id", id).Msg("Error deleting user")
		return errDeleteUser
	}

	return nil
}
