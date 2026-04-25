package rpc

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/updateconfig"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestGetVersionInfoRequiresAuthentication(t *testing.T) {
	states := versiontracker.NewVersionStateMap()
	states.Set("server-1", versiontracker.VersionState{
		Status: versiontracker.VersionStatusChecked,
	})

	service := &XylonaService{
		versionState: states,
	}

	_, errGet := service.GetVersionInfo(
		context.Background(),
		connect.NewRequest(&xylona.GetVersionInfoRequest{GameServerId: "server-1"}),
	)
	if connect.CodeOf(errGet) != connect.CodeUnauthenticated {
		t.Fatalf("GetVersionInfo() code = %v, want %v", connect.CodeOf(errGet), connect.CodeUnauthenticated)
	}
}

func TestCheckForUpdateRequiresAuthentication(t *testing.T) {
	service := &XylonaService{}

	_, errCheck := service.CheckForUpdate(
		context.Background(),
		connect.NewRequest(&xylona.CheckForUpdateRequest{GameServerId: "server-1"}),
	)
	if connect.CodeOf(errCheck) != connect.CodeUnauthenticated {
		t.Fatalf("CheckForUpdate() code = %v, want %v", connect.CodeOf(errCheck), connect.CodeUnauthenticated)
	}
}

func TestRemoteVersionTrackerContextUsesOwningNodeUpdateCommand(t *testing.T) {
	service := &XylonaService{ctx: context.Background()}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(&nodeclient.FakeNodeClient{
		NodeID:         "node-remote-linux",
		SnapshotResult: &node.NodeSnapshot{OS: "linux"},
	})
	service.nodeRegistry = registry

	game := &models.Game{
		ID:                   "legacy-steam",
		LinuxUpdateCommand:   "steamcmd +app_update 294420",
		WindowsUpdateCommand: "steamcmd +app_update 111111",
	}
	gameServer := &models.GameServer{
		ID:     "server-remote-steam",
		GameID: game.ID,
		NodeID: "node-remote-linux",
	}
	gameServer.R.Game = game

	info := service.remoteVersionTrackerContext(gameServer)
	if info.UpdateCommand != game.LinuxUpdateCommand {
		t.Fatalf("UpdateCommand = %q, want linux command %q", info.UpdateCommand, game.LinuxUpdateCommand)
	}
	if versiontracker.ResolveTrackerWithContext(versiontracker.ResolverConfig{}, info) == nil {
		t.Fatal("ResolveTrackerWithContext() = nil, want legacy steam tracker")
	}
}

func TestGetVersionInfoRequiresViewPermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.service.versionState = versiontracker.NewVersionStateMap()
	fixture.service.versionState.Set("server-local-1", versiontracker.VersionState{
		Status: versiontracker.VersionStatusChecked,
	})

	request := connect.NewRequest(&xylona.GetVersionInfoRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-other")

	_, errGet := fixture.service.GetVersionInfo(context.Background(), request)
	if connect.CodeOf(errGet) != connect.CodePermissionDenied {
		t.Fatalf("GetVersionInfo() code = %v, want %v", connect.CodeOf(errGet), connect.CodePermissionDenied)
	}
}

func TestCheckForUpdateRequiresViewPermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.service.versionState = versiontracker.NewVersionStateMap()

	request := connect.NewRequest(&xylona.CheckForUpdateRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-other")

	_, errCheck := fixture.service.CheckForUpdate(context.Background(), request)
	if connect.CodeOf(errCheck) != connect.CodePermissionDenied {
		t.Fatalf("CheckForUpdate() code = %v, want %v", connect.CodeOf(errCheck), connect.CodePermissionDenied)
	}
}

func TestSetDummyUpdateFailureRequiresAuthentication(t *testing.T) {
	service := &XylonaService{
		dummyTracker: versiontracker.NewDummyTracker(),
		versionState: versiontracker.NewVersionStateMap(),
	}

	_, errSet := service.SetDummyUpdateFailure(
		context.Background(),
		connect.NewRequest(&xylona.SetDummyUpdateFailureRequest{
			SimulateFailure: true,
		}),
	)
	if connect.CodeOf(errSet) != connect.CodeUnauthenticated {
		t.Fatalf("SetDummyUpdateFailure() code = %v, want %v", connect.CodeOf(errSet), connect.CodeUnauthenticated)
	}
}

func TestClearDummyVersionStatesOnlyRemovesDummyEntries(t *testing.T) {
	tracker := versiontracker.NewDummyTracker()
	tracker.MarkUpdated()
	states := versiontracker.NewVersionStateMap()
	states.Set("server-1", versiontracker.VersionState{
		Status:           versiontracker.VersionStatusChecked,
		InstalledVersion: "2.0.0",
		LatestVersion:    "2.0.0",
		TrackerType:      "dummy",
	})
	states.Set("server-2", versiontracker.VersionState{
		Status:           versiontracker.VersionStatusChecked,
		InstalledVersion: "1.21.4",
		LatestVersion:    "1.21.4",
		TrackerType:      "minecraft",
	})

	tracker.Reset()
	tracker.SetSimulateFailure(true)
	clearDummyVersionStates(states)

	installedVersion, errInstalled := tracker.GetInstalledVersion(context.Background(), nil)
	if errInstalled != nil {
		t.Fatalf("GetInstalledVersion() error = %v", errInstalled)
	}
	if installedVersion != "1.0.0" {
		t.Errorf("installed version = %q, want %q", installedVersion, "1.0.0")
	}
	if !tracker.SimulateFailure() {
		t.Error("SimulateFailure() = false, want true")
	}
	if state := states.Get("server-1"); state.Status != versiontracker.VersionStatusNoTracker {
		t.Errorf("version state after reset = %+v, want deleted/no tracker", state)
	}
	if state := states.Get("server-2"); state.Status != versiontracker.VersionStatusChecked {
		t.Errorf("non-dummy state after reset = %+v, want preserved checked state", state)
	}
}

func TestResolveRemoteVersionStateUsesProbeForSteamManifest(t *testing.T) {
	game := &models.Game{
		ID:         "steam-game",
		SteamAppID: "294420",
	}
	errConfig := updateconfig.SaveGameConfigToModel(game, updateproviders.GameConfig{
		UpdateProvider: updateproviders.ProviderConfig{
			Kind: updateproviders.ProviderKindSteamCMD,
		},
	})
	if errConfig != nil {
		t.Fatalf("SaveGameConfigToModel() error = %v", errConfig)
	}

	gameServer := &models.GameServer{
		ID:        "server-remote-steam",
		GameID:    "steam-game",
		Directory: "/srv/steam-server",
		NodeID:    "node-remote",
	}
	gameServer.R.Game = game

	states := versiontracker.NewVersionStateMap()
	service := &XylonaService{
		ctx:          context.Background(),
		versionState: states,
	}
	contextKey := service.remoteVersionTrackerContext(gameServer).CacheKey()
	states.Set(gameServer.ID, versiontracker.VersionState{
		Status:          versiontracker.VersionStatusChecked,
		LatestVersion:   "9000",
		LatestCheckTime: time.Now(),
		TrackerType:     "steam",
		ContextKey:      contextKey,
	})

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		ProbeInstalledVersionResult: node.InstalledVersionProbeResult{
			Found:   true,
			Version: "8000",
		},
	}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(remoteClient)
	service.nodeRegistry = registry

	state, errResolve := service.resolveRemoteVersionState(context.Background(), gameServer, false)
	if errResolve != nil {
		t.Fatalf("resolveRemoteVersionState() error = %v", errResolve)
	}

	if state.InstalledVersion != "8000" {
		t.Fatalf("InstalledVersion = %q, want %q", state.InstalledVersion, "8000")
	}
	if state.LatestVersion != "9000" {
		t.Fatalf("LatestVersion = %q, want cached %q", state.LatestVersion, "9000")
	}
	if len(remoteClient.ProbeInstalledVersionCalls) != 1 {
		t.Fatalf("ProbeInstalledVersion call count = %d, want 1", len(remoteClient.ProbeInstalledVersionCalls))
	}
	probeCall := remoteClient.ProbeInstalledVersionCalls[0]
	if probeCall.Kind != node.InstalledVersionProbeKindSteamManifest {
		t.Fatalf("ProbeInstalledVersion kind = %v, want steam manifest", probeCall.Kind)
	}
	if probeCall.PreferredSteamAppID != "294420" {
		t.Fatalf("ProbeInstalledVersion preferred app ID = %q, want %q", probeCall.PreferredSteamAppID, "294420")
	}
	if len(remoteClient.ReadFileCalls) != 0 {
		t.Fatalf("ReadFile call count = %d, want 0", len(remoteClient.ReadFileCalls))
	}
	if len(remoteClient.ListFilesCalls) != 0 {
		t.Fatalf("ListFiles call count = %d, want 0", len(remoteClient.ListFilesCalls))
	}
}
