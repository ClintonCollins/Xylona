package versiontracker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/steamcache"
)

func TestSteamTracker_CheckForUpdate_UsesPreferredSteamBranchMetadata(t *testing.T) {
	fixture := mustReadSteamTrackerFixture(t, "steamcmd-294420.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	dir := t.TempDir()
	writeACF(t, dir, "appmanifest_294420.acf", `"AppState"
{
	"appid"		"294420"
	"buildid"		"21600865"
}
`)

	tracker := &SteamTracker{
		preferredAppID: "294420",
		steamCache: steamcache.NewWithOptions(steamcache.ClientOptions{
			HTTPClient:       server.Client(),
			DetailsURLFormat: server.URL + "/%s",
			ReleaseFreshTTL:  time.Minute,
			ReleaseStaleTTL:  time.Hour,
		}),
	}
	gameServer := &models.GameServer{
		Directory: dir,
		Branch:    "latest_experimental",
	}

	info, errCheck := tracker.CheckForUpdate(context.Background(), gameServer)
	if errCheck != nil {
		t.Fatalf("CheckForUpdate() error = %v", errCheck)
	}
	if info == nil {
		t.Fatal("CheckForUpdate() = nil, want update info")
	}
	if !info.UpdateAvailable {
		t.Fatal("UpdateAvailable = false, want true")
	}
	if info.InstalledVersion != "21600865" {
		t.Fatalf("InstalledVersion = %q, want %q", info.InstalledVersion, "21600865")
	}
	if info.LatestVersion != "22422094" {
		t.Fatalf("LatestVersion = %q, want %q", info.LatestVersion, "22422094")
	}
	if info.InstalledBranch != "public" {
		t.Fatalf("InstalledBranch = %q, want %q", info.InstalledBranch, "public")
	}
	if info.LatestBranch != "latest_experimental" {
		t.Fatalf("LatestBranch = %q, want %q", info.LatestBranch, "latest_experimental")
	}
	if info.InstalledVersionLabel != "Public (21600865)" {
		t.Fatalf("InstalledVersionLabel = %q, want %q", info.InstalledVersionLabel, "Public (21600865)")
	}
	if info.LatestVersionLabel != "Unstable build (22422094)" {
		t.Fatalf("LatestVersionLabel = %q, want %q", info.LatestVersionLabel, "Unstable build (22422094)")
	}
}

func TestSteamTracker_CheckForUpdate_FallsBackToRawBuildIDsWhenBranchMetadataUnavailable(t *testing.T) {
	dir := t.TempDir()
	writeACF(t, dir, "appmanifest_294420.acf", `"AppState"
{
	"appid"		"294420"
	"buildid"		"21600865"
}
`)

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"response":{"success":true,"required_version":22000000}}`))
	}))
	defer apiServer.Close()

	tracker := &SteamTracker{
		preferredAppID: "294420",
		steamAPIURL:    apiServer.URL,
		httpClient:     apiServer.Client(),
		steamCache: steamcache.NewWithOptions(steamcache.ClientOptions{
			HTTPClient:       apiServer.Client(),
			DetailsURLFormat: "http://127.0.0.1:1/%s",
			ReleaseFreshTTL:  time.Minute,
		}),
	}
	gameServer := &models.GameServer{Directory: dir, Branch: "latest_experimental"}

	info, errCheck := tracker.CheckForUpdate(context.Background(), gameServer)
	if errCheck != nil {
		t.Fatalf("CheckForUpdate() error = %v", errCheck)
	}
	if info == nil {
		t.Fatal("CheckForUpdate() = nil, want update info")
	}
	if info.InstalledVersionLabel != "21600865" {
		t.Fatalf("InstalledVersionLabel = %q, want %q", info.InstalledVersionLabel, "21600865")
	}
	if info.LatestVersionLabel != "22000000" {
		t.Fatalf("LatestVersionLabel = %q, want %q", info.LatestVersionLabel, "22000000")
	}
	if info.InstalledBranch != "" {
		t.Fatalf("InstalledBranch = %q, want empty string", info.InstalledBranch)
	}
	if info.LatestBranch != "" {
		t.Fatalf("LatestBranch = %q, want empty string", info.LatestBranch)
	}
}

func mustReadSteamTrackerFixture(t *testing.T, fileName string) []byte {
	t.Helper()

	path := filepath.Join("..", "..", "steamcache", "testdata", fileName)
	body, errRead := os.ReadFile(path)
	if errRead != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", path, errRead)
	}
	return body
}
