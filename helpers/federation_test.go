package helpers

import (
	"testing"
	"time"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestNodeModelToProtoFederation(t *testing.T) {
	tests := []struct {
		name     string
		input    *models.Node
		wantID   string
		wantName string
		wantURL  string
	}{
		{
			name: "remote node",
			input: &models.Node{
				ID:              "node-123",
				Name:            "Test Peer",
				IsLocal:         false,
				BaseURL:         "http://192.168.1.100:8080",
				Enabled:         true,
				HealthStatus:    "healthy",
				Version:         "0.1.0",
				ProtocolVersion: 1,
				Capabilities:    "server_list,server_detail",
				CreatedAt:       null.From(time.Now()),
				UpdatedAt:       null.From(time.Now()),
			},
			wantID:   "node-123",
			wantName: "Test Peer",
			wantURL:  "http://192.168.1.100:8080",
		},
		{
			name: "remote node with empty optional fields",
			input: &models.Node{
				ID:           "node-empty",
				Name:         "",
				IsLocal:      false,
				BaseURL:      "http://localhost:9090",
				Enabled:      false,
				HealthStatus: "unknown",
				CreatedAt:    null.From(time.Now()),
				UpdatedAt:    null.From(time.Now()),
			},
			wantID:   "node-empty",
			wantName: "",
			wantURL:  "http://localhost:9090",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NodeModelToProto(tt.input)
			if got.Id != tt.wantID {
				t.Errorf("NodeModelToProto().Id = %v, want %v", got.Id, tt.wantID)
			}
			if got.Name != tt.wantName {
				t.Errorf("NodeModelToProto().Name = %v, want %v", got.Name, tt.wantName)
			}
			if got.BaseUrl != tt.wantURL {
				t.Errorf("NodeModelToProto().BaseUrl = %v, want %v", got.BaseUrl, tt.wantURL)
			}
			if got.Enabled != tt.input.Enabled {
				t.Errorf("NodeModelToProto().Enabled = %v, want %v", got.Enabled, tt.input.Enabled)
			}
			if got.HealthStatus != tt.input.HealthStatus {
				t.Errorf("NodeModelToProto().HealthStatus = %v, want %v", got.HealthStatus, tt.input.HealthStatus)
			}
		})
	}
}

func TestRemoteServerCacheModelToProto(t *testing.T) {
	tests := []struct {
		name       string
		input      *models.RemoteServerCache
		wantID     string
		wantName   string
		wantStatus xylona.Status
		wantStale  bool
	}{
		{
			name: "online remote server",
			input: &models.RemoteServerCache{
				ID:             "cache-1",
				SourceNodeID:   "node-abc",
				NodeID:         "node-123",
				RemoteServerID: "server-xyz",
				DisplayName:    "My Minecraft",
				Status:         "ONLINE",
				GameName:       "Minecraft",
				GameID:         "minecraft",
				IPAddress:      "10.0.0.5",
				Port:           25565,
				MaxPlayers:     32,
				CurrentPlayers: 5,
				MapName:        "world",
				Version:        "1.20.4",
				NodeName:       "Remote Node",
				NodeHost:       "192.168.1.100",
				IsStale:        false,
			},
			wantID:     "cache-1",
			wantName:   "My Minecraft",
			wantStatus: xylona.Status_ONLINE,
			wantStale:  false,
		},
		{
			name: "stale offline server",
			input: &models.RemoteServerCache{
				ID:             "cache-2",
				SourceNodeID:   "node-def",
				NodeID:         "node-456",
				RemoteServerID: "server-uvw",
				DisplayName:    "Stale Server",
				Status:         "OFFLINE",
				GameName:       "7 Days to Die",
				IsStale:        true,
			},
			wantID:     "cache-2",
			wantName:   "Stale Server",
			wantStatus: xylona.Status_OFFLINE,
			wantStale:  true,
		},
		{
			name: "unknown status",
			input: &models.RemoteServerCache{
				ID:             "cache-3",
				SourceNodeID:   "node-ghi",
				NodeID:         "node-789",
				RemoteServerID: "server-rst",
				DisplayName:    "Unknown Server",
				Status:         "SOMETHING_INVALID",
				IsStale:        false,
			},
			wantID:     "cache-3",
			wantName:   "Unknown Server",
			wantStatus: xylona.Status_UNKNOWN,
			wantStale:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RemoteServerCacheModelToProto(tt.input)
			if got.Id != tt.wantID {
				t.Errorf("RemoteServerCacheModelToProto().Id = %v, want %v", got.Id, tt.wantID)
			}
			if got.DisplayName != tt.wantName {
				t.Errorf("RemoteServerCacheModelToProto().DisplayName = %v, want %v", got.DisplayName, tt.wantName)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("RemoteServerCacheModelToProto().Status = %v, want %v", got.Status, tt.wantStatus)
			}
			if got.IsStale != tt.wantStale {
				t.Errorf("RemoteServerCacheModelToProto().IsStale = %v, want %v", got.IsStale, tt.wantStale)
			}
			if got.SourceNodeId != tt.input.SourceNodeID {
				t.Errorf("RemoteServerCacheModelToProto().SourceNodeId = %v, want %v", got.SourceNodeId, tt.input.SourceNodeID)
			}
		})
	}
}

func TestGameServerModelStatusToProtoStatus(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  xylona.Status
	}{
		{name: "online", input: "ONLINE", want: xylona.Status_ONLINE},
		{name: "offline", input: "OFFLINE", want: xylona.Status_OFFLINE},
		{name: "installing", input: "INSTALLING", want: xylona.Status_INSTALLING},
		{name: "updating", input: "UPDATING", want: xylona.Status_UPDATING},
		{name: "unknown", input: "UNKNOWN", want: xylona.Status_UNKNOWN},
		{name: "empty string", input: "", want: xylona.Status_UNKNOWN},
		{name: "invalid", input: "BOGUS", want: xylona.Status_UNKNOWN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GameServerModelStatusToProtoStatus(tt.input)
			if got != tt.want {
				t.Errorf("GameServerModelStatusToProtoStatus(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestRemoteServerCacheToProto(t *testing.T) {
	tests := []struct {
		name       string
		cache      *models.RemoteServerCache
		node       *models.Node
		wantID     string
		wantName   string
		wantStatus xylona.Status
		wantNodeID string
		wantPort   int64
	}{
		{
			name: "online remote server converts to GameServer proto",
			cache: &models.RemoteServerCache{
				RemoteServerID: "srv-abc",
				DisplayName:    "My MC Server",
				GameID:         "minecraft",
				GameName:       "Minecraft",
				Status:         "ONLINE",
				IPAddress:      "10.0.0.5",
				Port:           25565,
				QueryPort:      25566,
				MaxPlayers:     20,
				CurrentPlayers: 8,
				MapName:        "world",
				Version:        "1.21.0",
			},
			node: &models.Node{
				ID:      "node-remote-1",
				Name:    "Remote Node 1",
				BaseURL: "http://192.168.1.50:8080",
			},
			wantID:     "srv-abc",
			wantName:   "My MC Server",
			wantStatus: xylona.Status_ONLINE,
			wantNodeID: "node-remote-1",
			wantPort:   25565,
		},
		{
			name: "offline remote server",
			cache: &models.RemoteServerCache{
				RemoteServerID: "srv-xyz",
				DisplayName:    "Offline Server",
				GameID:         "csgo",
				GameName:       "CS:GO",
				Status:         "OFFLINE",
				IPAddress:      "10.0.0.10",
				Port:           27015,
				MaxPlayers:     32,
				CurrentPlayers: 0,
			},
			node: &models.Node{
				ID:      "node-remote-2",
				Name:    "Remote Node 2",
				BaseURL: "http://10.0.0.1:8080",
			},
			wantID:     "srv-xyz",
			wantName:   "Offline Server",
			wantStatus: xylona.Status_OFFLINE,
			wantNodeID: "node-remote-2",
			wantPort:   27015,
		},
		{
			name: "unknown status remote server",
			cache: &models.RemoteServerCache{
				RemoteServerID: "srv-unknown",
				DisplayName:    "Unknown Status",
				Status:         "INVALID_STATUS",
			},
			node: &models.Node{
				ID:      "node-3",
				Name:    "Node 3",
				BaseURL: "http://node3:8080",
			},
			wantID:     "srv-unknown",
			wantName:   "Unknown Status",
			wantStatus: xylona.Status_UNKNOWN,
			wantNodeID: "node-3",
			wantPort:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RemoteServerCacheToProto(tt.cache, tt.node)
			if got.Id != tt.wantID {
				t.Errorf("RemoteServerCacheToProto().Id = %v, want %v", got.Id, tt.wantID)
			}
			if got.Name != tt.wantName {
				t.Errorf("RemoteServerCacheToProto().Name = %v, want %v", got.Name, tt.wantName)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("RemoteServerCacheToProto().Status = %v, want %v", got.Status, tt.wantStatus)
			}
			if got.NodeId != tt.wantNodeID {
				t.Errorf("RemoteServerCacheToProto().NodeId = %v, want %v", got.NodeId, tt.wantNodeID)
			}
			if got.Port != tt.wantPort {
				t.Errorf("RemoteServerCacheToProto().Port = %v, want %v", got.Port, tt.wantPort)
			}
			if got.NodeName != tt.node.Name {
				t.Errorf("RemoteServerCacheToProto().NodeName = %v, want %v", got.NodeName, tt.node.Name)
			}
			if got.NodeHost != tt.node.BaseURL {
				t.Errorf("RemoteServerCacheToProto().NodeHost = %v, want %v", got.NodeHost, tt.node.BaseURL)
			}
			if got.GameId != tt.cache.GameID {
				t.Errorf("RemoteServerCacheToProto().GameId = %v, want %v", got.GameId, tt.cache.GameID)
			}
			if got.GameName != tt.cache.GameName {
				t.Errorf("RemoteServerCacheToProto().GameName = %v, want %v", got.GameName, tt.cache.GameName)
			}
			if got.Ip == nil {
				t.Fatalf("RemoteServerCacheToProto().Ip should not be nil")
			}
			if got.Ip.Address != tt.cache.IPAddress {
				t.Errorf("RemoteServerCacheToProto().Ip.Address = %v, want %v", got.Ip.Address, tt.cache.IPAddress)
			}
			if got.MaxPlayers != int64(tt.cache.MaxPlayers) {
				t.Errorf("RemoteServerCacheToProto().MaxPlayers = %v, want %v", got.MaxPlayers, tt.cache.MaxPlayers)
			}
			if got.CurrentPlayerCount != int64(tt.cache.CurrentPlayers) {
				t.Errorf("RemoteServerCacheToProto().CurrentPlayerCount = %v, want %v", got.CurrentPlayerCount, tt.cache.CurrentPlayers)
			}
		})
	}
}

func TestNodeModelToProtoTimestamps(t *testing.T) {
	now := time.Now()
	input := &models.Node{
		ID:         "ts-test",
		IsLocal:    false,
		BaseURL:    "http://test:8080",
		LastSeenAt: null.From(now),
		LastSyncAt: null.From(now),
		CreatedAt:  null.From(now),
		UpdatedAt:  null.From(now),
	}

	got := NodeModelToProto(input)

	if got.LastSeenAt == nil {
		t.Fatalf("NodeModelToProto().LastSeenAt should not be nil")
	}
	if got.LastSyncAt == nil {
		t.Fatalf("NodeModelToProto().LastSyncAt should not be nil")
	}
	if got.CreatedAt == nil {
		t.Fatalf("NodeModelToProto().CreatedAt should not be nil")
	}

	gotLastSeen := got.LastSeenAt.AsTime()
	if gotLastSeen.Unix() != now.Unix() {
		t.Errorf("LastSeenAt = %v, want %v", gotLastSeen, now)
	}
}
