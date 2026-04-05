package steamcache

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFetchReleases_ParsesRecordedSevenDaysToDieFixture(t *testing.T) {
	fixture := readSteamcacheFixture(t, "steamcmd-294420.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	client := &Client{
		detailsURLFmt:   server.URL + "/%s",
		httpClient:      server.Client(),
		releaseFreshTTL: time.Minute,
		releaseStaleTTL: time.Hour,
		localReleaseFunc: func(context.Context, string) ([]SteamRelease, error) {
			return nil, errors.New("unexpected local fallback")
		},
	}

	releases, errFetch := client.FetchReleases(context.Background(), "294420")
	if errFetch != nil {
		t.Fatalf("FetchReleases() error = %v", errFetch)
	}
	if len(releases) < 5 {
		t.Fatalf("len(releases) = %d, want at least 5 real-world branches", len(releases))
	}

	if releases[0].Name != "public" {
		t.Fatalf("releases[0].Name = %q, want %q", releases[0].Name, "public")
	}
	if releases[0].DisplayLabel != "Public" {
		t.Fatalf("releases[0].DisplayLabel = %q, want %q", releases[0].DisplayLabel, "Public")
	}
	if releases[0].BuildID != "21600865" {
		t.Fatalf("releases[0].BuildID = %q, want %q", releases[0].BuildID, "21600865")
	}

	experimental := findReleaseByName(t, releases, "latest_experimental")
	if experimental.BuildID != "22422094" {
		t.Errorf("latest_experimental BuildID = %q, want %q", experimental.BuildID, "22422094")
	}
	if experimental.DisplayLabel != "Unstable build" {
		t.Errorf("latest_experimental DisplayLabel = %q, want %q", experimental.DisplayLabel, "Unstable build")
	}

	v25 := findReleaseByName(t, releases, "v2.5")
	if v25.DisplayLabel != "Version 2.5 Stable" {
		t.Errorf("v2.5 DisplayLabel = %q, want %q", v25.DisplayLabel, "Version 2.5 Stable")
	}
	if len(v25.DepotManifestIDs) != 2 {
		t.Errorf("len(v2.5.DepotManifestIDs) = %d, want 2", len(v25.DepotManifestIDs))
	}
}

func TestFetchReleases_UsesStaleCacheOnAPIFailure(t *testing.T) {
	fixture := readSteamcacheFixture(t, "steamcmd-294420.json")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			_, _ = w.Write(fixture)
			return
		}
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	client := &Client{
		detailsURLFmt:   server.URL + "/%s",
		httpClient:      server.Client(),
		releaseFreshTTL: 0,
		releaseStaleTTL: time.Hour,
		localReleaseFunc: func(context.Context, string) ([]SteamRelease, error) {
			return nil, errors.New("unexpected local fallback")
		},
	}

	initial, errInitial := client.FetchReleases(context.Background(), "294420")
	if errInitial != nil {
		t.Fatalf("initial FetchReleases() error = %v", errInitial)
	}

	cached, errCached := client.FetchReleases(context.Background(), "294420")
	if errCached != nil {
		t.Fatalf("cached FetchReleases() error = %v", errCached)
	}
	if len(initial) != len(cached) {
		t.Fatalf("len(cached) = %d, want %d", len(cached), len(initial))
	}
	if findReleaseByName(t, cached, "public").BuildID != "21600865" {
		t.Fatalf("cached public BuildID = %q, want %q", findReleaseByName(t, cached, "public").BuildID, "21600865")
	}
}

func TestFetchReleases_UsesLocalFallbackWhenAPIUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer server.Close()

	client := &Client{
		detailsURLFmt:   server.URL + "/%s",
		httpClient:      server.Client(),
		releaseFreshTTL: time.Minute,
		releaseStaleTTL: time.Hour,
		localReleaseFunc: func(_ context.Context, appID string) ([]SteamRelease, error) {
			if appID != "294420" {
				t.Fatalf("local fallback appID = %q, want %q", appID, "294420")
			}
			return []SteamRelease{
				{Name: "public", DisplayLabel: "Public", BuildID: "12345"},
			}, nil
		},
	}

	releases, errFetch := client.FetchReleases(context.Background(), "294420")
	if errFetch != nil {
		t.Fatalf("FetchReleases() error = %v", errFetch)
	}
	if len(releases) != 1 {
		t.Fatalf("len(releases) = %d, want 1", len(releases))
	}
	if releases[0].BuildID != "12345" {
		t.Fatalf("releases[0].BuildID = %q, want %q", releases[0].BuildID, "12345")
	}
}

func TestParseLocalSteamCMDReleases_ParsesBranchesAndManifestIDs(t *testing.T) {
	output := []byte(`
"294420"
{
	"depots"
	{
		"294421"
		{
			"manifests"
			{
				"public"
				{
					"gid"		"111"
				}
				"latest_experimental"
				{
					"gid"		"222"
				}
			}
		}
		"294422"
		{
			"manifests"
			{
				"public"
				{
					"gid"		"333"
				}
			}
		}
		"branches"
		{
			"public"
			{
				"buildid"		"21600865"
				"timeupdated"	"1769450968"
			}
			"latest_experimental"
			{
				"buildid"			"22422094"
				"description"		"Unstable build"
				"timebuildupdated"	"1773951890"
			}
		}
	}
}
`)

	releases, errParse := parseLocalSteamCMDReleases(output, "294420")
	if errParse != nil {
		t.Fatalf("parseLocalSteamCMDReleases() error = %v", errParse)
	}
	if len(releases) != 2 {
		t.Fatalf("len(releases) = %d, want 2", len(releases))
	}

	publicRelease := findReleaseByName(t, releases, "public")
	if publicRelease.BuildID != "21600865" {
		t.Fatalf("public BuildID = %q, want %q", publicRelease.BuildID, "21600865")
	}
	if publicRelease.DisplayLabel != "Public" {
		t.Fatalf("public DisplayLabel = %q, want %q", publicRelease.DisplayLabel, "Public")
	}
	if len(publicRelease.DepotManifestIDs) != 2 {
		t.Fatalf("len(public.DepotManifestIDs) = %d, want 2", len(publicRelease.DepotManifestIDs))
	}

	experimentalRelease := findReleaseByName(t, releases, "latest_experimental")
	if experimentalRelease.DisplayLabel != "Unstable build" {
		t.Fatalf(
			"latest_experimental DisplayLabel = %q, want %q",
			experimentalRelease.DisplayLabel,
			"Unstable build",
		)
	}
	if experimentalRelease.DepotManifestIDs["294421"] != "222" {
		t.Fatalf(
			"latest_experimental depot 294421 manifest = %q, want %q",
			experimentalRelease.DepotManifestIDs["294421"],
			"222",
		)
	}
}

func readSteamcacheFixture(t *testing.T, fileName string) []byte {
	t.Helper()

	path := filepath.Join("testdata", fileName)
	body, errRead := os.ReadFile(path)
	if errRead != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", path, errRead)
	}
	return body
}

func findReleaseByName(t *testing.T, releases []SteamRelease, name string) SteamRelease {
	t.Helper()

	for _, release := range releases {
		if release.Name == name {
			return release
		}
	}

	t.Fatalf("release %q not found", name)
	return SteamRelease{}
}
