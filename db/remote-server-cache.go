package db

import (
	"time"

	"github.com/stephenafamo/bob/dialect/sqlite"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func (c *Connection) GetAllRemoteServerCaches() ([]*models.RemoteServerCache, error) {
	servers, err := models.RemoteServerCaches.Query().All(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return servers, nil
}

func (c *Connection) GetRemoteServerCachesByPeerNodeID(peerNodeID string) ([]*models.RemoteServerCache, error) {
	servers, err := models.RemoteServerCaches.Query(
		models.SelectWhere.RemoteServerCaches.PeerNodeID.EQ(peerNodeID),
	).All(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return servers, nil
}

func (c *Connection) GetRemoteServerCacheByCompositeKey(sourceNodeID string, remoteServerID string) (*models.RemoteServerCache, error) {
	server, err := models.RemoteServerCaches.Query(
		models.SelectWhere.RemoteServerCaches.SourceNodeID.EQ(sourceNodeID),
		models.SelectWhere.RemoteServerCaches.RemoteServerID.EQ(remoteServerID),
	).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return server, nil
}

func (c *Connection) GetRemoteServerCacheByID(id string) (*models.RemoteServerCache, error) {
	server, err := models.RemoteServerCaches.Query(
		models.SelectWhere.RemoteServerCaches.ID.EQ(id),
	).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return server, nil
}

func (c *Connection) UpsertRemoteServerCache(
	id string,
	sourceNodeID string,
	peerNodeID string,
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
			(id, source_node_id, peer_node_id, remote_server_id, display_name, status, game_name, game_id,
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
		id, sourceNodeID, peerNodeID, remoteServerID, displayName, status, gameName, gameID,
		ipAddress, port, queryPort, maxPlayers, currentPlayers, mapName, version, nodeName, nodeHost,
		lastRemoteUpdate, now, now, now,
	).Exec(c.ctx, c.DB)
	return err
}

func (c *Connection) MarkRemoteServerCacheStaleByPeerNodeID(peerNodeID string) error {
	_, err := sqlite.RawQuery(
		`UPDATE remote_server_cache SET is_stale = true, updated_at = ? WHERE peer_node_id = ?`,
		time.Now(), peerNodeID,
	).Exec(c.ctx, c.DB)
	return err
}

func (c *Connection) DeleteRemoteServerCacheByPeerNodeID(peerNodeID string) error {
	_, err := sqlite.RawQuery(
		`DELETE FROM remote_server_cache WHERE peer_node_id = ?`,
		peerNodeID,
	).Exec(c.ctx, c.DB)
	return err
}

func (c *Connection) GetRemoteServerCacheByRemoteServerID(remoteServerID string) (*models.RemoteServerCache, error) {
	server, err := models.RemoteServerCaches.Query(
		models.SelectWhere.RemoteServerCaches.RemoteServerID.EQ(remoteServerID),
	).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return server, nil
}

func (c *Connection) UpdateRemoteServerCacheStatus(sourceNodeID string, remoteServerID string, status string) error {
	_, err := sqlite.RawQuery(
		`UPDATE remote_server_cache SET status = ?, updated_at = ? WHERE source_node_id = ? AND remote_server_id = ?`,
		status, time.Now(), sourceNodeID, remoteServerID,
	).Exec(c.ctx, c.DB)
	return err
}

func (c *Connection) DeleteStaleRemoteServerCacheByPeerNodeID(peerNodeID string, olderThan time.Time) error {
	_, err := sqlite.RawQuery(
		`DELETE FROM remote_server_cache WHERE peer_node_id = ? AND is_stale = true AND updated_at < ?`,
		peerNodeID, olderThan,
	).Exec(c.ctx, c.DB)
	return err
}
