package db

import (
	"database/sql"
	"errors"
	"fmt"

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
func (c *Connection) decryptAPIKey(stored string) (string, error) {
	if len(c.encryptionKey) == 0 {
		return stored, nil
	}
	decrypted, errDecrypt := xycrypt.Decrypt(c.encryptionKey, stored)
	if errDecrypt != nil {
		return "", fmt.Errorf("decrypt API key: %w", errDecrypt)
	}
	return decrypted, nil
}

// decryptNodeAPIKey decrypts the APIKey field of a NodeAPIKey model in place.
func (c *Connection) decryptNodeAPIKey(key *models.NodeAPIKey) error {
	decrypted, errDecrypt := c.decryptAPIKey(key.APIKey)
	if errDecrypt != nil {
		return errDecrypt
	}
	key.APIKey = decrypted
	return nil
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

	// Decrypt for the caller.
	errDecrypt := c.decryptNodeAPIKey(key)
	if errDecrypt != nil {
		log.Error().Err(errDecrypt).Msg("Error decrypting node API key after upsert")
		return nil, errDecrypt
	}

	return key, nil
}

// GetNodeApiKeys fetches all node API keys, decrypting them before returning.
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
		errDecrypt := c.decryptNodeAPIKey(k)
		if errDecrypt != nil {
			log.Error().Err(errDecrypt).Str("service", k.ServiceName).Msg("Error decrypting node API key")
			return nil, errDecrypt
		}
	}

	return keys, nil
}

// GetNodeApiKeyByServiceName fetches a node API key by service name, decrypting it before returning.
func (c *Connection) GetNodeApiKeyByServiceName(serviceName string) (*models.NodeAPIKey, error) {
	key, errGet := models.NodeAPIKeys.Query(models.SelectWhere.NodeAPIKeys.ServiceName.EQ(serviceName)).One(c.ctx, c.DB)
	if errGet != nil {
		if !errors.Is(errGet, sql.ErrNoRows) {
			log.Error().Err(errGet).Msg("Error querying node API key by service name")
		}
		return nil, errGet
	}

	errDecrypt := c.decryptNodeAPIKey(key)
	if errDecrypt != nil {
		log.Error().Err(errDecrypt).Str("service", serviceName).Msg("Error decrypting node API key")
		return nil, errDecrypt
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
