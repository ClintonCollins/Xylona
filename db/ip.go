package db

import (
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func (c *Connection) UpsertIP(ipSetter *models.IPSetter) (*models.IP, error) {
	ip, err := models.Ips.Upsert(c.ctx, c.DB, true, nil, nil, ipSetter)
	if err != nil {
		log.Error().Err(err).Msg("Error upserting IP")
		return nil, err
	}
	return ip, err
}

func (c *Connection) GetAllIPs() ([]*models.IP, error) {
	ips, err := models.Ips.Query(c.ctx, c.DB).All()
	if err != nil {
		log.Error().Err(err).Msg("Error querying IPs")
		return nil, err
	}
	return ips, err
}
