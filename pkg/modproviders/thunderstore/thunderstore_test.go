package thunderstore

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
)

// newTestProvider creates a Provider that routes all requests to the given httptest.Server.
func newTestProvider(srv *httptest.Server) *Provider {
	p := New()
	p.baseURL = srv.URL
	return p
}

// fixtureHandler returns an http.HandlerFunc that serves files from the testdata directory
// based on a simple URL-path-to-filename mapping passed via the routes map.
// routes: map[urlPath] -> filename relative to testdata/
func fixtureHandler(t *testing.T, routes map[string]string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		fixture, ok := routes[r.URL.Path]
		if !ok {
			t.Logf("fixtureHandler: unmatched path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		data, errRead := os.ReadFile(filepath.Join("testdata", fixture))
		if errRead != nil {
			t.Errorf("fixtureHandler: read %s: %v", fixture, errRead)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}
}

// --------------------------------------------------------------------------
// Search
// --------------------------------------------------------------------------

func TestSearch_ReturnsFilteredResults(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/c/valheim/api/v1/package/": "packages_valheim.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	searchResult, errSearch := p.Search(context.Background(), "BepInEx", map[string]any{"community": "valheim"})
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	// Only BepInExPack should match "BepInEx" in name/description.
	if len(searchResult.Results) != 1 {
		t.Fatalf("Search() len = %d, want 1 (only BepInExPack)", len(searchResult.Results))
	}
	if searchResult.TotalHits != -1 {
		t.Errorf("Search().TotalHits = %d, want -1 for unknown total", searchResult.TotalHits)
	}

	first := searchResult.Results[0]
	if first.SourceID != "denikson-BepInExPack_Valheim" {
		t.Errorf("results[0].SourceID = %q, want %q", first.SourceID, "denikson-BepInExPack_Valheim")
	}
	if first.Name != "BepInExPack" {
		t.Errorf("results[0].Name = %q, want %q", first.Name, "BepInExPack")
	}
	if first.Source != providerID {
		t.Errorf("results[0].Source = %q, want %q", first.Source, providerID)
	}
	if first.Author != "denikson" {
		t.Errorf("results[0].Author = %q, want %q", first.Author, "denikson")
	}
	if first.Downloads != 15000000 {
		t.Errorf("results[0].Downloads = %d, want 15000000", first.Downloads)
	}
	if len(first.CompatibleVersions) == 0 {
		t.Error("results[0].CompatibleVersions is empty, want non-empty")
	}
	if first.LatestVersion != "5.4.2202" {
		t.Errorf("results[0].LatestVersion = %q, want %q", first.LatestVersion, "5.4.2202")
	}
}

func TestSearch_EmptyQueryReturnsAllNonDeprecated(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/c/valheim/api/v1/package/": "packages_valheim.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	searchResult, errSearch := p.Search(context.Background(), "", map[string]any{"community": "valheim"})
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	// All 3 packages are non-deprecated in the fixture.
	if len(searchResult.Results) != 3 {
		t.Fatalf("Search() len = %d, want 3 (all non-deprecated)", len(searchResult.Results))
	}
	if searchResult.TotalHits != -1 {
		t.Errorf("Search().TotalHits = %d, want -1 for unknown total", searchResult.TotalHits)
	}
}

func TestSearch_DefaultsCommunityToValheim(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	searchResult, errSearch := p.Search(context.Background(), "test", nil)
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	if searchResult.Results == nil {
		t.Error("Search() returned nil Results, want empty (non-nil) slice")
	}
	if capturedPath != "/c/valheim/api/v1/package/" {
		t.Errorf("capturedPath = %q, want /c/valheim/api/v1/package/", capturedPath)
	}
}

func TestSearch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	_, errSearch := p.Search(context.Background(), "bepinex", map[string]any{"community": "valheim"})
	if errSearch == nil {
		t.Fatal("Search() error = nil, want non-nil on HTTP 429")
	}
}

func TestSearch_SkipsDeprecatedPackages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"name": "DeprecatedMod",
				"full_name": "Author-DeprecatedMod",
				"owner": "Author",
				"package_url": "https://thunderstore.io/c/valheim/p/Author/DeprecatedMod/",
				"date_created": "2021-01-01T00:00:00.000000Z",
				"date_updated": "2021-01-01T00:00:00.000000Z",
				"rating_score": 10,
				"is_pinned": false,
				"is_deprecated": true,
				"total_downloads": 100,
				"latest": {
					"name": "DeprecatedMod",
					"full_name": "Author-DeprecatedMod-1.0.0",
					"description": "A deprecated mod",
					"icon": "",
					"version_number": "1.0.0",
					"dependencies": [],
					"download_url": "https://thunderstore.io/package/download/Author/DeprecatedMod/1.0.0/",
					"downloads": 100,
					"date_created": "2021-01-01T00:00:00.000000Z",
					"file_size": 1024
				},
				"community_listings": [],
				"versions": []
			}
		]`))
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	searchResult, errSearch := p.Search(context.Background(), "", map[string]any{"community": "valheim"})
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	if len(searchResult.Results) != 0 {
		t.Errorf("Search() len = %d, want 0 (deprecated package should be skipped)", len(searchResult.Results))
	}
}

// --------------------------------------------------------------------------
// GetModDetails
// --------------------------------------------------------------------------

func TestGetModDetails_ReturnsFull(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/c/valheim/api/v1/package/": "packages_valheim.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	details, errDetails := p.GetModDetails(context.Background(), "ValheimPlus-ValheimPlus", map[string]any{"community": "valheim"})
	if errDetails != nil {
		t.Fatalf("GetModDetails() error = %v", errDetails)
	}

	if details.SourceID != "ValheimPlus-ValheimPlus" {
		t.Errorf("details.SourceID = %q, want %q", details.SourceID, "ValheimPlus-ValheimPlus")
	}
	if details.Name != "ValheimPlus" {
		t.Errorf("details.Name = %q, want %q", details.Name, "ValheimPlus")
	}
	if details.Source != providerID {
		t.Errorf("details.Source = %q, want %q", details.Source, providerID)
	}
	if details.Author != "ValheimPlus" {
		t.Errorf("details.Author = %q, want %q", details.Author, "ValheimPlus")
	}
	if details.Downloads != 8500000 {
		t.Errorf("details.Downloads = %d, want 8500000", details.Downloads)
	}
	if !strings.Contains(details.Description, "Valheim") {
		t.Errorf("details.Description = %q, want to contain 'Valheim'", details.Description)
	}
	if details.SourceURL == "" {
		t.Error("details.SourceURL is empty, want non-empty")
	}
	if len(details.Versions) != 2 {
		t.Errorf("details.Versions len = %d, want 2", len(details.Versions))
	}
}

func TestGetModDetails_NotFound(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/c/valheim/api/v1/package/": "packages_valheim.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	_, errDetails := p.GetModDetails(context.Background(), "Nobody-NonExistentMod", map[string]any{"community": "valheim"})
	if errDetails == nil {
		t.Fatal("GetModDetails() error = nil, want non-nil for unknown package")
	}
}

func TestGetModDetails_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	_, errDetails := p.GetModDetails(context.Background(), "ValheimPlus-ValheimPlus", map[string]any{"community": "valheim"})
	if errDetails == nil {
		t.Fatal("GetModDetails() error = nil, want non-nil for 404")
	}
}

// --------------------------------------------------------------------------
// GetVersions
// --------------------------------------------------------------------------

func TestGetVersions_ReturnsMapped(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/c/valheim/api/v1/package/": "packages_valheim.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	versions, errVersions := p.GetVersions(context.Background(), "ValheimPlus-ValheimPlus", "", map[string]any{"community": "valheim"})
	if errVersions != nil {
		t.Fatalf("GetVersions() error = %v", errVersions)
	}
	if len(versions) != 2 {
		t.Fatalf("GetVersions() len = %d, want 2", len(versions))
	}

	v := versions[0]
	if v.VersionID != "0.9.12.0" {
		t.Errorf("versions[0].VersionID = %q, want %q", v.VersionID, "0.9.12.0")
	}
	if v.VersionString != "0.9.12.0" {
		t.Errorf("versions[0].VersionString = %q, want %q", v.VersionString, "0.9.12.0")
	}
	if v.DownloadURL == "" {
		t.Error("versions[0].DownloadURL is empty, want non-empty")
	}
	if v.FileSize != 1048576 {
		t.Errorf("versions[0].FileSize = %d, want 1048576", v.FileSize)
	}
}

func TestGetVersions_DependenciesMapped(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/c/valheim/api/v1/package/": "packages_valheim.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	// SomeDifferentMod has a dependency on BepInExPack.
	versions, errVersions := p.GetVersions(context.Background(), "SomeAuthor-SomeDifferentMod", "", map[string]any{"community": "valheim"})
	if errVersions != nil {
		t.Fatalf("GetVersions() error = %v", errVersions)
	}
	if len(versions) != 1 {
		t.Fatalf("GetVersions() len = %d, want 1", len(versions))
	}
	v := versions[0]
	if len(v.Dependencies) != 1 {
		t.Errorf("versions[0].Dependencies len = %d, want 1", len(v.Dependencies))
	}
	if v.Dependencies[0].SourceID != "denikson-BepInExPack_Valheim-5.4.2202" {
		t.Errorf("dependencies[0].SourceID = %q, want %q", v.Dependencies[0].SourceID, "denikson-BepInExPack_Valheim-5.4.2202")
	}
	if !v.Dependencies[0].Required {
		t.Error("dependencies[0].Required = false, want true")
	}
}

func TestGetVersions_NotFound(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/c/valheim/api/v1/package/": "packages_valheim.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	_, errVersions := p.GetVersions(context.Background(), "Nobody-NoSuchMod", "", map[string]any{"community": "valheim"})
	if errVersions == nil {
		t.Fatal("GetVersions() error = nil, want non-nil for unknown package")
	}
}

// --------------------------------------------------------------------------
// CheckForUpdate
// --------------------------------------------------------------------------

func TestCheckForUpdate_ReturnsLatest(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/c/valheim/api/v1/package/": "packages_valheim.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	v, errCheck := p.CheckForUpdate(context.Background(), "ValheimPlus-ValheimPlus", "")
	if errCheck != nil {
		t.Fatalf("CheckForUpdate() error = %v", errCheck)
	}
	if v == nil {
		t.Fatal("CheckForUpdate() = nil, want non-nil")
	}
	if v.VersionID != "0.9.12.0" {
		t.Errorf("CheckForUpdate().VersionID = %q, want %q", v.VersionID, "0.9.12.0")
	}
}

func TestCheckForUpdate_NoVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"name": "EmptyMod",
				"full_name": "Author-EmptyMod",
				"owner": "Author",
				"package_url": "https://thunderstore.io/c/valheim/p/Author/EmptyMod/",
				"date_created": "2021-01-01T00:00:00.000000Z",
				"date_updated": "2021-01-01T00:00:00.000000Z",
				"rating_score": 0,
				"is_pinned": false,
				"is_deprecated": false,
				"total_downloads": 0,
				"latest": {
					"name": "EmptyMod",
					"full_name": "Author-EmptyMod-0.0.1",
					"description": "",
					"icon": "",
					"version_number": "0.0.1",
					"dependencies": [],
					"download_url": "",
					"downloads": 0,
					"date_created": "2021-01-01T00:00:00.000000Z",
					"file_size": 0
				},
				"community_listings": [],
				"versions": []
			}
		]`))
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	v, errCheck := p.CheckForUpdate(context.Background(), "Author-EmptyMod", "")
	if !errors.Is(errCheck, modproviders.ErrNoUpdateAvailable) {
		t.Fatalf("CheckForUpdate() error = %v, want %v", errCheck, modproviders.ErrNoUpdateAvailable)
	}
	if v != nil {
		t.Errorf("CheckForUpdate() = %+v, want nil when no versions", v)
	}
}

// --------------------------------------------------------------------------
// Cache
// --------------------------------------------------------------------------

func TestCache_HitsOnSecondCall(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		data, errRead := os.ReadFile(filepath.Join("testdata", "packages_valheim.json"))
		if errRead != nil {
			t.Errorf("read fixture: %v", errRead)
			http.Error(w, "error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	params := map[string]any{"community": "valheim"}

	_, errFirst := p.Search(context.Background(), "", params)
	if errFirst != nil {
		t.Fatalf("first Search() error = %v", errFirst)
	}

	_, errSecond := p.Search(context.Background(), "", params)
	if errSecond != nil {
		t.Fatalf("second Search() error = %v", errSecond)
	}

	if callCount != 1 {
		t.Errorf("HTTP call count = %d, want 1 (second call should use cache)", callCount)
	}
}

func TestCache_RefetchesAfterTTL(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	params := map[string]any{"community": "valheim"}

	_, errFirst := p.Search(context.Background(), "", params)
	if errFirst != nil {
		t.Fatalf("first Search() error = %v", errFirst)
	}

	// Manually expire the cache entry by backdating its fetchedAt time.
	p.mu.Lock()
	entry := p.cache["valheim"]
	entry.fetchedAt = time.Now().Add(-(cacheTTL + time.Second))
	p.cache["valheim"] = entry
	p.mu.Unlock()

	_, errSecond := p.Search(context.Background(), "", params)
	if errSecond != nil {
		t.Fatalf("second Search() error = %v", errSecond)
	}

	if callCount != 2 {
		t.Errorf("HTTP call count = %d, want 2 (stale cache should trigger re-fetch)", callCount)
	}
}

func TestDownload_UsesResolvedCommunityForSource(t *testing.T) {
	var packagePath string
	var serverURL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/c/lethal-company/api/v1/package/":
			packagePath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `[
				{
					"name": "SharedMod",
					"full_name": "Author-SharedMod",
					"owner": "Author",
					"package_url": "%s/c/lethal-company/p/Author/SharedMod/",
					"date_created": "2021-01-01T00:00:00.000000Z",
					"date_updated": "2021-01-01T00:00:00.000000Z",
					"rating_score": 10,
					"is_pinned": false,
					"is_deprecated": false,
					"total_downloads": 100,
					"latest": {
						"name": "SharedMod",
						"full_name": "Author-SharedMod-1.0.0",
						"description": "A shared mod",
						"icon": "",
						"version_number": "1.0.0",
						"dependencies": [],
						"download_url": "%s/download/sharedmod.zip",
						"downloads": 100,
						"date_created": "2021-01-01T00:00:00.000000Z",
						"file_size": 7
					},
					"versions": [
						{
						"name": "SharedMod",
						"full_name": "Author-SharedMod-1.0.0",
						"description": "A shared mod",
						"icon": "",
						"version_number": "1.0.0",
						"dependencies": [],
						"download_url": "%s/download/sharedmod.zip",
							"downloads": 100,
							"date_created": "2021-01-01T00:00:00.000000Z",
							"file_size": 7
						}
					]
				}
			]`, serverURL, serverURL, serverURL)
		case "/download/sharedmod.zip":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("payload"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	serverURL = srv.URL

	p := newTestProvider(srv)
	_, errDetails := p.GetModDetails(context.Background(), "Author-SharedMod", map[string]any{"community": "lethal-company"})
	if errDetails != nil {
		t.Fatalf("GetModDetails() error = %v", errDetails)
	}

	_, errDownload := p.Download(context.Background(), "Author-SharedMod", "1.0.0", t.TempDir())
	if errDownload != nil {
		t.Fatalf("Download() error = %v", errDownload)
	}

	if packagePath != "/c/lethal-company/api/v1/package/" {
		t.Fatalf("Download() package path = %q, want %q", packagePath, "/c/lethal-company/api/v1/package/")
	}
}

// --------------------------------------------------------------------------
// extractCommunity
// --------------------------------------------------------------------------

func TestExtractCommunity(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{name: "nil params", params: nil, want: ""},
		{name: "no community key", params: map[string]any{"other": "val"}, want: ""},
		{name: "community valheim", params: map[string]any{"community": "valheim"}, want: "valheim"},
		{name: "non-string community", params: map[string]any{"community": 42}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCommunity(tt.params)
			if got != tt.want {
				t.Errorf("extractCommunity() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Provider identity
// --------------------------------------------------------------------------

func TestID(t *testing.T) {
	p := New()
	if p.ID() != providerID {
		t.Errorf("ID() = %q, want %q", p.ID(), providerID)
	}
}

func TestRequiresAPIKey(t *testing.T) {
	p := New()
	if p.RequiresAPIKey() {
		t.Error("RequiresAPIKey() = true, want false")
	}
}
