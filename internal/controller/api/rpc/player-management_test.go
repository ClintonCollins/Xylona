package rpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"

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
		t.Fatalf("players = %+v, want Alex with stable ID", getResponse.Msg.GetPlayers())
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
