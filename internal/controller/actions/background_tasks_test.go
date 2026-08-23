package actions

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"

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

func TestServerQueryFromNodeResultPreservesSourcePlayerList(t *testing.T) {
	t.Parallel()

	gameServer := &models.GameServer{ID: "server-1", Name: "Remote Source"}
	result := serverQueryFromNodeResult(gameServer, node.GameServerQueryResult{
		Kind: node.GameServerQueryKindSource,
		Source: &node.SourceQueryInfo{
			Players:             2,
			MaxPlayers:          24,
			PlayerList:          []string{"Alyx", "Gordon"},
			PlayerListSupported: true,
		},
	})

	if result.GetType() != xylona.ServerQuery_Source || result.GetSource() == nil {
		t.Fatalf("server query = %+v, want Source payload", result)
	}
	if !result.GetSource().GetPlayerListSupported() || !slices.Equal(result.GetSource().GetPlayerList(), []string{"Alyx", "Gordon"}) {
		t.Fatalf("Source player data = %+v, want supported [Alyx Gordon]", result.GetSource())
	}
}

func TestFillSourcePlayerNamesFromSevenDaysToDieMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		source        *xylona.SourceQueryInfo
		snapshot      *node.SevenDaysToDieMapSnapshot
		wantNames     []string
		wantSupported bool
	}{
		{
			name:   "fills a blank A2S roster",
			source: &xylona.SourceQueryInfo{Players: 1, PlayerListSupported: true},
			snapshot: &node.SevenDaysToDieMapSnapshot{Players: []node.SevenDaysToDieMapPlayer{
				{Name: "Alex", Online: true},
			}},
			wantNames:     []string{"Alex"},
			wantSupported: true,
		},
		{
			name: "appends dashboard names to a partial A2S roster",
			source: &xylona.SourceQueryInfo{
				Players:             3,
				PlayerList:          []string{"Alyx"},
				PlayerListSupported: true,
			},
			snapshot: &node.SevenDaysToDieMapSnapshot{Players: []node.SevenDaysToDieMapPlayer{
				{Name: "Gordon", Online: true},
				{Name: "Chell", Online: true},
			}},
			wantNames:     []string{"Alyx", "Gordon", "Chell"},
			wantSupported: true,
		},
		{
			name: "preserves a complete A2S roster",
			source: &xylona.SourceQueryInfo{
				Players:             1,
				PlayerList:          []string{"Alyx"},
				PlayerListSupported: true,
			},
			snapshot: &node.SevenDaysToDieMapSnapshot{Players: []node.SevenDaysToDieMapPlayer{
				{Name: "Alex", Online: true},
			}},
			wantNames:     []string{"Alyx"},
			wantSupported: true,
		},
		{
			name:   "uses only current usable names up to the A2S count",
			source: &xylona.SourceQueryInfo{Players: 1},
			snapshot: &node.SevenDaysToDieMapSnapshot{Players: []node.SevenDaysToDieMapPlayer{
				{Name: "Old", Online: false},
				{Name: " ", Online: true},
				{Name: "Current", Online: true},
				{Name: "Joined later", Online: true},
			}},
			wantNames:     []string{"Current"},
			wantSupported: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fillSourcePlayerNamesFromSevenDaysToDieMap(test.source, test.snapshot)

			if !slices.Equal(test.source.GetPlayerList(), test.wantNames) {
				t.Fatalf("player list = %v, want %v", test.source.GetPlayerList(), test.wantNames)
			}
			if test.source.GetPlayerListSupported() != test.wantSupported {
				t.Fatalf("player list supported = %t, want %t", test.source.GetPlayerListSupported(), test.wantSupported)
			}
		})
	}
}

func TestQueryGameServersFillsSevenDaysToDieRosterGapsFromDashboard(t *testing.T) {
	inst := newTestInstance(t)
	inst.db.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	_, errNode := inst.db.SQLDb.ExecContext(
		t.Context(),
		`insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)
		 on conflict(id) do nothing`,
		"node-remote", "Remote Node", "http://localhost:8081", true,
	)
	if errNode != nil {
		t.Fatalf("insert node setup error = %v", errNode)
	}
	_, errIP := inst.db.SQLDb.ExecContext(
		t.Context(),
		`insert into ip (address, usable, external, node_id) values (?, ?, ?, ?)
		 on conflict(address, node_id) do nothing`,
		"127.0.0.1", true, false, "node-remote",
	)
	if errIP != nil {
		t.Fatalf("insert IP setup error = %v", errIP)
	}
	_, errUser := inst.db.SQLDb.ExecContext(
		t.Context(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 on conflict(id) do nothing`,
		"user-roster", "owner", "owner@example.com", "Owner", "User", "hash", false,
	)
	if errUser != nil {
		t.Fatalf("insert user setup error = %v", errUser)
	}
	directory := t.TempDir()
	_, errServer := inst.db.InsertGameServer(inst.db.DB, &models.GameServerSetter{
		ID:               omit.From("server-roster"),
		UserID:           omit.From("user-roster"),
		Name:             omit.From("7DTD Server"),
		GameID:           omit.From(sevenDaysToDieGameID),
		Status:           omit.From(xylona.Status_ONLINE.String()),
		SetPlayers:       omit.From(int64(32)),
		MaxPlayers:       omit.From(int64(32)),
		Map:              omit.From("world"),
		IP:               omit.From("127.0.0.1"),
		Port:             omit.From(int64(26900)),
		QueryPort:        omit.From(int64(26904)),
		Directory:        omit.From(directory),
		NodeID:           omit.From("node-remote"),
		StartArgsPatches: omit.From("[]"),
	})
	if errServer != nil {
		t.Fatalf("InsertGameServer() error = %v", errServer)
	}

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-remote",
		GetProcessSnapshotResult: &node.ProcessSnapshot{
			ID:     "server-roster",
			Status: xylona.Status_ONLINE.String(),
		},
		GetProcessSnapshotFound: true,
		QueryGameServerResult: node.GameServerQueryResult{
			Kind: node.GameServerQueryKindSource,
			Source: &node.SourceQueryInfo{
				Players:   1,
				Responded: true,
			},
		},
		QuerySevenDaysToDieMapResult: &node.SevenDaysToDieMapSnapshot{
			Players: []node.SevenDaysToDieMapPlayer{{Name: "Alex", Online: true}},
		},
	}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(remoteClient)
	inst.nodeRegistry = registry
	gameServer := &models.GameServer{
		ID:        "server-roster",
		Name:      "7DTD Server",
		GameID:    sevenDaysToDieGameID,
		Directory: directory,
		NodeID:    "node-remote",
		Port:      26900,
		QueryPort: 26904,
	}
	gameServer.R.Game = adminInputTestGameDefinition(sevenDaysToDieGameID)
	gameServer.R.Game.UsesSourceQuery = true

	inst.queryGameServers(t.Context(), []*models.GameServer{gameServer})
	inst.serverQueriesMutex.RLock()
	result := inst.serverQueriesInfoMap[gameServer.ID]
	inst.serverQueriesMutex.RUnlock()
	if !slices.Equal(result.GetSource().GetPlayerList(), []string{"Alex"}) {
		t.Fatalf("player list = %v, want [Alex]", result.GetSource().GetPlayerList())
	}
	if len(remoteClient.QueryGameServerCalls) != 1 {
		t.Fatalf("A2S calls = %d, want 1", len(remoteClient.QueryGameServerCalls))
	}
	if len(remoteClient.QuerySevenDaysToDieMapCalls) != 1 {
		t.Fatalf("dashboard calls = %d, want 1", len(remoteClient.QuerySevenDaysToDieMapCalls))
	}
	request := remoteClient.QuerySevenDaysToDieMapCalls[0]
	if request.WorkingDirectory != directory || request.TokenName == "" || request.TokenSecret == "" || request.IncludeTactical {
		t.Fatal("dashboard request omitted base fields or unexpectedly included tactical data")
	}

	remoteClient.QueryGameServerResult.Source = &node.SourceQueryInfo{
		Players:             1,
		PlayerList:          []string{"Alyx"},
		PlayerListSupported: true,
		Responded:           true,
	}
	inst.queryGameServers(t.Context(), []*models.GameServer{gameServer})
	if len(remoteClient.QuerySevenDaysToDieMapCalls) != 1 {
		t.Fatalf("dashboard calls = %d, want no fallback for a complete A2S roster", len(remoteClient.QuerySevenDaysToDieMapCalls))
	}

	remoteClient.QueryGameServerResult.Source = &node.SourceQueryInfo{
		Players:             2,
		PlayerList:          []string{"Alyx"},
		PlayerListSupported: true,
		Responded:           true,
	}
	remoteClient.QuerySevenDaysToDieMapErr = errors.New("dashboard unavailable")
	inst.queryGameServers(t.Context(), []*models.GameServer{gameServer})
	inst.serverQueriesMutex.RLock()
	result = inst.serverQueriesInfoMap[gameServer.ID]
	inst.serverQueriesMutex.RUnlock()
	if !slices.Equal(result.GetSource().GetPlayerList(), []string{"Alyx"}) {
		t.Fatalf("player list after dashboard failure = %v, want preserved A2S roster", result.GetSource().GetPlayerList())
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
				Responded:       true,
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
	telemetry := inst.GetGameServerQueryTelemetry("server-1")
	if telemetry.Status != GameServerQueryTelemetryStatusSuccess {
		t.Fatalf("telemetry status = %q, want success", telemetry.Status)
	}
	if !telemetry.PlayerCountValid || telemetry.PlayerCount != 7 {
		t.Fatalf("telemetry player count = (%d, %t), want (7, true)", telemetry.PlayerCount, telemetry.PlayerCountValid)
	}
	if !telemetry.PlayerCapacityValid || telemetry.PlayerCapacity != 20 {
		t.Fatalf("telemetry player capacity = (%d, %t), want (20, true)", telemetry.PlayerCapacity, telemetry.PlayerCapacityValid)
	}
	status := inst.GetCachedGameServerStatus("server-1")
	if status != xylona.Status_ONLINE {
		t.Fatalf("cached status = %v, want online", status)
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
	telemetry := inst.GetGameServerQueryTelemetry("server-1")
	if telemetry.Status != GameServerQueryTelemetryStatusUnavailable {
		t.Fatalf("telemetry status = %q, want unavailable", telemetry.Status)
	}
	status := inst.GetCachedGameServerStatus("server-1")
	if status != xylona.Status_OFFLINE {
		t.Fatalf("cached status = %v, want offline", status)
	}
}

func TestQueryGameServersRecordsRemoteQueryFailureBeforeFallback(t *testing.T) {
	t.Parallel()

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "remote-node",
		GetProcessSnapshotResult: &node.ProcessSnapshot{
			ID:     "server-1",
			Status: xylona.Status_ONLINE.String(),
		},
		GetProcessSnapshotFound: true,
		QueryGameServerErr:      errors.New("query unavailable"),
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

	telemetry := inst.GetGameServerQueryTelemetry("server-1")
	if telemetry.Status != GameServerQueryTelemetryStatusFailure {
		t.Fatalf("telemetry status = %q, want failure", telemetry.Status)
	}
	if telemetry.CheckedAt.IsZero() {
		t.Fatal("failure telemetry checked_at is zero")
	}
	if telemetry.DurationValid {
		t.Fatal("failed query must not produce a valid duration sample")
	}
	if telemetry.PlayerCountValid || telemetry.PlayerCapacityValid {
		t.Fatalf("failure player valid flags = (%t, %t), want false false", telemetry.PlayerCountValid, telemetry.PlayerCapacityValid)
	}

	inst.serverQueriesMutex.RLock()
	result := inst.serverQueriesInfoMap["server-1"]
	inst.serverQueriesMutex.RUnlock()
	if result.GetMinecraft().GetMaxPlayers() != 20 {
		t.Fatalf("fallback max players = %d, want 20", result.GetMinecraft().GetMaxPlayers())
	}
}

func TestGameServerQueryTelemetrySnapshots(t *testing.T) {
	tests := []struct {
		name     string
		record   func(*Instance, time.Time)
		want     GameServerQueryTelemetryStatus
		wantType xylona.ServerQuery_Type
		valid    bool
		players  uint32
		capacity uint32
		palworld bool
		duration bool
	}{
		{
			name:   "not yet queried is explicit",
			record: func(_ *Instance, _ time.Time) {},
			want:   GameServerQueryTelemetryStatusNotYetQueried,
		},
		{
			name: "unsupported is explicit",
			record: func(inst *Instance, _ time.Time) {
				inst.recordUnsupportedGameServerQuery("server-1")
			},
			want: GameServerQueryTelemetryStatusUnsupported,
		},
		{
			name: "minecraft success preserves zero players as known",
			record: func(inst *Instance, startedAt time.Time) {
				inst.recordSuccessfulGameServerQuery("server-1", xylona.ServerQuery_Minecraft, startedAt, &xylona.ServerQuery{
					Type:      xylona.ServerQuery_Minecraft,
					Minecraft: &xylona.MinecraftQueryInfo{MaxPlayers: 20},
				})
			},
			want:     GameServerQueryTelemetryStatusSuccess,
			wantType: xylona.ServerQuery_Minecraft,
			valid:    true,
			capacity: 20,
			duration: true,
		},
		{
			name: "palworld success includes typed performance values",
			record: func(inst *Instance, startedAt time.Time) {
				inst.recordSuccessfulGameServerQuery("server-1", xylona.ServerQuery_Palworld, startedAt, &xylona.ServerQuery{
					Type: xylona.ServerQuery_Palworld,
					Palworld: &xylona.PalworldQueryInfo{
						Players:           5,
						MaxPlayers:        32,
						ServerFps:         59.5,
						ServerFrameTimeMs: 16.8,
						UptimeSeconds:     7200,
					},
				})
			},
			want:     GameServerQueryTelemetryStatusSuccess,
			wantType: xylona.ServerQuery_Palworld,
			valid:    true,
			players:  5,
			capacity: 32,
			palworld: true,
			duration: true,
		},
		{
			name: "failure preserves last success but marks values unknown",
			record: func(inst *Instance, startedAt time.Time) {
				inst.recordSuccessfulGameServerQuery("server-1", xylona.ServerQuery_Source, startedAt, &xylona.ServerQuery{
					Type:   xylona.ServerQuery_Source,
					Source: &xylona.SourceQueryInfo{Players: 3, MaxPlayers: 12},
				})
				inst.recordFailedGameServerQuery("server-1", xylona.ServerQuery_Source, startedAt)
			},
			want:     GameServerQueryTelemetryStatusFailure,
			wantType: xylona.ServerQuery_Source,
		},
		{
			name: "offline query becomes unavailable without a synthetic duration",
			record: func(inst *Instance, startedAt time.Time) {
				inst.recordSuccessfulGameServerQuery("server-1", xylona.ServerQuery_Source, startedAt, &xylona.ServerQuery{
					Type:   xylona.ServerQuery_Source,
					Source: &xylona.SourceQueryInfo{Players: 3, MaxPlayers: 12},
				})
				inst.recordUnavailableGameServerQuery("server-1", xylona.ServerQuery_Source)
			},
			want:     GameServerQueryTelemetryStatusUnavailable,
			wantType: xylona.ServerQuery_Source,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inst := &Instance{}
			startedAt := time.Now().Add(-time.Millisecond)
			test.record(inst, startedAt)

			snapshot := inst.GetGameServerQueryTelemetry("server-1")
			if snapshot.Status != test.want {
				t.Fatalf("status = %q, want %q", snapshot.Status, test.want)
			}
			if snapshot.QueryType != test.wantType {
				t.Fatalf("query type = %v, want %v", snapshot.QueryType, test.wantType)
			}
			if snapshot.PlayerCountValid != test.valid || snapshot.PlayerCapacityValid != test.valid {
				t.Fatalf("player valid flags = (%t, %t), want (%t, %t)", snapshot.PlayerCountValid, snapshot.PlayerCapacityValid, test.valid, test.valid)
			}
			if test.valid && (snapshot.PlayerCount != test.players || snapshot.PlayerCapacity != test.capacity) {
				t.Fatalf("player values = (%d, %d), want (%d, %d)", snapshot.PlayerCount, snapshot.PlayerCapacity, test.players, test.capacity)
			}
			if test.palworld && (!snapshot.PalworldServerFPSValid || !snapshot.PalworldFrameTimeMSValid || !snapshot.PalworldUptimeSecondsValid) {
				t.Fatalf("Palworld valid flags = (%t, %t, %t), want all true", snapshot.PalworldServerFPSValid, snapshot.PalworldFrameTimeMSValid, snapshot.PalworldUptimeSecondsValid)
			}
			if snapshot.DurationValid != test.duration {
				t.Fatalf("duration valid = %t, want %t", snapshot.DurationValid, test.duration)
			}
			if (test.want == GameServerQueryTelemetryStatusFailure || test.want == GameServerQueryTelemetryStatusUnavailable) && snapshot.LastSuccessAt.IsZero() {
				t.Fatal("unavailable snapshot did not preserve last success timestamp")
			}
		})
	}
}
