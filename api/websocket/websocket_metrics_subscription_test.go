package websocket

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/noderegistry"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func newTestConnection() *connection {
	return &connection{
		id:                           uuid.New(),
		requestedGameServerOutputIDs: make(map[string]struct{}),
		subscribedMetricsServerIDs:   make(map[string]struct{}),
		consoleStreamCancels:         make(map[string]context.CancelFunc),
		RWMutex:                      &sync.RWMutex{},
	}
}

func TestConnection_ShouldReceiveMetrics(t *testing.T) {
	tests := []struct {
		name        string
		subscribed  []string
		queryServer string
		want        bool
	}{
		{
			name:        "subscribed server returns true",
			subscribed:  []string{"server-1"},
			queryServer: "server-1",
			want:        true,
		},
		{
			name:        "unsubscribed server returns false",
			subscribed:  []string{"server-1"},
			queryServer: "server-2",
			want:        false,
		},
		{
			name:        "no subscriptions returns false",
			subscribed:  nil,
			queryServer: "server-1",
			want:        false,
		},
		{
			name:        "multiple subscriptions returns true for subscribed",
			subscribed:  []string{"server-1", "server-2", "server-3"},
			queryServer: "server-2",
			want:        true,
		},
		{
			name:        "empty server ID returns false when not subscribed",
			subscribed:  []string{"server-1"},
			queryServer: "",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestConnection()
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

func TestConnection_ShouldReceiveMetrics_ConcurrentAccess(_ *testing.T) {
	c := newTestConnection()

	// Pre-populate a subscription
	c.subscribedMetricsServerIDs["server-1"] = struct{}{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			_ = c.shouldReceiveMetrics("server-1")
		}
	}()

	for range 100 {
		_ = c.shouldReceiveMetrics("server-1")
	}

	<-done
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
	conn.outputStreamChannel = make(chan *xylona.Message, 1)

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
		if msg.GetType() != xylona.Message_GameServerConsole {
			t.Fatalf("message type = %v, want %v", msg.GetType(), xylona.Message_GameServerConsole)
		}
		if msg.GetGameServerConsoleOutput().GetGameServerId() != "server-remote" {
			t.Fatalf("gameServerId = %q, want %q", msg.GetGameServerConsoleOutput().GetGameServerId(), "server-remote")
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
