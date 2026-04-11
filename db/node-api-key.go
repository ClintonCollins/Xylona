package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/sqlite"
	"github.com/stephenafamo/bob/dialect/sqlite/im"

	"github.com/ClintonCollins/Xylona/pkg/xycrypt"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// encryptAPIKey encrypts an API key with the configured encryption key.
func (c *Connection) encryptAPIKey(plaintext string) (string, error) {
	if len(c.encryptionKey) == 0 {
		return plaintext, nil
	}
	encrypted, errEncrypt := xycrypt.Encrypt(c.encryptionKey, plaintext)
	if errEncrypt != nil {
		return "", fmt.Errorf("encrypt API key: %w", errEncrypt)
	}
	return encrypted, nil
}

// decryptAPIKey decrypts an API key with the configured encryption key. The
// second return value reports whether the fallback key was used.
func (c *Connection) decryptAPIKey(stored string) (string, bool, error) {
	if len(c.encryptionKey) == 0 {
		return stored, false, nil
	}
	decrypted, errDecrypt := xycrypt.Decrypt(c.encryptionKey, stored)
	if errDecrypt == nil {
		return decrypted, false, nil
	}

	if len(c.fallbackEncryptionKey) > 0 {
		decryptedFallback, errDecryptFallback := xycrypt.Decrypt(c.fallbackEncryptionKey, stored)
		if errDecryptFallback == nil {
			return decryptedFallback, true, nil
		}
	}

	return "", false, fmt.Errorf("decrypt API key: %w", errDecrypt)
}

// decryptNodeAPIKey decrypts the APIKey field of a NodeAPIKey model in place.
// The return value reports whether the fallback key was used.
func (c *Connection) decryptNodeAPIKey(key *models.NodeAPIKey) (bool, error) {
	decrypted, usedFallback, errDecrypt := c.decryptAPIKey(key.APIKey)
	if errDecrypt != nil {
		return false, errDecrypt
	}
	key.APIKey = decrypted
	return usedFallback, nil
}

func (c *Connection) reencryptNodeAPIKey(serviceName, plaintext string) {
	encrypted, errEncrypt := c.encryptAPIKey(plaintext)
	if errEncrypt != nil {
		log.Warn().Err(errEncrypt).Str("service", serviceName).
			Msg("Failed to re-encrypt node API key under primary key")
		return
	}

	_, errUpdate := c.SQLDb.ExecContext(
		c.ctx,
		`UPDATE node_api_key SET api_key = ?, updated_at = ? WHERE service_name = ?`,
		encrypted,
		time.Now().UTC(),
		serviceName,
	)
	if errUpdate != nil {
		log.Warn().Err(errUpdate).Str("service", serviceName).
			Msg("Failed to persist re-encrypted node API key")
		return
	}
}

// InsertOrUpdateNodeAPIKey upserts a node API key by service name.
// The API key is encrypted before storage when an encryption key is configured.
func (c *Connection) InsertOrUpdateNodeAPIKey(exec bob.Executor, setter *models.NodeAPIKeySetter) (*models.NodeAPIKey, error) {
	// Encrypt the API key before storing.
	if setter.APIKey.IsValue() {
		encrypted, errEncrypt := c.encryptAPIKey(setter.APIKey.MustGet())
		if errEncrypt != nil {
			log.Error().Err(errEncrypt).Msg("Error encrypting node API key")
			return nil, fmt.Errorf("encrypt node API key: %w", errEncrypt)
		}
		setter.APIKey = omit.From(encrypted)
	}

	key, errUpsert := models.NodeAPIKeys.Insert(
		im.OnConflict(models.NodeAPIKeys.Columns.ServiceName).DoUpdate(
			im.SetExcluded("api_key", "updated_at"),
		),
		setter,
	).One(c.ctx, exec)
	if errUpsert != nil {
		log.Error().Err(errUpsert).Msg("Error upserting node API key")
		return nil, fmt.Errorf("insert or update node API key: %w", errUpsert)
	}

	// Decrypt for the caller before returning the newly written value.
	_, errDecrypt := c.decryptNodeAPIKey(key)
	if errDecrypt != nil {
		log.Error().Err(errDecrypt).Msg("Error decrypting node API key after upsert")
		return nil, errDecrypt
	}

	return key, nil
}

// GetNodeAPIKeys fetches all node API keys, decrypting them before returning.
func (c *Connection) GetNodeAPIKeys() ([]*models.NodeAPIKey, error) {
	keys, errGet := models.NodeAPIKeys.Query().All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Msg("Error querying node API keys")
		return nil, fmt.Errorf("get node API keys: %w", errGet)
	}

	for _, k := range keys {
		usedFallback, errDecrypt := c.decryptNodeAPIKey(k)
		if errDecrypt != nil {
			log.Error().Err(errDecrypt).Str("service", k.ServiceName).Msg("Error decrypting node API key")
			return nil, errDecrypt
		}
		if usedFallback {
			c.reencryptNodeAPIKey(k.ServiceName, k.APIKey)
		}
	}

	return keys, nil
}

// GetNodeAPIKeyByServiceName fetches a node API key by service name,
// decrypting it before returning.
func (c *Connection) GetNodeAPIKeyByServiceName(serviceName string) (*models.NodeAPIKey, error) {
	key, errGet := models.NodeAPIKeys.Query(models.SelectWhere.NodeAPIKeys.ServiceName.EQ(serviceName)).One(c.ctx, c.DB)
	if errGet != nil {
		if !errors.Is(errGet, sql.ErrNoRows) {
			log.Error().Err(errGet).Msg("Error querying node API key by service name")
		}
		return nil, fmt.Errorf("get node API key by service name: %w", errGet)
	}

	usedFallback, errDecrypt := c.decryptNodeAPIKey(key)
	if errDecrypt != nil {
		log.Error().Err(errDecrypt).Str("service", serviceName).Msg("Error decrypting node API key")
		return nil, errDecrypt
	}
	if usedFallback {
		c.reencryptNodeAPIKey(serviceName, key.APIKey)
	}

	return key, nil
}

// DeleteNodeAPIKeyByServiceName deletes a node API key by service name.
func (c *Connection) DeleteNodeAPIKeyByServiceName(serviceName string) error {
	_, errExec := sqlite.RawQuery(
		`DELETE FROM node_api_key WHERE service_name = ?`,
		serviceName,
	).Exec(c.ctx, c.DB)
	if errExec != nil {
		log.Error().Err(errExec).Msg("Error deleting node API key")
		return fmt.Errorf("delete node API key by service name: %w", errExec)
	}
	return nil
}
