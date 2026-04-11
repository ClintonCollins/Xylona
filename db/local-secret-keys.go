package db

import (
	"fmt"
	"time"

	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetSecretKeyByID returns a local secret key by numeric ID.
func (c *Connection) GetSecretKeyByID(secretKeyID int64) (*models.LocalSecretKey, error) {
	localSecretKey, err := models.LocalSecretKeys.Query(models.SelectWhere.LocalSecretKeys.ID.EQ(secretKeyID)).One(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get secret key by ID: %w", err)
	}
	return localSecretKey, nil
}

// GetSecretKeyByHash returns a local secret key by its hash.
func (c *Connection) GetSecretKeyByHash(secretKeyHash string) (*models.LocalSecretKey, error) {
	localSecretKey, err := models.LocalSecretKeys.Query(models.SelectWhere.LocalSecretKeys.SecretKeyHash.EQ(secretKeyHash)).One(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get secret key by hash: %w", err)
	}
	return localSecretKey, nil
}

// GetAllSecretKeys returns all local secret keys.
func (c *Connection) GetAllSecretKeys() ([]*models.LocalSecretKey, error) {
	localSecretKeys, err := models.LocalSecretKeys.Query().All(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get all secret keys: %w", err)
	}
	return localSecretKeys, nil
}

// InsertSecretKey inserts a new local secret key record.
func (c *Connection) InsertSecretKey(secretKeySetter *models.LocalSecretKeySetter) (*models.LocalSecretKey, error) {
	localSecretKey, err := models.LocalSecretKeys.Insert(secretKeySetter).One(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("insert secret key: %w", err)
	}
	return localSecretKey, nil
}

// DeleteSecretKeyByID deletes a local secret key by ID.
func (c *Connection) DeleteSecretKeyByID(secretKeyID int64) error {
	_, err := c.SQLDb.Exec("delete from local_secret_keys where id = ?", secretKeyID) //nolint:noctx // database/sql only exposes ExecContext on the wrapped executor path used elsewhere.
	if err != nil {
		return fmt.Errorf("delete secret key by ID: %w", err)
	}
	return nil
}

// UpdateSecretKeyUsage persists the last successful usage metadata for a local secret key.
func (c *Connection) UpdateSecretKeyUsage(secretKey *models.LocalSecretKey, accessedFrom string, usedAt time.Time) error {
	errUpdate := secretKey.Update(c.ctx, c.DB, &models.LocalSecretKeySetter{
		LastAccessedFrom: omitnull.From(accessedFrom),
		LastUsedAt:       omitnull.From(usedAt),
	})
	if errUpdate != nil {
		return fmt.Errorf("update secret key usage: %w", errUpdate)
	}
	return nil
}
