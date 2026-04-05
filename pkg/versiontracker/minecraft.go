package versiontracker

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// minecraftVersionJSON represents the version.json file embedded in a Minecraft server jar.
type minecraftVersionJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// maxVersionAPIResponseBytes is the maximum size (in bytes) allowed for reading
// version provider API responses (10 MiB).
const maxVersionAPIResponseBytes = 10 << 20

var rePaperBuildVersion = regexp.MustCompile(`(?i)(\d+\.\d+(?:\.\d+)?)-(\d+)`)

// paperMCProjectsResponse is the response from the PaperMC /v2/projects/{project} endpoint.
type paperMCProjectsResponse struct {
	ProjectID   string   `json:"project_id"`
	ProjectName string   `json:"project_name"`
	Versions    []string `json:"versions"`
}

type mojangManifestResponse struct {
	Latest struct {
		Release string `json:"release"`
	} `json:"latest"`
	Versions []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"versions"`
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
	httpClient        *http.Client
	paperMCURL        string
	mojangManifestURL string
	providerKind      string
	providerSourceID  string
	target            string
}

// NewMinecraftTracker creates a new MinecraftTracker using the live PaperMC API.
func NewMinecraftTracker() *MinecraftTracker {
	return NewConfiguredMinecraftTracker("", "", "")
}

// NewConfiguredMinecraftTracker creates a MinecraftTracker bound to a resolved
// typed provider and selected target.
func NewConfiguredMinecraftTracker(providerKind string, providerSourceID string, target string) *MinecraftTracker {
	return &MinecraftTracker{
		httpClient:        &http.Client{Timeout: 15 * time.Second},
		paperMCURL:        "https://api.papermc.io/v2",
		mojangManifestURL: "https://launchermeta.mojang.com/mc/game/version_manifest.json",
		providerKind:      strings.ToLower(strings.TrimSpace(providerKind)),
		providerSourceID:  strings.ToLower(strings.TrimSpace(providerSourceID)),
		target:            strings.TrimSpace(target),
	}
}

// newMinecraftTrackerWithURL creates a MinecraftTracker that queries a custom base URL.
// This is intended for use in tests only.
func newMinecraftTrackerWithURL(baseURL string) *MinecraftTracker {
	tracker := NewMinecraftTracker()
	tracker.httpClient = &http.Client{Timeout: 5 * time.Second}
	tracker.paperMCURL = baseURL
	return tracker
}

func selectedMinecraftSoftware(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "[") {
		var entries []serverSoftwareEntry
		errUnmarshal := json.Unmarshal([]byte(trimmed), &entries)
		if errUnmarshal == nil && len(entries) > 0 {
			if entries[0].ID != "" {
				return strings.ToLower(entries[0].ID)
			}
			if entries[0].JarSource != "" {
				return strings.ToLower(entries[0].JarSource)
			}
		}
	}
	return strings.ToLower(trimmed)
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
	return jarSourceToProject(selectedMinecraftSoftware(gs.ServerSoftware.GetOr("")))
}

func (m *MinecraftTracker) resolvedProviderKind(gs *models.GameServer) string {
	if strings.TrimSpace(m.providerKind) != "" {
		return strings.TrimSpace(m.providerKind)
	}
	if strings.TrimSpace(gs.ServerSoftware.GetOr("")) == "" {
		return ""
	}
	if selectedMinecraftSoftware(gs.ServerSoftware.GetOr("")) == "vanilla" {
		return "mojang"
	}
	return "papermc"
}

func (m *MinecraftTracker) resolvedPaperProject(gs *models.GameServer) string {
	if strings.TrimSpace(m.providerSourceID) != "" {
		return strings.TrimSpace(m.providerSourceID)
	}
	return paperMCProject(gs)
}

func (m *MinecraftTracker) resolvedTarget(gs *models.GameServer) string {
	_ = gs
	return strings.TrimSpace(m.target)
}

func parsePaperExecutableVersion(executable string) string {
	match := rePaperBuildVersion.FindStringSubmatch(filepath.Base(strings.TrimSpace(executable)))
	if len(match) != 3 {
		return ""
	}
	return fmt.Sprintf("%s-%s", strings.TrimSpace(match[1]), strings.TrimSpace(match[2]))
}

func displayMinecraftVersion(providerKind string, target string, version string) string {
	normalizedTarget := strings.TrimSpace(target)
	normalizedVersion := strings.TrimSpace(version)
	if strings.EqualFold(strings.TrimSpace(providerKind), "papermc") {
		if normalizedTarget == "" {
			normalizedTarget = normalizedVersion
		}
		match := rePaperBuildVersion.FindStringSubmatch(normalizedVersion)
		if len(match) == 3 {
			return fmt.Sprintf("%s (Build %s)", match[1], match[2])
		}
		if normalizedTarget != "" {
			return normalizedTarget
		}
	}
	return normalizedVersion
}

// GetInstalledVersion extracts the Minecraft version from the active server jar by reading
// version.json inside the jar. Falls back to the database Version field if the jar cannot be read.
func (m *MinecraftTracker) GetInstalledVersion(_ context.Context, gs *models.GameServer) (string, error) {
	if strings.EqualFold(m.resolvedProviderKind(gs), "papermc") {
		parsed := parsePaperExecutableVersion(gs.ServerExecutable.GetOr(""))
		if parsed != "" {
			return parsed, nil
		}
	}

	version, errJar := ReadMinecraftJarVersion(gs.Directory, gs.ServerExecutable.GetOr(""))
	if errJar != nil {
		return gs.Version, errJar
	}
	return version, nil
}

// ReadMinecraftJarVersion opens the active server jar in the given directory and extracts
// the version name from the embedded version.json file. It prefers the configured executable
// and falls back to minecraft_server.jar for legacy vanilla servers.
func ReadMinecraftJarVersion(dir string, executable string) (string, error) {
	candidates := []string{}
	if strings.TrimSpace(executable) != "" {
		candidates = append(candidates, executable)
	}
	if strings.TrimSpace(executable) != "minecraft_server.jar" {
		candidates = append(candidates, "minecraft_server.jar")
	}

	var lastErr error
	for _, candidate := range candidates {
		version, errRead := readMinecraftJarVersion(filepath.Join(dir, candidate))
		if errRead == nil {
			return version, nil
		}
		lastErr = errRead
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no minecraft jar candidates found")
	}
	return "", lastErr
}

func readMinecraftJarVersion(jarPath string) (string, error) {
	zr, errOpen := zip.OpenReader(jarPath)
	if errOpen != nil {
		return "", fmt.Errorf("open jar: %w", errOpen)
	}
	defer func() {
		errClose := zr.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("failed to close minecraft jar")
		}
	}()

	for _, f := range zr.File {
		if f.Name != "version.json" {
			continue
		}
		rc, errFile := f.Open()
		if errFile != nil {
			return "", fmt.Errorf("open version.json: %w", errFile)
		}
		defer func() {
			errClose := rc.Close()
			if errClose != nil {
				log.Warn().Err(errClose).Msg("failed to close version.json reader")
			}
		}()

		var ver minecraftVersionJSON
		errDecode := json.NewDecoder(rc).Decode(&ver)
		if errDecode != nil {
			return "", fmt.Errorf("decode version.json: %w", errDecode)
		}
		return ver.Name, nil
	}
	return "", fmt.Errorf("version.json not found in jar")
}

// GetLatestVersion queries the PaperMC API and returns the most recent version available
// for the server's software project (derived from the ServerSoftware JSON field).
func (m *MinecraftTracker) GetLatestVersion(ctx context.Context, gs *models.GameServer) (string, error) {
	switch m.resolvedProviderKind(gs) {
	case "mojang":
		return m.resolveMojangReleaseTarget(ctx, m.resolvedTarget(gs))
	case "papermc":
		project := m.resolvedPaperProject(gs)
		target, errTarget := m.resolvePaperTarget(ctx, project, m.resolvedTarget(gs))
		if errTarget != nil {
			return "", errTarget
		}
		if target == "" {
			return "", nil
		}
		buildVersion, errBuild := m.getLatestPaperBuildVersion(ctx, project, target)
		if errBuild == nil && buildVersion != "" {
			return buildVersion, nil
		}
		return target, nil
	}

	if selectedMinecraftSoftware(gs.ServerSoftware.GetOr("")) == "vanilla" {
		return m.getLatestVanillaVersion(ctx)
	}

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

	body, errRead := io.ReadAll(io.LimitReader(resp.Body, maxVersionAPIResponseBytes+1))
	if errRead != nil {
		return "", fmt.Errorf("read papermc response: %w", errRead)
	}
	if len(body) > maxVersionAPIResponseBytes {
		return "", fmt.Errorf("papermc response exceeded %d bytes", maxVersionAPIResponseBytes)
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

func (m *MinecraftTracker) getLatestVanillaVersion(ctx context.Context) (string, error) {
	return m.resolveMojangReleaseTarget(ctx, "")
}

func (m *MinecraftTracker) resolveMojangReleaseTarget(ctx context.Context, preferred string) (string, error) {
	manifest, errManifest := m.getMojangManifest(ctx)
	if errManifest != nil {
		return "", errManifest
	}

	normalizedPreferred := strings.TrimSpace(preferred)
	if normalizedPreferred != "" {
		for _, version := range manifest.Versions {
			if version.Type == "release" && strings.TrimSpace(version.ID) == normalizedPreferred {
				return normalizedPreferred, nil
			}
		}
	}

	return strings.TrimSpace(manifest.Latest.Release), nil
}

func (m *MinecraftTracker) getMojangManifest(ctx context.Context) (mojangManifestResponse, error) {
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, m.mojangManifestURL, nil)
	if errReq != nil {
		return mojangManifestResponse{}, fmt.Errorf("build mojang request: %w", errReq)
	}

	resp, errDo := m.httpClient.Do(req)
	if errDo != nil {
		return mojangManifestResponse{}, fmt.Errorf("query mojang manifest: %w", errDo)
	}
	defer func() {
		errClose := resp.Body.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("failed to close Mojang manifest response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return mojangManifestResponse{}, fmt.Errorf("mojang manifest returned status %d", resp.StatusCode)
	}

	body, errRead := io.ReadAll(io.LimitReader(resp.Body, maxVersionAPIResponseBytes+1))
	if errRead != nil {
		return mojangManifestResponse{}, fmt.Errorf("read mojang manifest response: %w", errRead)
	}
	if len(body) > maxVersionAPIResponseBytes {
		return mojangManifestResponse{}, fmt.Errorf("mojang manifest response exceeded %d bytes", maxVersionAPIResponseBytes)
	}

	var parsed mojangManifestResponse
	errJSON := json.Unmarshal(body, &parsed)
	if errJSON != nil {
		return mojangManifestResponse{}, fmt.Errorf("parse mojang manifest response: %w", errJSON)
	}
	return parsed, nil
}

func (m *MinecraftTracker) resolvePaperTarget(ctx context.Context, project string, preferred string) (string, error) {
	versions, errVersions := m.getPaperTargets(ctx, project)
	if errVersions != nil {
		return "", errVersions
	}
	if len(versions) == 0 {
		return "", nil
	}

	normalizedPreferred := strings.TrimSpace(preferred)
	if normalizedPreferred != "" {
		for _, version := range versions {
			if strings.TrimSpace(version) == normalizedPreferred {
				return normalizedPreferred, nil
			}
		}
	}

	return versions[len(versions)-1], nil
}

func (m *MinecraftTracker) getPaperTargets(ctx context.Context, project string) ([]string, error) {
	url := fmt.Sprintf("%s/projects/%s", m.paperMCURL, project)

	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if errReq != nil {
		return nil, fmt.Errorf("build papermc request: %w", errReq)
	}

	resp, errDo := m.httpClient.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("query papermc api: %w", errDo)
	}
	defer func() {
		errClose := resp.Body.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("failed to close PaperMC response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("papermc api returned status %d", resp.StatusCode)
	}

	body, errRead := io.ReadAll(io.LimitReader(resp.Body, maxVersionAPIResponseBytes+1))
	if errRead != nil {
		return nil, fmt.Errorf("read papermc response: %w", errRead)
	}
	if len(body) > maxVersionAPIResponseBytes {
		return nil, fmt.Errorf("papermc response exceeded %d bytes", maxVersionAPIResponseBytes)
	}

	var parsed paperMCProjectsResponse
	errJSON := json.Unmarshal(body, &parsed)
	if errJSON != nil {
		return nil, fmt.Errorf("parse papermc response: %w", errJSON)
	}

	return parsed.Versions, nil
}

type paperMCBuildsResponse struct {
	Builds []struct {
		Build int `json:"build"`
	} `json:"builds"`
}

func (m *MinecraftTracker) getLatestPaperBuildVersion(ctx context.Context, project string, target string) (string, error) {
	url := fmt.Sprintf("%s/projects/%s/versions/%s/builds", m.paperMCURL, project, target)

	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if errReq != nil {
		return "", fmt.Errorf("build papermc build request: %w", errReq)
	}

	resp, errDo := m.httpClient.Do(req)
	if errDo != nil {
		return "", fmt.Errorf("query papermc builds api: %w", errDo)
	}
	defer func() {
		errClose := resp.Body.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("failed to close PaperMC builds response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("papermc builds api returned status %d", resp.StatusCode)
	}

	body, errRead := io.ReadAll(io.LimitReader(resp.Body, maxVersionAPIResponseBytes+1))
	if errRead != nil {
		return "", fmt.Errorf("read papermc builds response: %w", errRead)
	}
	if len(body) > maxVersionAPIResponseBytes {
		return "", fmt.Errorf("papermc builds response exceeded %d bytes", maxVersionAPIResponseBytes)
	}

	var parsed paperMCBuildsResponse
	errJSON := json.Unmarshal(body, &parsed)
	if errJSON != nil {
		return "", fmt.Errorf("parse papermc builds response: %w", errJSON)
	}
	if len(parsed.Builds) == 0 {
		return "", nil
	}
	build := parsed.Builds[len(parsed.Builds)-1].Build
	return fmt.Sprintf("%s-%d", target, build), nil
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
		InstalledVersion:      installed,
		LatestVersion:         latest,
		UpdateAvailable:       true,
		InstalledVersionLabel: displayMinecraftVersion(m.resolvedProviderKind(gs), m.resolvedTarget(gs), installed),
		LatestVersionLabel:    displayMinecraftVersion(m.resolvedProviderKind(gs), m.resolvedTarget(gs), latest),
	}, nil
}
