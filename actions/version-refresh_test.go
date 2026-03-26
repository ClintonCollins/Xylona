package actions

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type versionBroadcastCall struct {
	serverID    string
	version     string
	versionInfo *xylona.VersionInfo
}

type mockVersionBroadcaster struct {
	mu    sync.Mutex
	calls []versionBroadcastCall
}

func (m *mockVersionBroadcaster) BroadcastGameServerVersion(serverID string, version string, versionInfo *xylona.VersionInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, versionBroadcastCall{
		serverID:    serverID,
		version:     version,
		versionInfo: versionInfo,
	})
}

func (m *mockVersionBroadcaster) snapshot() []versionBroadcastCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]versionBroadcastCall, len(m.calls))
	copy(out, m.calls)
	return out
}

type versionRefreshTestTracker struct {
	mu              sync.Mutex
	installedCalls  int
	latestCalls     int
	latestVersion   string
	installedDelay  time.Duration
	latestDelay     time.Duration
	installedErr    error
	latestErr       error
	installedSource func(*models.GameServer) (string, error)
}

func (t *versionRefreshTestTracker) GetInstalledVersion(_ context.Context, gs *models.GameServer) (string, error) {
	if t.installedDelay > 0 {
		time.Sleep(t.installedDelay)
	}
	t.mu.Lock()
	t.installedCalls++
	t.mu.Unlock()
	if t.installedErr != nil {
		return "", t.installedErr
	}
	return t.installedSource(gs)
}

func (t *versionRefreshTestTracker) GetLatestVersion(_ context.Context, _ *models.GameServer) (string, error) {
	if t.latestDelay > 0 {
		time.Sleep(t.latestDelay)
	}
	t.mu.Lock()
	t.latestCalls++
	latest := t.latestVersion
	errLatest := t.latestErr
	t.mu.Unlock()
	return latest, errLatest
}

func (t *versionRefreshTestTracker) CheckForUpdate(ctx context.Context, gs *models.GameServer) (*versiontracker.UpdateInfo, error) {
	installed, errInstalled := t.GetInstalledVersion(ctx, gs)
	if errInstalled != nil {
		return nil, errInstalled
	}
	latest, errLatest := t.GetLatestVersion(ctx, gs)
	if errLatest != nil {
		return nil, errLatest
	}
	return &versiontracker.UpdateInfo{
		InstalledVersion: installed,
		LatestVersion:    latest,
		UpdateAvailable:  installed != latest,
	}, nil
}

func (t *versionRefreshTestTracker) counts() (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.installedCalls, t.latestCalls
}

func TestResolveVersionDataUsesInstalledAndLatestTTLs(t *testing.T) {
	tracker := &versionRefreshTestTracker{
		latestVersion: "2.0.0",
		installedSource: func(_ *models.GameServer) (string, error) {
			return "1.0.0", nil
		},
	}
	inst := &Instance{
		ctx:                 context.Background(),
		versionState:        versiontracker.NewVersionStateMap(),
		versionInstalledTTL: 15 * time.Second,
		versionLatestTTL:    2 * time.Minute,
		resolverConfig: versiontracker.ResolverConfig{
			CustomTrackerFactory: func(info versiontracker.TrackerContext) versiontracker.VersionTracker {
				if info.GameID == "dummy-game" {
					return tracker
				}
				return nil
			},
		},
	}

	gameServer := &models.GameServer{ID: "server-1", GameID: "dummy-game", Version: "1.0.0"}
	gameServer.R.Game = &models.Game{ID: "dummy-game", ServerSoftware: null.From("dummy")}

	_, state := inst.ResolveVersionData(context.Background(), gameServer, VersionResolveOptions{})
	if state.Status != versiontracker.VersionStatusChecked {
		t.Fatalf("ResolveVersionData() status = %v, want %v", state.Status, versiontracker.VersionStatusChecked)
	}
	installedCalls, latestCalls := tracker.counts()
	if installedCalls != 1 || latestCalls != 1 {
		t.Fatalf("initial tracker calls = (%d, %d), want (1, 1)", installedCalls, latestCalls)
	}

	_, _ = inst.ResolveVersionData(context.Background(), gameServer, VersionResolveOptions{})
	installedCalls, latestCalls = tracker.counts()
	if installedCalls != 1 || latestCalls != 1 {
		t.Fatalf("fresh tracker calls = (%d, %d), want (1, 1)", installedCalls, latestCalls)
	}

	stored, ok := inst.versionState.GetWithOK(gameServer.ID)
	if !ok {
		t.Fatal("expected stored version state")
	}
	stored.InstalledCheckTime = time.Now().Add(-16 * time.Second)
	stored.LatestCheckTime = time.Now()
	inst.versionState.Set(gameServer.ID, stored)

	_, _ = inst.ResolveVersionData(context.Background(), gameServer, VersionResolveOptions{})
	installedCalls, latestCalls = tracker.counts()
	if installedCalls != 2 || latestCalls != 1 {
		t.Fatalf("installed-stale tracker calls = (%d, %d), want (2, 1)", installedCalls, latestCalls)
	}
}

func TestResolveVersionDataRefreshesWhenTrackerContextChanges(t *testing.T) {
	tracker := &versionRefreshTestTracker{
		latestVersion: "2.0.0",
		installedSource: func(_ *models.GameServer) (string, error) {
			return "1.0.0", nil
		},
	}
	inst := &Instance{
		ctx:                 context.Background(),
		versionState:        versiontracker.NewVersionStateMap(),
		versionInstalledTTL: 15 * time.Second,
		versionLatestTTL:    2 * time.Minute,
		resolverConfig: versiontracker.ResolverConfig{
			CustomTrackerFactory: func(info versiontracker.TrackerContext) versiontracker.VersionTracker {
				if info.GameID == "dummy-game" {
					return tracker
				}
				return nil
			},
		},
	}

	gameServer := &models.GameServer{ID: "server-1", GameID: "dummy-game", Version: "1.0.0"}
	gameServer.R.Game = &models.Game{ID: "dummy-game", ServerSoftware: null.From("dummy")}

	inst.versionState.Set(gameServer.ID, versiontracker.VersionState{
		Status:             versiontracker.VersionStatusChecked,
		InstalledVersion:   "1.0.0",
		LatestVersion:      "26.1",
		UpdateAvailable:    true,
		LastCheckTime:      time.Now(),
		InstalledCheckTime: time.Now(),
		LatestCheckTime:    time.Now(),
		TrackerType:        "dummy",
		ContextKey:         "stale-context",
	})

	_, state := inst.ResolveVersionData(context.Background(), gameServer, VersionResolveOptions{})

	installedCalls, latestCalls := tracker.counts()
	if installedCalls != 1 || latestCalls != 1 {
		t.Fatalf("tracker calls after context change = (%d, %d), want (1, 1)", installedCalls, latestCalls)
	}
	if state.LatestVersion != "2.0.0" {
		t.Fatalf("latest version = %q, want %q", state.LatestVersion, "2.0.0")
	}
	if state.ContextKey == "" || state.ContextKey == "stale-context" {
		t.Fatalf("context key = %q, want refreshed key", state.ContextKey)
	}
}

func TestRefreshVersionStatePreservesPartialDataAndReportsError(t *testing.T) {
	baseTime := time.Date(2026, time.March, 23, 12, 0, 0, 0, time.UTC)
	testCases := []struct {
		name                string
		installedErr        error
		latestErr           error
		wantInstalled       string
		wantLatest          string
		wantInstalledUpdate bool
		wantLatestUpdate    bool
	}{
		{
			name:                "installed probe fails",
			installedErr:        context.Canceled,
			wantInstalled:       "1.0.0",
			wantLatest:          "2.1.0",
			wantInstalledUpdate: false,
			wantLatestUpdate:    true,
		},
		{
			name:                "latest probe fails",
			latestErr:           context.DeadlineExceeded,
			wantInstalled:       "1.1.0",
			wantLatest:          "2.0.0",
			wantInstalledUpdate: true,
			wantLatestUpdate:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tracker := &versionRefreshTestTracker{
				latestVersion: "2.1.0",
				installedErr:  tc.installedErr,
				latestErr:     tc.latestErr,
				installedSource: func(_ *models.GameServer) (string, error) {
					return "1.1.0", nil
				},
			}
			inst := &Instance{
				versionState: versiontracker.NewVersionStateMap(),
			}

			initialState := versiontracker.VersionState{
				Status:             versiontracker.VersionStatusChecked,
				InstalledVersion:   "1.0.0",
				LatestVersion:      "2.0.0",
				UpdateAvailable:    true,
				LastCheckTime:      baseTime.Add(-10 * time.Minute),
				InstalledCheckTime: baseTime.Add(-10 * time.Minute),
				LatestCheckTime:    baseTime.Add(-5 * time.Minute),
			}
			inst.versionState.Set("server-1", initialState)

			gameServer := &models.GameServer{ID: "server-1", GameID: "dummy-game"}
			inst.refreshVersionState(context.Background(), gameServer, tracker, "dummy", true, true)

			stored, ok := inst.versionState.GetWithOK("server-1")
			if !ok {
				t.Fatal("expected stored version state")
			}
			if stored.Status != versiontracker.VersionStatusError {
				t.Fatalf("status = %v, want %v", stored.Status, versiontracker.VersionStatusError)
			}
			if stored.InstalledVersion != tc.wantInstalled {
				t.Fatalf("installed version = %q, want %q", stored.InstalledVersion, tc.wantInstalled)
			}
			if stored.LatestVersion != tc.wantLatest {
				t.Fatalf("latest version = %q, want %q", stored.LatestVersion, tc.wantLatest)
			}
			if stored.UpdateAvailable != true {
				t.Fatalf("update available = %v, want true", stored.UpdateAvailable)
			}
			if tc.wantInstalledUpdate && !stored.InstalledCheckTime.After(initialState.InstalledCheckTime) {
				t.Fatalf("installed check time = %v, want newer than %v", stored.InstalledCheckTime, initialState.InstalledCheckTime)
			}
			if !tc.wantInstalledUpdate && !stored.InstalledCheckTime.Equal(initialState.InstalledCheckTime) {
				t.Fatalf("installed check time = %v, want %v", stored.InstalledCheckTime, initialState.InstalledCheckTime)
			}
			if tc.wantLatestUpdate && !stored.LatestCheckTime.After(initialState.LatestCheckTime) {
				t.Fatalf("latest check time = %v, want newer than %v", stored.LatestCheckTime, initialState.LatestCheckTime)
			}
			if !tc.wantLatestUpdate && !stored.LatestCheckTime.Equal(initialState.LatestCheckTime) {
				t.Fatalf("latest check time = %v, want %v", stored.LatestCheckTime, initialState.LatestCheckTime)
			}
			if stored.LastCheckTime.Before(initialState.LastCheckTime) {
				t.Fatalf("last check time = %v, want at least %v", stored.LastCheckTime, initialState.LastCheckTime)
			}
		})
	}
}

func TestResolveVersionDataCoalescesConcurrentRefreshes(t *testing.T) {
	tracker := &versionRefreshTestTracker{
		latestVersion:  "2.0.0",
		installedDelay: 40 * time.Millisecond,
		installedSource: func(_ *models.GameServer) (string, error) {
			return "1.0.0", nil
		},
	}
	inst := &Instance{
		ctx:                 context.Background(),
		versionState:        versiontracker.NewVersionStateMap(),
		versionInstalledTTL: 15 * time.Second,
		versionLatestTTL:    2 * time.Minute,
		resolverConfig: versiontracker.ResolverConfig{
			CustomTrackerFactory: func(info versiontracker.TrackerContext) versiontracker.VersionTracker {
				if info.GameID == "dummy-game" {
					return tracker
				}
				return nil
			},
		},
	}

	gameServer := &models.GameServer{ID: "server-1", GameID: "dummy-game", Version: "1.0.0"}
	gameServer.R.Game = &models.Game{ID: "dummy-game", ServerSoftware: null.From("dummy")}

	_, _ = inst.ResolveVersionData(context.Background(), gameServer, VersionResolveOptions{})
	stored, ok := inst.versionState.GetWithOK(gameServer.ID)
	if !ok {
		t.Fatal("expected stored version state")
	}
	stored.InstalledCheckTime = time.Now().Add(-16 * time.Second)
	stored.LatestCheckTime = time.Now()
	inst.versionState.Set(gameServer.ID, stored)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, _ = inst.ResolveVersionData(context.Background(), gameServer, VersionResolveOptions{})
		})
	}
	wg.Wait()

	installedCalls, latestCalls := tracker.counts()
	if installedCalls != 2 || latestCalls != 1 {
		t.Fatalf("tracker calls after concurrent refresh = (%d, %d), want (2, 1)", installedCalls, latestCalls)
	}
}

func TestCheckAllServerVersionsDetectsOutOfBandMinecraftJarReplacement(t *testing.T) {
	inst := newTestInstance(t)
	tracker := &versionRefreshTestTracker{
		latestVersion: "1.21.5",
		installedSource: func(gs *models.GameServer) (string, error) {
			return versiontracker.ReadMinecraftJarVersion(gs.Directory, gs.ServerExecutable.GetOr(""))
		},
	}
	broadcaster := &mockVersionBroadcaster{}
	inst.versionState = versiontracker.NewVersionStateMap()
	inst.versionInstalledTTL = 15 * time.Second
	inst.versionLatestTTL = 2 * time.Minute
	inst.resolverConfig = versiontracker.ResolverConfig{
		CustomTrackerFactory: func(info versiontracker.TrackerContext) versiontracker.VersionTracker {
			if info.GameID == "minecraft" {
				return tracker
			}
			return nil
		},
	}
	inst.SetVersionBroadcaster(broadcaster)

	serverDir := t.TempDir()
	createVersionTestMinecraftJar(t, serverDir, "server.jar", "1.20.4")

	_, errGame := inst.db.SQLDb.ExecContext(
		context.Background(),
		`insert into game (
			id,
			name,
			default_port,
			default_query_port,
			default_max_players,
			linux_support,
			windows_support,
			linux_start_args_template,
			windows_start_args_template,
			linux_base_command,
			windows_base_command,
			created_at,
			updated_at,
			server_software
		)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?)
		 on conflict(id) do update set server_software = excluded.server_software`,
		"minecraft",
		"Minecraft",
		25565,
		25565,
		20,
		true,
		true,
		`[{"id":"jar","order":0,"ownership":"system","tokens":["-jar","server.jar"],"label":"Jar"}]`,
		`[{"id":"jar","order":0,"ownership":"system","tokens":["-jar","server.jar"],"label":"Jar"}]`,
		"java",
		"java",
		`[{"id":"paper","name":"Paper","jar_source":"paper"}]`,
	)
	if errGame != nil {
		t.Fatalf("insert game: %v", errGame)
	}

	_, errNode := inst.db.SQLDb.ExecContext(
		context.Background(),
		`insert into node (id, name, is_local, host, port, base_url, enabled) values (?, ?, ?, ?, ?, ?, ?)
		 on conflict(id) do nothing`,
		"node-local", "Local", true, "localhost", 8080, "http://localhost:8080", true,
	)
	if errNode != nil {
		t.Fatalf("insert node: %v", errNode)
	}

	_, errIP := inst.db.SQLDb.ExecContext(
		context.Background(),
		`insert into ip (address, usable, external) values (?, ?, ?)
		 on conflict(address) do nothing`,
		"127.0.0.1", true, false,
	)
	if errIP != nil {
		t.Fatalf("insert ip: %v", errIP)
	}

	_, errUser := inst.db.SQLDb.ExecContext(
		context.Background(),
		`insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, created_at, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 on conflict(id) do nothing`,
		"user-1", "owner", "owner@example.com", "Owner", "User", "hash", false,
	)
	if errUser != nil {
		t.Fatalf("insert user: %v", errUser)
	}

	_, errServer := inst.db.InsertGameServer(inst.db.DB, &models.GameServerSetter{
		ID:               omit.From("server-1"),
		UserID:           omit.From("user-1"),
		Name:             omit.From("Local Minecraft"),
		GameID:           omit.From("minecraft"),
		StartArgsPatches: omit.From("[]"),
		Status:           omit.From("OFFLINE"),
		SetPlayers:       omit.From(int64(20)),
		MaxPlayers:       omit.From(int64(20)),
		Map:              omit.From("world"),
		IP:               omit.From("127.0.0.1"),
		Port:             omit.From(int64(25565)),
		QueryPort:        omit.From(int64(25565)),
		Directory:        omit.From(serverDir),
		NodeID:           omit.From("node-local"),
		ServerExecutable: omitnull.From("server.jar"),
		Version:          omit.From("1.20.4"),
	})
	if errServer != nil {
		t.Fatalf("insert server: %v", errServer)
	}

	inst.checkAllServerVersions()
	initial := inst.versionState.Get("server-1")
	if initial.InstalledVersion != "1.20.4" {
		t.Fatalf("initial installed version = %q, want %q", initial.InstalledVersion, "1.20.4")
	}

	createVersionTestMinecraftJar(t, serverDir, "server.jar", "1.21.5")
	inst.checkAllServerVersions()

	updated := inst.versionState.Get("server-1")
	if updated.InstalledVersion != "1.21.5" {
		t.Fatalf("updated installed version = %q, want %q", updated.InstalledVersion, "1.21.5")
	}

	createVersionTestMinecraftJar(t, serverDir, "server.jar", "1.19.4")
	inst.checkAllServerVersions()

	downgraded := inst.versionState.Get("server-1")
	if downgraded.InstalledVersion != "1.19.4" {
		t.Fatalf("downgraded installed version = %q, want %q", downgraded.InstalledVersion, "1.19.4")
	}

	calls := broadcaster.snapshot()
	if len(calls) < 3 {
		t.Fatalf("version broadcaster call count = %d, want at least 3", len(calls))
	}
	if calls[len(calls)-1].version != "1.19.4" {
		t.Fatalf("last broadcast version = %q, want %q", calls[len(calls)-1].version, "1.19.4")
	}
}

func createVersionTestMinecraftJar(t *testing.T, dir string, fileName string, version string) {
	t.Helper()

	jarPath := filepath.Join(dir, fileName)
	errRemove := os.Remove(jarPath)
	if errRemove != nil && !os.IsNotExist(errRemove) {
		t.Fatalf("remove existing jar: %v", errRemove)
	}

	file, errCreate := os.Create(jarPath)
	if errCreate != nil {
		t.Fatalf("create jar: %v", errCreate)
	}
	defer func() {
		if errClose := file.Close(); errClose != nil {
			t.Errorf("close jar file: %v", errClose)
		}
	}()

	zw := zip.NewWriter(file)
	defer func() {
		if errClose := zw.Close(); errClose != nil {
			t.Errorf("close zip writer: %v", errClose)
		}
	}()

	w, errEntry := zw.Create("version.json")
	if errEntry != nil {
		t.Fatalf("create version.json: %v", errEntry)
	}

	versionJSON := []byte(`{"id":"` + version + `","name":"` + version + `"}`)
	if _, errWrite := w.Write(versionJSON); errWrite != nil {
		t.Fatalf("write version.json: %v", errWrite)
	}
}
