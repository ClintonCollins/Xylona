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

// ErrMinecraftMapShareNotFound indicates that a public map capability is invalid or revoked.
var ErrMinecraftMapShareNotFound = errors.New("minecraft map share was not found")

const minecraftMapShareTokenByteLen = 32

// GameServerMinecraftMap contains the persisted BlueMap settings for one server.
type GameServerMinecraftMap struct {
	GameServerID    string
	Enabled         bool
	WorldName       string
	ShareTokenHash  sql.NullString
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

// RegenerateGameServerMinecraftMapShare replaces and returns the public
// capability token. Only its SHA-256 digest is persisted.
func (c *Connection) RegenerateGameServerMinecraftMapShare(gameServerID string, updatedByUserID string) (string, error) {
	rawToken := make([]byte, minecraftMapShareTokenByteLen)
	_, errRand := rand.Read(rawToken)
	if errRand != nil {
		return "", fmt.Errorf("generate Minecraft map share token: %w", errRand)
	}
	token := hex.EncodeToString(rawToken)
	now := time.Now().UTC()
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`insert into game_server_minecraft_map
			(game_server_id, share_token_hash, updated_by_user_id, created_at, updated_at)
		 values (?, ?, ?, ?, ?)
		 on conflict(game_server_id) do update set
			share_token_hash = excluded.share_token_hash,
			updated_by_user_id = excluded.updated_by_user_id,
			updated_at = excluded.updated_at`,
		gameServerID,
		hashMinecraftMapShareToken(token),
		nullableTrimmedString(updatedByUserID),
		now,
		now,
	)
	if errExec != nil {
		return "", fmt.Errorf("regenerate game server Minecraft map share: %w", errExec)
	}
	return token, nil
}

// RevokeGameServerMinecraftMapShare clears the current public capability.
func (c *Connection) RevokeGameServerMinecraftMapShare(gameServerID string, updatedByUserID string) error {
	now := time.Now().UTC()
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`insert into game_server_minecraft_map
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
		return fmt.Errorf("revoke game server Minecraft map share: %w", errExec)
	}
	return nil
}

// GetGameServerMinecraftMapByShareToken resolves a public capability token.
func (c *Connection) GetGameServerMinecraftMapByShareToken(token string) (*GameServerMinecraftMap, error) {
	token = strings.TrimSpace(token)
	decoded, errDecode := hex.DecodeString(token)
	if errDecode != nil || len(decoded) != minecraftMapShareTokenByteLen {
		return nil, ErrMinecraftMapShareNotFound
	}
	row := c.SQLDb.QueryRowContext(
		c.ctx,
		minecraftMapSelect+" where share_token_hash = ?",
		hashMinecraftMapShareToken(token),
	)
	settings, errScan := scanGameServerMinecraftMap(row)
	if errors.Is(errScan, sql.ErrNoRows) {
		return nil, ErrMinecraftMapShareNotFound
	}
	if errScan != nil {
		return nil, fmt.Errorf("get Minecraft map by share token: %w", errScan)
	}
	return settings, nil
}

const minecraftMapSelect = `select game_server_id, enabled, world_name, share_token_hash,
	 accepted_at, updated_by_user_id, created_at, updated_at
	 from game_server_minecraft_map`

func scanGameServerMinecraftMap(row scanner) (*GameServerMinecraftMap, error) {
	var settings GameServerMinecraftMap
	errScan := row.Scan(
		&settings.GameServerID,
		&settings.Enabled,
		&settings.WorldName,
		&settings.ShareTokenHash,
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

func hashMinecraftMapShareToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
