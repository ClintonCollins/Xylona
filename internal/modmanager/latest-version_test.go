package modmanager

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/internal/db/dbtest"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/modproviders/thunderstore"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// steamWorkshopMock stands in for the real Steam Workshop provider, which has
// versions to report but cannot install them.
var steamWorkshopMock = sync.OnceValue(func() *mockProvider {
	provider := newMockProvider("steam_workshop")
	provider.versions = []modproviders.ModVersion{{VersionID: "2", VersionString: "updated_20260101"}}
	modproviders.RegisterProvider(provider)
	return provider
})

func registerVersionsProvider(t *testing.T, versions []modproviders.ModVersion, versionsErr error) *mockProvider {
	t.Helper()
	provider := newMockProvider(newUniqueProviderID())
	provider.versions = versions
	provider.versionsErr = versionsErr
	modproviders.RegisterProvider(provider)
	return provider
}

func TestNewestCompatibleVersion(t *testing.T) {
	versions := []modproviders.ModVersion{
		{VersionID: "v4", GameVersions: []string{"1.21.4"}},
		{VersionID: "v3", GameVersions: []string{"1.21.1", "1.21.4"}},
		{VersionID: "v2", GameVersions: []string{"1.20.1", "1.21.1"}},
		{VersionID: "v1", GameVersions: []string{"1.20.1"}},
	}
	tests := []struct {
		name               string
		versions           []modproviders.ModVersion
		installedVersionID string
		gameVersionUnknown bool
		wantID             string
		wantFound          bool
	}{
		{name: "no versions", installedVersionID: "v1", gameVersionUnknown: true},
		{name: "known game version takes the newest", versions: versions, installedVersionID: "v1", wantID: "v4", wantFound: true},
		{name: "unknown game version keeps to the installed game versions", versions: versions, installedVersionID: "v1", gameVersionUnknown: true, wantID: "v2", wantFound: true},
		{name: "unknown game version follows shared game versions", versions: versions, installedVersionID: "v2", gameVersionUnknown: true, wantID: "v3", wantFound: true},
		{name: "newest already installed", versions: versions, installedVersionID: "v4", gameVersionUnknown: true, wantID: "v4", wantFound: true},
		{name: "installed version no longer listed", versions: versions, installedVersionID: "v0", gameVersionUnknown: true, wantID: "v4", wantFound: true},
		{
			name:               "provider without game versions",
			versions:           []modproviders.ModVersion{{VersionID: "2.0.0"}, {VersionID: "1.0.0"}},
			installedVersionID: "1.0.0",
			gameVersionUnknown: true,
			wantID:             "2.0.0",
			wantFound:          true,
		},
		{
			name: "no newer version shares a game version",
			versions: []modproviders.ModVersion{
				{VersionID: "v2", GameVersions: []string{"1.21"}},
				{VersionID: "v1", GameVersions: []string{"1.20"}},
			},
			installedVersionID: "v1",
			gameVersionUnknown: true,
			wantID:             "v1",
			wantFound:          true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, found := newestCompatibleVersion(tc.versions, tc.installedVersionID, tc.gameVersionUnknown)
			if found != tc.wantFound || got.VersionID != tc.wantID {
				t.Fatalf("newestCompatibleVersion() = %q, %v; want %q, %v", got.VersionID, found, tc.wantID, tc.wantFound)
			}
		})
	}
}

func TestNewUpdateScope(t *testing.T) {
	paperConfig := `{"update_provider":{"kind":"papermc","source_id":"paper"},"mod_profile":{"install_path":"plugins","sources":[` +
		`{"id":"modrinth","search_params_json":"{\"facets\":{\"categories\":[\"paper\"]}}"},` +
		`{"id":"hangar","search_params_json":"{\"platform\":\"PAPER\"}"}]}}`
	tests := []struct {
		name            string
		game            *models.Game
		server          *models.GameServer
		wantGameVersion string
		wantParams      map[string]modproviders.SearchParams
	}{
		{
			name:            "pinned paper target is the game version",
			game:            &models.Game{ID: "minecraft", ServerSoftware: null.From(paperConfig)},
			server:          &models.GameServer{GameID: "minecraft", Branch: "1.21.4", TargetPinned: true},
			wantGameVersion: "1.21.4",
			wantParams: map[string]modproviders.SearchParams{
				"modrinth": {"facets": map[string]any{"categories": []any{"paper"}}},
				"hangar":   {"platform": "PAPER"},
			},
		},
		{
			name:       "paper tracking latest has no game version",
			game:       &models.Game{ID: "minecraft", ServerSoftware: null.From(paperConfig)},
			server:     &models.GameServer{GameID: "minecraft", Branch: "1.21.4"},
			wantParams: map[string]modproviders.SearchParams{"modrinth": {"facets": map[string]any{"categories": []any{"paper"}}}, "hangar": {"platform": "PAPER"}},
		},
		{
			name:       "steam branch is not a game version",
			game:       &models.Game{ID: "valheim", ServerSoftware: null.From(`{"update_provider":{"kind":"steamcmd"}}`)},
			server:     &models.GameServer{GameID: "valheim", Branch: "public-test"},
			wantParams: map[string]modproviders.SearchParams{"thunderstore": {"community": "valheim"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scope, errScope := NewUpdateScope(tc.game, tc.server)
			if errScope != nil {
				t.Fatalf("NewUpdateScope() error = %v", errScope)
			}
			if scope.GameID != tc.server.GameID || scope.GameVersion != tc.wantGameVersion {
				t.Fatalf("NewUpdateScope() game = %q version = %q, want %q %q", scope.GameID, scope.GameVersion, tc.server.GameID, tc.wantGameVersion)
			}
			if !reflect.DeepEqual(scope.Params, tc.wantParams) {
				t.Fatalf("NewUpdateScope() params = %#v, want %#v", scope.Params, tc.wantParams)
			}
		})
	}
}

func TestLatestVersion(t *testing.T) {
	steamWorkshopMock()
	errProviderDown := errors.New("provider down")
	listed := registerVersionsProvider(t, []modproviders.ModVersion{
		{VersionID: "v2", VersionString: "2.0.0"},
		{VersionID: "v1", VersionString: "1.0.0"},
	}, nil)
	empty := registerVersionsProvider(t, nil, nil)
	failing := registerVersionsProvider(t, nil, errProviderDown)

	tests := []struct {
		name    string
		mod     *models.InstalledMod
		scope   UpdateScope
		wantID  string
		wantErr error
	}{
		{
			name:   "newest listed version",
			mod:    &models.InstalledMod{Source: listed.ID(), SourceID: "mod", InstalledVersionID: "v1"},
			wantID: "v2",
		},
		{
			name:    "steam workshop cannot install a newer version",
			mod:     &models.InstalledMod{Source: "steam_workshop", SourceID: "123", InstalledVersionID: "1"},
			wantErr: ErrLatestUnsupported,
		},
		{
			name:    "unregistered source",
			mod:     &models.InstalledMod{Source: "missing-provider", SourceID: "mod", InstalledVersionID: "v1"},
			wantErr: ErrLatestUnsupported,
		},
		{
			name:   "valheim loader stays on the verified release",
			mod:    &models.InstalledMod{Source: "thunderstore", SourceID: thunderstore.ValheimLoaderID, InstalledVersionID: "5.4.2200"},
			scope:  UpdateScope{GameID: "valheim"},
			wantID: thunderstore.ValheimLoaderVersion,
		},
		{
			name:    "provider lists no versions",
			mod:     &models.InstalledMod{Source: empty.ID(), SourceID: "mod", InstalledVersionID: "v1"},
			wantErr: modproviders.ErrNoUpdateAvailable,
		},
		{
			name:    "provider failure",
			mod:     &models.InstalledMod{Source: failing.ID(), SourceID: "mod", InstalledVersionID: "v1"},
			wantErr: errProviderDown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			latest, errLatest := New(nil).LatestVersion(t.Context(), tc.mod, tc.scope)
			if tc.wantErr != nil {
				if !errors.Is(errLatest, tc.wantErr) {
					t.Fatalf("LatestVersion() error = %v, want %v", errLatest, tc.wantErr)
				}
				return
			}
			if errLatest != nil {
				t.Fatalf("LatestVersion() error = %v", errLatest)
			}
			if latest.VersionID != tc.wantID {
				t.Fatalf("LatestVersion() = %q, want %q", latest.VersionID, tc.wantID)
			}
		})
	}
}

func TestLatestVersionUsesScopeAndCachesProviderAnswers(t *testing.T) {
	// A provider that lists an older, featured version first.
	provider := registerVersionsProvider(t, []modproviders.ModVersion{
		{VersionID: "v1", VersionString: "1.0.0", PublishedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{VersionID: "v2", VersionString: "2.0.0", Changelog: "large changelog", PublishedAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
	}, nil)
	failing := registerVersionsProvider(t, nil, errors.New("provider down"))
	mgr := New(nil)
	scope := UpdateScope{
		GameVersion: "1.21.4",
		Params: map[string]modproviders.SearchParams{
			provider.ID(): {"platform": "PAPER"},
			"other":       {"platform": "VELOCITY"},
		},
	}
	mod := &models.InstalledMod{Source: provider.ID(), SourceID: "mod", InstalledVersionID: "v1"}

	for range 2 {
		latest, errLatest := mgr.LatestVersion(t.Context(), mod, scope)
		if errLatest != nil {
			t.Fatalf("LatestVersion() error = %v", errLatest)
		}
		if latest.VersionString != "2.0.0" || latest.Changelog != "" {
			t.Fatalf("LatestVersion() = %+v, want 2.0.0 without changelog", latest)
		}
	}
	wantRequests := []versionRequest{{gameVersion: "1.21.4", params: modproviders.SearchParams{"platform": "PAPER"}}}
	if got := provider.recordedVersionRequests(); !reflect.DeepEqual(got, wantRequests) {
		t.Fatalf("GetVersions requests = %+v, want %+v", got, wantRequests)
	}

	// Another game version is a different question for the provider.
	scope.GameVersion = "1.21.5"
	_, errOther := mgr.LatestVersion(t.Context(), mod, scope)
	if errOther != nil {
		t.Fatalf("LatestVersion() error = %v", errOther)
	}
	if got := len(provider.recordedVersionRequests()); got != 2 {
		t.Fatalf("GetVersions requests after game version change = %d, want 2", got)
	}

	// Failures are retried rather than remembered.
	failingMod := &models.InstalledMod{Source: failing.ID(), SourceID: "mod", InstalledVersionID: "v1"}
	for range 2 {
		_, errFailing := mgr.LatestVersion(t.Context(), failingMod, UpdateScope{})
		if errFailing == nil {
			t.Fatal("LatestVersion() error = nil, want provider failure")
		}
	}
	if got := len(failing.recordedVersionRequests()); got != 2 {
		t.Fatalf("GetVersions requests for failing provider = %d, want 2", got)
	}
}

func TestCheckUpdates(t *testing.T) {
	steamWorkshopMock()
	provider := registerVersionsProvider(t, []modproviders.ModVersion{
		{VersionID: "v2", VersionString: "2.0.0"},
		{VersionID: "v1", VersionString: "1.0.0"},
	}, nil)
	failing := registerVersionsProvider(t, nil, errors.New("provider down"))
	mods := []*models.InstalledMod{
		{ID: "outdated", Source: provider.ID(), SourceID: "a", InstalledVersionID: "v1", Enabled: 1},
		{ID: "current", Source: provider.ID(), SourceID: "b", InstalledVersionID: "v2", Enabled: 1},
		{ID: "pinned", Source: provider.ID(), SourceID: "c", InstalledVersionID: "v1", Enabled: 1, PinnedVersion: null.From("1.0.0")},
		{ID: "disabled", Source: provider.ID(), SourceID: "d", InstalledVersionID: "v1", Enabled: 0},
		{ID: "workshop", Source: "steam_workshop", SourceID: "123", InstalledVersionID: "1", Enabled: 1},
		{ID: "failing", Source: failing.ID(), SourceID: "e", InstalledVersionID: "v1", Enabled: 1},
	}

	updates, errCheck := New(nil).CheckUpdates(t.Context(), mods, UpdateScope{})
	if errCheck != nil {
		t.Fatalf("CheckUpdates() error = %v", errCheck)
	}
	gotIDs := slices.Sorted(maps.Keys(updates))
	if wantIDs := []string{"outdated", "pinned"}; !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("CheckUpdates() mods = %v, want %v", gotIDs, wantIDs)
	}
	if updates["outdated"].VersionString != "2.0.0" {
		t.Fatalf("CheckUpdates() latest = %q, want 2.0.0", updates["outdated"].VersionString)
	}
}

func TestUpdateMovesPin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name   string
		pinned string
		want   string
	}{
		{name: "pinned mod stays pinned on the installed version", pinned: "1.0.0", want: "2.0.0"},
		{name: "unpinned mod stays unpinned"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conn := dbtest.NewMigratedConnection(t, "mm-update-pin.sqlite")
			seedTestFixture(t, conn)
			mock := newMockProvider(newUniqueProviderID())
			modproviders.RegisterProvider(mock)
			mgr := New(conn)
			serverDir := t.TempDir()
			ctx := context.Background()

			mod, errInstall := mgr.Install(ctx, "server-1", mock.ID(), "mod-src-1", "v1", serverDir, "mods")
			if errInstall != nil {
				t.Fatalf("Install() error = %v", errInstall)
			}
			if tc.pinned != "" {
				_, errPin := conn.SQLDb.ExecContext(ctx, `UPDATE installed_mod SET pinned_version = ? WHERE id = ?`, tc.pinned, mod.ID)
				if errPin != nil {
					t.Fatalf("pin mod: %v", errPin)
				}
			}
			mock.downloadedFiles = []modproviders.DownloadedFile{{Path: "testmod-2.0.0.jar", Hash: "def456", Size: 2048, IsPrimary: true}}

			updated, errUpdate := mgr.Update(ctx, mod.ID, "v2", serverDir)
			if errUpdate != nil {
				t.Fatalf("Update() error = %v", errUpdate)
			}
			if updated.PinnedVersion.GetOr("") != tc.want {
				t.Fatalf("Update() pinned version = %q, want %q", updated.PinnedVersion.GetOr(""), tc.want)
			}
		})
	}
}
