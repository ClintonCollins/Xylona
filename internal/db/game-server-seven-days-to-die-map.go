package db

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrSevenDaysToDieMapShareNotFound indicates that a public map capability is invalid or revoked.
var ErrSevenDaysToDieMapShareNotFound = errors.New("7 Days to Die map share was not found")

const sevenDaysToDieMapShareTokenByteLen = 32

// GameServerSevenDaysToDieMap stores the cached native snapshot, local notes,
// and hashed public capability for one server.
type GameServerSevenDaysToDieMap struct {
	GameServerID    string
	ShareTokenHash  sql.NullString
	NotesJSON       string
	SnapshotJSON    string
	SnapshotAt      sql.NullTime
	UpdatedByUserID sql.NullString
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// GetGameServerSevenDaysToDieMap returns the stored map settings for a game server.
func (c *Connection) GetGameServerSevenDaysToDieMap(gameServerID string) (*GameServerSevenDaysToDieMap, error) {
	row := c.SQLDb.QueryRowContext(c.ctx, sevenDaysToDieMapSelect+" where game_server_id = ?", gameServerID)
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

// RegenerateGameServerSevenDaysToDieMapShare replaces and returns the public capability token.
func (c *Connection) RegenerateGameServerSevenDaysToDieMapShare(gameServerID string, updatedByUserID string) (string, error) {
	rawToken := make([]byte, sevenDaysToDieMapShareTokenByteLen)
	_, errRand := rand.Read(rawToken)
	if errRand != nil {
		return "", fmt.Errorf("generate 7 Days to Die map share token: %w", errRand)
	}
	token := hex.EncodeToString(rawToken)
	now := time.Now().UTC()
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`insert into game_server_seven_days_to_die_map
			(game_server_id, share_token_hash, updated_by_user_id, created_at, updated_at)
		 values (?, ?, ?, ?, ?)
		 on conflict(game_server_id) do update set
			share_token_hash = excluded.share_token_hash,
			updated_by_user_id = excluded.updated_by_user_id,
			updated_at = excluded.updated_at`,
		gameServerID,
		hashSevenDaysToDieMapShareToken(token),
		nullableTrimmedString(updatedByUserID),
		now,
		now,
	)
	if errExec != nil {
		return "", fmt.Errorf("regenerate game server 7 Days to Die map share: %w", errExec)
	}
	return token, nil
}

// RevokeGameServerSevenDaysToDieMapShare clears the public capability token.
func (c *Connection) RevokeGameServerSevenDaysToDieMapShare(gameServerID string, updatedByUserID string) error {
	now := time.Now().UTC()
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`insert into game_server_seven_days_to_die_map
			(game_server_id, share_token_hash, updated_by_user_id, created_at, updated_at)
		 values (?, null, ?, ?, ?)
		 on conflict(game_server_id) do update set
			share_token_hash = null,
			updated_by_user_id = excluded.updated_by_user_id,
			updated_at = excluded.updated_at`,
		gameServerID,
		nullableTrimmedString(updatedByUserID),
		now,
		now,
	)
	if errExec != nil {
		return fmt.Errorf("revoke game server 7 Days to Die map share: %w", errExec)
	}
	return nil
}

// GetGameServerSevenDaysToDieMapByShareToken resolves a valid public capability token.
func (c *Connection) GetGameServerSevenDaysToDieMapByShareToken(token string) (*GameServerSevenDaysToDieMap, error) {
	token = strings.TrimSpace(token)
	decoded, errDecode := hex.DecodeString(token)
	if errDecode != nil || len(decoded) != sevenDaysToDieMapShareTokenByteLen {
		return nil, ErrSevenDaysToDieMapShareNotFound
	}
	row := c.SQLDb.QueryRowContext(
		c.ctx,
		sevenDaysToDieMapSelect+" where share_token_hash = ?",
		hashSevenDaysToDieMapShareToken(token),
	)
	settings, errScan := scanGameServerSevenDaysToDieMap(row)
	if errors.Is(errScan, sql.ErrNoRows) {
		return nil, ErrSevenDaysToDieMapShareNotFound
	}
	if errScan != nil {
		return nil, fmt.Errorf("get 7 Days to Die map by share token: %w", errScan)
	}
	return settings, nil
}

const sevenDaysToDieMapSelect = `select game_server_id, share_token_hash, notes_json, snapshot_json,
	 snapshot_at, updated_by_user_id, created_at, updated_at
	 from game_server_seven_days_to_die_map`

func scanGameServerSevenDaysToDieMap(row scanner) (*GameServerSevenDaysToDieMap, error) {
	var settings GameServerSevenDaysToDieMap
	errScan := row.Scan(
		&settings.GameServerID,
		&settings.ShareTokenHash,
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

func hashSevenDaysToDieMapShareToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
