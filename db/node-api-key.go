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

// encryptAPIKey encrypts an API key if an encryption key is configured.
// Returns the original value unchanged when no encryption key is set (backward-compatible).
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

// decryptAPIKey decrypts an API key if an encryption key is configured.
// Returns the stored value unchanged when no encryption key is set (backward-compatible).
// If the primary key fails and a fallback key is configured, the fallback key
// is tried to support encryption key migration. The second return value is
// true when the fallback key was used, signaling that the caller should
// re-encrypt the value under the primary key.
func (c *Connection) decryptAPIKey(stored string) (string, bool, error) {
	if len(c.encryptionKey) == 0 {
		return stored, false, nil
	}
	decrypted, errDecrypt := xycrypt.Decrypt(c.encryptionKey, stored)
	if errDecrypt == nil {
		return decrypted, false, nil
	}

	// Try the fallback key if configured (supports key migration).
	if len(c.fallbackEncryptionKey) > 0 {
		decryptedFallback, errFallback := xycrypt.Decrypt(c.fallbackEncryptionKey, stored)
		if errFallback == nil {
			return decryptedFallback, true, nil
		}
	}

	return "", false, fmt.Errorf("decrypt API key: %w", errDecrypt)
}

// decryptNodeAPIKey decrypts the APIKey field of a NodeAPIKey model in place.
// Returns true if the fallback key was used (caller should re-encrypt).
func (c *Connection) decryptNodeAPIKey(key *models.NodeAPIKey) (bool, error) {
	decrypted, usedFallback, errDecrypt := c.decryptAPIKey(key.APIKey)
	if errDecrypt != nil {
		return false, errDecrypt
	}
	key.APIKey = decrypted
	return usedFallback, nil
}

// reencryptNodeAPIKey re-encrypts a node API key under the primary encryption
// key. Called when a fallback-key decrypt succeeds so legacy ciphertext is
// migrated transparently.
func (c *Connection) reencryptNodeAPIKey(serviceName, plaintext string) {
	encrypted, errEncrypt := c.encryptAPIKey(plaintext)
	if errEncrypt != nil {
		log.Warn().Err(errEncrypt).Str("service", serviceName).
			Msg("Failed to re-encrypt node API key under primary key")
		return
	}
	_, errUpdate := c.SQLDb.ExecContext(c.ctx,
		`UPDATE node_api_key SET api_key = ?, updated_at = ? WHERE service_name = ?`,
		encrypted, time.Now().UTC(), serviceName,
	)
	if errUpdate != nil {
		log.Warn().Err(errUpdate).Str("service", serviceName).
			Msg("Failed to persist re-encrypted node API key")
		return
	}
	log.Info().Str("service", serviceName).
		Msg("Migrated node API key to primary encryption key")
}

// InsertOrUpdateNodeApiKey upserts a node API key by service name.
// The API key is encrypted before storage when an encryption key is configured.
func (c *Connection) InsertOrUpdateNodeApiKey(exec bob.Executor, setter *models.NodeAPIKeySetter) (*models.NodeAPIKey, error) {
	// Encrypt the API key before storing.
	if setter.APIKey.IsValue() {
		encrypted, errEncrypt := c.encryptAPIKey(setter.APIKey.MustGet())
		if errEncrypt != nil {
			log.Error().Err(errEncrypt).Msg("Error encrypting node API key")
			return nil, errEncrypt
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
		return nil, errUpsert
	}

	// Decrypt for the caller. No re-encrypt needed here since we just wrote
	// the value with the current key.
	_, errDecrypt := c.decryptNodeAPIKey(key)
	if errDecrypt != nil {
		log.Error().Err(errDecrypt).Msg("Error decrypting node API key after upsert")
		return nil, errDecrypt
	}

	return key, nil
}

// GetNodeApiKeys fetches all node API keys, decrypting them before returning.
// Any keys decrypted via fallback key are transparently re-encrypted under
// the primary key.
func (c *Connection) GetNodeApiKeys() ([]*models.NodeAPIKey, error) {
	keys, errGet := models.NodeAPIKeys.Query().All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Msg("Error querying node API keys")
		return nil, errGet
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

// GetNodeApiKeyByServiceName fetches a node API key by service name,
// decrypting it before returning. If the key was decrypted using the fallback
// key, it is transparently re-encrypted under the primary key.
func (c *Connection) GetNodeApiKeyByServiceName(serviceName string) (*models.NodeAPIKey, error) {
	key, errGet := models.NodeAPIKeys.Query(models.SelectWhere.NodeAPIKeys.ServiceName.EQ(serviceName)).One(c.ctx, c.DB)
	if errGet != nil {
		if !errors.Is(errGet, sql.ErrNoRows) {
			log.Error().Err(errGet).Msg("Error querying node API key by service name")
		}
		return nil, errGet
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

// DeleteNodeApiKeyByServiceName deletes a node API key by service name.
func (c *Connection) DeleteNodeApiKeyByServiceName(serviceName string) error {
	_, errExec := sqlite.RawQuery(
		`DELETE FROM node_api_key WHERE service_name = ?`,
		serviceName,
	).Exec(c.ctx, c.DB)
	if errExec != nil {
		log.Error().Err(errExec).Msg("Error deleting node API key")
		return errExec
	}
	return nil
}
