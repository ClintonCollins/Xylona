package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/stephenafamo/bob/dialect/sqlite"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// ErrAmbiguousRemoteServerCache indicates a cache lookup matched multiple rows.
var ErrAmbiguousRemoteServerCache = errors.New("ambiguous remote server cache lookup")

// GetAllRemoteServerCaches returns all remote server cache rows.
func (c *Connection) GetAllRemoteServerCaches() ([]*models.RemoteServerCache, error) {
	servers, err := models.RemoteServerCaches.Query().All(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get all remote server caches: %w", err)
	}
	return servers, nil
}

// GetRemoteServerCachesByNodeID returns cache rows for a source node.
func (c *Connection) GetRemoteServerCachesByNodeID(nodeID string) ([]*models.RemoteServerCache, error) {
	servers, err := models.RemoteServerCaches.Query(
		models.SelectWhere.RemoteServerCaches.NodeID.EQ(nodeID),
	).All(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get remote server caches by node ID: %w", err)
	}
	return servers, nil
}

// GetRemoteServerCacheByCompositeKey returns a cache row by source node and server ID.
func (c *Connection) GetRemoteServerCacheByCompositeKey(sourceNodeID string, remoteServerID string) (*models.RemoteServerCache, error) {
	server, err := models.RemoteServerCaches.Query(
		models.SelectWhere.RemoteServerCaches.SourceNodeID.EQ(sourceNodeID),
		models.SelectWhere.RemoteServerCaches.RemoteServerID.EQ(remoteServerID),
	).One(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get remote server cache by composite key: %w", err)
	}
	return server, nil
}

// GetRemoteServerCacheByID returns a cache row by ID.
func (c *Connection) GetRemoteServerCacheByID(id string) (*models.RemoteServerCache, error) {
	server, err := models.RemoteServerCaches.Query(
		models.SelectWhere.RemoteServerCaches.ID.EQ(id),
	).One(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get remote server cache by ID: %w", err)
	}
	return server, nil
}

// UpsertRemoteServerCache inserts or updates a remote server cache row.
func (c *Connection) UpsertRemoteServerCache(
	id string,
	sourceNodeID string,
	nodeID string,
	remoteServerID string,
	displayName string,
	status string,
	gameName string,
	gameID string,
	ipAddress string,
	port int32,
	queryPort int32,
	maxPlayers int32,
	currentPlayers int32,
	mapName string,
	version string,
	nodeName string,
	nodeHost string,
	lastRemoteUpdate time.Time,
) error {
	now := time.Now()
	_, err := sqlite.RawQuery(
		`INSERT INTO remote_server_cache
			(id, source_node_id, node_id, remote_server_id, display_name, status, game_name, game_id,
			 ip_address, port, query_port, max_players, current_players, map_name, version, node_name, node_host,
			 last_remote_update, last_synced_at, is_stale, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, false, ?, ?)
		ON CONFLICT(source_node_id, remote_server_id) DO UPDATE SET
			display_name = excluded.display_name,
			status = excluded.status,
			game_name = excluded.game_name,
			game_id = excluded.game_id,
			ip_address = excluded.ip_address,
			port = excluded.port,
			query_port = excluded.query_port,
			max_players = excluded.max_players,
			current_players = excluded.current_players,
			map_name = excluded.map_name,
			version = excluded.version,
			node_name = excluded.node_name,
			node_host = excluded.node_host,
			last_remote_update = excluded.last_remote_update,
			last_synced_at = excluded.last_synced_at,
			is_stale = false,
			updated_at = excluded.updated_at`,
		id, sourceNodeID, nodeID, remoteServerID, displayName, status, gameName, gameID,
		ipAddress, port, queryPort, maxPlayers, currentPlayers, mapName, version, nodeName, nodeHost,
		lastRemoteUpdate, now, now, now,
	).Exec(c.ctx, c.DB)
	if err != nil {
		return fmt.Errorf("upsert remote server cache: %w", err)
	}
	return nil
}

// MarkRemoteServerCacheStaleByNodeID marks cache rows stale for a node.
func (c *Connection) MarkRemoteServerCacheStaleByNodeID(nodeID string) error {
	_, err := sqlite.RawQuery(
		`UPDATE remote_server_cache SET is_stale = true, updated_at = ? WHERE node_id = ?`,
		time.Now(), nodeID,
	).Exec(c.ctx, c.DB)
	if err != nil {
		return fmt.Errorf("mark remote server cache stale by node ID: %w", err)
	}
	return nil
}

// DeleteRemoteServerCacheByNodeID deletes cache rows for a source node.
func (c *Connection) DeleteRemoteServerCacheByNodeID(nodeID string) error {
	_, err := sqlite.RawQuery(
		`DELETE FROM remote_server_cache WHERE node_id = ?`,
		nodeID,
	).Exec(c.ctx, c.DB)
	if err != nil {
		return fmt.Errorf("delete remote server cache by node ID: %w", err)
	}
	return nil
}

// DeleteRemoteServerCacheByCompositeKey deletes a cache row by composite key.
func (c *Connection) DeleteRemoteServerCacheByCompositeKey(sourceNodeID string, remoteServerID string) error {
	_, err := sqlite.RawQuery(
		`DELETE FROM remote_server_cache WHERE source_node_id = ? AND remote_server_id = ?`,
		sourceNodeID, remoteServerID,
	).Exec(c.ctx, c.DB)
	if err != nil {
		return fmt.Errorf("delete remote server cache by composite key: %w", err)
	}
	return nil
}

// DeleteOrphanedRemoteServerCacheByNodeReferences deletes cache rows with missing nodes.
func (c *Connection) DeleteOrphanedRemoteServerCacheByNodeReferences() error {
	_, err := sqlite.RawQuery(
		`DELETE FROM remote_server_cache
		WHERE node_id NOT IN (SELECT id FROM node WHERE is_local = false)
		   OR source_node_id NOT IN (SELECT id FROM node WHERE is_local = false)`,
	).Exec(c.ctx, c.DB)
	if err != nil {
		return fmt.Errorf("delete orphaned remote server cache by node references: %w", err)
	}
	return nil
}

// GetRemoteServerCacheByRemoteServerID returns a cache row by remote server ID.
func (c *Connection) GetRemoteServerCacheByRemoteServerID(remoteServerID string) (*models.RemoteServerCache, error) {
	servers, err := models.RemoteServerCaches.Query(
		models.SelectWhere.RemoteServerCaches.RemoteServerID.EQ(remoteServerID),
	).All(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get remote server cache by remote server ID: %w", err)
	}

	if len(servers) == 0 {
		return nil, sql.ErrNoRows
	}
	if len(servers) > 1 {
		return nil, ErrAmbiguousRemoteServerCache
	}

	return servers[0], nil
}

// UpdateRemoteServerCacheStatus updates cached server status.
func (c *Connection) UpdateRemoteServerCacheStatus(sourceNodeID string, remoteServerID string, status string) error {
	now := time.Now()
	_, err := sqlite.RawQuery(
		`UPDATE remote_server_cache
		SET status = ?, last_synced_at = ?, is_stale = false, updated_at = ?
		WHERE source_node_id = ? AND remote_server_id = ?`,
		status, now, now, sourceNodeID, remoteServerID,
	).Exec(c.ctx, c.DB)
	if err != nil {
		return fmt.Errorf("update remote server cache status: %w", err)
	}
	return nil
}

// UpdateRemoteServerCacheVersion updates cached server version data.
func (c *Connection) UpdateRemoteServerCacheVersion(sourceNodeID string, remoteServerID string, version string) error {
	now := time.Now()
	_, err := sqlite.RawQuery(
		`UPDATE remote_server_cache
		SET version = ?, last_synced_at = ?, is_stale = false, updated_at = ?
		WHERE source_node_id = ? AND remote_server_id = ?`,
		version, now, now, sourceNodeID, remoteServerID,
	).Exec(c.ctx, c.DB)
	if err != nil {
		return fmt.Errorf("update remote server cache version: %w", err)
	}
	return nil
}

// DeleteStaleRemoteServerCacheByNodeID deletes stale cache rows for a node.
func (c *Connection) DeleteStaleRemoteServerCacheByNodeID(nodeID string, olderThan time.Time) error {
	_, err := sqlite.RawQuery(
		`DELETE FROM remote_server_cache WHERE node_id = ? AND is_stale = true AND updated_at < ?`,
		nodeID, olderThan,
	).Exec(c.ctx, c.DB)
	if err != nil {
		return fmt.Errorf("delete stale remote server cache by node ID: %w", err)
	}
	return nil
}
