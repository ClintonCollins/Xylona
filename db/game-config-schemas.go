package db

import (
	"database/sql"
	"errors"

	"github.com/rs/zerolog/log"
)

// PermissionGameServerConfig is the RBAC permission key for editing game server
// configuration files.
const PermissionGameServerConfig = "game_server.config"

func (c *Connection) GetGameConfigSchemas(gameID string) (string, error) {
	var schemas sql.NullString
	errGetSchemas := c.SQLDb.QueryRowContext(c.ctx,
		"SELECT config_schemas FROM game WHERE id = ?", gameID,
	).Scan(&schemas)
	if errGetSchemas != nil {
		if errors.Is(errGetSchemas, sql.ErrNoRows) {
			return "", errGetSchemas
		}
		log.Error().Err(errGetSchemas).Str("game_id", gameID).Msg("Error getting config schemas")
		return "", errGetSchemas
	}
	if !schemas.Valid {
		return "", nil
	}
	return schemas.String, nil
}

func (c *Connection) UpdateGameConfigSchemas(gameID string, schemasJSON string) error {
	var value any
	if schemasJSON == "" {
		value = nil
	} else {
		value = schemasJSON
	}
	_, errUpdateSchemas := c.SQLDb.ExecContext(c.ctx,
		"UPDATE game SET config_schemas = ? WHERE id = ?", value, gameID,
	)
	if errUpdateSchemas != nil {
		log.Error().Err(errUpdateSchemas).Str("game_id", gameID).Msg("Error updating config schemas")
		return errUpdateSchemas
	}
	return nil
}
