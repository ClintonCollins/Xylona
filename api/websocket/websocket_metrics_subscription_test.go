package websocket

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
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

func TestConnection_ShouldReceiveMetrics_ConcurrentAccess(t *testing.T) {
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
