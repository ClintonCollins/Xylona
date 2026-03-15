package db

import (
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/stephenafamo/bob/dialect/sqlite"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (c *Connection) GetAllPeerNodes() ([]*models.PeerNode, error) {
	peerNodes, err := models.PeerNodes.Query().All(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return peerNodes, nil
}

func (c *Connection) GetEnabledPeerNodes() ([]*models.PeerNode, error) {
	peerNodes, err := models.PeerNodes.Query(models.SelectWhere.PeerNodes.Enabled.EQ(true)).All(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return peerNodes, nil
}

func (c *Connection) GetPeerNodeByID(id string) (*models.PeerNode, error) {
	peerNode, err := models.PeerNodes.Query(models.SelectWhere.PeerNodes.ID.EQ(id)).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return peerNode, nil
}

func (c *Connection) GetPeerNodeByNodeID(nodeID string) (*models.PeerNode, error) {
	peerNode, err := models.PeerNodes.Query(models.SelectWhere.PeerNodes.NodeID.EQ(nodeID)).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return peerNode, nil
}

func (c *Connection) GetPeerNodeByBaseURL(baseURL string) (*models.PeerNode, error) {
	peerNode, err := models.PeerNodes.Query(models.SelectWhere.PeerNodes.BaseURL.EQ(baseURL)).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return peerNode, nil
}

func (c *Connection) InsertPeerNode(peerNodeSetter *models.PeerNodeSetter) (*models.PeerNode, error) {
	peerNode, err := models.PeerNodes.Insert(peerNodeSetter).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return peerNode, nil
}

func (c *Connection) UpdatePeerNode(peerNode *models.PeerNode, peerNodeSetter *models.PeerNodeSetter) (*models.PeerNode, error) {
	errUpdate := peerNode.Update(c.ctx, c.DB, peerNodeSetter)
	if errUpdate != nil {
		return nil, errUpdate
	}
	return peerNode, nil
}

func (c *Connection) DeletePeerNodeByID(id string) error {
	peerNodes := models.PeerNodeSlice{&models.PeerNode{ID: id}}
	return peerNodes.DeleteAll(c.ctx, c.DB)
}

func (c *Connection) UpdatePeerNodeHealth(id string, healthStatus string, lastSeenAt time.Time) error {
	_, err := sqlite.RawQuery(
		`UPDATE peer_node SET health_status = ?, last_seen_at = ?, updated_at = ? WHERE id = ?`,
		healthStatus, lastSeenAt, time.Now(), id,
	).Exec(c.ctx, c.DB)
	return err
}

func (c *Connection) UpdatePeerNodeSyncStatus(id string, syncStatus string, lastSyncAt time.Time) error {
	_, err := sqlite.RawQuery(
		`UPDATE peer_node SET last_sync_status = ?, last_sync_at = ?, updated_at = ? WHERE id = ?`,
		syncStatus, lastSyncAt, time.Now(), id,
	).Exec(c.ctx, c.DB)
	return err
}

func (c *Connection) UpdatePeerNodeIdentity(id string, nodeID string, name string, version string, protocolVersion int32, capabilities string) error {
	_, err := sqlite.RawQuery(
		`UPDATE peer_node SET node_id = ?, name = ?, version = ?, protocol_version = ?, capabilities = ?, updated_at = ? WHERE id = ?`,
		nodeID, name, version, protocolVersion, capabilities, time.Now(), id,
	).Exec(c.ctx, c.DB)
	return err
}

func (c *Connection) UpdatePeerNodeLastSeen(id string) error {
	_, err := sqlite.RawQuery(
		`UPDATE peer_node SET last_seen_at = ?, updated_at = ? WHERE id = ?`,
		time.Now(), time.Now(), id,
	).Exec(c.ctx, c.DB)
	return err
}

// GetOrCreatePeerSyncState retrieves or creates a sync state record for a peer.
func (c *Connection) GetOrCreatePeerSyncState(peerNodeID string) (*models.PeerSyncState, error) {
	syncState, err := models.PeerSyncStates.Query(
		models.SelectWhere.PeerSyncStates.PeerNodeID.EQ(peerNodeID),
	).One(c.ctx, c.DB)
	if err != nil {
		newID, errID := helpers.GenerateUniqueID()
		if errID != nil {
			return nil, errID
		}
		setter := &models.PeerSyncStateSetter{
			ID:         omit.From(newID.String()),
			PeerNodeID: omit.From(peerNodeID),
		}
		syncState, err = models.PeerSyncStates.Insert(setter).One(c.ctx, c.DB)
		if err != nil {
			return nil, err
		}
	}
	return syncState, nil
}

func (c *Connection) UpdatePeerSyncStateError(peerNodeID string, lastError string, retryCount int32, nextRetryAt time.Time) error {
	_, err := sqlite.RawQuery(
		`UPDATE peer_sync_state SET last_error = ?, retry_count = ?, next_retry_at = ?, updated_at = ? WHERE peer_node_id = ?`,
		lastError, retryCount, nextRetryAt, time.Now(), peerNodeID,
	).Exec(c.ctx, c.DB)
	return err
}

func (c *Connection) UpdatePeerSyncStateSuccess(peerNodeID string, cursor string) error {
	now := time.Now()
	_, err := sqlite.RawQuery(
		`UPDATE peer_sync_state SET last_cursor = ?, last_full_sync_at = ?, last_error = '', retry_count = 0, next_retry_at = NULL, updated_at = ? WHERE peer_node_id = ?`,
		cursor, now, now, peerNodeID,
	).Exec(c.ctx, c.DB)
	return err
}
