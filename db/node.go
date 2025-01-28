package db

import "github.com/ClintonCollins/Xylona/sql/models"

func (c *Connection) GetAllNodes() ([]*models.Node, error) {
	nodes, err := models.Nodes.Query().All(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

func (c *Connection) GetNodeByID(id string) (*models.Node, error) {
	node, err := models.Nodes.Query(models.SelectWhere.Nodes.ID.EQ(id)).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (c *Connection) InsertNode(nodeSetter *models.NodeSetter) (*models.Node, error) {
	node, err := models.Nodes.Insert(nodeSetter).One(c.ctx, c.DB)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (c *Connection) UpdateNode(node *models.Node, nodeSetter *models.NodeSetter) (*models.Node, error) {
	err := node.Update(c.ctx, c.DB, nodeSetter)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (c *Connection) DeleteNodeByID(id string) error {
	nodes := models.NodeSlice{&models.Node{ID: id}}
	return nodes.DeleteAll(c.ctx, c.DB)
}