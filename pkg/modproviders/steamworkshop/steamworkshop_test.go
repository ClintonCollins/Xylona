package steamworkshop

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

// newTestProviderWithKey creates a Provider with an API key and routes all requests
// to the given httptest.Server.
func newTestProviderWithKey(srv *httptest.Server, apiKey string) *Provider {
	p := newTestProvider(srv)
	p.apiKey = apiKey
	return p
}

// fixtureHandler returns an http.HandlerFunc that serves fixture files from the testdata
// directory. The routes map maps URL paths to filenames relative to testdata/.
// POST and GET requests to the same path both work.
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
	if !p.RequiresAPIKey() {
		t.Error("RequiresAPIKey() = false, want true")
	}
}

func TestSetAPIKey(t *testing.T) {
	p := New()
	if p.apiKey != "" {
		t.Errorf("New() apiKey = %q, want empty", p.apiKey)
	}
	p.SetAPIKey("test-key")
	if p.apiKey != "test-key" {
		t.Errorf("SetAPIKey() apiKey = %q, want %q", p.apiKey, "test-key")
	}
}

func TestSetSteamCMDPath(t *testing.T) {
	p := New()
	if p.steamCMDPath != "" {
		t.Errorf("New() steamCMDPath = %q, want empty", p.steamCMDPath)
	}
	p.SetSteamCMDPath("/usr/local/bin/steamcmd")
	if p.steamCMDPath != "/usr/local/bin/steamcmd" {
		t.Errorf("SetSteamCMDPath() steamCMDPath = %q, want %q", p.steamCMDPath, "/usr/local/bin/steamcmd")
	}
}

// --------------------------------------------------------------------------
// Search
// --------------------------------------------------------------------------

func TestSearch_WithAPIKey_ReturnsResults(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/IPublishedFileService/QueryFiles/v1/": "search_results.json",
	}))
	defer srv.Close()

	p := newTestProviderWithKey(srv, "fake-api-key")
	searchResult, errSearch := p.Search(context.Background(), "helicopter", nil)
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	if len(searchResult.Results) != 2 {
		t.Fatalf("Search() len = %d, want 2", len(searchResult.Results))
	}
	if searchResult.TotalHits != 2 {
		t.Errorf("Search().TotalHits = %d, want 2", searchResult.TotalHits)
	}

	first := searchResult.Results[0]
	if first.SourceID != "2128699613" {
		t.Errorf("results[0].SourceID = %q, want %q", first.SourceID, "2128699613")
	}
	if first.Name != "Project Zomboid Expanded Helicopter Events" {
		t.Errorf("results[0].Name = %q, want %q", first.Name, "Project Zomboid Expanded Helicopter Events")
	}
	if first.Source != providerID {
		t.Errorf("results[0].Source = %q, want %q", first.Source, providerID)
	}
	if first.Author != "76561198012345678" {
		t.Errorf("results[0].Author = %q, want %q", first.Author, "76561198012345678")
	}
	if first.Downloads != 500000 {
		t.Errorf("results[0].Downloads = %d, want 500000", first.Downloads)
	}
	if first.IconURL == "" {
		t.Error("results[0].IconURL is empty, want non-empty")
	}
	if len(first.Categories) != 2 {
		t.Errorf("results[0].Categories len = %d, want 2", len(first.Categories))
	}
	if first.DateModified == "" {
		t.Error("results[0].DateModified is empty, want non-empty")
	}
}

func TestSearch_WithoutAPIKey_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Should never be called — the provider should return early.
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	_, errSearch := p.Search(context.Background(), "helicopter", nil)
	if errSearch == nil {
		t.Fatal("Search() error = nil, want non-nil when API key is missing")
	}
	if !strings.Contains(errSearch.Error(), "API key") {
		t.Errorf("Search() error = %q, want message to contain 'API key'", errSearch.Error())
	}
}

func TestSearch_HTTPError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := newTestProviderWithKey(srv, "fake-api-key")
	_, errSearch := p.Search(context.Background(), "helicopter", nil)
	if errSearch == nil {
		t.Fatal("Search() error = nil, want non-nil on HTTP 429")
	}
}

func TestSearch_WithAppID_IncludesInRequest(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"total":0,"publishedfiledetails":[]}}`))
	}))
	defer srv.Close()

	p := newTestProviderWithKey(srv, "fake-api-key")
	params := map[string]any{"app_id": "108600"}
	_, errSearch := p.Search(context.Background(), "mod", params)
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	if !strings.Contains(capturedQuery, "appid=108600") {
		t.Errorf("query string = %q, want to contain 'appid=108600'", capturedQuery)
	}
}

func TestSearch_EmptyResults_ReturnsEmptySlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"total":0,"publishedfiledetails":[]}}`))
	}))
	defer srv.Close()

	p := newTestProviderWithKey(srv, "fake-api-key")
	searchResult, errSearch := p.Search(context.Background(), "nonexistent", nil)
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

// --------------------------------------------------------------------------
// GetModDetails
// --------------------------------------------------------------------------

func TestGetModDetails_ReturnsFull(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/ISteamRemoteStorage/GetPublishedFileDetails/v1/": "published_file_details.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	details, errDetails := p.GetModDetails(context.Background(), "2128699613", nil)
	if errDetails != nil {
		t.Fatalf("GetModDetails() error = %v", errDetails)
	}

	if details.SourceID != "2128699613" {
		t.Errorf("details.SourceID = %q, want %q", details.SourceID, "2128699613")
	}
	if details.Name != "Project Zomboid Expanded Helicopter Events" {
		t.Errorf("details.Name = %q, want %q", details.Name, "Project Zomboid Expanded Helicopter Events")
	}
	if details.Source != providerID {
		t.Errorf("details.Source = %q, want %q", details.Source, providerID)
	}
	if details.Author != "76561198012345678" {
		t.Errorf("details.Author = %q, want %q", details.Author, "76561198012345678")
	}
	if details.Downloads != 500000 {
		t.Errorf("details.Downloads = %d, want 500000", details.Downloads)
	}
	if details.IconURL == "" {
		t.Error("details.IconURL is empty, want non-empty")
	}
	if len(details.Categories) != 3 {
		t.Errorf("details.Categories len = %d, want 3", len(details.Categories))
	}
	if len(details.Versions) != 1 {
		t.Errorf("details.Versions len = %d, want 1 (current state only)", len(details.Versions))
	}
}

func TestGetModDetails_HTTPError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	_, errDetails := p.GetModDetails(context.Background(), "9999999999", nil)
	if errDetails == nil {
		t.Fatal("GetModDetails() error = nil, want non-nil for 404")
	}
}

// --------------------------------------------------------------------------
// GetVersions
// --------------------------------------------------------------------------

func TestGetVersions_ReturnsSingleCurrentVersion(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/ISteamRemoteStorage/GetPublishedFileDetails/v1/": "published_file_details.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	versions, errVersions := p.GetVersions(context.Background(), "2128699613", "", nil)
	if errVersions != nil {
		t.Fatalf("GetVersions() error = %v", errVersions)
	}
	if len(versions) != 1 {
		t.Fatalf("GetVersions() len = %d, want 1 (current state only)", len(versions))
	}

	v := versions[0]
	if v.VersionID != "2128699613" {
		t.Errorf("versions[0].VersionID = %q, want %q", v.VersionID, "2128699613")
	}
	if !strings.HasPrefix(v.VersionString, "updated_") {
		t.Errorf("versions[0].VersionString = %q, want to start with 'updated_'", v.VersionString)
	}
	if v.FileSize != 15728640 {
		t.Errorf("versions[0].FileSize = %d, want 15728640", v.FileSize)
	}
	if v.Changelog == "" {
		t.Error("versions[0].Changelog is empty, want non-empty")
	}
}

func TestGetVersions_NonSuccessResult_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"result":1,"resultcount":1,"publishedfiledetails":[{"publishedfileid":"9999","result":9}]}}`))
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	_, errVersions := p.GetVersions(context.Background(), "9999", "", nil)
	if errVersions == nil {
		t.Fatal("GetVersions() error = nil, want non-nil for non-success result")
	}
}

// --------------------------------------------------------------------------
// CheckForUpdate
// --------------------------------------------------------------------------

func TestCheckForUpdate_ReturnsCurrentVersion(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/ISteamRemoteStorage/GetPublishedFileDetails/v1/": "published_file_details.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	v, errCheck := p.CheckForUpdate(context.Background(), "2128699613", "")
	if errCheck != nil {
		t.Fatalf("CheckForUpdate() error = %v", errCheck)
	}
	if v == nil {
		t.Fatal("CheckForUpdate() = nil, want non-nil")
	}
	if v.VersionID != "2128699613" {
		t.Errorf("CheckForUpdate().VersionID = %q, want %q", v.VersionID, "2128699613")
	}
}

func TestCheckForUpdate_HTTPError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	_, errCheck := p.CheckForUpdate(context.Background(), "2128699613", "")
	if errCheck == nil {
		t.Fatal("CheckForUpdate() error = nil, want non-nil on HTTP 500")
	}
}

// --------------------------------------------------------------------------
// Download
// --------------------------------------------------------------------------

func TestDownload_WithoutSteamCMDPath_ReturnsError(t *testing.T) {
	p := New()
	tmpDir := t.TempDir()
	_, errDownload := p.Download(context.Background(), "2128699613", "", tmpDir)
	if errDownload == nil {
		t.Fatal("Download() error = nil, want non-nil when steamcmd path is not set")
	}
	if !strings.Contains(errDownload.Error(), "steamcmd") {
		t.Errorf("Download() error = %q, want message to contain 'steamcmd'", errDownload.Error())
	}
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

func TestExtractAppID_Nil(t *testing.T) {
	result := extractAppID(nil)
	if result != "" {
		t.Errorf("extractAppID(nil) = %q, want empty", result)
	}
}

func TestExtractAppID_String(t *testing.T) {
	params := map[string]any{"app_id": "108600"}
	result := extractAppID(params)
	if result != "108600" {
		t.Errorf("extractAppID() = %q, want %q", result, "108600")
	}
}

func TestExtractAppID_Int(t *testing.T) {
	params := map[string]any{"app_id": 108600}
	result := extractAppID(params)
	if result != "108600" {
		t.Errorf("extractAppID() = %q, want %q", result, "108600")
	}
}

func TestExtractAppID_Float64(t *testing.T) {
	params := map[string]any{"app_id": float64(108600)}
	result := extractAppID(params)
	if result != "108600" {
		t.Errorf("extractAppID() = %q, want %q", result, "108600")
	}
}

func TestExtractAppID_NoKey(t *testing.T) {
	params := map[string]any{"other": "value"}
	result := extractAppID(params)
	if result != "" {
		t.Errorf("extractAppID() = %q, want empty when no app_id key", result)
	}
}

func TestParseFileSize_Valid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int64
	}{
		{name: "non-zero size", input: "15728640", want: 15728640},
		{name: "zero size", input: "0", want: 0},
		{name: "empty string", input: "", want: 0},
		{name: "whitespace", input: "  8388608  ", want: 8388608},
		{name: "invalid", input: "not-a-number", want: 0},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := parseFileSize(tt.input)
			if got != tt.want {
				t.Errorf("parseFileSize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestCopyDirContents_CopiesFiles(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Write test files to src.
	errWrite1 := os.WriteFile(filepath.Join(srcDir, "mod.lua"), []byte("-- mod script"), 0o644)
	if errWrite1 != nil {
		t.Fatalf("write test file: %v", errWrite1)
	}
	subDir := filepath.Join(srcDir, "textures")
	errMkdir := os.MkdirAll(subDir, 0o755)
	if errMkdir != nil {
		t.Fatalf("create subdir: %v", errMkdir)
	}
	errWrite2 := os.WriteFile(filepath.Join(subDir, "icon.png"), []byte("fake png"), 0o644)
	if errWrite2 != nil {
		t.Fatalf("write test file in subdir: %v", errWrite2)
	}

	downloaded, errCopy := copyDirContents(srcDir, dstDir)
	if errCopy != nil {
		t.Fatalf("copyDirContents() error = %v", errCopy)
	}
	if len(downloaded) != 2 {
		t.Fatalf("copyDirContents() len = %d, want 2", len(downloaded))
	}

	// First file should be marked primary.
	if !downloaded[0].IsPrimary {
		t.Error("downloaded[0].IsPrimary = false, want true")
	}
	if downloaded[1].IsPrimary {
		t.Error("downloaded[1].IsPrimary = true, want false")
	}

	// Verify files actually exist at destination.
	// Path contract: copyDirContents must return relative paths, not absolute.
	for _, f := range downloaded {
		if filepath.IsAbs(f.Path) {
			t.Errorf("DownloadedFile.Path = %q, want relative path (not absolute)", f.Path)
		}
		fullPath := filepath.Join(dstDir, f.Path)
		_, errStat := os.Stat(fullPath)
		if errStat != nil {
			t.Errorf("copied file not found at %s: %v", fullPath, errStat)
		}
		if f.Size == 0 {
			t.Errorf("copied file %s has Size=0, want >0", f.Path)
		}
	}
}

func TestCopyDirContents_NonexistentSrc_ReturnsError(t *testing.T) {
	dstDir := t.TempDir()
	_, errCopy := copyDirContents("/nonexistent/path/that/does/not/exist", dstDir)
	if errCopy == nil {
		t.Fatal("copyDirContents() error = nil, want non-nil for nonexistent source")
	}
}
