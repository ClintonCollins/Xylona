package rpc

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestPlayerManagementAuthorizationAndDispatch(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	client := &nodeclient.FakeNodeClient{
		NodeID:                    "node-local",
		RuntimeCapabilitiesResult: node.RuntimeCapabilities{PlayerActions: true},
		GetProcessSnapshotResult:  &node.ProcessSnapshot{Status: xylona.Status_ONLINE.String()},
		GetProcessSnapshotFound:   true,
		QueryGameServerResult: node.GameServerQueryResult{
			Kind: node.GameServerQueryKindMinecraft,
			Minecraft: &node.MinecraftQueryInfo{
				PlayerDetails: []node.GameServerPlayer{{Name: "Alex", ID: "Alex"}},
			},
		},
	}
	actionsContext, cancelActions := context.WithCancel(t.Context())
	fixture.service.actionsInst = actions.NewInstance(
		actionsContext,
		fixture.conn,
		client,
		nil,
		nil,
		versiontracker.NewVersionStateMap(),
		versiontracker.ResolverConfig{},
	)
	t.Cleanup(cancelActions)

	getRequest := connect.NewRequest(&xylona.GetGameServerPlayerManagementRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getRequest, "user-owner")
	getResponse, errGet := fixture.service.GetGameServerPlayerManagement(t.Context(), getRequest)
	if errGet != nil {
		t.Fatalf("GetGameServerPlayerManagement() error = %v", errGet)
	}
	if len(getResponse.Msg.GetPlayers()) != 1 || getResponse.Msg.GetPlayers()[0].GetId() != "Alex" {
		t.Fatalf("legacy players = %+v, want Alex with stable ID", getResponse.Msg.GetPlayers())
	}
	if len(getResponse.Msg.GetManagementPlayers()) != 1 || getResponse.Msg.GetManagementPlayers()[0].GetActionIdentifier() != "Alex" {
		t.Fatalf("management players = %+v, want Alex with stable ID", getResponse.Msg.GetManagementPlayers())
	}
	if !getResponse.Msg.GetCapabilities().GetActionsSupported() || len(getResponse.Msg.GetCapabilities().GetSupportedActions()) != 5 {
		t.Fatalf("capabilities = %+v", getResponse.Msg.GetCapabilities())
	}

	reason := "Repeated abuse"
	actionRequest := connect.NewRequest(&xylona.PerformGameServerPlayerActionRequest{
		GameServerId: "server-local-1",
		Action:       xylona.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_BAN,
		PlayerId:     "Alex",
		Reason:       &reason,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, actionRequest, "user-owner")
	_, errAction := fixture.service.PerformGameServerPlayerAction(t.Context(), actionRequest)
	if errAction != nil {
		t.Fatalf("PerformGameServerPlayerAction() error = %v", errAction)
	}
	if len(client.PerformGameServerPlayerActionCalls) != 1 {
		t.Fatalf("player action calls = %+v", client.PerformGameServerPlayerActionCalls)
	}
	dispatched := client.PerformGameServerPlayerActionCalls[0]
	if dispatched.Action != node.GameServerPlayerActionBan || dispatched.PlayerID != "Alex" || dispatched.Reason != reason {
		t.Fatalf("dispatched action = %+v", dispatched)
	}

	deniedRequest := connect.NewRequest(&xylona.GetGameServerPlayerManagementRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, deniedRequest, "user-other")
	_, errDenied := fixture.service.GetGameServerPlayerManagement(t.Context(), deniedRequest)
	if connect.CodeOf(errDenied) != connect.CodePermissionDenied {
		t.Fatalf("other user error = %v, want permission denied", errDenied)
	}
}

func TestSevenDaysToDiePlayerManagementUsesNativeRoster(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_ONLINE.String(), "node-local")
	falseValue := false
	zeroInt := int32(0)
	zeroFloat := float32(0)
	client := &nodeclient.FakeNodeClient{
		NodeID:                    "node-local",
		RuntimeCapabilitiesResult: node.RuntimeCapabilities{PlayerActions: true, ProtocolVersion: node.RuntimeProtocolVersion},
		GetProcessSnapshotResult:  &node.ProcessSnapshot{Status: xylona.Status_ONLINE.String()},
		GetProcessSnapshotFound:   true,
		QuerySevenDaysToDiePlayersResult: &node.SevenDaysToDiePlayers{
			ConnectionState: node.SevenDaysToDieWebAPIConnectionStateAvailable,
			State:           node.SevenDaysToDieWebAPIValueStateAvailable,
			Players: []node.SevenDaysToDiePlayer{{
				Name: "Player", ActionID: "Steam_1", EntityID: "7", PlatformID: "Steam_1", CrossPlatformID: "EOS_1",
				Online: &falseValue, Ping: &zeroInt, Level: &zeroInt, Health: &zeroInt, Stamina: &zeroFloat,
				Score: &zeroInt, Deaths: &zeroInt, ZombieKills: &zeroInt, PlayerKills: &zeroInt, Banned: &falseValue,
			}, {Name: "Visible without action ID"}},
		},
	}
	actionsContext, cancelActions := context.WithCancel(t.Context())
	fixture.service.actionsInst = actions.NewInstance(
		actionsContext,
		fixture.conn,
		client,
		nil,
		nil,
		versiontracker.NewVersionStateMap(),
		versiontracker.ResolverConfig{},
	)
	t.Cleanup(cancelActions)

	request := connect.NewRequest(&xylona.GetGameServerPlayerManagementRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
	response, errGet := fixture.service.GetGameServerPlayerManagement(t.Context(), request)
	if errGet != nil {
		t.Fatalf("GetGameServerPlayerManagement() error = %v", errGet)
	}
	if len(client.QuerySevenDaysToDiePlayersCalls) != 1 || len(client.QueryGameServerCalls) != 0 {
		t.Fatalf("native calls = %d, generic calls = %d", len(client.QuerySevenDaysToDiePlayersCalls), len(client.QueryGameServerCalls))
	}
	call := client.QuerySevenDaysToDiePlayersCalls[0]
	if call.WorkingDirectory != "/tmp/server-local-1" || call.TokenName == "" || call.TokenSecret == "" {
		t.Fatal("native player query did not receive the node-local directory and credentials")
	}
	if response.Msg.GetCapabilities().GetRosterState() != xylona.GameServerPlayerManagementRosterState_GAME_SERVER_PLAYER_MANAGEMENT_ROSTER_STATE_AVAILABLE ||
		!response.Msg.GetCapabilities().GetActionsSupported() {
		t.Fatalf("capabilities = %+v", response.Msg.GetCapabilities())
	}
	if len(response.Msg.GetPlayers()) != 2 || response.Msg.GetPlayers()[0].GetId() != "" {
		t.Fatalf("legacy players = %+v, want names without native action IDs", response.Msg.GetPlayers())
	}
	players := response.Msg.GetManagementPlayers()
	if len(players) != 2 || players[0].GetActionIdentifier() != "Steam_1" || players[1].GetActionIdentifier() != "" {
		t.Fatalf("players = %+v", players)
	}
	player := players[0]
	if player.GetEntityId() != "7" || player.GetPlatformId() != "Steam_1" || player.GetCrossPlatformId() != "EOS_1" ||
		player.Online == nil || player.GetOnline() || player.Ping == nil || player.GetPing() != 0 ||
		player.Stamina == nil || player.GetStamina() != 0 || player.Banned == nil || player.GetBanned() {
		t.Fatalf("mapped player = %+v", player)
	}
	responseJSON, errMarshal := protojson.Marshal(response.Msg)
	if errMarshal != nil {
		t.Fatalf("marshal response: %v", errMarshal)
	}
	if strings.Contains(string(responseJSON), call.TokenName) || strings.Contains(string(responseJSON), call.TokenSecret) {
		t.Fatal("management response exposed WebAPI credentials")
	}
}

func TestSevenDaysToDiePlayerManagementPreservesManualActionsWhenRosterFails(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_ONLINE.String(), "node-local")
	client := &nodeclient.FakeNodeClient{
		NodeID:                    "node-local",
		RuntimeCapabilitiesResult: node.RuntimeCapabilities{PlayerActions: true, ProtocolVersion: node.RuntimeProtocolVersion},
		GetProcessSnapshotResult:  &node.ProcessSnapshot{Status: xylona.Status_ONLINE.String()},
		GetProcessSnapshotFound:   true,
		QuerySevenDaysToDiePlayersResult: &node.SevenDaysToDiePlayers{
			ConnectionState: node.SevenDaysToDieWebAPIConnectionStateAuthenticationDenied,
			State:           node.SevenDaysToDieWebAPIValueStateUnavailable,
		},
	}
	actionsContext, cancelActions := context.WithCancel(t.Context())
	fixture.service.actionsInst = actions.NewInstance(
		actionsContext, fixture.conn, client, nil, nil, versiontracker.NewVersionStateMap(), versiontracker.ResolverConfig{},
	)
	t.Cleanup(cancelActions)
	request := connect.NewRequest(&xylona.GetGameServerPlayerManagementRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
	response, errGet := fixture.service.GetGameServerPlayerManagement(t.Context(), request)
	if errGet != nil {
		t.Fatalf("GetGameServerPlayerManagement() error = %v", errGet)
	}
	capabilities := response.Msg.GetCapabilities()
	if !capabilities.GetActionsSupported() || capabilities.GetRosterState() != xylona.GameServerPlayerManagementRosterState_GAME_SERVER_PLAYER_MANAGEMENT_ROSTER_STATE_PERMISSION_DENIED {
		t.Fatalf("capabilities = %+v", capabilities)
	}
}

func TestSevenDaysToDiePlayerManagementGatesLegacyNodeRoster(t *testing.T) {
	tests := []struct {
		name            string
		protocolVersion int64
	}{
		{name: "protocol 9", protocolVersion: sevenDaysToDiePrivateWebAPINodeProtocol - 1},
		{name: "unknown protocol", protocolVersion: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)
			setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_ONLINE.String(), "node-local")
			client := &nodeclient.FakeNodeClient{
				NodeID: "node-local",
				RuntimeCapabilitiesResult: node.RuntimeCapabilities{
					PlayerActions:   true,
					ProtocolVersion: test.protocolVersion,
				},
				GetProcessSnapshotResult: &node.ProcessSnapshot{Status: xylona.Status_ONLINE.String()},
				GetProcessSnapshotFound:  true,
			}
			actionsContext, cancelActions := context.WithCancel(t.Context())
			t.Cleanup(cancelActions)
			fixture.service.actionsInst = actions.NewInstance(
				actionsContext, fixture.conn, client, nil, nil,
				versiontracker.NewVersionStateMap(), versiontracker.ResolverConfig{},
			)
			request := connect.NewRequest(&xylona.GetGameServerPlayerManagementRequest{GameServerId: "server-local-1"})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

			response, errGet := fixture.service.GetGameServerPlayerManagement(t.Context(), request)
			if errGet != nil {
				t.Fatalf("GetGameServerPlayerManagement() error = %v", errGet)
			}
			capabilities := response.Msg.GetCapabilities()
			if capabilities.GetRosterState() != xylona.GameServerPlayerManagementRosterState_GAME_SERVER_PLAYER_MANAGEMENT_ROSTER_STATE_UNSUPPORTED {
				t.Fatalf("capabilities = %+v, want unsupported roster", capabilities)
			}
			if len(client.QuerySevenDaysToDiePlayersCalls) != 0 {
				t.Fatalf("native roster calls = %d, want 0", len(client.QuerySevenDaysToDiePlayersCalls))
			}
		})
	}
}

func TestPlayerManagementPermissionMigration(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	var permissionCount int
	errPermission := fixture.conn.SQLDb.QueryRowContext(
		t.Context(),
		`select count(*) from permission where id = ?`,
		permissionPlayerManage,
	).Scan(&permissionCount)
	if errPermission != nil {
		t.Fatalf("query player-management permission: %v", errPermission)
	}
	if permissionCount != 1 {
		t.Fatalf("permission count = %d, want 1", permissionCount)
	}

	var roleCount int
	errRoles := fixture.conn.SQLDb.QueryRowContext(
		t.Context(),
		`select count(*) from role_permission where permission_id = ? and role_id in ('operator', 'admin')`,
		permissionPlayerManage,
	).Scan(&roleCount)
	if errRoles != nil {
		t.Fatalf("query player-management role grants: %v", errRoles)
	}
	if roleCount != 2 {
		t.Fatalf("role grant count = %d, want 2", roleCount)
	}
}
