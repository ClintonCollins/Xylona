package rpc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if response == nil || response.Msg == nil || response.Msg.GetVersionInfo() == nil {
		t.Fatalf("GetRemoteVersionInfo() returned empty response")
	}

	got := response.Msg.GetVersionInfo()
	if got.GetInstalledVersion() != "1.20.4" {
		t.Errorf("InstalledVersion = %q, want %q", got.GetInstalledVersion(), "1.20.4")
	}
	if got.GetLatestVersion() != "1.21.1" {
		t.Errorf("LatestVersion = %q, want %q", got.GetLatestVersion(), "1.21.1")
	}
	if !got.GetUpdateAvailable() {
		t.Error("UpdateAvailable = false, want true")
	}
	if got.GetTrackerType() != "minecraft" {
		t.Errorf("TrackerType = %q, want %q", got.GetTrackerType(), "minecraft")
	}
	if got.GetStatus() != xylona.VersionStatus_VERSION_STATUS_CHECKED {
		t.Errorf("Status = %v, want %v", got.GetStatus(), xylona.VersionStatus_VERSION_STATUS_CHECKED)
	}
	if got.GetLastCheckTime() != lastCheck.Unix() {
		t.Errorf("LastCheckTime = %d, want %d", got.GetLastCheckTime(), lastCheck.Unix())
	}
}

func TestUpdateRemoteServerReturnsErrorForUnsupportedMinecraftVariant(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-remote-peer")

	errGrant := fixture.conn.CreateFederatedAccessGrant(
		"fed-update-grant-1",
		"server-local-1",
		"node-remote-peer",
		"external-user-id",
		"External User",
		"admin",
		"user-owner",
	)
	if errGrant != nil {
		t.Fatalf("CreateFederatedAccessGrant() error = %v", errGrant)
	}

	_, errUpdateGame := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`update game set server_software = ? where id = ?`,
		`[
			{"id":"vanilla","name":"Vanilla","jar_source":null},
			{"id":"fabric","name":"Fabric","jar_source":null}
		]`,
		"minecraft",
	)
	if errUpdateGame != nil {
		t.Fatalf("update minecraft game server_software: %v", errUpdateGame)
	}

	_, errUpdateServer := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`update game_server set server_software = ?, server_executable = ? where id = ?`,
		"fabric",
		"fabric-server.jar",
		"server-local-1",
	)
	if errUpdateServer != nil {
		t.Fatalf("update game server software: %v", errUpdateServer)
	}

	actionsCtx, cancelActions := context.WithCancel(context.Background())
	t.Cleanup(cancelActions)

	supervisorInst, errSupervisor := supervisor.New(actionsCtx)
	if errSupervisor != nil {
		t.Fatalf("supervisor.New() error = %v", errSupervisor)
	}

	actionsInst := actions.NewInstance(
		actionsCtx,
		fixture.conn,
		supervisorInst,
		nil,
		nil,
		versiontracker.NewVersionStateMap(),
		versiontracker.ResolverConfig{},
	)

	service := FederationService{
		ctx:         actionsCtx,
		db:          fixture.conn,
		actionsInst: actionsInst,
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

	_, errUpdate := client.UpdateRemoteServer(context.Background(), request)
	if errUpdate == nil {
		t.Fatal("UpdateRemoteServer() error = nil, want error")
	}
	if connect.CodeOf(errUpdate) != connect.CodeFailedPrecondition {
		t.Fatalf("UpdateRemoteServer() code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeFailedPrecondition)
	}
}

func TestFederationUpdateErrorCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want connect.Code
	}{
		{
			name: "unsupported minecraft variant is failed precondition",
			err:  actions.ErrMinecraftVariantUpdateNotSupported,
			want: connect.CodeFailedPrecondition,
		},
		{
			name: "missing game update command is failed precondition",
			err:  actions.ErrGameUpdateNotConfigured,
			want: connect.CodeFailedPrecondition,
		},
		{
			name: "missing internal updater is failed precondition",
			err:  actions.ErrInternalGameUpdateMissing,
			want: connect.CodeFailedPrecondition,
		},
		{
			name: "unexpected errors stay internal",
			err:  errors.New("unexpected update failure"),
			want: connect.CodeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := federationUpdateErrorCode(tt.err)
			if got != tt.want {
				t.Fatalf("federationUpdateErrorCode() = %v, want %v", got, tt.want)
			}
		})
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
	if response == nil || response.Msg == nil || response.Msg.GetVersionInfo() == nil {
		t.Fatalf("GetVersionInfo() returned empty response")
	}
	if response.Msg.GetVersionInfo().GetTrackerType() != "minecraft" {
		t.Errorf("TrackerType = %q, want %q", response.Msg.GetVersionInfo().GetTrackerType(), "minecraft")
	}
	if response.Msg.GetVersionInfo().GetStatus() != xylona.VersionStatus_VERSION_STATUS_CHECKED {
		t.Errorf("Status = %v, want %v", response.Msg.GetVersionInfo().GetStatus(), xylona.VersionStatus_VERSION_STATUS_CHECKED)
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
	if response == nil || response.Msg == nil || response.Msg.GetVersionInfo() == nil {
		t.Fatalf("CheckForUpdate() returned empty response")
	}

	got := response.Msg.GetVersionInfo()
	if got.GetStatus() != xylona.VersionStatus_VERSION_STATUS_CHECKED {
		t.Errorf("Status = %v, want %v", got.GetStatus(), xylona.VersionStatus_VERSION_STATUS_CHECKED)
	}
	if got.GetTrackerType() != "dummy" {
		t.Errorf("TrackerType = %q, want %q", got.GetTrackerType(), "dummy")
	}
	if got.GetInstalledVersion() != "1.0.0" {
		t.Errorf("InstalledVersion = %q, want %q", got.GetInstalledVersion(), "1.0.0")
	}
	if got.GetLatestVersion() != "2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", got.GetLatestVersion(), "2.0.0")
	}
	if !got.GetUpdateAvailable() {
		t.Error("UpdateAvailable = false, want true")
	}
}

func TestUpdateGameServerDelegatesUnsupportedMinecraftVariantFailure(t *testing.T) {
	localFixture := newRBACRPCFixture(t)
	remoteFixture := newRBACRPCFixture(t)

	seedRemoteNodeForRBACRPCTests(t, localFixture.conn, "node-remote-peer")
	insertFederatedTestServer(t, remoteFixture.conn, "server-remote-update-1", "Remote Fabric", "minecraft")

	errGrant := remoteFixture.conn.CreateFederatedAccessGrant(
		"fed-update-grant-2",
		"server-remote-update-1",
		"node-local",
		"user-owner",
		"Owner User",
		"admin",
		"user-owner",
	)
	if errGrant != nil {
		t.Fatalf("CreateFederatedAccessGrant() error = %v", errGrant)
	}

	_, errUpdateGame := remoteFixture.conn.SQLDb.ExecContext(
		context.Background(),
		`update game set server_software = ? where id = ?`,
		`[
			{"id":"vanilla","name":"Vanilla","jar_source":null},
			{"id":"fabric","name":"Fabric","jar_source":null}
		]`,
		"minecraft",
	)
	if errUpdateGame != nil {
		t.Fatalf("update minecraft game server_software: %v", errUpdateGame)
	}

	_, errUpdateServer := remoteFixture.conn.SQLDb.ExecContext(
		context.Background(),
		`update game_server set server_software = ?, server_executable = ? where id = ?`,
		"fabric",
		"fabric-server.jar",
		"server-remote-update-1",
	)
	if errUpdateServer != nil {
		t.Fatalf("update remote game server software: %v", errUpdateServer)
	}

	errUpsertCache := localFixture.conn.UpsertRemoteServerCache(
		"remote-cache-update-1",
		"node-remote-peer",
		"node-remote-peer",
		"server-remote-update-1",
		"Remote Fabric",
		"OFFLINE",
		"Minecraft",
		"minecraft",
		"203.0.113.11",
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
		versiontracker.NewVersionStateMap(),
		versiontracker.ResolverConfig{},
	)

	remoteService := FederationService{
		ctx:         actionsCtx,
		db:          remoteFixture.conn,
		actionsInst: actionsInst,
	}

	mux := http.NewServeMux()
	path, handler := xylonaconnect.NewFederationHandler(&remoteService)
	mux.Handle(path, injectFederationPeerIdentity("node-local", handler))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	localFixture.service.remoteFederationClientFactory = func(_ *models.Node, _ string) (xylonaconnect.FederationClient, error) {
		return xylonaconnect.NewFederationClient(http.DefaultClient, server.URL), nil
	}

	request := connect.NewRequest(&xylona.UpdateGameServerRequest{ServerId: "server-remote-update-1"})
	addSessionCookieHeader(t, localFixture.conn, localFixture.secureCookie, request, "user-owner")

	_, errUpdate := localFixture.service.UpdateGameServer(context.Background(), request)
	if errUpdate == nil {
		t.Fatal("UpdateGameServer() error = nil, want error")
	}
	if connect.CodeOf(errUpdate) != connect.CodeFailedPrecondition {
		t.Fatalf("UpdateGameServer() code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeFailedPrecondition)
	}
	if !strings.Contains(errUpdate.Error(), actions.ErrMinecraftVariantUpdateNotSupported.Error()) {
		t.Fatalf("UpdateGameServer() error = %q, want substring %q", errUpdate.Error(), actions.ErrMinecraftVariantUpdateNotSupported.Error())
	}
}

func insertFederatedTestServer(t *testing.T, conn *db.Connection, serverID string, serverName string, gameID string) {
	t.Helper()

	_, errInsert := conn.SQLDb.ExecContext(
		context.Background(),
		`insert into game_server
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		serverID, "user-owner", serverName, gameID, "OFFLINE",
		20, 20, "world", "127.0.0.1", 25565, 25565, fmt.Sprintf("/tmp/%s", serverID), "node-local", "[]",
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
