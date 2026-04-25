package versiontracker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ClintonCollins/Xylona/sql/models"
)

const testACFContent = `"AppState"
{
	"appid"		"730"
	"Universe"		"1"
	"name"		"Counter-Strike 2"
	"buildid"		"7567790"
	"LastUpdated"		"1706000000"
}
`

const testACFMalformed = `"AppState"
{
	"appid"		"730"
	"name"		"Counter-Strike 2"
}
`

// writeACF writes an ACF file with the given filename into dir.
func writeACF(t *testing.T, dir, filename, content string) {
	t.Helper()
	path := filepath.Join(dir, filename)
	errWrite := os.WriteFile(path, []byte(content), 0o600)
	if errWrite != nil {
		t.Fatalf("failed to write ACF file: %v", errWrite)
	}
}

func TestSteamTracker_GetInstalledVersion_ParsesBuildID(t *testing.T) {
	dir := t.TempDir()
	writeACF(t, dir, "appmanifest_730.acf", testACFContent)

	tracker := NewSteamTracker()
	gs := &models.GameServer{Directory: dir}

	version, errGet := tracker.GetInstalledVersion(context.Background(), gs)
	if errGet != nil {
		t.Fatalf("unexpected error: %v", errGet)
	}
	if version != "7567790" {
		t.Errorf("expected buildid 7567790, got %q", version)
	}
}

func TestSteamTracker_GetInstalledVersion_NoManifest(t *testing.T) {
	dir := t.TempDir()

	tracker := NewSteamTracker()
	gs := &models.GameServer{Directory: dir}

	version, errGet := tracker.GetInstalledVersion(context.Background(), gs)
	if errGet != nil {
		t.Fatalf("unexpected error: %v", errGet)
	}
	if version != "" {
		t.Errorf("expected empty string when no manifest, got %q", version)
	}
}

func TestSteamTracker_GetInstalledVersion_MalformedManifest(t *testing.T) {
	dir := t.TempDir()
	writeACF(t, dir, "appmanifest_730.acf", testACFMalformed)

	tracker := NewSteamTracker()
	gs := &models.GameServer{Directory: dir}

	version, errGet := tracker.GetInstalledVersion(context.Background(), gs)
	if errGet != nil {
		t.Fatalf("unexpected error on malformed manifest: %v", errGet)
	}
	if version != "" {
		t.Errorf("expected empty string for malformed manifest, got %q", version)
	}
}

func TestSteamTracker_GetInstalledVersion_FindsManifestInSteamAppsDirectory(t *testing.T) {
	dir := t.TempDir()
	steamappsDir := filepath.Join(dir, "steamapps")
	errMkdir := os.MkdirAll(steamappsDir, 0o750)
	if errMkdir != nil {
		t.Fatalf("failed to create steamapps directory: %v", errMkdir)
	}
	writeACF(t, steamappsDir, "appmanifest_730.acf", testACFContent)

	tracker := NewSteamTracker()
	gs := &models.GameServer{Directory: dir}

	version, errGet := tracker.GetInstalledVersion(context.Background(), gs)
	if errGet != nil {
		t.Fatalf("unexpected error: %v", errGet)
	}
	if version != "7567790" {
		t.Errorf("expected buildid 7567790 from steamapps manifest, got %q", version)
	}
}

func TestSteamTracker_GetInstalledVersion_PrefersConfiguredAppIDWhenMultipleManifestsExist(t *testing.T) {
	dir := t.TempDir()
	steamappsDir := filepath.Join(dir, "steamapps")
	errMkdir := os.MkdirAll(steamappsDir, 0o750)
	if errMkdir != nil {
		t.Fatalf("failed to create steamapps directory: %v", errMkdir)
	}
	writeACF(t, steamappsDir, "appmanifest_228980.acf", `"AppState"
{
	"appid"		"228980"
	"buildid"		"19222509"
}
`)
	writeACF(t, steamappsDir, "appmanifest_294420.acf", `"AppState"
{
	"appid"		"294420"
	"buildid"		"21600865"
}
`)

	tracker := NewSteamTrackerWithAppID("294420")
	gs := &models.GameServer{Directory: dir}

	version, errGet := tracker.GetInstalledVersion(context.Background(), gs)
	if errGet != nil {
		t.Fatalf("unexpected error: %v", errGet)
	}
	if version != "21600865" {
		t.Errorf("expected buildid 21600865 from preferred app manifest, got %q", version)
	}
}

func TestSteamTracker_GetLatestVersion_QueriesSteamAPI(t *testing.T) {
	type steamResponse struct {
		Response struct {
			Success         bool  `json:"success"`
			UpToDate        bool  `json:"up_to_date"`
			RequiredVersion int64 `json:"required_version"`
		} `json:"response"`
	}
	resp := steamResponse{}
	resp.Response.Success = true
	resp.Response.UpToDate = false
	resp.Response.RequiredVersion = 99999999

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		errEncode := json.NewEncoder(w).Encode(resp)
		if errEncode != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	writeACF(t, dir, "appmanifest_730.acf", testACFContent)

	tracker := &SteamTracker{
		httpClient:  server.Client(),
		steamAPIURL: server.URL,
	}
	gs := &models.GameServer{Directory: dir}

	version, errGet := tracker.GetLatestVersion(context.Background(), gs)
	if errGet != nil {
		t.Fatalf("unexpected error: %v", errGet)
	}
	if version != "99999999" {
		t.Errorf("expected required_version 99999999, got %q", version)
	}
}

func TestSteamTracker_GetLatestVersion_NoManifest(t *testing.T) {
	dir := t.TempDir()

	tracker := NewSteamTracker()
	gs := &models.GameServer{Directory: dir}

	version, errGet := tracker.GetLatestVersion(context.Background(), gs)
	if errGet != nil {
		t.Fatalf("unexpected error: %v", errGet)
	}
	if version != "" {
		t.Errorf("expected empty string when no manifest, got %q", version)
	}
}

func TestSteamTracker_CheckForUpdate_UpdateAvailable(t *testing.T) {
	// Use an httptest server to mock Steam API returning latest=2000.
	type steamResponse struct {
		Response struct {
			Success         bool  `json:"success"`
			RequiredVersion int64 `json:"required_version"`
		} `json:"response"`
	}
	resp := steamResponse{}
	resp.Response.Success = true
	resp.Response.RequiredVersion = 2000

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		errEncode := json.NewEncoder(w).Encode(resp)
		if errEncode != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	// Write a manifest with buildid 1000 so installed != latest.
	acfContent := `"AppState"
{
	"appid"		"730"
	"buildid"		"1000"
}
`
	dir := t.TempDir()
	writeACF(t, dir, "appmanifest_730.acf", acfContent)

	tracker := &SteamTracker{
		httpClient:  server.Client(),
		steamAPIURL: server.URL,
	}
	gs := &models.GameServer{Directory: dir}

	info, errCheck := tracker.CheckForUpdate(context.Background(), gs)
	if errCheck != nil {
		t.Fatalf("unexpected error: %v", errCheck)
	}
	if info == nil {
		t.Fatal("expected non-nil UpdateInfo when update is available")
	}
	if !info.UpdateAvailable {
		t.Error("expected UpdateAvailable to be true")
	}
	if info.InstalledVersion != "1000" {
		t.Errorf("expected InstalledVersion 1000, got %q", info.InstalledVersion)
	}
	if info.LatestVersion != "2000" {
		t.Errorf("expected LatestVersion 2000, got %q", info.LatestVersion)
	}
}

func TestSteamTracker_CheckForUpdate_UpToDate(t *testing.T) {
	// Steam API returns required_version matching the installed buildid.
	type steamResponse struct {
		Response struct {
			Success         bool  `json:"success"`
			RequiredVersion int64 `json:"required_version"`
		} `json:"response"`
	}
	resp := steamResponse{}
	resp.Response.Success = true
	resp.Response.RequiredVersion = 7567790

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		errEncode := json.NewEncoder(w).Encode(resp)
		if errEncode != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	writeACF(t, dir, "appmanifest_730.acf", testACFContent) // buildid=7567790

	tracker := &SteamTracker{
		httpClient:  server.Client(),
		steamAPIURL: server.URL,
	}
	gs := &models.GameServer{Directory: dir}

	info, errCheck := tracker.CheckForUpdate(context.Background(), gs)
	if errCheck != nil {
		t.Fatalf("unexpected error: %v", errCheck)
	}
	if info == nil {
		t.Fatal("expected non-nil UpdateInfo when up to date")
	}
	if info.UpdateAvailable {
		t.Errorf("expected UpdateAvailable = false when up to date, got %+v", info)
	}
}

func TestSteamTracker_CheckForUpdate_NoManifest(t *testing.T) {
	dir := t.TempDir()

	tracker := NewSteamTracker()
	gs := &models.GameServer{Directory: dir}

	info, errCheck := tracker.CheckForUpdate(context.Background(), gs)
	if errCheck != nil {
		t.Fatalf("unexpected error: %v", errCheck)
	}
	if info == nil {
		t.Fatal("expected non-nil UpdateInfo when no manifest is present")
	}
	if info.UpdateAvailable {
		t.Errorf("expected UpdateAvailable = false when no manifest is present, got %+v", info)
	}
}
