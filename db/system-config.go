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
// before returning. Returns sql.ErrNoRows if the key does not exist.
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

	decrypted, errDecrypt := c.decryptConfig(config.Value)
	if errDecrypt != nil {
		log.Error().Err(errDecrypt).Str("key", key).Msg("Error decrypting system config")
		return "", fmt.Errorf("decrypt system config %q: %w", key, errDecrypt)
	}

	return decrypted, nil
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
