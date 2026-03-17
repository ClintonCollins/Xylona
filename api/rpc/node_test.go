package rpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestGetNode(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// Get seeded local node
	request := connect.NewRequest(&xylona.GetNodeRequest{
		NodeId: "node-local",
	})

	resp, errGet := fixture.service.GetNode(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetNode() error = %v", errGet)
	}
	if resp.Msg == nil || resp.Msg.Node == nil {
		t.Fatalf("GetNode() returned empty response")
	}
	if resp.Msg.Node.Id != "node-local" {
		t.Errorf("GetNode().Node.Id = %q, want %q", resp.Msg.Node.Id, "node-local")
	}
	if resp.Msg.Node.Name != "Local Node" {
		t.Errorf("GetNode().Node.Name = %q, want %q", resp.Msg.Node.Name, "Local Node")
	}
	if !resp.Msg.Node.Local {
		t.Errorf("GetNode().Node.Local = false, want true")
	}

	// Nonexistent ID → error
	badRequest := connect.NewRequest(&xylona.GetNodeRequest{
		NodeId: "nonexistent-node",
	})

	_, errBad := fixture.service.GetNode(context.Background(), badRequest)
	if errBad == nil {
		t.Fatalf("GetNode(nonexistent) expected error, got nil")
	}
}

func TestListNodes(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.ListNodesRequest{})

	resp, errList := fixture.service.ListNodes(context.Background(), request)
	if errList != nil {
		t.Fatalf("ListNodes() error = %v", errList)
	}
	if resp.Msg == nil || len(resp.Msg.Nodes) == 0 {
		t.Fatalf("ListNodes() returned no nodes")
	}

	foundLocal := false
	for _, node := range resp.Msg.Nodes {
		if node.Id == "node-local" {
			foundLocal = true
			break
		}
	}
	if !foundLocal {
		t.Errorf("ListNodes() did not include seeded local node")
	}
}

func TestEditNode(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.EditNodeRequest{
		Node: &xylona.Node{
			Id:   "node-local",
			Name: "Updated Local Node",
			Host: "localhost",
			Port: 8080,
		},
	})

	resp, errEdit := fixture.service.EditNode(context.Background(), request)
	if errEdit != nil {
		t.Fatalf("EditNode() error = %v", errEdit)
	}
	if resp.Msg == nil || resp.Msg.Node == nil {
		t.Fatalf("EditNode() returned empty response")
	}
	if resp.Msg.Node.Name != "Updated Local Node" {
		t.Errorf("EditNode().Node.Name = %q, want %q", resp.Msg.Node.Name, "Updated Local Node")
	}

	// Verify persistence
	getReq := connect.NewRequest(&xylona.GetNodeRequest{NodeId: "node-local"})
	getResp, errGet := fixture.service.GetNode(context.Background(), getReq)
	if errGet != nil {
		t.Fatalf("GetNode() after edit error = %v", errGet)
	}
	if getResp.Msg.Node.Name != "Updated Local Node" {
		t.Errorf("GetNode() after edit Name = %q, want %q", getResp.Msg.Node.Name, "Updated Local Node")
	}
}

func TestRemoveNode(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// Seed a remote node to delete (don't delete the local one needed by other tests)
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-to-delete")

	// Verify it exists
	getReq := connect.NewRequest(&xylona.GetNodeRequest{NodeId: "node-to-delete"})
	_, errGet := fixture.service.GetNode(context.Background(), getReq)
	if errGet != nil {
		t.Fatalf("GetNode(node-to-delete) before remove error = %v", errGet)
	}

	// Remove it
	removeReq := connect.NewRequest(&xylona.RemoveNodeRequest{NodeId: "node-to-delete"})
	_, errRemove := fixture.service.RemoveNode(context.Background(), removeReq)
	if errRemove != nil {
		t.Fatalf("RemoveNode() error = %v", errRemove)
	}

	// Verify it's gone
	_, errGetAfter := fixture.service.GetNode(context.Background(), getReq)
	if errGetAfter == nil {
		t.Errorf("GetNode() after remove expected error, got nil")
	}
}

func TestCreateAndDeleteLocalSecretKey(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// Create a secret key
	createReq := connect.NewRequest(&xylona.CreateLocalSecretKeyRequest{
		Name: "test-key",
	})

	createResp, errCreate := fixture.service.CreateLocalSecretKey(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateLocalSecretKey() error = %v", errCreate)
	}
	if createResp.Msg == nil {
		t.Fatalf("CreateLocalSecretKey() returned nil message")
	}
	if createResp.Msg.Id == 0 {
		t.Errorf("CreateLocalSecretKey().Id is zero")
	}
	if createResp.Msg.SecretKey == "" {
		t.Errorf("CreateLocalSecretKey().SecretKey is empty")
	}

	// Delete the key
	deleteReq := connect.NewRequest(&xylona.DeleteLocalSecretKeyRequest{
		Id: createResp.Msg.Id,
	})

	_, errDelete := fixture.service.DeleteLocalSecretKey(context.Background(), deleteReq)
	if errDelete != nil {
		t.Fatalf("DeleteLocalSecretKey() error = %v", errDelete)
	}
}

func TestListLocalSecretKeys(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	// Create a key first
	createReq := connect.NewRequest(&xylona.CreateLocalSecretKeyRequest{
		Name: "list-test-key",
	})
	createResp, errCreate := fixture.service.CreateLocalSecretKey(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateLocalSecretKey() error = %v", errCreate)
	}

	// List keys
	listReq := connect.NewRequest(&xylona.ListLocalSecretKeysRequest{})
	listResp, errList := fixture.service.ListLocalSecretKeys(context.Background(), listReq)
	if errList != nil {
		t.Fatalf("ListLocalSecretKeys() error = %v", errList)
	}
	if listResp.Msg == nil {
		t.Fatalf("ListLocalSecretKeys() returned nil message")
	}

	foundKey := false
	for _, key := range listResp.Msg.SecretKeys {
		if key.Id == createResp.Msg.Id {
			foundKey = true
			if key.Name != "list-test-key" {
				t.Errorf("SecretKey.Name = %q, want %q", key.Name, "list-test-key")
			}
			break
		}
	}
	if !foundKey {
		t.Errorf("ListLocalSecretKeys() did not include created key %d", createResp.Msg.Id)
	}
}
