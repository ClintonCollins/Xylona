package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetGameByID returns a game by ID.
func (c *Connection) GetGameByID(id string) (*models.Game, error) {
	game, err := models.Games.Query(models.SelectWhere.Games.ID.EQ(id)).One(c.ctx, c.DB)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Err(err).Msg("Error querying game")
		}
		return nil, fmt.Errorf("get game by ID: %w", err)
	}
	return game, nil
}

// GetGames returns all configured games.
func (c *Connection) GetGames() ([]*models.Game, error) {
	games, err := models.Games.Query().All(c.ctx, c.DB)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(err).Msg("Error querying games")
		return nil, fmt.Errorf("get games: %w", err)
	}
	return games, nil
}

// InsertGame inserts a new game record.
func (c *Connection) InsertGame(exec bob.Executor, gameSetter *models.GameSetter) (*models.Game, error) {
	game, err := models.Games.Insert(gameSetter).One(c.ctx, exec)
	if err != nil {
		log.Error().Err(err).Msg("Error inserting game")
		return nil, fmt.Errorf("insert game: %w", err)
	}
	return game, nil
}

// UpdateGame updates an existing game record.
func (c *Connection) UpdateGame(exec bob.Executor, game *models.Game, gameSetter *models.GameSetter) (*models.Game, error) {
	err := game.Update(c.ctx, exec, gameSetter)
	if err != nil {
		log.Error().Err(err).Msg("Error updating game")
		return nil, fmt.Errorf("update game: %w", err)
	}
	game, errUpdatedGame := c.GetGameByID(gameSetter.ID.MustGet())
	if errUpdatedGame != nil {
		log.Error().Err(errUpdatedGame).Msg("Error getting updated game")
		return nil, errUpdatedGame
	}
	return game, nil
}

// DeleteGameByID deletes a game by ID.
func (c *Connection) DeleteGameByID(id string) error {
	gs := models.GameSlice{&models.Game{ID: id}}
	errWrap := gs.DeleteAll(c.ctx, c.DB)
	if errWrap != nil {
		return fmt.Errorf("delete game by ID: %w", errWrap)
	}
	return nil
}
