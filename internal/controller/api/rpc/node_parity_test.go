package rpc

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type blockingSnapshotNodeClient struct {
	*nodeclient.FakeNodeClient
	entered chan<- string
	release <-chan struct{}
}

type cancellationAwareConsoleNodeClient struct {
	*nodeclient.FakeNodeClient
}

type blockingProcessSnapshotNodeClient struct {
	*nodeclient.FakeNodeClient
	entered chan<- string
}

func (c *blockingProcessSnapshotNodeClient) GetProcessSnapshot(
	ctx context.Context,
	processID string,
) (*node.ProcessSnapshot, bool, error) {
	c.entered <- processID
	<-ctx.Done()
	return nil, false, fmt.Errorf("blocking process snapshot wait: %w", ctx.Err())
}

func (c *cancellationAwareConsoleNodeClient) ReadConsoleBuffer(ctx context.Context, processID string) (node.ConsoleChunk, error) {
	<-ctx.Done()
	return node.ConsoleChunk{ProcessID: processID}, fmt.Errorf("read console buffer wait: %w", ctx.Err())
}

func (c *cancellationAwareConsoleNodeClient) SendConsoleInput(ctx context.Context, _ string, _ string) error {
	<-ctx.Done()
	return fmt.Errorf("send console input wait: %w", ctx.Err())
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

type blockingProbeInstalledVersionNodeClient struct {
	*nodeclient.FakeNodeClient
	entered chan<- string
	release <-chan struct{}
	calls   atomic.Int32
}

func (c *blockingProbeInstalledVersionNodeClient) ProbeInstalledVersion(ctx context.Context, req node.InstalledVersionProbeRequest) (node.InstalledVersionProbeResult, error) {
	c.calls.Add(1)
	c.entered <- fmt.Sprint(req.Kind)

	select {
	case <-ctx.Done():
		return node.InstalledVersionProbeResult{}, fmt.Errorf("blocking probe wait: %w", ctx.Err())
	case <-c.release:
	}

	if c.ProbeInstalledVersionErr != nil {
		return node.InstalledVersionProbeResult{}, c.ProbeInstalledVersionErr
	}
	return c.ProbeInstalledVersionResult, nil
}

func (c *blockingProbeInstalledVersionNodeClient) probeCallCount() int {
	return int(c.calls.Load())
}

type rpcVersionTestTracker struct {
	mu          sync.Mutex
	installed   string
	latest      string
	installHits int
	latestHits  int
}

func (t *rpcVersionTestTracker) GetInstalledVersion(_ context.Context, _ *models.GameServer) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.installHits++
	return t.installed, nil
}

func (t *rpcVersionTestTracker) GetLatestVersion(_ context.Context, _ *models.GameServer) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.latestHits++
	return t.latest, nil
}

func (t *rpcVersionTestTracker) CheckForUpdate(ctx context.Context, gs *models.GameServer) (*versiontracker.UpdateInfo, error) {
	installed, errInstalled := t.GetInstalledVersion(ctx, gs)
	if errInstalled != nil {
		return nil, errInstalled
	}

	latest, errLatest := t.GetLatestVersion(ctx, gs)
	if errLatest != nil {
		return nil, errLatest
	}

	return &versiontracker.UpdateInfo{
		InstalledVersion: installed,
		LatestVersion:    latest,
		UpdateAvailable:  installed != latest,
	}, nil
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

	insertNodeScopedIPForParityTests(t, fixture, nodeID, "127.0.0.1")
}

func insertNodeScopedIPForParityTests(t *testing.T, fixture *rbacRPCFixture, nodeID string, address string) {
	t.Helper()

	_, errInsert := fixture.conn.InsertIP(&models.IPSetter{
		Address:            omit.From(address),
		Usable:             omit.From(true),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(false),
		NodeID:             omit.From(nodeID),
	})
	if errInsert != nil {
		t.Fatalf("InsertIP() error = %v", errInsert)
	}
}

func insertRemoteServerForParityTests(t *testing.T, fixture *rbacRPCFixture, serverID string) {
	t.Helper()
	insertServerOnNodeWithDetailsForParityTests(
		t,
		fixture,
		serverID,
		"Remote Server",
		"node-remote",
		"/srv/remote-server",
		25575,
	)
}

func insertServerOnNodeForParityTests(
	t *testing.T,
	fixture *rbacRPCFixture,
	serverID string,
	nodeID string,
	port int64,
) {
	t.Helper()
	insertServerOnNodeWithDetailsForParityTests(
		t,
		fixture,
		serverID,
		"Remote Server "+serverID,
		nodeID,
		"/srv/"+serverID,
		port,
	)
}

func insertServerOnNodeWithDetailsForParityTests(
	t *testing.T,
	fixture *rbacRPCFixture,
	serverID string,
	serverName string,
	nodeID string,
	directory string,
	port int64,
) {
	t.Helper()

	_, errInsert := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		serverID, "user-owner", serverName, "minecraft", "OFFLINE",
		12, 20, "world_remote", "127.0.0.1", port, port+1, directory, nodeID, "[]",
	)
	if errInsert != nil {
		t.Fatalf("insert remote game server error = %v", errInsert)
	}
}

func configureRemoteVersionTrackingForTests(
	t *testing.T,
	fixture *rbacRPCFixture,
	registry *noderegistry.Registry,
	tracker versiontracker.VersionTracker,
) {
	t.Helper()

	fixture.service.versionState = versiontracker.NewVersionStateMap()
	fixture.service.nodeRegistry = registry
	fixture.service.actionsInst = actions.NewInstance(
		context.Background(),
		fixture.conn,
		nil,
		registry,
		nil,
		fixture.service.versionState,
		versiontracker.ResolverConfig{
			CustomTrackerFactory: func(info versiontracker.TrackerContext) versiontracker.VersionTracker {
				if info.GameID == "minecraft" {
					return tracker
				}
				return nil
			},
		},
	)

	_, errUpdate := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`update game_server set server_executable = ? where id = ?`,
		"server.jar",
		"server-remote-1",
	)
	if errUpdate != nil {
		t.Fatalf("update remote server executable: %v", errUpdate)
	}
}

func waitForVersionValues(t *testing.T, states *versiontracker.VersionStateMap, serverID string, wantInstalled string, wantLatest string) versiontracker.VersionState {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state, ok := states.GetWithOK(serverID)
		if ok && state.Status == versiontracker.VersionStatusChecked &&
			state.InstalledVersion == wantInstalled && state.LatestVersion == wantLatest {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}

	state := states.Get(serverID)
	t.Fatalf(
		"version state for %s = %+v, want installed=%q latest=%q",
		serverID,
		state,
		wantInstalled,
		wantLatest,
	)
	return versiontracker.VersionState{}
}

func remoteVersionContextKeyForParityTests(t *testing.T, fixture *rbacRPCFixture, serverID string) string {
	t.Helper()

	gameServer, errGameServer := fixture.conn.GetGameServerByID(serverID)
	if errGameServer != nil {
		t.Fatalf("GetGameServerByID(%q) error = %v", serverID, errGameServer)
	}
	return fixture.service.remoteVersionTrackerContext(gameServer).CacheKey()
}

func testParityRegistry(self nodeclient.NodeClient, remote nodeclient.NodeClient) *noderegistry.Registry {
	registry := noderegistry.New("node-local", self)
	if remote != nil {
		registry.Register(remote)
	}
	return registry
}

func configureLifecycleActionsForParityTests(
	t *testing.T,
	fixture *rbacRPCFixture,
	registry *noderegistry.Registry,
) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	fixture.service.nodeRegistry = registry
	fixture.service.actionsInst = actions.NewInstance(
		ctx,
		fixture.conn,
		nil,
		registry,
		nil,
		versiontracker.NewVersionStateMap(),
		versiontracker.ResolverConfig{},
	)
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

func TestEditNodeCancelsRuntimeSnapshotWithRequest(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")

	entered := make(chan string, 1)
	release := make(chan struct{})
	remoteClient := &blockingSnapshotNodeClient{
		FakeNodeClient: &nodeclient.FakeNodeClient{NodeID: "node-remote"},
		entered:        entered,
		release:        release,
	}
	fixture.service.nodeRegistry = testParityRegistry(
		&nodeclient.FakeNodeClient{NodeID: "node-local"},
		remoteClient,
	)

	request := connect.NewRequest(&xylona.EditNodeRequest{Node: &xylona.Node{
		Id:      "node-remote",
		Name:    "Renamed Remote Node",
		BaseUrl: "https://node-remote.example.com",
		Enabled: true,
	}})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, errEdit := fixture.service.EditNode(ctx, request)
		result <- errEdit
	}()

	select {
	case nodeID := <-entered:
		if nodeID != "node-remote" {
			t.Fatalf("runtime snapshot node ID = %q, want %q", nodeID, "node-remote")
		}
		cancel()
	case <-time.After(time.Second):
		cancel()
		t.Fatal("timed out waiting for EditNode runtime snapshot")
	}

	select {
	case errEdit := <-result:
		if connect.CodeOf(errEdit) != connect.CodeCanceled {
			t.Fatalf("EditNode() code = %v, want %v (error %v)", connect.CodeOf(errEdit), connect.CodeCanceled, errEdit)
		}
	case <-time.After(time.Second):
		t.Fatal("EditNode() did not return after request cancellation")
	}
}

func TestSetServerVariantCancellationPreventsMutation(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-variant-cancel")
	before, errBefore := fixture.conn.GetGameServerByID("server-remote-variant-cancel")
	if errBefore != nil {
		t.Fatalf("GetGameServerByID(before) error = %v", errBefore)
	}

	entered := make(chan string, 1)
	remoteClient := &blockingProcessSnapshotNodeClient{
		FakeNodeClient: &nodeclient.FakeNodeClient{NodeID: "node-remote"},
		entered:        entered,
	}
	fixture.service.nodeRegistry = testParityRegistry(
		&nodeclient.FakeNodeClient{NodeID: "node-local"},
		remoteClient,
	)

	request := connect.NewRequest(&xylona.SetServerVariantRequest{
		GameServerId: "server-remote-variant-cancel",
		VariantId:    "paper",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, errSet := fixture.service.SetServerVariant(ctx, request)
		result <- errSet
	}()

	select {
	case processID := <-entered:
		if processID != "server-remote-variant-cancel" {
			t.Fatalf("process snapshot ID = %q, want %q", processID, "server-remote-variant-cancel")
		}
		cancel()
	case <-time.After(time.Second):
		cancel()
		t.Fatal("timed out waiting for SetServerVariant process snapshot")
	}

	select {
	case errSet := <-result:
		if connect.CodeOf(errSet) != connect.CodeCanceled {
			t.Fatalf("SetServerVariant() code = %v, want %v (error %v)", connect.CodeOf(errSet), connect.CodeCanceled, errSet)
		}
	case <-time.After(time.Second):
		t.Fatal("SetServerVariant() did not return after request cancellation")
	}

	after, errAfter := fixture.conn.GetGameServerByID("server-remote-variant-cancel")
	if errAfter != nil {
		t.Fatalf("GetGameServerByID(after) error = %v", errAfter)
	}
	if after.ServerSoftware != before.ServerSoftware || after.Branch != before.Branch || after.TargetPinned != before.TargetPinned {
		t.Fatalf(
			"variant selection changed after cancellation: before=(%v,%q,%v) after=(%v,%q,%v)",
			before.ServerSoftware,
			before.Branch,
			before.TargetPinned,
			after.ServerSoftware,
			after.Branch,
			after.TargetPinned,
		)
	}
}

func TestStopGameServerReportsRemoteStopFailure(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:         "node-remote",
		SnapshotResult: &node.NodeSnapshot{OS: "linux"},
		StopProcessErr: errors.New("node connection lost"),
	}
	registry := testParityRegistry(
		&nodeclient.FakeNodeClient{NodeID: "node-local", SnapshotResult: &node.NodeSnapshot{OS: "linux"}},
		remoteClient,
	)
	configureLifecycleActionsForParityTests(t, fixture, registry)

	request := connect.NewRequest(&xylona.StopGameServerRequest{ServerId: "server-remote-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")
	_, errStop := fixture.service.StopGameServer(context.Background(), request)
	if connect.CodeOf(errStop) != connect.CodeUnavailable {
		t.Fatalf("StopGameServer() code = %v, want %v (error %v)", connect.CodeOf(errStop), connect.CodeUnavailable, errStop)
	}
	if len(remoteClient.StopProcessCalls) != 1 {
		t.Fatalf("StopProcess call count = %d, want 1", len(remoteClient.StopProcessCalls))
	}
}

func TestRestartGameServerRejectsActiveUpdateOperation(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")

	remoteClient := &nodeclient.FakeNodeClient{NodeID: "node-remote"}
	registry := testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)
	configureLifecycleActionsForParityTests(t, fixture, registry)
	releaseUpdate, errOperation := fixture.service.actionsInst.TryBeginGameServerLifecycleOperation("server-remote-1")
	if errOperation != nil {
		t.Fatalf("TryBeginGameServerLifecycleOperation() error = %v", errOperation)
	}
	defer releaseUpdate()

	request := connect.NewRequest(&xylona.RestartGameServerRequest{ServerId: "server-remote-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")
	_, errRestart := fixture.service.RestartGameServer(t.Context(), request)
	if connect.CodeOf(errRestart) != connect.CodeAlreadyExists {
		t.Fatalf("RestartGameServer() code = %v, want %v (error %v)", connect.CodeOf(errRestart), connect.CodeAlreadyExists, errRestart)
	}
	if len(remoteClient.StopProcessCalls) != 0 || len(remoteClient.StartProcessCalls) != 0 {
		t.Fatalf(
			"restart process calls = (stop %d, start %d), want none while update operation is active",
			len(remoteClient.StopProcessCalls),
			len(remoteClient.StartProcessCalls),
		)
	}
}

func TestRemoveGameServerPreservesRecordWhenShutdownOrCleanupFails(t *testing.T) {
	cases := []struct {
		name                 string
		stopErr              error
		deleteErr            error
		processSnapshot      *node.ProcessSnapshot
		processSnapshotFound bool
		requestTimeout       time.Duration
		wantCode             connect.Code
		wantDeleteFilesCalls int
	}{
		{
			name:     "remote stop fails",
			stopErr:  errors.New("node connection lost"),
			wantCode: connect.CodeUnavailable,
		},
		{
			name:                 "file cleanup fails",
			deleteErr:            errors.New("remote directory unavailable"),
			wantCode:             connect.CodeInternal,
			wantDeleteFilesCalls: 1,
		},
		{
			name: "shutdown cannot be confirmed",
			processSnapshot: &node.ProcessSnapshot{
				ID:     "server-remote-1",
				Status: xylona.Status_ONLINE.String(),
			},
			processSnapshotFound: true,
			requestTimeout:       50 * time.Millisecond,
			wantCode:             connect.CodeDeadlineExceeded,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)
			insertRemoteNodeForParityTests(t, fixture, "node-remote")
			insertRemoteServerForParityTests(t, fixture, "server-remote-1")

			remoteClient := &nodeclient.FakeNodeClient{
				NodeID:                   "node-remote",
				SnapshotResult:           &node.NodeSnapshot{OS: "linux"},
				StopProcessErr:           tc.stopErr,
				DeleteFilesErr:           tc.deleteErr,
				GetProcessSnapshotResult: tc.processSnapshot,
				GetProcessSnapshotFound:  tc.processSnapshotFound,
			}
			registry := testParityRegistry(
				&nodeclient.FakeNodeClient{NodeID: "node-local", SnapshotResult: &node.NodeSnapshot{OS: "linux"}},
				remoteClient,
			)
			configureLifecycleActionsForParityTests(t, fixture, registry)

			request := connect.NewRequest(&xylona.RemoveGameServerRequest{ServerId: "server-remote-1"})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")
			requestCtx := context.Background()
			var cancel context.CancelFunc
			if tc.requestTimeout > 0 {
				requestCtx, cancel = context.WithTimeout(requestCtx, tc.requestTimeout)
				defer cancel()
			}

			_, errRemove := fixture.service.RemoveGameServer(requestCtx, request)
			if connect.CodeOf(errRemove) != tc.wantCode {
				t.Fatalf("RemoveGameServer() code = %v, want %v (error %v)", connect.CodeOf(errRemove), tc.wantCode, errRemove)
			}
			_, errGet := fixture.conn.GetGameServerByID("server-remote-1")
			if errGet != nil {
				t.Fatalf("GetGameServerByID() after failed remove error = %v; database row should remain", errGet)
			}
			if len(remoteClient.DeleteFilesCalls) != tc.wantDeleteFilesCalls {
				t.Fatalf("DeleteFiles call count = %d, want %d", len(remoteClient.DeleteFilesCalls), tc.wantDeleteFilesCalls)
			}
		})
	}
}

func TestListAggregatedGameServersIncludesRemoteRows(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.service.allPermissionIDs = []string{"game_server.start", "game_server.stop", "game_server.settings"}
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")
	insertServerOnNodeForParityTests(t, fixture, "server-remote-2", "node-remote", 25585)

	selfClient := &nodeclient.FakeNodeClient{
		NodeID: "node-local",
	}
	collectedAt := time.Now().UTC().Truncate(time.Second)
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		SnapshotResult: &node.NodeSnapshot{
			Collected: collectedAt,
			Processes: []node.ProcessSnapshot{
				{
					ID:     "server-remote-1",
					Name:   "Remote Server",
					Status: xylona.Status_ONLINE.String(),
				},
				{
					ID:     "server-remote-2",
					Name:   "Remote Server 2",
					Status: xylona.Status_OFFLINE.String(),
				},
			},
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
	if remoteSummary.GetIsStale() {
		t.Fatal("remote summary is_stale = true, want false for fresh node snapshot")
	}
	if !remoteSummary.GetResolvedHasUpdate() {
		t.Fatal("remote summary resolved_has_update = false, want true")
	}
	if !slices.Contains(remoteSummary.GetEffectivePermissions(), "game_server.start") {
		t.Fatalf("remote summary effective_permissions = %v, want game_server.start", remoteSummary.GetEffectivePermissions())
	}
	if remoteSummary.GetCurrentPlayers() != 0 {
		t.Fatalf("remote summary current_players = %d, want 0 without live query telemetry", remoteSummary.GetCurrentPlayers())
	}
	if remoteSummary.GetMaxPlayers() != 12 {
		t.Fatalf("remote summary max_players = %d, want configured capacity 12", remoteSummary.GetMaxPlayers())
	}
	if !remoteSummary.GetLastRemoteUpdate().AsTime().Equal(collectedAt) {
		t.Fatalf("remote summary last_remote_update = %v, want %v", remoteSummary.GetLastRemoteUpdate().AsTime(), collectedAt)
	}
	if remoteClient.SnapshotCalls != 1 {
		t.Fatalf("remote node snapshot call count = %d, want 1 for two servers", remoteClient.SnapshotCalls)
	}
	if len(remoteClient.GetProcessSnapshotCalls) != 0 {
		t.Fatalf("per-process snapshot calls = %v, want none", remoteClient.GetProcessSnapshotCalls)
	}
}

func TestListAggregatedGameServersDistinguishesOfflineFromUnavailable(t *testing.T) {
	cases := []struct {
		name       string
		client     *nodeclient.FakeNodeClient
		wantStatus xylona.Status
		wantStale  bool
	}{
		{
			name: "successful snapshot without process is offline",
			client: &nodeclient.FakeNodeClient{
				NodeID:         "node-remote",
				SnapshotResult: &node.NodeSnapshot{},
			},
			wantStatus: xylona.Status_OFFLINE,
		},
		{
			name: "snapshot failure is unknown and stale",
			client: &nodeclient.FakeNodeClient{
				NodeID:      "node-remote",
				SnapshotErr: errors.New("snapshot unavailable"),
			},
			wantStatus: xylona.Status_UNKNOWN,
			wantStale:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)
			insertRemoteNodeForParityTests(t, fixture, "node-remote")
			insertRemoteServerForParityTests(t, fixture, "server-remote-1")

			_, errUpdate := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
				ID:     omit.From("server-remote-1"),
				Status: omit.From(xylona.Status_ONLINE.String()),
			})
			if errUpdate != nil {
				t.Fatalf("UpdateGameServer() error = %v", errUpdate)
			}

			fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, tc.client)

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
			if remoteSummary.GetStatus() != tc.wantStatus {
				t.Fatalf("remote summary status = %v, want %v", remoteSummary.GetStatus(), tc.wantStatus)
			}
			if remoteSummary.GetIsStale() != tc.wantStale {
				t.Fatalf("remote summary is_stale = %v, want %v", remoteSummary.GetIsStale(), tc.wantStale)
			}
		})
	}
}

func TestListAggregatedGameServersFetchesOwningNodesConcurrently(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote-a")
	insertRemoteNodeForParityTests(t, fixture, "node-remote-b")
	insertServerOnNodeForParityTests(t, fixture, "server-remote-a", "node-remote-a", 25585)
	insertServerOnNodeForParityTests(t, fixture, "server-remote-b", "node-remote-b", 25595)

	entered := make(chan string, 2)
	release := make(chan struct{})
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{
		NodeID:         "node-local",
		SnapshotResult: &node.NodeSnapshot{},
	})
	for _, nodeID := range []string{"node-remote-a", "node-remote-b"} {
		registry.Register(&blockingSnapshotNodeClient{
			FakeNodeClient: &nodeclient.FakeNodeClient{
				NodeID:         nodeID,
				SnapshotResult: &node.NodeSnapshot{},
			},
			entered: entered,
			release: release,
		})
	}
	fixture.service.nodeRegistry = registry

	request := connect.NewRequest(&xylona.ListAggregatedGameServersRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")
	result := make(chan error, 1)
	go func() {
		_, errList := fixture.service.ListAggregatedGameServers(context.Background(), request)
		result <- errList
	}()

	seen := make(map[string]bool, 2)
	for range 2 {
		select {
		case nodeID := <-entered:
			seen[nodeID] = true
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for concurrent node snapshots")
		}
	}
	if !seen["node-remote-a"] || !seen["node-remote-b"] {
		t.Fatalf("concurrent snapshot nodes = %v, want both remote nodes", seen)
	}
	close(release)

	select {
	case errList := <-result:
		if errList != nil {
			t.Fatalf("ListAggregatedGameServers() error = %v", errList)
		}
	case <-time.After(time.Second):
		t.Fatal("ListAggregatedGameServers() did not finish after node snapshots were released")
	}
}

func TestListAggregatedGameServersRedactsLocalRowsForNonSuperuser(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, nil)

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From("/srv/local-backups"),
		MaxBackups:      omit.From(int64(5)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	request := connect.NewRequest(&xylona.ListAggregatedGameServersRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errList := fixture.service.ListAggregatedGameServers(context.Background(), request)
	if errList != nil {
		t.Fatalf("ListAggregatedGameServers() error = %v", errList)
	}

	var localServer *xylona.GameServer
	for _, server := range response.Msg.GetServers() {
		if server.GetIsLocal() && server.GetLocalServer().GetId() == "server-local-1" {
			localServer = server.GetLocalServer()
			break
		}
	}
	if localServer == nil {
		t.Fatal("ListAggregatedGameServers() missing local server-local-1 row")
	}
	if !localServer.GetBackupsEnabled() {
		t.Fatal("local server BackupsEnabled = false, want true")
	}
	if localServer.GetBackupDirectory() != "" {
		t.Fatalf("local server BackupDirectory = %q, want empty for non-superuser", localServer.GetBackupDirectory())
	}
	if localServer.GetMaxBackups() != 5 {
		t.Fatalf("local server MaxBackups = %d, want 5", localServer.GetMaxBackups())
	}
}

func TestListAggregatedGameServersRequiresMetricsPermissionForProcessMetrics(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.service.allPermissionIDs = []string{"game_server.view", "game_server.metrics"}
	localClient := &nodeclient.FakeNodeClient{
		NodeID: "node-local",
		SnapshotResult: &node.NodeSnapshot{Processes: []node.ProcessSnapshot{
			{
				ID:            "server-local-1",
				Status:        xylona.Status_ONLINE.String(),
				CPUPercent:    37.5,
				MemoryRSS:     128 * 1024 * 1024,
				MemoryPercent: 42.5,
				MetricsValid:  true,
				CPUValid:      true,
			},
		}},
	}
	fixture.service.nodeRegistry = testParityRegistry(localClient, nil)

	grantRequest := connect.NewRequest(&xylona.GrantGameServerAccessRequest{
		GameServerId: "server-local-1",
		UserId:       "user-other",
		RoleId:       "viewer",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, grantRequest, "user-owner")
	_, errGrant := fixture.service.GrantGameServerAccess(t.Context(), grantRequest)
	if errGrant != nil {
		t.Fatalf("GrantGameServerAccess() error = %v", errGrant)
	}

	tests := []struct {
		name        string
		userID      string
		wantMetrics bool
	}{
		{name: "owner receives process metrics", userID: "user-owner", wantMetrics: true},
		{name: "viewer does not receive process metrics", userID: "user-other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := connect.NewRequest(&xylona.ListAggregatedGameServersRequest{})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, tt.userID)
			response, errList := fixture.service.ListAggregatedGameServers(t.Context(), request)
			if errList != nil {
				t.Fatalf("ListAggregatedGameServers() error = %v", errList)
			}

			var localServer *xylona.GameServer
			for _, server := range response.Msg.GetServers() {
				if server.GetIsLocal() && server.GetLocalServer().GetId() == "server-local-1" {
					localServer = server.GetLocalServer()
					break
				}
			}
			if localServer == nil {
				t.Fatal("ListAggregatedGameServers() missing server-local-1")
			}
			if tt.wantMetrics && (localServer.GetCpuPercent() == 0 || localServer.GetMemoryWorkingSetBytes() == 0) {
				t.Fatalf(
					"owner metrics = (CPU %d, memory %d), want non-zero values",
					localServer.GetCpuPercent(),
					localServer.GetMemoryWorkingSetBytes(),
				)
			}
			if !tt.wantMetrics && (localServer.GetCpuPercent() != 0 || localServer.GetMemoryWorkingSetBytes() != 0) {
				t.Fatalf(
					"redacted metrics = (CPU %d, memory %d), want zero values",
					localServer.GetCpuPercent(),
					localServer.GetMemoryWorkingSetBytes(),
				)
			}
		})
	}
}

func TestListGameServersUsesOneSnapshotPerOwningNode(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")
	insertServerOnNodeForParityTests(t, fixture, "server-remote-2", "node-remote", 25585)

	selfClient := &nodeclient.FakeNodeClient{
		NodeID:         "node-local",
		SnapshotResult: &node.NodeSnapshot{},
	}
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		SnapshotResult: &node.NodeSnapshot{
			Processes: []node.ProcessSnapshot{
				{ID: "server-remote-1", Status: xylona.Status_ONLINE.String()},
				{ID: "server-remote-2", Status: xylona.Status_UPDATING.String()},
			},
		},
	}
	fixture.service.nodeRegistry = testParityRegistry(selfClient, remoteClient)

	request := connect.NewRequest(&xylona.ListGameServersRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")
	response, errList := fixture.service.ListGameServers(context.Background(), request)
	if errList != nil {
		t.Fatalf("ListGameServers() error = %v", errList)
	}

	statuses := make(map[string]xylona.Status, len(response.Msg.GetGameServers()))
	for _, gameServer := range response.Msg.GetGameServers() {
		statuses[gameServer.GetId()] = gameServer.GetStatus()
	}
	if statuses["server-remote-1"] != xylona.Status_ONLINE {
		t.Fatalf("server-remote-1 status = %v, want %v", statuses["server-remote-1"], xylona.Status_ONLINE)
	}
	if statuses["server-remote-2"] != xylona.Status_UPDATING {
		t.Fatalf("server-remote-2 status = %v, want %v", statuses["server-remote-2"], xylona.Status_UPDATING)
	}
	if remoteClient.SnapshotCalls != 1 {
		t.Fatalf("remote snapshot call count = %d, want 1", remoteClient.SnapshotCalls)
	}
	if len(remoteClient.GetProcessSnapshotCalls) != 0 {
		t.Fatalf("remote per-process snapshot calls = %v, want none", remoteClient.GetProcessSnapshotCalls)
	}
}

func TestListGameServersReportsUnknownWhenNodeSnapshotUnavailable(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")
	_, errUpdate := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:     omit.From("server-remote-1"),
		Status: omit.From(xylona.Status_ONLINE.String()),
	})
	if errUpdate != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdate)
	}

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:      "node-remote",
		SnapshotErr: errors.New("node unavailable"),
	}
	fixture.service.nodeRegistry = testParityRegistry(
		&nodeclient.FakeNodeClient{NodeID: "node-local", SnapshotResult: &node.NodeSnapshot{}},
		remoteClient,
	)

	request := connect.NewRequest(&xylona.ListGameServersRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")
	response, errList := fixture.service.ListGameServers(context.Background(), request)
	if errList != nil {
		t.Fatalf("ListGameServers() error = %v", errList)
	}
	for _, gameServer := range response.Msg.GetGameServers() {
		if gameServer.GetId() != "server-remote-1" {
			continue
		}
		if gameServer.GetStatus() != xylona.Status_UNKNOWN {
			t.Fatalf("remote status = %v, want %v", gameServer.GetStatus(), xylona.Status_UNKNOWN)
		}
		return
	}
	t.Fatal("ListGameServers() missing server-remote-1")
}

func TestListGameServersCancelsInFlightNodeSnapshots(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")

	entered := make(chan string, 1)
	release := make(chan struct{})
	remoteClient := &blockingSnapshotNodeClient{
		FakeNodeClient: &nodeclient.FakeNodeClient{NodeID: "node-remote"},
		entered:        entered,
		release:        release,
	}
	fixture.service.nodeRegistry = testParityRegistry(
		&nodeclient.FakeNodeClient{NodeID: "node-local", SnapshotResult: &node.NodeSnapshot{}},
		remoteClient,
	)

	request := connect.NewRequest(&xylona.ListGameServersRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, errList := fixture.service.ListGameServers(ctx, request)
		result <- errList
	}()

	select {
	case <-entered:
		cancel()
	case <-time.After(time.Second):
		cancel()
		t.Fatal("timed out waiting for remote snapshot request")
	}

	select {
	case errList := <-result:
		if connect.CodeOf(errList) != connect.CodeCanceled {
			t.Fatalf("ListGameServers() code = %v, want %v (error %v)", connect.CodeOf(errList), connect.CodeCanceled, errList)
		}
	case <-time.After(time.Second):
		t.Fatal("ListGameServers() did not return after request cancellation")
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

func TestReadGameServerOutputPreservesRequestCancellation(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")

	remoteClient := &cancellationAwareConsoleNodeClient{
		FakeNodeClient: &nodeclient.FakeNodeClient{NodeID: "node-remote"},
	}
	registry := testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)
	configureLifecycleActionsForParityTests(t, fixture, registry)

	request := connect.NewRequest(&xylona.ReadGameServerOutputRequest{ServerId: "server-remote-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, errRead := fixture.service.ReadGameServerOutput(ctx, request)
	if connect.CodeOf(errRead) != connect.CodeCanceled {
		t.Fatalf("ReadGameServerOutput() code = %v, want %v (error %v)", connect.CodeOf(errRead), connect.CodeCanceled, errRead)
	}
}

func TestSendGameServerInputPreservesRequestCancellation(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")

	remoteClient := &cancellationAwareConsoleNodeClient{
		FakeNodeClient: &nodeclient.FakeNodeClient{NodeID: "node-remote"},
	}
	fixture.service.nodeRegistry = testParityRegistry(
		&nodeclient.FakeNodeClient{NodeID: "node-local"},
		remoteClient,
	)

	request := connect.NewRequest(&xylona.SendGameServerInputRequest{ServerId: "server-remote-1", Input: "status"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, errSend := fixture.service.SendGameServerInput(ctx, request)
	if connect.CodeOf(errSend) != connect.CodeCanceled {
		t.Fatalf("SendGameServerInput() code = %v, want %v (error %v)", connect.CodeOf(errSend), connect.CodeCanceled, errSend)
	}
}

func TestSendGameServerInputMapsConsoleErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		inputError  error
		wantCode    connect.Code
		wantMessage string
	}{
		{
			name:        "returns sanitized command rejection",
			inputError:  node.NewConsoleInputRejectedError("Palworld API returned 401 Unauthorized"),
			wantCode:    connect.CodeInvalidArgument,
			wantMessage: "Palworld API returned 401 Unauthorized",
		},
		{
			name:        "returns reconnect message for unavailable transport",
			inputError:  node.ErrConsoleInputUnavailable,
			wantCode:    connect.CodeUnavailable,
			wantMessage: "game server console input is reconnecting; retry shortly",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fixture := newRBACRPCFixture(t)
			insertRemoteNodeForParityTests(t, fixture, "node-remote")
			insertRemoteServerForParityTests(t, fixture, "server-remote-1")
			remoteClient := &nodeclient.FakeNodeClient{
				NodeID:              "node-remote",
				SendConsoleInputErr: tc.inputError,
			}
			fixture.service.nodeRegistry = testParityRegistry(
				&nodeclient.FakeNodeClient{NodeID: "node-local"},
				remoteClient,
			)

			request := connect.NewRequest(&xylona.SendGameServerInputRequest{
				ServerId: "server-remote-1",
				Input:    "/Save",
			})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
			_, errSend := fixture.service.SendGameServerInput(t.Context(), request)
			if connect.CodeOf(errSend) != tc.wantCode {
				t.Fatalf("SendGameServerInput() code = %v, want %v (error %v)", connect.CodeOf(errSend), tc.wantCode, errSend)
			}
			if !strings.Contains(errSend.Error(), tc.wantMessage) {
				t.Fatalf("SendGameServerInput() error = %v, want containing %q", errSend, tc.wantMessage)
			}
		})
	}
}

func TestGetGameServerReportsOfflineWhenRemoteProcessSnapshotMissing(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")

	_, errUpdate := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:     omit.From("server-remote-1"),
		Status: omit.From(xylona.Status_ONLINE.String()),
	})
	if errUpdate != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdate)
	}

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.GetGameServerRequest{Id: "server-remote-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errGet := fixture.service.GetGameServer(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetGameServer() error = %v", errGet)
	}
	if response.Msg.GetGameServer().GetStatus() != xylona.Status_OFFLINE {
		t.Fatalf("GetGameServer().Status = %v, want %v", response.Msg.GetGameServer().GetStatus(), xylona.Status_OFFLINE)
	}
	if len(remoteClient.GetProcessSnapshotCalls) != 1 || remoteClient.GetProcessSnapshotCalls[0] != "server-remote-1" {
		t.Fatalf("GetProcessSnapshot calls = %+v, want [server-remote-1]", remoteClient.GetProcessSnapshotCalls)
	}
}

func TestGetGameServerReportsUnknownWhenRemoteProcessSnapshotFails(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")

	_, errUpdate := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:     omit.From("server-remote-1"),
		Status: omit.From(xylona.Status_ONLINE.String()),
	})
	if errUpdate != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdate)
	}

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                "node-remote",
		GetProcessSnapshotErr: errors.New("remote snapshot unavailable"),
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.GetGameServerRequest{Id: "server-remote-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errGet := fixture.service.GetGameServer(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetGameServer() error = %v", errGet)
	}
	if response.Msg.GetGameServer().GetStatus() != xylona.Status_UNKNOWN {
		t.Fatalf("GetGameServer().Status = %v, want %v", response.Msg.GetGameServer().GetStatus(), xylona.Status_UNKNOWN)
	}
	if len(remoteClient.GetProcessSnapshotCalls) != 1 || remoteClient.GetProcessSnapshotCalls[0] != "server-remote-1" {
		t.Fatalf("GetProcessSnapshot calls = %+v, want [server-remote-1]", remoteClient.GetProcessSnapshotCalls)
	}
}

func TestGetGameServerHydratesRemoteSnapshotAndTiming(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                  "node-remote",
		GetProcessSnapshotFound: true,
		GetProcessSnapshotResult: &node.ProcessSnapshot{
			ID:                   "server-remote-1",
			Status:               xylona.Status_ONLINE.String(),
			CPUPercent:           37.5,
			CPUCores:             2,
			MemoryVMS:            256 * 1024 * 1024,
			MemoryRSS:            128 * 1024 * 1024,
			MemoryPercent:        42.5,
			NumThreads:           17,
			DiskUsageBytes:       999,
			IOReadRate:           11,
			IOWriteRate:          22,
			IOValid:              true,
			ConnectionCount:      5,
			ConnectionCountValid: true,
			UnixStartedAt:        time.Now().Add(-2 * time.Minute).Unix(),
		},
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.GetGameServerRequest{Id: "server-remote-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errGet := fixture.service.GetGameServer(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetGameServer() error = %v", errGet)
	}

	gameServer := response.Msg.GetGameServer()
	if gameServer.GetStatus() != xylona.Status_ONLINE {
		t.Fatalf("GetGameServer().Status = %v, want %v", gameServer.GetStatus(), xylona.Status_ONLINE)
	}
	if gameServer.GetCpuPercent() != 37 {
		t.Fatalf("GetGameServer().CpuPercent = %d, want %d", gameServer.GetCpuPercent(), 37)
	}
	if gameServer.GetMemoryBytes() != 256*1024*1024 {
		t.Fatalf("GetGameServer().MemoryBytes = %d, want %d", gameServer.GetMemoryBytes(), 256*1024*1024)
	}
	if gameServer.GetConnectionCount() != 5 {
		t.Fatalf("GetGameServer().ConnectionCount = %d, want %d", gameServer.GetConnectionCount(), 5)
	}
	if !gameServer.GetIoValid() || !gameServer.GetConnectionCountValid() {
		t.Fatalf("GetGameServer() metric validity = (IO %t, connections %t), want both true", gameServer.GetIoValid(), gameServer.GetConnectionCountValid())
	}
}

func TestGetGameServerReturnsCheckingBeforeRemoteVersionStagingCompletes(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")

	readEntered := make(chan string, 4)
	readRelease := make(chan struct{})
	remoteClient := &blockingProbeInstalledVersionNodeClient{
		FakeNodeClient: &nodeclient.FakeNodeClient{
			NodeID: "node-remote",
			ProbeInstalledVersionResult: node.InstalledVersionProbeResult{
				Found:   true,
				Version: "1.20.4",
			},
			GetProcessSnapshotResult: &node.ProcessSnapshot{
				ID:     "server-remote-1",
				Status: xylona.Status_ONLINE.String(),
			},
			GetProcessSnapshotFound: true,
		},
		entered: readEntered,
		release: readRelease,
	}
	registry := testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)
	configureRemoteVersionTrackingForTests(t, fixture, registry, &rpcVersionTestTracker{
		installed: "1.20.4",
		latest:    "1.21.1",
	})
	_, errUpdate := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:      omit.From("server-remote-1"),
		Version: omit.From("1.18.2"),
	})
	if errUpdate != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdate)
	}
	fixture.service.versionState.Set("server-remote-1", versiontracker.VersionState{
		Status:          versiontracker.VersionStatusUnchecked,
		LatestVersion:   "1.21.1",
		LatestCheckTime: time.Now(),
		TrackerType:     "minecraft",
		ContextKey:      remoteVersionContextKeyForParityTests(t, fixture, "server-remote-1"),
	})

	request := connect.NewRequest(&xylona.GetGameServerRequest{Id: "server-remote-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	resultCh := make(chan *connect.Response[xylona.GetGameServerResponse], 1)
	errCh := make(chan error, 1)
	go func() {
		response, errGet := fixture.service.GetGameServer(context.Background(), request)
		resultCh <- response
		errCh <- errGet
	}()

	var response *connect.Response[xylona.GetGameServerResponse]
	select {
	case response = <-resultCh:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("GetGameServer() blocked on remote version refresh")
	}

	if errGet := <-errCh; errGet != nil {
		t.Fatalf("GetGameServer() error = %v", errGet)
	}

	gameServer := response.Msg.GetGameServer()
	if gameServer.GetStatus() != xylona.Status_ONLINE {
		t.Fatalf("GetGameServer().Status = %v, want %v", gameServer.GetStatus(), xylona.Status_ONLINE)
	}
	if gameServer.GetVersion() != "1.18.2" {
		t.Fatalf("GetGameServer().Version = %q, want persisted remote %q", gameServer.GetVersion(), "1.18.2")
	}
	if gameServer.GetVersionInfo().GetStatus() != xylona.VersionStatus_VERSION_STATUS_CHECKING {
		t.Fatalf("GetGameServer().VersionInfo.Status = %v, want %v", gameServer.GetVersionInfo().GetStatus(), xylona.VersionStatus_VERSION_STATUS_CHECKING)
	}

	select {
	case <-readEntered:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("remote version read did not start in the background")
	}

	close(readRelease)

	state := waitForVersionValues(t, fixture.service.versionState, "server-remote-1", "1.20.4", "1.21.1")
	if state.InstalledVersion != "1.20.4" {
		t.Fatalf("installed version = %q, want %q", state.InstalledVersion, "1.20.4")
	}
	if state.LatestVersion != "1.21.1" {
		t.Fatalf("latest version = %q, want %q", state.LatestVersion, "1.21.1")
	}
}

func TestGetGameServerCoalescesAsyncRemoteVersionRefreshes(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")

	readEntered := make(chan string, 4)
	readRelease := make(chan struct{})
	remoteClient := &blockingProbeInstalledVersionNodeClient{
		FakeNodeClient: &nodeclient.FakeNodeClient{
			NodeID: "node-remote",
			ProbeInstalledVersionResult: node.InstalledVersionProbeResult{
				Found:   true,
				Version: "1.20.4",
			},
			GetProcessSnapshotResult: &node.ProcessSnapshot{
				ID:     "server-remote-1",
				Status: xylona.Status_ONLINE.String(),
			},
			GetProcessSnapshotFound: true,
		},
		entered: readEntered,
		release: readRelease,
	}
	registry := testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)
	tracker := &rpcVersionTestTracker{
		installed: "1.20.4",
		latest:    "1.21.1",
	}
	configureRemoteVersionTrackingForTests(t, fixture, registry, tracker)
	_, errUpdate := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:      omit.From("server-remote-1"),
		Version: omit.From("1.18.2"),
	})
	if errUpdate != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdate)
	}
	fixture.service.versionState.Set("server-remote-1", versiontracker.VersionState{
		Status:             versiontracker.VersionStatusChecked,
		InstalledVersion:   "1.19.4",
		LatestVersion:      "1.21.1",
		UpdateAvailable:    true,
		InstalledCheckTime: time.Now().Add(-1 * time.Minute),
		LatestCheckTime:    time.Now(),
		LastCheckTime:      time.Now().Add(-3 * time.Minute),
		TrackerType:        "minecraft",
		ContextKey:         remoteVersionContextKeyForParityTests(t, fixture, "server-remote-1"),
	})

	requestA := connect.NewRequest(&xylona.GetGameServerRequest{Id: "server-remote-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, requestA, "user-owner")
	requestB := connect.NewRequest(&xylona.GetGameServerRequest{Id: "server-remote-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, requestB, "user-owner")

	type result struct {
		response *connect.Response[xylona.GetGameServerResponse]
		err      error
	}
	results := make(chan result, 2)
	go func() {
		response, errGet := fixture.service.GetGameServer(context.Background(), requestA)
		results <- result{response: response, err: errGet}
	}()
	go func() {
		response, errGet := fixture.service.GetGameServer(context.Background(), requestB)
		results <- result{response: response, err: errGet}
	}()

	for range 2 {
		select {
		case getResult := <-results:
			if getResult.err != nil {
				t.Fatalf("GetGameServer() error = %v", getResult.err)
			}
			if getResult.response.Msg.GetGameServer().GetVersionInfo().GetStatus() != xylona.VersionStatus_VERSION_STATUS_CHECKED {
				t.Fatalf("GetGameServer().VersionInfo.Status = %v, want cached checked state", getResult.response.Msg.GetGameServer().GetVersionInfo().GetStatus())
			}
			if getResult.response.Msg.GetGameServer().GetVersion() != "1.19.4" {
				t.Fatalf("GetGameServer().Version = %q, want cached %q", getResult.response.Msg.GetGameServer().GetVersion(), "1.19.4")
			}
			if getResult.response.Msg.GetGameServer().GetVersionInfo().GetInstalledVersion() != "1.19.4" {
				t.Fatalf("GetGameServer().VersionInfo.InstalledVersion = %q, want cached %q", getResult.response.Msg.GetGameServer().GetVersionInfo().GetInstalledVersion(), "1.19.4")
			}
		case <-time.After(250 * time.Millisecond):
			t.Fatal("GetGameServer() blocked while async remote refresh was pending")
		}
	}

	select {
	case <-readEntered:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected one background remote version read")
	}

	select {
	case extra := <-readEntered:
		t.Fatalf("unexpected duplicate remote version read %q before release", extra)
	case <-time.After(100 * time.Millisecond):
	}

	if remoteClient.probeCallCount() != 1 {
		t.Fatalf("remote ProbeInstalledVersion call count = %d, want 1 while refresh is coalesced", remoteClient.probeCallCount())
	}

	close(readRelease)

	state := waitForVersionValues(t, fixture.service.versionState, "server-remote-1", "1.20.4", "1.21.1")
	if state.InstalledVersion != "1.20.4" {
		t.Fatalf("installed version = %q, want %q", state.InstalledVersion, "1.20.4")
	}
	if state.LatestVersion != "1.21.1" {
		t.Fatalf("latest version = %q, want %q", state.LatestVersion, "1.21.1")
	}

}

func TestGetNodeRequiresSuperuser(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	request := connect.NewRequest(&xylona.GetNodeRequest{NodeId: "node-local"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	_, errGet := fixture.service.GetNode(context.Background(), request)
	if errGet == nil {
		t.Fatal("GetNode(non-superuser) expected error, got nil")
	}
	if connect.CodeOf(errGet) != connect.CodePermissionDenied {
		t.Fatalf("GetNode(non-superuser) code = %v, want %v", connect.CodeOf(errGet), connect.CodePermissionDenied)
	}
}

func TestListNodesRedactsListenURLForNonSuperuser(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")

	request := connect.NewRequest(&xylona.ListNodesRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errList := fixture.service.ListNodes(context.Background(), request)
	if errList != nil {
		t.Fatalf("ListNodes() error = %v", errList)
	}
	if len(response.Msg.GetNodes()) == 0 {
		t.Fatal("ListNodes() returned no nodes")
	}
	for _, entry := range response.Msg.GetNodes() {
		if entry.GetBaseUrl() != "" {
			t.Fatalf("ListNodes(non-superuser) leaked BaseUrl %q on node %q", entry.GetBaseUrl(), entry.GetId())
		}
		if entry.GetName() == "" {
			t.Fatalf("ListNodes(non-superuser) missing name for node %q", entry.GetId())
		}
	}

	adminRequest := connect.NewRequest(&xylona.ListNodesRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, adminRequest, "user-admin")
	adminResponse, errAdmin := fixture.service.ListNodes(context.Background(), adminRequest)
	if errAdmin != nil {
		t.Fatalf("ListNodes(admin) error = %v", errAdmin)
	}
	foundListenURL := false
	for _, entry := range adminResponse.Msg.GetNodes() {
		if entry.GetBaseUrl() != "" {
			foundListenURL = true
			break
		}
	}
	if !foundListenURL {
		t.Fatal("ListNodes(admin) expected at least one listen URL")
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
