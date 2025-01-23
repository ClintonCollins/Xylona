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
