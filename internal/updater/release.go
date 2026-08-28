package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strings"
	"time"
)

const (
	// ComponentController is the controller binary artifact.
	ComponentController = "controller"
	// ComponentNode is the remote node binary artifact.
	ComponentNode = "node"
)

// Release describes a GitHub release and its downloadable artifacts.
type Release struct {
	TagName     string
	Version     string
	PublishedAt time.Time
	Assets      []Asset
}

// Asset is a release asset relevant to updater verification.
type Asset struct {
	Name               string
	BrowserDownloadURL string
	Size               int64
	Digest             string
	SHA256             string
}

// Artifact is the selected asset for one component/platform.
type Artifact struct {
	Component   string
	GOOS        string
	GOARCH      string
	Name        string
	DownloadURL string
	Size        int64
	SHA256      string
}

// Client fetches GitHub Releases metadata.
type Client struct {
	Owner      string
	Repo       string
	BaseURL    string
	HTTPClient *http.Client
}

// HTTPStatusError reports a non-success response from a release metadata request.
type HTTPStatusError struct {
	StatusCode int
	Body       string
}

func (e *HTTPStatusError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("updater: fetch latest release: status %d", e.StatusCode)
	}
	return fmt.Sprintf("updater: fetch latest release: status %d: %s", e.StatusCode, body)
}

// NewGitHubClient returns a release client for a GitHub repository.
func NewGitHubClient(owner string, repo string) *Client {
	return &Client{
		Owner:      owner,
		Repo:       repo,
		BaseURL:    "https://api.github.com",
		HTTPClient: http.DefaultClient,
	}
}

// LatestRelease fetches the latest GitHub Release.
func (c *Client) LatestRelease(ctx context.Context) (*Release, error) {
	if c == nil {
		return nil, errors.New("updater: nil client")
	}
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}

	var endpoint string
	parsed, errParse := url.Parse(baseURL)
	if errParse == nil && strings.HasSuffix(parsed.Path, "/releases/latest") {
		endpoint = baseURL
	} else {
		endpoint = fmt.Sprintf("%s/repos/%s/%s/releases/latest", baseURL, url.PathEscape(c.Owner), url.PathEscape(c.Repo))
	}

	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if errReq != nil {
		return nil, fmt.Errorf("updater: create release request: %w", errReq)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Xylona-Updater")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, errDo := httpClient.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("updater: fetch latest release: %w", errDo)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, &HTTPStatusError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	var gh githubRelease
	errDecode := json.NewDecoder(resp.Body).Decode(&gh)
	if errDecode != nil {
		return nil, fmt.Errorf("updater: decode latest release: %w", errDecode)
	}

	release := &Release{
		TagName:     gh.TagName,
		Version:     strings.TrimPrefix(gh.TagName, "v"),
		PublishedAt: gh.PublishedAt,
		Assets:      make([]Asset, 0, len(gh.Assets)),
	}
	for _, asset := range gh.Assets {
		release.Assets = append(release.Assets, Asset{
			Name:               asset.Name,
			BrowserDownloadURL: asset.BrowserDownloadURL,
			Size:               asset.Size,
			Digest:             asset.Digest,
			SHA256:             digestSHA256(asset.Digest),
		})
	}
	return release, nil
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

func digestSHA256(digest string) string {
	trimmed := strings.TrimSpace(strings.ToLower(digest))
	return strings.TrimPrefix(trimmed, "sha256:")
}

// FindArtifact selects the release asset for component/goos/goarch.
func FindArtifact(release *Release, component string, goos string, goarch string) (Artifact, bool) {
	if release == nil {
		return Artifact{}, false
	}
	component = strings.TrimSpace(component)
	goos = normalizeGOOS(goos)
	goarch = normalizeGOARCH(goarch)
	prefix := artifactPrefix(component)
	if prefix == "" {
		return Artifact{}, false
	}

	for _, asset := range release.Assets {
		if isArtifactMatch(asset.Name, prefix, goos, goarch) {
			return Artifact{
				Component:   component,
				GOOS:        goos,
				GOARCH:      goarch,
				Name:        asset.Name,
				DownloadURL: asset.BrowserDownloadURL,
				Size:        asset.Size,
				SHA256:      asset.SHA256,
			}, true
		}
	}
	return Artifact{}, false
}

// FindChecksumAsset returns a checksums.txt release asset when present.
func FindChecksumAsset(release *Release) (Asset, bool) {
	if release == nil {
		return Asset{}, false
	}
	for _, asset := range release.Assets {
		if strings.EqualFold(asset.Name, "checksums.txt") {
			return asset, true
		}
	}
	return Asset{}, false
}

// FindChecksumBundleAsset returns the Sigstore bundle for checksums.txt.
func FindChecksumBundleAsset(release *Release) (Asset, bool) {
	if release == nil {
		return Asset{}, false
	}
	for _, asset := range release.Assets {
		if strings.EqualFold(asset.Name, "checksums.txt.sigstore.json") {
			return asset, true
		}
	}
	return Asset{}, false
}

func artifactPrefix(component string) string {
	switch component {
	case ComponentController:
		return "xylona"
	case ComponentNode:
		return "xylona-node"
	default:
		return ""
	}
}

func isArtifactMatch(name string, prefix string, goos string, goarch string) bool {
	base := strings.ToLower(path.Base(name))
	if prefix == "xylona" && strings.HasPrefix(base, "xylona-node") {
		return false
	}
	if !strings.HasPrefix(base, prefix) {
		return false
	}

	archAliases := []string{goarch}
	if goarch == "amd64" {
		archAliases = append(archAliases, "x86_64")
	}

	if !strings.Contains(base, goos) {
		return false
	}
	for _, arch := range archAliases {
		if strings.Contains(base, arch) {
			return true
		}
	}
	return false
}

func normalizeGOOS(goos string) string {
	trimmed := strings.ToLower(strings.TrimSpace(goos))
	if trimmed == "" {
		return runtime.GOOS
	}
	return trimmed
}

func normalizeGOARCH(goarch string) string {
	trimmed := strings.ToLower(strings.TrimSpace(goarch))
	if trimmed == "" {
		return runtime.GOARCH
	}
	switch trimmed {
	case "x86_64":
		return "amd64"
	default:
		return trimmed
	}
}
