package actions

import (
	"testing"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestBuildLocalPeerList_Empty(t *testing.T) {
	inst := newTestInstance(t)
	peers, errBuild := inst.BuildLocalPeerList()
	if errBuild != nil {
		t.Fatalf("BuildLocalPeerList() error = %v", errBuild)
	}
	if len(peers) != 0 {
		t.Errorf("len(peers) = %d, want 0", len(peers))
	}
}

func TestProcessReceivedPeerList_SkipsKnownNodes(t *testing.T) {
	inst := newTestInstance(t)
	// Insert a known remote node.
	_, errInsert := inst.db.InsertRemoteNode(&models.NodeSetter{
		ID:      omit.From("known-node"),
		Name:    omit.From("Known"),
		IsLocal: omit.From(false),
		Host:    omit.From("known.example.com"),
		Port:    omit.From(int64(9444)),
		BaseURL: omit.From("https://known.example.com"),
		Enabled: omit.From(true),
	})
	if errInsert != nil {
		t.Fatalf("InsertRemoteNode() error = %v", errInsert)
	}

	// Process a peer list containing the known node — should not create duplicates.
	inst.ProcessReceivedPeerList([]*xylona.PeerInfo{
		{
			NodeId:          "known-node",
			Name:            "Known",
			BaseUrl:         "https://known.example.com",
			CertFingerprint: "abc123",
			FederationPort:  9444,
		},
	}, "introducer-id")

	// Verify no advisory was created (no auto-pairing happened).
	count, errCount := inst.db.GetUnreadAdvisoryCount()
	if errCount != nil {
		t.Fatalf("GetUnreadAdvisoryCount() error = %v", errCount)
	}
	if count != 0 {
		t.Errorf("unread count = %d, want 0 (known node should be skipped)", count)
	}
}

func TestHandleNodeDeparture(t *testing.T) {
	inst := newTestInstance(t)

	// Insert a remote node that will depart.
	_, errInsert := inst.db.InsertRemoteNode(&models.NodeSetter{
		ID:      omit.From("departing-node"),
		Name:    omit.From("Departing"),
		IsLocal: omit.From(false),
		Host:    omit.From("departing.example.com"),
		Port:    omit.From(int64(9444)),
		BaseURL: omit.From("https://departing.example.com"),
		Enabled: omit.From(true),
	})
	if errInsert != nil {
		t.Fatalf("InsertRemoteNode() error = %v", errInsert)
	}

	inst.HandleNodeDeparture("departing-node", "shutting down")

	// Verify node was deleted.
	_, errGet := inst.db.GetRemoteNodeByID("departing-node")
	if errGet == nil {
		t.Error("departing node should have been deleted")
	}

	// Verify advisory was created.
	count, errCount := inst.db.GetUnreadAdvisoryCount()
	if errCount != nil {
		t.Fatalf("GetUnreadAdvisoryCount() error = %v", errCount)
	}
	if count != 1 {
		t.Errorf("unread count = %d, want 1", count)
	}
}
