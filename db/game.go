package db

import (
	"database/sql"
	"errors"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func (c *Connection) GetGameByID(id string) (*models.Game, error) {
	game, err := models.Games.Query(c.ctx, c.DB, models.SelectWhere.Games.ID.EQ(id)).One()
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Err(err).Msg("Error querying game")
		}
		return nil, err
	}
	return game, err
}

func (c *Connection) GetGames() ([]*models.Game, error) {
	games, err := models.Games.Query(c.ctx, c.DB).All()
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(err).Msg("Error querying games")
		return nil, err
	}
	return games, nil
}
