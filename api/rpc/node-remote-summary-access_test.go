package rpc

import (
	"context"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestListRemoteNodeSummariesDoesNotUseStaleCacheForNonSuperUser(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	staleSummary := &xylona.RemoteServerSummary{
		Id:             "node-remote/server-1",
		SourceNodeId:   "node-remote",
		NodeId:         "node-remote",
		RemoteServerId: "server-1",
		DisplayName:    "Remote Server",
		Status:         xylona.Status_ONLINE,
	}
	fixture.service.listCache.set("node-remote", []*xylona.RemoteServerSummary{staleSummary}, time.Now().Add(-2*time.Minute))

	user, errUser := fixture.conn.GetUserByID("user-owner")
	if errUser != nil {
		t.Fatalf("failed to load non-super test user: %v", errUser)
	}

	node := &models.Node{
		ID:      "node-remote",
		Name:    "Remote",
		BaseURL: "https://remote.example.com",
	}

	summaries, usedStaleData, errSummaries := fixture.service.listRemoteNodeSummaries(context.Background(), node, user)
	if errSummaries == nil {
		t.Fatalf("listRemoteNodeSummaries() error = nil, want error")
	}
	if usedStaleData {
		t.Fatalf("listRemoteNodeSummaries() usedStaleData = true, want false for non-super user")
	}
	if summaries != nil {
		t.Fatalf("listRemoteNodeSummaries() summaries = %v, want nil", summaries)
	}
}

func TestListRemoteNodeSummariesUsesStaleCacheForSuperUser(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	staleSummary := &xylona.RemoteServerSummary{
		Id:             "node-remote/server-1",
		SourceNodeId:   "node-remote",
		NodeId:         "node-remote",
		RemoteServerId: "server-1",
		DisplayName:    "Remote Server",
		Status:         xylona.Status_ONLINE,
	}
	fixture.service.listCache.set("node-remote", []*xylona.RemoteServerSummary{staleSummary}, time.Now().Add(-2*time.Minute))

	user, errUser := fixture.conn.GetUserByID("user-admin")
	if errUser != nil {
		t.Fatalf("failed to load super-user test user: %v", errUser)
	}

	node := &models.Node{
		ID:      "node-remote",
		Name:    "Remote",
		BaseURL: "https://remote.example.com",
	}

	summaries, usedStaleData, errSummaries := fixture.service.listRemoteNodeSummaries(context.Background(), node, user)
	if errSummaries != nil {
		t.Fatalf("listRemoteNodeSummaries() error = %v", errSummaries)
	}
	if !usedStaleData {
		t.Fatalf("listRemoteNodeSummaries() usedStaleData = false, want true for super user fallback")
	}
	if len(summaries) != 1 {
		t.Fatalf("len(summaries) = %d, want 1", len(summaries))
	}
	if summaries[0].Status != xylona.Status_OFFLINE {
		t.Fatalf("summaries[0].Status = %v, want %v for stale fallback", summaries[0].Status, xylona.Status_OFFLINE)
	}
}
