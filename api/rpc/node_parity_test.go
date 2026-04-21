package rpc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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

func minecraftRemoteVersionContextKeyForParityTests() string {
	return versiontracker.TrackerContext{
		GameID:           "minecraft",
		ProviderKind:     "mojang",
		ProviderSourceID: "vanilla",
	}.CacheKey()
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

func TestListAggregatedGameServersReportsRemoteOfflineWhenSnapshotUnavailable(t *testing.T) {
	cases := []struct {
		name   string
		client *nodeclient.FakeNodeClient
	}{
		{
			name: "snapshot not found",
			client: &nodeclient.FakeNodeClient{
				NodeID: "node-remote",
			},
		},
		{
			name: "snapshot fails",
			client: &nodeclient.FakeNodeClient{
				NodeID:                "node-remote",
				GetProcessSnapshotErr: errors.New("snapshot unavailable"),
			},
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
			if remoteSummary.GetStatus() != xylona.Status_OFFLINE {
				t.Fatalf("remote summary status = %v, want %v", remoteSummary.GetStatus(), xylona.Status_OFFLINE)
			}
		})
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

func TestGetGameServerReportsOfflineWhenRemoteProcessSnapshotFails(t *testing.T) {
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
		GetProcessSnapshotErr: errors.New("remote snapshot should not be called"),
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

func TestGetGameServerHydratesRemoteSnapshotAndTiming(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-1")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                  "node-remote",
		GetProcessSnapshotFound: true,
		GetProcessSnapshotResult: &node.ProcessSnapshot{
			ID:              "server-remote-1",
			Status:          xylona.Status_ONLINE.String(),
			CPUPercent:      37.5,
			CPUCores:        2,
			MemoryVMS:       256 * 1024 * 1024,
			MemoryRSS:       128 * 1024 * 1024,
			MemoryPercent:   42.5,
			NumThreads:      17,
			DiskUsageBytes:  999,
			IOReadRate:      11,
			IOWriteRate:     22,
			ConnectionCount: 5,
			UnixStartedAt:   time.Now().Add(-2 * time.Minute).Unix(),
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
		ContextKey:      minecraftRemoteVersionContextKeyForParityTests(),
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
		ContextKey:         minecraftRemoteVersionContextKeyForParityTests(),
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
