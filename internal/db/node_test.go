package db

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestInsertNodeAndGetNodeByID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "node-insert.sqlite")

	setter := &models.NodeSetter{
		ID:        omit.From("node-test"),
		Name:      omit.From("Test Node"),
		ListenURL: omit.From("http://10.0.0.5:8080"),
		Enabled:   omit.From(true),
	}

	node, errInsert := conn.InsertNode(setter)
	if errInsert != nil {
		t.Fatalf("InsertNode() error = %v", errInsert)
	}
	if node.ID != "node-test" {
		t.Errorf("InsertNode().ID = %q, want %q", node.ID, "node-test")
	}
	if node.Name != "Test Node" {
		t.Errorf("InsertNode().Name = %q, want %q", node.Name, "Test Node")
	}

	fetched, errGet := conn.GetNodeByID("node-test")
	if errGet != nil {
		t.Fatalf("GetNodeByID() error = %v", errGet)
	}
	if fetched.ListenURL != "http://10.0.0.5:8080" {
		t.Errorf("GetNodeByID().ListenURL = %q, want %q", fetched.ListenURL, "http://10.0.0.5:8080")
	}
	if !fetched.Enabled {
		t.Errorf("GetNodeByID().Enabled = false, want true")
	}
}

func TestGetNodeByIDNotFound(t *testing.T) {
	conn := newRBACMigratedConnection(t, "node-not-found.sqlite")

	_, errGet := conn.GetNodeByID("nonexistent")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetNodeByID() error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestGetAllNodes(t *testing.T) {
	conn := newRBACMigratedConnection(t, "node-list.sqlite")

	for i, id := range []string{"node-a", "node-b"} {
		setter := &models.NodeSetter{
			ID:        omit.From(id),
			Name:      omit.From("Node " + id),
			ListenURL: omit.From(fmt.Sprintf("http://localhost:%d", 8080+i)),
			Enabled:   omit.From(true),
		}
		_, errInsert := conn.InsertNode(setter)
		if errInsert != nil {
			t.Fatalf("InsertNode(%s) error = %v", id, errInsert)
		}
	}

	nodes, errGet := conn.GetAllNodes()
	if errGet != nil {
		t.Fatalf("GetAllNodes() error = %v", errGet)
	}
	if len(nodes) < 2 {
		t.Errorf("GetAllNodes() len = %d, want >= 2", len(nodes))
	}
}

func TestUpdateNode(t *testing.T) {
	conn := newRBACMigratedConnection(t, "node-update.sqlite")

	setter := &models.NodeSetter{
		ID:        omit.From("node-update"),
		Name:      omit.From("Before Update"),
		ListenURL: omit.From("http://10.0.0.1:8080"),
		Enabled:   omit.From(true),
	}

	node, errInsert := conn.InsertNode(setter)
	if errInsert != nil {
		t.Fatalf("InsertNode() error = %v", errInsert)
	}

	updateSetter := &models.NodeSetter{
		Name:    omit.From("After Update"),
		Enabled: omit.From(false),
	}

	updated, errUpdate := conn.UpdateNode(node, updateSetter)
	if errUpdate != nil {
		t.Fatalf("UpdateNode() error = %v", errUpdate)
	}
	if updated.Name != "After Update" {
		t.Errorf("UpdateNode().Name = %q, want %q", updated.Name, "After Update")
	}
	if updated.Enabled {
		t.Errorf("UpdateNode().Enabled = true, want false")
	}
}

func TestDeleteNodeByID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "node-delete.sqlite")

	setter := &models.NodeSetter{
		ID:        omit.From("node-delete"),
		Name:      omit.From("Delete Me"),
		ListenURL: omit.From("http://10.0.0.3:8080"),
		Enabled:   omit.From(true),
	}

	_, errInsert := conn.InsertNode(setter)
	if errInsert != nil {
		t.Fatalf("InsertNode() error = %v", errInsert)
	}
	// An IP row left behind after the node's last game server was deleted
	// must not block removing the node.
	_, errInsertIP := conn.SQLDb.ExecContext(t.Context(),
		`insert into ip (address, usable, external, node_id) values (?, ?, ?, ?)`,
		"10.0.0.3", true, false, "node-delete")
	if errInsertIP != nil {
		t.Fatalf("insert ip error = %v", errInsertIP)
	}

	errDelete := conn.DeleteNodeByID("node-delete")
	if errDelete != nil {
		t.Fatalf("DeleteNodeByID() error = %v", errDelete)
	}

	_, errGet := conn.GetNodeByID("node-delete")
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetNodeByID() after delete error = %v, want %v", errGet, sql.ErrNoRows)
	}
	ips, errGetIPs := conn.GetIPsByNodeID("node-delete")
	if errGetIPs != nil {
		t.Fatalf("GetIPsByNodeID() error = %v", errGetIPs)
	}
	if len(ips) != 0 {
		t.Errorf("GetIPsByNodeID() after delete len = %d, want 0", len(ips))
	}
}

func TestDeleteNodeByIDRefusesWhileGameServersRemain(t *testing.T) {
	conn := newRBACMigratedConnection(t, "node-delete-in-use.sqlite")
	seedRBACFixture(t, conn)

	errDelete := conn.DeleteNodeByID("node-local")
	var inUse *NodeInUseError
	if !errors.As(errDelete, &inUse) {
		t.Fatalf("DeleteNodeByID() error = %v, want *NodeInUseError", errDelete)
	}
	if inUse.GameServerCount != 1 {
		t.Errorf("GameServerCount = %d, want 1", inUse.GameServerCount)
	}

	_, errGet := conn.GetNodeByID("node-local")
	if errGet != nil {
		t.Errorf("GetNodeByID() after refused delete error = %v", errGet)
	}
	ips, errGetIPs := conn.GetIPsByNodeID("node-local")
	if errGetIPs != nil {
		t.Fatalf("GetIPsByNodeID() error = %v", errGetIPs)
	}
	if len(ips) != 1 {
		t.Errorf("GetIPsByNodeID() after refused delete len = %d, want 1", len(ips))
	}
}

func TestInsertNodeDuplicateID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "node-dup.sqlite")

	setter := &models.NodeSetter{
		ID:        omit.From("node-dup"),
		Name:      omit.From("First"),
		ListenURL: omit.From("http://localhost:8080"),
		Enabled:   omit.From(true),
	}
	_, errFirst := conn.InsertNode(setter)
	if errFirst != nil {
		t.Fatalf("InsertNode(first) error = %v", errFirst)
	}

	setter2 := &models.NodeSetter{
		ID:        omit.From("node-dup"),
		Name:      omit.From("Second"),
		ListenURL: omit.From("http://localhost:9090"),
		Enabled:   omit.From(true),
	}
	_, errSecond := conn.InsertNode(setter2)
	if errSecond == nil {
		t.Fatalf("InsertNode(duplicate) expected error, got nil")
	}
}
