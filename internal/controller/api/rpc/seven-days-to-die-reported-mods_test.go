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
	t.Run("rejects non seven days to die servers", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		request := connect.NewRequest(&xylona.GetSevenDaysToDieReportedModsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		_, errMods := fixture.service.GetSevenDaysToDieReportedMods(t.Context(), request)
		if connect.CodeOf(errMods) != connect.CodeFailedPrecondition {
			t.Fatalf("GetSevenDaysToDieReportedMods() code = %v, want %v", connect.CodeOf(errMods), connect.CodeFailedPrecondition)
		}
	})

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

	t.Run("reports a missing live process as offline", func(t *testing.T) {
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
		if len(client.GetProcessSnapshotCalls) != 1 || len(client.QuerySevenDaysToDieReportedModsCalls) != 0 {
			t.Fatalf("process calls = %v, reported-mod query calls = %d", client.GetProcessSnapshotCalls, len(client.QuerySevenDaysToDieReportedModsCalls))
		}
	})

	t.Run("routes to the owning node without mutating managed mods", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
		insertRemoteNodeForParityTests(t, fixture, "node-remote")
		insertNodeScopedIPForParityTests(t, fixture, "node-remote", "127.0.0.2")
		setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_OFFLINE.String(), "node-remote")

		localClient := &nodeclient.FakeNodeClient{NodeID: "node-local"}
		remoteClient := &nodeclient.FakeNodeClient{
			NodeID:                   "node-remote",
			GetProcessSnapshotResult: &node.ProcessSnapshot{Status: xylona.Status_ONLINE.String()},
			GetProcessSnapshotFound:  true,
			RuntimeCapabilitiesResult: node.RuntimeCapabilities{
				ProtocolVersion: node.RuntimeProtocolVersion,
			},
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

		_, errSeedMod := fixture.conn.SQLDb.ExecContext(t.Context(), `insert into installed_mod
			(id, game_server_id, source, source_id, mod_name, mod_author, installed_version, installed_version_id,
			 file_hash, auto_update, enabled, pinned_version, created_at, updated_at)
			values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"managed-mod-1", "server-local-1", "curseforge", "source-1", "Managed Mod", "Managed Author",
			"1.0.0", "version-1", "sha256:managed", 1, 0, "0.9.0", "2026-08-20 01:02:03", "2026-08-21 04:05:06")
		if errSeedMod != nil {
			t.Fatalf("seed managed mod: %v", errSeedMod)
		}
		const snapshotQuery = `select json_array(id, game_server_id, source, source_id, mod_name, mod_author,
			installed_version, installed_version_id, file_hash, auto_update, enabled, pinned_version, created_at, updated_at)
			from installed_mod where id = ?`
		var beforeContents string
		errBefore := fixture.conn.SQLDb.QueryRowContext(t.Context(), snapshotQuery, "managed-mod-1").Scan(&beforeContents)
		if errBefore != nil {
			t.Fatalf("read managed mod before query: %v", errBefore)
		}
		var beforeCount int
		errBeforeCount := fixture.conn.SQLDb.QueryRowContext(t.Context(), "select count(*) from installed_mod").Scan(&beforeCount)
		if errBeforeCount != nil {
			t.Fatalf("count managed mods before query: %v", errBeforeCount)
		}
		request := connect.NewRequest(&xylona.GetSevenDaysToDieReportedModsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
		response, errMods := fixture.service.GetSevenDaysToDieReportedMods(t.Context(), request)
		if errMods != nil {
			t.Fatalf("GetSevenDaysToDieReportedMods() error = %v", errMods)
		}
		var afterContents string
		errAfter := fixture.conn.SQLDb.QueryRowContext(t.Context(), snapshotQuery, "managed-mod-1").Scan(&afterContents)
		if errAfter != nil {
			t.Fatalf("read managed mod after query: %v", errAfter)
		}
		if afterContents != beforeContents {
			t.Fatalf("managed mod contents changed from %s to %s", beforeContents, afterContents)
		}
		var afterCount int
		errAfterCount := fixture.conn.SQLDb.QueryRowContext(t.Context(), "select count(*) from installed_mod").Scan(&afterCount)
		if errAfterCount != nil {
			t.Fatalf("count managed mods after query: %v", errAfterCount)
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

	states := []struct {
		name           string
		connection     node.SevenDaysToDieWebAPIConnectionState
		state          node.SevenDaysToDieWebAPIValueState
		wantConnection xylona.SevenDaysToDieWebAPIConnectionState
		wantState      xylona.SevenDaysToDieWebAPIValueState
	}{
		{
			name:           "unsupported",
			connection:     node.SevenDaysToDieWebAPIConnectionStateAvailable,
			state:          node.SevenDaysToDieWebAPIValueStateUnsupported,
			wantConnection: xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
			wantState:      xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED,
		},
		{
			name:           "permission denied",
			connection:     node.SevenDaysToDieWebAPIConnectionStateAuthenticationDenied,
			state:          node.SevenDaysToDieWebAPIValueStateUnavailable,
			wantConnection: xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AUTHENTICATION_DENIED,
			wantState:      xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
		},
		{
			name:           "unavailable",
			connection:     node.SevenDaysToDieWebAPIConnectionStateAvailable,
			state:          node.SevenDaysToDieWebAPIValueStateUnavailable,
			wantConnection: xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
			wantState:      xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
		},
	}

	invalidInventories := []struct {
		name string
		mods []node.SevenDaysToDieReportedMod
	}{
		{name: "over-count", mods: make([]node.SevenDaysToDieReportedMod, node.SevenDaysToDieReportedModCountLimit+1)},
		{name: "over-limit field", mods: []node.SevenDaysToDieReportedMod{{Description: strings.Repeat("x", node.SevenDaysToDieReportedModFieldByteLimit+1)}}},
	}
	for _, test := range invalidInventories {
		t.Run("rejects invalid "+test.name+" inventory", func(t *testing.T) {
			fixture, _ := newReportedModsRPCFixture(t, &node.SevenDaysToDieReportedMods{Mods: test.mods})
			request := connect.NewRequest(&xylona.GetSevenDaysToDieReportedModsRequest{GameServerId: "server-local-1"})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

			_, errMods := fixture.service.GetSevenDaysToDieReportedMods(t.Context(), request)
			if connect.CodeOf(errMods) != connect.CodeInternal {
				t.Fatalf("GetSevenDaysToDieReportedMods() code = %v, want %v", connect.CodeOf(errMods), connect.CodeInternal)
			}
		})
	}
	for _, test := range states {
		t.Run("projects "+test.name, func(t *testing.T) {
			fixture, _ := newReportedModsRPCFixture(t, &node.SevenDaysToDieReportedMods{
				ConnectionState: test.connection,
				State:           test.state,
			})
			request := connect.NewRequest(&xylona.GetSevenDaysToDieReportedModsRequest{GameServerId: "server-local-1"})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

			response, errMods := fixture.service.GetSevenDaysToDieReportedMods(t.Context(), request)
			if errMods != nil {
				t.Fatalf("GetSevenDaysToDieReportedMods() error = %v", errMods)
			}
			if response.Msg.GetConnectionState() != test.wantConnection || response.Msg.GetState() != test.wantState {
				t.Fatalf("response state = %+v", response.Msg)
			}
		})
	}

	t.Run("preserves parent cancellation", func(t *testing.T) {
		fixture, client := newReportedModsRPCFixture(t, nil)
		client.QuerySevenDaysToDieReportedModsFunc = func(ctx context.Context, _ node.SevenDaysToDieReportedModsQueryRequest) (*node.SevenDaysToDieReportedMods, error) {
			return nil, ctx.Err()
		}
		request := connect.NewRequest(&xylona.GetSevenDaysToDieReportedModsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, errMods := fixture.service.GetSevenDaysToDieReportedMods(ctx, request)
		if connect.CodeOf(errMods) != connect.CodeCanceled {
			t.Fatalf("GetSevenDaysToDieReportedMods() code = %v, want %v", connect.CodeOf(errMods), connect.CodeCanceled)
		}
	})

	legacyProtocols := []struct {
		name    string
		version int64
	}{
		{name: "protocol 9", version: sevenDaysToDiePrivateWebAPINodeProtocol - 1},
		{name: "unknown protocol", version: 0},
	}
	for _, legacyProtocol := range legacyProtocols {
		t.Run("gates "+legacyProtocol.name, func(t *testing.T) {
			fixture, client := newReportedModsRPCFixture(t, &node.SevenDaysToDieReportedMods{})
			client.RuntimeCapabilitiesResult.ProtocolVersion = legacyProtocol.version
			request := connect.NewRequest(&xylona.GetSevenDaysToDieReportedModsRequest{GameServerId: "server-local-1"})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

			response, errMods := fixture.service.GetSevenDaysToDieReportedMods(t.Context(), request)
			if errMods != nil {
				t.Fatalf("GetSevenDaysToDieReportedMods() error = %v", errMods)
			}
			if response.Msg.GetState() != xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED ||
				len(client.QuerySevenDaysToDieReportedModsCalls) != 0 {
				t.Fatalf("response = %+v, query calls = %d", response.Msg, len(client.QuerySevenDaysToDieReportedModsCalls))
			}
		})
	}
}

func newReportedModsRPCFixture(t *testing.T, result *node.SevenDaysToDieReportedMods) (*rbacRPCFixture, *nodeclient.FakeNodeClient) {
	t.Helper()
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_ONLINE.String(), "node-local")
	client := &nodeclient.FakeNodeClient{
		NodeID:                                "node-local",
		GetProcessSnapshotResult:              &node.ProcessSnapshot{Status: xylona.Status_ONLINE.String()},
		GetProcessSnapshotFound:               true,
		RuntimeCapabilitiesResult:             node.RuntimeCapabilities{ProtocolVersion: node.RuntimeProtocolVersion},
		QuerySevenDaysToDieReportedModsResult: result,
	}
	registry := noderegistry.New("node-local", client)
	fixture.service.nodeRegistry = registry
	actionsContext, cancelActions := context.WithCancel(t.Context())
	t.Cleanup(cancelActions)
	fixture.service.actionsInst = actions.NewInstance(
		actionsContext, fixture.conn, client, registry, nil,
		versiontracker.NewVersionStateMap(), versiontracker.ResolverConfig{},
	)
	return fixture, client
}
