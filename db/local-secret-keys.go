package db

import "github.com/ClintonCollins/Xylona/sql/models"

func (c *Connection) GetSecretKeyByID(secretKeyID int32) (*models.LocalSecretKey, error) {
	localSecretKey, err := models.LocalSecretKeys.Query(models.SelectWhere.LocalSecretKeys.ID.EQ(secretKeyID)).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return localSecretKey, nil
}

func (c *Connection) GetSecretKeyByHash(secretKeyHash string) (*models.LocalSecretKey, error) {
	localSecretKey, err := models.LocalSecretKeys.Query(models.SelectWhere.LocalSecretKeys.SecretKeyHash.EQ(secretKeyHash)).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return localSecretKey, nil
}

func (c *Connection) GetAllSecretKeys() ([]*models.LocalSecretKey, error) {
	localSecretKeys, err := models.LocalSecretKeys.Query().All(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return localSecretKeys, nil
}

func (c *Connection) InsertSecretKey(secretKeySetter *models.LocalSecretKeySetter) (*models.LocalSecretKey, error) {
	localSecretKey, err := models.LocalSecretKeys.Insert(secretKeySetter).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return localSecretKey, nil
}

func (c *Connection) DeleteSecretKeyByID(secretKeyID int32) error {
	_, err := c.SQLDb.Exec("delete from local_secret_keys where id = ?", secretKeyID)
	if err != nil {
		return err
	}
	return nil
}
