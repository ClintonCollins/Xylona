package steamcache

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	maxSearchResults = 20
	refreshInterval  = 24 * time.Hour
	detailsURLFmt    = "https://api.steamcmd.net/v1/info/%s"
	appListURL       = "https://api.steampowered.com/ISteamApps/GetAppList/v2/"
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
	WindowsSupport   bool
	LinuxSupport     bool
	InstallDirectory string
	ParentAppID      string
	LaunchConfigs    []LaunchConfig
}

// Fetcher defines the interface for fetching Steam app data.
type Fetcher interface {
	FetchAppList(ctx context.Context) ([]SteamApp, error)
}

// Cache holds a cached list of Steam apps and provides search functionality.
type Cache struct {
	fetcher Fetcher
	mu      sync.RWMutex
	apps    []SteamApp
}

// New creates a new Cache with the given Fetcher.
func New(fetcher Fetcher) *Cache {
	return &Cache{
		fetcher: fetcher,
	}
}

// FilterApps filters a list of SteamApp entries to those containing
// "server" or "dedicated" in their name (case-insensitive).
func FilterApps(apps []SteamApp) []SteamApp {
	var filtered []SteamApp
	for _, app := range apps {
		lower := strings.ToLower(app.Name)
		if strings.Contains(lower, "server") || strings.Contains(lower, "dedicated") {
			filtered = append(filtered, app)
		}
	}
	return filtered
}

// Start performs an initial fetch and then refreshes every 24 hours in the
// background. It returns when the context is cancelled.
func (c *Cache) Start(ctx context.Context) {
	errLoad := c.loadApps(ctx)
	if errLoad != nil {
		log.Error().Err(errLoad).Msg("steamcache: initial app list fetch failed")
	}

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			errRefresh := c.loadApps(ctx)
			if errRefresh != nil {
				log.Error().Err(errRefresh).Msg("steamcache: refresh failed, keeping existing data")
			}
		}
	}
}

// loadApps fetches apps from the fetcher and updates the cache.
// On failure it returns the error without clearing existing cached data.
func (c *Cache) loadApps(ctx context.Context) error {
	apps, errFetch := c.fetcher.FetchAppList(ctx)
	if errFetch != nil {
		return fmt.Errorf("fetching app list: %w", errFetch)
	}

	c.mu.Lock()
	c.apps = apps
	c.mu.Unlock()

	return nil
}

// Search returns apps whose name contains the query (case-insensitive),
// up to a maximum of 20 results. Returns nil for empty queries.
func (c *Cache) Search(query string) []SteamApp {
	if query == "" {
		return nil
	}

	lowerQuery := strings.ToLower(query)

	c.mu.RLock()
	defer c.mu.RUnlock()

	var results []SteamApp
	for _, app := range c.apps {
		if strings.Contains(strings.ToLower(app.Name), lowerQuery) {
			results = append(results, app)
			if len(results) >= maxSearchResults {
				break
			}
		}
	}
	return results
}

// FindByID returns the cached SteamApp with the given appID, or nil if not found.
func (c *Cache) FindByID(appID string) *SteamApp {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, app := range c.apps {
		if app.AppID == appID {
			return &app
		}
	}
	return nil
}

// FetchDetails retrieves detailed information about a Steam app from the
// steamcmd.net API.
func (c *Cache) FetchDetails(ctx context.Context, appID string) (*SteamAppDetails, error) {
	url := fmt.Sprintf(detailsURLFmt, appID)
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if errReq != nil {
		return nil, fmt.Errorf("creating request for app %s: %w", appID, errReq)
	}

	resp, errDo := http.DefaultClient.Do(req)
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

	return details, nil
}

// --- SteamAPIFetcher: default Fetcher implementation ---

// SteamAPIFetcher fetches the Steam app list from the Steam Web API and filters
// to entries whose name contains "server" or "dedicated".
type SteamAPIFetcher struct{}

func (f *SteamAPIFetcher) FetchAppList(ctx context.Context) ([]SteamApp, error) {
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, appListURL, nil)
	if errReq != nil {
		return nil, fmt.Errorf("creating steam app list request: %w", errReq)
	}

	resp, errDo := http.DefaultClient.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("fetching steam app list: %w", errDo)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam app list returned status %d", resp.StatusCode)
	}

	var raw steamAppListResponse
	errDecode := json.NewDecoder(resp.Body).Decode(&raw)
	if errDecode != nil {
		return nil, fmt.Errorf("decoding steam app list: %w", errDecode)
	}

	apps := make([]SteamApp, 0, len(raw.AppList.Apps))
	for _, a := range raw.AppList.Apps {
		apps = append(apps, SteamApp{
			AppID: fmt.Sprintf("%d", a.AppID),
			Name:  a.Name,
		})
	}

	return FilterApps(apps), nil
}

// --- JSON response types ---

type steamAppListResponse struct {
	AppList struct {
		Apps []struct {
			AppID int    `json:"appid"`
			Name  string `json:"name"`
		} `json:"apps"`
	} `json:"applist"`
}

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
		InstallDir string                        `json:"installdir"`
		Launch     map[string]steamCmdLaunchEntry `json:"launch"`
	} `json:"config"`
}

type steamCmdLaunchEntry struct {
	Executable  string `json:"executable"`
	Arguments   string `json:"arguments"`
	Description string `json:"description"`
	Config      struct {
		OSList string `json:"oslist"`
	} `json:"config"`
}
