package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/stephenafamo/bob/dialect/sqlite"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetAllRemoteNodes returns all remote node records.
func (c *Connection) GetAllRemoteNodes() ([]*models.Node, error) {
	nodes, err := models.Nodes.Query(models.SelectWhere.Nodes.IsLocal.EQ(false)).All(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get all remote nodes: %w", err)
	}
	return nodes, nil
}

// GetEnabledRemoteNodes returns all enabled remote node records.
func (c *Connection) GetEnabledRemoteNodes() ([]*models.Node, error) {
	nodes, err := models.Nodes.Query(
		models.SelectWhere.Nodes.IsLocal.EQ(false),
		models.SelectWhere.Nodes.Enabled.EQ(true),
	).All(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get enabled remote nodes: %w", err)
	}
	return nodes, nil
}

// GetRemoteNodeByID returns a remote node by ID.
func (c *Connection) GetRemoteNodeByID(id string) (*models.Node, error) {
	node, err := models.Nodes.Query(
		models.SelectWhere.Nodes.ID.EQ(id),
		models.SelectWhere.Nodes.IsLocal.EQ(false),
	).One(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get remote node by ID: %w", err)
	}
	return node, nil
}

// GetRemoteNodeByBaseURL returns a remote node by base URL.
func (c *Connection) GetRemoteNodeByBaseURL(baseURL string) (*models.Node, error) {
	node, err := models.Nodes.Query(
		models.SelectWhere.Nodes.BaseURL.EQ(baseURL),
		models.SelectWhere.Nodes.IsLocal.EQ(false),
	).One(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get remote node by base URL: %w", err)
	}
	return node, nil
}

// InsertRemoteNode inserts a new remote node record.
func (c *Connection) InsertRemoteNode(nodeSetter *models.NodeSetter) (*models.Node, error) {
	node, err := models.Nodes.Insert(nodeSetter).One(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("insert remote node: %w", err)
	}
	return node, nil
}

// UpdateRemoteNode updates an existing remote node record.
func (c *Connection) UpdateRemoteNode(node *models.Node, nodeSetter *models.NodeSetter) (*models.Node, error) {
	errUpdate := node.Update(c.ctx, c.DB, nodeSetter)
	if errUpdate != nil {
		return nil, fmt.Errorf("update remote node: %w", errUpdate)
	}
	return node, nil
}

// DeleteRemoteNodeByID deletes a remote node by ID.
func (c *Connection) DeleteRemoteNodeByID(id string) error {
	nodes := models.NodeSlice{&models.Node{ID: id}}
	errWrap := nodes.DeleteAll(c.ctx, c.DB)
	if errWrap != nil {
		return fmt.Errorf("delete remote node by ID: %w", errWrap)
	}
	return nil
}

// UpdateNodeHealth stores the latest health state for a remote node.
func (c *Connection) UpdateNodeHealth(id string, healthStatus string, lastSeenAt time.Time) error {
	_, err := sqlite.RawQuery(
		`UPDATE node SET health_status = ?, last_seen_at = ?, updated_at = ? WHERE id = ?`,
		healthStatus, lastSeenAt, time.Now(), id,
	).Exec(c.ctx, c.DB)
	if err != nil {
		return fmt.Errorf("update node health: %w", err)
	}
	return nil
}

// UpdateNodeSyncStatus stores the latest sync state for a remote node.
func (c *Connection) UpdateNodeSyncStatus(id string, syncStatus string, lastSyncAt time.Time) error {
	_, err := sqlite.RawQuery(
		`UPDATE node SET last_sync_status = ?, last_sync_at = ?, updated_at = ? WHERE id = ?`,
		syncStatus, lastSyncAt, time.Now(), id,
	).Exec(c.ctx, c.DB)
	if err != nil {
		return fmt.Errorf("update node sync status: %w", err)
	}
	return nil
}

// UpdateNodeIdentity stores identity metadata reported by a remote node.
func (c *Connection) UpdateNodeIdentity(id string, version string, protocolVersion int32, capabilities string, os string) error {
	_, err := sqlite.RawQuery(
		`UPDATE node SET version = ?, protocol_version = ?, capabilities = ?, os = ?, updated_at = ? WHERE id = ?`,
		version, protocolVersion, capabilities, os, time.Now(), id,
	).Exec(c.ctx, c.DB)
	if err != nil {
		return fmt.Errorf("update node identity: %w", err)
	}
	return nil
}

// UpdateNodeLastSeen records the latest successful contact time for a node.
func (c *Connection) UpdateNodeLastSeen(id string) error {
	_, err := sqlite.RawQuery(
		`UPDATE node SET last_seen_at = ?, updated_at = ? WHERE id = ?`,
		time.Now(), time.Now(), id,
	).Exec(c.ctx, c.DB)
	if err != nil {
		return fmt.Errorf("update node last seen: %w", err)
	}
	return nil
}

// GetNodeSyncIntervalSeconds returns the configured sync interval for a node.
func (c *Connection) GetNodeSyncIntervalSeconds(id string) (int32, error) {
	var syncIntervalSeconds int32
	errQuery := c.SQLDb.QueryRowContext(c.ctx, `SELECT sync_interval_seconds FROM node WHERE id = ?`, id).Scan(&syncIntervalSeconds)
	if errQuery != nil {
		return 0, fmt.Errorf("get node sync interval seconds: %w", errQuery)
	}
	return syncIntervalSeconds, nil
}

// GetOrCreatePeerSyncState retrieves or creates a sync state record for a node.
// GetOrCreatePeerSyncState returns the sync state row for a peer node.
func (c *Connection) GetOrCreatePeerSyncState(nodeID string) (*models.PeerSyncState, error) {
	syncState, errQuery := models.PeerSyncStates.Query(
		models.SelectWhere.PeerSyncStates.NodeID.EQ(nodeID),
	).One(c.ctx, c.DB)
	if errQuery == nil {
		return syncState, nil
	}

	if !errors.Is(errQuery, sql.ErrNoRows) {
		return nil, fmt.Errorf("get peer sync state: %w", errQuery)
	}

	newID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		return nil, fmt.Errorf("generate peer sync state ID: %w", errID)
	}
	setter := &models.PeerSyncStateSetter{
		ID:     omit.From(newID.String()),
		NodeID: omit.From(nodeID),
	}

	syncState, errInsert := models.PeerSyncStates.Insert(setter).One(c.ctx, c.DB)
	if errInsert == nil {
		return syncState, nil
	}
	if !isSQLiteUniqueConstraintError(errInsert) {
		return nil, fmt.Errorf("insert peer sync state: %w", errInsert)
	}

	syncState, errQuery = models.PeerSyncStates.Query(
		models.SelectWhere.PeerSyncStates.NodeID.EQ(nodeID),
	).One(c.ctx, c.DB)
	if errQuery != nil {
		return nil, fmt.Errorf("get peer sync state after insert race: %w", errQuery)
	}
	return syncState, nil
}

// UpdatePeerSyncStateError stores retry state after a sync failure.
func (c *Connection) UpdatePeerSyncStateError(nodeID string, lastError string, retryCount int64, nextRetryAt time.Time) error {
	_, err := sqlite.RawQuery(
		`UPDATE peer_sync_state SET last_error = ?, retry_count = ?, next_retry_at = ?, updated_at = ? WHERE node_id = ?`,
		lastError, retryCount, nextRetryAt, time.Now(), nodeID,
	).Exec(c.ctx, c.DB)
	if err != nil {
		return fmt.Errorf("update peer sync state error: %w", err)
	}
	return nil
}

// UpdatePeerSyncStateSuccess stores sync state after a successful sync.
func (c *Connection) UpdatePeerSyncStateSuccess(nodeID string, cursor string) error {
	now := time.Now()
	_, err := sqlite.RawQuery(
		`UPDATE peer_sync_state SET last_cursor = ?, last_full_sync_at = ?, last_error = '', retry_count = 0, next_retry_at = NULL, updated_at = ? WHERE node_id = ?`,
		cursor, now, now, nodeID,
	).Exec(c.ctx, c.DB)
	if err != nil {
		return fmt.Errorf("update peer sync state success: %w", err)
	}
	return nil
}

// SetNodeDeparted stores whether a remote node has departed the cluster.
func (c *Connection) SetNodeDeparted(id string, departed bool) error {
	_, errExec := sqlite.RawQuery(
		`UPDATE node SET departed = ?, updated_at = ? WHERE id = ?`,
		departed, time.Now(), id,
	).Exec(c.ctx, c.DB)
	if errExec != nil {
		return fmt.Errorf("set node departed: %w", errExec)
	}
	return nil
}

// GetNodeDeparted returns whether a remote node is marked departed.
func (c *Connection) GetNodeDeparted(id string) (bool, error) {
	var departed bool
	errQuery := c.SQLDb.QueryRowContext(c.ctx,
		`SELECT departed FROM node WHERE id = ?`, id,
	).Scan(&departed)
	if errQuery != nil {
		return false, fmt.Errorf("get node departed: %w", errQuery)
	}
	return departed, nil
}

func isSQLiteUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}

// IsBusyError reports whether the error represents SQLite database contention.
func (c *Connection) IsBusyError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "SQLITE_BUSY")
}
