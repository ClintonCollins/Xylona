// Package papermc implements the PaperMC-compatible mod provider.
package papermc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/modproviders/internal/providerhttp"
)

const (
	defaultBaseURL = "https://fill.papermc.io/v3"
	userAgent      = "Xylona/1.0 (github.com/ClintonCollins/Xylona)"
	providerID     = "papermc"
	serverDownload = "server:default"
)

// alternateBaseURLs maps project IDs that use the PaperMC API format but are
// hosted on different domains (e.g., PurpurMC).
var alternateBaseURLs = map[string]string{
	"purpur": "https://api.purpurmc.org/v2",
}

func init() {
	modproviders.RegisterProvider(New())
}

// Provider implements modproviders.ModProvider for PaperMC's downloads service.
// It covers Paper, Folia, Velocity, and Waterfall server software.
type Provider struct {
	httpClient *http.Client
	baseURL    string
}

// New creates a new PaperMC provider with a default HTTP client that injects the
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

// baseURLFor returns the API base URL for the given project.
// Projects like "purpur" use a compatible API at a different host.
func (p *Provider) baseURLFor(sourceID string) string {
	if alt, ok := alternateBaseURLs[sourceID]; ok {
		return alt
	}
	return p.baseURL
}

// --------------------------------------------------------------------------
// API response types
// --------------------------------------------------------------------------

type projectIdentity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type projectsListResponse struct {
	Projects []projectIdentity
}

type projectVersionsResponse struct {
	ProjectID   string
	ProjectName string
	Versions    []string
	NewestFirst bool
}

type buildsResponse struct {
	Builds []buildEntry
}

type buildEntry struct {
	Build       int
	Time        string
	Channel     string
	Changes     []string
	FileName    string
	SHA256      string
	FileSize    int64
	DownloadURL string
}

// --------------------------------------------------------------------------
// Search
// --------------------------------------------------------------------------

// Search returns the list of available PaperMC projects (paper, folia, velocity,
// waterfall) as search results. The query parameter is ignored because PaperMC
// has no freetext search endpoint.
func (p *Provider) Search(ctx context.Context, _ string, _ modproviders.SearchParams) (modproviders.SearchResult, error) {
	endpoint := fmt.Sprintf("%s/projects", p.baseURL)

	var resp projectsListResponse
	errFetch := p.getJSON(ctx, endpoint, &resp)
	if errFetch != nil {
		return modproviders.SearchResult{}, fmt.Errorf("papermc search: %w", errFetch)
	}

	results := make([]modproviders.ModSearchResult, 0, len(resp.Projects))
	for _, project := range resp.Projects {
		name := project.Name
		if name == "" {
			name = titleCase(project.ID)
		}
		results = append(results, modproviders.ModSearchResult{
			Source:      providerID,
			SourceID:    project.ID,
			Name:        name,
			Description: fmt.Sprintf("PaperMC project: %s", project.ID),
		})
	}
	return modproviders.SearchResult{
		Results:   results,
		TotalHits: modproviders.UnknownTotalHits,
	}, nil
}

// --------------------------------------------------------------------------
// GetModDetails
// --------------------------------------------------------------------------

// GetModDetails returns details about a PaperMC project, including its available
// versions. sourceID is the project name (e.g., "paper", "folia").
func (p *Provider) GetModDetails(ctx context.Context, sourceID string, _ modproviders.SearchParams) (*modproviders.ModDetails, error) {
	endpoint := fmt.Sprintf("%s/projects/%s", p.baseURLFor(sourceID), sourceID)

	var projectResp projectVersionsResponse
	errFetch := p.getJSON(ctx, endpoint, &projectResp)
	if errFetch != nil {
		return nil, fmt.Errorf("papermc get project %q: %w", sourceID, errFetch)
	}

	// Build ModVersion entries for each game version (not individual builds).
	// This lets callers like GetServerSoftwareVersions list available game versions.
	gameVersionEntries := make([]modproviders.ModVersion, 0, len(projectResp.Versions))
	for i := range projectResp.Versions {
		versionIndex := i
		if !projectResp.NewestFirst {
			versionIndex = len(projectResp.Versions) - 1 - i
		}
		gv := projectResp.Versions[versionIndex]
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
		fileName := b.FileName
		if fileName == "" {
			fileName = fmt.Sprintf("%s-%s-%d.jar", sourceID, gameVersion, b.Build)
		}
		downloadURL := b.DownloadURL
		if downloadURL == "" {
			downloadURL = fmt.Sprintf("%s/projects/%s/versions/%s/builds/%d/downloads/%s",
				p.baseURLFor(sourceID), sourceID, gameVersion, b.Build, fileName)
		}

		changelog := strings.Join(b.Changes, "\n")

		versions = append(versions, modproviders.ModVersion{
			VersionID:      versionID,
			VersionString:  fmt.Sprintf("Build %d", b.Build),
			GameVersions:   []string{gameVersion},
			DownloadURL:    downloadURL,
			FileSize:       b.FileSize,
			FileHashSHA256: b.SHA256,
			Changelog:      changelog,
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

	fileName := targetBuild.FileName
	if fileName == "" {
		fileName = fmt.Sprintf("%s-%s-%d.jar", sourceID, version, buildNum)
	}
	expectedSHA256 := targetBuild.SHA256

	downloadURL := targetBuild.DownloadURL
	if downloadURL == "" {
		downloadURL = fmt.Sprintf("%s/projects/%s/versions/%s/builds/%d/downloads/%s",
			p.baseURLFor(sourceID), sourceID, version, buildNum, fileName)
	}

	destPath := filepath.Join(targetDir, fileName)
	written, hash, errDownload := providerhttp.DownloadToFile(ctx, p.httpClient, downloadURL, destPath, providerID)
	if errDownload != nil {
		return nil, fmt.Errorf("papermc download: %w", errDownload)
	}

	if expectedSHA256 == "" {
		return nil, fmt.Errorf("papermc download: missing SHA-256 for %s: %w", fileName, modproviders.ErrMissingIntegrityMetadata)
	}
	if !strings.EqualFold(hash, expectedSHA256) {
		return nil, fmt.Errorf("papermc download: SHA-256 mismatch for %s: got %s, want %s: %w", fileName, hash, expectedSHA256, modproviders.ErrIntegrityMismatch)
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
		return nil, modproviders.ErrNoUpdateAvailable
	}
	latestIndex := 0
	latestBuild := -1
	for i := range versions {
		_, build, errParse := parseVersionID(versions[i].VersionID)
		if errParse == nil && build > latestBuild {
			latestBuild = build
			latestIndex = i
		}
	}
	v := versions[latestIndex]
	return &v, nil
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

// getJSON performs a GET request to the given URL and decodes the JSON body into dest.
func (p *Provider) getJSON(ctx context.Context, endpoint string, dest any) error {
	errGet := providerhttp.GetJSON(ctx, p.httpClient, endpoint, dest, providerID)
	if errGet != nil {
		return fmt.Errorf("papermc get JSON: %w", errGet)
	}
	return nil
}

func (r *projectsListResponse) UnmarshalJSON(data []byte) error {
	var legacy struct {
		Projects []string `json:"projects"`
	}
	errLegacy := json.Unmarshal(data, &legacy)
	if errLegacy == nil && legacy.Projects != nil {
		r.Projects = make([]projectIdentity, 0, len(legacy.Projects))
		for _, projectID := range legacy.Projects {
			r.Projects = append(r.Projects, projectIdentity{ID: projectID})
		}
		return nil
	}

	var current struct {
		Projects []struct {
			Project projectIdentity `json:"project"`
		} `json:"projects"`
	}
	errCurrent := json.Unmarshal(data, &current)
	if errCurrent != nil {
		return fmt.Errorf("decode projects response: %w", errCurrent)
	}
	r.Projects = make([]projectIdentity, 0, len(current.Projects))
	for _, project := range current.Projects {
		r.Projects = append(r.Projects, project.Project)
	}
	return nil
}

func (r *projectVersionsResponse) UnmarshalJSON(data []byte) error {
	var raw struct {
		ProjectID   string          `json:"project_id"`
		ProjectName string          `json:"project_name"`
		Project     projectIdentity `json:"project"`
		Versions    json.RawMessage `json:"versions"`
	}
	errDecode := json.Unmarshal(data, &raw)
	if errDecode != nil {
		return fmt.Errorf("decode project response: %w", errDecode)
	}

	r.ProjectID = raw.ProjectID
	r.ProjectName = raw.ProjectName
	if raw.Project.ID != "" {
		r.ProjectID = raw.Project.ID
		r.ProjectName = raw.Project.Name
	}

	versions, newestFirst, errVersions := decodeVersions(raw.Versions)
	if errVersions != nil {
		return errVersions
	}
	r.Versions = versions
	r.NewestFirst = newestFirst
	return nil
}

func (r *buildsResponse) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return errors.New("decode builds response: empty response")
	}

	if trimmed[0] == '[' {
		var current []struct {
			ID      int    `json:"id"`
			Time    string `json:"time"`
			Channel string `json:"channel"`
			Commits []struct {
				Message string `json:"message"`
			} `json:"commits"`
			Downloads map[string]struct {
				Name      string `json:"name"`
				Checksums struct {
					SHA256 string `json:"sha256"`
				} `json:"checksums"`
				Size int64  `json:"size"`
				URL  string `json:"url"`
			} `json:"downloads"`
		}
		errDecode := json.Unmarshal(trimmed, &current)
		if errDecode != nil {
			return fmt.Errorf("decode current builds response: %w", errDecode)
		}
		r.Builds = make([]buildEntry, 0, len(current))
		for _, build := range current {
			download := build.Downloads[serverDownload]
			changes := make([]string, 0, len(build.Commits))
			for _, commit := range build.Commits {
				changes = append(changes, commit.Message)
			}
			r.Builds = append(r.Builds, buildEntry{
				Build:       build.ID,
				Time:        build.Time,
				Channel:     build.Channel,
				Changes:     changes,
				FileName:    download.Name,
				SHA256:      download.Checksums.SHA256,
				FileSize:    download.Size,
				DownloadURL: download.URL,
			})
		}
		return nil
	}

	var legacy struct {
		Builds []struct {
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
		} `json:"builds"`
	}
	errDecode := json.Unmarshal(trimmed, &legacy)
	if errDecode != nil {
		return fmt.Errorf("decode legacy builds response: %w", errDecode)
	}
	r.Builds = make([]buildEntry, 0, len(legacy.Builds))
	for _, build := range legacy.Builds {
		changes := make([]string, 0, len(build.Changes))
		for _, change := range build.Changes {
			changes = append(changes, change.Summary)
		}
		r.Builds = append(r.Builds, buildEntry{
			Build:    build.Build,
			Time:     build.Time,
			Channel:  build.Channel,
			Changes:  changes,
			FileName: build.Downloads.Application.Name,
			SHA256:   build.Downloads.Application.SHA256,
		})
	}
	return nil
}

func decodeVersions(raw json.RawMessage) ([]string, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return []string{}, false, nil
	}
	if trimmed[0] == '[' {
		var versions []string
		errDecode := json.Unmarshal(trimmed, &versions)
		if errDecode != nil {
			return nil, false, fmt.Errorf("decode legacy project versions: %w", errDecode)
		}
		return versions, false, nil
	}
	if trimmed[0] != '{' {
		return nil, false, errors.New("decode project versions: expected array or object")
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	_, errToken := decoder.Token()
	if errToken != nil {
		return nil, false, fmt.Errorf("decode project version groups: %w", errToken)
	}
	versions := make([]string, 0)
	for decoder.More() {
		_, errKey := decoder.Token()
		if errKey != nil {
			return nil, false, fmt.Errorf("decode project version group name: %w", errKey)
		}
		var groupedVersions []string
		errGroup := decoder.Decode(&groupedVersions)
		if errGroup != nil {
			return nil, false, fmt.Errorf("decode project version group: %w", errGroup)
		}
		versions = append(versions, groupedVersions...)
	}
	_, errToken = decoder.Token()
	if errToken != nil {
		return nil, false, fmt.Errorf("finish project version groups: %w", errToken)
	}
	return versions, true, nil
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
