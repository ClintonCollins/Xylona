package db

import (
	"fmt"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetAllNodes returns all node records.
func (c *Connection) GetAllNodes() ([]*models.Node, error) {
	nodes, err := models.Nodes.Query().All(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get all nodes: %w", err)
	}
	return nodes, nil
}

// GetNodeByID returns a node by ID.
func (c *Connection) GetNodeByID(id string) (*models.Node, error) {
	node, err := models.Nodes.Query(models.SelectWhere.Nodes.ID.EQ(id)).One(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("get node by ID: %w", err)
	}
	return node, nil
}

// InsertNode inserts a new node record.
func (c *Connection) InsertNode(nodeSetter *models.NodeSetter) (*models.Node, error) {
	node, err := models.Nodes.Insert(nodeSetter).One(c.ctx, c.DB)
	if err != nil {
		return nil, fmt.Errorf("insert node: %w", err)
	}
	return node, nil
}

// UpdateNode updates an existing node record.
func (c *Connection) UpdateNode(node *models.Node, nodeSetter *models.NodeSetter) (*models.Node, error) {
	err := node.Update(c.ctx, c.DB, nodeSetter)
	if err != nil {
		return nil, fmt.Errorf("update node: %w", err)
	}
	return node, nil
}

// UpsertSelfNode ensures the controller's embedded node row exists with the
// given id and display name. Called during startup; never updates the
// cert_fingerprint / shared_secret_encrypted fields because the embedded node
// does not use them (the in-process NodeClient bypasses the transport).
func (c *Connection) UpsertSelfNode(id string, name string) error {
	setter := &models.NodeSetter{
		ID:      omit.From(id),
		Name:    omit.From(name),
		Enabled: omit.From(true),
	}
	_, err := models.Nodes.Insert(setter).One(c.ctx, c.DB)
	if err != nil {
		return fmt.Errorf("upsert self node: %w", err)
	}
	return nil
}

// RegisterRemoteNode inserts a fresh remote-node row after a successful
// join-token bootstrap. The controller owns the generated id and
// shared-secret material; the node provides only its name, listen URL, and
// pinned certificate fingerprint.
func (c *Connection) RegisterRemoteNode(id, name, listenURL, certFingerprint, sharedSecretEncrypted string) (*models.Node, error) {
	now := time.Now().UTC()
	setter := &models.NodeSetter{
		ID:                    omit.From(id),
		Name:                  omit.From(name),
		ListenURL:             omit.From(listenURL),
		CertFingerprint:       omit.From(certFingerprint),
		SharedSecretEncrypted: omit.From(sharedSecretEncrypted),
		Enabled:               omit.From(true),
		LastSeenAt:            omitnull.From(now),
	}
	node, errInsert := models.Nodes.Insert(setter).One(c.ctx, c.DB)
	if errInsert != nil {
		return nil, fmt.Errorf("register remote node: %w", errInsert)
	}
	return node, nil
}

// UpdateNodeLastSeen stamps last_seen_at on the given node id. Called
// opportunistically after a successful controller-to-node RPC so the UI can
// surface per-node health without a separate heartbeat.
func (c *Connection) UpdateNodeLastSeen(id string, at time.Time) error {
	_, err := c.SQLDb.ExecContext(c.ctx,
		`update node set last_seen_at = ?, updated_at = ? where id = ?`,
		at.UTC(), at.UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("update node last_seen_at: %w", err)
	}
	return nil
}

// DeleteNodeByID deletes a node by ID.
func (c *Connection) DeleteNodeByID(id string) error {
	nodes := models.NodeSlice{&models.Node{ID: id}}
	errWrap := nodes.DeleteAll(c.ctx, c.DB)
	if errWrap != nil {
		return fmt.Errorf("delete node by ID: %w", errWrap)
	}
	return nil
}
