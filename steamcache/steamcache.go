// Package steamcache provides Steam app detail lookups via the api.steamcmd.net API.
package steamcache

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

const defaultDetailsURLFmt = "https://api.steamcmd.net/v1/info/%s"

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

// Client provides Steam app lookups via the api.steamcmd.net API.
type Client struct {
	detailsURLFmt string
}

// New creates a new Client.
func New() *Client {
	return &Client{
		detailsURLFmt: defaultDetailsURLFmt,
	}
}

func (c *Client) client() *http.Client {
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
