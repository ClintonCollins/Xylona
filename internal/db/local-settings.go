package db

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob/dialect/sqlite"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetLocalSettings returns the singleton local settings row.
func (c *Connection) GetLocalSettings() (*models.LocalSetting, error) {
	localSettings, err := models.LocalSettings.Query(models.SelectWhere.LocalSettings.ID.EQ(1)).One(c.ctx, c.DB)
	if err != nil {
		log.Err(err).Msg("Unable to get local settings.")
		return nil, fmt.Errorf("get local settings: %w", err)
	}
	return localSettings, nil
}

// UpdateLocalSettings stores the singleton local settings row.
func (c *Connection) UpdateLocalSettings(localSettings *models.LocalSetting) error {
	_, err := sqlite.RawQuery(
		`insert into local_settings (id, node_id) values (1, ?) on conflict do update set node_id = excluded.node_id`,
		localSettings.NodeID).Exec(c.ctx, c.DB)
	if err != nil {
		log.Err(err).Msg("Unable to update local settings.")
		return fmt.Errorf("update local settings: %w", err)
	}
	return nil
}
