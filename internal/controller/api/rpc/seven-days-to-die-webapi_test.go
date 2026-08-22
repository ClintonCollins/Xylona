package rpc

import (
	"context"
	"errors"
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

func TestGetSevenDaysToDieWebAPIStatus(t *testing.T) {
	t.Run("requires authentication", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		request := connect.NewRequest(&xylona.GetSevenDaysToDieWebAPIStatusRequest{GameServerId: "server-local-1"})

		_, errStatus := fixture.service.GetSevenDaysToDieWebAPIStatus(t.Context(), request)
		if connect.CodeOf(errStatus) != connect.CodeUnauthenticated {
			t.Fatalf("GetSevenDaysToDieWebAPIStatus() code = %v, want %v", connect.CodeOf(errStatus), connect.CodeUnauthenticated)
		}
	})

	t.Run("rejects access without game server view", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_ONLINE.String(), "node-local")
		request := connect.NewRequest(&xylona.GetSevenDaysToDieWebAPIStatusRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-other")

		_, errStatus := fixture.service.GetSevenDaysToDieWebAPIStatus(t.Context(), request)
		if connect.CodeOf(errStatus) != connect.CodePermissionDenied {
			t.Fatalf("GetSevenDaysToDieWebAPIStatus() code = %v, want %v", connect.CodeOf(errStatus), connect.CodePermissionDenied)
		}
	})

	t.Run("rejects non seven days to die servers", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		request := connect.NewRequest(&xylona.GetSevenDaysToDieWebAPIStatusRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		_, errStatus := fixture.service.GetSevenDaysToDieWebAPIStatus(t.Context(), request)
		if connect.CodeOf(errStatus) != connect.CodeFailedPrecondition {
			t.Fatalf("GetSevenDaysToDieWebAPIStatus() code = %v, want %v", connect.CodeOf(errStatus), connect.CodeFailedPrecondition)
		}
	})

	t.Run("reports an offline server without querying its node", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_OFFLINE.String(), "node-local")
		client := &nodeclient.FakeNodeClient{NodeID: "node-local"}
		fixture.service.nodeRegistry = noderegistry.New("node-local", client)
		request := connect.NewRequest(&xylona.GetSevenDaysToDieWebAPIStatusRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		response, errStatus := fixture.service.GetSevenDaysToDieWebAPIStatus(t.Context(), request)
		if errStatus != nil {
			t.Fatalf("GetSevenDaysToDieWebAPIStatus() error = %v", errStatus)
		}
		if response.Msg.GetStatus().GetConnectionState() != xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE {
			t.Fatalf("connection state = %v, want server offline", response.Msg.GetStatus().GetConnectionState())
		}
		if len(client.QuerySevenDaysToDieWebAPIStatusCalls) != 0 {
			t.Fatalf("node query call count = %d, want 0", len(client.QuerySevenDaysToDieWebAPIStatusCalls))
		}
	})

	t.Run("reports an unavailable node", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_ONLINE.String(), "node-local")
		request := connect.NewRequest(&xylona.GetSevenDaysToDieWebAPIStatusRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		response, errStatus := fixture.service.GetSevenDaysToDieWebAPIStatus(t.Context(), request)
		if errStatus != nil {
			t.Fatalf("GetSevenDaysToDieWebAPIStatus() error = %v", errStatus)
		}
		if response.Msg.GetStatus().GetConnectionState() != xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_NODE_UNAVAILABLE {
			t.Fatalf("connection state = %v, want node unavailable", response.Msg.GetStatus().GetConnectionState())
		}
	})

	t.Run("reports a missing live process as offline", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_ONLINE.String(), "node-local")
		client := &nodeclient.FakeNodeClient{NodeID: "node-local"}
		fixture.service.nodeRegistry = noderegistry.New("node-local", client)
		request := connect.NewRequest(&xylona.GetSevenDaysToDieWebAPIStatusRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		response, errStatus := fixture.service.GetSevenDaysToDieWebAPIStatus(t.Context(), request)
		if errStatus != nil {
			t.Fatalf("GetSevenDaysToDieWebAPIStatus() error = %v", errStatus)
		}
		if response.Msg.GetStatus().GetConnectionState() != xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE {
			t.Fatalf("connection state = %v, want server offline", response.Msg.GetStatus().GetConnectionState())
		}
		if len(client.GetProcessSnapshotCalls) != 1 || len(client.QuerySevenDaysToDieWebAPIStatusCalls) != 0 {
			t.Fatalf("process calls = %v, query calls = %d", client.GetProcessSnapshotCalls, len(client.QuerySevenDaysToDieWebAPIStatusCalls))
		}
	})

	t.Run("reports a process transport failure as node unavailable", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_ONLINE.String(), "node-local")
		client := &nodeclient.FakeNodeClient{
			NodeID:                "node-local",
			GetProcessSnapshotErr: errors.New("node transport failed"),
		}
		fixture.service.nodeRegistry = noderegistry.New("node-local", client)
		request := connect.NewRequest(&xylona.GetSevenDaysToDieWebAPIStatusRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

		response, errStatus := fixture.service.GetSevenDaysToDieWebAPIStatus(t.Context(), request)
		if errStatus != nil {
			t.Fatalf("GetSevenDaysToDieWebAPIStatus() error = %v", errStatus)
		}
		if response.Msg.GetStatus().GetConnectionState() != xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_NODE_UNAVAILABLE {
			t.Fatalf("connection state = %v, want node unavailable", response.Msg.GetStatus().GetConnectionState())
		}
		if len(client.QuerySevenDaysToDieWebAPIStatusCalls) != 0 {
			t.Fatalf("node query call count = %d, want 0", len(client.QuerySevenDaysToDieWebAPIStatusCalls))
		}
	})

	t.Run("routes to the owning node and exposes only typed diagnostics", func(t *testing.T) {
		fixture := newRBACRPCFixture(t)
		fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
		_, errNode := fixture.conn.SQLDb.ExecContext(
			t.Context(),
			"insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)",
			"node-remote", "Remote Node", "https://node.example.test", true,
		)
		if errNode != nil {
			t.Fatalf("insert remote node: %v", errNode)
		}
		_, errIP := fixture.conn.SQLDb.ExecContext(
			t.Context(),
			"insert into ip (address, usable, external, node_id) values (?, ?, ?, ?)",
			"127.0.0.2", true, false, "node-remote",
		)
		if errIP != nil {
			t.Fatalf("insert remote node IP: %v", errIP)
		}
		setSevenDaysToDieWebAPITestServer(t, fixture, xylona.Status_ONLINE.String(), "node-remote")

		active := false
		observedAt := time.Date(2026, time.August, 22, 18, 30, 0, 0, time.UTC)
		localClient := &nodeclient.FakeNodeClient{NodeID: "node-local"}
		remoteClient := &nodeclient.FakeNodeClient{
			NodeID:                   "node-remote",
			GetProcessSnapshotResult: &node.ProcessSnapshot{Status: xylona.Status_ONLINE.String()},
			GetProcessSnapshotFound:  true,
			QuerySevenDaysToDieWebAPIStatusResult: &node.SevenDaysToDieWebAPIStatus{
				ConnectionState: node.SevenDaysToDieWebAPIConnectionStateAvailable,
				APIVersion:      "1.4.2",
				Capabilities: node.SevenDaysToDieWebAPICapabilities{
					PlayerData:                true,
					RuntimeSettings:           true,
					HostileAndAnimalPositions: true,
					ReportedMods:              true,
				},
				WorldTimeState:   node.SevenDaysToDieWebAPIValueStateAvailable,
				WorldTime:        &node.SevenDaysToDieGameTime{Day: 12, Hour: 18, Minute: 30},
				BloodMoonState:   node.SevenDaysToDieWebAPIValueStateAvailable,
				BloodMoonActive:  &active,
				NextBloodMoon:    &node.SevenDaysToDieGameTime{Day: 14, Hour: 22},
				NextBloodMoonEnd: &node.SevenDaysToDieGameTime{Day: 15, Hour: 4},
				ObservedAt:       observedAt,
			},
		}
		registry := noderegistry.New("node-local", localClient)
		registry.Register(remoteClient)
		fixture.service.nodeRegistry = registry

		actionsContext, cancelActions := context.WithCancel(t.Context())
		t.Cleanup(cancelActions)
		fixture.service.actionsInst = actions.NewInstance(
			actionsContext,
			fixture.conn,
			localClient,
			registry,
			nil,
			versiontracker.NewVersionStateMap(),
			versiontracker.ResolverConfig{},
		)

		request := connect.NewRequest(&xylona.GetSevenDaysToDieWebAPIStatusRequest{GameServerId: "server-local-1"})
		addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
		response, errStatus := fixture.service.GetSevenDaysToDieWebAPIStatus(t.Context(), request)
		if errStatus != nil {
			t.Fatalf("GetSevenDaysToDieWebAPIStatus() error = %v", errStatus)
		}
		if len(localClient.QuerySevenDaysToDieWebAPIStatusCalls) != 0 {
			t.Fatalf("local node query call count = %d, want 0", len(localClient.QuerySevenDaysToDieWebAPIStatusCalls))
		}
		if len(localClient.GetProcessSnapshotCalls) != 0 || len(remoteClient.GetProcessSnapshotCalls) != 1 {
			t.Fatalf("process snapshot routing: local = %v, remote = %v", localClient.GetProcessSnapshotCalls, remoteClient.GetProcessSnapshotCalls)
		}
		if len(remoteClient.QuerySevenDaysToDieWebAPIStatusCalls) != 1 {
			t.Fatalf("remote node query call count = %d, want 1", len(remoteClient.QuerySevenDaysToDieWebAPIStatusCalls))
		}
		call := remoteClient.QuerySevenDaysToDieWebAPIStatusCalls[0]
		if call.WorkingDirectory != "/tmp/server-local-1" || call.TokenName == "" || call.TokenSecret == "" {
			t.Fatal("node query did not include the working directory and derived credentials")
		}

		status := response.Msg.GetStatus()
		if status.GetApiVersion() != "1.4.2" || status.GetWorldTime().GetDay() != 12 || status.GetNextBloodMoonEnd().GetHour() != 4 {
			t.Fatalf("status mapping = %+v", status)
		}
		if status.BloodMoonActive == nil || status.GetBloodMoonActive() {
			t.Fatalf("blood moon active = %v, want present false", status.BloodMoonActive)
		}
		if !status.GetCapabilities().GetPlayerData() || !status.GetCapabilities().GetHostileAndAnimalPositions() || status.GetCapabilities().GetNativeLog() {
			t.Fatalf("capability mapping = %+v", status.GetCapabilities())
		}
		if status.GetObservedAt().AsTime() != observedAt {
			t.Fatalf("observed at = %v, want %v", status.GetObservedAt().AsTime(), observedAt)
		}

		responseJSON, errMarshal := protojson.Marshal(response.Msg)
		if errMarshal != nil {
			t.Fatalf("marshal response: %v", errMarshal)
		}
		if strings.Contains(string(responseJSON), call.TokenName) || strings.Contains(string(responseJSON), call.TokenSecret) {
			t.Fatal("response exposed WebAPI credentials")
		}
	})
}

func setSevenDaysToDieWebAPITestServer(t *testing.T, fixture *rbacRPCFixture, status string, nodeID string) {
	t.Helper()
	serverIP := "127.0.0.1"
	if nodeID != "node-local" {
		serverIP = "127.0.0.2"
	}
	_, errUpdate := fixture.conn.SQLDb.ExecContext(
		t.Context(),
		"update game_server set game_id = ?, status = ?, ip = ?, node_id = ? where id = ?",
		sevenDaysToDieGameID,
		status,
		serverIP,
		nodeID,
		"server-local-1",
	)
	if errUpdate != nil {
		t.Fatalf("update 7 Days to Die test server: %v", errUpdate)
	}
}
