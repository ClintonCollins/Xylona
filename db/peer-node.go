package db

import (
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/stephenafamo/bob/dialect/sqlite"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (c *Connection) GetAllRemoteNodes() ([]*models.Node, error) {
	nodes, err := models.Nodes.Query(models.SelectWhere.Nodes.IsLocal.EQ(false)).All(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

func (c *Connection) GetEnabledRemoteNodes() ([]*models.Node, error) {
	nodes, err := models.Nodes.Query(
		models.SelectWhere.Nodes.IsLocal.EQ(false),
		models.SelectWhere.Nodes.Enabled.EQ(true),
	).All(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

func (c *Connection) GetRemoteNodeByID(id string) (*models.Node, error) {
	node, err := models.Nodes.Query(
		models.SelectWhere.Nodes.ID.EQ(id),
		models.SelectWhere.Nodes.IsLocal.EQ(false),
	).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (c *Connection) GetRemoteNodeByBaseURL(baseURL string) (*models.Node, error) {
	node, err := models.Nodes.Query(
		models.SelectWhere.Nodes.BaseURL.EQ(baseURL),
		models.SelectWhere.Nodes.IsLocal.EQ(false),
	).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (c *Connection) InsertRemoteNode(nodeSetter *models.NodeSetter) (*models.Node, error) {
	node, err := models.Nodes.Insert(nodeSetter).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (c *Connection) UpdateRemoteNode(node *models.Node, nodeSetter *models.NodeSetter) (*models.Node, error) {
	errUpdate := node.Update(c.ctx, c.DB, nodeSetter)
	if errUpdate != nil {
		return nil, errUpdate
	}
	return node, nil
}

func (c *Connection) DeleteRemoteNodeByID(id string) error {
	nodes := models.NodeSlice{&models.Node{ID: id}}
	return nodes.DeleteAll(c.ctx, c.DB)
}

func (c *Connection) UpdateNodeHealth(id string, healthStatus string, lastSeenAt time.Time) error {
	_, err := sqlite.RawQuery(
		`UPDATE node SET health_status = ?, last_seen_at = ?, updated_at = ? WHERE id = ?`,
		healthStatus, lastSeenAt, time.Now(), id,
	).Exec(c.ctx, c.DB)
	return err
}

func (c *Connection) UpdateNodeSyncStatus(id string, syncStatus string, lastSyncAt time.Time) error {
	_, err := sqlite.RawQuery(
		`UPDATE node SET last_sync_status = ?, last_sync_at = ?, updated_at = ? WHERE id = ?`,
		syncStatus, lastSyncAt, time.Now(), id,
	).Exec(c.ctx, c.DB)
	return err
}

func (c *Connection) UpdateNodeIdentity(id string, name string, version string, protocolVersion int32, capabilities string) error {
	_, err := sqlite.RawQuery(
		`UPDATE node SET name = ?, version = ?, protocol_version = ?, capabilities = ?, updated_at = ? WHERE id = ?`,
		name, version, protocolVersion, capabilities, time.Now(), id,
	).Exec(c.ctx, c.DB)
	return err
}

func (c *Connection) UpdateNodeLastSeen(id string) error {
	_, err := sqlite.RawQuery(
		`UPDATE node SET last_seen_at = ?, updated_at = ? WHERE id = ?`,
		time.Now(), time.Now(), id,
	).Exec(c.ctx, c.DB)
	return err
}

// GetOrCreatePeerSyncState retrieves or creates a sync state record for a node.
func (c *Connection) GetOrCreatePeerSyncState(nodeID string) (*models.PeerSyncState, error) {
	syncState, err := models.PeerSyncStates.Query(
		models.SelectWhere.PeerSyncStates.NodeID.EQ(nodeID),
	).One(c.ctx, c.DB)
	if err != nil {
		newID, errID := helpers.GenerateUniqueID()
		if errID != nil {
			return nil, errID
		}
		setter := &models.PeerSyncStateSetter{
			ID:     omit.From(newID.String()),
			NodeID: omit.From(nodeID),
		}
		syncState, err = models.PeerSyncStates.Insert(setter).One(c.ctx, c.DB)
		if err != nil {
			return nil, err
		}
	}
	return syncState, nil
}

func (c *Connection) UpdatePeerSyncStateError(nodeID string, lastError string, retryCount int32, nextRetryAt time.Time) error {
	_, err := sqlite.RawQuery(
		`UPDATE peer_sync_state SET last_error = ?, retry_count = ?, next_retry_at = ?, updated_at = ? WHERE node_id = ?`,
		lastError, retryCount, nextRetryAt, time.Now(), nodeID,
	).Exec(c.ctx, c.DB)
	return err
}

func (c *Connection) UpdatePeerSyncStateSuccess(nodeID string, cursor string) error {
	now := time.Now()
	_, err := sqlite.RawQuery(
		`UPDATE peer_sync_state SET last_cursor = ?, last_full_sync_at = ?, last_error = '', retry_count = 0, next_retry_at = NULL, updated_at = ? WHERE node_id = ?`,
		cursor, now, now, nodeID,
	).Exec(c.ctx, c.DB)
	return err
}
