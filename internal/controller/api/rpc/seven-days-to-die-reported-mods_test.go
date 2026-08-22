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
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestListInstalledModsRequiresGameServerPermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	request := connect.NewRequest(&xylona.ListInstalledModsRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-other")

	_, errList := fixture.service.ListInstalledMods(t.Context(), request)
	if connect.CodeOf(errList) != connect.CodePermissionDenied {
		t.Fatalf("ListInstalledMods() code = %v, want %v", connect.CodeOf(errList), connect.CodePermissionDenied)
	}
}

func TestGetSevenDaysToDieReportedMods(t *testing.T) {
	t.Run("requires game server mod permission", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_ONLINE.String(), "node-local")
		client := &nodeclient.FakeNodeClient{NodeID: "node-local"}
		fixture.service.nodeRegistry = noderegistry.New("node-local", client)
		request := connect.NewRequest(&xylona.GetSevenDaysToDieReportedModsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-other")

		_, errMods := fixture.service.GetSevenDaysToDieReportedMods(t.Context(), request)
		if connect.CodeOf(errMods) != connect.CodePermissionDenied {
			t.Fatalf("GetSevenDaysToDieReportedMods() code = %v, want %v", connect.CodeOf(errMods), connect.CodePermissionDenied)
		}
		if len(client.QuerySevenDaysToDieReportedModsCalls) != 0 {
			t.Fatalf("node query call count = %d, want 0", len(client.QuerySevenDaysToDieReportedModsCalls))
		}
	})

	t.Run("reports an offline server without querying its node", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_OFFLINE.String(), "node-local")
		client := &nodeclient.FakeNodeClient{NodeID: "node-local"}
		fixture.service.nodeRegistry = noderegistry.New("node-local", client)
		request := connect.NewRequest(&xylona.GetSevenDaysToDieReportedModsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		response, errMods := fixture.service.GetSevenDaysToDieReportedMods(t.Context(), request)
		if errMods != nil {
			t.Fatalf("GetSevenDaysToDieReportedMods() error = %v", errMods)
		}
		if response.Msg.GetConnectionState() != xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE ||
			response.Msg.GetState() != xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE {
			t.Fatalf("response state = %+v", response.Msg)
		}
		if len(client.QuerySevenDaysToDieReportedModsCalls) != 0 {
			t.Fatalf("node query call count = %d, want 0", len(client.QuerySevenDaysToDieReportedModsCalls))
		}
	})

	t.Run("routes to the owning node without mutating managed mods", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
		insertRemoteNodeForParityTests(t, fixture, "node-remote")
		insertNodeScopedIPForParityTests(t, fixture, "node-remote", "127.0.0.2")
		setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_ONLINE.String(), "node-remote")

		localClient := &nodeclient.FakeNodeClient{NodeID: "node-local"}
		remoteClient := &nodeclient.FakeNodeClient{
			NodeID:                   "node-remote",
			GetProcessSnapshotResult: &node.ProcessSnapshot{Status: xylona.Status_ONLINE.String()},
			GetProcessSnapshotFound:  true,
			QuerySevenDaysToDieReportedModsResult: &node.SevenDaysToDieReportedMods{
				ConnectionState: node.SevenDaysToDieWebAPIConnectionStateAvailable,
				State:           node.SevenDaysToDieWebAPIValueStateAvailable,
				Mods: []node.SevenDaysToDieReportedMod{{
					Name: "example", DisplayName: "Example Mod", Description: "Description", Author: "Author", Version: "1.2.3",
				}},
			},
		}
		registry := noderegistry.New("node-local", localClient)
		registry.Register(remoteClient)
		fixture.service.nodeRegistry = registry
		actionsContext, cancelActions := context.WithCancel(t.Context())
		t.Cleanup(cancelActions)
		fixture.service.actionsInst = actions.NewInstance(
			actionsContext, fixture.conn, localClient, registry, nil,
			versiontracker.NewVersionStateMap(), versiontracker.ResolverConfig{},
		)

		var beforeCount int
		errBefore := fixture.conn.SQLDb.QueryRowContext(t.Context(), "select count(*) from installed_mod where game_server_id = ?", "server-local-1").Scan(&beforeCount)
		if errBefore != nil {
			t.Fatalf("count managed mods before query: %v", errBefore)
		}
		request := connect.NewRequest(&xylona.GetSevenDaysToDieReportedModsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
		response, errMods := fixture.service.GetSevenDaysToDieReportedMods(t.Context(), request)
		if errMods != nil {
			t.Fatalf("GetSevenDaysToDieReportedMods() error = %v", errMods)
		}
		var afterCount int
		errAfter := fixture.conn.SQLDb.QueryRowContext(t.Context(), "select count(*) from installed_mod where game_server_id = ?", "server-local-1").Scan(&afterCount)
		if errAfter != nil {
			t.Fatalf("count managed mods after query: %v", errAfter)
		}
		if afterCount != beforeCount {
			t.Fatalf("managed mod count = %d, want unchanged %d", afterCount, beforeCount)
		}

		if len(localClient.QuerySevenDaysToDieReportedModsCalls) != 0 || len(remoteClient.QuerySevenDaysToDieReportedModsCalls) != 1 {
			t.Fatalf("query routing: local = %d, remote = %d", len(localClient.QuerySevenDaysToDieReportedModsCalls), len(remoteClient.QuerySevenDaysToDieReportedModsCalls))
		}
		call := remoteClient.QuerySevenDaysToDieReportedModsCalls[0]
		if call.WorkingDirectory != "/tmp/server-local-1" || call.TokenName == "" || call.TokenSecret == "" {
			t.Fatal("reported mod query did not receive the node-local directory and credentials")
		}
		mods := response.Msg.GetMods()
		if response.Msg.GetState() != xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE ||
			len(mods) != 1 || mods[0].GetName() != "example" || mods[0].GetDisplayName() != "Example Mod" ||
			mods[0].GetDescription() != "Description" || mods[0].GetAuthor() != "Author" || mods[0].GetVersion() != "1.2.3" {
			t.Fatalf("reported mods response = %+v", response.Msg)
		}
		responseJSON, errMarshal := protojson.Marshal(response.Msg)
		if errMarshal != nil {
			t.Fatalf("marshal response: %v", errMarshal)
		}
		if strings.Contains(string(responseJSON), call.TokenName) || strings.Contains(string(responseJSON), call.TokenSecret) {
			t.Fatal("reported mods response exposed WebAPI credentials")
		}
	})
}
