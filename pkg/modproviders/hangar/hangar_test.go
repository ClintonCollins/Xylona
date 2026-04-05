package hangar

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestSearch_ReturnsResults(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/projects": "search.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	searchResult, errSearch := p.Search(context.Background(), "worldedit", nil)
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	if len(searchResult.Results) != 2 {
		t.Fatalf("Search() len = %d, want 2", len(searchResult.Results))
	}

	first := searchResult.Results[0]
	if first.SourceID != "EngineHub/WorldEdit" {
		t.Errorf("results[0].SourceID = %q, want %q", first.SourceID, "EngineHub/WorldEdit")
	}
	if first.Name != "WorldEdit" {
		t.Errorf("results[0].Name = %q, want %q", first.Name, "WorldEdit")
	}
	if first.Source != providerID {
		t.Errorf("results[0].Source = %q, want %q", first.Source, providerID)
	}
	if first.Author != "EngineHub" {
		t.Errorf("results[0].Author = %q, want %q", first.Author, "EngineHub")
	}
	if first.Downloads != 5000000 {
		t.Errorf("results[0].Downloads = %d, want 5000000", first.Downloads)
	}
}

func TestSearch_WithPlatformParam(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[],"pagination":{"count":0}}`))
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	params := modprovidersSearchParams{"platform": "PAPER"}
	_, errSearch := p.Search(context.Background(), "test", params)
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	if capturedQuery == "" {
		t.Fatal("no query captured")
	}
	// platform=PAPER should appear in the query string.
	if !containsString(capturedQuery, "platform=PAPER") {
		t.Errorf("query %q does not contain platform=PAPER", capturedQuery)
	}
}

func TestSearch_EmptyQueryReturnsEmptySlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[],"pagination":{"count":0}}`))
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	searchResult, errSearch := p.Search(context.Background(), "", nil)
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	if searchResult.Results == nil {
		t.Error("Search() returned nil Results, want empty (non-nil) slice")
	}
	if len(searchResult.Results) != 0 {
		t.Errorf("Search() len = %d, want 0", len(searchResult.Results))
	}
	if searchResult.TotalHits != 0 {
		t.Errorf("Search().TotalHits = %d, want 0", searchResult.TotalHits)
	}
}

func TestSearch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	_, errSearch := p.Search(context.Background(), "worldedit", nil)
	if errSearch == nil {
		t.Fatal("Search() error = nil, want non-nil on HTTP 429")
	}
}

// --------------------------------------------------------------------------
// GetModDetails
// --------------------------------------------------------------------------

func TestGetModDetails_ReturnsFull(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/projects/EngineHub/WorldEdit":          "project_worldedit.json",
		"/projects/EngineHub/WorldEdit/versions": "versions_worldedit.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	details, errDetails := p.GetModDetails(context.Background(), "EngineHub/WorldEdit", nil)
	if errDetails != nil {
		t.Fatalf("GetModDetails() error = %v", errDetails)
	}

	if details.SourceID != "EngineHub/WorldEdit" {
		t.Errorf("details.SourceID = %q, want %q", details.SourceID, "EngineHub/WorldEdit")
	}
	if details.Name != "WorldEdit" {
		t.Errorf("details.Name = %q, want %q", details.Name, "WorldEdit")
	}
	if details.Source != providerID {
		t.Errorf("details.Source = %q, want %q", details.Source, providerID)
	}
	if details.Author != "EngineHub" {
		t.Errorf("details.Author = %q, want %q", details.Author, "EngineHub")
	}
	if details.License != "LGPL-3.0" {
		t.Errorf("details.License = %q, want %q", details.License, "LGPL-3.0")
	}
	if details.SourceURL != "https://github.com/enginehub/WorldEdit" {
		t.Errorf("details.SourceURL = %q, want %q", details.SourceURL, "https://github.com/enginehub/WorldEdit")
	}
	if len(details.Categories) != 2 {
		t.Errorf("details.Categories len = %d, want 2", len(details.Categories))
	}
	if len(details.Versions) != 2 {
		t.Errorf("details.Versions len = %d, want 2", len(details.Versions))
	}
}

func TestGetModDetails_InvalidSourceID(t *testing.T) {
	p := New()
	_, errDetails := p.GetModDetails(context.Background(), "invalidsourceid", nil)
	if errDetails == nil {
		t.Fatal("GetModDetails() error = nil, want non-nil for invalid sourceID")
	}
}

func TestGetModDetails_ProjectHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	_, errDetails := p.GetModDetails(context.Background(), "SomeAuthor/nonexistent", nil)
	if errDetails == nil {
		t.Fatal("GetModDetails() error = nil, want non-nil for 404")
	}
}

// --------------------------------------------------------------------------
// GetVersions
// --------------------------------------------------------------------------

func TestGetVersions_ReturnsMapped(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/projects/EngineHub/WorldEdit/versions": "versions_worldedit.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	versions, errVersions := p.GetVersions(context.Background(), "EngineHub/WorldEdit", "", nil)
	if errVersions != nil {
		t.Fatalf("GetVersions() error = %v", errVersions)
	}
	if len(versions) != 2 {
		t.Fatalf("GetVersions() len = %d, want 2", len(versions))
	}

	v := versions[0]
	if v.VersionID != "7.2.15" {
		t.Errorf("versions[0].VersionID = %q, want %q", v.VersionID, "7.2.15")
	}
	if v.VersionString != "7.2.15" {
		t.Errorf("versions[0].VersionString = %q, want %q", v.VersionString, "7.2.15")
	}
	if v.DownloadURL == "" {
		t.Error("versions[0].DownloadURL is empty, want non-empty")
	}
	if v.FileSize != 2048576 {
		t.Errorf("versions[0].FileSize = %d, want 2048576", v.FileSize)
	}
	if len(v.Dependencies) != 1 {
		t.Errorf("versions[0].Dependencies len = %d, want 1", len(v.Dependencies))
	}
	if !v.Dependencies[0].Required {
		t.Error("versions[0].Dependencies[0].Required = false, want true")
	}
	if v.Changelog != "Bug fixes and performance improvements." {
		t.Errorf("versions[0].Changelog = %q, want %q", v.Changelog, "Bug fixes and performance improvements.")
	}
}

func TestGetVersions_FilteredByGameVersion(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/projects/EngineHub/WorldEdit/versions": "versions_worldedit.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	// 1.19.4 only exists in the second version fixture entry.
	versions, errVersions := p.GetVersions(context.Background(), "EngineHub/WorldEdit", "1.19.4", nil)
	if errVersions != nil {
		t.Fatalf("GetVersions() error = %v", errVersions)
	}
	if len(versions) != 1 {
		t.Fatalf("GetVersions() len = %d, want 1 (filtered to 1.19.4)", len(versions))
	}
	if versions[0].VersionID != "7.2.14" {
		t.Errorf("versions[0].VersionID = %q, want %q", versions[0].VersionID, "7.2.14")
	}
}

func TestGetVersions_InvalidSourceID(t *testing.T) {
	p := New()
	_, errVersions := p.GetVersions(context.Background(), "bad-id", "", nil)
	if errVersions == nil {
		t.Fatal("GetVersions() error = nil, want non-nil for invalid sourceID")
	}
}

func TestGetVersions_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[]}`))
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	versions, errVersions := p.GetVersions(context.Background(), "SomeAuthor/no-versions-plugin", "", nil)
	if errVersions != nil {
		t.Fatalf("GetVersions() error = %v", errVersions)
	}
	if versions == nil {
		t.Error("GetVersions() returned nil, want empty (non-nil) slice")
	}
	if len(versions) != 0 {
		t.Errorf("GetVersions() len = %d, want 0", len(versions))
	}
}

// --------------------------------------------------------------------------
// CheckForUpdate
// --------------------------------------------------------------------------

func TestCheckForUpdate_ReturnsLatest(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/projects/EngineHub/WorldEdit/versions": "versions_worldedit.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	v, errCheck := p.CheckForUpdate(context.Background(), "EngineHub/WorldEdit", "1.20.1")
	if errCheck != nil {
		t.Fatalf("CheckForUpdate() error = %v", errCheck)
	}
	if v == nil {
		t.Fatal("CheckForUpdate() = nil, want non-nil")
	}
	if v.VersionID != "7.2.15" {
		t.Errorf("CheckForUpdate().VersionID = %q, want %q", v.VersionID, "7.2.15")
	}
}

func TestCheckForUpdate_NoVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[]}`))
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	v, errCheck := p.CheckForUpdate(context.Background(), "SomeAuthor/obscure-plugin", "1.20.1")
	if !errors.Is(errCheck, modproviders.ErrNoUpdateAvailable) {
		t.Fatalf("CheckForUpdate() error = %v, want %v", errCheck, modproviders.ErrNoUpdateAvailable)
	}
	if v != nil {
		t.Errorf("CheckForUpdate() = %+v, want nil when no versions", v)
	}
}

// --------------------------------------------------------------------------
// splitSourceID
// --------------------------------------------------------------------------

func TestSplitSourceID(t *testing.T) {
	tests := []struct {
		name       string
		sourceID   string
		wantAuthor string
		wantSlug   string
		wantErr    bool
	}{
		{
			name:       "valid author/slug",
			sourceID:   "EngineHub/WorldEdit",
			wantAuthor: "EngineHub",
			wantSlug:   "WorldEdit",
			wantErr:    false,
		},
		{
			name:       "valid with path-like slug containing extra slash",
			sourceID:   "SomeAuthor/some/slug",
			wantAuthor: "SomeAuthor",
			wantSlug:   "some/slug",
			wantErr:    false,
		},
		{
			name:     "missing slash",
			sourceID: "NoSlashHere",
			wantErr:  true,
		},
		{
			name:     "empty string",
			sourceID: "",
			wantErr:  true,
		},
		{
			name:     "slash only — empty author",
			sourceID: "/slug",
			wantErr:  true,
		},
		{
			name:     "slash only — empty slug",
			sourceID: "author/",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			author, slug, errSplit := splitSourceID(tt.sourceID)
			if (errSplit != nil) != tt.wantErr {
				t.Fatalf("splitSourceID(%q) error = %v, wantErr %v", tt.sourceID, errSplit, tt.wantErr)
			}
			if !tt.wantErr {
				if author != tt.wantAuthor {
					t.Errorf("author = %q, want %q", author, tt.wantAuthor)
				}
				if slug != tt.wantSlug {
					t.Errorf("slug = %q, want %q", slug, tt.wantSlug)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// extractPlatform
// --------------------------------------------------------------------------

func TestExtractPlatform(t *testing.T) {
	tests := []struct {
		name   string
		params modprovidersSearchParams
		want   string
	}{
		{name: "nil params", params: nil, want: ""},
		{name: "no platform key", params: modprovidersSearchParams{"other": "val"}, want: ""},
		{name: "platform PAPER", params: modprovidersSearchParams{"platform": "PAPER"}, want: "PAPER"},
		{name: "non-string platform", params: modprovidersSearchParams{"platform": 42}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPlatform(tt.params)
			if got != tt.want {
				t.Errorf("extractPlatform() = %q, want %q", got, tt.want)
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

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// modprovidersSearchParams is a local alias to avoid import cycles in tests.
type modprovidersSearchParams = map[string]any

func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}
