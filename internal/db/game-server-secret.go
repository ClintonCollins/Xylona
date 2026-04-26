package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// GameServerSecretKindEnv stores per-server secret process environment values.
const GameServerSecretKindEnv = "env"

// GameServerSecretState is the safe public state for a configured server secret.
type GameServerSecretState struct {
	Name       string
	Configured bool
	UpdatedAt  time.Time
}

// SetGameServerSecretEnv creates or replaces one encrypted per-server env secret.
func (c *Connection) SetGameServerSecretEnv(gameServerID string, name string, value string, updatedByUserID string) error {
	encryptedValue, errEncrypt := c.EncryptText(value)
	if errEncrypt != nil {
		return fmt.Errorf("set game server secret env: %w", errEncrypt)
	}

	tx, errBegin := c.SQLDb.BeginTx(c.ctx, nil)
	if errBegin != nil {
		return fmt.Errorf("begin game server secret env transaction: %w", errBegin)
	}
	committed := false
	defer rollbackTxIfNeeded(tx, &committed, "game server secret env set")

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
		GameServerSecretKindEnv,
		name,
	).Scan(&existingName)
	if errCanonicalName != nil && !errors.Is(errCanonicalName, sql.ErrNoRows) {
		return fmt.Errorf("lookup game server secret env name: %w", errCanonicalName)
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
		GameServerSecretKindEnv,
		canonicalName,
		encryptedValue,
		updatedBy,
		now,
		now,
	)
	if errExec != nil {
		return fmt.Errorf("upsert game server secret env: %w", errExec)
	}

	errCommit := tx.Commit()
	if errCommit != nil {
		return fmt.Errorf("commit game server secret env transaction: %w", errCommit)
	}
	committed = true
	return nil
}

// ClearGameServerSecretEnv deletes one per-server env secret.
func (c *Connection) ClearGameServerSecretEnv(gameServerID string, name string) error {
	tx, errBegin := c.SQLDb.BeginTx(c.ctx, nil)
	if errBegin != nil {
		return fmt.Errorf("begin clear game server secret env transaction: %w", errBegin)
	}
	committed := false
	defer rollbackTxIfNeeded(tx, &committed, "game server secret env clear")

	_, errExec := tx.ExecContext(
		c.ctx,
		`delete from game_server_secret
		 where game_server_id = ? and kind = ? and lower(name) = lower(?)`,
		gameServerID,
		GameServerSecretKindEnv,
		name,
	)
	if errExec != nil {
		return fmt.Errorf("clear game server secret env: %w", errExec)
	}

	errCommit := tx.Commit()
	if errCommit != nil {
		return fmt.Errorf("commit clear game server secret env transaction: %w", errCommit)
	}
	committed = true
	return nil
}

// ListGameServerSecretEnvStates lists configured secret env names without values.
func (c *Connection) ListGameServerSecretEnvStates(gameServerID string) ([]GameServerSecretState, error) {
	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`select name, updated_at
		 from game_server_secret
		 where game_server_id = ? and kind = ?
		 order by lower(name), name`,
		gameServerID,
		GameServerSecretKindEnv,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("list game server secret env states: %w", errQuery)
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
			return nil, fmt.Errorf("scan game server secret env state: %w", errScan)
		}
		state.Configured = true
		states = append(states, state)
	}

	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate game server secret env states: %w", errRows)
	}
	return states, nil
}

// DecryptGameServerSecretEnv decrypts per-server env secrets for process launch.
func (c *Connection) DecryptGameServerSecretEnv(gameServerID string) (map[string]string, error) {
	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`select name, value_encrypted
		 from game_server_secret
		 where game_server_id = ? and kind = ?
		 order by lower(name), name`,
		gameServerID,
		GameServerSecretKindEnv,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("query game server secret env: %w", errQuery)
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
			return nil, fmt.Errorf("scan game server secret env: %w", errScan)
		}

		plainValue, errDecrypt := c.DecryptText(encryptedValue)
		if errDecrypt != nil {
			return nil, fmt.Errorf("decrypt game server secret env %q: %w", name, errDecrypt)
		}
		secrets[name] = plainValue
	}

	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate game server secret env: %w", errRows)
	}
	return secrets, nil
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
