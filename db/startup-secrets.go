package db

import (
	"database/sql"
	"errors"
	"fmt"
)

// ValidateEncryptedSecretStorage eagerly decrypts all encrypted-at-rest
// records so startup can fail before serving traffic when the configured key
// cannot read existing secrets.
func (c *Connection) ValidateEncryptedSecretStorage() error {
	errValidateNonFederationSecrets := c.ValidateEncryptedSecretStorageWithoutFederationLocalIdentity()
	if errValidateNonFederationSecrets != nil {
		return errValidateNonFederationSecrets
	}

	errValidateFederationIdentity := c.validateEncryptedFederationLocalIdentity()
	if errValidateFederationIdentity != nil {
		return errValidateFederationIdentity
	}

	return nil
}

// ValidateEncryptedSecretStorageWithoutFederationLocalIdentity eagerly
// decrypts encrypted-at-rest records that predate federation local identity
// key migration so startup can fail before mutating legacy plaintext identity
// material with the wrong runtime key.
func (c *Connection) ValidateEncryptedSecretStorageWithoutFederationLocalIdentity() error {
	errValidateSystemConfig := c.validateEncryptedSystemConfig()
	if errValidateSystemConfig != nil {
		return errValidateSystemConfig
	}

	errValidateNotificationChannels := c.validateEncryptedNotificationChannels()
	if errValidateNotificationChannels != nil {
		return errValidateNotificationChannels
	}

	errValidateNodeAPIKeys := c.validateEncryptedNodeAPIKeys()
	if errValidateNodeAPIKeys != nil {
		return errValidateNodeAPIKeys
	}

	return nil
}

func (c *Connection) validateEncryptedSystemConfig() error {
	rows, errQuery := c.SQLDb.QueryContext(c.ctx, `select key, value from system_config`)
	if errQuery != nil {
		return fmt.Errorf("validate encrypted system config: %w", errQuery)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var key string
		var value string
		errScan := rows.Scan(&key, &value)
		if errScan != nil {
			return fmt.Errorf("validate encrypted system config: scan row: %w", errScan)
		}

		decrypted, usedFallback, errDecrypt := c.decryptConfig(value)
		if errDecrypt != nil {
			return fmt.Errorf("validate encrypted system config %q: %w", key, errDecrypt)
		}
		if usedFallback {
			c.reencryptSystemConfig(key, decrypted)
		}
	}

	errRows := rows.Err()
	if errRows != nil {
		return fmt.Errorf("validate encrypted system config: iterate rows: %w", errRows)
	}

	return nil
}

func (c *Connection) validateEncryptedNotificationChannels() error {
	rows, errQuery := c.SQLDb.QueryContext(c.ctx, `select id, config from notification_channel`)
	if errQuery != nil {
		return fmt.Errorf("validate encrypted notification channels: %w", errQuery)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var id string
		var config string
		errScan := rows.Scan(&id, &config)
		if errScan != nil {
			return fmt.Errorf("validate encrypted notification channels: scan row: %w", errScan)
		}

		decrypted, usedFallback, errDecrypt := c.decryptConfig(config)
		if errDecrypt != nil {
			return fmt.Errorf("validate encrypted notification channel %q: %w", id, errDecrypt)
		}
		if usedFallback {
			c.reencryptNotificationChannelConfig(id, decrypted)
		}
	}

	errRows := rows.Err()
	if errRows != nil {
		return fmt.Errorf("validate encrypted notification channels: iterate rows: %w", errRows)
	}

	return nil
}

func (c *Connection) validateEncryptedNodeAPIKeys() error {
	rows, errQuery := c.SQLDb.QueryContext(c.ctx, `select service_name, api_key from node_api_key`)
	if errQuery != nil {
		return fmt.Errorf("validate encrypted node API keys: %w", errQuery)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var serviceName string
		var apiKey string
		errScan := rows.Scan(&serviceName, &apiKey)
		if errScan != nil {
			return fmt.Errorf("validate encrypted node API keys: scan row: %w", errScan)
		}

		decrypted, usedFallback, errDecrypt := c.decryptAPIKey(apiKey)
		if errDecrypt != nil {
			return fmt.Errorf("validate encrypted node API key %q: %w", serviceName, errDecrypt)
		}
		if usedFallback {
			c.reencryptNodeAPIKey(serviceName, decrypted)
		}
	}

	errRows := rows.Err()
	if errRows != nil {
		return fmt.Errorf("validate encrypted node API keys: iterate rows: %w", errRows)
	}

	return nil
}

func (c *Connection) validateEncryptedFederationLocalIdentity() error {
	row, errGetRow := c.getFederationLocalIdentityRow()
	if errGetRow != nil {
		if errors.Is(errGetRow, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("validate encrypted federation local identity: %w", errGetRow)
	}

	if row.KeyPEMFormat != federationLocalIdentityKeyPEMFormatEncryptedV1 {
		return nil
	}

	_, errDecrypt := c.decryptFederationLocalIdentityKeyPEM(row.KeyPEM, row.KeyPEMFormat)
	if errDecrypt != nil {
		return fmt.Errorf("validate encrypted federation local identity key PEM: %w", errDecrypt)
	}

	return nil
}
