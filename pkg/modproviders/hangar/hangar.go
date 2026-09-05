// Package hangar implements the Hangar mod provider.
package hangar

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/modproviders/internal/providerhttp"
)

const (
	defaultBaseURL = "https://hangar.papermc.io/api/v1"
	userAgent      = "Xylona/1.0 (github.com/ClintonCollins/Xylona)"
	providerID     = "hangar"
)

func init() {
	modproviders.RegisterProvider(New())
}

// Provider implements modproviders.ModProvider for Hangar (hangar.papermc.io/api/v1).
type Provider struct {
	httpClient *http.Client
	baseURL    string
}

// New creates a new Hangar provider with a default HTTP client that injects the
// required User-Agent header on every request.
func New() *Provider {
	return &Provider{
		httpClient: providerhttp.NewUserAgentClient(userAgent),
		baseURL:    defaultBaseURL,
	}
}

// ID returns the provider identifier.
func (p *Provider) ID() string {
	return providerID
}

// --------------------------------------------------------------------------
// JSON types
// --------------------------------------------------------------------------

type hangarNamespace struct {
	Owner string `json:"owner"`
	Slug  string `json:"slug"`
}

type hangarProjectStats struct {
	Downloads int64 `json:"downloads"`
	Stars     int64 `json:"stars"`
}

type hangarSettingsLink struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type hangarSettingsLicense struct {
	Name string `json:"name"`
}

type hangarProjectSettings struct {
	Tags    []string              `json:"tags"`
	Links   []hangarSettingsLink  `json:"links"`
	License hangarSettingsLicense `json:"license"`
}

type hangarProject struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Namespace   hangarNamespace       `json:"namespace"`
	Stats       hangarProjectStats    `json:"stats"`
	AvatarURL   string                `json:"avatarUrl"`
	Settings    hangarProjectSettings `json:"settings"`
}

type hangarPagination struct {
	Count int `json:"count"`
}

type hangarSearchResponse struct {
	Result     []hangarProject  `json:"result"`
	Pagination hangarPagination `json:"pagination"`
}

type hangarVersionFileInfo struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	SHA256    string `json:"sha256Hash"`
}

type hangarPlatformDownload struct {
	FileInfo    hangarVersionFileInfo `json:"fileInfo"`
	DownloadURL string                `json:"downloadUrl"`
}

type hangarVersionStats struct {
	Downloads int64 `json:"downloads"`
}

type hangarPluginDependency struct {
	Name      string          `json:"name"`
	Required  bool            `json:"required"`
	Namespace hangarNamespace `json:"namespace"`
}

type hangarVersion struct {
	Name                 string                              `json:"name"`
	Description          string                              `json:"description"`
	Stats                hangarVersionStats                  `json:"stats"`
	CreatedAt            string                              `json:"createdAt"`
	Downloads            map[string]hangarPlatformDownload   `json:"downloads"`
	PlatformDependencies map[string][]string                 `json:"platformDependencies"`
	PluginDependencies   map[string][]hangarPluginDependency `json:"pluginDependencies"`
}

type hangarVersionsResponse struct {
	Result []hangarVersion `json:"result"`
}

// --------------------------------------------------------------------------
// Search
// --------------------------------------------------------------------------

// Search queries the Hangar search endpoint and returns matching plugins.
func (p *Provider) Search(ctx context.Context, query string, params modproviders.SearchParams) (modproviders.SearchResult, error) {
	queryParams := url.Values{}
	queryParams.Set("q", query)

	limit := providerhttp.IntParam(params, modproviders.ParamLimit, 20)
	queryParams.Set("limit", fmt.Sprintf("%d", limit))

	offset := providerhttp.IntParam(params, modproviders.ParamOffset, 0)
	queryParams.Set("offset", fmt.Sprintf("%d", offset))

	sortBy := providerhttp.StringParam(params, modproviders.ParamSortBy)
	if sortBy != "" {
		hangarSort := mapSortToHangar(sortBy)
		if hangarSort != "" {
			queryParams.Set("sort", hangarSort)
		}
	}

	gameVersion := providerhttp.StringParam(params, modproviders.ParamGameVersion)
	if gameVersion != "" {
		queryParams.Set("version", gameVersion)
	}

	platform := extractPlatform(params)
	if platform != "" {
		queryParams.Set("platform", platform)
	}

	endpoint := fmt.Sprintf("%s/projects?%s", p.baseURL, queryParams.Encode())

	var searchResp hangarSearchResponse
	errFetch := p.getJSON(ctx, endpoint, &searchResp)
	if errFetch != nil {
		return modproviders.SearchResult{}, fmt.Errorf("hangar search: %w", errFetch)
	}

	results := make([]modproviders.ModSearchResult, 0, len(searchResp.Result))
	for _, proj := range searchResp.Result {
		sourceID := proj.Namespace.Owner + "/" + proj.Namespace.Slug
		results = append(results, modproviders.ModSearchResult{
			Source:      providerID,
			SourceID:    sourceID,
			Name:        proj.Name,
			Author:      proj.Namespace.Owner,
			Description: proj.Description,
			IconURL:     proj.AvatarURL,
			Downloads:   proj.Stats.Downloads,
			Categories:  proj.Settings.Tags,
		})
	}
	return modproviders.SearchResult{
		Results:   results,
		TotalHits: searchResp.Pagination.Count,
	}, nil
}

// --------------------------------------------------------------------------
// GetModDetails
// --------------------------------------------------------------------------

// GetModDetails fetches full project metadata and its versions.
func (p *Provider) GetModDetails(ctx context.Context, sourceID string, params modproviders.SearchParams) (*modproviders.ModDetails, error) {
	author, slug, errSplit := splitSourceID(sourceID)
	if errSplit != nil {
		return nil, fmt.Errorf("hangar get project %q: %w", sourceID, errSplit)
	}

	projectEndpoint := fmt.Sprintf("%s/projects/%s/%s", p.baseURL, url.PathEscape(author), url.PathEscape(slug))

	var project hangarProject
	errProject := p.getJSON(ctx, projectEndpoint, &project)
	if errProject != nil {
		return nil, fmt.Errorf("hangar get project %q: %w", sourceID, errProject)
	}

	versions, errVersions := p.GetVersions(ctx, sourceID, "", params)
	if errVersions != nil {
		log.Warn().Err(errVersions).Str("sourceID", sourceID).Msg("hangar: failed to fetch versions for mod details")
	}

	sourceURL := ""
	for _, link := range project.Settings.Links {
		if strings.EqualFold(link.Type, "SOURCE") {
			sourceURL = link.URL
			break
		}
	}

	tags := make([]string, 0, len(project.Settings.Tags))
	tags = append(tags, project.Settings.Tags...)

	return &modproviders.ModDetails{
		Source:      providerID,
		SourceID:    sourceID,
		Name:        project.Name,
		Author:      project.Namespace.Owner,
		Description: project.Description,
		IconURL:     project.AvatarURL,
		Downloads:   project.Stats.Downloads,
		Categories:  tags,
		License:     project.Settings.License.Name,
		SourceURL:   sourceURL,
		Versions:    versions,
	}, nil
}

// --------------------------------------------------------------------------
// GetVersions
// --------------------------------------------------------------------------

// GetVersions returns versions for the given project, optionally filtered to a specific game version.
func (p *Provider) GetVersions(ctx context.Context, sourceID string, gameVersion string, params modproviders.SearchParams) ([]modproviders.ModVersion, error) {
	author, slug, errSplit := splitSourceID(sourceID)
	if errSplit != nil {
		return nil, fmt.Errorf("hangar get versions %q: %w", sourceID, errSplit)
	}

	queryParams := url.Values{}
	platform := extractPlatform(params)
	if platform != "" {
		queryParams.Set("platform", platform)
	}

	endpoint := fmt.Sprintf("%s/projects/%s/%s/versions", p.baseURL, url.PathEscape(author), url.PathEscape(slug))
	if len(queryParams) > 0 {
		endpoint = fmt.Sprintf("%s?%s", endpoint, queryParams.Encode())
	}

	var raw hangarVersionsResponse
	errFetch := p.getJSON(ctx, endpoint, &raw)
	if errFetch != nil {
		return nil, fmt.Errorf("hangar get versions %q: %w", sourceID, errFetch)
	}

	versions := make([]modproviders.ModVersion, 0, len(raw.Result))
	for _, v := range raw.Result {
		downloadURL, fileSize, fileHashSHA256 := primaryDownloadFor(v.Downloads, platform)

		gameVersions := collectGameVersions(v.PlatformDependencies)
		if gameVersion != "" {
			if !slices.Contains(gameVersions, gameVersion) {
				continue
			}
		}

		deps := collectDependencies(v.PluginDependencies, platform)

		versions = append(versions, modproviders.ModVersion{
			VersionID:      v.Name,
			VersionString:  v.Name,
			GameVersions:   gameVersions,
			DownloadURL:    downloadURL,
			FileSize:       fileSize,
			FileHashSHA256: fileHashSHA256,
			Dependencies:   deps,
			Changelog:      v.Description,
		})
	}
	return versions, nil
}

// --------------------------------------------------------------------------
// Download
// --------------------------------------------------------------------------

// Download fetches the primary file for the given version and writes it to targetDir.
// For Hangar, versionID is the version name string (e.g., "7.2.15").
func (p *Provider) Download(ctx context.Context, sourceID string, versionID string, targetDir string) ([]modproviders.DownloadedFile, error) {
	author, slug, errSplit := splitSourceID(sourceID)
	if errSplit != nil {
		return nil, fmt.Errorf("hangar download %q: %w", sourceID, errSplit)
	}

	versionsEndpoint := fmt.Sprintf("%s/projects/%s/%s/versions", p.baseURL, url.PathEscape(author), url.PathEscape(slug))

	var raw hangarVersionsResponse
	errFetch := p.getJSON(ctx, versionsEndpoint, &raw)
	if errFetch != nil {
		return nil, fmt.Errorf("hangar download — fetch versions for %q: %w", sourceID, errFetch)
	}

	var targetVersion *hangarVersion
	for i := range raw.Result {
		if raw.Result[i].Name == versionID {
			targetVersion = &raw.Result[i]
			break
		}
	}
	if targetVersion == nil {
		return nil, fmt.Errorf("hangar download: version %q not found for %q", versionID, sourceID)
	}

	var targetDownload hangarPlatformDownload
	targetDownloadFound := false
	for _, download := range targetVersion.Downloads {
		targetDownload = download
		targetDownloadFound = true
		break
	}
	if !targetDownloadFound {
		return nil, fmt.Errorf("hangar download: no download URL found for version %q of %q", versionID, sourceID)
	}

	downloadURL := targetDownload.DownloadURL
	if downloadURL == "" {
		return nil, fmt.Errorf("hangar download: no download URL found for version %q of %q", versionID, sourceID)
	}
	expectedSHA256 := targetDownload.FileInfo.SHA256
	if expectedSHA256 == "" {
		return nil, fmt.Errorf("hangar download: missing SHA-256 for version %q of %q: %w", versionID, sourceID, modproviders.ErrMissingIntegrityMetadata)
	}

	fileName := filepath.Base(downloadURL)
	if fileName == "" || fileName == "." {
		fileName = fmt.Sprintf("%s-%s.jar", slug, versionID)
	}
	destPath := filepath.Join(targetDir, fileName)

	written, hash, errDownload := providerhttp.DownloadToFile(ctx, p.httpClient, downloadURL, destPath, providerID)
	if errDownload != nil {
		return nil, fmt.Errorf("hangar download: %w", errDownload)
	}
	if !strings.EqualFold(hash, expectedSHA256) {
		return nil, fmt.Errorf("hangar download: SHA-256 mismatch for %s: got %s, want %s: %w", fileName, hash, expectedSHA256, modproviders.ErrIntegrityMismatch)
	}

	return []modproviders.DownloadedFile{
		{
			Path:      fileName,
			Hash:      hash,
			Size:      written,
			IsPrimary: true,
		},
	}, nil
}

// --------------------------------------------------------------------------
// CheckForUpdate
// --------------------------------------------------------------------------

// CheckForUpdate returns the latest version compatible with the given game version.
func (p *Provider) CheckForUpdate(ctx context.Context, sourceID string, gameVersion string) (*modproviders.ModVersion, error) {
	versions, errVersions := p.GetVersions(ctx, sourceID, gameVersion, nil)
	if errVersions != nil {
		return nil, fmt.Errorf("hangar check for update %q: %w", sourceID, errVersions)
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

// getJSON performs a GET request to the given URL and decodes the JSON body into dest.
func (p *Provider) getJSON(ctx context.Context, endpoint string, dest any) error {
	errGet := providerhttp.GetJSON(ctx, p.httpClient, endpoint, dest, providerID)
	if errGet != nil {
		return fmt.Errorf("hangar get JSON: %w", errGet)
	}
	return nil
}

// splitSourceID splits an "author/slug" sourceID into its components.
func splitSourceID(sourceID string) (author, slug string, err error) {
	parts := strings.SplitN(sourceID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid sourceID %q: expected \"author/slug\" format", sourceID)
	}
	return parts[0], parts[1], nil
}

// extractPlatform pulls the platform string from SearchParams["platform"].
func extractPlatform(params modproviders.SearchParams) string {
	return providerhttp.StringParam(params, "platform")
}

// primaryDownloadFor returns the download URL and file size for the best available platform.
// If platform is specified and present, that platform is preferred. Otherwise the first
// available platform is used.
func primaryDownloadFor(downloads map[string]hangarPlatformDownload, platform string) (downloadURL string, fileSize int64, fileHashSHA256 string) {
	if platform != "" {
		if d, ok := downloads[platform]; ok {
			return d.DownloadURL, d.FileInfo.SizeBytes, d.FileInfo.SHA256
		}
	}
	// Fall back to first available platform entry.
	for _, d := range downloads {
		return d.DownloadURL, d.FileInfo.SizeBytes, d.FileInfo.SHA256
	}
	return "", 0, ""
}

// collectGameVersions merges all game version arrays from all platforms into a deduplicated slice.
func collectGameVersions(platformDependencies map[string][]string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, versions := range platformDependencies {
		for _, v := range versions {
			if _, exists := seen[v]; !exists {
				seen[v] = struct{}{}
				result = append(result, v)
			}
		}
	}
	return result
}

// mapSortToHangar maps well-known sort values to Hangar's sort parameter format.
func mapSortToHangar(sortBy string) string {
	switch sortBy {
	case "downloads":
		return "-downloads"
	case "updated":
		return "-updated"
	case "newest":
		return "-created_at"
	case "relevance":
		return "relevance"
	default:
		return ""
	}
}

// collectDependencies returns ModDependency entries from plugin dependencies.
// If platform is specified, only that platform's deps are returned; otherwise all are merged.
func collectDependencies(pluginDeps map[string][]hangarPluginDependency, platform string) []modproviders.ModDependency {
	var deps []modproviders.ModDependency
	if platform != "" {
		platformDeps, ok := pluginDeps[platform]
		if ok {
			for _, d := range platformDeps {
				sourceID := d.Namespace.Owner + "/" + d.Namespace.Slug
				if d.Namespace.Owner == "" || d.Namespace.Slug == "" {
					sourceID = d.Name
				}
				deps = append(deps, modproviders.ModDependency{
					SourceID: sourceID,
					Name:     d.Name,
					Required: d.Required,
				})
			}
		}
		return deps
	}
	// No platform — collect from all platforms, deduplicating by name.
	seen := make(map[string]struct{})
	for _, platformDeps := range pluginDeps {
		for _, d := range platformDeps {
			if _, exists := seen[d.Name]; exists {
				continue
			}
			seen[d.Name] = struct{}{}
			sourceID := d.Namespace.Owner + "/" + d.Namespace.Slug
			if d.Namespace.Owner == "" || d.Namespace.Slug == "" {
				sourceID = d.Name
			}
			deps = append(deps, modproviders.ModDependency{
				SourceID: sourceID,
				Name:     d.Name,
				Required: d.Required,
			})
		}
	}
	return deps
}
