package rpc

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestGetSevenDaysToDieSandboxSettings(t *testing.T) {
	t.Run("requires configuration permission before querying the node", func(t *testing.T) {
		fixture, client := newSandboxSettingsRPCFixture(t, &node.SevenDaysToDieSandboxSettings{})
		request := connect.NewRequest(&xylona.GetSevenDaysToDieSandboxSettingsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-other")

		_, errGet := fixture.service.GetSevenDaysToDieSandboxSettings(t.Context(), request)
		if connect.CodeOf(errGet) != connect.CodePermissionDenied {
			t.Fatalf("GetSevenDaysToDieSandboxSettings() code = %v, want %v", connect.CodeOf(errGet), connect.CodePermissionDenied)
		}
		if len(client.QuerySevenDaysToDieSandboxSettingsCalls) != 0 {
			t.Fatalf("node query call count = %d, want 0", len(client.QuerySevenDaysToDieSandboxSettingsCalls))
		}
	})

	t.Run("rejects other games", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		request := connect.NewRequest(&xylona.GetSevenDaysToDieSandboxSettingsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		_, errGet := fixture.service.GetSevenDaysToDieSandboxSettings(t.Context(), request)
		if connect.CodeOf(errGet) != connect.CodeFailedPrecondition {
			t.Fatalf("GetSevenDaysToDieSandboxSettings() code = %v, want %v", connect.CodeOf(errGet), connect.CodeFailedPrecondition)
		}
	})

	t.Run("reports offline without a native query", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_OFFLINE.String(), "node-local")
		client := &nodeclient.FakeNodeClient{NodeID: "node-local"}
		fixture.service.nodeRegistry = noderegistry.New("node-local", client)
		request := connect.NewRequest(&xylona.GetSevenDaysToDieSandboxSettingsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		response, errGet := fixture.service.GetSevenDaysToDieSandboxSettings(t.Context(), request)
		if errGet != nil {
			t.Fatalf("GetSevenDaysToDieSandboxSettings() error = %v", errGet)
		}
		if response.Msg.GetConnectionState() != xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE ||
			len(client.QuerySevenDaysToDieSandboxSettingsCalls) != 0 {
			t.Fatalf("offline response = %+v, calls = %d", response.Msg, len(client.QuerySevenDaysToDieSandboxSettingsCalls))
		}
	})

	t.Run("routes to the paired owning node and exposes no credentials", func(t *testing.T) {
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
			QuerySevenDaysToDieSandboxSettingsResult: &node.SevenDaysToDieSandboxSettings{
				ConnectionState: node.SevenDaysToDieWebAPIConnectionStateAvailable,
				State:           node.SevenDaysToDieWebAPIValueStateAvailable,
				ComparisonState: node.SevenDaysToDieSandboxComparisonStateMismatch,
				ConfiguredCode:  "SAVED",
				EffectiveCode:   "RUNNING",
				ObservedAt:      time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC),
				Settings: []node.SevenDaysToDieSandboxSetting{{
					Key: "EnemySpawnMode", Label: "Enemy spawning", Description: "<b>Upstream text</b>", Group: "World",
					EffectiveValue: "1",
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
		request := connect.NewRequest(&xylona.GetSevenDaysToDieSandboxSettingsRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		response, errGet := fixture.service.GetSevenDaysToDieSandboxSettings(t.Context(), request)
		if errGet != nil {
			t.Fatalf("GetSevenDaysToDieSandboxSettings() error = %v", errGet)
		}
		if len(localClient.QuerySevenDaysToDieSandboxSettingsCalls) != 0 || len(remoteClient.QuerySevenDaysToDieSandboxSettingsCalls) != 1 {
			t.Fatalf("query routing: local = %d, remote = %d", len(localClient.QuerySevenDaysToDieSandboxSettingsCalls), len(remoteClient.QuerySevenDaysToDieSandboxSettingsCalls))
		}
		call := remoteClient.QuerySevenDaysToDieSandboxSettingsCalls[0]
		if call.WorkingDirectory != "/tmp/server-local-1" || call.TokenName == "" || call.TokenSecret == "" {
			t.Fatalf("node query = %+v", call)
		}
		if response.Msg.GetComparisonState() != xylona.SevenDaysToDieSandboxComparisonState_SEVEN_DAYS_TO_DIE_SANDBOX_COMPARISON_STATE_MISMATCH ||
			len(response.Msg.GetSettings()) != 1 || response.Msg.GetSettings()[0].GetDescription() != "<b>Upstream text</b>" {
			t.Fatalf("response = %+v", response.Msg)
		}
		responseJSON, errMarshal := protojson.Marshal(response.Msg)
		if errMarshal != nil {
			t.Fatalf("marshal response: %v", errMarshal)
		}
		if strings.Contains(string(responseJSON), call.TokenName) || strings.Contains(string(responseJSON), call.TokenSecret) {
			t.Fatal("response exposed WebAPI credentials")
		}
	})

	invalidResults := []struct {
		name   string
		result *node.SevenDaysToDieSandboxSettings
	}{
		{name: "missing", result: nil},
		{name: "over-count", result: &node.SevenDaysToDieSandboxSettings{Settings: make([]node.SevenDaysToDieSandboxSetting, node.SevenDaysToDieSandboxSettingCountLimit+1)}},
		{name: "oversized", result: &node.SevenDaysToDieSandboxSettings{ConfiguredCode: strings.Repeat("x", node.SevenDaysToDieSandboxTextByteLimit+1)}},
	}
	for _, test := range invalidResults {
		t.Run("rejects "+test.name+" node responses", func(t *testing.T) {
			fixture, _ := newSandboxSettingsRPCFixture(t, test.result)
			request := connect.NewRequest(&xylona.GetSevenDaysToDieSandboxSettingsRequest{GameServerId: "server-local-1"})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

			_, errGet := fixture.service.GetSevenDaysToDieSandboxSettings(t.Context(), request)
			if connect.CodeOf(errGet) != connect.CodeInternal {
				t.Fatalf("GetSevenDaysToDieSandboxSettings() code = %v, want %v", connect.CodeOf(errGet), connect.CodeInternal)
			}
		})
	}
}

func newSandboxSettingsRPCFixture(t *testing.T, result *node.SevenDaysToDieSandboxSettings) (*rbacRPCFixture, *nodeclient.FakeNodeClient) {
	t.Helper()
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_ONLINE.String(), "node-local")
	client := &nodeclient.FakeNodeClient{
		NodeID:                                   "node-local",
		GetProcessSnapshotResult:                 &node.ProcessSnapshot{Status: xylona.Status_ONLINE.String()},
		GetProcessSnapshotFound:                  true,
		QuerySevenDaysToDieSandboxSettingsResult: result,
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
