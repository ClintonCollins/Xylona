//go:build e2e_hub_spoke

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// TestHubSpokeBootstrap exercises the complete hub-spoke harness: it spins up
// a controller + remote xylona-node, drives the join-token bootstrap, and
// verifies the controller sees the remote node in its registry.
//
// Guarded by //go:build e2e_hub_spoke so it stays out of the default test
// sweep (it builds three binaries and listens on real ports).
func TestHubSpokeBootstrap(t *testing.T) {
	projectRoot, errAbs := filepath.Abs(filepath.Join("..", ".."))
	if errAbs != nil {
		t.Fatalf("resolve project root: %v", errAbs)
	}
	e2eDir := filepath.Join(projectRoot, "frontend", "e2e")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	const (
		httpPort      = 9391
		nodePort      = 9591
		adminUsername = "hub-spoke-admin"
		adminPassword = "HubSpokeAdmin!"
	)

	t.Cleanup(func() { runHubSpokeTeardown(e2eDir) })

	errSetup := runHubSpokeSetup(ctx, httpPort, nodePort, adminUsername, adminPassword, e2eDir, projectRoot)
	if errSetup != nil {
		t.Fatalf("hub-spoke setup: %v", errSetup)
	}

	backendURL := fmt.Sprintf("http://localhost:%d", httpPort)
	client, errLogin := newAuthenticatedClient(ctx, backendURL, adminUsername, adminPassword)
	if errLogin != nil {
		t.Fatalf("admin login: %v", errLogin)
	}

	listResp, errList := client.rpc.ListNodes(ctx, connect.NewRequest(&xylona.ListNodesRequest{}))
	if errList != nil {
		t.Fatalf("list nodes: %v", errList)
	}
	if len(listResp.Msg.GetNodes()) < 2 {
		t.Fatalf("expected >=2 nodes (self + remote), got %d", len(listResp.Msg.GetNodes()))
	}
}
