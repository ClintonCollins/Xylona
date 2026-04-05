// Package steamcache provides Steam app detail lookups via the api.steamcmd.net API.
package steamcache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const defaultDetailsURLFmt = "https://api.steamcmd.net/v1/info/%s"

const (
	defaultReleaseFreshTTL = 5 * time.Minute
	defaultReleaseStaleTTL = 24 * time.Hour
)

// SteamApp represents a Steam application with its ID and name.
type SteamApp struct {
	AppID string
	Name  string
}

// LaunchConfig represents a launch configuration for a Steam app.
type LaunchConfig struct {
	Executable  string
	Arguments   string
	OS          string
	Description string
}

// SteamAppDetails contains detailed information about a Steam application.
type SteamAppDetails struct {
	AppID            string
	Name             string
	Type             string // e.g., "Game", "Tool"
	WindowsSupport   bool
	LinuxSupport     bool
	InstallDirectory string
	ParentAppID      string
	LaunchConfigs    []LaunchConfig
}

// SteamRelease contains a Steam branch/build entry normalized for display and
// update selection.
type SteamRelease struct {
	Name             string
	DisplayLabel     string
	BuildID          string
	Description      string
	TimeUpdated      string
	DepotManifestIDs map[string]string
}

// ClientOptions customizes Steam app and release lookups.
type ClientOptions struct {
	HTTPClient       *http.Client
	DetailsURLFormat string
	ReleaseFreshTTL  time.Duration
	ReleaseStaleTTL  time.Duration
	LocalReleaseFunc func(ctx context.Context, appID string) ([]SteamRelease, error)
}

type cachedReleaseResult struct {
	fetchedAt time.Time
	releases  []SteamRelease
}

// Client provides Steam app lookups via the api.steamcmd.net API.
type Client struct {
	detailsURLFmt    string
	httpClient       *http.Client
	releaseFreshTTL  time.Duration
	releaseStaleTTL  time.Duration
	localReleaseFunc func(ctx context.Context, appID string) ([]SteamRelease, error)
	releaseCacheMu   sync.Mutex
	releaseCache     map[string]cachedReleaseResult
}

// New creates a new Client.
func New() *Client {
	return NewWithOptions(ClientOptions{})
}

// NewWithOptions creates a client with the provided transport and cache settings.
func NewWithOptions(options ClientOptions) *Client {
	detailsURLFmt := options.DetailsURLFormat
	if detailsURLFmt == "" {
		detailsURLFmt = defaultDetailsURLFmt
	}

	releaseFreshTTL := options.ReleaseFreshTTL
	if releaseFreshTTL <= 0 {
		releaseFreshTTL = defaultReleaseFreshTTL
	}

	releaseStaleTTL := options.ReleaseStaleTTL
	if releaseStaleTTL <= 0 {
		releaseStaleTTL = defaultReleaseStaleTTL
	}

	return &Client{
		detailsURLFmt:    detailsURLFmt,
		httpClient:       options.HTTPClient,
		releaseFreshTTL:  releaseFreshTTL,
		releaseStaleTTL:  releaseStaleTTL,
		localReleaseFunc: options.LocalReleaseFunc,
		releaseCache:     make(map[string]cachedReleaseResult),
	}
}

func (c *Client) client() *http.Client {
	if c.httpClient != nil {
		return c.httpClient
	}
	return http.DefaultClient
}

// FetchDetails retrieves detailed information about a Steam app from the
// steamcmd.net API.
func (c *Client) FetchDetails(ctx context.Context, appID string) (*SteamAppDetails, error) {
	detailsURL := fmt.Sprintf(c.detailsURLFmt, appID)
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, detailsURL, nil)
	if errReq != nil {
		return nil, fmt.Errorf("creating request for app %s: %w", appID, errReq)
	}

	resp, errDo := c.client().Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("fetching details for app %s: %w", appID, errDo)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for app %s", resp.StatusCode, appID)
	}

	var raw steamCmdResponse
	errDecode := json.NewDecoder(resp.Body).Decode(&raw)
	if errDecode != nil {
		return nil, fmt.Errorf("decoding details for app %s: %w", appID, errDecode)
	}

	appData, ok := raw.Data[appID]
	if !ok {
		return nil, fmt.Errorf("no data found for app %s in response", appID)
	}

	osList := strings.ToLower(appData.Common.OSList)
	details := &SteamAppDetails{
		AppID:            appID,
		Name:             appData.Common.Name,
		Type:             appData.Common.Type,
		WindowsSupport:   strings.Contains(osList, "windows"),
		LinuxSupport:     strings.Contains(osList, "linux"),
		InstallDirectory: appData.Config.InstallDir,
		ParentAppID:      appData.Common.Parent,
	}

	for _, lc := range appData.Config.Launch {
		osName := ""
		if lc.Config.OSList != "" {
			osName = lc.Config.OSList
		}
		details.LaunchConfigs = append(details.LaunchConfigs, LaunchConfig{
			Executable:  lc.Executable,
			Arguments:   lc.Arguments,
			OS:          osName,
			Description: lc.Description,
		})
	}

	log.Debug().
		Str("app_id", appID).
		Str("name", details.Name).
		Str("type", details.Type).
		Msg("steamcache: fetched app details")

	return details, nil
}

// FetchReleases returns normalized Steam branch metadata for an app.
func (c *Client) FetchReleases(ctx context.Context, appID string) ([]SteamRelease, error) {
	now := time.Now()
	if cached, ok := c.getCachedReleases(appID); ok {
		if now.Sub(cached.fetchedAt) <= c.releaseFreshTTL {
			return cloneReleases(cached.releases), nil
		}
	}

	releases, errFetch := c.fetchReleasesFromAPI(ctx, appID)
	if errFetch == nil {
		c.storeCachedReleases(appID, releases, now)
		return cloneReleases(releases), nil
	}

	localReleaseFunc := c.localReleaseFunc
	if localReleaseFunc == nil {
		localReleaseFunc = fetchReleasesFromLocalSteamCMD
	}
	localReleases, errLocal := localReleaseFunc(ctx, appID)
	if errLocal == nil && len(localReleases) > 0 {
		c.storeCachedReleases(appID, localReleases, now)
		return cloneReleases(localReleases), nil
	}

	if cached, ok := c.getCachedReleases(appID); ok {
		if now.Sub(cached.fetchedAt) <= c.releaseStaleTTL {
			log.Warn().
				Err(errFetch).
				Str("app_id", appID).
				Msg("steamcache: serving stale release metadata after fetch failure")
			return cloneReleases(cached.releases), nil
		}
	}

	return nil, errFetch
}

func (c *Client) getCachedReleases(appID string) (cachedReleaseResult, bool) {
	c.releaseCacheMu.Lock()
	defer c.releaseCacheMu.Unlock()

	result, ok := c.releaseCache[appID]
	return result, ok
}

func (c *Client) storeCachedReleases(appID string, releases []SteamRelease, fetchedAt time.Time) {
	c.releaseCacheMu.Lock()
	defer c.releaseCacheMu.Unlock()

	if c.releaseCache == nil {
		c.releaseCache = make(map[string]cachedReleaseResult)
	}

	c.releaseCache[appID] = cachedReleaseResult{
		fetchedAt: fetchedAt,
		releases:  cloneReleases(releases),
	}
}

func (c *Client) fetchReleasesFromAPI(ctx context.Context, appID string) ([]SteamRelease, error) {
	detailsURL := fmt.Sprintf(c.detailsURLFmt, appID)
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, detailsURL, nil)
	if errReq != nil {
		return nil, fmt.Errorf("creating release request for app %s: %w", appID, errReq)
	}

	resp, errDo := c.client().Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("fetching release metadata for app %s: %w", appID, errDo)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for app %s", resp.StatusCode, appID)
	}

	var raw steamCmdResponse
	errDecode := json.NewDecoder(resp.Body).Decode(&raw)
	if errDecode != nil {
		return nil, fmt.Errorf("decoding release metadata for app %s: %w", appID, errDecode)
	}

	appData, ok := raw.Data[appID]
	if !ok {
		return nil, fmt.Errorf("no data found for app %s in response", appID)
	}

	return parseSteamReleases(appData)
}

func cloneReleases(releases []SteamRelease) []SteamRelease {
	cloned := make([]SteamRelease, len(releases))
	for i, release := range releases {
		cloned[i] = release
		if release.DepotManifestIDs != nil {
			cloned[i].DepotManifestIDs = make(map[string]string, len(release.DepotManifestIDs))
			maps.Copy(cloned[i].DepotManifestIDs, release.DepotManifestIDs)
		}
	}
	return cloned
}

func parseSteamReleases(appData steamCmdAppData) ([]SteamRelease, error) {
	branchesRaw, ok := appData.Depots["branches"]
	if !ok {
		return nil, errors.New("steam release metadata is missing depots.branches")
	}

	var branches map[string]steamCmdBranchEntry
	errBranches := json.Unmarshal(branchesRaw, &branches)
	if errBranches != nil {
		return nil, fmt.Errorf("decoding depots.branches: %w", errBranches)
	}
	if len(branches) == 0 {
		return nil, errors.New("steam release metadata contains no branches")
	}

	releasesByName := make(map[string]SteamRelease, len(branches))
	for branchName, branch := range branches {
		timeUpdated := strings.TrimSpace(branch.TimeUpdated)
		if timeUpdated == "" {
			timeUpdated = strings.TrimSpace(branch.TimeBuildUpdated)
		}

		releasesByName[branchName] = SteamRelease{
			Name:             branchName,
			DisplayLabel:     steamReleaseLabel(branchName, branch.Description),
			BuildID:          strings.TrimSpace(branch.BuildID),
			Description:      strings.TrimSpace(branch.Description),
			TimeUpdated:      timeUpdated,
			DepotManifestIDs: make(map[string]string),
		}
	}

	for depotID, depotRaw := range appData.Depots {
		if depotID == "branches" || depotID == "overridescddb" || depotID == "privatebranches" {
			continue
		}

		var depot steamCmdDepotEntry
		errDepot := json.Unmarshal(depotRaw, &depot)
		if errDepot != nil {
			continue
		}

		for branchName, manifest := range depot.Manifests {
			release, releaseExists := releasesByName[branchName]
			if !releaseExists {
				continue
			}
			if manifest.GID != "" {
				release.DepotManifestIDs[depotID] = manifest.GID
				releasesByName[branchName] = release
			}
		}
	}

	releases := make([]SteamRelease, 0, len(releasesByName))
	for _, release := range releasesByName {
		releases = append(releases, release)
	}

	slices.SortStableFunc(releases, compareSteamReleases)
	return releases, nil
}

func compareSteamReleases(left SteamRelease, right SteamRelease) int {
	leftPriority := steamReleasePriority(left.Name)
	rightPriority := steamReleasePriority(right.Name)
	if leftPriority != rightPriority {
		return leftPriority - rightPriority
	}

	leftTime := steamReleaseUpdatedUnix(left.TimeUpdated)
	rightTime := steamReleaseUpdatedUnix(right.TimeUpdated)
	if leftTime != rightTime {
		if leftTime > rightTime {
			return -1
		}
		return 1
	}

	return strings.Compare(left.Name, right.Name)
}

func steamReleasePriority(branchName string) int {
	switch strings.TrimSpace(strings.ToLower(branchName)) {
	case "public":
		return 0
	case "latest_experimental":
		return 1
	default:
		return 2
	}
}

func steamReleaseUpdatedUnix(value string) int64 {
	parsed, errParse := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if errParse != nil {
		return 0
	}
	return parsed
}

func steamReleaseLabel(branchName string, description string) string {
	trimmedDescription := strings.TrimSpace(description)
	if trimmedDescription != "" {
		return trimmedDescription
	}

	trimmedName := strings.TrimSpace(branchName)
	if trimmedName == "" {
		return ""
	}
	if strings.EqualFold(trimmedName, "public") {
		return "Public"
	}

	parts := strings.Fields(strings.NewReplacer("_", " ", "-", " ").Replace(trimmedName))
	for index, part := range parts {
		if part == "" {
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

// --- JSON response types ---

type steamCmdResponse struct {
	Data   map[string]steamCmdAppData `json:"data"`
	Status string                     `json:"status"`
}

type steamCmdAppData struct {
	Common struct {
		Name   string `json:"name"`
		OSList string `json:"oslist"`
		Parent string `json:"parent"`
		Type   string `json:"type"`
	} `json:"common"`
	Config struct {
		InstallDir string                         `json:"installdir"`
		Launch     map[string]steamCmdLaunchEntry `json:"launch"`
	} `json:"config"`
	Depots map[string]json.RawMessage `json:"depots"`
}

type steamCmdLaunchEntry struct {
	Executable  string `json:"executable"`
	Arguments   string `json:"arguments"`
	Description string `json:"description"`
	Config      struct {
		OSList string `json:"oslist"`
	} `json:"config"`
}

type steamCmdDepotEntry struct {
	Manifests map[string]steamCmdManifestEntry `json:"manifests"`
}

type steamCmdManifestEntry struct {
	GID string `json:"gid"`
}

type steamCmdBranchEntry struct {
	BuildID          string `json:"buildid"`
	Description      string `json:"description"`
	TimeBuildUpdated string `json:"timebuildupdated"`
	TimeUpdated      string `json:"timeupdated"`
}
