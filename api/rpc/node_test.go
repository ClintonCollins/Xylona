package rpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
)

func TestGetNode(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	// Get seeded local node
	request := connect.NewRequest(&xylona.GetNodeRequest{
		NodeId: "node-local",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	resp, errGet := fixture.service.GetNode(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetNode() error = %v", errGet)
	}
	if resp.Msg == nil || resp.Msg.GetNode() == nil {
		t.Fatalf("GetNode() returned empty response")
	}
	if resp.Msg.GetNode().GetId() != "node-local" {
		t.Errorf("GetNode().Node.Id = %q, want %q", resp.Msg.GetNode().GetId(), "node-local")
	}
	if resp.Msg.GetNode().GetName() != "Local Node" {
		t.Errorf("GetNode().Node.Name = %q, want %q", resp.Msg.GetNode().GetName(), "Local Node")
	}
	if !resp.Msg.GetNode().GetLocal() {
		t.Errorf("GetNode().Node.Local = false, want true")
	}
	if resp.Msg.GetNode().GetOs() != "linux" {
		t.Errorf("GetNode().Node.Os = %q, want %q", resp.Msg.GetNode().GetOs(), "linux")
	}

	// Nonexistent ID → error
	badRequest := connect.NewRequest(&xylona.GetNodeRequest{
		NodeId: "nonexistent-node",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, badRequest, "user-admin")

	_, errBad := fixture.service.GetNode(context.Background(), badRequest)
	if errBad == nil {
		t.Fatalf("GetNode(nonexistent) expected error, got nil")
	}
}

func TestListNodes(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.ListNodesRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	resp, errList := fixture.service.ListNodes(context.Background(), request)
	if errList != nil {
		t.Fatalf("ListNodes() error = %v", errList)
	}
	if resp.Msg == nil || len(resp.Msg.GetNodes()) == 0 {
		t.Fatalf("ListNodes() returned no nodes")
	}

	foundLocal := false
	for _, node := range resp.Msg.GetNodes() {
		if node.GetId() == "node-local" {
			foundLocal = true
			if node.GetOs() != "linux" {
				t.Errorf("ListNodes().Node.Os = %q, want %q", node.GetOs(), "linux")
			}
			break
		}
	}
	if !foundLocal {
		t.Errorf("ListNodes() did not include seeded local node")
	}
}

func TestEditNode(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.EditNodeRequest{
		Node: &xylona.Node{
			Id:   "node-local",
			Name: "Updated Local Node",
			Host: "localhost",
			Port: 8080,
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	resp, errEdit := fixture.service.EditNode(context.Background(), request)
	if errEdit != nil {
		t.Fatalf("EditNode() error = %v", errEdit)
	}
	if resp.Msg == nil || resp.Msg.GetNode() == nil {
		t.Fatalf("EditNode() returned empty response")
	}
	if resp.Msg.GetNode().GetName() != "Updated Local Node" {
		t.Errorf("EditNode().Node.Name = %q, want %q", resp.Msg.GetNode().GetName(), "Updated Local Node")
	}

	// Verify persistence
	getReq := connect.NewRequest(&xylona.GetNodeRequest{NodeId: "node-local"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getReq, "user-admin")
	getResp, errGet := fixture.service.GetNode(context.Background(), getReq)
	if errGet != nil {
		t.Fatalf("GetNode() after edit error = %v", errGet)
	}
	if getResp.Msg.GetNode().GetName() != "Updated Local Node" {
		t.Errorf("GetNode() after edit Name = %q, want %q", getResp.Msg.GetNode().GetName(), "Updated Local Node")
	}
}

func TestRemoveNode(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	// Seed a remote node to delete (don't delete the local one needed by other tests)
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-to-delete")

	// Verify it exists
	getReq := connect.NewRequest(&xylona.GetNodeRequest{NodeId: "node-to-delete"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getReq, "user-admin")
	_, errGet := fixture.service.GetNode(context.Background(), getReq)
	if errGet != nil {
		t.Fatalf("GetNode(node-to-delete) before remove error = %v", errGet)
	}

	// Remove it
	removeReq := connect.NewRequest(&xylona.RemoveNodeRequest{NodeId: "node-to-delete"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, removeReq, "user-admin")
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
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	// Create a secret key
	createReq := connect.NewRequest(&xylona.CreateLocalSecretKeyRequest{
		Name: "test-key",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-admin")

	createResp, errCreate := fixture.service.CreateLocalSecretKey(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateLocalSecretKey() error = %v", errCreate)
	}
	if createResp.Msg == nil {
		t.Fatalf("CreateLocalSecretKey() returned nil message")
	}
	if createResp.Msg.GetId() == 0 {
		t.Errorf("CreateLocalSecretKey().Id is zero")
	}
	if createResp.Msg.GetSecretKey() == "" {
		t.Errorf("CreateLocalSecretKey().SecretKey is empty")
	}

	// Delete the key
	deleteReq := connect.NewRequest(&xylona.DeleteLocalSecretKeyRequest{
		Id: createResp.Msg.GetId(),
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, deleteReq, "user-admin")

	_, errDelete := fixture.service.DeleteLocalSecretKey(context.Background(), deleteReq)
	if errDelete != nil {
		t.Fatalf("DeleteLocalSecretKey() error = %v", errDelete)
	}
}

func TestListLocalSecretKeys(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	// Create a key first
	createReq := connect.NewRequest(&xylona.CreateLocalSecretKeyRequest{
		Name: "list-test-key",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-admin")
	createResp, errCreate := fixture.service.CreateLocalSecretKey(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateLocalSecretKey() error = %v", errCreate)
	}

	// List keys
	listReq := connect.NewRequest(&xylona.ListLocalSecretKeysRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listReq, "user-admin")
	listResp, errList := fixture.service.ListLocalSecretKeys(context.Background(), listReq)
	if errList != nil {
		t.Fatalf("ListLocalSecretKeys() error = %v", errList)
	}
	if listResp.Msg == nil {
		t.Fatalf("ListLocalSecretKeys() returned nil message")
	}

	foundKey := false
	for _, key := range listResp.Msg.GetSecretKeys() {
		if key.GetId() == createResp.Msg.GetId() {
			foundKey = true
			if key.GetName() != "list-test-key" {
				t.Errorf("SecretKey.Name = %q, want %q", key.GetName(), "list-test-key")
			}
			break
		}
	}
	if !foundKey {
		t.Errorf("ListLocalSecretKeys() did not include created key %d", createResp.Msg.GetId())
	}
}

func TestVerifyNodeAcceptsCreatedLocalSecretKey(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	createReq := connect.NewRequest(&xylona.CreateLocalSecretKeyRequest{
		Name: "verify-test-key",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-admin")

	createResp, errCreate := fixture.service.CreateLocalSecretKey(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateLocalSecretKey() error = %v", errCreate)
	}

	verifyReq := connect.NewRequest(&xylona.VerifyNodeRequest{
		SecretKey: createResp.Msg.GetSecretKey(),
	})

	verifyResp, errVerify := fixture.service.VerifyNode(context.Background(), verifyReq)
	if errVerify != nil {
		t.Fatalf("VerifyNode() error = %v", errVerify)
	}
	if verifyResp.Msg == nil || verifyResp.Msg.GetNode() == nil {
		t.Fatalf("VerifyNode() returned empty response")
	}
	if verifyResp.Msg.GetNode().GetId() == "" {
		t.Fatalf("VerifyNode().Node.Id is empty")
	}
	if !verifyResp.Msg.GetNode().GetLocal() {
		t.Errorf("VerifyNode().Node.Local = false, want true")
	}

	storedKey, errGet := fixture.conn.GetSecretKeyByID(createResp.Msg.GetId())
	if errGet != nil {
		t.Fatalf("GetSecretKeyByID() error = %v", errGet)
	}
	if storedKey.LastUsedAt.GetOrZero().IsZero() {
		t.Errorf("VerifyNode() did not persist LastUsedAt")
	}
}

func TestVerifyNodeRejectsWrongSecret(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	createReq := connect.NewRequest(&xylona.CreateLocalSecretKeyRequest{
		Name: "verify-wrong-secret-key",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-admin")

	_, errCreate := fixture.service.CreateLocalSecretKey(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateLocalSecretKey() error = %v", errCreate)
	}

	verifyReq := connect.NewRequest(&xylona.VerifyNodeRequest{
		SecretKey: "definitely-not-the-secret",
	})

	_, errVerify := fixture.service.VerifyNode(context.Background(), verifyReq)
	if errVerify == nil {
		t.Fatalf("VerifyNode() error = nil, want permission denied")
	}
	if connect.CodeOf(errVerify) != connect.CodePermissionDenied {
		t.Fatalf("VerifyNode() code = %v, want %v", connect.CodeOf(errVerify), connect.CodePermissionDenied)
	}
}

func newVerifyNodeRPCServer(t *testing.T, service *XylonaService) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	path, handler := xylonaconnect.NewXylonaHandler(service)
	mux.Handle(path, handler)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func TestVerifyNodeAcceptsExistingLocalSecretKeyAndPersistsUsageMetadata(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	createReq := connect.NewRequest(&xylona.CreateLocalSecretKeyRequest{
		Name: "verify-node-key",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-admin")

	createResp, errCreate := fixture.service.CreateLocalSecretKey(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateLocalSecretKey() error = %v", errCreate)
	}
	if createResp.Msg == nil {
		t.Fatal("CreateLocalSecretKey() returned nil message")
	}

	beforeSecretKey, errGetBefore := fixture.conn.GetSecretKeyByID(createResp.Msg.GetId())
	if errGetBefore != nil {
		t.Fatalf("GetSecretKeyByID(before VerifyNode) error = %v", errGetBefore)
	}

	server := newVerifyNodeRPCServer(t, fixture.service)
	client := xylonaconnect.NewXylonaClient(server.Client(), server.URL)

	verifyReq := connect.NewRequest(&xylona.VerifyNodeRequest{
		SecretKey: createResp.Msg.GetSecretKey(),
	})

	response, errVerify := client.VerifyNode(context.Background(), verifyReq)
	if errVerify != nil {
		t.Fatalf("VerifyNode() error = %v", errVerify)
	}
	if response == nil || response.Msg == nil || response.Msg.GetNode() == nil {
		t.Fatal("VerifyNode() returned empty response")
	}
	if response.Msg.GetNode().GetId() == "" {
		t.Fatal("VerifyNode().Node.Id = empty, want a node identifier")
	}
	if !response.Msg.GetNode().GetLocal() {
		t.Fatal("VerifyNode().Node.Local = false, want true")
	}

	afterSecretKey, errGetAfter := fixture.conn.GetSecretKeyByID(createResp.Msg.GetId())
	if errGetAfter != nil {
		t.Fatalf("GetSecretKeyByID(after VerifyNode) error = %v", errGetAfter)
	}

	beforeLastUsedAt := beforeSecretKey.LastUsedAt.GetOr(time.Time{})
	afterLastUsedAt := afterSecretKey.LastUsedAt.GetOr(time.Time{})
	if !afterLastUsedAt.After(beforeLastUsedAt) {
		t.Fatalf("VerifyNode() did not persist a newer last_used_at: before=%v after=%v", beforeLastUsedAt, afterLastUsedAt)
	}

	if got := afterSecretKey.LastAccessedFrom.GetOr(""); got == "" {
		t.Fatal("VerifyNode() did not persist last_accessed_from")
	}
}

func TestVerifyNodeRejectsIncorrectSecretKey(t *testing.T) {
	t.Parallel()

	fixture := newRBACRPCFixture(t)

	createReq := connect.NewRequest(&xylona.CreateLocalSecretKeyRequest{
		Name: "verify-node-reject-key",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, createReq, "user-admin")

	createResp, errCreate := fixture.service.CreateLocalSecretKey(context.Background(), createReq)
	if errCreate != nil {
		t.Fatalf("CreateLocalSecretKey() error = %v", errCreate)
	}
	if createResp.Msg == nil {
		t.Fatal("CreateLocalSecretKey() returned nil message")
	}

	server := newVerifyNodeRPCServer(t, fixture.service)
	client := xylonaconnect.NewXylonaClient(server.Client(), server.URL)

	verifyReq := connect.NewRequest(&xylona.VerifyNodeRequest{
		SecretKey: createResp.Msg.GetSecretKey() + "-wrong",
	})

	_, errVerify := client.VerifyNode(context.Background(), verifyReq)
	if errVerify == nil {
		t.Fatal("VerifyNode() with incorrect secret key error = nil, want permission denied")
	}
	if connect.CodeOf(errVerify) != connect.CodePermissionDenied {
		t.Fatalf("VerifyNode() with incorrect secret key code = %v, want %v", connect.CodeOf(errVerify), connect.CodePermissionDenied)
	}
}
