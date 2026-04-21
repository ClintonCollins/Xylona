// Package modrinth implements the Modrinth mod provider.
package modrinth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/modproviders/internal/providerhttp"
)

const (
	defaultBaseURL = "https://api.modrinth.com/v2"
	userAgent      = "Xylona/1.0 (github.com/ClintonCollins/Xylona)"
	providerID     = "modrinth"
)

func init() {
	modproviders.RegisterProvider(New())
}

// Provider implements modproviders.ModProvider for Modrinth (api.modrinth.com/v2).
type Provider struct {
	httpClient *http.Client
	baseURL    string
}

// New creates a new Modrinth provider with a default HTTP client that injects the
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

// RequiresAPIKey returns false — Modrinth read API is public.
func (p *Provider) RequiresAPIKey() bool {
	return false
}

// --------------------------------------------------------------------------
// Search
// --------------------------------------------------------------------------

// modrinthSearchHit maps the JSON returned by the /search endpoint.
type modrinthSearchHit struct {
	Slug         string   `json:"slug"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Author       string   `json:"author"`
	IconURL      string   `json:"icon_url"`
	Downloads    int64    `json:"downloads"`
	Versions     []string `json:"versions"`
	Categories   []string `json:"categories"`
	DateModified string   `json:"date_modified"`
}

type modrinthSearchResponse struct {
	Hits      []modrinthSearchHit `json:"hits"`
	TotalHits int                 `json:"total_hits"`
	Offset    int                 `json:"offset"`
	Limit     int                 `json:"limit"`
}

// Search queries the Modrinth search endpoint and returns matching mods.
func (p *Provider) Search(ctx context.Context, query string, params modproviders.SearchParams) (modproviders.SearchResult, error) {
	facets := buildFacets(params)

	// Merge well-known filter params into facets.
	facets = mergeFacets(facets, params)

	queryParams := url.Values{}
	queryParams.Set("query", query)

	limit := providerhttp.IntParam(params, modproviders.ParamLimit, 20)
	queryParams.Set("limit", fmt.Sprintf("%d", limit))

	offset := providerhttp.IntParam(params, modproviders.ParamOffset, 0)
	if offset > 0 {
		queryParams.Set("offset", fmt.Sprintf("%d", offset))
	}

	sortBy := providerhttp.StringParam(params, modproviders.ParamSortBy)
	if sortBy != "" {
		// Modrinth uses the same names: downloads, updated, newest, relevance.
		queryParams.Set("index", sortBy)
	}

	if facets != "" {
		queryParams.Set("facets", facets)
	}

	endpoint := fmt.Sprintf("%s/search?%s", p.baseURL, queryParams.Encode())

	var searchResp modrinthSearchResponse
	errFetch := p.getJSON(ctx, endpoint, &searchResp)
	if errFetch != nil {
		return modproviders.SearchResult{}, fmt.Errorf("modrinth search: %w", errFetch)
	}

	results := make([]modproviders.ModSearchResult, 0, len(searchResp.Hits))
	for _, hit := range searchResp.Hits {
		latest := ""
		if len(hit.Versions) > 0 {
			latest = hit.Versions[len(hit.Versions)-1]
		}
		results = append(results, modproviders.ModSearchResult{
			Source:             providerID,
			SourceID:           hit.Slug,
			Name:               hit.Title,
			Author:             hit.Author,
			Description:        hit.Description,
			IconURL:            hit.IconURL,
			Downloads:          hit.Downloads,
			LatestVersion:      latest,
			CompatibleVersions: hit.Versions,
			Categories:         hit.Categories,
			DateModified:       hit.DateModified,
		})
	}
	return modproviders.SearchResult{
		Results:   results,
		TotalHits: searchResp.TotalHits,
	}, nil
}

// --------------------------------------------------------------------------
// GetModDetails
// --------------------------------------------------------------------------

type modrinthProject struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Body        string `json:"body"`
	IconURL     string `json:"icon_url"`
	Downloads   int64  `json:"downloads"`
	Gallery     []struct {
		URL string `json:"url"`
	} `json:"gallery"`
	Categories []string `json:"categories"`
	License    struct {
		ID string `json:"id"`
	} `json:"license"`
	SourceURL string `json:"source_url"`
}

// GetModDetails fetches full project metadata and its versions.
func (p *Provider) GetModDetails(ctx context.Context, sourceID string, params modproviders.SearchParams) (*modproviders.ModDetails, error) {
	projectEndpoint := fmt.Sprintf("%s/project/%s", p.baseURL, url.PathEscape(sourceID))

	var project modrinthProject
	errProject := p.getJSON(ctx, projectEndpoint, &project)
	if errProject != nil {
		return nil, fmt.Errorf("modrinth get project %q: %w", sourceID, errProject)
	}

	versions, errVersions := p.GetVersions(ctx, sourceID, "", params)
	if errVersions != nil {
		log.Warn().Err(errVersions).Str("sourceID", sourceID).Msg("modrinth: failed to fetch versions for mod details")
	}

	gallery := make([]string, 0, len(project.Gallery))
	for _, g := range project.Gallery {
		gallery = append(gallery, g.URL)
	}

	return &modproviders.ModDetails{
		Source:        providerID,
		SourceID:      project.Slug,
		Name:          project.Title,
		Description:   project.Description,
		Body:          project.Body,
		IconURL:       project.IconURL,
		Downloads:     project.Downloads,
		GalleryImages: gallery,
		Categories:    project.Categories,
		License:       project.License.ID,
		SourceURL:     project.SourceURL,
		Versions:      versions,
	}, nil
}

// --------------------------------------------------------------------------
// GetVersions
// --------------------------------------------------------------------------

type modrinthVersionFile struct {
	URL    string `json:"url"`
	Hashes struct {
		SHA256 string `json:"sha256"`
	} `json:"hashes"`
	Size    int64 `json:"size"`
	Primary bool  `json:"primary"`
}

type modrinthDependency struct {
	ProjectID      string `json:"project_id"`
	DependencyType string `json:"dependency_type"`
}

type modrinthVersion struct {
	ID            string                `json:"id"`
	VersionNumber string                `json:"version_number"`
	GameVersions  []string              `json:"game_versions"`
	Loaders       []string              `json:"loaders"`
	Files         []modrinthVersionFile `json:"files"`
	Dependencies  []modrinthDependency  `json:"dependencies"`
	Changelog     string                `json:"changelog"`
}

// GetVersions returns versions for the given project, optionally filtered to a specific game version.
func (p *Provider) GetVersions(ctx context.Context, sourceID string, gameVersion string, params modproviders.SearchParams) ([]modproviders.ModVersion, error) {
	queryParams := url.Values{}

	loaders := extractLoaders(params)
	if len(loaders) > 0 {
		loadersJSON, errEncode := json.Marshal(loaders)
		if errEncode != nil {
			return nil, fmt.Errorf("modrinth encode loaders: %w", errEncode)
		}
		queryParams.Set("loaders", string(loadersJSON))
	}

	if gameVersion != "" {
		gvJSON, errEncode := json.Marshal([]string{gameVersion})
		if errEncode != nil {
			return nil, fmt.Errorf("modrinth encode game_versions: %w", errEncode)
		}
		queryParams.Set("game_versions", string(gvJSON))
	}

	endpoint := fmt.Sprintf("%s/project/%s/version", p.baseURL, url.PathEscape(sourceID))
	if len(queryParams) > 0 {
		endpoint = fmt.Sprintf("%s?%s", endpoint, queryParams.Encode())
	}

	var raw []modrinthVersion
	errFetch := p.getJSON(ctx, endpoint, &raw)
	if errFetch != nil {
		return nil, fmt.Errorf("modrinth get versions %q: %w", sourceID, errFetch)
	}

	versions := make([]modproviders.ModVersion, 0, len(raw))
	for _, v := range raw {
		primaryFile := primaryFileFor(v.Files)
		deps := make([]modproviders.ModDependency, 0, len(v.Dependencies))
		for _, d := range v.Dependencies {
			deps = append(deps, modproviders.ModDependency{
				SourceID: d.ProjectID,
				Required: d.DependencyType == "required",
			})
		}
		versions = append(versions, modproviders.ModVersion{
			VersionID:      v.ID,
			VersionString:  v.VersionNumber,
			GameVersions:   v.GameVersions,
			DownloadURL:    primaryFile.URL,
			FileSize:       primaryFile.Size,
			FileHashSHA256: primaryFile.Hashes.SHA256,
			Dependencies:   deps,
			Changelog:      v.Changelog,
		})
	}
	return versions, nil
}

// --------------------------------------------------------------------------
// Download
// --------------------------------------------------------------------------

// Download fetches the primary file for the given version and writes it to targetDir.
func (p *Provider) Download(ctx context.Context, _ string, versionID string, targetDir string) ([]modproviders.DownloadedFile, error) {
	endpoint := fmt.Sprintf("%s/version/%s", p.baseURL, url.PathEscape(versionID))

	var v modrinthVersion
	errFetch := p.getJSON(ctx, endpoint, &v)
	if errFetch != nil {
		return nil, fmt.Errorf("modrinth download — fetch version %q: %w", versionID, errFetch)
	}

	primary := primaryFileFor(v.Files)
	if primary.URL == "" {
		return nil, fmt.Errorf("modrinth download: no files found for version %q", versionID)
	}
	if primary.Hashes.SHA256 == "" {
		return nil, fmt.Errorf("modrinth download: missing SHA-256 for version %q: %w", versionID, modproviders.ErrMissingIntegrityMetadata)
	}

	fileName := filepath.Base(primary.URL)
	if fileName == "" || fileName == "." {
		fileName = fmt.Sprintf("%s.jar", versionID)
	}
	destPath := filepath.Join(targetDir, fileName)

	written, hash, errDownload := providerhttp.DownloadToFile(ctx, p.httpClient, primary.URL, destPath, providerID)
	if errDownload != nil {
		return nil, fmt.Errorf("modrinth download: %w", errDownload)
	}
	if !strings.EqualFold(hash, primary.Hashes.SHA256) {
		return nil, fmt.Errorf("modrinth download: SHA-256 mismatch for %s: got %s, want %s: %w", fileName, hash, primary.Hashes.SHA256, modproviders.ErrIntegrityMismatch)
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
		return nil, fmt.Errorf("modrinth check for update %q: %w", sourceID, errVersions)
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
		return fmt.Errorf("modrinth get JSON: %w", errGet)
	}
	return nil
}

// buildFacets converts a SearchParams "facets" entry into a Modrinth facet JSON string.
//
// SearchParams may contain a "facets" key whose value is a map[string]any.  Each
// key in that map becomes a facet category and each value (string or []any/[]string)
// becomes the allowed values within that category.
//
// Modrinth's facet format is a JSON array-of-arrays where:
//   - The outer array is AND logic (all facet groups must match).
//   - The inner array is OR logic (any value in the group satisfies it).
//   - Each element is a string of the form "category:value".
//
// Example input:
//
//	{"project_type": "plugin", "categories": ["paper", "spigot"]}
//
// Produces:
//
//	[["project_type:plugin"],["categories:paper","categories:spigot"]]
func buildFacets(params modproviders.SearchParams) string {
	if params == nil {
		return ""
	}
	facetsRaw, ok := params["facets"]
	if !ok {
		return ""
	}
	facetMap, ok := facetsRaw.(map[string]any)
	if !ok {
		return ""
	}

	outer := make([][]string, 0, len(facetMap))
	for category, valRaw := range facetMap {
		var inner []string
		switch v := valRaw.(type) {
		case string:
			inner = []string{category + ":" + v}
		case []string:
			inner = make([]string, 0, len(v))
			for _, s := range v {
				inner = append(inner, category+":"+s)
			}
		case []any:
			inner = make([]string, 0, len(v))
			for _, item := range v {
				itemString, itemOK := item.(string)
				if itemOK {
					inner = append(inner, category+":"+itemString)
				}
			}
		default:
			inner = []string{fmt.Sprintf("%s:%v", category, v)}
		}
		if len(inner) > 0 {
			outer = append(outer, inner)
		}
	}

	if len(outer) == 0 {
		return ""
	}

	encoded, errEncode := json.Marshal(outer)
	if errEncode != nil {
		return ""
	}
	return string(encoded)
}

// extractLoaders pulls a list of loader strings from SearchParams for use in the
// versions endpoint.  Loaders are stored under facets["categories"].
func extractLoaders(params modproviders.SearchParams) []string {
	if params == nil {
		return nil
	}
	facetsRaw, ok := params["facets"]
	if !ok {
		return nil
	}
	facetMap, ok := facetsRaw.(map[string]any)
	if !ok {
		return nil
	}
	catRaw, ok := facetMap["categories"]
	if !ok {
		return nil
	}
	switch v := catRaw.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			itemString, itemOK := item.(string)
			if itemOK {
				result = append(result, itemString)
			}
		}
		return result
	}
	return nil
}

// mergeFacets takes existing facets JSON and merges in well-known filter params
// (game_version, categories) from SearchParams. Returns the updated facets JSON string.
func mergeFacets(existingFacets string, params modproviders.SearchParams) string {
	if params == nil {
		return existingFacets
	}

	var outer [][]string
	if existingFacets != "" {
		errDecode := json.Unmarshal([]byte(existingFacets), &outer)
		if errDecode != nil {
			outer = nil
		}
	}

	gameVersion := providerhttp.StringParam(params, modproviders.ParamGameVersion)
	if gameVersion != "" {
		outer = append(outer, []string{"versions:" + gameVersion})
	}

	categories := providerhttp.StringSliceParam(params, modproviders.ParamCategories)
	if len(categories) > 0 {
		inner := make([]string, 0, len(categories))
		for _, cat := range categories {
			inner = append(inner, "categories:"+cat)
		}
		outer = append(outer, inner)
	}

	if len(outer) == 0 {
		return ""
	}

	encoded, errEncode := json.Marshal(outer)
	if errEncode != nil {
		return existingFacets
	}
	return string(encoded)
}

// primaryFileFor returns the primary file from the list, falling back to the first
// entry if no file is explicitly marked primary.
func primaryFileFor(files []modrinthVersionFile) modrinthVersionFile {
	for _, f := range files {
		if f.Primary {
			return f
		}
	}
	if len(files) > 0 {
		return files[0]
	}
	return modrinthVersionFile{}
}
