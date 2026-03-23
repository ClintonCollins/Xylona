package rpc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

func TestGetRemoteVersionInfoReturnsVersionState(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-remote-peer")

	errCreateGrant := fixture.conn.CreateFederatedAccessGrant(
		"fed-version-grant-1",
		"server-local-1",
		"node-remote-peer",
		"external-user-id",
		"External User",
		"viewer",
		"user-owner",
	)
	if errCreateGrant != nil {
		t.Fatalf("CreateFederatedAccessGrant() error = %v", errCreateGrant)
	}

	versionState := versiontracker.NewVersionStateMap()
	lastCheck := time.Unix(1_700_000_000, 0).UTC()
	versionState.Set("server-local-1", versiontracker.VersionState{
		Status:           versiontracker.VersionStatusChecked,
		InstalledVersion: "1.20.4",
		LatestVersion:    "1.21.1",
		UpdateAvailable:  true,
		LastCheckTime:    lastCheck,
		TrackerType:      "minecraft",
	})

	service := FederationService{
		ctx:          context.Background(),
		db:           fixture.conn,
		versionState: versionState,
	}

	mux := http.NewServeMux()
	path, handler := xylonaconnect.NewFederationHandler(&service)
	mux.Handle(path, injectFederationPeerIdentity("node-remote-peer", handler))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := xylonaconnect.NewFederationClient(http.DefaultClient, server.URL)

	request := connect.NewRequest(&xylona.FederationRemoteActionRequest{
		ServerId: "server-local-1",
	})
	request.Header().Set(helpers.FederationActingUserIDHeader, "external-user-id")
	request.Header().Set(helpers.FederationOriginNodeIDHeader, "node-remote-peer")

	response, errGet := client.GetRemoteVersionInfo(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetRemoteVersionInfo() error = %v", errGet)
	}
	if response == nil || response.Msg == nil || response.Msg.VersionInfo == nil {
		t.Fatalf("GetRemoteVersionInfo() returned empty response")
	}

	got := response.Msg.VersionInfo
	if got.InstalledVersion != "1.20.4" {
		t.Errorf("InstalledVersion = %q, want %q", got.InstalledVersion, "1.20.4")
	}
	if got.LatestVersion != "1.21.1" {
		t.Errorf("LatestVersion = %q, want %q", got.LatestVersion, "1.21.1")
	}
	if !got.UpdateAvailable {
		t.Error("UpdateAvailable = false, want true")
	}
	if got.TrackerType != "minecraft" {
		t.Errorf("TrackerType = %q, want %q", got.TrackerType, "minecraft")
	}
	if got.Status != xylona.VersionStatus_VERSION_STATUS_CHECKED {
		t.Errorf("Status = %v, want %v", got.Status, xylona.VersionStatus_VERSION_STATUS_CHECKED)
	}
	if got.LastCheckTime != lastCheck.Unix() {
		t.Errorf("LastCheckTime = %d, want %d", got.LastCheckTime, lastCheck.Unix())
	}
}

func injectFederationPeerIdentity(nodeID string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), federationPeerIdentityKey, FederationPeerIdentity{
			NodeID: nodeID,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TestGetVersionInfoDelegatesToFederatedServer(t *testing.T) {
	localFixture := newRBACRPCFixture(t)
	remoteFixture := newRBACRPCFixture(t)

	seedRemoteNodeForRBACRPCTests(t, localFixture.conn, "node-remote-peer")
	insertFederatedTestServer(t, remoteFixture.conn, "server-remote-1", "Remote One", "minecraft")

	errCreateGrant := remoteFixture.conn.CreateFederatedAccessGrant(
		"fed-version-grant-2",
		"server-remote-1",
		"node-local",
		"user-owner",
		"Owner User",
		"viewer",
		"user-owner",
	)
	if errCreateGrant != nil {
		t.Fatalf("CreateFederatedAccessGrant() error = %v", errCreateGrant)
	}

	errUpsertCache := localFixture.conn.UpsertRemoteServerCache(
		"remote-cache-version-1",
		"node-remote-peer",
		"node-remote-peer",
		"server-remote-1",
		"Remote One",
		"OFFLINE",
		"Minecraft",
		"minecraft",
		"203.0.113.10",
		25565,
		25565,
		20,
		0,
		"world",
		"1.20.4",
		"Remote Node",
		"node-remote-peer.remote.test",
		time.Now().UTC(),
	)
	if errUpsertCache != nil {
		t.Fatalf("UpsertRemoteServerCache() error = %v", errUpsertCache)
	}

	remoteState := versiontracker.NewVersionStateMap()
	remoteState.Set("server-remote-1", versiontracker.VersionState{
		Status:           versiontracker.VersionStatusChecked,
		InstalledVersion: "1.20.4",
		LatestVersion:    "1.21.1",
		UpdateAvailable:  true,
		TrackerType:      "minecraft",
	})

	remoteService := FederationService{
		ctx:          context.Background(),
		db:           remoteFixture.conn,
		versionState: remoteState,
	}

	mux := http.NewServeMux()
	path, handler := xylonaconnect.NewFederationHandler(&remoteService)
	mux.Handle(path, injectFederationPeerIdentity("node-local", handler))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	localFixture.service.remoteFederationClientFactory = func(_ *models.Node, _ string) (xylonaconnect.FederationClient, error) {
		return xylonaconnect.NewFederationClient(http.DefaultClient, server.URL), nil
	}
	localFixture.service.versionState = versiontracker.NewVersionStateMap()

	request := connect.NewRequest(&xylona.GetVersionInfoRequest{GameServerId: "server-remote-1"})
	addSessionCookieHeader(t, localFixture.conn, localFixture.secureCookie, request, "user-owner")

	response, errGet := localFixture.service.GetVersionInfo(context.Background(), request)
	if errGet != nil {
		t.Fatalf("GetVersionInfo() error = %v", errGet)
	}
	if response == nil || response.Msg == nil || response.Msg.VersionInfo == nil {
		t.Fatalf("GetVersionInfo() returned empty response")
	}
	if response.Msg.VersionInfo.TrackerType != "minecraft" {
		t.Errorf("TrackerType = %q, want %q", response.Msg.VersionInfo.TrackerType, "minecraft")
	}
	if response.Msg.VersionInfo.Status != xylona.VersionStatus_VERSION_STATUS_CHECKED {
		t.Errorf("Status = %v, want %v", response.Msg.VersionInfo.Status, xylona.VersionStatus_VERSION_STATUS_CHECKED)
	}
}

func TestCheckForUpdateDelegatesToFederatedServer(t *testing.T) {
	localFixture := newRBACRPCFixture(t)
	remoteFixture := newRBACRPCFixture(t)

	seedRemoteNodeForRBACRPCTests(t, localFixture.conn, "node-remote-peer")
	insertFederatedDummyGame(t, remoteFixture.conn)
	insertFederatedTestServer(t, remoteFixture.conn, "server-remote-2", "Remote Dummy", "dummy-game")

	errCreateGrant := remoteFixture.conn.CreateFederatedAccessGrant(
		"fed-version-grant-3",
		"server-remote-2",
		"node-local",
		"user-owner",
		"Owner User",
		"viewer",
		"user-owner",
	)
	if errCreateGrant != nil {
		t.Fatalf("CreateFederatedAccessGrant() error = %v", errCreateGrant)
	}

	errUpsertCache := localFixture.conn.UpsertRemoteServerCache(
		"remote-cache-version-2",
		"node-remote-peer",
		"node-remote-peer",
		"server-remote-2",
		"Remote Dummy",
		"OFFLINE",
		"Dummy Game",
		"dummy-game",
		"203.0.113.11",
		25565,
		25565,
		20,
		0,
		"world",
		"1.0.0",
		"Remote Node",
		"node-remote-peer.remote.test",
		time.Now().UTC(),
	)
	if errUpsertCache != nil {
		t.Fatalf("UpsertRemoteServerCache() error = %v", errUpsertCache)
	}

	remoteState := versiontracker.NewVersionStateMap()
	dummyTracker := versiontracker.NewDummyTracker()

	actionsCtx, cancelActions := context.WithCancel(context.Background())
	t.Cleanup(cancelActions)

	supervisorInst, errSupervisor := supervisor.New(actionsCtx)
	if errSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errSupervisor)
	}

	actionsInst := actions.NewInstance(
		actionsCtx,
		remoteFixture.conn,
		supervisorInst,
		nil,
		nil,
		remoteState,
		versiontracker.ResolverConfig{
			DummyTracker: dummyTracker,
			DummyGameID:  "dummy-game",
		},
	)

	remoteService := FederationService{
		ctx:          actionsCtx,
		db:           remoteFixture.conn,
		actionsInst:  actionsInst,
		versionState: remoteState,
	}

	mux := http.NewServeMux()
	path, handler := xylonaconnect.NewFederationHandler(&remoteService)
	mux.Handle(path, injectFederationPeerIdentity("node-local", handler))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	localFixture.service.remoteFederationClientFactory = func(_ *models.Node, _ string) (xylonaconnect.FederationClient, error) {
		return xylonaconnect.NewFederationClient(http.DefaultClient, server.URL), nil
	}

	request := connect.NewRequest(&xylona.CheckForUpdateRequest{GameServerId: "server-remote-2"})
	addSessionCookieHeader(t, localFixture.conn, localFixture.secureCookie, request, "user-owner")

	response, errCheck := localFixture.service.CheckForUpdate(context.Background(), request)
	if errCheck != nil {
		t.Fatalf("CheckForUpdate() error = %v", errCheck)
	}
	if response == nil || response.Msg == nil || response.Msg.VersionInfo == nil {
		t.Fatalf("CheckForUpdate() returned empty response")
	}

	got := response.Msg.VersionInfo
	if got.Status != xylona.VersionStatus_VERSION_STATUS_CHECKED {
		t.Errorf("Status = %v, want %v", got.Status, xylona.VersionStatus_VERSION_STATUS_CHECKED)
	}
	if got.TrackerType != "dummy" {
		t.Errorf("TrackerType = %q, want %q", got.TrackerType, "dummy")
	}
	if got.InstalledVersion != "1.0.0" {
		t.Errorf("InstalledVersion = %q, want %q", got.InstalledVersion, "1.0.0")
	}
	if got.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", got.LatestVersion, "2.0.0")
	}
	if !got.UpdateAvailable {
		t.Error("UpdateAvailable = false, want true")
	}
}

func insertFederatedTestServer(t *testing.T, conn *db.Connection, serverID string, serverName string, gameID string) {
	t.Helper()

	_, errInsert := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server
		 (id, user_id, name, game_id, start_command, status, set_players, max_players, map, ip, port, query_port, directory, node_id)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		serverID, "user-owner", serverName, gameID, "java -jar server.jar", "OFFLINE",
		20, 20, "world", "127.0.0.1", 25565, 25565, fmt.Sprintf("/tmp/%s", serverID), "node-local",
	)
	if errInsert != nil {
		t.Fatalf("insert test game server: %v", errInsert)
	}
}

func insertFederatedDummyGame(t *testing.T, conn *db.Connection) {
	t.Helper()

	_, errInsert := conn.InsertGame(conn.DB, &models.GameSetter{
		ID:                omit.From("dummy-game"),
		Name:              omit.From("Dummy Game"),
		DefaultPort:       omit.From(int64(25565)),
		DefaultQueryPort:  omit.From(int64(25565)),
		DefaultMaxPlayers: omit.From(int64(20)),
		WindowsSupport:    omit.From(true),
	})
	if errInsert != nil {
		t.Fatalf("InsertGame() dummy setup error = %v", errInsert)
	}
}
