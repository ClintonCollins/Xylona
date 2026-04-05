package db

import (
	"fmt"

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

// DeleteNodeByID deletes a node by ID.
func (c *Connection) DeleteNodeByID(id string) error {
	nodes := models.NodeSlice{&models.Node{ID: id}}
	errWrap := nodes.DeleteAll(c.ctx, c.DB)
	if errWrap != nil {
		return fmt.Errorf("delete node by ID: %w", errWrap)
	}
	return nil
}
