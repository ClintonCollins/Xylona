package db

import (
	"database/sql"
	"errors"

	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob/dialect/sqlite"
	"github.com/stephenafamo/bob/dialect/sqlite/im"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func (c *Connection) RemoveAutomaticallyAddedIPs() error {
	_, err := sqlite.RawQuery(
		`delete from ip where automatically_added = 1`).Exec(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Msg("Error removing automatically added")
		return err
	}
	return nil
}

func (c *Connection) UpsertIP(ipSetter *models.IPSetter) (*models.IP, error) {
	ip, err := models.Ips.Insert(im.OnConflict(models.IPColumns.Address).DoNothing(), ipSetter).One(c.ctx, c.DB)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(err).Msg("Error upserting IP")
		return nil, err
	}
	return ip, err
}

func (c *Connection) GetAllIPs() ([]*models.IP, error) {
	ips, err := models.Ips.Query().All(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Msg("Error querying IPs")
		return nil, err
	}
	return ips, err
}
