package rpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/noderegistry"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type blockingSnapshotNodeClient struct {
	*nodeclient.FakeNodeClient
	entered chan<- string
	release <-chan struct{}
}

func (c *blockingSnapshotNodeClient) GetNodeSnapshot(ctx context.Context) (*node.NodeSnapshot, error) {
	c.entered <- c.NodeID

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("blocking snapshot wait: %w", ctx.Err())
	case <-c.release:
	}

	return c.SnapshotResult, c.SnapshotErr
}

func insertRemoteNodeForParityTests(t *testing.T, fixture *rbacRPCFixture, nodeID string) {
	t.Helper()

	_, errInsert := fixture.conn.InsertNode(&models.NodeSetter{
		ID:         omit.From(nodeID),
		Name:       omit.From("Remote Node"),
		ListenURL:  omit.From("https://" + nodeID + ".example.com"),
		Enabled:    omit.From(true),
		LastSeenAt: omitnull.From(time.Now().UTC()),
	})
	if errInsert != nil {
		t.Fatalf("InsertNode() error = %v", errInsert)
	}
}

func insertRemoteServerForParityTests(t *testing.T, fixture *rbacRPCFixture, serverID string) {
	t.Helper()

	_, errInsert := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		serverID, "user-owner", "Remote Server", "minecraft", "OFFLINE",
		12, 20, "world_remote", "127.0.0.1", 25575, 25576, "/srv/remote-server", "node-remote", "[]",
	)
	if errInsert != nil {
		t.Fatalf("insert remote game server error = %v", errInsert)
	}
}

func testParityRegistry(self nodeclient.NodeClient, remote nodeclient.NodeClient) *noderegistry.Registry {
	registry := noderegistry.New("node-local", self)
	if remote != nil {
		registry.Register(remote)
	}
	return registry
}

func TestRemoveNodeRejectsEmbeddedSelfNode(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, nil)

	request := connect.NewRequest(&xylona.RemoveNodeRequest{NodeId: "node-local"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errRemove := fixture.service.RemoveNode(context.Background(), request)
	if errRemove == nil {
		t.Fatal("RemoveNode() error = nil, want failed precondition")
	}
	if connect.CodeOf(errRemove) != connect.CodeFailedPrecondition {
		t.Fatalf("RemoveNode() code = %v, want %v", connect.CodeOf(errRemove), connect.CodeFailedPrecondition)
	}

	_, errGet := fixture.conn.GetNodeByID("node-local")
	if errGet != nil {
		t.Fatalf("GetNodeByID(node-local) after rejected remove error = %v", errGet)
	}
}

func TestListAggregatedGameServersIncludesRemoteRows(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")

	selfClient := &nodeclient.FakeNodeClient{
		NodeID: "node-local",
	}
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                  "node-remote",
		GetProcessSnapshotFound: true,
		GetProcessSnapshotResult: &node.ProcessSnapshot{
			ID:     "server-remote-1",
			Name:   "Remote Server",
			Status: xylona.Status_ONLINE.String(),
		},
	}
	fixture.service.nodeRegistry = testParityRegistry(selfClient, remoteClient)

	request := connect.NewRequest(&xylona.ListAggregatedGameServersRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	response, errList := fixture.service.ListAggregatedGameServers(context.Background(), request)
	if errList != nil {
		t.Fatalf("ListAggregatedGameServers() error = %v", errList)
	}

	var remoteSummary *xylona.RemoteServerSummary
	for _, server := range response.Msg.GetServers() {
		if !server.GetIsLocal() && server.GetRemoteServer().GetRemoteServerId() == "server-remote-1" {
			remoteSummary = server.GetRemoteServer()
			break
		}
	}
	if remoteSummary == nil {
		t.Fatal("ListAggregatedGameServers() missing remote server summary for server-remote-1")
	}
	if remoteSummary.GetStatus() != xylona.Status_ONLINE {
		t.Fatalf("remote summary status = %v, want %v", remoteSummary.GetStatus(), xylona.Status_ONLINE)
	}
	if remoteSummary.GetNodeId() != "node-remote" {
		t.Fatalf("remote summary node_id = %q, want %q", remoteSummary.GetNodeId(), "node-remote")
	}
	if remoteSummary.GetNodeName() != "Remote Node" {
		t.Fatalf("remote summary node_name = %q, want %q", remoteSummary.GetNodeName(), "Remote Node")
	}
}

func TestReadGameServerOutputReadsRemoteNodeBuffer(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		ReadConsoleBufferResult: node.ConsoleChunk{
			ProcessID: "server-remote-1",
			Data:      "remote-console-line",
		},
	}
	registry := testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)
	fixture.service.nodeRegistry = registry
	fixture.service.actionsInst = actions.NewInstance(
		context.Background(),
		fixture.conn,
		nil,
		registry,
		nil,
		nil,
		versiontracker.ResolverConfig{},
	)

	request := connect.NewRequest(&xylona.ReadGameServerOutputRequest{ServerId: "server-remote-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errRead := fixture.service.ReadGameServerOutput(context.Background(), request)
	if errRead != nil {
		t.Fatalf("ReadGameServerOutput() error = %v", errRead)
	}
	if response.Msg.GetOutput() != "remote-console-line" {
		t.Fatalf("ReadGameServerOutput().Output = %q, want %q", response.Msg.GetOutput(), "remote-console-line")
	}
	if len(remoteClient.ReadConsoleBufferCalls) != 1 || remoteClient.ReadConsoleBufferCalls[0] != "server-remote-1" {
		t.Fatalf("remote ReadConsoleBuffer calls = %+v, want [server-remote-1]", remoteClient.ReadConsoleBufferCalls)
	}
}

func TestListNodesIncludesLocalHealthAndOSMetadata(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")

	selfClient := &nodeclient.FakeNodeClient{
		NodeID: "node-local",
		SnapshotResult: &node.NodeSnapshot{
			OS: "linux",
		},
	}
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		SnapshotResult: &node.NodeSnapshot{
			OS: "windows",
		},
	}
	fixture.service.nodeRegistry = testParityRegistry(selfClient, remoteClient)

	request := connect.NewRequest(&xylona.ListNodesRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	response, errList := fixture.service.ListNodes(context.Background(), request)
	if errList != nil {
		t.Fatalf("ListNodes() error = %v", errList)
	}

	nodesByID := make(map[string]*xylona.Node, len(response.Msg.GetNodes()))
	for _, entry := range response.Msg.GetNodes() {
		nodesByID[entry.GetId()] = entry
	}

	localNode := nodesByID["node-local"]
	if localNode == nil {
		t.Fatal("ListNodes() missing local node")
	}
	if !localNode.GetLocal() {
		t.Fatal("ListNodes().node-local.Local = false, want true")
	}
	if localNode.GetOs() != "linux" {
		t.Fatalf("ListNodes().node-local.Os = %q, want %q", localNode.GetOs(), "linux")
	}
	if localNode.GetHealthStatus() != "healthy" {
		t.Fatalf("ListNodes().node-local.HealthStatus = %q, want %q", localNode.GetHealthStatus(), "healthy")
	}

	remoteNode := nodesByID["node-remote"]
	if remoteNode == nil {
		t.Fatal("ListNodes() missing remote node")
	}
	if remoteNode.GetLocal() {
		t.Fatal("ListNodes().node-remote.Local = true, want false")
	}
	if remoteNode.GetOs() != "windows" {
		t.Fatalf("ListNodes().node-remote.Os = %q, want %q", remoteNode.GetOs(), "windows")
	}
	if remoteNode.GetHealthStatus() != "healthy" {
		t.Fatalf("ListNodes().node-remote.HealthStatus = %q, want %q", remoteNode.GetHealthStatus(), "healthy")
	}
}

func TestListNodesFetchesRemoteRuntimeMetadataConcurrently(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote-a")
	insertRemoteNodeForParityTests(t, fixture, "node-remote-b")

	entered := make(chan string, 2)
	release := make(chan struct{})

	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{
		NodeID: "node-local",
		SnapshotResult: &node.NodeSnapshot{
			OS: "linux",
		},
	})
	registry.Register(&blockingSnapshotNodeClient{
		FakeNodeClient: &nodeclient.FakeNodeClient{
			NodeID: "node-remote-a",
			SnapshotResult: &node.NodeSnapshot{
				OS: "windows",
			},
		},
		entered: entered,
		release: release,
	})
	registry.Register(&blockingSnapshotNodeClient{
		FakeNodeClient: &nodeclient.FakeNodeClient{
			NodeID: "node-remote-b",
			SnapshotResult: &node.NodeSnapshot{
				OS: "linux",
			},
		},
		entered: entered,
		release: release,
	})
	fixture.service.nodeRegistry = registry

	request := connect.NewRequest(&xylona.ListNodesRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	type listNodesResult struct {
		err error
	}
	resultCh := make(chan listNodesResult, 1)
	go func() {
		_, errList := fixture.service.ListNodes(context.Background(), request)
		resultCh <- listNodesResult{err: errList}
	}()

	for _, wantNodeID := range []string{"node-remote-a", "node-remote-b"} {
		select {
		case gotNodeID := <-entered:
			if gotNodeID != wantNodeID && gotNodeID != "node-remote-a" && gotNodeID != "node-remote-b" {
				t.Fatalf("unexpected blocking snapshot client %q entered", gotNodeID)
			}
		case <-time.After(250 * time.Millisecond):
			t.Fatalf("timed out waiting for concurrent node snapshot fetch %q", wantNodeID)
		}
	}

	close(release)

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("ListNodes() error = %v", result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ListNodes() to complete")
	}
}

func TestGetDashboardOverviewReusesNodeSnapshotsForNodeMetadata(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")

	selfClient := &nodeclient.FakeNodeClient{
		NodeID: "node-local",
		SnapshotResult: &node.NodeSnapshot{
			OS:            "linux",
			CPUModel:      "Ryzen",
			CPUCores:      8,
			CPUThreads:    16,
			TotalMemory:   1024,
			XylonaVersion: "1.0.0",
		},
	}
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		SnapshotResult: &node.NodeSnapshot{
			OS:            "windows",
			CPUModel:      "Xeon",
			CPUCores:      4,
			CPUThreads:    8,
			TotalMemory:   2048,
			XylonaVersion: "2.0.0",
		},
	}
	fixture.service.nodeRegistry = testParityRegistry(selfClient, remoteClient)

	request := connect.NewRequest(&xylona.GetDashboardOverviewRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errOverview := fixture.service.GetDashboardOverview(context.Background(), request)
	if errOverview != nil {
		t.Fatalf("GetDashboardOverview() error = %v", errOverview)
	}
	if selfClient.SnapshotCalls != 1 {
		t.Fatalf("self snapshot call count = %d, want 1", selfClient.SnapshotCalls)
	}
	if remoteClient.SnapshotCalls != 1 {
		t.Fatalf("remote snapshot call count = %d, want 1", remoteClient.SnapshotCalls)
	}
}
