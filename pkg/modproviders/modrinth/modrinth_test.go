package modrinth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		"/search": "search.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	results, errSearch := p.Search(context.Background(), "worldedit", nil)
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	if len(results) != 2 {
		t.Fatalf("Search() len = %d, want 2", len(results))
	}

	first := results[0]
	if first.SourceID != "worldedit" {
		t.Errorf("results[0].SourceID = %q, want %q", first.SourceID, "worldedit")
	}
	if first.Name != "WorldEdit" {
		t.Errorf("results[0].Name = %q, want %q", first.Name, "WorldEdit")
	}
	if first.Source != providerID {
		t.Errorf("results[0].Source = %q, want %q", first.Source, providerID)
	}
	if first.Author != "sk89q" {
		t.Errorf("results[0].Author = %q, want %q", first.Author, "sk89q")
	}
	if first.Downloads != 5000000 {
		t.Errorf("results[0].Downloads = %d, want 5000000", first.Downloads)
	}
	if len(first.CompatibleVersions) == 0 {
		t.Error("results[0].CompatibleVersions is empty, want non-empty")
	}
	// LatestVersion should be the last entry in versions array.
	if first.LatestVersion != "1.18.2" {
		t.Errorf("results[0].LatestVersion = %q, want %q", first.LatestVersion, "1.18.2")
	}
}

func TestSearch_EmptyQueryReturnsEmptySlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":[],"total_hits":0,"offset":0,"limit":20}`))
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	results, errSearch := p.Search(context.Background(), "", nil)
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	if results == nil {
		t.Error("Search() returned nil, want empty (non-nil) slice")
	}
	if len(results) != 0 {
		t.Errorf("Search() len = %d, want 0", len(results))
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
		"/project/worldedit":         "project_worldedit.json",
		"/project/worldedit/version": "versions_worldedit.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	details, errDetails := p.GetModDetails(context.Background(), "worldedit", nil)
	if errDetails != nil {
		t.Fatalf("GetModDetails() error = %v", errDetails)
	}

	if details.SourceID != "worldedit" {
		t.Errorf("details.SourceID = %q, want %q", details.SourceID, "worldedit")
	}
	if details.Name != "WorldEdit" {
		t.Errorf("details.Name = %q, want %q", details.Name, "WorldEdit")
	}
	if details.Source != providerID {
		t.Errorf("details.Source = %q, want %q", details.Source, providerID)
	}
	if !strings.Contains(details.Body, "WorldEdit") {
		t.Errorf("details.Body = %q, want to contain 'WorldEdit'", details.Body)
	}
	if details.License != "lgpl-3" {
		t.Errorf("details.License = %q, want %q", details.License, "lgpl-3")
	}
	if details.SourceURL != "https://github.com/enginehub/WorldEdit" {
		t.Errorf("details.SourceURL = %q, want %q", details.SourceURL, "https://github.com/enginehub/WorldEdit")
	}
	if len(details.GalleryImages) != 2 {
		t.Errorf("details.GalleryImages len = %d, want 2", len(details.GalleryImages))
	}
	if len(details.Categories) != 2 {
		t.Errorf("details.Categories len = %d, want 2", len(details.Categories))
	}
	if len(details.Versions) != 2 {
		t.Errorf("details.Versions len = %d, want 2", len(details.Versions))
	}
}

func TestGetModDetails_ProjectHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	_, errDetails := p.GetModDetails(context.Background(), "nonexistent", nil)
	if errDetails == nil {
		t.Fatal("GetModDetails() error = nil, want non-nil for 404")
	}
}

// --------------------------------------------------------------------------
// GetVersions
// --------------------------------------------------------------------------

func TestGetVersions_ReturnsMapped(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/project/worldedit/version": "versions_worldedit.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	versions, errVersions := p.GetVersions(context.Background(), "worldedit", "", nil)
	if errVersions != nil {
		t.Fatalf("GetVersions() error = %v", errVersions)
	}
	if len(versions) != 2 {
		t.Fatalf("GetVersions() len = %d, want 2", len(versions))
	}

	v := versions[0]
	if v.VersionID != "version-abc123" {
		t.Errorf("versions[0].VersionID = %q, want %q", v.VersionID, "version-abc123")
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

func TestGetVersions_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	versions, errVersions := p.GetVersions(context.Background(), "no-versions-mod", "", nil)
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
		"/project/worldedit/version": "versions_worldedit.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	v, errCheck := p.CheckForUpdate(context.Background(), "worldedit", "1.20.1")
	if errCheck != nil {
		t.Fatalf("CheckForUpdate() error = %v", errCheck)
	}
	if v == nil {
		t.Fatal("CheckForUpdate() = nil, want non-nil")
	}
	if v.VersionID != "version-abc123" {
		t.Errorf("CheckForUpdate().VersionID = %q, want %q", v.VersionID, "version-abc123")
	}
}

func TestCheckForUpdate_NoVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	v, errCheck := p.CheckForUpdate(context.Background(), "obscure-mod", "1.20.1")
	if errCheck != nil {
		t.Fatalf("CheckForUpdate() error = %v", errCheck)
	}
	if v != nil {
		t.Errorf("CheckForUpdate() = %+v, want nil when no versions", v)
	}
}

// --------------------------------------------------------------------------
// buildFacets
// --------------------------------------------------------------------------

func TestBuildFacets_Nil(t *testing.T) {
	result := buildFacets(nil)
	if result != "" {
		t.Errorf("buildFacets(nil) = %q, want %q", result, "")
	}
}

func TestBuildFacets_NoFacetsKey(t *testing.T) {
	params := map[string]any{"other": "value"}
	result := buildFacets(params)
	if result != "" {
		t.Errorf("buildFacets() = %q, want %q when no facets key", result, "")
	}
}

func TestBuildFacets_StringValue(t *testing.T) {
	params := map[string]any{
		"facets": map[string]any{
			"project_type": "plugin",
		},
	}
	result := buildFacets(params)
	if result == "" {
		t.Fatal("buildFacets() = empty, want non-empty")
	}
	// Should contain the facet value.
	if !strings.Contains(result, "project_type:plugin") {
		t.Errorf("buildFacets() = %q, want to contain %q", result, "project_type:plugin")
	}
}

func TestBuildFacets_SliceValue(t *testing.T) {
	params := map[string]any{
		"facets": map[string]any{
			"categories": []any{"paper", "spigot"},
		},
	}
	result := buildFacets(params)
	if result == "" {
		t.Fatal("buildFacets() = empty, want non-empty")
	}
	if !strings.Contains(result, "categories:paper") {
		t.Errorf("buildFacets() = %q, want to contain %q", result, "categories:paper")
	}
	if !strings.Contains(result, "categories:spigot") {
		t.Errorf("buildFacets() = %q, want to contain %q", result, "categories:spigot")
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
