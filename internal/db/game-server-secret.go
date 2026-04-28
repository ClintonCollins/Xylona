package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// GameServerSecretKindEnv stores per-server secret process environment values.
	GameServerSecretKindEnv = "env"
	// GameServerSecretKindSteamGSLT stores Steam Game Server Login Tokens.
	// #nosec G101 -- This is a database kind label, not a secret value.
	GameServerSecretKindSteamGSLT = "steam_gslt"
	// GameServerSecretKindHytaleRefreshToken stores Hytale OAuth refresh tokens.
	// #nosec G101 -- This is a database kind label, not a secret value.
	GameServerSecretKindHytaleRefreshToken = "hytale_refresh_token"

	// GameServerSecretNameSteamGSLT is the single secret name used for Steam GSLT values.
	// #nosec G101 -- This is a database name label, not a secret value.
	GameServerSecretNameSteamGSLT = "token"
	// GameServerSecretNameHytaleRefreshToken is the single secret name used for Hytale refresh tokens.
	// #nosec G101 -- This is a database name label, not a secret value.
	GameServerSecretNameHytaleRefreshToken = "refresh_token"
)

// GameServerSecretState is the safe public state for a configured server secret.
type GameServerSecretState struct {
	Name       string
	Configured bool
	UpdatedAt  time.Time
}

// SetGameServerSecretEnv creates or replaces one encrypted per-server env secret.
func (c *Connection) SetGameServerSecretEnv(gameServerID string, name string, value string, updatedByUserID string) error {
	return c.SetGameServerSecret(gameServerID, GameServerSecretKindEnv, name, value, updatedByUserID)
}

// SetGameServerSecret creates or replaces one encrypted per-server secret.
func (c *Connection) SetGameServerSecret(gameServerID string, kind string, name string, value string, updatedByUserID string) error {
	errKind := validateGameServerSecretKind(kind)
	if errKind != nil {
		return errKind
	}

	encryptedValue, errEncrypt := c.EncryptText(value)
	if errEncrypt != nil {
		return fmt.Errorf("set game server secret %s %q: %w", kind, name, errEncrypt)
	}

	tx, errBegin := c.SQLDb.BeginTx(c.ctx, nil)
	if errBegin != nil {
		return fmt.Errorf("begin game server secret %s transaction: %w", kind, errBegin)
	}
	committed := false
	defer rollbackTxIfNeeded(tx, &committed, "game server secret set")

	now := time.Now().UTC()
	updatedBy := sql.NullString{String: strings.TrimSpace(updatedByUserID), Valid: strings.TrimSpace(updatedByUserID) != ""}
	canonicalName := name
	var existingName string
	errCanonicalName := tx.QueryRowContext(
		c.ctx,
		`select name
		 from game_server_secret
		 where game_server_id = ? and kind = ? and lower(name) = lower(?)
		 order by name
		 limit 1`,
		gameServerID,
		kind,
		name,
	).Scan(&existingName)
	if errCanonicalName != nil && !errors.Is(errCanonicalName, sql.ErrNoRows) {
		return fmt.Errorf("lookup game server secret %s name: %w", kind, errCanonicalName)
	}
	if existingName != "" {
		canonicalName = existingName
	}

	_, errExec := tx.ExecContext(
		c.ctx,
		`insert into game_server_secret
			(game_server_id, kind, name, value_encrypted, updated_by_user_id, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?)
		 on conflict(game_server_id, kind, name) do update set
			value_encrypted = excluded.value_encrypted,
			updated_by_user_id = excluded.updated_by_user_id,
			updated_at = excluded.updated_at`,
		gameServerID,
		kind,
		canonicalName,
		encryptedValue,
		updatedBy,
		now,
		now,
	)
	if errExec != nil {
		return fmt.Errorf("upsert game server secret %s %q: %w", kind, name, errExec)
	}

	errCommit := tx.Commit()
	if errCommit != nil {
		return fmt.Errorf("commit game server secret %s transaction: %w", kind, errCommit)
	}
	committed = true
	return nil
}

// ClearGameServerSecretEnv deletes one per-server env secret.
func (c *Connection) ClearGameServerSecretEnv(gameServerID string, name string) error {
	return c.ClearGameServerSecret(gameServerID, GameServerSecretKindEnv, name)
}

// ClearGameServerSecret deletes one per-server secret.
func (c *Connection) ClearGameServerSecret(gameServerID string, kind string, name string) error {
	errKind := validateGameServerSecretKind(kind)
	if errKind != nil {
		return errKind
	}

	tx, errBegin := c.SQLDb.BeginTx(c.ctx, nil)
	if errBegin != nil {
		return fmt.Errorf("begin clear game server secret %s transaction: %w", kind, errBegin)
	}
	committed := false
	defer rollbackTxIfNeeded(tx, &committed, "game server secret clear")

	_, errExec := tx.ExecContext(
		c.ctx,
		`delete from game_server_secret
		 where game_server_id = ? and kind = ? and lower(name) = lower(?)`,
		gameServerID,
		kind,
		name,
	)
	if errExec != nil {
		return fmt.Errorf("clear game server secret %s %q: %w", kind, name, errExec)
	}

	errCommit := tx.Commit()
	if errCommit != nil {
		return fmt.Errorf("commit clear game server secret %s transaction: %w", kind, errCommit)
	}
	committed = true
	return nil
}

// ListGameServerSecretEnvStates lists configured secret env names without values.
func (c *Connection) ListGameServerSecretEnvStates(gameServerID string) ([]GameServerSecretState, error) {
	return c.ListGameServerSecretStates(gameServerID, GameServerSecretKindEnv)
}

// ListGameServerSecretStates lists configured secret names without values.
func (c *Connection) ListGameServerSecretStates(gameServerID string, kind string) ([]GameServerSecretState, error) {
	errKind := validateGameServerSecretKind(kind)
	if errKind != nil {
		return nil, errKind
	}

	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`select name, updated_at
		 from game_server_secret
		 where game_server_id = ? and kind = ?
		 order by lower(name), name`,
		gameServerID,
		kind,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("list game server secret %s states: %w", kind, errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("Failed to close rows in ListGameServerSecretEnvStates")
		}
	}()

	var states []GameServerSecretState
	for rows.Next() {
		var state GameServerSecretState
		errScan := rows.Scan(&state.Name, &state.UpdatedAt)
		if errScan != nil {
			return nil, fmt.Errorf("scan game server secret %s state: %w", kind, errScan)
		}
		state.Configured = true
		states = append(states, state)
	}

	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate game server secret %s states: %w", kind, errRows)
	}
	return states, nil
}

// DecryptGameServerSecretEnv decrypts per-server env secrets for process launch.
func (c *Connection) DecryptGameServerSecretEnv(gameServerID string) (map[string]string, error) {
	return c.DecryptGameServerSecrets(gameServerID, GameServerSecretKindEnv)
}

// DecryptGameServerSecrets decrypts per-server secrets of one kind.
func (c *Connection) DecryptGameServerSecrets(gameServerID string, kind string) (map[string]string, error) {
	errKind := validateGameServerSecretKind(kind)
	if errKind != nil {
		return nil, errKind
	}

	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`select name, value_encrypted
		 from game_server_secret
		 where game_server_id = ? and kind = ?
		 order by lower(name), name`,
		gameServerID,
		kind,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("query game server secret %s: %w", kind, errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("Failed to close rows in DecryptGameServerSecretEnv")
		}
	}()

	secrets := map[string]string{}
	for rows.Next() {
		var name string
		var encryptedValue string
		errScan := rows.Scan(&name, &encryptedValue)
		if errScan != nil {
			return nil, fmt.Errorf("scan game server secret %s: %w", kind, errScan)
		}

		plainValue, errDecrypt := c.DecryptText(encryptedValue)
		if errDecrypt != nil {
			return nil, fmt.Errorf("decrypt game server secret %s %q: %w", kind, name, errDecrypt)
		}
		secrets[name] = plainValue
	}

	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate game server secret %s: %w", kind, errRows)
	}
	return secrets, nil
}

// HasGameServerSecret reports whether one per-server secret is configured.
func (c *Connection) HasGameServerSecret(gameServerID string, kind string, name string) (bool, error) {
	errKind := validateGameServerSecretKind(kind)
	if errKind != nil {
		return false, errKind
	}

	var configured int
	errQuery := c.SQLDb.QueryRowContext(
		c.ctx,
		`select 1
		 from game_server_secret
		 where game_server_id = ? and kind = ? and lower(name) = lower(?)
		 limit 1`,
		gameServerID,
		kind,
		name,
	).Scan(&configured)
	if errQuery != nil {
		if errors.Is(errQuery, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("check game server secret %s %q: %w", kind, name, errQuery)
	}
	return configured == 1, nil
}

// DecryptGameServerSecret decrypts one per-server secret.
func (c *Connection) DecryptGameServerSecret(gameServerID string, kind string, name string) (string, bool, error) {
	errKind := validateGameServerSecretKind(kind)
	if errKind != nil {
		return "", false, errKind
	}

	var encryptedValue string
	errQuery := c.SQLDb.QueryRowContext(
		c.ctx,
		`select value_encrypted
		 from game_server_secret
		 where game_server_id = ? and kind = ? and lower(name) = lower(?)
		 limit 1`,
		gameServerID,
		kind,
		name,
	).Scan(&encryptedValue)
	if errQuery != nil {
		if errors.Is(errQuery, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("query game server secret %s %q: %w", kind, name, errQuery)
	}

	value, errDecrypt := c.DecryptText(encryptedValue)
	if errDecrypt != nil {
		return "", false, fmt.Errorf("decrypt game server secret %s %q: %w", kind, name, errDecrypt)
	}
	return value, true, nil
}

func validateGameServerSecretKind(kind string) error {
	switch kind {
	case GameServerSecretKindEnv, GameServerSecretKindSteamGSLT, GameServerSecretKindHytaleRefreshToken:
		return nil
	default:
		return fmt.Errorf("unsupported game server secret kind %q", kind)
	}
}

func rollbackTxIfNeeded(tx *sql.Tx, committed *bool, operation string) {
	if committed != nil && *committed {
		return
	}
	errRollback := tx.Rollback()
	if errRollback != nil && !errors.Is(errRollback, sql.ErrTxDone) {
		log.Warn().Err(errRollback).Str("operation", operation).Msg("Failed to rollback transaction")
	}
}
