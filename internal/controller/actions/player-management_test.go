package actions

import (
	"errors"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestGetPlayerManagement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		gameServer           func() *models.GameServer
		queryResult          node.GameServerQueryResult
		playerActions        bool
		wantActionsSupported bool
		wantActions          int
		wantPlayers          []node.GameServerPlayer
		wantReasonContains   string
		wantRuntimeCalls     int
	}{
		{
			name:                 "Minecraft exposes typed actions and stable names",
			gameServer:           minecraftPlayerTestServer,
			playerActions:        true,
			wantActionsSupported: true,
			wantActions:          5,
			wantPlayers:          []node.GameServerPlayer{{Name: "Alex", ID: "Alex"}},
			wantRuntimeCalls:     1,
			queryResult: node.GameServerQueryResult{
				Kind: node.GameServerQueryKindMinecraft,
				Minecraft: &node.MinecraftQueryInfo{
					PlayerDetails: []node.GameServerPlayer{{Name: "Alex", ID: "Alex"}},
				},
			},
		},
		{
			name:               "Source roster stays read only",
			gameServer:         sourcePlayerTestServer,
			wantPlayers:        []node.GameServerPlayer{{Name: "Gordon"}},
			wantReasonContains: "read-only",
			queryResult: node.GameServerQueryResult{
				Kind:   node.GameServerQueryKindSource,
				Source: &node.SourceQueryInfo{PlayerList: []string{"Gordon"}},
			},
		},
		{
			name:               "legacy node advertises no actions",
			gameServer:         minecraftPlayerTestServer,
			wantActions:        5,
			wantPlayers:        []node.GameServerPlayer{{Name: "Alex", ID: "Alex"}},
			wantReasonContains: "Upgrade the node",
			wantRuntimeCalls:   1,
			queryResult: node.GameServerQueryResult{
				Kind:      node.GameServerQueryKindMinecraft,
				Minecraft: &node.MinecraftQueryInfo{PlayerList: []string{"Alex"}},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := &nodeclient.FakeNodeClient{
				NodeID:                    "node-1",
				RuntimeCapabilitiesResult: node.RuntimeCapabilities{PlayerActions: tc.playerActions},
				GetProcessSnapshotResult:  &node.ProcessSnapshot{Status: xylona.Status_ONLINE.String()},
				GetProcessSnapshotFound:   true,
				QueryGameServerResult:     tc.queryResult,
			}
			inst := &Instance{embeddedNodeClient: client}

			management, errManagement := inst.GetPlayerManagement(t.Context(), tc.gameServer())
			if errManagement != nil {
				t.Fatalf("GetPlayerManagement() error = %v", errManagement)
			}
			if management.ActionsSupported != tc.wantActionsSupported {
				t.Fatalf("ActionsSupported = %t, want %t", management.ActionsSupported, tc.wantActionsSupported)
			}
			if len(management.SupportedActions) != tc.wantActions {
				t.Fatalf("supported actions = %v, want %d", management.SupportedActions, tc.wantActions)
			}
			if len(management.Players) != len(tc.wantPlayers) {
				t.Fatalf("players = %+v, want %+v", management.Players, tc.wantPlayers)
			}
			for index := range tc.wantPlayers {
				if management.Players[index] != tc.wantPlayers[index] {
					t.Fatalf("players = %+v, want %+v", management.Players, tc.wantPlayers)
				}
			}
			if tc.wantReasonContains != "" && !strings.Contains(management.UnavailableReason, tc.wantReasonContains) {
				t.Fatalf("unavailable reason = %q, want containing %q", management.UnavailableReason, tc.wantReasonContains)
			}
			if client.RuntimeCapabilitiesCalls != tc.wantRuntimeCalls {
				t.Fatalf("runtime capability calls = %d, want %d", client.RuntimeCapabilitiesCalls, tc.wantRuntimeCalls)
			}
		})
	}
}

func TestPerformPlayerAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		gameServer    func() *models.GameServer
		action        node.GameServerPlayerAction
		playerActions bool
		wantErr       error
		wantCall      bool
	}{
		{
			name:          "dispatches supported Minecraft action",
			gameServer:    minecraftPlayerTestServer,
			action:        node.GameServerPlayerActionBan,
			playerActions: true,
			wantCall:      true,
		},
		{
			name:       "rejects Source action",
			gameServer: sourcePlayerTestServer,
			action:     node.GameServerPlayerActionKick,
			wantErr:    node.ErrPlayerActionUnsupported,
		},
		{
			name:       "rejects legacy node",
			gameServer: minecraftPlayerTestServer,
			action:     node.GameServerPlayerActionKick,
			wantErr:    node.ErrPlayerActionUnsupported,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := &nodeclient.FakeNodeClient{
				NodeID:                    "node-1",
				RuntimeCapabilitiesResult: node.RuntimeCapabilities{PlayerActions: tc.playerActions},
			}
			inst := &Instance{embeddedNodeClient: client}
			errAction := inst.PerformPlayerAction(t.Context(), tc.gameServer(), tc.action, " Alex ", " Abuse ")
			if tc.wantErr != nil {
				if !errors.Is(errAction, tc.wantErr) {
					t.Fatalf("PerformPlayerAction() error = %v, want %v", errAction, tc.wantErr)
				}
				return
			}
			if errAction != nil {
				t.Fatalf("PerformPlayerAction() error = %v", errAction)
			}
			if tc.wantCall != (len(client.PerformGameServerPlayerActionCalls) == 1) {
				t.Fatalf("action calls = %+v", client.PerformGameServerPlayerActionCalls)
			}
			if tc.wantCall {
				request := client.PerformGameServerPlayerActionCalls[0]
				if request.Kind != node.GameServerQueryKindMinecraft || request.Action != tc.action || request.PlayerID != "Alex" || request.Reason != "Abuse" {
					t.Fatalf("action request = %+v", request)
				}
			}
		})
	}
}

func minecraftPlayerTestServer() *models.GameServer {
	return &models.GameServer{
		ID:         "server-1",
		GameID:     minecraftGameID,
		NodeID:     "node-1",
		Status:     xylona.Status_ONLINE.String(),
		IP:         "127.0.0.1",
		QueryPort:  25565,
		MaxPlayers: 20,
	}
}

func sourcePlayerTestServer() *models.GameServer {
	gameServer := &models.GameServer{
		ID:         "server-1",
		GameID:     "source-game",
		NodeID:     "node-1",
		Status:     xylona.Status_ONLINE.String(),
		IP:         "127.0.0.1",
		QueryPort:  27015,
		MaxPlayers: 20,
	}
	gameServer.R.Game = &models.Game{UsesSourceQuery: true}
	return gameServer
}
