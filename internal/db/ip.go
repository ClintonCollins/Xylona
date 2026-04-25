package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob/dialect/sqlite"
	"github.com/stephenafamo/bob/dialect/sqlite/im"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// ErrIPConflict is returned when an IP upsert hits a conflict and DoNothing is applied.
var ErrIPConflict = errors.New("ip conflict: address already exists for node")

// RemoveAutomaticallyAddedIPs deletes IPs that were auto-discovered.
func (c *Connection) RemoveAutomaticallyAddedIPs() error {
	_, err := sqlite.RawQuery(
		`delete from ip where automatically_added = 1`).Exec(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Msg("Error removing automatically added")
		return fmt.Errorf("remove automatically added IPs: %w", err)
	}
	return nil
}

// UpsertIP inserts or updates an IP record.
func (c *Connection) UpsertIP(ipSetter *models.IPSetter) (*models.IP, error) {
	ip, err := models.Ips.Insert(
		im.OnConflict(models.Ips.Columns.Address, models.Ips.Columns.NodeID).DoNothing(),
		ipSetter,
	).One(c.ctx, c.DB)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrIPConflict
		}
		log.Error().Err(err).Msg("Error upserting IP")
		return nil, fmt.Errorf("upsert IP: %w", err)
	}
	return ip, nil
}

// GetAllIPs returns all stored IP addresses.
func (c *Connection) GetAllIPs() ([]*models.IP, error) {
	ips, err := models.Ips.Query().All(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Msg("Error querying IPs")
		return nil, fmt.Errorf("get all IPs: %w", err)
	}
	return ips, nil
}

// GetIPsByNodeID returns all stored IP addresses for a node.
func (c *Connection) GetIPsByNodeID(nodeID string) ([]*models.IP, error) {
	ips, errIPs := models.Ips.Query(
		models.SelectWhere.Ips.NodeID.EQ(nodeID),
	).All(c.ctx, c.DB)
	if errIPs != nil {
		log.Error().Err(errIPs).Str("node_id", nodeID).Msg("Error querying IPs by node")
		return nil, fmt.Errorf("get IPs by node ID: %w", errIPs)
	}
	return ips, nil
}

// GetIPByAddress returns an IP record by address.
func (c *Connection) GetIPByAddress(address string) (*models.IP, error) {
	ip, errGetIP := models.Ips.Query(models.SelectWhere.Ips.Address.EQ(address)).One(c.ctx, c.DB)
	if errGetIP != nil {
		return nil, fmt.Errorf("get IP by address: %w", errGetIP)
	}
	return ip, nil
}

// GetIPByNodeIDAndAddress returns an IP record by node ID and address.
func (c *Connection) GetIPByNodeIDAndAddress(nodeID string, address string) (*models.IP, error) {
	ip, errGetIP := models.Ips.Query(
		models.SelectWhere.Ips.Address.EQ(address),
		models.SelectWhere.Ips.NodeID.EQ(nodeID),
	).One(c.ctx, c.DB)
	if errGetIP != nil {
		return nil, fmt.Errorf("get IP by node ID and address: %w", errGetIP)
	}
	return ip, nil
}

// InsertIP inserts a new IP record.
func (c *Connection) InsertIP(ipSetter *models.IPSetter) (*models.IP, error) {
	ip, errInsert := models.Ips.Insert(ipSetter).One(c.ctx, c.DB)
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Error inserting IP")
		return nil, fmt.Errorf("insert IP: %w", errInsert)
	}
	return ip, nil
}

// DeleteIP deletes an IP record by address.
func (c *Connection) DeleteIP(address string) error {
	ips := models.IPSlice{&models.IP{Address: address}}
	errWrap := ips.DeleteAll(c.ctx, c.DB)
	if errWrap != nil {
		return fmt.Errorf("delete IP: %w", errWrap)
	}
	return nil
}

// DeleteIPByNodeID deletes an IP record scoped to a node.
func (c *Connection) DeleteIPByNodeID(nodeID string, address string) error {
	ip, errGetIP := c.GetIPByNodeIDAndAddress(nodeID, address)
	if errGetIP != nil {
		return fmt.Errorf("delete IP by node ID: %w", errGetIP)
	}

	ips := models.IPSlice{ip}
	errWrap := ips.DeleteAll(c.ctx, c.DB)
	if errWrap != nil {
		return fmt.Errorf("delete IP by node ID: %w", errWrap)
	}
	return nil
}
