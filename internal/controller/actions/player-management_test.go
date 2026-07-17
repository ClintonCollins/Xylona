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
		protocolVersion      int64
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
			name:               "older player-action protocol cannot expose expanded actions",
			gameServer:         hytalePlayerTestServer,
			playerActions:      true,
			protocolVersion:    expandedPlayerActionsProtocolVersion - 1,
			wantActions:        5,
			wantReasonContains: "Upgrade the node",
			wantRuntimeCalls:   1,
		},
		{
			name:                 "managed Factorio definition exposes actions",
			gameServer:           managedFactorioPlayerTestServer,
			playerActions:        true,
			protocolVersion:      expandedPlayerActionsProtocolVersion,
			wantActionsSupported: true,
			wantActions:          5,
			wantRuntimeCalls:     1,
		},
		{
			name:               "legacy Factorio definition does not expose remote actions",
			gameServer:         legacyFactorioPlayerTestServer,
			playerActions:      true,
			protocolVersion:    expandedPlayerActionsProtocolVersion,
			wantActions:        5,
			wantReasonContains: "game definition",
			wantRuntimeCalls:   0,
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
				NodeID: "node-1",
				RuntimeCapabilitiesResult: node.RuntimeCapabilities{
					ProtocolVersion: tc.protocolVersion,
					PlayerActions:   tc.playerActions,
				},
				GetProcessSnapshotResult: &node.ProcessSnapshot{Status: xylona.Status_ONLINE.String()},
				GetProcessSnapshotFound:  true,
				QueryGameServerResult:    tc.queryResult,
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
		name             string
		gameServer       func() *models.GameServer
		action           node.GameServerPlayerAction
		playerActions    bool
		protocolVersion  int64
		wantErr          error
		wantCall         bool
		wantKind         node.GameServerQueryKind
		wantRuntimeCalls int
	}{
		{
			name:             "dispatches supported Minecraft action",
			gameServer:       minecraftPlayerTestServer,
			action:           node.GameServerPlayerActionBan,
			playerActions:    true,
			wantCall:         true,
			wantKind:         node.GameServerQueryKindMinecraft,
			wantRuntimeCalls: 1,
		},
		{
			name:             "dispatches action for managed Factorio definition",
			gameServer:       managedFactorioPlayerTestServer,
			action:           node.GameServerPlayerActionKick,
			playerActions:    true,
			protocolVersion:  expandedPlayerActionsProtocolVersion,
			wantCall:         true,
			wantKind:         node.GameServerQueryKindFactorio,
			wantRuntimeCalls: 1,
		},
		{
			name:            "rejects action for legacy Factorio definition",
			gameServer:      legacyFactorioPlayerTestServer,
			action:          node.GameServerPlayerActionKick,
			playerActions:   true,
			protocolVersion: expandedPlayerActionsProtocolVersion,
			wantErr:         node.ErrPlayerActionUnsupported,
		},
		{
			name:             "rejects expanded action on older protocol",
			gameServer:       hytalePlayerTestServer,
			action:           node.GameServerPlayerActionKick,
			playerActions:    true,
			protocolVersion:  expandedPlayerActionsProtocolVersion - 1,
			wantErr:          node.ErrPlayerActionUnsupported,
			wantRuntimeCalls: 1,
		},
		{
			name:       "rejects Source action",
			gameServer: sourcePlayerTestServer,
			action:     node.GameServerPlayerActionKick,
			wantErr:    node.ErrPlayerActionUnsupported,
		},
		{
			name:             "rejects legacy node",
			gameServer:       minecraftPlayerTestServer,
			action:           node.GameServerPlayerActionKick,
			wantErr:          node.ErrPlayerActionUnsupported,
			wantRuntimeCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := &nodeclient.FakeNodeClient{
				NodeID: "node-1",
				RuntimeCapabilitiesResult: node.RuntimeCapabilities{
					ProtocolVersion: tc.protocolVersion,
					PlayerActions:   tc.playerActions,
				},
			}
			inst := &Instance{embeddedNodeClient: client}
			errAction := inst.PerformPlayerAction(t.Context(), tc.gameServer(), tc.action, " Alex ", " Abuse ")
			if tc.wantErr != nil {
				if !errors.Is(errAction, tc.wantErr) {
					t.Fatalf("PerformPlayerAction() error = %v, want %v", errAction, tc.wantErr)
				}
			} else if errAction != nil {
				t.Fatalf("PerformPlayerAction() error = %v", errAction)
			}
			if client.RuntimeCapabilitiesCalls != tc.wantRuntimeCalls {
				t.Fatalf("runtime capability calls = %d, want %d", client.RuntimeCapabilitiesCalls, tc.wantRuntimeCalls)
			}
			if tc.wantCall != (len(client.PerformGameServerPlayerActionCalls) == 1) {
				t.Fatalf("action calls = %+v", client.PerformGameServerPlayerActionCalls)
			}
			if tc.wantCall {
				request := client.PerformGameServerPlayerActionCalls[0]
				if request.Kind != tc.wantKind || request.Action != tc.action || request.PlayerID != "Alex" || request.Reason != "Abuse" {
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

func hytalePlayerTestServer() *models.GameServer {
	return &models.GameServer{
		ID:     "server-1",
		GameID: hytaleGameID,
		NodeID: "node-1",
		Status: xylona.Status_ONLINE.String(),
		IP:     "127.0.0.1",
		Port:   5520,
	}
}

func managedFactorioPlayerTestServer() *models.GameServer {
	gameServer := factorioPlayerTestServer()
	gameServer.R.Game = adminInputTestGameDefinition(factorioGameID)
	return gameServer
}

func legacyFactorioPlayerTestServer() *models.GameServer {
	gameServer := factorioPlayerTestServer()
	gameServer.R.Game = &models.Game{ID: factorioGameID, LinuxSupport: true}
	return gameServer
}

func factorioPlayerTestServer() *models.GameServer {
	return &models.GameServer{
		ID:        "server-1",
		GameID:    factorioGameID,
		NodeID:    "node-1",
		Status:    xylona.Status_ONLINE.String(),
		IP:        "127.0.0.1",
		QueryPort: 34197,
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

func TestPlayerManagementProfilesCoverDocumentedAdminConsoles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		gameID      string
		queryKind   node.GameServerQueryKind
		actionKind  node.GameServerQueryKind
		actionCount int
	}{
		{gameID: minecraftGameID, queryKind: node.GameServerQueryKindMinecraft, actionKind: node.GameServerQueryKindMinecraft, actionCount: 5},
		{gameID: palworldGameID, queryKind: node.GameServerQueryKindPalworld, actionKind: node.GameServerQueryKindPalworld, actionCount: 3},
		{gameID: sevenDaysToDieGameID, queryKind: node.GameServerQueryKindSource, actionKind: node.GameServerQueryKindSevenDaysToDie, actionCount: 5},
		{gameID: factorioGameID, actionKind: node.GameServerQueryKindFactorio, actionCount: 5},
		{gameID: hytaleGameID, actionKind: node.GameServerQueryKindHytale, actionCount: 5},
		{gameID: projectZomboidGameID, queryKind: node.GameServerQueryKindSource, actionKind: node.GameServerQueryKindProjectZomboid, actionCount: 4},
		{gameID: terrariaGameID, actionKind: node.GameServerQueryKindTerraria, actionCount: 2},
		{gameID: counterStrikeTwoGameID, queryKind: node.GameServerQueryKindSource, actionKind: node.GameServerQueryKindSourceRCON, actionCount: 3},
		{gameID: garrysModGameID, queryKind: node.GameServerQueryKindSource, actionKind: node.GameServerQueryKindSourceRCON, actionCount: 3},
		{gameID: teamFortressTwoGameID, queryKind: node.GameServerQueryKindSource, actionKind: node.GameServerQueryKindSourceRCON, actionCount: 3},
		{gameID: rustGameID, queryKind: node.GameServerQueryKindSource, actionKind: node.GameServerQueryKindRust, actionCount: 3},
	}

	for _, tc := range tests {
		t.Run(tc.gameID, func(t *testing.T) {
			t.Parallel()
			profile := playerManagementProfileForServer(&models.GameServer{GameID: tc.gameID})
			if profile.queryKind != tc.queryKind || profile.actionKind != tc.actionKind ||
				len(profile.supportedActions) != tc.actionCount {
				t.Fatalf("profile = %+v", profile)
			}
		})
	}
}
