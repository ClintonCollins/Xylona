package db

import (
	"database/sql"
	"errors"

	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/sqlite"
	"github.com/stephenafamo/bob/dialect/sqlite/im"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// InsertOrUpdateNodeApiKey upserts a node API key by service name.
func (c *Connection) InsertOrUpdateNodeApiKey(exec bob.Executor, setter *models.NodeAPIKeySetter) (*models.NodeAPIKey, error) {
	key, errUpsert := models.NodeAPIKeys.Insert(
		im.OnConflict(models.NodeAPIKeys.Columns.ServiceName).DoUpdate(
			im.SetExcluded("api_key", "updated_at"),
		),
		setter,
	).One(c.ctx, c.DB)
	if errUpsert != nil {
		log.Error().Err(errUpsert).Msg("Error upserting node API key")
		return nil, errUpsert
	}
	return key, nil
}

// GetNodeApiKeys fetches all node API keys.
func (c *Connection) GetNodeApiKeys() ([]*models.NodeAPIKey, error) {
	keys, errGet := models.NodeAPIKeys.Query().All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Msg("Error querying node API keys")
		return nil, errGet
	}
	return keys, nil
}

// GetNodeApiKeyByServiceName fetches a node API key by service name.
func (c *Connection) GetNodeApiKeyByServiceName(serviceName string) (*models.NodeAPIKey, error) {
	key, errGet := models.NodeAPIKeys.Query(models.SelectWhere.NodeAPIKeys.ServiceName.EQ(serviceName)).One(c.ctx, c.DB)
	if errGet != nil {
		if !errors.Is(errGet, sql.ErrNoRows) {
			log.Error().Err(errGet).Msg("Error querying node API key by service name")
		}
		return nil, errGet
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
