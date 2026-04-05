package websocket

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func newTestConnection() *connection {
	return &connection{
		id:                           uuid.New(),
		requestedGameServerOutputIDs: make(map[string]struct{}),
		subscribedMetricsServerIDs:   make(map[string]struct{}),
		remoteConsoleCancels:         make(map[string]context.CancelFunc),
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
