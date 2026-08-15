// Package thunderstore implements the Thunderstore mod provider.
package thunderstore

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/modproviders/internal/providerhttp"
)

const (
	defaultBaseURL     = "https://thunderstore.io"
	userAgent          = "Xylona/1.0 (github.com/ClintonCollins/Xylona)"
	providerID         = "thunderstore"
	defaultCommunity   = "valheim"
	cyberstormPageSize = 20
	maxSearchLimit     = 100
	// maxAPIResponseBytes bounds each paginated Cyberstorm response while
	// avoiding the legacy API's unbounded full-community package feed.
	maxAPIResponseBytes = 8 << 20
)

func init() {
	modproviders.RegisterProvider(New())
}

// Provider implements modproviders.ModProvider for Thunderstore (thunderstore.io).
type Provider struct {
	httpClient *http.Client
	baseURL    string
}

// New creates a new Thunderstore provider with a default HTTP client that injects the
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

type categoryInfo struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type packagePreview struct {
	Categories          []categoryInfo `json:"categories"`
	CommunityIdentifier string         `json:"community_identifier"`
	Description         string         `json:"description"`
	DownloadCount       int64          `json:"download_count"`
	IconURL             string         `json:"icon_url"`
	IsDeprecated        bool           `json:"is_deprecated"`
	LastUpdated         string         `json:"last_updated"`
	Name                string         `json:"name"`
	Namespace           string         `json:"namespace"`
}

type packageSearchResponse struct {
	Count   int              `json:"count"`
	Next    *string          `json:"next"`
	Results []packagePreview `json:"results"`
}

type dependencyInfo struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	VersionNumber string `json:"version_number"`
}

type packageDetailsResponse struct {
	Categories          []categoryInfo   `json:"categories"`
	CommunityIdentifier string           `json:"community_identifier"`
	Dependencies        []dependencyInfo `json:"dependencies"`
	Description         string           `json:"description"`
	DownloadCount       int64            `json:"download_count"`
	DownloadURL         string           `json:"download_url"`
	IconURL             string           `json:"icon_url"`
	LatestVersionNumber string           `json:"latest_version_number"`
	Name                string           `json:"name"`
	Namespace           string           `json:"namespace"`
	Size                int64            `json:"size"`
	Team                struct {
		Name string `json:"name"`
	} `json:"team"`
}

type packageVersionResponse struct {
	VersionNumber   string `json:"version_number"`
	DateTimeCreated string `json:"datetime_created"`
	DownloadURL     string `json:"download_url"`
}

type packageReference struct {
	namespace string
	name      string
}

// Search queries Thunderstore's paginated Cyberstorm API. SearchParams["community"]
// specifies the Thunderstore community (for example, "valheim").
func (p *Provider) Search(ctx context.Context, query string, params modproviders.SearchParams) (modproviders.SearchResult, error) {
	community := communityFromParams(params)
	limit := providerhttp.IntParam(params, modproviders.ParamLimit, cyberstormPageSize)
	if limit <= 0 {
		limit = cyberstormPageSize
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}
	offset := providerhttp.IntParam(params, modproviders.ParamOffset, 0)
	offset = max(offset, 0)

	page := (offset / cyberstormPageSize) + 1
	pageOffset := offset % cyberstormPageSize
	needed := pageOffset + limit
	previews := make([]packagePreview, 0, needed)
	totalHits := 0

	for len(previews) < needed {
		endpoint := p.searchEndpoint(community, query, params, page)
		var response packageSearchResponse
		errFetch := p.getJSON(ctx, endpoint, &response)
		if errFetch != nil {
			return modproviders.SearchResult{}, fmt.Errorf("thunderstore search: %w", errFetch)
		}

		totalHits = response.Count
		previews = append(previews, response.Results...)
		if response.Next == nil || len(response.Results) == 0 {
			break
		}
		page++
	}

	if pageOffset > len(previews) {
		pageOffset = len(previews)
	}
	end := min(pageOffset+limit, len(previews))

	results := make([]modproviders.ModSearchResult, 0, end-pageOffset)
	for _, preview := range previews[pageOffset:end] {
		results = append(results, modproviders.ModSearchResult{
			Source:       providerID,
			SourceID:     sourceID(preview.Namespace, preview.Name),
			Name:         preview.Name,
			Author:       preview.Namespace,
			Description:  preview.Description,
			IconURL:      preview.IconURL,
			Downloads:    preview.DownloadCount,
			Categories:   categoryNames(preview.Categories),
			DateModified: preview.LastUpdated,
		})
	}

	return modproviders.SearchResult{Results: results, TotalHits: totalHits}, nil
}

// GetModDetails returns full details and versions for a specific mod.
// sourceID is the package full name (for example, "ValheimPlus-ValheimPlus").
func (p *Provider) GetModDetails(ctx context.Context, sourceIDValue string, params modproviders.SearchParams) (*modproviders.ModDetails, error) {
	reference, errReference := parseSourceID(sourceIDValue)
	if errReference != nil {
		return nil, fmt.Errorf("thunderstore get mod details: %w", errReference)
	}
	community := communityFromParams(params)

	details, errDetails := p.getPackageDetails(ctx, community, reference)
	if errDetails != nil {
		return nil, fmt.Errorf("thunderstore get mod details %q: %w", sourceIDValue, errDetails)
	}
	versionResponses, errVersions := p.getPackageVersions(ctx, reference)
	if errVersions != nil {
		return nil, fmt.Errorf("thunderstore get mod details %q versions: %w", sourceIDValue, errVersions)
	}
	versions := versionsFromResponses(versionResponses, &details)

	return &modproviders.ModDetails{
		Source:      providerID,
		SourceID:    sourceID(details.Namespace, details.Name),
		Name:        details.Name,
		Author:      details.Team.Name,
		Description: details.Description,
		IconURL:     details.IconURL,
		Downloads:   details.DownloadCount,
		Categories:  categoryNames(details.Categories),
		SourceURL:   p.packagePageURL(community, reference),
		Versions:    versions,
	}, nil
}

// GetVersions returns all versions for a package. Thunderstore does not expose
// game-version compatibility metadata for these entries.
func (p *Provider) GetVersions(ctx context.Context, sourceIDValue string, _ string, params modproviders.SearchParams) ([]modproviders.ModVersion, error) {
	reference, errReference := parseSourceID(sourceIDValue)
	if errReference != nil {
		return nil, fmt.Errorf("thunderstore get versions: %w", errReference)
	}

	versionResponses, errVersions := p.getPackageVersions(ctx, reference)
	if errVersions != nil {
		return nil, fmt.Errorf("thunderstore get versions %q: %w", sourceIDValue, errVersions)
	}

	community := communityFromParams(params)
	details, errDetails := p.getPackageDetails(ctx, community, reference)
	if errDetails != nil {
		return nil, fmt.Errorf("thunderstore get versions %q details: %w", sourceIDValue, errDetails)
	}
	return versionsFromResponses(versionResponses, &details), nil
}

// Download fetches the ZIP for the specified version and writes it to targetDir.
func (p *Provider) Download(ctx context.Context, sourceIDValue string, versionID string, targetDir string) ([]modproviders.DownloadedFile, error) {
	reference, errReference := parseSourceID(sourceIDValue)
	if errReference != nil {
		return nil, fmt.Errorf("thunderstore download: %w", errReference)
	}
	versionID = strings.TrimSpace(versionID)
	if !isSafeIdentifier(versionID) {
		return nil, fmt.Errorf("thunderstore download: invalid versionID %q", versionID)
	}

	downloadURL := fmt.Sprintf("%s/package/download/%s/%s/%s/",
		p.baseURL,
		url.PathEscape(reference.namespace),
		url.PathEscape(reference.name),
		url.PathEscape(versionID),
	)
	fileName := fmt.Sprintf("%s-%s.zip", sourceID(reference.namespace, reference.name), versionID)
	destPath := filepath.Join(targetDir, fileName)

	written, hash, errDownload := providerhttp.DownloadToFile(ctx, p.httpClient, downloadURL, destPath, providerID)
	if errDownload != nil {
		return nil, fmt.Errorf("thunderstore download: %w", errDownload)
	}

	return []modproviders.DownloadedFile{{
		Path:      fileName,
		Hash:      hash,
		Size:      written,
		IsPrimary: true,
	}}, nil
}

// CheckForUpdate returns the newest version for the given package.
func (p *Provider) CheckForUpdate(ctx context.Context, sourceIDValue string, _ string) (*modproviders.ModVersion, error) {
	reference, errReference := parseSourceID(sourceIDValue)
	if errReference != nil {
		return nil, fmt.Errorf("thunderstore check for update: %w", errReference)
	}
	versionResponses, errVersions := p.getPackageVersions(ctx, reference)
	if errVersions != nil {
		return nil, fmt.Errorf("thunderstore check for update %q: %w", sourceIDValue, errVersions)
	}
	versions := versionsFromResponses(versionResponses, nil)
	if len(versions) == 0 {
		return nil, modproviders.ErrNoUpdateAvailable
	}
	version := versions[0]
	return &version, nil
}

func (p *Provider) searchEndpoint(community string, query string, params modproviders.SearchParams, page int) string {
	values := url.Values{}
	if strings.TrimSpace(query) != "" {
		values.Set("q", strings.TrimSpace(query))
	}
	values.Set("page", fmt.Sprintf("%d", page))
	values.Set("ordering", thunderstoreOrdering(params))
	for _, category := range providerhttp.StringSliceParam(params, modproviders.ParamCategories) {
		values.Add("included_categories", category)
	}
	return fmt.Sprintf("%s/api/cyberstorm/listing/%s/?%s", p.baseURL, url.PathEscape(community), values.Encode())
}

func (p *Provider) getPackageDetails(ctx context.Context, community string, reference packageReference) (packageDetailsResponse, error) {
	endpoint := fmt.Sprintf("%s/api/cyberstorm/listing/%s/%s/%s/",
		p.baseURL,
		url.PathEscape(community),
		url.PathEscape(reference.namespace),
		url.PathEscape(reference.name),
	)
	var details packageDetailsResponse
	errFetch := p.getJSON(ctx, endpoint, &details)
	if errFetch != nil {
		return packageDetailsResponse{}, errFetch
	}
	return details, nil
}

func (p *Provider) getPackageVersions(ctx context.Context, reference packageReference) ([]packageVersionResponse, error) {
	endpoint := fmt.Sprintf("%s/api/cyberstorm/package/%s/%s/versions/",
		p.baseURL,
		url.PathEscape(reference.namespace),
		url.PathEscape(reference.name),
	)
	versions := make([]packageVersionResponse, 0)
	errFetch := p.getJSON(ctx, endpoint, &versions)
	if errFetch != nil {
		return nil, errFetch
	}
	return versions, nil
}

func (p *Provider) packagePageURL(community string, reference packageReference) string {
	return fmt.Sprintf("%s/c/%s/p/%s/%s/",
		p.baseURL,
		url.PathEscape(community),
		url.PathEscape(reference.namespace),
		url.PathEscape(reference.name),
	)
}

func (p *Provider) getJSON(ctx context.Context, endpoint string, dest any) error {
	errGet := providerhttp.GetJSONLimited(ctx, p.httpClient, endpoint, dest, providerID, maxAPIResponseBytes)
	if errGet != nil {
		return fmt.Errorf("thunderstore get JSON: %w", errGet)
	}
	return nil
}

func versionsFromResponses(responses []packageVersionResponse, details *packageDetailsResponse) []modproviders.ModVersion {
	sort.SliceStable(responses, func(i int, j int) bool {
		return responses[i].DateTimeCreated > responses[j].DateTimeCreated
	})

	versions := make([]modproviders.ModVersion, 0, len(responses))
	for _, response := range responses {
		version := modproviders.ModVersion{
			VersionID:     response.VersionNumber,
			VersionString: response.VersionNumber,
			DownloadURL:   response.DownloadURL,
		}
		if details != nil && response.VersionNumber == details.LatestVersionNumber {
			version.FileSize = details.Size
			version.Dependencies = dependenciesFromDetails(details.Dependencies)
		}
		versions = append(versions, version)
	}
	return versions
}

func dependenciesFromDetails(dependencies []dependencyInfo) []modproviders.ModDependency {
	result := make([]modproviders.ModDependency, 0, len(dependencies))
	for _, dependency := range dependencies {
		result = append(result, modproviders.ModDependency{
			SourceID: sourceID(dependency.Namespace, dependency.Name) + "-" + dependency.VersionNumber,
			Name:     dependency.Name,
			Required: true,
		})
	}
	return result
}

func categoryNames(categories []categoryInfo) []string {
	result := make([]string, 0, len(categories))
	for _, category := range categories {
		result = append(result, category.Name)
	}
	return result
}

func sourceID(namespace string, name string) string {
	return namespace + "-" + name
}

func parseSourceID(value string) (packageReference, error) {
	parts := strings.SplitN(strings.TrimSpace(value), "-", 2)
	if len(parts) != 2 {
		return packageReference{}, fmt.Errorf("invalid sourceID %q", value)
	}
	namespace := strings.TrimSpace(parts[0])
	name := strings.TrimSpace(parts[1])
	if !isSafeIdentifier(namespace) || !isSafeIdentifier(name) {
		return packageReference{}, fmt.Errorf("invalid sourceID %q", value)
	}
	return packageReference{namespace: namespace, name: name}, nil
}

func isSafeIdentifier(value string) bool {
	return value != "" &&
		value != "." &&
		value != ".." &&
		!strings.ContainsAny(value, `/\:`) &&
		!strings.ContainsRune(value, '\x00')
}

func communityFromParams(params modproviders.SearchParams) string {
	community := extractCommunity(params)
	if community == "" {
		return defaultCommunity
	}
	return community
}

func thunderstoreOrdering(params modproviders.SearchParams) string {
	switch providerhttp.StringParam(params, modproviders.ParamSortBy) {
	case "downloads":
		return "most-downloaded"
	case "newest":
		return "newest"
	case "updated":
		return "last-updated"
	default:
		return "last-updated"
	}
}

// extractCommunity pulls the community string from SearchParams["community"].
func extractCommunity(params modproviders.SearchParams) string {
	if params == nil {
		return ""
	}
	value, ok := params["community"]
	if !ok {
		return ""
	}
	community, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(community)
}
