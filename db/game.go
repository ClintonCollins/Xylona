package db

import (
	"database/sql"
	"errors"

	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"

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

func (c *Connection) InsertGame(exec bob.Executor, gameSetter *models.GameSetter) (*models.Game, error) {
	game, err := models.Games.Insert(c.ctx, exec, gameSetter)
	if err != nil {
		log.Error().Err(err).Msg("Error inserting game")
		return nil, err
	}
	return game, nil
}

func (c *Connection) UpdateGame(exec bob.Executor, game *models.Game, gameSetter *models.GameSetter) (*models.Game, error) {
	err := game.Update(c.ctx, exec, gameSetter)
	if err != nil {
		log.Error().Err(err).Msg("Error updating game")
		return nil, err
	}
	game, errUpdatedGame := c.GetGameByID(gameSetter.ID.MustGet())
	if errUpdatedGame != nil {
		log.Error().Err(errUpdatedGame).Msg("Error getting updated game")
		return nil, errUpdatedGame
	}
	return game, nil
}

func (c *Connection) DeleteGameByID(id string) error {
	return models.Games.Delete(c.ctx, c.DB, &models.Game{ID: id})
}
