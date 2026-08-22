package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// GameServerMinecraftMap contains the persisted BlueMap settings for one server.
type GameServerMinecraftMap struct {
	GameServerID    string
	Enabled         bool
	WorldName       string
	AcceptedAt      sql.NullTime
	UpdatedByUserID sql.NullString
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// GetGameServerMinecraftMap returns stored settings or safe defaults when the
// map has not been configured yet.
func (c *Connection) GetGameServerMinecraftMap(gameServerID string) (*GameServerMinecraftMap, error) {
	row := c.SQLDb.QueryRowContext(c.ctx, minecraftMapSelect+" where game_server_id = ?", gameServerID)
	settings, errScan := scanGameServerMinecraftMap(row)
	if errors.Is(errScan, sql.ErrNoRows) {
		return &GameServerMinecraftMap{GameServerID: gameServerID, WorldName: "world"}, nil
	}
	if errScan != nil {
		return nil, fmt.Errorf("get game server Minecraft map: %w", errScan)
	}
	return settings, nil
}

// UpdateGameServerMinecraftMapConfig enables or disables the managed map and
// records the explicit BlueMap/Mojang download acceptance when supplied.
func (c *Connection) UpdateGameServerMinecraftMapConfig(
	gameServerID string,
	enabled bool,
	worldName string,
	accepted bool,
	updatedByUserID string,
) error {
	now := time.Now().UTC()
	acceptedAt := sql.NullTime{}
	if accepted {
		acceptedAt = sql.NullTime{Time: now, Valid: true}
	}
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`insert into game_server_minecraft_map
			(game_server_id, enabled, world_name, accepted_at, updated_by_user_id, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?)
		 on conflict(game_server_id) do update set
			enabled = excluded.enabled,
			world_name = excluded.world_name,
			accepted_at = coalesce(excluded.accepted_at, game_server_minecraft_map.accepted_at),
			updated_by_user_id = excluded.updated_by_user_id,
			updated_at = excluded.updated_at`,
		gameServerID,
		enabled,
		worldName,
		acceptedAt,
		nullableTrimmedString(updatedByUserID),
		now,
		now,
	)
	if errExec != nil {
		return fmt.Errorf("update game server Minecraft map config: %w", errExec)
	}
	return nil
}

const minecraftMapSelect = `select game_server_id, enabled, world_name,
	 accepted_at, updated_by_user_id, created_at, updated_at
	 from game_server_minecraft_map`

func scanGameServerMinecraftMap(row scanner) (*GameServerMinecraftMap, error) {
	var settings GameServerMinecraftMap
	errScan := row.Scan(
		&settings.GameServerID,
		&settings.Enabled,
		&settings.WorldName,
		&settings.AcceptedAt,
		&settings.UpdatedByUserID,
		&settings.CreatedAt,
		&settings.UpdatedAt,
	)
	if errScan != nil {
		return nil, fmt.Errorf("scan game server Minecraft map: %w", errScan)
	}
	return &settings, nil
}
