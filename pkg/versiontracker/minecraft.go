package versiontracker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// maxPaperMCResponseBytes is the maximum size (in bytes) allowed for reading a PaperMC API response (10 MiB).
const maxPaperMCResponseBytes = 10 << 20

// paperMCProjectsResponse is the response from the PaperMC /v2/projects/{project} endpoint.
type paperMCProjectsResponse struct {
	ProjectID   string   `json:"project_id"`
	ProjectName string   `json:"project_name"`
	Versions    []string `json:"versions"`
}

// serverSoftwareEntry represents a single entry in a game server's ServerSoftware JSON array.
type serverSoftwareEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	JarSource string `json:"jar_source"`
}

// MinecraftTracker is a VersionTracker for Minecraft servers using PaperMC-compatible software
// (Paper, Folia, Waterfall, Velocity, etc.).
type MinecraftTracker struct {
	httpClient *http.Client
	paperMCURL string
}

// NewMinecraftTracker creates a new MinecraftTracker using the live PaperMC API.
func NewMinecraftTracker() *MinecraftTracker {
	return &MinecraftTracker{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		paperMCURL: "https://api.papermc.io/v2",
	}
}

// newMinecraftTrackerWithURL creates a MinecraftTracker that queries a custom base URL.
// This is intended for use in tests only.
func newMinecraftTrackerWithURL(baseURL string) *MinecraftTracker {
	return &MinecraftTracker{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		paperMCURL: baseURL,
	}
}

// jarSourceToProject maps a jar_source value to a PaperMC project name.
func jarSourceToProject(jarSource string) string {
	switch jarSource {
	case "papermc", "paper":
		return "paper"
	case "folia":
		return "folia"
	case "waterfall":
		return "waterfall"
	case "velocity":
		return "velocity"
	default:
		return "paper"
	}
}

// paperMCProject determines the PaperMC project name from a game server's ServerSoftware field.
// Defaults to "paper" if the field is absent, empty, or unparseable.
func paperMCProject(gs *models.GameServer) string {
	raw := gs.ServerSoftware.GetOr("")
	if raw == "" {
		return "paper"
	}
	var entries []serverSoftwareEntry
	errUnmarshal := json.Unmarshal([]byte(raw), &entries)
	if errUnmarshal != nil || len(entries) == 0 {
		return "paper"
	}
	return jarSourceToProject(entries[0].JarSource)
}

// GetInstalledVersion returns the Minecraft version stored in the database for this server.
// Returns an empty string (and nil error) if no version has been recorded.
func (m *MinecraftTracker) GetInstalledVersion(_ context.Context, gs *models.GameServer) (string, error) {
	return gs.Version, nil
}

// GetLatestVersion queries the PaperMC API and returns the most recent version available
// for the server's software project (derived from the ServerSoftware JSON field).
func (m *MinecraftTracker) GetLatestVersion(ctx context.Context, gs *models.GameServer) (string, error) {
	project := paperMCProject(gs)
	url := fmt.Sprintf("%s/projects/%s", m.paperMCURL, project)

	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if errReq != nil {
		return "", fmt.Errorf("build papermc request: %w", errReq)
	}

	resp, errDo := m.httpClient.Do(req)
	if errDo != nil {
		return "", fmt.Errorf("query papermc api: %w", errDo)
	}
	defer func() {
		errClose := resp.Body.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("failed to close PaperMC response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("papermc api returned status %d", resp.StatusCode)
	}

	body, errRead := io.ReadAll(io.LimitReader(resp.Body, maxPaperMCResponseBytes+1))
	if errRead != nil {
		return "", fmt.Errorf("read papermc response: %w", errRead)
	}
	if len(body) > maxPaperMCResponseBytes {
		return "", fmt.Errorf("papermc response exceeded %d bytes", maxPaperMCResponseBytes)
	}

	var parsed paperMCProjectsResponse
	errJSON := json.Unmarshal(body, &parsed)
	if errJSON != nil {
		return "", fmt.Errorf("parse papermc response: %w", errJSON)
	}

	if len(parsed.Versions) == 0 {
		return "", nil
	}
	return parsed.Versions[len(parsed.Versions)-1], nil
}

// CheckForUpdate compares the installed version against the latest available version.
// Returns nil if either version cannot be determined, or if the server is already up to date.
func (m *MinecraftTracker) CheckForUpdate(ctx context.Context, gs *models.GameServer) (*UpdateInfo, error) {
	installed, errInstalled := m.GetInstalledVersion(ctx, gs)
	if errInstalled != nil {
		return nil, errInstalled
	}
	latest, errLatest := m.GetLatestVersion(ctx, gs)
	if errLatest != nil {
		return nil, errLatest
	}
	installed = normalizeVersion(installed)
	latest = normalizeVersion(latest)
	if installed == "" || latest == "" {
		return nil, nil
	}
	if versionsEqual(installed, latest) {
		return nil, nil
	}
	return &UpdateInfo{
		InstalledVersion: installed,
		LatestVersion:    latest,
		UpdateAvailable:  true,
	}, nil
}
