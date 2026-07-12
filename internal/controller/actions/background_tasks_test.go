package actions

import (
	"context"
	"sync"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestGameServerQueryPort(t *testing.T) {
	tests := []struct {
		name       string
		gameServer *models.GameServer
		want       int64
	}{
		{name: "missing server", want: 0},
		{
			name:       "ordinary query port",
			gameServer: &models.GameServer{GameID: "valheim", Port: 2456, QueryPort: 2457},
			want:       2457,
		},
		{
			name:       "7 Days to Die A2S uses game port",
			gameServer: &models.GameServer{GameID: "7_days_to_die", Port: 26900, QueryPort: 26904},
			want:       26900,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := gameServerQueryPort(test.gameServer)
			if got != test.want {
				t.Fatalf("gameServerQueryPort() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestCheckServerVersionSetsResolvedTrackerType(t *testing.T) {
	t.Parallel()

	inst := &Instance{
		ctx:          context.Background(),
		versionState: versiontracker.NewVersionStateMap(),
		resolverConfig: versiontracker.ResolverConfig{
			DummyTracker: versiontracker.NewDummyTracker(),
			DummyGameID:  "dummy-game",
		},
	}

	gs := &models.GameServer{
		ID:     "server-1",
		GameID: "dummy-game",
	}
	gs.R.Game = &models.Game{}

	inst.checkServerVersion(gs, eventbus.Get())

	state := inst.versionState.Get(gs.ID)
	if state.Status != versiontracker.VersionStatusChecked {
		t.Fatalf("status = %v, want %v", state.Status, versiontracker.VersionStatusChecked)
	}
	if state.TrackerType != "dummy" {
		t.Fatalf("tracker type = %q, want %q", state.TrackerType, "dummy")
	}
}

func TestCheckServerVersionPopulatesVersionsWhenUpdateAvailable(t *testing.T) {
	t.Parallel()

	dummy := versiontracker.NewDummyTracker()
	inst := &Instance{
		ctx:          context.Background(),
		versionState: versiontracker.NewVersionStateMap(),
		resolverConfig: versiontracker.ResolverConfig{
			DummyTracker: dummy,
			DummyGameID:  "dummy-game",
		},
	}

	gs := &models.GameServer{
		ID:     "server-1",
		GameID: "dummy-game",
	}
	gs.R.Game = &models.Game{}

	inst.checkServerVersion(gs, eventbus.Get())

	state := inst.versionState.Get(gs.ID)
	if state.InstalledVersion != "1.0.0" {
		t.Errorf("InstalledVersion = %q, want %q", state.InstalledVersion, "1.0.0")
	}
	if state.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", state.LatestVersion, "2.0.0")
	}
	if !state.UpdateAvailable {
		t.Errorf("UpdateAvailable = false, want true")
	}
}

func TestCheckServerVersionPopulatesVersionsWhenUpToDate(t *testing.T) {
	t.Parallel()

	dummy := versiontracker.NewDummyTracker()
	dummy.MarkUpdated() // installed=2.0.0, latest=2.0.0 — no update
	inst := &Instance{
		ctx:          context.Background(),
		versionState: versiontracker.NewVersionStateMap(),
		resolverConfig: versiontracker.ResolverConfig{
			DummyTracker: dummy,
			DummyGameID:  "dummy-game",
		},
	}

	gs := &models.GameServer{
		ID:     "server-1",
		GameID: "dummy-game",
	}
	gs.R.Game = &models.Game{}

	inst.checkServerVersion(gs, eventbus.Get())

	state := inst.versionState.Get(gs.ID)
	if state.Status != versiontracker.VersionStatusChecked {
		t.Fatalf("status = %v, want %v", state.Status, versiontracker.VersionStatusChecked)
	}
	if state.InstalledVersion != "2.0.0" {
		t.Errorf("InstalledVersion = %q, want %q", state.InstalledVersion, "2.0.0")
	}
	if state.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", state.LatestVersion, "2.0.0")
	}
	if state.UpdateAvailable {
		t.Errorf("UpdateAvailable = true, want false")
	}
}

func TestQueryGameServersUsesNodeClientForRemoteOnlineServer(t *testing.T) {
	t.Parallel()

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "remote-node",
		GetProcessSnapshotResult: &node.ProcessSnapshot{
			ID:     "server-1",
			Status: xylona.Status_ONLINE.String(),
		},
		GetProcessSnapshotFound: true,
		QueryGameServerResult: node.GameServerQueryResult{
			Kind: node.GameServerQueryKindMinecraft,
			Minecraft: &node.MinecraftQueryInfo{
				NumberOfPlayers: 7,
				MaxPlayers:      20,
			},
		},
	}
	registry := noderegistry.New("local-node", &nodeclient.FakeNodeClient{NodeID: "local-node"})
	registry.Register(remoteClient)
	inst := &Instance{
		ctx:                  context.Background(),
		nodeRegistry:         registry,
		serverQueriesInfoMap: make(map[string]*xylona.ServerQuery),
		serverQueriesMutex:   &sync.RWMutex{},
	}
	gs := &models.GameServer{
		ID:         "server-1",
		Name:       "Remote Minecraft",
		NodeID:     "remote-node",
		IP:         "10.0.0.5",
		QueryPort:  25565,
		MaxPlayers: 20,
	}
	gs.R.Game = &models.Game{ID: "minecraft"}

	inst.queryGameServers(context.Background(), []*models.GameServer{gs})

	if len(remoteClient.QueryGameServerCalls) != 1 {
		t.Fatalf("QueryGameServer calls = %d, want 1", len(remoteClient.QueryGameServerCalls))
	}
	call := remoteClient.QueryGameServerCalls[0]
	if call.Kind != node.GameServerQueryKindMinecraft || call.IP != "10.0.0.5" || call.QueryPort != 25565 {
		t.Fatalf("QueryGameServer call = %+v, want Minecraft 10.0.0.5:25565", call)
	}

	inst.serverQueriesMutex.RLock()
	result := inst.serverQueriesInfoMap["server-1"]
	inst.serverQueriesMutex.RUnlock()
	if result.GetType() != xylona.ServerQuery_Minecraft {
		t.Fatalf("query type = %v, want Minecraft", result.GetType())
	}
	if result.GetMinecraft().GetNumberOfPlayers() != 7 {
		t.Fatalf("players = %d, want 7", result.GetMinecraft().GetNumberOfPlayers())
	}
}

func TestQueryGameServersKeepsOfflineDefaultForRemoteServer(t *testing.T) {
	t.Parallel()

	remoteClient := &nodeclient.FakeNodeClient{NodeID: "remote-node"}
	registry := noderegistry.New("local-node", &nodeclient.FakeNodeClient{NodeID: "local-node"})
	registry.Register(remoteClient)
	inst := &Instance{
		ctx:                  context.Background(),
		nodeRegistry:         registry,
		serverQueriesInfoMap: make(map[string]*xylona.ServerQuery),
		serverQueriesMutex:   &sync.RWMutex{},
	}
	gs := &models.GameServer{
		ID:         "server-1",
		Name:       "Remote Source",
		NodeID:     "remote-node",
		IP:         "10.0.0.5",
		QueryPort:  27015,
		MaxPlayers: 16,
	}
	gs.R.Game = &models.Game{ID: "source-game", UsesSourceQuery: true}

	inst.queryGameServers(context.Background(), []*models.GameServer{gs})

	if len(remoteClient.QueryGameServerCalls) != 0 {
		t.Fatalf("QueryGameServer calls = %d, want 0 for offline server", len(remoteClient.QueryGameServerCalls))
	}
	inst.serverQueriesMutex.RLock()
	result := inst.serverQueriesInfoMap["server-1"]
	inst.serverQueriesMutex.RUnlock()
	if result.GetType() != xylona.ServerQuery_Source {
		t.Fatalf("query type = %v, want Source", result.GetType())
	}
	if result.GetSource().GetMaxPlayers() != 16 {
		t.Fatalf("max players = %d, want 16", result.GetSource().GetMaxPlayers())
	}
}
