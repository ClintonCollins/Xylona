package versiontracker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// newTestPaperMCServer returns an httptest.Server that serves a static versions list
// for the given project. It also records the last request path for inspection.
func newTestPaperMCServer(t *testing.T, project string, versions []string, requestPath *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestPath != nil {
			*requestPath = r.URL.Path
		}
		resp := paperMCProjectsResponse{
			ProjectID:   project,
			ProjectName: project,
			Versions:    versions,
		}
		w.Header().Set("Content-Type", "application/json")
		errEncode := json.NewEncoder(w).Encode(resp)
		if errEncode != nil {
			t.Errorf("test server encode error: %v", errEncode)
		}
	}))
}

// serverSoftwareJSON builds the JSON string for a single software entry.
func serverSoftwareJSON(jarSource string) string {
	entries := []serverSoftwareEntry{{ID: jarSource, Name: jarSource, JarSource: jarSource}}
	b, _ := json.Marshal(entries)
	return string(b)
}

// --- GetInstalledVersion ---

func TestMinecraftTracker_GetInstalledVersion_ReturnsVersionField(t *testing.T) {
	tracker := NewMinecraftTracker()
	gs := &models.GameServer{Version: "1.21.4"}
	version, errGet := tracker.GetInstalledVersion(context.Background(), gs)
	if errGet != nil {
		t.Fatalf("unexpected error: %v", errGet)
	}
	if version != "1.21.4" {
		t.Errorf("expected 1.21.4, got %s", version)
	}
}

func TestMinecraftTracker_GetInstalledVersion_EmptyVersion(t *testing.T) {
	tracker := NewMinecraftTracker()
	gs := &models.GameServer{Version: ""}
	version, errGet := tracker.GetInstalledVersion(context.Background(), gs)
	if errGet != nil {
		t.Fatalf("unexpected error: %v", errGet)
	}
	if version != "" {
		t.Errorf("expected empty string, got %s", version)
	}
}

// --- GetLatestVersion ---

func TestMinecraftTracker_GetLatestVersion_QueriesPaperMCAPI(t *testing.T) {
	srv := newTestPaperMCServer(t, "paper", []string{"1.20.0", "1.21.0", "1.21.4"}, nil)
	defer srv.Close()

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		ServerSoftware: null.From(serverSoftwareJSON("papermc")),
	}
	version, errLatest := tracker.GetLatestVersion(context.Background(), gs)
	if errLatest != nil {
		t.Fatalf("unexpected error: %v", errLatest)
	}
	if version != "1.21.4" {
		t.Errorf("expected 1.21.4, got %s", version)
	}
}

func TestMinecraftTracker_GetLatestVersion_DefaultsToPaper(t *testing.T) {
	var capturedPath string
	srv := newTestPaperMCServer(t, "paper", []string{"1.19.0", "1.21.4"}, &capturedPath)
	defer srv.Close()

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		// Empty ServerSoftware — should default to "paper"
		ServerSoftware: null.FromPtr[string](nil),
	}
	version, errLatest := tracker.GetLatestVersion(context.Background(), gs)
	if errLatest != nil {
		t.Fatalf("unexpected error: %v", errLatest)
	}
	if version != "1.21.4" {
		t.Errorf("expected 1.21.4, got %s", version)
	}
	if capturedPath != "/projects/paper" {
		t.Errorf("expected request to /projects/paper, got %s", capturedPath)
	}
}

func TestMinecraftTracker_GetLatestVersion_FoliaProject(t *testing.T) {
	var capturedPath string
	srv := newTestPaperMCServer(t, "folia", []string{"1.21.0", "1.21.4"}, &capturedPath)
	defer srv.Close()

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		ServerSoftware: null.From(serverSoftwareJSON("folia")),
	}
	version, errLatest := tracker.GetLatestVersion(context.Background(), gs)
	if errLatest != nil {
		t.Fatalf("unexpected error: %v", errLatest)
	}
	if version != "1.21.4" {
		t.Errorf("expected 1.21.4, got %s", version)
	}
	if capturedPath != "/projects/folia" {
		t.Errorf("expected request to /projects/folia, got %s", capturedPath)
	}
}

func TestMinecraftTracker_GetLatestVersion_WaterfallProject(t *testing.T) {
	var capturedPath string
	srv := newTestPaperMCServer(t, "waterfall", []string{"1.20.0", "1.21.0"}, &capturedPath)
	defer srv.Close()

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		ServerSoftware: null.From(serverSoftwareJSON("waterfall")),
	}
	version, errLatest := tracker.GetLatestVersion(context.Background(), gs)
	if errLatest != nil {
		t.Fatalf("unexpected error: %v", errLatest)
	}
	if version != "1.21.0" {
		t.Errorf("expected 1.21.0, got %s", version)
	}
	if capturedPath != "/projects/waterfall" {
		t.Errorf("expected request to /projects/waterfall, got %s", capturedPath)
	}
}

func TestMinecraftTracker_GetLatestVersion_VelocityProject(t *testing.T) {
	var capturedPath string
	srv := newTestPaperMCServer(t, "velocity", []string{"3.3.0", "3.4.0"}, &capturedPath)
	defer srv.Close()

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		ServerSoftware: null.From(serverSoftwareJSON("velocity")),
	}
	version, errLatest := tracker.GetLatestVersion(context.Background(), gs)
	if errLatest != nil {
		t.Fatalf("unexpected error: %v", errLatest)
	}
	if version != "3.4.0" {
		t.Errorf("expected 3.4.0, got %s", version)
	}
	if capturedPath != "/projects/velocity" {
		t.Errorf("expected request to /projects/velocity, got %s", capturedPath)
	}
}

func TestMinecraftTracker_GetLatestVersion_UnknownJarSourceDefaultsToPaper(t *testing.T) {
	var capturedPath string
	srv := newTestPaperMCServer(t, "paper", []string{"1.21.4"}, &capturedPath)
	defer srv.Close()

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		ServerSoftware: null.From(serverSoftwareJSON("some-unknown-source")),
	}
	version, errLatest := tracker.GetLatestVersion(context.Background(), gs)
	if errLatest != nil {
		t.Fatalf("unexpected error: %v", errLatest)
	}
	if version != "1.21.4" {
		t.Errorf("expected 1.21.4, got %s", version)
	}
	if capturedPath != "/projects/paper" {
		t.Errorf("expected default request to /projects/paper, got %s", capturedPath)
	}
}

func TestMinecraftTracker_GetLatestVersion_EmptyVersionsList(t *testing.T) {
	srv := newTestPaperMCServer(t, "paper", []string{}, nil)
	defer srv.Close()

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{}
	version, errLatest := tracker.GetLatestVersion(context.Background(), gs)
	if errLatest != nil {
		t.Fatalf("unexpected error: %v", errLatest)
	}
	if version != "" {
		t.Errorf("expected empty string for empty versions list, got %s", version)
	}
}

func TestMinecraftTracker_GetLatestVersion_RejectsOversizedResponses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, errWrite := fmt.Fprintf(
			w,
			`{"project_id":"paper","project_name":"paper","versions":["%s"]}`,
			strings.Repeat("x", (maxPaperMCResponseBytes)+128),
		)
		if errWrite != nil {
			t.Errorf("test server write error: %v", errWrite)
		}
	}))
	defer srv.Close()

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{}

	_, errLatest := tracker.GetLatestVersion(context.Background(), gs)
	if errLatest == nil {
		t.Fatal("GetLatestVersion() error = nil, want oversized response error")
	}
}

// --- CheckForUpdate ---

func TestMinecraftTracker_CheckForUpdate_UpdateAvailable(t *testing.T) {
	srv := newTestPaperMCServer(t, "paper", []string{"1.20.0", "1.21.4"}, nil)
	defer srv.Close()

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		Version:        "1.20.0",
		ServerSoftware: null.From(serverSoftwareJSON("paper")),
	}
	info, errCheck := tracker.CheckForUpdate(context.Background(), gs)
	if errCheck != nil {
		t.Fatalf("unexpected error: %v", errCheck)
	}
	if info == nil {
		t.Fatal("expected non-nil UpdateInfo")
	}
	if !info.UpdateAvailable {
		t.Error("expected UpdateAvailable to be true")
	}
	if info.InstalledVersion != "1.20.0" {
		t.Errorf("expected InstalledVersion 1.20.0, got %s", info.InstalledVersion)
	}
	if info.LatestVersion != "1.21.4" {
		t.Errorf("expected LatestVersion 1.21.4, got %s", info.LatestVersion)
	}
}

func TestMinecraftTracker_CheckForUpdate_UpToDate(t *testing.T) {
	srv := newTestPaperMCServer(t, "paper", []string{"1.21.4"}, nil)
	defer srv.Close()

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		Version:        "1.21.4",
		ServerSoftware: null.From(serverSoftwareJSON("paper")),
	}
	info, errCheck := tracker.CheckForUpdate(context.Background(), gs)
	if errCheck != nil {
		t.Fatalf("unexpected error: %v", errCheck)
	}
	if info != nil {
		t.Errorf("expected nil UpdateInfo (up to date), got %+v", info)
	}
}

func TestMinecraftTracker_CheckForUpdate_TrimsWhitespaceBeforeComparing(t *testing.T) {
	srv := newTestPaperMCServer(t, "paper", []string{"1.21.4"}, nil)
	defer srv.Close()

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		Version:        " 1.21.4 \n",
		ServerSoftware: null.From(serverSoftwareJSON("paper")),
	}

	info, errCheck := tracker.CheckForUpdate(context.Background(), gs)
	if errCheck != nil {
		t.Fatalf("unexpected error: %v", errCheck)
	}
	if info != nil {
		t.Fatalf("expected nil UpdateInfo after trimming, got %+v", info)
	}
}

func TestMinecraftTracker_CheckForUpdate_EmptyInstalled(t *testing.T) {
	srv := newTestPaperMCServer(t, "paper", []string{"1.21.4"}, nil)
	defer srv.Close()

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		Version:        "",
		ServerSoftware: null.From(serverSoftwareJSON("paper")),
	}
	info, errCheck := tracker.CheckForUpdate(context.Background(), gs)
	if errCheck != nil {
		t.Fatalf("unexpected error: %v", errCheck)
	}
	if info != nil {
		t.Errorf("expected nil UpdateInfo (can't determine), got %+v", info)
	}
}

func TestMinecraftTracker_CheckForUpdate_EmptyLatest(t *testing.T) {
	// Server returns empty versions list → latest is "" → can't determine
	srv := newTestPaperMCServer(t, "paper", []string{}, nil)
	defer srv.Close()

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		Version:        "1.21.4",
		ServerSoftware: null.From(serverSoftwareJSON("paper")),
	}
	info, errCheck := tracker.CheckForUpdate(context.Background(), gs)
	if errCheck != nil {
		t.Fatalf("unexpected error: %v", errCheck)
	}
	if info != nil {
		t.Errorf("expected nil UpdateInfo (can't determine latest), got %+v", info)
	}
}
