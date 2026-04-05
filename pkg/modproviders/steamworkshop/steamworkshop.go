// Package steamworkshop implements the Steam Workshop mod provider.
package steamworkshop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
)

const (
	defaultBaseURL = "https://api.steampowered.com"
	providerID     = "steam_workshop"
)

func init() {
	modproviders.RegisterProvider(New())
}

// Provider implements modproviders.ModProvider for Steam Workshop.
// It uses the Steam Web API for search and mod details, and SteamCMD for downloads.
type Provider struct {
	httpClient   *http.Client
	baseURL      string
	apiKey       string
	steamCMDPath string
}

// New creates a new Steam Workshop provider. The apiKey and steamCMDPath fields
// are empty by default and must be set via SetAPIKey and SetSteamCMDPath before use.
func New() *Provider {
	return &Provider{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: defaultBaseURL,
	}
}

// SetAPIKey sets the Steam Web API key used for search requests.
func (p *Provider) SetAPIKey(key string) {
	p.apiKey = key
}

// SetSteamCMDPath sets the path to the SteamCMD binary used for downloads.
func (p *Provider) SetSteamCMDPath(path string) {
	p.steamCMDPath = path
}

// ID returns the provider identifier.
func (p *Provider) ID() string {
	return providerID
}

// RequiresAPIKey returns true — Steam Workshop search requires a Steam Web API key.
func (p *Provider) RequiresAPIKey() bool {
	return true
}

// --------------------------------------------------------------------------
// Steam API response types
// --------------------------------------------------------------------------

type steamTag struct {
	Tag         string `json:"tag"`
	DisplayName string `json:"display_name"`
}

type steamPublishedFileDetail struct {
	PublishedFileID       string     `json:"publishedfileid"`
	Result                int        `json:"result"`
	Creator               string     `json:"creator"`
	ConsumerAppID         int        `json:"consumer_appid"`
	FileSize              string     `json:"file_size"`
	PreviewURL            string     `json:"preview_url"`
	Title                 string     `json:"title"`
	FileDescription       string     `json:"file_description"`
	TimeCreated           int64      `json:"time_created"`
	TimeUpdated           int64      `json:"time_updated"`
	Subscriptions         int64      `json:"subscriptions"`
	LifetimeSubscriptions int64      `json:"lifetime_subscriptions"`
	Tags                  []steamTag `json:"tags"`
}

type steamPublishedFileDetailsResponse struct {
	Response struct {
		Result               int                        `json:"result"`
		ResultCount          int                        `json:"resultcount"`
		PublishedFileDetails []steamPublishedFileDetail `json:"publishedfiledetails"`
	} `json:"response"`
}

type steamQueryFilesResponse struct {
	Response struct {
		Total                int                        `json:"total"`
		PublishedFileDetails []steamPublishedFileDetail `json:"publishedfiledetails"`
	} `json:"response"`
}

// --------------------------------------------------------------------------
// Search
// --------------------------------------------------------------------------

// Search queries the IPublishedFileService/QueryFiles endpoint.
// An API key is required; returns an error if none is set.
func (p *Provider) Search(ctx context.Context, query string, params modproviders.SearchParams) (modproviders.SearchResult, error) {
	if p.apiKey == "" {
		return modproviders.SearchResult{}, errors.New("steam Workshop search requires an API key")
	}

	appID := extractAppID(params)

	queryParams := url.Values{}
	queryParams.Set("key", p.apiKey)
	queryParams.Set("query_type", "1")
	queryParams.Set("search_text", query)
	queryParams.Set("return_metadata", "true")
	queryParams.Set("numperpage", "20")
	if appID != "" {
		queryParams.Set("appid", appID)
	}

	endpoint := fmt.Sprintf("%s/IPublishedFileService/QueryFiles/v1/?%s", p.baseURL, queryParams.Encode())

	var searchResp steamQueryFilesResponse
	errFetch := p.getJSON(ctx, endpoint, &searchResp)
	if errFetch != nil {
		return modproviders.SearchResult{}, fmt.Errorf("steam workshop search: %w", errFetch)
	}

	results := make([]modproviders.ModSearchResult, 0, len(searchResp.Response.PublishedFileDetails))
	for _, detail := range searchResp.Response.PublishedFileDetails {
		results = append(results, mapDetailToSearchResult(detail))
	}
	return modproviders.SearchResult{
		Results:   results,
		TotalHits: searchResp.Response.Total,
	}, nil
}

// --------------------------------------------------------------------------
// GetModDetails
// --------------------------------------------------------------------------

// GetModDetails fetches full details for a Steam Workshop item using its published file ID.
// This endpoint does not require an API key.
func (p *Provider) GetModDetails(ctx context.Context, sourceID string, params modproviders.SearchParams) (*modproviders.ModDetails, error) {
	detail, errDetail := p.getPublishedFileDetails(ctx, sourceID)
	if errDetail != nil {
		return nil, fmt.Errorf("steam workshop get mod details %q: %w", sourceID, errDetail)
	}

	tags := make([]string, 0, len(detail.Tags))
	for _, t := range detail.Tags {
		tags = append(tags, t.Tag)
	}

	versions, errVersions := p.GetVersions(ctx, sourceID, "", params)
	if errVersions != nil {
		log.Warn().Err(errVersions).Str("sourceID", sourceID).Msg("steam workshop: failed to fetch versions for mod details")
	}

	return &modproviders.ModDetails{
		Source:      providerID,
		SourceID:    detail.PublishedFileID,
		Name:        detail.Title,
		Author:      detail.Creator,
		Description: detail.FileDescription,
		IconURL:     detail.PreviewURL,
		Downloads:   detail.LifetimeSubscriptions,
		Categories:  tags,
		Versions:    versions,
	}, nil
}

// --------------------------------------------------------------------------
// GetVersions
// --------------------------------------------------------------------------

// GetVersions returns a single "version" for the given Workshop item representing
// its current state. Steam Workshop items do not have traditional version histories.
func (p *Provider) GetVersions(ctx context.Context, sourceID string, _ string, _ modproviders.SearchParams) ([]modproviders.ModVersion, error) {
	detail, errDetail := p.getPublishedFileDetails(ctx, sourceID)
	if errDetail != nil {
		return nil, fmt.Errorf("steam workshop get versions %q: %w", sourceID, errDetail)
	}

	updatedAt := time.Unix(detail.TimeUpdated, 0).UTC()
	versionStr := fmt.Sprintf("updated_%s", updatedAt.Format("20060102"))

	return []modproviders.ModVersion{
		{
			VersionID:     detail.PublishedFileID,
			VersionString: versionStr,
			GameVersions:  []string{},
			DownloadURL:   "",
			FileSize:      parseFileSize(detail.FileSize),
			Dependencies:  []modproviders.ModDependency{},
			Changelog:     fmt.Sprintf("Last updated: %s", updatedAt.Format(time.RFC3339)),
		},
	}, nil
}

// --------------------------------------------------------------------------
// Download
// --------------------------------------------------------------------------

// Download uses SteamCMD to download the Workshop item and copies the files to targetDir.
// steamCMDPath must be set via SetSteamCMDPath before calling this method.
func (p *Provider) Download(ctx context.Context, sourceID string, _ string, targetDir string) ([]modproviders.DownloadedFile, error) {
	if p.steamCMDPath == "" {
		return nil, errors.New("steam workshop download requires steamcmd: set the steamcmd path via SetSteamCMDPath")
	}

	appID := ""
	detail, errDetail := p.getPublishedFileDetails(ctx, sourceID)
	if errDetail != nil {
		log.Warn().Err(errDetail).Str("sourceID", sourceID).Msg("steam workshop: could not fetch app ID from published file details; download may fail")
	} else {
		appID = fmt.Sprintf("%d", detail.ConsumerAppID)
	}

	if appID == "" || appID == "0" {
		return nil, fmt.Errorf("steam workshop download: could not determine app ID for workshop item %q", sourceID)
	}

	parsedAppID, errAppID := strconv.ParseUint(appID, 10, 32)
	if errAppID != nil || parsedAppID == 0 {
		return nil, fmt.Errorf("steam workshop download: invalid app ID %q for workshop item %q", appID, sourceID)
	}

	parsedSourceID, errSourceID := strconv.ParseUint(sourceID, 10, 64)
	if errSourceID != nil || parsedSourceID == 0 {
		return nil, fmt.Errorf("steam workshop download: invalid workshop item ID %q", sourceID)
	}

	steamCMDPath := filepath.Clean(p.steamCMDPath)
	steamCMDBinary := strings.ToLower(filepath.Base(steamCMDPath))
	if steamCMDBinary != "steamcmd" && steamCMDBinary != "steamcmd.exe" {
		return nil, fmt.Errorf("steam workshop download: invalid steamcmd binary path %q", p.steamCMDPath)
	}

	cmd := exec.CommandContext(ctx, steamCMDPath,
		"+login", "anonymous",
		"+workshop_download_item", strconv.FormatUint(parsedAppID, 10), strconv.FormatUint(parsedSourceID, 10),
		"+quit",
	)

	output, errRun := cmd.CombinedOutput()
	if errRun != nil {
		return nil, fmt.Errorf("steam workshop download: steamcmd failed for item %s (app %s): %w\noutput: %s",
			sourceID, appID, errRun, string(output))
	}

	steamappsDir := filepath.Dir(steamCMDPath)
	contentDir := filepath.Join(steamappsDir, "steamapps", "workshop", "content", strconv.FormatUint(parsedAppID, 10), strconv.FormatUint(parsedSourceID, 10))

	errMkdir := os.MkdirAll(targetDir, 0o750)
	if errMkdir != nil {
		return nil, fmt.Errorf("steam workshop download: create target dir %s: %w", targetDir, errMkdir)
	}

	downloadedFiles, errCopy := copyDirContents(contentDir, targetDir)
	if errCopy != nil {
		return nil, fmt.Errorf("steam workshop download: copy files from %s to %s: %w", contentDir, targetDir, errCopy)
	}

	return downloadedFiles, nil
}

// --------------------------------------------------------------------------
// CheckForUpdate
// --------------------------------------------------------------------------

// CheckForUpdate returns a ModVersion representing the current published state of
// the Workshop item. Callers can compare the VersionID (which encodes time_updated)
// against the installed timestamp to detect updates.
func (p *Provider) CheckForUpdate(ctx context.Context, sourceID string, _ string) (*modproviders.ModVersion, error) {
	versions, errVersions := p.GetVersions(ctx, sourceID, "", nil)
	if errVersions != nil {
		return nil, fmt.Errorf("steam workshop check for update %q: %w", sourceID, errVersions)
	}
	if len(versions) == 0 {
		return nil, modproviders.ErrNoUpdateAvailable
	}
	v := versions[0]
	return &v, nil
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

// getPublishedFileDetails fetches details for a single published file using the
// ISteamRemoteStorage/GetPublishedFileDetails endpoint (no API key required).
func (p *Provider) getPublishedFileDetails(ctx context.Context, publishedFileID string) (*steamPublishedFileDetail, error) {
	endpoint := fmt.Sprintf("%s/ISteamRemoteStorage/GetPublishedFileDetails/v1/", p.baseURL)

	formBody := url.Values{}
	formBody.Set("itemcount", "1")
	formBody.Set("publishedfileids[0]", publishedFileID)

	req, errReq := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(formBody.Encode()))
	if errReq != nil {
		return nil, fmt.Errorf("build request for %s: %w", endpoint, errReq)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, errDo := p.httpClient.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("POST %s: %w", endpoint, errDo)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Warn().Err(errClose).Str("url", endpoint).Msg("steam workshop: failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("POST %s: unexpected status %d", endpoint, resp.StatusCode)
	}

	var fileDetailsResp steamPublishedFileDetailsResponse
	errDecode := json.NewDecoder(resp.Body).Decode(&fileDetailsResp)
	if errDecode != nil {
		return nil, fmt.Errorf("decode response from %s: %w", endpoint, errDecode)
	}

	details := fileDetailsResp.Response.PublishedFileDetails
	if len(details) == 0 {
		return nil, fmt.Errorf("no published file details returned for ID %q", publishedFileID)
	}

	detail := details[0]
	if detail.Result != 1 {
		return nil, fmt.Errorf("published file %q returned non-success result: %d", publishedFileID, detail.Result)
	}

	return &detail, nil
}

// getJSON performs a GET request to the given URL and decodes the JSON body into dest.
func (p *Provider) getJSON(ctx context.Context, endpoint string, dest any) error {
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if errReq != nil {
		return fmt.Errorf("build request for %s: %w", endpoint, errReq)
	}

	resp, errDo := p.httpClient.Do(req)
	if errDo != nil {
		return fmt.Errorf("GET %s: %w", endpoint, errDo)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Warn().Err(errClose).Str("url", endpoint).Msg("steam workshop: failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status %d", endpoint, resp.StatusCode)
	}

	errDecode := json.NewDecoder(resp.Body).Decode(dest)
	if errDecode != nil {
		return fmt.Errorf("decode response from %s: %w", endpoint, errDecode)
	}
	return nil
}

// mapDetailToSearchResult converts a Steam published file detail to a ModSearchResult.
func mapDetailToSearchResult(detail steamPublishedFileDetail) modproviders.ModSearchResult {
	tags := make([]string, 0, len(detail.Tags))
	for _, t := range detail.Tags {
		tags = append(tags, t.Tag)
	}

	dateModified := ""
	if detail.TimeUpdated > 0 {
		dateModified = time.Unix(detail.TimeUpdated, 0).UTC().Format(time.RFC3339)
	}

	return modproviders.ModSearchResult{
		Source:       providerID,
		SourceID:     detail.PublishedFileID,
		Name:         detail.Title,
		Author:       detail.Creator,
		Description:  detail.FileDescription,
		IconURL:      detail.PreviewURL,
		Downloads:    detail.LifetimeSubscriptions,
		Categories:   tags,
		DateModified: dateModified,
	}
}

// extractAppID pulls the "app_id" string from SearchParams if present.
func extractAppID(params modproviders.SearchParams) string {
	if params == nil {
		return ""
	}
	raw, ok := params["app_id"]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%v", raw)
}

// parseFileSize converts the file_size string from the Steam API to an int64.
// The field is a quoted integer (e.g., "15728640").
func parseFileSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var size int64
	_, errScan := fmt.Sscanf(s, "%d", &size)
	if errScan != nil {
		return 0
	}
	return size
}

// copyDirContents recursively copies files from src to dst, returning a list
// of files written.
func copyDirContents(src, dst string) ([]modproviders.DownloadedFile, error) {
	var downloaded []modproviders.DownloadedFile

	errWalk := filepath.Walk(src, func(path string, info os.FileInfo, errWalk error) error {
		if errWalk != nil {
			return errWalk
		}
		if info.IsDir() {
			return nil
		}

		rel, errRel := filepath.Rel(src, path)
		if errRel != nil {
			return fmt.Errorf("get relative path: %w", errRel)
		}

		destPath := filepath.Join(dst, rel)
		destDir := filepath.Dir(destPath)
		errMkdir := os.MkdirAll(destDir, 0o750)
		if errMkdir != nil {
			return fmt.Errorf("create directory %s: %w", destDir, errMkdir)
		}

		written, errCopy := copyFile(path, destPath)
		if errCopy != nil {
			return fmt.Errorf("copy %s -> %s: %w", path, destPath, errCopy)
		}

		downloaded = append(downloaded, modproviders.DownloadedFile{
			Path:      rel,
			Size:      written,
			IsPrimary: len(downloaded) == 0,
		})
		return nil
	})
	if errWalk != nil {
		return nil, fmt.Errorf("walk copied workshop files: %w", errWalk)
	}

	return downloaded, nil
}

// copyFile copies a single file from src to dst and returns the number of bytes written.
func copyFile(src, dst string) (int64, error) {
	srcFile, errOpen := os.Open(src)
	if errOpen != nil {
		return 0, fmt.Errorf("open source file: %w", errOpen)
	}
	defer func() {
		if errClose := srcFile.Close(); errClose != nil {
			log.Warn().Err(errClose).Str("path", src).Msg("steam workshop: failed to close source file")
		}
	}()

	dstFile, errCreate := os.Create(dst)
	if errCreate != nil {
		return 0, fmt.Errorf("create destination file: %w", errCreate)
	}
	defer func() {
		if errClose := dstFile.Close(); errClose != nil {
			log.Warn().Err(errClose).Str("path", dst).Msg("steam workshop: failed to close destination file")
		}
	}()

	written, errCopy := io.Copy(dstFile, srcFile)
	if errCopy != nil {
		return 0, fmt.Errorf("copy data: %w", errCopy)
	}

	return written, nil
}
