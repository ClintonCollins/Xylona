package papermc

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
// based on a URL-path-to-filename mapping passed via the routes map.
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
	if p.RequiresAPIKey() {
		t.Error("RequiresAPIKey() = true, want false")
	}
}

// --------------------------------------------------------------------------
// Search
// --------------------------------------------------------------------------

func TestSearch_ReturnsProjects(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/projects": "projects.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	searchResult, errSearch := p.Search(context.Background(), "ignored", nil)
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	if len(searchResult.Results) != 4 {
		t.Fatalf("Search() len = %d, want 4", len(searchResult.Results))
	}
	if searchResult.TotalHits != -1 {
		t.Errorf("Search().TotalHits = %d, want -1 for unknown total", searchResult.TotalHits)
	}

	sourceIDs := make(map[string]bool)
	for _, r := range searchResult.Results {
		sourceIDs[r.SourceID] = true
		if r.Source != providerID {
			t.Errorf("result.Source = %q, want %q", r.Source, providerID)
		}
		if r.Name == "" {
			t.Errorf("result.Name for %q is empty", r.SourceID)
		}
	}

	for _, expected := range []string{"paper", "folia", "velocity", "waterfall"} {
		if !sourceIDs[expected] {
			t.Errorf("Search() did not return project %q", expected)
		}
	}
}

func TestSearch_QueryIsIgnored(t *testing.T) {
	// Both calls with different queries should hit the same /projects endpoint.
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/projects": "projects.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	r1, errSearch1 := p.Search(context.Background(), "paper", nil)
	if errSearch1 != nil {
		t.Fatalf("Search(paper) error = %v", errSearch1)
	}
	r2, errSearch2 := p.Search(context.Background(), "completely different query", nil)
	if errSearch2 != nil {
		t.Fatalf("Search(other) error = %v", errSearch2)
	}
	if len(r1.Results) != len(r2.Results) {
		t.Errorf("Search results differ by query: %d vs %d", len(r1.Results), len(r2.Results))
	}
}

func TestSearch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	_, errSearch := p.Search(context.Background(), "paper", nil)
	if errSearch == nil {
		t.Fatal("Search() error = nil, want non-nil on HTTP 503")
	}
}

// --------------------------------------------------------------------------
// GetModDetails
// --------------------------------------------------------------------------

func TestGetModDetails_ReturnsFull(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/projects/paper":                        "project_paper.json",
		"/projects/paper/versions/1.21.4/builds": "builds_paper_1.21.4.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	details, errDetails := p.GetModDetails(context.Background(), "paper", nil)
	if errDetails != nil {
		t.Fatalf("GetModDetails() error = %v", errDetails)
	}
	if details.SourceID != "paper" {
		t.Errorf("details.SourceID = %q, want %q", details.SourceID, "paper")
	}
	if details.Source != providerID {
		t.Errorf("details.Source = %q, want %q", details.Source, providerID)
	}
	if details.Name == "" {
		t.Error("details.Name is empty")
	}
	// Should return game versions (1.20.4, 1.21.1, 1.21.4) from project data.
	if len(details.Versions) != 3 {
		t.Errorf("details.Versions len = %d, want 3", len(details.Versions))
	}
	// Newest first (reversed from API order).
	if len(details.Versions) > 0 && details.Versions[0].VersionID != "1.21.4" {
		t.Errorf("details.Versions[0].VersionID = %q, want %q (newest first)", details.Versions[0].VersionID, "1.21.4")
	}
}

func TestGetModDetails_HTTPError(t *testing.T) {
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

func TestGetModDetails_NoVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"project_id":"paper","versions":[]}`))
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	details, errDetails := p.GetModDetails(context.Background(), "paper", nil)
	if errDetails != nil {
		t.Fatalf("GetModDetails() error = %v", errDetails)
	}
	if len(details.Versions) != 0 {
		t.Errorf("details.Versions len = %d, want 0 when project has no versions", len(details.Versions))
	}
}

// --------------------------------------------------------------------------
// GetVersions
// --------------------------------------------------------------------------

func TestGetVersions_ReturnsMapped(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/projects/paper/versions/1.21.4/builds": "builds_paper_1.21.4.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	versions, errVersions := p.GetVersions(context.Background(), "paper", "1.21.4", nil)
	if errVersions != nil {
		t.Fatalf("GetVersions() error = %v", errVersions)
	}
	if len(versions) != 2 {
		t.Fatalf("GetVersions() len = %d, want 2", len(versions))
	}

	v := versions[0]
	if v.VersionID != "1.21.4-100" {
		t.Errorf("versions[0].VersionID = %q, want %q", v.VersionID, "1.21.4-100")
	}
	if v.VersionString != "Build 100" {
		t.Errorf("versions[0].VersionString = %q, want %q", v.VersionString, "Build 100")
	}
	if len(v.GameVersions) != 1 || v.GameVersions[0] != "1.21.4" {
		t.Errorf("versions[0].GameVersions = %v, want [1.21.4]", v.GameVersions)
	}
	if !strings.Contains(v.Changelog, "Fix chunk loading issue") {
		t.Errorf("versions[0].Changelog = %q, want to contain 'Fix chunk loading issue'", v.Changelog)
	}
	if !strings.Contains(v.Changelog, "Improve performance") {
		t.Errorf("versions[0].Changelog = %q, want to contain 'Improve performance'", v.Changelog)
	}
}

func TestGetVersions_EmptyBuilds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"builds":[]}`))
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	versions, errVersions := p.GetVersions(context.Background(), "paper", "1.21.4", nil)
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

func TestGetVersions_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	_, errVersions := p.GetVersions(context.Background(), "paper", "9.99.99", nil)
	if errVersions == nil {
		t.Fatal("GetVersions() error = nil, want non-nil on HTTP 404")
	}
}

// --------------------------------------------------------------------------
// CheckForUpdate
// --------------------------------------------------------------------------

func TestCheckForUpdate_ReturnsLatestBuild(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/projects/paper/versions/1.21.4/builds": "builds_paper_1.21.4.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	v, errCheck := p.CheckForUpdate(context.Background(), "paper", "1.21.4")
	if errCheck != nil {
		t.Fatalf("CheckForUpdate() error = %v", errCheck)
	}
	if v == nil {
		t.Fatal("CheckForUpdate() = nil, want non-nil")
	}
	// Should return the last build (highest number = latest).
	if v.VersionID != "1.21.4-101" {
		t.Errorf("CheckForUpdate().VersionID = %q, want %q", v.VersionID, "1.21.4-101")
	}
}

func TestCheckForUpdate_NoBuilds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"builds":[]}`))
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	v, errCheck := p.CheckForUpdate(context.Background(), "paper", "1.21.4")
	if errCheck != nil {
		t.Fatalf("CheckForUpdate() error = %v", errCheck)
	}
	if v != nil {
		t.Errorf("CheckForUpdate() = %+v, want nil when no builds", v)
	}
}

// --------------------------------------------------------------------------
// Download
// --------------------------------------------------------------------------

func TestDownload_WritesFile(t *testing.T) {
	jarContent := []byte("fake jar content for testing")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/projects/paper/versions/1.21.4/builds":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"builds": [{
					"build": 100,
					"time": "2024-12-01T10:00:00Z",
					"channel": "default",
					"changes": [],
					"downloads": {
						"application": {
							"name": "paper-1.21.4-100.jar",
							"sha256": ""
						}
					}
				}]
			}`))
		case "/projects/paper/versions/1.21.4/builds/100/downloads/paper-1.21.4-100.jar":
			w.Header().Set("Content-Type", "application/java-archive")
			_, _ = w.Write(jarContent)
		default:
			t.Logf("unexpected path: %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	tmpDir := t.TempDir()

	files, errDownload := p.Download(context.Background(), "paper", "1.21.4-100", tmpDir)
	if errDownload != nil {
		t.Fatalf("Download() error = %v", errDownload)
	}
	if len(files) != 1 {
		t.Fatalf("Download() len = %d, want 1", len(files))
	}

	f := files[0]
	if !f.IsPrimary {
		t.Error("DownloadedFile.IsPrimary = false, want true")
	}
	if f.Size != int64(len(jarContent)) {
		t.Errorf("DownloadedFile.Size = %d, want %d", f.Size, len(jarContent))
	}
	if f.Hash == "" {
		t.Error("DownloadedFile.Hash is empty")
	}

	// Path contract: Download must return a bare filename, not a full path.
	if f.Path != filepath.Base(f.Path) {
		t.Errorf("DownloadedFile.Path = %q, want bare filename (no directory separators)", f.Path)
	}

	// Verify the file was actually written.
	fullPath := filepath.Join(tmpDir, f.Path)
	data, errRead := os.ReadFile(fullPath)
	if errRead != nil {
		t.Fatalf("ReadFile(%s) error = %v", fullPath, errRead)
	}
	if string(data) != string(jarContent) {
		t.Errorf("downloaded file content = %q, want %q", string(data), string(jarContent))
	}
}

func TestDownload_InvalidVersionID(t *testing.T) {
	p := New()
	_, errDownload := p.Download(context.Background(), "paper", "nohyphen", t.TempDir())
	if errDownload == nil {
		t.Fatal("Download() error = nil, want non-nil for invalid versionID")
	}
}

func TestDownload_BuildNotFound(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler(t, map[string]string{
		"/projects/paper/versions/1.21.4/builds": "builds_paper_1.21.4.json",
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	// Build 999 doesn't exist in the fixture.
	_, errDownload := p.Download(context.Background(), "paper", "1.21.4-999", t.TempDir())
	if errDownload == nil {
		t.Fatal("Download() error = nil, want non-nil when build not found")
	}
}

func TestDownload_SHA256Mismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/projects/paper/versions/1.21.4/builds":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"builds": [{
					"build": 100,
					"time": "2024-12-01T10:00:00Z",
					"channel": "default",
					"changes": [],
					"downloads": {
						"application": {
							"name": "paper-1.21.4-100.jar",
							"sha256": "badhash000000000000000000000000000000000000000000000000000000000"
						}
					}
				}]
			}`))
		case "/projects/paper/versions/1.21.4/builds/100/downloads/paper-1.21.4-100.jar":
			_, _ = w.Write([]byte("real content"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	_, errDownload := p.Download(context.Background(), "paper", "1.21.4-100", t.TempDir())
	if errDownload == nil {
		t.Fatal("Download() error = nil, want non-nil on SHA-256 mismatch")
	}
	if !strings.Contains(errDownload.Error(), "SHA-256 mismatch") {
		t.Errorf("Download() error = %q, want to contain 'SHA-256 mismatch'", errDownload.Error())
	}
}

// --------------------------------------------------------------------------
// parseVersionID
// --------------------------------------------------------------------------

func TestParseVersionID(t *testing.T) {
	tests := []struct {
		name        string
		versionID   string
		wantVersion string
		wantBuild   int
		wantErr     bool
	}{
		{
			name:        "simple version",
			versionID:   "1.21.4-100",
			wantVersion: "1.21.4",
			wantBuild:   100,
			wantErr:     false,
		},
		{
			name:        "version with pre-release",
			versionID:   "1.21.4-pre1-50",
			wantVersion: "1.21.4-pre1",
			wantBuild:   50,
			wantErr:     false,
		},
		{
			name:        "no hyphen",
			versionID:   "1214100",
			wantErr:     true,
		},
		{
			name:        "non-integer build",
			versionID:   "1.21.4-abc",
			wantErr:     true,
		},
		{
			name:        "empty string",
			versionID:   "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			version, build, errParse := parseVersionID(tt.versionID)
			if (errParse != nil) != tt.wantErr {
				t.Fatalf("parseVersionID(%q) error = %v, wantErr %v", tt.versionID, errParse, tt.wantErr)
			}
			if !tt.wantErr {
				if version != tt.wantVersion {
					t.Errorf("parseVersionID(%q) version = %q, want %q", tt.versionID, version, tt.wantVersion)
				}
				if build != tt.wantBuild {
					t.Errorf("parseVersionID(%q) build = %d, want %d", tt.versionID, build, tt.wantBuild)
				}
			}
		})
	}
}
