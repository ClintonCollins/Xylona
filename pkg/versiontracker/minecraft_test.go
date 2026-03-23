package versiontracker

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// createTestMinecraftJar creates a minimal jar in dir with a version.json
// containing the given version string.
func createTestMinecraftJar(t *testing.T, dir string, fileName string, version string) {
	t.Helper()
	jarPath := filepath.Join(dir, fileName)
	f, errCreate := os.Create(jarPath)
	if errCreate != nil {
		t.Fatalf("create jar: %v", errCreate)
	}
	zw := zip.NewWriter(f)
	w, errEntry := zw.Create("version.json")
	if errEntry != nil {
		t.Fatalf("create zip entry: %v", errEntry)
	}
	versionJSON := fmt.Sprintf(`{"id":"%s","name":"%s"}`, version, version)
	_, errWrite := w.Write([]byte(versionJSON))
	if errWrite != nil {
		t.Fatalf("write version.json: %v", errWrite)
	}
	errClose := zw.Close()
	if errClose != nil {
		t.Fatalf("close zip writer: %v", errClose)
	}
	errCloseFile := f.Close()
	if errCloseFile != nil {
		t.Fatalf("close jar file: %v", errCloseFile)
	}
}

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

func newTestMojangManifestServer(t *testing.T, latestRelease string, requestPath *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestPath != nil {
			*requestPath = r.URL.Path
		}
		w.Header().Set("Content-Type", "application/json")
		_, errWrite := fmt.Fprintf(w, `{
			"latest": {"release": %q, "snapshot": "24w01a"},
			"versions": []
		}`, latestRelease)
		if errWrite != nil {
			t.Errorf("test server write error: %v", errWrite)
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

func TestMinecraftTracker_GetInstalledVersion_ReadsVersionFromJar(t *testing.T) {
	dir := t.TempDir()
	createTestMinecraftJar(t, dir, "minecraft_server.jar", "1.21.4")

	tracker := NewMinecraftTracker()
	gs := &models.GameServer{Directory: dir}
	version, errGet := tracker.GetInstalledVersion(context.Background(), gs)
	if errGet != nil {
		t.Fatalf("unexpected error: %v", errGet)
	}
	if version != "1.21.4" {
		t.Errorf("expected 1.21.4, got %s", version)
	}
}

func TestMinecraftTracker_GetInstalledVersion_UsesServerExecutableJar(t *testing.T) {
	dir := t.TempDir()
	createTestMinecraftJar(t, dir, "paper-1.21.4-100.jar", "1.21.4")

	tracker := NewMinecraftTracker()
	gs := &models.GameServer{
		Directory:        dir,
		ServerExecutable: null.From("paper-1.21.4-100.jar"),
	}
	version, errGet := tracker.GetInstalledVersion(context.Background(), gs)
	if errGet != nil {
		t.Fatalf("unexpected error: %v", errGet)
	}
	if version != "1.21.4" {
		t.Errorf("expected 1.21.4, got %s", version)
	}
}

func TestMinecraftTracker_GetInstalledVersion_FallsBackToDBVersion(t *testing.T) {
	dir := t.TempDir()
	// No jar file in this directory.

	tracker := NewMinecraftTracker()
	gs := &models.GameServer{Directory: dir, Version: "1.20.0"}
	version, errGet := tracker.GetInstalledVersion(context.Background(), gs)
	if errGet != nil {
		t.Fatalf("unexpected error: %v", errGet)
	}
	if version != "1.20.0" {
		t.Errorf("expected 1.20.0, got %s", version)
	}
}

func TestMinecraftTracker_GetInstalledVersion_NoJarNoDBVersion(t *testing.T) {
	dir := t.TempDir()

	tracker := NewMinecraftTracker()
	gs := &models.GameServer{Directory: dir, Version: ""}
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

func TestMinecraftTracker_GetLatestVersion_VanillaUsesMojangLatestRelease(t *testing.T) {
	var paperPath string
	paperSrv := newTestPaperMCServer(t, "paper", []string{"1.20.0", "1.21.4"}, &paperPath)
	defer paperSrv.Close()

	var mojangPath string
	mojangSrv := newTestMojangManifestServer(t, "1.21.5", &mojangPath)
	defer mojangSrv.Close()

	tracker := newMinecraftTrackerWithURL(paperSrv.URL)
	tracker.mojangManifestURL = mojangSrv.URL + "/mc/game/version_manifest.json"

	gs := &models.GameServer{
		ServerSoftware: null.From("vanilla"),
	}
	version, errLatest := tracker.GetLatestVersion(context.Background(), gs)
	if errLatest != nil {
		t.Fatalf("unexpected error: %v", errLatest)
	}
	if version != "1.21.5" {
		t.Errorf("expected 1.21.5, got %s", version)
	}
	if mojangPath != "/mc/game/version_manifest.json" {
		t.Errorf("expected request to /mc/game/version_manifest.json, got %s", mojangPath)
	}
	if paperPath != "" {
		t.Errorf("expected no PaperMC request for vanilla, got %s", paperPath)
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
			strings.Repeat("x", maxVersionAPIResponseBytes+128),
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

	dir := t.TempDir()
	createTestMinecraftJar(t, dir, "minecraft_server.jar", "1.20.0")

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		Directory:      dir,
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

	dir := t.TempDir()
	createTestMinecraftJar(t, dir, "minecraft_server.jar", "1.21.4")

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		Directory:      dir,
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

func TestMinecraftTracker_CheckForUpdate_NoJarFallsBackToDBVersion(t *testing.T) {
	srv := newTestPaperMCServer(t, "paper", []string{"1.21.4"}, nil)
	defer srv.Close()

	dir := t.TempDir()
	// No jar — falls back to gs.Version from DB.

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		Directory:      dir,
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

	dir := t.TempDir()
	// No jar, empty DB version → can't determine installed.

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		Directory:      dir,
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

	dir := t.TempDir()
	createTestMinecraftJar(t, dir, "minecraft_server.jar", "1.21.4")

	tracker := newMinecraftTrackerWithURL(srv.URL)
	gs := &models.GameServer{
		Directory:      dir,
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
