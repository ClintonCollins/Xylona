package papermc

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
)

const (
	defaultBaseURL = "https://api.papermc.io/v2"
	userAgent      = "Xylona/1.0 (github.com/ClintonCollins/Xylona)"
	providerID     = "papermc"
)

// alternateBaseURLs maps project IDs that use the PaperMC API format but are
// hosted on different domains (e.g., PurpurMC).
var alternateBaseURLs = map[string]string{
	"purpur": "https://api.purpurmc.org/v2",
}

func init() {
	modproviders.RegisterProvider(New())
}

// Provider implements modproviders.ModProvider for PaperMC (api.papermc.io/v2).
// It covers Paper, Folia, Velocity, and Waterfall server software.
type Provider struct {
	httpClient *http.Client
	baseURL    string
}

// New creates a new PaperMC provider with a default HTTP client that injects the
// required User-Agent header on every request.
func New() *Provider {
	return &Provider{
		httpClient: &http.Client{
			Transport: &userAgentTransport{wrapped: http.DefaultTransport},
		},
		baseURL: defaultBaseURL,
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

// baseURLFor returns the API base URL for the given project.
// Projects like "purpur" use a compatible API at a different host.
func (p *Provider) baseURLFor(sourceID string) string {
	if alt, ok := alternateBaseURLs[sourceID]; ok {
		return alt
	}
	return p.baseURL
}

// RequiresAPIKey returns false — the PaperMC API is public.
func (p *Provider) RequiresAPIKey() bool {
	return false
}

// --------------------------------------------------------------------------
// API response types
// --------------------------------------------------------------------------

type projectsListResponse struct {
	Projects []string `json:"projects"`
}

type projectVersionsResponse struct {
	ProjectID string   `json:"project_id"`
	Versions  []string `json:"versions"`
}

type buildsResponse struct {
	Builds []buildEntry `json:"builds"`
}

type buildEntry struct {
	Build   int    `json:"build"`
	Time    string `json:"time"`
	Channel string `json:"channel"`
	Changes []struct {
		Summary string `json:"summary"`
	} `json:"changes"`
	Downloads struct {
		Application struct {
			Name   string `json:"name"`
			SHA256 string `json:"sha256"`
		} `json:"application"`
	} `json:"downloads"`
}

// --------------------------------------------------------------------------
// Search
// --------------------------------------------------------------------------

// Search returns the list of available PaperMC projects (paper, folia, velocity,
// waterfall) as search results. The query parameter is ignored because PaperMC
// has no freetext search endpoint.
func (p *Provider) Search(ctx context.Context, _ string, _ modproviders.SearchParams) ([]modproviders.ModSearchResult, error) {
	endpoint := fmt.Sprintf("%s/projects", p.baseURL)

	var resp projectsListResponse
	errFetch := p.getJSON(ctx, endpoint, &resp)
	if errFetch != nil {
		return nil, fmt.Errorf("papermc search: %w", errFetch)
	}

	results := make([]modproviders.ModSearchResult, 0, len(resp.Projects))
	for _, project := range resp.Projects {
		results = append(results, modproviders.ModSearchResult{
			Source:      providerID,
			SourceID:    project,
			Name:        titleCase(project),
			Description: fmt.Sprintf("PaperMC project: %s", project),
		})
	}
	return results, nil
}

// --------------------------------------------------------------------------
// GetModDetails
// --------------------------------------------------------------------------

// GetModDetails returns details about a PaperMC project, including its available
// versions. sourceID is the project name (e.g., "paper", "folia").
func (p *Provider) GetModDetails(ctx context.Context, sourceID string, params modproviders.SearchParams) (*modproviders.ModDetails, error) {
	endpoint := fmt.Sprintf("%s/projects/%s", p.baseURLFor(sourceID), sourceID)

	var projectResp projectVersionsResponse
	errFetch := p.getJSON(ctx, endpoint, &projectResp)
	if errFetch != nil {
		return nil, fmt.Errorf("papermc get project %q: %w", sourceID, errFetch)
	}

	// Build ModVersion entries for each game version (not individual builds).
	// This lets callers like GetServerSoftwareVersions list available game versions.
	// Reverse the order so newest versions appear first (API returns oldest first).
	gameVersionEntries := make([]modproviders.ModVersion, 0, len(projectResp.Versions))
	for i := len(projectResp.Versions) - 1; i >= 0; i-- {
		gv := projectResp.Versions[i]
		gameVersionEntries = append(gameVersionEntries, modproviders.ModVersion{
			VersionID:     gv,
			VersionString: gv,
			GameVersions:  []string{gv},
		})
	}

	return &modproviders.ModDetails{
		Source:      providerID,
		SourceID:    sourceID,
		Name:        titleCase(sourceID),
		Description: fmt.Sprintf("PaperMC project: %s", sourceID),
		Versions:    gameVersionEntries,
	}, nil
}

// --------------------------------------------------------------------------
// GetVersions
// --------------------------------------------------------------------------

// GetVersions returns the builds for a PaperMC project at the given game version.
// sourceID is the project name and gameVersion is the Minecraft/server version
// string (e.g., "1.21.4"). Each build is returned as a ModVersion where
// VersionID is "{version}-{build}" and VersionString is "Build {build}".
func (p *Provider) GetVersions(ctx context.Context, sourceID string, gameVersion string, _ modproviders.SearchParams) ([]modproviders.ModVersion, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/versions/%s/builds", p.baseURLFor(sourceID), sourceID, gameVersion)

	var resp buildsResponse
	errFetch := p.getJSON(ctx, endpoint, &resp)
	if errFetch != nil {
		return nil, fmt.Errorf("papermc get versions %q %q: %w", sourceID, gameVersion, errFetch)
	}

	versions := make([]modproviders.ModVersion, 0, len(resp.Builds))
	for _, b := range resp.Builds {
		versionID := fmt.Sprintf("%s-%d", gameVersion, b.Build)

		var changelog string
		if len(b.Changes) > 0 {
			summaries := make([]string, 0, len(b.Changes))
			for _, c := range b.Changes {
				summaries = append(summaries, c.Summary)
			}
			changelog = strings.Join(summaries, "\n")
		}

		versions = append(versions, modproviders.ModVersion{
			VersionID:     versionID,
			VersionString: fmt.Sprintf("Build %d", b.Build),
			GameVersions:  []string{gameVersion},
			Changelog:     changelog,
		})
	}
	return versions, nil
}

// --------------------------------------------------------------------------
// Download
// --------------------------------------------------------------------------

// Download fetches the application JAR for the given versionID and writes it to
// targetDir. versionID must be in the format "{version}-{build}" (e.g.,
// "1.21.4-100"). The filename is resolved from the build metadata.
func (p *Provider) Download(ctx context.Context, sourceID string, versionID string, targetDir string) ([]modproviders.DownloadedFile, error) {
	version, buildNum, errParse := parseVersionID(versionID)
	if errParse != nil {
		return nil, fmt.Errorf("papermc download: %w", errParse)
	}

	// Fetch build info to get the filename.
	buildsEndpoint := fmt.Sprintf("%s/projects/%s/versions/%s/builds", p.baseURLFor(sourceID), sourceID, version)
	var resp buildsResponse
	errFetch := p.getJSON(ctx, buildsEndpoint, &resp)
	if errFetch != nil {
		return nil, fmt.Errorf("papermc download — fetch builds for %q %q: %w", sourceID, version, errFetch)
	}

	var targetBuild *buildEntry
	for i := range resp.Builds {
		if resp.Builds[i].Build == buildNum {
			targetBuild = &resp.Builds[i]
			break
		}
	}
	if targetBuild == nil {
		return nil, fmt.Errorf("papermc download: build %d not found for %s %s", buildNum, sourceID, version)
	}

	fileName := targetBuild.Downloads.Application.Name
	if fileName == "" {
		fileName = fmt.Sprintf("%s-%s-%d.jar", sourceID, version, buildNum)
	}
	expectedSHA256 := targetBuild.Downloads.Application.SHA256

	downloadURL := fmt.Sprintf("%s/projects/%s/versions/%s/builds/%d/downloads/%s",
		p.baseURLFor(sourceID), sourceID, version, buildNum, fileName)

	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if errReq != nil {
		return nil, fmt.Errorf("papermc download: build request: %w", errReq)
	}

	dlResp, errGet := p.httpClient.Do(req)
	if errGet != nil {
		return nil, fmt.Errorf("papermc download: GET %s: %w", downloadURL, errGet)
	}
	defer func() {
		if errClose := dlResp.Body.Close(); errClose != nil {
			log.Warn().Err(errClose).Msg("papermc: failed to close download response body")
		}
	}()

	if dlResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("papermc download: unexpected status %d for %s", dlResp.StatusCode, downloadURL)
	}

	destPath := filepath.Join(targetDir, fileName)
	outFile, errCreate := os.Create(destPath)
	if errCreate != nil {
		return nil, fmt.Errorf("papermc download: create file %s: %w", destPath, errCreate)
	}
	defer func() {
		if errClose := outFile.Close(); errClose != nil {
			log.Warn().Err(errClose).Str("path", destPath).Msg("papermc: failed to close output file")
		}
	}()

	hasher := sha256.New()
	writer := io.MultiWriter(outFile, hasher)

	limitedBody := io.LimitReader(dlResp.Body, modproviders.MaxModDownloadSize+1)
	written, errCopy := io.Copy(writer, limitedBody)
	if errCopy != nil {
		return nil, fmt.Errorf("papermc download: write file %s: %w", destPath, errCopy)
	}
	if written > modproviders.MaxModDownloadSize {
		return nil, fmt.Errorf("papermc download: file %s (%d bytes): %w", destPath, written, modproviders.ErrDownloadTooLarge)
	}

	hash := fmt.Sprintf("%x", hasher.Sum(nil))

	if expectedSHA256 != "" && hash != expectedSHA256 {
		return nil, fmt.Errorf("papermc download: SHA-256 mismatch for %s: got %s, want %s", fileName, hash, expectedSHA256)
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

// CheckForUpdate returns the latest build for the given project and game version.
func (p *Provider) CheckForUpdate(ctx context.Context, sourceID string, gameVersion string) (*modproviders.ModVersion, error) {
	versions, errVersions := p.GetVersions(ctx, sourceID, gameVersion, nil)
	if errVersions != nil {
		return nil, fmt.Errorf("papermc check for update %q: %w", sourceID, errVersions)
	}
	if len(versions) == 0 {
		return nil, nil
	}
	// Builds are returned in ascending order; the last one is the latest.
	v := versions[len(versions)-1]
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
			log.Warn().Err(errClose).Str("url", endpoint).Msg("papermc: failed to close response body")
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

// titleCase returns the string with its first letter uppercased. It is used for
// display names of project IDs (e.g., "paper" → "Paper"). Unlike strings.Title,
// it only operates on ASCII and does not capitalize after word boundaries, which
// is the correct behavior for project identifiers.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 'a' - 'A'
	}
	return string(b)
}

// parseVersionID splits a versionID of the form "{version}-{build}" (e.g.,
// "1.21.4-100") into its version string and integer build number.
// The version part may itself contain hyphens (e.g., "1.21.4-pre1-100").
func parseVersionID(versionID string) (string, int, error) {
	lastDash := strings.LastIndex(versionID, "-")
	if lastDash < 0 {
		return "", 0, fmt.Errorf("invalid versionID %q: expected format {version}-{build}", versionID)
	}
	version := versionID[:lastDash]
	buildStr := versionID[lastDash+1:]
	buildNum, errParse := strconv.Atoi(buildStr)
	if errParse != nil {
		return "", 0, fmt.Errorf("invalid versionID %q: build number %q is not an integer", versionID, buildStr)
	}
	return version, buildNum, nil
}
