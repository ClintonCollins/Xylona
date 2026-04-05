package db

import (
	"database/sql"
	"fmt"
)

// CountGameServers returns the total number of game servers.
func (c *Connection) CountGameServers() (int, error) {
	var count int
	errQuery := c.SQLDb.QueryRowContext(c.ctx, `SELECT COUNT(*) FROM game_server`).Scan(&count)
	if errQuery != nil {
		return 0, fmt.Errorf("count game servers: %w", errQuery)
	}
	return count, nil
}

// CountRunningGameServers returns the number of game servers with status 'ONLINE'.
func (c *Connection) CountRunningGameServers() (int, error) {
	var count int
	errQuery := c.SQLDb.QueryRowContext(c.ctx, `SELECT COUNT(*) FROM game_server WHERE status = 'ONLINE'`).Scan(&count)
	if errQuery != nil {
		return 0, fmt.Errorf("count running game servers: %w", errQuery)
	}
	return count, nil
}

// CountUsers returns the total number of users.
func (c *Connection) CountUsers() (int, error) {
	var count int
	errQuery := c.SQLDb.QueryRowContext(c.ctx, `SELECT COUNT(*) FROM user`).Scan(&count)
	if errQuery != nil {
		return 0, fmt.Errorf("count users: %w", errQuery)
	}
	return count, nil
}

// GetLocalNodeID returns the local node ID from settings.
func (c *Connection) GetLocalNodeID() (string, error) {
	var nodeID string
	errQuery := c.SQLDb.QueryRowContext(c.ctx, `SELECT node_id FROM local_settings WHERE id = 1`).Scan(&nodeID)
	if errQuery != nil {
		if errQuery == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("get local node ID: %w", errQuery)
	}
	return nodeID, nil
}
