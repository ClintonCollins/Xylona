package db

import (
	"database/sql"
	"errors"

	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob/dialect/sqlite"
	"github.com/stephenafamo/bob/dialect/sqlite/im"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// ErrIPConflict is returned when an IP upsert hits a conflict and DoNothing is applied.
var ErrIPConflict = errors.New("ip conflict: address already exists")

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
	ip, err := models.Ips.Insert(im.OnConflict(models.Ips.Columns.Address).DoNothing(), ipSetter).One(c.ctx, c.DB)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrIPConflict
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

func (c *Connection) GetIPByAddress(address string) (*models.IP, error) {
	ip, errGetIP := models.Ips.Query(models.SelectWhere.Ips.Address.EQ(address)).One(c.ctx, c.DB)
	if errGetIP != nil {
		return nil, errGetIP
	}
	return ip, nil
}

func (c *Connection) InsertIP(ipSetter *models.IPSetter) (*models.IP, error) {
	ip, errInsert := models.Ips.Insert(ipSetter).One(c.ctx, c.DB)
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Error inserting IP")
		return nil, errInsert
	}
	return ip, nil
}

func (c *Connection) DeleteIP(address string) error {
	ips := models.IPSlice{&models.IP{Address: address}}
	return ips.DeleteAll(c.ctx, c.DB)
}
