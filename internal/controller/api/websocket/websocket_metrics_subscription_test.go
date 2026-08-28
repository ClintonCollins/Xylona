package websocket

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func newTestConnection() *connection {
	return &connection{
		id:                           uuid.New(),
		metricsPermissionLookup:      func(string) (bool, error) { return true, nil },
		consolePermissionLookup:      func(string) (bool, error) { return true, nil },
		requestedGameServerOutputIDs: make(map[string]struct{}),
		subscribedMetricsServerIDs:   make(map[string]struct{}),
		consoleStreamCancels:         make(map[string]context.CancelFunc),
		consoleStreamTokens:          make(map[string]*struct{}),
		RWMutex:                      &sync.RWMutex{},
	}
}

func TestProcessSnapshotToGameServerMetricsPreservesMetricValidity(t *testing.T) {
	t.Parallel()

	metrics := processSnapshotToGameServerMetrics(&node.ProcessSnapshot{
		IOValid:              true,
		ConnectionCountValid: true,
	}, time.Now())
	if metrics == nil {
		t.Fatal("processSnapshotToGameServerMetrics() returned nil")
	}
	if !metrics.GetIoValid() || !metrics.GetConnectionCountValid() {
		t.Fatalf("metric validity = (IO %t, connections %t), want both true", metrics.GetIoValid(), metrics.GetConnectionCountValid())
	}
}

func TestMetricsEqualDetectsMetricValidityChanges(t *testing.T) {
	t.Parallel()

	timestamp := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	base := processSnapshotToGameServerMetrics(&node.ProcessSnapshot{}, timestamp)
	ioChanged := processSnapshotToGameServerMetrics(&node.ProcessSnapshot{IOValid: true}, timestamp)
	connectionsChanged := processSnapshotToGameServerMetrics(&node.ProcessSnapshot{ConnectionCountValid: true}, timestamp)
	if metricsEqual(base, ioChanged) {
		t.Fatal("metricsEqual() ignored IO validity change")
	}
	if metricsEqual(base, connectionsChanged) {
		t.Fatal("metricsEqual() ignored connection validity change")
	}
}

func TestQueryEqualIncludesSourcePlayerList(t *testing.T) {
	t.Parallel()

	base := &xylona.ServerQuery{
		ServerId: "server-1",
		Type:     xylona.ServerQuery_Source,
		Source: &xylona.SourceQueryInfo{
			Players:             2,
			PlayerList:          []string{"Alyx", "Gordon"},
			PlayerListSupported: true,
		},
	}
	tests := []struct {
		name   string
		source *xylona.SourceQueryInfo
		want   bool
	}{
		{
			name: "matching player data",
			source: &xylona.SourceQueryInfo{
				Players:             2,
				PlayerList:          []string{"Alyx", "Gordon"},
				PlayerListSupported: true,
			},
			want: true,
		},
		{
			name: "different player list",
			source: &xylona.SourceQueryInfo{
				Players:             2,
				PlayerList:          []string{"Alyx", "Barney"},
				PlayerListSupported: true,
			},
			want: false,
		},
		{
			name: "different player list support",
			source: &xylona.SourceQueryInfo{
				Players:    2,
				PlayerList: []string{"Alyx", "Gordon"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			other := &xylona.ServerQuery{ServerId: "server-1", Type: xylona.ServerQuery_Source, Source: tt.source}
			got := queryEqual(base, other)
			if got != tt.want {
				t.Fatalf("queryEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConnection_ShouldReceiveMetrics(t *testing.T) {
	tests := []struct {
		name        string
		subscribed  []string
		accessible  []string
		queryServer string
		want        bool
	}{
		{
			name:        "subscribed server returns true",
			subscribed:  []string{"server-1"},
			accessible:  []string{"server-1"},
			queryServer: "server-1",
			want:        true,
		},
		{
			name:        "unsubscribed server returns false",
			subscribed:  []string{"server-1"},
			accessible:  []string{"server-1", "server-2"},
			queryServer: "server-2",
			want:        false,
		},
		{
			name:        "no subscriptions returns false",
			subscribed:  nil,
			accessible:  []string{"server-1"},
			queryServer: "server-1",
			want:        false,
		},
		{
			name:        "multiple subscriptions returns true for subscribed",
			subscribed:  []string{"server-1", "server-2", "server-3"},
			accessible:  []string{"server-1", "server-2", "server-3"},
			queryServer: "server-2",
			want:        true,
		},
		{
			name:        "unauthorized subscription returns false",
			subscribed:  []string{"server-1"},
			accessible:  []string{"server-2"},
			queryServer: "server-1",
			want:        false,
		},
		{
			name:        "empty server ID returns false when not subscribed",
			subscribed:  []string{"server-1"},
			accessible:  []string{"server-1"},
			queryServer: "",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestConnection()
			c.allGameServerIDs = tt.accessible
			for _, id := range tt.subscribed {
				c.subscribedMetricsServerIDs[id] = struct{}{}
			}

			got := c.shouldReceiveMetrics(tt.queryServer)
			if got != tt.want {
				t.Errorf("shouldReceiveMetrics(%q) = %v, want %v", tt.queryServer, got, tt.want)
			}
		})
	}
}

func TestConnection_ShouldReceiveMetrics_AfterUnsubscribe(t *testing.T) {
	c := newTestConnection()
	c.allGameServerIDs = []string{"server-1"}

	// Subscribe to server-1
	c.subscribedMetricsServerIDs["server-1"] = struct{}{}

	if !c.shouldReceiveMetrics("server-1") {
		t.Fatal("expected shouldReceiveMetrics to return true after subscribing")
	}

	// Unsubscribe from server-1
	delete(c.subscribedMetricsServerIDs, "server-1")

	if c.shouldReceiveMetrics("server-1") {
		t.Fatal("expected shouldReceiveMetrics to return false after unsubscribing")
	}
}

func TestConnection_ShouldReceiveMetrics_AfterPermissionRevocation(t *testing.T) {
	c := newTestConnection()
	c.allGameServerIDs = []string{"server-1"}
	c.subscribedMetricsServerIDs["server-1"] = struct{}{}
	permissionAllowed := true
	c.metricsPermissionLookup = func(string) (bool, error) {
		return permissionAllowed, nil
	}

	if !c.shouldReceiveMetrics("server-1") {
		t.Fatal("shouldReceiveMetrics() = false before permission revocation, want true")
	}
	permissionAllowed = false
	if c.shouldReceiveMetrics("server-1") {
		t.Fatal("shouldReceiveMetrics() = true after permission revocation, want false")
	}
}

func TestConnection_CanSubscribeToServerMetrics(t *testing.T) {
	tests := []struct {
		name             string
		hasAccess        bool
		hasLookup        bool
		permission       bool
		permissionErr    error
		want             bool
		wantLookupCalled bool
	}{
		{
			name:             "accessible server with metrics permission",
			hasAccess:        true,
			hasLookup:        true,
			permission:       true,
			want:             true,
			wantLookupCalled: true,
		},
		{
			name:             "accessible server without metrics permission",
			hasAccess:        true,
			hasLookup:        true,
			wantLookupCalled: true,
		},
		{
			name:      "accessible server without permission lookup fails closed",
			hasAccess: true,
		},
		{
			name:             "inaccessible server fails before permission lookup",
			hasLookup:        true,
			permission:       true,
			permissionErr:    errors.New("lookup should not run"),
			wantLookupCalled: false,
		},
		{
			name:             "permission lookup failure fails closed",
			hasAccess:        true,
			hasLookup:        true,
			permissionErr:    errors.New("permission lookup failed"),
			wantLookupCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestConnection()
			c.metricsPermissionLookup = nil
			if tt.hasAccess {
				c.allGameServerIDs = []string{"server-1"}
			}
			lookupCalled := false
			if tt.hasLookup {
				c.metricsPermissionLookup = func(serverID string) (bool, error) {
					lookupCalled = true
					if serverID != "server-1" {
						t.Fatalf("metrics permission server ID = %q, want server-1", serverID)
					}
					return tt.permission, tt.permissionErr
				}
			}

			got := c.canSubscribeToServerMetrics("server-1")
			if got != tt.want {
				t.Errorf("canSubscribeToServerMetrics() = %t, want %t", got, tt.want)
			}
			if lookupCalled != tt.wantLookupCalled {
				t.Errorf("metrics permission lookup called = %t, want %t", lookupCalled, tt.wantLookupCalled)
			}
		})
	}
}

func TestWebSocket_CollectSubscribedServerMetrics(t *testing.T) {
	gameServers := []*models.GameServer{
		{ID: "server-1", NodeID: "node-1"},
		{ID: "server-2", NodeID: "node-1"},
	}
	tests := []struct {
		name          string
		subscribed    []string
		accessible    []string
		wantServerIDs []string
		wantCalls     int
	}{
		{
			name:       "no subscriptions skip snapshot collection",
			accessible: []string{"server-1", "server-2"},
		},
		{
			name:          "one subscription bounds collection and payload",
			subscribed:    []string{"server-1"},
			accessible:    []string{"server-1", "server-2"},
			wantServerIDs: []string{"server-1"},
			wantCalls:     1,
		},
		{
			name:          "multiple subscriptions bound collection and payload",
			subscribed:    []string{"server-1", "server-2"},
			accessible:    []string{"server-1", "server-2"},
			wantServerIDs: []string{"server-1", "server-2"},
			wantCalls:     1,
		},
		{
			name:       "unauthorized subscription cannot trigger collection",
			subscribed: []string{"server-2"},
			accessible: []string{"server-1"},
		},
		{
			name:          "unauthorized subscription is excluded from mixed payload",
			subscribed:    []string{"server-1", "server-2"},
			accessible:    []string{"server-1"},
			wantServerIDs: []string{"server-1"},
			wantCalls:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &nodeclient.FakeNodeClient{
				NodeID: "node-1",
				SnapshotResult: &node.NodeSnapshot{
					Collected: time.Now(),
					Processes: []node.ProcessSnapshot{
						{ID: "server-1", MetricsValid: true},
						{ID: "server-2", MetricsValid: true},
					},
				},
			}
			conn := newTestConnection()
			conn.allGameServerIDs = tt.accessible
			for _, serverID := range tt.subscribed {
				conn.subscribedMetricsServerIDs[serverID] = struct{}{}
			}
			ws := &WebSocket{
				nodeRegistry: noderegistry.New("node-1", client),
			}

			metrics := ws.collectSubscribedServerMetrics(t.Context(), conn, gameServers)

			if client.SnapshotCalls != tt.wantCalls {
				t.Fatalf("GetNodeSnapshot calls = %d, want %d", client.SnapshotCalls, tt.wantCalls)
			}
			if len(tt.wantServerIDs) == 0 {
				if metrics != nil {
					t.Fatalf("metrics = %+v, want nil", metrics)
				}
				return
			}
			if metrics == nil {
				t.Fatal("metrics = nil, want payload")
			}
			if len(metrics.GetServers()) != len(tt.wantServerIDs) {
				t.Fatalf("payload server count = %d, want %d", len(metrics.GetServers()), len(tt.wantServerIDs))
			}
			for _, serverID := range tt.wantServerIDs {
				_, exists := metrics.GetServers()[serverID]
				if !exists {
					t.Errorf("payload missing subscribed server %q", serverID)
				}
			}
		})
	}
}

func TestWebSocket_CollectSubscribedServerMetricsStopsAfterPermissionRevocation(t *testing.T) {
	client := &nodeclient.FakeNodeClient{
		NodeID: "node-1",
		SnapshotResult: &node.NodeSnapshot{
			Collected: time.Now(),
			Processes: []node.ProcessSnapshot{{ID: "server-1", MetricsValid: true}},
		},
	}
	conn := newTestConnection()
	conn.allGameServerIDs = []string{"server-1"}
	conn.subscribedMetricsServerIDs["server-1"] = struct{}{}
	permissionAllowed := true
	conn.metricsPermissionLookup = func(string) (bool, error) {
		return permissionAllowed, nil
	}
	ws := &WebSocket{nodeRegistry: noderegistry.New("node-1", client)}
	servers := []*models.GameServer{{ID: "server-1", NodeID: "node-1"}}

	if metrics := ws.collectSubscribedServerMetrics(t.Context(), conn, servers); metrics == nil {
		t.Fatal("collectSubscribedServerMetrics() = nil before permission revocation")
	}
	permissionAllowed = false
	if metrics := ws.collectSubscribedServerMetrics(t.Context(), conn, servers); metrics != nil {
		t.Fatalf("collectSubscribedServerMetrics() = %+v after permission revocation, want nil", metrics)
	}
	if client.SnapshotCalls != 1 {
		t.Fatalf("GetNodeSnapshot calls = %d, want 1 because revoked delivery must not collect", client.SnapshotCalls)
	}
}

func TestWebSocket_MetricsConnectionsForServerStopsAfterPermissionRevocation(t *testing.T) {
	permissionAllowed := true
	conn := newTestConnection()
	conn.userID = "user-1"
	conn.allGameServerIDs = []string{"server-1"}
	conn.subscribedMetricsServerIDs["server-1"] = struct{}{}
	conn.metricsPermissionLookup = func(string) (bool, error) {
		return permissionAllowed, nil
	}
	ws := &WebSocket{
		userWebsocketConnections: map[string]map[uuid.UUID]*connection{
			conn.userID: {conn.id: conn},
		},
		userWebsocketConnectionsLock: &sync.RWMutex{},
	}

	if connections := ws.metricsConnectionsForServer("server-1"); len(connections) != 1 {
		t.Fatalf("metricsConnectionsForServer() len = %d before permission revocation, want 1", len(connections))
	}
	permissionAllowed = false
	if connections := ws.metricsConnectionsForServer("server-1"); len(connections) != 0 {
		t.Fatalf("metricsConnectionsForServer() len = %d after permission revocation, want 0", len(connections))
	}
}

func TestConnection_ConsumeRequestedGameServerOutputIDs_ClearsSubscriptions(t *testing.T) {
	c := newTestConnection()
	c.requestedGameServerOutputIDs["server-1"] = struct{}{}
	c.requestedGameServerOutputIDs["server-2"] = struct{}{}

	consumed := c.consumeRequestedGameServerOutputIDs()

	if len(consumed) != 2 {
		t.Fatalf("consumeRequestedGameServerOutputIDs() len = %d, want 2", len(consumed))
	}
	if len(c.requestedGameServerOutputIDs) != 0 {
		t.Fatalf("requestedGameServerOutputIDs len = %d, want 0 after consume", len(c.requestedGameServerOutputIDs))
	}
}

func TestResolveGameServerStatusDistinguishesOfflineFromUnavailable(t *testing.T) {
	tests := []struct {
		name       string
		client     *nodeclient.FakeNodeClient
		registry   bool
		wantStatus xylona.Status
	}{
		{
			name:       "registry unavailable",
			wantStatus: xylona.Status_UNKNOWN,
		},
		{
			name:       "node client unavailable",
			registry:   true,
			wantStatus: xylona.Status_UNKNOWN,
		},
		{
			name: "snapshot transport fails",
			client: &nodeclient.FakeNodeClient{
				NodeID:                "node-remote",
				GetProcessSnapshotErr: errors.New("snapshot unavailable"),
			},
			registry:   true,
			wantStatus: xylona.Status_UNKNOWN,
		},
		{
			name:       "process is not tracked",
			client:     &nodeclient.FakeNodeClient{NodeID: "node-remote"},
			registry:   true,
			wantStatus: xylona.Status_OFFLINE,
		},
		{
			name: "snapshot status is invalid",
			client: &nodeclient.FakeNodeClient{
				NodeID:                  "node-remote",
				GetProcessSnapshotFound: true,
				GetProcessSnapshotResult: &node.ProcessSnapshot{
					Status: "NOT_A_STATUS",
				},
			},
			registry:   true,
			wantStatus: xylona.Status_UNKNOWN,
		},
		{
			name: "online snapshot",
			client: &nodeclient.FakeNodeClient{
				NodeID:                  "node-remote",
				GetProcessSnapshotFound: true,
				GetProcessSnapshotResult: &node.ProcessSnapshot{
					Status: xylona.Status_ONLINE.String(),
				},
			},
			registry:   true,
			wantStatus: xylona.Status_ONLINE,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ws := &WebSocket{ctx: context.Background()}
			if test.registry {
				ws.nodeRegistry = noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
				if test.client != nil {
					ws.nodeRegistry.Register(test.client)
				}
			}

			status := ws.resolveGameServerStatus(&models.GameServer{ID: "server-remote", NodeID: "node-remote"})
			if status != test.wantStatus {
				t.Fatalf("resolveGameServerStatus() = %v, want %v", status, test.wantStatus)
			}
		})
	}
}

type snapshotGateClient struct {
	*nodeclient.FakeNodeClient
	started chan<- string
	release <-chan struct{}
}

func (c *snapshotGateClient) GetNodeSnapshot(ctx context.Context) (*node.NodeSnapshot, error) {
	select {
	case c.started <- c.ID():
	case <-ctx.Done():
		return nil, fmt.Errorf("snapshot gate start: %w", ctx.Err())
	}

	select {
	case <-c.release:
		return c.SnapshotResult, c.SnapshotErr
	case <-ctx.Done():
		return nil, fmt.Errorf("snapshot gate release: %w", ctx.Err())
	}
}

func TestFetchNodeSnapshots(t *testing.T) {
	t.Run("fetches distinct nodes concurrently", func(t *testing.T) {
		started := make(chan string, 2)
		release := make(chan struct{})
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		clients := map[string]nodeclient.NodeClient{
			"node-a": &snapshotGateClient{
				FakeNodeClient: &nodeclient.FakeNodeClient{
					NodeID:         "node-a",
					SnapshotResult: &node.NodeSnapshot{},
				},
				started: started,
				release: release,
			},
			"node-b": &snapshotGateClient{
				FakeNodeClient: &nodeclient.FakeNodeClient{
					NodeID:         "node-b",
					SnapshotResult: &node.NodeSnapshot{},
				},
				started: started,
				release: release,
			},
		}
		resultsDone := make(chan map[string]nodeSnapshotResult, 1)
		go func() {
			resultsDone <- fetchNodeSnapshots(ctx, clients)
		}()

		seen := make(map[string]struct{}, len(clients))
		for range clients {
			select {
			case nodeID := <-started:
				seen[nodeID] = struct{}{}
			case <-time.After(time.Second):
				t.Fatal("distinct node snapshots did not start concurrently")
			}
		}
		close(release)

		results := <-resultsDone
		if len(seen) != len(clients) || len(results) != len(clients) {
			t.Fatalf("started nodes = %v, results = %v, want both nodes", seen, results)
		}
		for nodeID, result := range results {
			if result.err != nil || result.snapshot == nil {
				t.Fatalf("result for %s = %+v, want snapshot without error", nodeID, result)
			}
		}
	})

	t.Run("propagates caller cancellation", func(t *testing.T) {
		started := make(chan string, 1)
		release := make(chan struct{})
		client := &snapshotGateClient{
			FakeNodeClient: &nodeclient.FakeNodeClient{
				NodeID:         "node-canceled",
				SnapshotResult: &node.NodeSnapshot{},
			},
			started: started,
			release: release,
		}
		ctx, cancel := context.WithCancel(context.Background())
		resultsDone := make(chan map[string]nodeSnapshotResult, 1)
		go func() {
			resultsDone <- fetchNodeSnapshots(ctx, map[string]nodeclient.NodeClient{
				client.ID(): client,
			})
		}()

		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("snapshot did not start")
		}
		cancel()

		select {
		case results := <-resultsDone:
			result := results[client.ID()]
			if !errors.Is(result.err, context.Canceled) {
				t.Fatalf("snapshot error = %v, want context canceled", result.err)
			}
		case <-time.After(time.Second):
			t.Fatal("snapshot collection did not stop after caller cancellation")
		}
	})
}

func TestConnection_HasGameServerAccess_RevalidatesRevokedSuperUser(t *testing.T) {
	c := newTestConnection()
	c.userID = "user-1"
	c.isSuperUser = true
	c.lastSuperUserCheck = time.Now().Add(-10 * time.Second)
	c.userLookup = func(string) (*models.User, error) {
		return &models.User{ID: "user-1", SuperUser: false}, nil
	}

	if c.hasGameServerAccess("server-1") {
		t.Fatal("hasGameServerAccess() = true, want false after superuser revocation")
	}
}

func TestConnection_HasGameServerAccess_FallsBackToOwnedServersAfterRevocation(t *testing.T) {
	c := newTestConnection()
	c.userID = "user-1"
	c.isSuperUser = true
	c.allGameServerIDs = []string{"server-1"}
	c.lastSuperUserCheck = time.Now().Add(-10 * time.Second)
	c.userLookup = func(string) (*models.User, error) {
		return &models.User{ID: "user-1", SuperUser: false}, nil
	}

	if !c.hasGameServerAccess("server-1") {
		t.Fatal("hasGameServerAccess() = false, want true for owned server after superuser revocation")
	}
	if c.hasGameServerAccess("server-2") {
		t.Fatal("hasGameServerAccess() = true, want false for unowned server after superuser revocation")
	}
}

func TestConnection_HasGameServerAccess_DropsElevatedAccessOnRefreshFailure(t *testing.T) {
	c := newTestConnection()
	c.userID = "user-1"
	c.isSuperUser = true
	c.lastSuperUserCheck = time.Now().Add(-10 * time.Second)
	c.userLookup = func(string) (*models.User, error) {
		return nil, errors.New("lookup failed")
	}

	if c.hasGameServerAccess("server-1") {
		t.Fatal("hasGameServerAccess() = true, want false after refresh failure")
	}
}

func TestNodeMetricsLoopActionSkipsTickWhenSuperUserRevoked(t *testing.T) {
	c := newTestConnection()
	c.userID = "user-1"
	c.isSuperUser = true
	c.lastSuperUserCheck = time.Now().Add(-10 * time.Second)
	c.userLookup = func(string) (*models.User, error) {
		return &models.User{ID: "user-1", SuperUser: false}, nil
	}

	action := determineNodeMetricsLoopAction(c, nil)
	if action != nodeMetricsLoopActionSkip {
		t.Fatalf("determineNodeMetricsLoopAction() = %v, want %v", action, nodeMetricsLoopActionSkip)
	}
}

func TestWebSocket_GameServerConnectionsWithAccess(t *testing.T) {
	ws := &WebSocket{
		userWebsocketConnections:     make(map[string]map[uuid.UUID]*connection),
		userWebsocketConnectionsLock: &sync.RWMutex{},
	}

	allowed := newTestConnection()
	allowed.userID = "user-1"
	allowed.allGameServerIDs = []string{"server-1"}

	denied := newTestConnection()
	denied.userID = "user-2"
	denied.allGameServerIDs = []string{"server-2"}

	superUser := newTestConnection()
	superUser.userID = "user-3"
	superUser.isSuperUser = true

	ws.userWebsocketConnections[allowed.userID] = map[uuid.UUID]*connection{
		allowed.id: allowed,
	}
	ws.userWebsocketConnections[denied.userID] = map[uuid.UUID]*connection{
		denied.id: denied,
	}
	ws.userWebsocketConnections[superUser.userID] = map[uuid.UUID]*connection{
		superUser.id: superUser,
	}

	connections := ws.gameServerConnectionsWithAccess("server-1")

	if len(connections) != 2 {
		t.Fatalf("gameServerConnectionsWithAccess() len = %d, want 2", len(connections))
	}

	seen := make(map[uuid.UUID]struct{}, len(connections))
	for _, conn := range connections {
		seen[conn.id] = struct{}{}
	}

	if _, ok := seen[allowed.id]; !ok {
		t.Fatal("expected allowed connection to receive install broadcasts")
	}
	if _, ok := seen[superUser.id]; !ok {
		t.Fatal("expected superuser connection to receive install broadcasts")
	}
	if _, ok := seen[denied.id]; ok {
		t.Fatal("expected unauthorized connection to be excluded from install broadcasts")
	}
}

func TestWebSocket_StartConsoleStreamForwardsRemoteChunks(t *testing.T) {
	stream := make(chan node.ConsoleChunk, 1)
	stream <- node.ConsoleChunk{ProcessID: "server-remote", Data: "hello remote\n"}

	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                     "node-remote",
		StreamConsoleOutputChannel: stream,
	}
	registry.Register(remoteClient)

	conn := newTestConnection()
	conn.outputStreamChannel = make(chan *xylona.Message, 2)

	ws := &WebSocket{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}

	errStart := ws.startConsoleStream(conn, &models.GameServer{
		ID:     "server-remote",
		NodeID: "node-remote",
	})
	if errStart != nil {
		t.Fatalf("startConsoleStream() error = %v", errStart)
	}

	select {
	case msg := <-conn.outputStreamChannel:
		state := msg.GetGameServerConsoleOutput()
		if state.Reconnecting == nil || state.GetReconnecting() {
			t.Fatalf("initial connection state = %+v, want present false", state)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for connected console state")
	}

	select {
	case msg := <-conn.outputStreamChannel:
		if msg.GetType() != xylona.Message_GameServerConsole {
			t.Fatalf("message type = %v, want %v", msg.GetType(), xylona.Message_GameServerConsole)
		}
		if msg.GetGameServerConsoleOutput().GetGameServerId() != "server-remote" {
			t.Fatalf("gameServerId = %q, want %q", msg.GetGameServerConsoleOutput().GetGameServerId(), "server-remote")
		}
		if msg.GetGameServerConsoleOutput().GetOutput() != "hello remote\n" {
			t.Fatalf("console output = %q, want %q", msg.GetGameServerConsoleOutput().GetOutput(), "hello remote\n")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for remote console output")
	}

	if len(remoteClient.StreamConsoleOutputCalls) != 1 || remoteClient.StreamConsoleOutputCalls[0] != "server-remote" {
		t.Fatalf("StreamConsoleOutput calls = %+v, want server-remote", remoteClient.StreamConsoleOutputCalls)
	}
}

func TestWebSocket_SubscribeGameServerConsoleUsesSelfNodeClientStream(t *testing.T) {
	stream := make(chan node.ConsoleChunk, 1)
	stream <- node.ConsoleChunk{ProcessID: "server-local", Data: "hello local\n"}

	localClient := &nodeclient.FakeNodeClient{
		NodeID:                     "node-local",
		StreamConsoleOutputChannel: stream,
	}
	registry := noderegistry.New("node-local", localClient)

	conn := newTestConnection()
	conn.outputStreamChannel = make(chan *xylona.Message, 2)

	ws := &WebSocket{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}

	errSubscribe := ws.subscribeGameServerConsole(conn, &models.GameServer{
		ID:     "server-local",
		NodeID: "node-local",
	})
	if errSubscribe != nil {
		t.Fatalf("subscribeGameServerConsole() error = %v", errSubscribe)
	}

	select {
	case msg := <-conn.outputStreamChannel:
		state := msg.GetGameServerConsoleOutput()
		if state.Reconnecting == nil || state.GetReconnecting() {
			t.Fatalf("initial connection state = %+v, want present false", state)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for connected console state")
	}

	select {
	case msg := <-conn.outputStreamChannel:
		if msg.GetGameServerConsoleOutput().GetOutput() != "hello local\n" {
			t.Fatalf("console output = %q, want %q", msg.GetGameServerConsoleOutput().GetOutput(), "hello local\n")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for local console output")
	}

	if len(localClient.StreamConsoleOutputCalls) != 1 || localClient.StreamConsoleOutputCalls[0] != "server-local" {
		t.Fatalf("StreamConsoleOutput calls = %+v, want server-local", localClient.StreamConsoleOutputCalls)
	}
}

type sequencedConsoleNodeClient struct {
	*nodeclient.FakeNodeClient

	mu       sync.Mutex
	streams  []<-chan node.ConsoleChunk
	attempts chan int
}

func (c *sequencedConsoleNodeClient) StreamConsoleOutput(_ context.Context, _ string) (<-chan node.ConsoleChunk, error) {
	c.mu.Lock()
	attempt := len(c.StreamConsoleOutputCalls)
	c.StreamConsoleOutputCalls = append(c.StreamConsoleOutputCalls, "server-remote")
	var stream <-chan node.ConsoleChunk
	if attempt < len(c.streams) {
		stream = c.streams[attempt]
	}
	c.mu.Unlock()

	select {
	case c.attempts <- attempt + 1:
	default:
	}
	return stream, nil
}

func (c *sequencedConsoleNodeClient) attemptCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.StreamConsoleOutputCalls)
}

func TestWebSocket_ConsoleStreamRetriesWithExplicitStateAndResetReplay(t *testing.T) {
	first := make(chan node.ConsoleChunk, 1)
	first <- node.ConsoleChunk{ProcessID: "server-remote", Data: "first\n", Sequence: 1}
	close(first)
	second := make(chan node.ConsoleChunk, 1)
	second <- node.ConsoleChunk{
		ProcessID:   "server-remote",
		Data:        "first\nsecond\n",
		Sequence:    2,
		ResetBuffer: true,
	}

	client := &sequencedConsoleNodeClient{
		FakeNodeClient: &nodeclient.FakeNodeClient{NodeID: "node-remote"},
		streams:        []<-chan node.ConsoleChunk{first, second},
		attempts:       make(chan int, 2),
	}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(client)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	conn := newTestConnection()
	conn.outputStreamChannel = make(chan *xylona.Message, 8)
	var permissionChecks atomic.Int32
	conn.consolePermissionLookup = func(string) (bool, error) {
		permissionChecks.Add(1)
		return true, nil
	}
	ws := &WebSocket{ctx: ctx, nodeRegistry: registry}

	errStart := ws.startConsoleStream(conn, &models.GameServer{ID: "server-remote", NodeID: "node-remote"})
	if errStart != nil {
		t.Fatalf("startConsoleStream() error = %v", errStart)
	}

	connected := receiveConsoleMessage(t, conn.outputStreamChannel, 2*time.Second)
	if connected.Reconnecting == nil || connected.GetReconnecting() || connected.GetOutput() != "" {
		t.Fatalf("initial connected control = %+v", connected)
	}
	live := receiveConsoleMessage(t, conn.outputStreamChannel, 2*time.Second)
	if live.Reconnecting != nil || live.GetResetBuffer() || live.GetSequence() != 1 || live.GetOutput() != "first\n" {
		t.Fatalf("live output = %+v", live)
	}
	reconnecting := receiveConsoleMessage(t, conn.outputStreamChannel, 2*time.Second)
	if reconnecting.Reconnecting == nil || !reconnecting.GetReconnecting() || !strings.Contains(reconnecting.GetOutput(), "reconnecting") {
		t.Fatalf("reconnecting control = %+v", reconnecting)
	}
	reconnected := receiveConsoleMessage(t, conn.outputStreamChannel, 2*time.Second)
	if reconnected.Reconnecting == nil || reconnected.GetReconnecting() || reconnected.GetOutput() != "" {
		t.Fatalf("reconnected control = %+v", reconnected)
	}
	replay := receiveConsoleMessage(t, conn.outputStreamChannel, 2*time.Second)
	if replay.Reconnecting != nil || !replay.GetResetBuffer() || replay.GetSequence() != 2 || replay.GetOutput() != "first\nsecond\n" {
		t.Fatalf("reset replay = %+v", replay)
	}
	if client.attemptCount() != 2 {
		t.Fatalf("console stream attempts = %d, want 2", client.attemptCount())
	}
	got := permissionChecks.Load()
	if got != 2 {
		t.Fatalf("console permission checks = %d, want one per stream attempt", got)
	}
}

func TestWebSocket_ConsoleStreamBackpressureRestartsForResetReplay(t *testing.T) {
	first := make(chan node.ConsoleChunk, 1)
	second := make(chan node.ConsoleChunk, 1)
	second <- node.ConsoleChunk{
		ProcessID:   "server-remote",
		Data:        "complete buffer\n",
		Sequence:    9,
		ResetBuffer: true,
	}
	client := &sequencedConsoleNodeClient{
		FakeNodeClient: &nodeclient.FakeNodeClient{NodeID: "node-remote"},
		streams:        []<-chan node.ConsoleChunk{first, second},
		attempts:       make(chan int, 2),
	}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(client)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	conn := newTestConnection()
	conn.outputStreamChannel = make(chan *xylona.Message, 1)
	ws := &WebSocket{ctx: ctx, nodeRegistry: registry}

	errStart := ws.startConsoleStream(conn, &models.GameServer{ID: "server-remote", NodeID: "node-remote"})
	if errStart != nil {
		t.Fatalf("startConsoleStream() error = %v", errStart)
	}
	connected := receiveConsoleMessage(t, conn.outputStreamChannel, time.Second)
	if connected.Reconnecting == nil || connected.GetReconnecting() {
		t.Fatalf("initial connected control = %+v", connected)
	}

	blocker := &xylona.Message{Type: xylona.Message_Unknown}
	conn.outputStreamChannel <- blocker
	first <- node.ConsoleChunk{ProcessID: "server-remote", Data: "will be replayed\n", Sequence: 8}

	timer := time.NewTimer(1250 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal("context canceled before backpressure timeout")
	}
	if got := <-conn.outputStreamChannel; got != blocker {
		t.Fatalf("backpressure blocker = %p, want %p", got, blocker)
	}

	reconnecting := receiveConsoleMessage(t, conn.outputStreamChannel, 2*time.Second)
	if reconnecting.Reconnecting == nil || !reconnecting.GetReconnecting() {
		t.Fatalf("reconnecting control = %+v", reconnecting)
	}
	reconnected := receiveConsoleMessage(t, conn.outputStreamChannel, 2*time.Second)
	if reconnected.Reconnecting == nil || reconnected.GetReconnecting() {
		t.Fatalf("reconnected control = %+v", reconnected)
	}
	replay := receiveConsoleMessage(t, conn.outputStreamChannel, 2*time.Second)
	if replay.Reconnecting != nil || !replay.GetResetBuffer() || replay.GetSequence() != 9 || replay.GetOutput() != "complete buffer\n" {
		t.Fatalf("reset replay = %+v", replay)
	}
	if client.attemptCount() < 2 {
		t.Fatalf("console stream attempts = %d, want at least 2", client.attemptCount())
	}
}

func TestWebSocket_ConsoleConnectionBackpressureIsDebugOnly(t *testing.T) {
	var logOutput bytes.Buffer
	previousLogger := log.Logger
	log.Logger = zerolog.New(&logOutput).Level(zerolog.DebugLevel)
	t.Cleanup(func() {
		log.Logger = previousLogger
	})

	client := &nodeclient.FakeNodeClient{
		NodeID:                     "node-remote",
		StreamConsoleOutputChannel: make(chan node.ConsoleChunk),
	}
	registry := noderegistry.New("node-local", nil)
	registry.Register(client)
	conn := newTestConnection()
	conn.outputStreamChannel = make(chan *xylona.Message)
	token := &struct{}{}
	conn.consoleStreamTokens = map[string]*struct{}{"server-remote": token}
	ctx, cancel := context.WithTimeout(t.Context(), 1100*time.Millisecond)
	defer cancel()

	ws := &WebSocket{ctx: ctx, nodeRegistry: registry}
	ws.runConsoleStream(ctx, conn, "server-remote", "node-remote", token)

	logs := logOutput.String()
	for _, message := range []string{
		"Unable to deliver console connection state",
		"Console connection-state update was backpressured; will retry",
	} {
		if !strings.Contains(logs, message) {
			t.Fatalf("logs did not exercise %q: %s", message, logs)
		}
	}
	if strings.Contains(logs, `"level":"warn"`) {
		t.Fatalf("routine console backpressure logged a warning: %s", logs)
	}
}

func receiveConsoleMessage(
	t *testing.T,
	messages <-chan *xylona.Message,
	timeout time.Duration,
) *xylona.GameServerConsoleOutput {
	t.Helper()
	select {
	case message := <-messages:
		if message == nil || message.GetType() != xylona.Message_GameServerConsole {
			t.Fatalf("console message = %+v", message)
		}
		output := message.GetGameServerConsoleOutput()
		if output == nil {
			t.Fatal("console output payload = nil")
		}
		return output
	case <-time.After(timeout):
		t.Fatal("timed out waiting for console message")
		return nil
	}
}

type contextCapturingNodeClient struct {
	*nodeclient.FakeNodeClient
	contexts chan context.Context
}

func (c *contextCapturingNodeClient) StreamConsoleOutput(ctx context.Context, processID string) (<-chan node.ConsoleChunk, error) {
	c.contexts <- ctx
	stream, errStream := c.FakeNodeClient.StreamConsoleOutput(ctx, processID)
	if errStream != nil {
		return nil, fmt.Errorf("test stream console output: %w", errStream)
	}
	return stream, nil
}

func TestWebSocket_CancelConsoleStreamsCancelsSelfNodeClientStream(t *testing.T) {
	stream := make(chan node.ConsoleChunk)
	localClient := &contextCapturingNodeClient{
		FakeNodeClient: &nodeclient.FakeNodeClient{
			NodeID:                     "node-local",
			StreamConsoleOutputChannel: stream,
		},
		contexts: make(chan context.Context, 1),
	}
	registry := noderegistry.New("node-local", localClient)

	conn := newTestConnection()
	conn.outputStreamChannel = make(chan *xylona.Message, 1)

	ws := &WebSocket{
		ctx:          context.Background(),
		nodeRegistry: registry,
	}

	errSubscribe := ws.subscribeGameServerConsole(conn, &models.GameServer{
		ID:     "server-local",
		NodeID: "node-local",
	})
	if errSubscribe != nil {
		t.Fatalf("subscribeGameServerConsole() error = %v", errSubscribe)
	}

	var streamCtx context.Context
	select {
	case streamCtx = <-localClient.contexts:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stream context")
	}

	ws.cancelConsoleStreams(conn)

	select {
	case <-streamCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("stream context was not canceled")
	}

	conn.RLock()
	remaining := len(conn.consoleStreamCancels)
	conn.RUnlock()
	if remaining != 0 {
		t.Fatalf("consoleStreamCancels len = %d, want 0", remaining)
	}
}

func TestWebSocket_ConsoleStreamStopsAfterPeriodicPermissionRevocationCheck(t *testing.T) {
	stream := make(chan node.ConsoleChunk, 1)
	localClient := &contextCapturingNodeClient{
		FakeNodeClient: &nodeclient.FakeNodeClient{
			NodeID:                     "node-local",
			StreamConsoleOutputChannel: stream,
		},
		contexts: make(chan context.Context, 1),
	}
	registry := noderegistry.New("node-local", localClient)
	conn := newTestConnection()
	conn.outputStreamChannel = make(chan *xylona.Message, 2)
	conn.requestedGameServerOutputIDs["server-local"] = struct{}{}
	var allowed atomic.Bool
	allowed.Store(true)
	conn.consolePermissionLookup = func(string) (bool, error) {
		return allowed.Load(), nil
	}
	ws := &WebSocket{
		ctx:                       t.Context(),
		nodeRegistry:              registry,
		consolePermissionInterval: 10 * time.Millisecond,
	}

	errSubscribe := ws.startConsoleStream(conn, &models.GameServer{ID: "server-local", NodeID: "node-local"})
	if errSubscribe != nil {
		t.Fatalf("startConsoleStream() error = %v", errSubscribe)
	}
	connected := receiveConsoleMessage(t, conn.outputStreamChannel, time.Second)
	if connected.Reconnecting == nil || connected.GetReconnecting() {
		t.Fatalf("initial connection state = %+v, want connected", connected)
	}

	var streamCtx context.Context
	select {
	case streamCtx = <-localClient.contexts:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stream context")
	}
	allowed.Store(false)

	select {
	case <-streamCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("console stream was not canceled after permission revocation")
	}
	conn.RLock()
	_, subscribed := conn.requestedGameServerOutputIDs["server-local"]
	conn.RUnlock()
	if subscribed {
		t.Fatal("console subscription remained after permission revocation")
	}
	select {
	case message := <-conn.outputStreamChannel:
		t.Fatalf("received console output after permission revocation: %+v", message)
	default:
	}
}
