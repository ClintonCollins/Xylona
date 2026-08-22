package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// GameServerSevenDaysToDieMap stores the cached native snapshot and local notes for one server.
type GameServerSevenDaysToDieMap struct {
	GameServerID    string
	NotesJSON       string
	SnapshotJSON    string
	SnapshotAt      sql.NullTime
	UpdatedByUserID sql.NullString
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// GetGameServerSevenDaysToDieMap returns the stored map settings for a game server.
func (c *Connection) GetGameServerSevenDaysToDieMap(gameServerID string) (*GameServerSevenDaysToDieMap, error) {
	row := c.SQLDb.QueryRowContext(c.ctx, sevenDaysToDieMapSelect+" where map.game_server_id = ?", gameServerID)
	settings, errScan := scanGameServerSevenDaysToDieMap(row)
	if errors.Is(errScan, sql.ErrNoRows) {
		return &GameServerSevenDaysToDieMap{GameServerID: gameServerID, NotesJSON: "[]"}, nil
	}
	if errScan != nil {
		return nil, fmt.Errorf("get game server 7 Days to Die map: %w", errScan)
	}
	return settings, nil
}

// UpdateGameServerSevenDaysToDieMapNotes replaces the locally managed map notes.
func (c *Connection) UpdateGameServerSevenDaysToDieMapNotes(gameServerID string, notesJSON string, updatedByUserID string) error {
	now := time.Now().UTC()
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`insert into game_server_seven_days_to_die_map
			(game_server_id, notes_json, updated_by_user_id, created_at, updated_at)
		 values (?, ?, ?, ?, ?)
		 on conflict(game_server_id) do update set
			notes_json = excluded.notes_json,
			updated_by_user_id = excluded.updated_by_user_id,
			updated_at = excluded.updated_at`,
		gameServerID,
		notesJSON,
		nullableTrimmedString(updatedByUserID),
		now,
		now,
	)
	if errExec != nil {
		return fmt.Errorf("update game server 7 Days to Die map notes: %w", errExec)
	}
	return nil
}

// StoreGameServerSevenDaysToDieMapSnapshot caches the latest native map snapshot.
func (c *Connection) StoreGameServerSevenDaysToDieMapSnapshot(gameServerID string, snapshotJSON string, snapshotAt time.Time) error {
	now := time.Now().UTC()
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`insert into game_server_seven_days_to_die_map
			(game_server_id, snapshot_json, snapshot_at, created_at, updated_at)
		 values (?, ?, ?, ?, ?)
		 on conflict(game_server_id) do update set
			snapshot_json = excluded.snapshot_json,
			snapshot_at = excluded.snapshot_at,
			updated_at = excluded.updated_at`,
		gameServerID,
		snapshotJSON,
		snapshotAt.UTC(),
		now,
		now,
	)
	if errExec != nil {
		return fmt.Errorf("store game server 7 Days to Die map snapshot: %w", errExec)
	}
	return nil
}

const sevenDaysToDieMapColumns = `select map.game_server_id, map.notes_json, map.snapshot_json,
	 map.snapshot_at, map.updated_by_user_id, map.created_at, map.updated_at`

const sevenDaysToDieMapSelect = sevenDaysToDieMapColumns + `
	 from game_server_seven_days_to_die_map as map`

func scanGameServerSevenDaysToDieMap(row scanner) (*GameServerSevenDaysToDieMap, error) {
	var settings GameServerSevenDaysToDieMap
	errScan := row.Scan(
		&settings.GameServerID,
		&settings.NotesJSON,
		&settings.SnapshotJSON,
		&settings.SnapshotAt,
		&settings.UpdatedByUserID,
		&settings.CreatedAt,
		&settings.UpdatedAt,
	)
	if errScan != nil {
		return nil, fmt.Errorf("scan game server 7 Days to Die map: %w", errScan)
	}
	return &settings, nil
}
