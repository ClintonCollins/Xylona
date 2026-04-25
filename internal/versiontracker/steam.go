package versiontracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/steamcache"
	"github.com/ClintonCollins/Xylona/sql/models"
)

var (
	reBuildID = regexp.MustCompile(`"buildid"\s+"(\d+)"`)
	reAppID   = regexp.MustCompile(`appmanifest_(\d+)\.acf$`)
)

// steamUpToDateResponse is the JSON structure returned by the Steam UpToDateCheck API.
type steamUpToDateResponse struct {
	Response struct {
		AppID             int64  `json:"appid"`
		Success           bool   `json:"success"`
		UpToDate          bool   `json:"up_to_date"`
		VersionIsListable bool   `json:"version_is_listable"`
		RequiredVersion   int64  `json:"required_version"`
		Message           string `json:"message"`
	} `json:"response"`
}

// SteamTracker is a VersionTracker for games updated via SteamCMD.
// It reads appmanifest_*.acf files to determine the installed buildid and
// queries the Steam UpToDateCheck API for the latest version.
type SteamTracker struct {
	httpClient     *http.Client
	steamAPIURL    string
	preferredAppID string
	steamCache     *steamcache.Client
}

// NewSteamTracker creates a new SteamTracker using the real Steam API.
func NewSteamTracker() *SteamTracker {
	return NewSteamTrackerWithAppID("")
}

// NewSteamTrackerWithAppID creates a new SteamTracker with an optional
// preferred Steam app ID to disambiguate manifests.
func NewSteamTrackerWithAppID(appID string) *SteamTracker {
	return &SteamTracker{
		httpClient:     &http.Client{Timeout: 15 * time.Second},
		steamAPIURL:    "https://api.steampowered.com",
		preferredAppID: strings.TrimSpace(appID),
		steamCache:     steamcache.New(),
	}
}

func findFirstExistingPath(paths []string) (string, error) {
	for _, candidate := range paths {
		info, errStat := os.Stat(candidate)
		if errStat == nil && !info.IsDir() {
			return candidate, nil
		}
		if errStat != nil && !os.IsNotExist(errStat) {
			return "", fmt.Errorf("stat manifest %s: %w", candidate, errStat)
		}
	}
	return "", nil
}

func globFirstManifest(patterns []string) (string, error) {
	for _, pattern := range patterns {
		matches, errGlob := filepath.Glob(pattern)
		if errGlob != nil {
			return "", fmt.Errorf("glob appmanifest: %w", errGlob)
		}
		if len(matches) > 0 {
			return matches[0], nil
		}
	}
	return "", nil
}

// findAppManifest returns the most relevant appmanifest file for the server
// directory, preferring a specific app ID when configured.
func findAppManifest(dir string, preferredAppID string) (string, error) {
	if preferredAppID != "" {
		manifestPath, errFind := findFirstExistingPath([]string{
			filepath.Join(dir, fmt.Sprintf("appmanifest_%s.acf", preferredAppID)),
			filepath.Join(dir, "steamapps", fmt.Sprintf("appmanifest_%s.acf", preferredAppID)),
		})
		if errFind != nil {
			return "", errFind
		}
		if manifestPath != "" {
			return manifestPath, nil
		}
	}

	return globFirstManifest([]string{
		filepath.Join(dir, "appmanifest_*.acf"),
		filepath.Join(dir, "steamapps", "appmanifest_*.acf"),
	})
}

// readBuildID reads an ACF file and extracts the buildid value.
// Returns "" if the file has no buildid field.
func readBuildID(acfPath string) (string, error) {
	data, errRead := os.ReadFile(acfPath)
	if errRead != nil {
		return "", fmt.Errorf("read ACF file: %w", errRead)
	}
	return ReadSteamBuildID(data), nil
}

// ReadSteamBuildID extracts the buildid field from an appmanifest ACF payload.
func ReadSteamBuildID(data []byte) string {
	sub := reBuildID.FindSubmatch(data)
	if sub == nil {
		return ""
	}
	return strings.TrimSpace(string(sub[1]))
}

// extractAppID parses the Steam app ID from an ACF filename.
// e.g. appmanifest_730.acf → "730". Returns "" if the name does not match.
func extractAppID(acfPath string) string {
	sub := reAppID.FindStringSubmatch(filepath.Base(acfPath))
	if sub == nil {
		return ""
	}
	return sub[1]
}

// ExtractSteamAppIDFromManifestName parses a Steam app ID from an appmanifest filename.
func ExtractSteamAppIDFromManifestName(name string) string {
	return extractAppID(name)
}

// GetInstalledVersion returns the buildid from the appmanifest ACF file in
// gs.Directory. Returns "" (no error) when no manifest is present.
func (s *SteamTracker) GetInstalledVersion(_ context.Context, gs *models.GameServer) (string, error) {
	acfPath, errFind := findAppManifest(gs.Directory, s.preferredAppID)
	if errFind != nil {
		return "", errFind
	}
	if acfPath == "" {
		return "", nil
	}
	buildID, errRead := readBuildID(acfPath)
	if errRead != nil {
		return "", errRead
	}
	return buildID, nil
}

// GetLatestVersion queries the Steam UpToDateCheck API and returns the
// required_version as a string. Returns "" (no error) when the app ID cannot
// be determined (no manifest present) or when the API does not report a
// required version.
func (s *SteamTracker) GetLatestVersion(ctx context.Context, gs *models.GameServer) (string, error) {
	appID, errAppID := s.resolveAppID(gs)
	if errAppID != nil {
		return "", errAppID
	}
	if appID == "" {
		return "", nil
	}

	releases, errReleases := s.fetchReleases(ctx, appID)
	if errReleases == nil {
		release := preferredSteamRelease(releases, NormalizeSteamBranch(gs.Branch))
		if release != nil && strings.TrimSpace(release.BuildID) != "" {
			return strings.TrimSpace(release.BuildID), nil
		}
	}

	url := fmt.Sprintf("%s/ISteamApps/UpToDateCheck/v1/?appid=%s&version=0", s.steamAPIURL, appID)
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if errReq != nil {
		return "", fmt.Errorf("build Steam API request: %w", errReq)
	}

	resp, errDo := s.httpClient.Do(req)
	if errDo != nil {
		return "", fmt.Errorf("steam API request: %w", errDo)
	}
	defer func() {
		errClose := resp.Body.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("failed to close Steam API response body")
		}
	}()

	var parsed steamUpToDateResponse
	errDecode := json.NewDecoder(resp.Body).Decode(&parsed)
	if errDecode != nil {
		return "", fmt.Errorf("decode Steam API response: %w", errDecode)
	}

	if parsed.Response.RequiredVersion == 0 {
		return "", nil
	}
	return fmt.Sprintf("%d", parsed.Response.RequiredVersion), nil
}

// CheckForUpdate compares the installed buildid against the Steam latest version.
func (s *SteamTracker) CheckForUpdate(ctx context.Context, gs *models.GameServer) (*UpdateInfo, error) {
	installed, errInstalled := s.GetInstalledVersion(ctx, gs)
	if errInstalled != nil {
		return nil, errInstalled
	}
	latest, errLatest := s.GetLatestVersion(ctx, gs)
	if errLatest != nil {
		return nil, errLatest
	}
	installed = normalizeVersion(installed)
	latest = normalizeVersion(latest)

	updateInfo := &UpdateInfo{
		InstalledVersion:      installed,
		LatestVersion:         latest,
		UpdateAvailable:       installed != "" && latest != "" && !versionsEqual(installed, latest),
		InstalledVersionLabel: installed,
		LatestVersionLabel:    latest,
	}

	s.populateSteamLabels(ctx, gs, &VersionState{
		InstalledVersion: installed,
		LatestVersion:    latest,
	}, updateInfo)

	return updateInfo, nil
}

// EnrichVersionState fills user-facing labels and branch metadata onto a version state.
func EnrichVersionState(ctx context.Context, tracker VersionTracker, gs *models.GameServer, state *VersionState) {
	if state == nil {
		return
	}

	if state.InstalledVersionLabel == "" {
		state.InstalledVersionLabel = state.InstalledVersion
	}
	if state.LatestVersionLabel == "" {
		state.LatestVersionLabel = state.LatestVersion
	}

	switch typedTracker := tracker.(type) {
	case *SteamTracker:
		typedTracker.populateSteamLabels(ctx, gs, state, nil)
	case *MinecraftTracker:
		providerKind := typedTracker.resolvedProviderKind(gs)
		target := typedTracker.resolvedTarget(gs)
		state.InstalledVersionLabel = displayMinecraftVersion(providerKind, target, state.InstalledVersion)
		state.LatestVersionLabel = displayMinecraftVersion(providerKind, target, state.LatestVersion)
	}
}

func (s *SteamTracker) populateSteamLabels(ctx context.Context, gs *models.GameServer, state *VersionState, info *UpdateInfo) {
	appID, errAppID := s.resolveAppID(gs)
	if errAppID != nil || appID == "" {
		return
	}

	releases, errReleases := s.fetchReleases(ctx, appID)
	if errReleases != nil || len(releases) == 0 {
		return
	}

	selectedBranch := NormalizeSteamBranch(gs.Branch)

	installedRelease := matchInstalledSteamRelease(releases, normalizeVersion(state.InstalledVersion), selectedBranch)
	latestRelease := preferredSteamRelease(releases, selectedBranch)

	if installedRelease != nil {
		state.InstalledBranch = installedRelease.Name
		state.InstalledVersionLabel = steamReleaseDisplay(*installedRelease)
		if info != nil {
			info.InstalledBranch = installedRelease.Name
			info.InstalledVersionLabel = steamReleaseDisplay(*installedRelease)
		}
	}

	if latestRelease != nil {
		state.LatestBranch = latestRelease.Name
		state.LatestVersionLabel = steamReleaseDisplay(*latestRelease)
		if info != nil {
			info.LatestBranch = latestRelease.Name
			info.LatestVersionLabel = steamReleaseDisplay(*latestRelease)
		}
	}
}

func (s *SteamTracker) resolveAppID(gs *models.GameServer) (string, error) {
	if strings.TrimSpace(s.preferredAppID) != "" {
		return strings.TrimSpace(s.preferredAppID), nil
	}

	acfPath, errFind := findAppManifest(gs.Directory, s.preferredAppID)
	if errFind != nil {
		return "", errFind
	}
	if acfPath == "" {
		return "", nil
	}
	return extractAppID(acfPath), nil
}

func (s *SteamTracker) fetchReleases(ctx context.Context, appID string) ([]steamcache.SteamRelease, error) {
	if s.steamCache == nil {
		return nil, errors.New("steam release metadata unavailable")
	}

	releases, errFetch := s.steamCache.FetchReleases(ctx, appID)
	if errFetch != nil {
		return nil, fmt.Errorf("fetch steam releases: %w", errFetch)
	}

	return releases, nil
}

func preferredSteamRelease(releases []steamcache.SteamRelease, branch string) *steamcache.SteamRelease {
	for index := range releases {
		if releases[index].Name == branch {
			return &releases[index]
		}
	}
	return nil
}

func matchInstalledSteamRelease(
	releases []steamcache.SteamRelease,
	buildID string,
	selectedBranch string,
) *steamcache.SteamRelease {
	if buildID == "" {
		return nil
	}

	selected := preferredSteamRelease(releases, selectedBranch)
	if selected != nil && selected.BuildID == buildID {
		return selected
	}

	for _, candidateName := range []string{"public", selectedBranch} {
		for index := range releases {
			if releases[index].Name == candidateName && releases[index].BuildID == buildID {
				return &releases[index]
			}
		}
	}

	for index := range releases {
		if releases[index].BuildID == buildID {
			return &releases[index]
		}
	}

	return nil
}

func steamReleaseDisplay(release steamcache.SteamRelease) string {
	label := strings.TrimSpace(release.DisplayLabel)
	if label == "" {
		label = strings.TrimSpace(release.Name)
	}
	buildID := strings.TrimSpace(release.BuildID)
	if buildID == "" {
		return label
	}
	if label == "" {
		return buildID
	}
	return fmt.Sprintf("%s (%s)", label, buildID)
}
