package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// GameServerPalworldMap stores the persistent map-source settings for one Palworld server.
type GameServerPalworldMap struct {
	GameServerID    string
	LayersJSON      string
	UpdatedByUserID sql.NullString
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// GetGameServerPalworldMap returns the persistent settings for one server. A
// server without settings receives an empty in-memory value.
func (c *Connection) GetGameServerPalworldMap(gameServerID string) (*GameServerPalworldMap, error) {
	row := c.SQLDb.QueryRowContext(
		c.ctx,
		`select game_server_id, layers_json, updated_by_user_id, created_at, updated_at
		 from game_server_palworld_map
		 where game_server_id = ?`,
		gameServerID,
	)
	settings, errScan := scanGameServerPalworldMap(row)
	if errors.Is(errScan, sql.ErrNoRows) {
		return &GameServerPalworldMap{GameServerID: gameServerID, LayersJSON: "[]"}, nil
	}
	if errScan != nil {
		return nil, fmt.Errorf("get game server palworld map: %w", errScan)
	}
	return settings, nil
}

// UpdateGameServerPalworldMapLayers stores the validated external tile-layer JSON.
func (c *Connection) UpdateGameServerPalworldMapLayers(gameServerID string, layersJSON string, updatedByUserID string) error {
	now := time.Now().UTC()
	updatedBy := nullableTrimmedString(updatedByUserID)
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`insert into game_server_palworld_map
			(game_server_id, layers_json, updated_by_user_id, created_at, updated_at)
		 values (?, ?, ?, ?, ?)
		 on conflict(game_server_id) do update set
			layers_json = excluded.layers_json,
			updated_by_user_id = excluded.updated_by_user_id,
			updated_at = excluded.updated_at`,
		gameServerID,
		layersJSON,
		updatedBy,
		now,
		now,
	)
	if errExec != nil {
		return fmt.Errorf("update game server palworld map layers: %w", errExec)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanGameServerPalworldMap(row scanner) (*GameServerPalworldMap, error) {
	var settings GameServerPalworldMap
	errScan := row.Scan(
		&settings.GameServerID,
		&settings.LayersJSON,
		&settings.UpdatedByUserID,
		&settings.CreatedAt,
		&settings.UpdatedAt,
	)
	if errScan != nil {
		return nil, fmt.Errorf("scan game server palworld map: %w", errScan)
	}
	return &settings, nil
}

func nullableTrimmedString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}
