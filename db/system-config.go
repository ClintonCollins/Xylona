package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob/dialect/sqlite/im"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetSystemConfig retrieves a system config value by key, decrypting it
// before returning. Returns sql.ErrNoRows if the key does not exist. If the
// value was decrypted using the fallback key, it is transparently re-encrypted
// under the primary key.
func (c *Connection) GetSystemConfig(key string) (string, error) {
	config, errGet := models.SystemConfigs.Query(
		models.SelectWhere.SystemConfigs.Key.EQ(key),
	).One(c.ctx, c.DB)
	if errGet != nil {
		if !errors.Is(errGet, sql.ErrNoRows) {
			log.Error().Err(errGet).Str("key", key).Msg("Error querying system config")
		}
		return "", fmt.Errorf("get system config: %w", errGet)
	}

	decrypted, usedFallback, errDecrypt := c.decryptConfig(config.Value)
	if errDecrypt != nil {
		log.Error().Err(errDecrypt).Str("key", key).Msg("Error decrypting system config")
		return "", fmt.Errorf("decrypt system config %q: %w", key, errDecrypt)
	}

	if usedFallback {
		c.reencryptSystemConfig(key, decrypted)
	}

	return decrypted, nil
}

// reencryptSystemConfig re-encrypts a system config value under the primary
// encryption key. Called when a fallback-key decrypt succeeds so legacy
// ciphertext is migrated transparently.
func (c *Connection) reencryptSystemConfig(key, plaintext string) {
	encrypted, errEncrypt := c.encryptConfig(plaintext)
	if errEncrypt != nil {
		log.Warn().Err(errEncrypt).Str("key", key).
			Msg("Failed to re-encrypt system config under primary key")
		return
	}
	_, errUpdate := c.SQLDb.ExecContext(c.ctx,
		`UPDATE system_config SET value = ?, updated_at = ? WHERE key = ?`,
		encrypted, time.Now().UTC(), key,
	)
	if errUpdate != nil {
		log.Warn().Err(errUpdate).Str("key", key).
			Msg("Failed to persist re-encrypted system config")
		return
	}
	log.Info().Str("key", key).
		Msg("Migrated system config to primary encryption key")
}

// SetSystemConfig sets a system config value by key, encrypting it before
// storage. If the key already exists, the value and updated_at are replaced.
func (c *Connection) SetSystemConfig(key, value string) error {
	encrypted, errEncrypt := c.encryptConfig(value)
	if errEncrypt != nil {
		log.Error().Err(errEncrypt).Str("key", key).Msg("Error encrypting system config")
		return fmt.Errorf("encrypt system config: %w", errEncrypt)
	}

	now := time.Now().UTC()

	setter := &models.SystemConfigSetter{
		Key:       omit.From(key),
		Value:     omit.From(encrypted),
		UpdatedAt: omit.From(now),
	}

	_, errUpsert := models.SystemConfigs.Insert(
		im.OnConflict(models.SystemConfigs.Columns.Key).DoUpdate(
			im.SetExcluded("value", "updated_at"),
		),
		setter,
	).One(c.ctx, c.DB)
	if errUpsert != nil {
		log.Error().Err(errUpsert).Str("key", key).Msg("Error upserting system config")
		return fmt.Errorf("set system config: %w", errUpsert)
	}

	return nil
}
