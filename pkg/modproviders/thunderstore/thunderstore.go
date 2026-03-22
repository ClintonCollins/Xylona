package thunderstore

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
)

const (
	defaultBaseURL = "https://thunderstore.io"
	userAgent      = "Xylona/1.0 (github.com/ClintonCollins/Xylona)"
	providerID     = "thunderstore"
	cacheTTL       = 5 * time.Minute
)

func init() {
	modproviders.RegisterProvider(New())
}

// cachedPackages holds a fetched package list with its fetch timestamp.
type cachedPackages struct {
	packages  []packageInfo
	fetchedAt time.Time
}

// Provider implements modproviders.ModProvider for Thunderstore (thunderstore.io).
type Provider struct {
	httpClient *http.Client
	baseURL    string
	mu         sync.Mutex
	cache      map[string]cachedPackages // keyed by community slug
}

// New creates a new Thunderstore provider with a default HTTP client that injects the
// required User-Agent header on every request.
func New() *Provider {
	return &Provider{
		httpClient: &http.Client{
			Transport: &userAgentTransport{wrapped: http.DefaultTransport},
		},
		baseURL: defaultBaseURL,
		cache:   make(map[string]cachedPackages),
	}
}

// userAgentTransport wraps an http.RoundTripper and injects the Xylona User-Agent.
type userAgentTransport struct {
	wrapped http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header.Set("User-Agent", userAgent)
	resp, errRT := t.wrapped.RoundTrip(cloned)
	if errRT != nil {
		return nil, errRT
	}
	return resp, nil
}

// ID returns the provider identifier.
func (p *Provider) ID() string {
	return providerID
}

// RequiresAPIKey returns false — Thunderstore read API is public.
func (p *Provider) RequiresAPIKey() bool {
	return false
}

// --------------------------------------------------------------------------
// JSON types
// --------------------------------------------------------------------------

// packageVersionInfo represents a single version within a package's versions array.
type packageVersionInfo struct {
	Name          string   `json:"name"`
	FullName      string   `json:"full_name"`
	Description   string   `json:"description"`
	Icon          string   `json:"icon"`
	VersionNumber string   `json:"version_number"`
	Dependencies  []string `json:"dependencies"`
	DownloadURL   string   `json:"download_url"`
	Downloads     int64    `json:"downloads"`
	DateCreated   string   `json:"date_created"`
	FileSize      int64    `json:"file_size"`
}

// packageInfo represents a single package in the community package list response.
type packageInfo struct {
	Name             string               `json:"name"`
	FullName         string               `json:"full_name"`
	Owner            string               `json:"owner"`
	PackageURL       string               `json:"package_url"`
	DateCreated      string               `json:"date_created"`
	DateUpdated      string               `json:"date_updated"`
	RatingScore      int                  `json:"rating_score"`
	IsPinned         bool                 `json:"is_pinned"`
	IsDeprecated     bool                 `json:"is_deprecated"`
	TotalDownloads   int64                `json:"total_downloads"`
	Latest           packageVersionInfo   `json:"latest"`
	Versions         []packageVersionInfo `json:"versions"`
}

// --------------------------------------------------------------------------
// Cache
// --------------------------------------------------------------------------

// getPackageList returns the cached package list for a community, fetching it if
// the cache is missing or stale (older than cacheTTL).
func (p *Provider) getPackageList(ctx context.Context, community string) ([]packageInfo, error) {
	p.mu.Lock()
	cached, ok := p.cache[community]
	if ok && time.Since(cached.fetchedAt) < cacheTTL {
		packages := cached.packages
		p.mu.Unlock()
		return packages, nil
	}
	p.mu.Unlock()

	endpoint := fmt.Sprintf("%s/c/%s/api/v1/package/", p.baseURL, community)

	var packages []packageInfo
	errFetch := p.getJSON(ctx, endpoint, &packages)
	if errFetch != nil {
		return nil, fmt.Errorf("thunderstore fetch package list for community %q: %w", community, errFetch)
	}

	p.mu.Lock()
	p.cache[community] = cachedPackages{
		packages:  packages,
		fetchedAt: time.Now(),
	}
	p.mu.Unlock()

	return packages, nil
}

// --------------------------------------------------------------------------
// Search
// --------------------------------------------------------------------------

// Search fetches the full community package list and filters in-memory by query.
// SearchParams["community"] specifies the Thunderstore community (e.g., "valheim").
func (p *Provider) Search(ctx context.Context, query string, params modproviders.SearchParams) ([]modproviders.ModSearchResult, error) {
	community := extractCommunity(params)
	if community == "" {
		community = "valheim"
	}

	packages, errList := p.getPackageList(ctx, community)
	if errList != nil {
		return nil, fmt.Errorf("thunderstore search: %w", errList)
	}

	queryLower := strings.ToLower(query)
	results := make([]modproviders.ModSearchResult, 0)

	for _, pkg := range packages {
		if pkg.IsDeprecated {
			continue
		}
		if query != "" {
			nameLower := strings.ToLower(pkg.Name)
			descLower := strings.ToLower(pkg.Latest.Description)
			if !strings.Contains(nameLower, queryLower) && !strings.Contains(descLower, queryLower) {
				continue
			}
		}

		versions := make([]string, 0, len(pkg.Versions))
		for _, v := range pkg.Versions {
			versions = append(versions, v.VersionNumber)
		}

		latestVersion := ""
		if len(pkg.Versions) > 0 {
			latestVersion = pkg.Versions[0].VersionNumber
		}

		results = append(results, modproviders.ModSearchResult{
			Source:             providerID,
			SourceID:           pkg.FullName,
			Name:               pkg.Name,
			Author:             pkg.Owner,
			Description:        pkg.Latest.Description,
			IconURL:            pkg.Latest.Icon,
			Downloads:          pkg.TotalDownloads,
			LatestVersion:      latestVersion,
			CompatibleVersions: versions,
		})
	}

	return results, nil
}

// --------------------------------------------------------------------------
// GetModDetails
// --------------------------------------------------------------------------

// GetModDetails returns full details and versions for a specific mod.
// sourceID is the full_name (e.g., "ValheimPlus-ValheimPlus").
func (p *Provider) GetModDetails(ctx context.Context, sourceID string, params modproviders.SearchParams) (*modproviders.ModDetails, error) {
	community := extractCommunity(params)
	if community == "" {
		community = "valheim"
	}

	packages, errList := p.getPackageList(ctx, community)
	if errList != nil {
		return nil, fmt.Errorf("thunderstore get mod details %q: %w", sourceID, errList)
	}

	var found *packageInfo
	for i := range packages {
		if packages[i].FullName == sourceID {
			found = &packages[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("thunderstore get mod details: package %q not found in community %q", sourceID, community)
	}

	versions, errVersions := p.versionsFromPackage(found)
	if errVersions != nil {
		log.Warn().Err(errVersions).Str("sourceID", sourceID).Msg("thunderstore: failed to map versions for mod details")
	}

	return &modproviders.ModDetails{
		Source:      providerID,
		SourceID:    found.FullName,
		Name:        found.Name,
		Author:      found.Owner,
		Description: found.Latest.Description,
		IconURL:     found.Latest.Icon,
		Downloads:   found.TotalDownloads,
		SourceURL:   found.PackageURL,
		Versions:    versions,
	}, nil
}

// --------------------------------------------------------------------------
// GetVersions
// --------------------------------------------------------------------------

// GetVersions returns all versions for a package.
// sourceID is the full_name (e.g., "ValheimPlus-ValheimPlus").
// gameVersion is ignored for Thunderstore as versions carry no game version metadata.
func (p *Provider) GetVersions(ctx context.Context, sourceID string, gameVersion string, params modproviders.SearchParams) ([]modproviders.ModVersion, error) {
	community := extractCommunity(params)
	if community == "" {
		community = "valheim"
	}

	packages, errList := p.getPackageList(ctx, community)
	if errList != nil {
		return nil, fmt.Errorf("thunderstore get versions %q: %w", sourceID, errList)
	}

	var found *packageInfo
	for i := range packages {
		if packages[i].FullName == sourceID {
			found = &packages[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("thunderstore get versions: package %q not found in community %q", sourceID, community)
	}

	versions, errVersions := p.versionsFromPackage(found)
	if errVersions != nil {
		return nil, fmt.Errorf("thunderstore get versions %q: %w", sourceID, errVersions)
	}
	return versions, nil
}

// --------------------------------------------------------------------------
// Download
// --------------------------------------------------------------------------

// Download fetches the ZIP for the specified version and writes it to targetDir.
// versionID is the version_number string (e.g., "0.9.12.0").
func (p *Provider) Download(ctx context.Context, sourceID string, versionID string, targetDir string) ([]modproviders.DownloadedFile, error) {
	community := extractCommunity(nil)
	if community == "" {
		community = "valheim"
	}

	packages, errList := p.getPackageList(ctx, community)
	if errList != nil {
		return nil, fmt.Errorf("thunderstore download %q: %w", sourceID, errList)
	}

	var found *packageInfo
	for i := range packages {
		if packages[i].FullName == sourceID {
			found = &packages[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("thunderstore download: package %q not found", sourceID)
	}

	downloadURL := ""
	for _, v := range found.Versions {
		if v.VersionNumber == versionID {
			downloadURL = v.DownloadURL
			break
		}
	}
	if downloadURL == "" {
		return nil, fmt.Errorf("thunderstore download: version %q not found for package %q", versionID, sourceID)
	}

	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if errReq != nil {
		return nil, fmt.Errorf("thunderstore download: build request: %w", errReq)
	}

	resp, errGet := p.httpClient.Do(req)
	if errGet != nil {
		return nil, fmt.Errorf("thunderstore download: GET %s: %w", downloadURL, errGet)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Warn().Err(errClose).Msg("thunderstore: failed to close download response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("thunderstore download: unexpected status %d for %s", resp.StatusCode, downloadURL)
	}

	fileName := fmt.Sprintf("%s-%s.zip", sourceID, versionID)
	destPath := filepath.Join(targetDir, fileName)

	outFile, errCreate := os.Create(destPath)
	if errCreate != nil {
		return nil, fmt.Errorf("thunderstore download: create file %s: %w", destPath, errCreate)
	}
	defer func() {
		if errClose := outFile.Close(); errClose != nil {
			log.Warn().Err(errClose).Str("path", destPath).Msg("thunderstore: failed to close output file")
		}
	}()

	hasher := sha256.New()
	writer := io.MultiWriter(outFile, hasher)

	limitedBody := io.LimitReader(resp.Body, modproviders.MaxModDownloadSize+1)
	written, errCopy := io.Copy(writer, limitedBody)
	if errCopy != nil {
		return nil, fmt.Errorf("thunderstore download: write file %s: %w", destPath, errCopy)
	}
	if written > modproviders.MaxModDownloadSize {
		return nil, fmt.Errorf("thunderstore download: file %s (%d bytes): %w", destPath, written, modproviders.ErrDownloadTooLarge)
	}

	hash := fmt.Sprintf("%x", hasher.Sum(nil))

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

// CheckForUpdate returns the latest version for the given package.
// gameVersion is ignored since Thunderstore carries no per-version game compatibility metadata.
func (p *Provider) CheckForUpdate(ctx context.Context, sourceID string, gameVersion string) (*modproviders.ModVersion, error) {
	versions, errVersions := p.GetVersions(ctx, sourceID, gameVersion, nil)
	if errVersions != nil {
		return nil, fmt.Errorf("thunderstore check for update %q: %w", sourceID, errVersions)
	}
	if len(versions) == 0 {
		return nil, nil
	}
	v := versions[0]
	return &v, nil
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

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
			log.Warn().Err(errClose).Str("url", endpoint).Msg("thunderstore: failed to close response body")
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

// versionsFromPackage maps the embedded versions array of a packageInfo into ModVersion entries.
func (p *Provider) versionsFromPackage(pkg *packageInfo) ([]modproviders.ModVersion, error) {
	versions := make([]modproviders.ModVersion, 0, len(pkg.Versions))
	for _, v := range pkg.Versions {
		deps := make([]modproviders.ModDependency, 0, len(v.Dependencies))
		for _, dep := range v.Dependencies {
			deps = append(deps, modproviders.ModDependency{
				SourceID: dep,
				Required: true,
			})
		}
		versions = append(versions, modproviders.ModVersion{
			VersionID:     v.VersionNumber,
			VersionString: v.VersionNumber,
			DownloadURL:   v.DownloadURL,
			FileSize:      v.FileSize,
			Dependencies:  deps,
		})
	}
	return versions, nil
}

// extractCommunity pulls the community string from SearchParams["community"].
func extractCommunity(params modproviders.SearchParams) string {
	if params == nil {
		return ""
	}
	v, ok := params["community"]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
