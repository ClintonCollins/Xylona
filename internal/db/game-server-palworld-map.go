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

var (
	// ErrPalworldMapShareNotFound intentionally covers unknown, revoked, and
	// malformed public tokens so callers do not reveal share-link state.
	ErrPalworldMapShareNotFound = errors.New("palworld map share was not found")
)

const palworldMapShareTokenByteLen = 32

// GameServerPalworldMap stores the persistent map-source and public-share
// settings for one Palworld server. Only the token hash is retained.
type GameServerPalworldMap struct {
	GameServerID    string
	ShareTokenHash  sql.NullString
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
		`select game_server_id, share_token_hash, layers_json, updated_by_user_id, created_at, updated_at
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

// UpdateGameServerPalworldMapLayers stores the validated external tile-layer
// JSON while preserving any active public share token.
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

// RegenerateGameServerPalworldMapShare replaces any prior public token and
// returns the plaintext token exactly once.
func (c *Connection) RegenerateGameServerPalworldMapShare(gameServerID string, updatedByUserID string) (string, error) {
	rawToken := make([]byte, palworldMapShareTokenByteLen)
	_, errRand := rand.Read(rawToken)
	if errRand != nil {
		return "", fmt.Errorf("generate palworld map share token: %w", errRand)
	}
	token := hex.EncodeToString(rawToken)
	tokenHash := hashPalworldMapShareToken(token)
	now := time.Now().UTC()
	updatedBy := nullableTrimmedString(updatedByUserID)
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`insert into game_server_palworld_map
			(game_server_id, share_token_hash, layers_json, updated_by_user_id, created_at, updated_at)
		 values (?, ?, '[]', ?, ?, ?)
		 on conflict(game_server_id) do update set
			share_token_hash = excluded.share_token_hash,
			updated_by_user_id = excluded.updated_by_user_id,
			updated_at = excluded.updated_at`,
		gameServerID,
		tokenHash,
		updatedBy,
		now,
		now,
	)
	if errExec != nil {
		return "", fmt.Errorf("regenerate game server palworld map share: %w", errExec)
	}
	return token, nil
}

// RevokeGameServerPalworldMapShare invalidates the current public link while
// preserving the map imagery configuration.
func (c *Connection) RevokeGameServerPalworldMapShare(gameServerID string, updatedByUserID string) error {
	now := time.Now().UTC()
	updatedBy := nullableTrimmedString(updatedByUserID)
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`insert into game_server_palworld_map
			(game_server_id, share_token_hash, layers_json, updated_by_user_id, created_at, updated_at)
		 values (?, null, '[]', ?, ?, ?)
		 on conflict(game_server_id) do update set
			share_token_hash = null,
			updated_by_user_id = excluded.updated_by_user_id,
			updated_at = excluded.updated_at`,
		gameServerID,
		updatedBy,
		now,
		now,
	)
	if errExec != nil {
		return fmt.Errorf("revoke game server palworld map share: %w", errExec)
	}
	return nil
}

// GetGameServerPalworldMapByShareToken resolves an active public capability
// token. Unknown, malformed, and revoked tokens return the same sentinel.
func (c *Connection) GetGameServerPalworldMapByShareToken(token string) (*GameServerPalworldMap, error) {
	token = strings.TrimSpace(token)
	decoded, errDecode := hex.DecodeString(token)
	if errDecode != nil || len(decoded) != palworldMapShareTokenByteLen {
		return nil, ErrPalworldMapShareNotFound
	}
	row := c.SQLDb.QueryRowContext(
		c.ctx,
		`select game_server_id, share_token_hash, layers_json, updated_by_user_id, created_at, updated_at
		 from game_server_palworld_map
		 where share_token_hash = ?`,
		hashPalworldMapShareToken(token),
	)
	settings, errScan := scanGameServerPalworldMap(row)
	if errors.Is(errScan, sql.ErrNoRows) {
		return nil, ErrPalworldMapShareNotFound
	}
	if errScan != nil {
		return nil, fmt.Errorf("get palworld map by share token: %w", errScan)
	}
	return settings, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanGameServerPalworldMap(row scanner) (*GameServerPalworldMap, error) {
	var settings GameServerPalworldMap
	errScan := row.Scan(
		&settings.GameServerID,
		&settings.ShareTokenHash,
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

func hashPalworldMapShareToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
