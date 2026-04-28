package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// GameServerReadiness stores public readiness state for a server/kind pair.
type GameServerReadiness struct {
	GameServerID    string
	Kind            string
	PublicData      string
	UpdatedByUserID sql.NullString
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// UpsertGameServerReadiness stores public readiness state for one server/kind pair.
func (c *Connection) UpsertGameServerReadiness(gameServerID string, kind string, publicData string, updatedByUserID string) error {
	now := time.Now().UTC()
	updatedBy := sql.NullString{
		String: strings.TrimSpace(updatedByUserID),
		Valid:  strings.TrimSpace(updatedByUserID) != "",
	}

	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`insert into game_server_readiness
			(game_server_id, kind, public_data, updated_by_user_id, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?)
		 on conflict(game_server_id, kind) do update set
			public_data = excluded.public_data,
			updated_by_user_id = excluded.updated_by_user_id,
			updated_at = excluded.updated_at`,
		gameServerID,
		kind,
		publicData,
		updatedBy,
		now,
		now,
	)
	if errExec != nil {
		return fmt.Errorf("upsert game server readiness: %w", errExec)
	}
	return nil
}

// GetGameServerReadiness returns public readiness state for one server/kind pair.
func (c *Connection) GetGameServerReadiness(gameServerID string, kind string) (*GameServerReadiness, error) {
	row := c.SQLDb.QueryRowContext(
		c.ctx,
		`select game_server_id, kind, public_data, updated_by_user_id, created_at, updated_at
		 from game_server_readiness
		 where game_server_id = ? and kind = ?`,
		gameServerID,
		kind,
	)

	var readiness GameServerReadiness
	errScan := row.Scan(
		&readiness.GameServerID,
		&readiness.Kind,
		&readiness.PublicData,
		&readiness.UpdatedByUserID,
		&readiness.CreatedAt,
		&readiness.UpdatedAt,
	)
	if errScan != nil {
		return nil, fmt.Errorf("get game server readiness: %w", errScan)
	}
	return &readiness, nil
}

// ListGameServerReadiness returns public readiness state for all kinds on a server.
func (c *Connection) ListGameServerReadiness(gameServerID string) ([]GameServerReadiness, error) {
	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`select game_server_id, kind, public_data, updated_by_user_id, created_at, updated_at
		 from game_server_readiness
		 where game_server_id = ?
		 order by kind`,
		gameServerID,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("list game server readiness: %w", errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("Failed to close rows in ListGameServerReadiness")
		}
	}()

	var states []GameServerReadiness
	for rows.Next() {
		var readiness GameServerReadiness
		errScan := rows.Scan(
			&readiness.GameServerID,
			&readiness.Kind,
			&readiness.PublicData,
			&readiness.UpdatedByUserID,
			&readiness.CreatedAt,
			&readiness.UpdatedAt,
		)
		if errScan != nil {
			return nil, fmt.Errorf("scan game server readiness: %w", errScan)
		}
		states = append(states, readiness)
	}

	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate game server readiness: %w", errRows)
	}
	return states, nil
}

// DeleteGameServerReadiness removes public readiness state for one server/kind pair.
func (c *Connection) DeleteGameServerReadiness(gameServerID string, kind string) error {
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`delete from game_server_readiness where game_server_id = ? and kind = ?`,
		gameServerID,
		kind,
	)
	if errExec != nil {
		return fmt.Errorf("delete game server readiness: %w", errExec)
	}
	return nil
}
