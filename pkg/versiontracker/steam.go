package versiontracker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

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
	sub := reBuildID.FindSubmatch(data)
	if sub == nil {
		return "", nil
	}
	return strings.TrimSpace(string(sub[1])), nil
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
	acfPath, errFind := findAppManifest(gs.Directory, s.preferredAppID)
	if errFind != nil {
		return "", errFind
	}
	if acfPath == "" {
		return "", nil
	}
	appID := extractAppID(acfPath)
	if appID == "" {
		return "", nil
	}

	url := fmt.Sprintf("%s/ISteamApps/UpToDateCheck/v1/?appid=%s&version=0", s.steamAPIURL, appID)
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if errReq != nil {
		return "", fmt.Errorf("build Steam API request: %w", errReq)
	}

	resp, errDo := s.httpClient.Do(req)
	if errDo != nil {
		return "", fmt.Errorf("Steam API request: %w", errDo)
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
// Returns nil when versions match or either version cannot be determined.
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
	if installed == "" || latest == "" || versionsEqual(installed, latest) {
		return nil, nil
	}
	return &UpdateInfo{
		InstalledVersion: installed,
		LatestVersion:    latest,
		UpdateAvailable:  true,
	}, nil
}
