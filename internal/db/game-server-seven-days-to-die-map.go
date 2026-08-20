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
const sevenDaysToDieMapShareIDByteLen = 16

// GameServerSevenDaysToDieMap stores the cached native snapshot, local notes,
// and public-share state for one server.
type GameServerSevenDaysToDieMap struct {
	GameServerID    string
	ShareEnabled    bool
	NotesJSON       string
	SnapshotJSON    string
	SnapshotAt      sql.NullTime
	UpdatedByUserID sql.NullString
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// GameServerSevenDaysToDieMapShare is one active public map capability.
type GameServerSevenDaysToDieMapShare struct {
	ID           string
	GameServerID string
	Token        string
	CreatedAt    time.Time
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

// CreateGameServerSevenDaysToDieMapShare adds an encrypted public capability.
func (c *Connection) CreateGameServerSevenDaysToDieMapShare(gameServerID string, createdByUserID string) (*GameServerSevenDaysToDieMapShare, error) {
	randomBytes := make([]byte, sevenDaysToDieMapShareTokenByteLen+sevenDaysToDieMapShareIDByteLen)
	_, errRand := rand.Read(randomBytes)
	if errRand != nil {
		return nil, fmt.Errorf("generate 7 Days to Die map share token: %w", errRand)
	}
	token := hex.EncodeToString(randomBytes[:sevenDaysToDieMapShareTokenByteLen])
	shareID := hex.EncodeToString(randomBytes[sevenDaysToDieMapShareTokenByteLen:])
	encryptedToken, errEncrypt := c.EncryptText(token)
	if errEncrypt != nil {
		return nil, fmt.Errorf("encrypt 7 Days to Die map share token: %w", errEncrypt)
	}
	now := time.Now().UTC()
	_, errMap := c.SQLDb.ExecContext(
		c.ctx,
		`insert into game_server_seven_days_to_die_map
			(game_server_id, updated_by_user_id, created_at, updated_at)
		 values (?, ?, ?, ?)
		 on conflict(game_server_id) do update set
			updated_by_user_id = excluded.updated_by_user_id,
			updated_at = excluded.updated_at`,
		gameServerID,
		nullableTrimmedString(createdByUserID),
		now,
		now,
	)
	if errMap != nil {
		return nil, fmt.Errorf("ensure game server 7 Days to Die map settings: %w", errMap)
	}
	_, errInsert := c.SQLDb.ExecContext(
		c.ctx,
		`insert into game_server_seven_days_to_die_map_share
			(id, game_server_id, token_hash, token_encrypted, created_by_user_id, created_at)
		 values (?, ?, ?, ?, ?, ?)`,
		shareID,
		gameServerID,
		hashSevenDaysToDieMapShareToken(token),
		encryptedToken,
		nullableTrimmedString(createdByUserID),
		now,
	)
	if errInsert != nil {
		return nil, fmt.Errorf("create game server 7 Days to Die map share: %w", errInsert)
	}
	return &GameServerSevenDaysToDieMapShare{
		ID: shareID, GameServerID: gameServerID, Token: token, CreatedAt: now,
	}, nil
}

// ListGameServerSevenDaysToDieMapShares returns every active public capability.
func (c *Connection) ListGameServerSevenDaysToDieMapShares(gameServerID string) (result []*GameServerSevenDaysToDieMapShare, resultErr error) {
	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`select id, game_server_id, token_encrypted, created_at
		 from game_server_seven_days_to_die_map_share
		 where game_server_id = ?
		 order by created_at desc, id desc`,
		gameServerID,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("list game server 7 Days to Die map shares: %w", errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close game server 7 Days to Die map shares: %w", errClose))
		}
	}()

	shares := make([]*GameServerSevenDaysToDieMapShare, 0)
	for rows.Next() {
		var share GameServerSevenDaysToDieMapShare
		var encryptedToken sql.NullString
		errScan := rows.Scan(&share.ID, &share.GameServerID, &encryptedToken, &share.CreatedAt)
		if errScan != nil {
			return nil, fmt.Errorf("scan game server 7 Days to Die map share: %w", errScan)
		}
		if encryptedToken.Valid {
			token, errDecrypt := c.DecryptText(encryptedToken.String)
			if errDecrypt != nil {
				return nil, fmt.Errorf("decrypt game server 7 Days to Die map share: %w", errDecrypt)
			}
			share.Token = token
		}
		shares = append(shares, &share)
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate game server 7 Days to Die map shares: %w", errRows)
	}
	return shares, nil
}

// RevokeGameServerSevenDaysToDieMapShare removes one public capability. An
// empty share ID retains the previous revoke-all API behavior.
func (c *Connection) RevokeGameServerSevenDaysToDieMapShare(gameServerID string, shareID string) error {
	shareID = strings.TrimSpace(shareID)
	query := "delete from game_server_seven_days_to_die_map_share where game_server_id = ?"
	args := []any{gameServerID}
	if shareID != "" {
		query += " and id = ?"
		args = append(args, shareID)
	}
	_, errExec := c.SQLDb.ExecContext(c.ctx, query, args...)
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
		sevenDaysToDieMapColumns+`
		 from game_server_seven_days_to_die_map as map
		 join game_server_seven_days_to_die_map_share as share on share.game_server_id = map.game_server_id
		 where share.token_hash = ?`,
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

const sevenDaysToDieMapColumns = `select map.game_server_id, map.notes_json, map.snapshot_json,
	 map.snapshot_at, map.updated_by_user_id, map.created_at, map.updated_at,
	 exists(select 1 from game_server_seven_days_to_die_map_share as active_share where active_share.game_server_id = map.game_server_id)`

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
		&settings.ShareEnabled,
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
