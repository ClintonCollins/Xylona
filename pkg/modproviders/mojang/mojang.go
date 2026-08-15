// Package mojang provides update-provider access to official Minecraft releases.
package mojang

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/modproviders/internal/providerhttp"
)

const (
	providerID         = "mojang"
	defaultManifestURL = "https://launchermeta.mojang.com/mc/game/version_manifest.json"
	serverJarName      = "minecraft_server.jar"
)

type globalManifest struct {
	Latest struct {
		Release string `json:"release"`
	} `json:"latest"`
	Versions []manifestVersion `json:"versions"`
}

type manifestVersion struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

type versionDetails struct {
	Downloads struct {
		Server struct {
			Sha1 string `json:"sha1"`
			Size int64  `json:"size"`
			URL  string `json:"url"`
		} `json:"server"`
	} `json:"downloads"`
}

// Provider implements the Mojang update-provider integration.
type Provider struct {
	httpClient  *http.Client
	manifestURL string
}

func init() {
	modproviders.RegisterProvider(New())
}

// New creates a Mojang update provider.
func New() *Provider {
	return &Provider{
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: helpers.GetXylonaHTTPClient().Transport,
		},
		manifestURL: defaultManifestURL,
	}
}

// ID returns the stable provider identifier.
func (p *Provider) ID() string {
	return providerID
}

// Search returns no results because Mojang only exposes direct version lookups.
func (p *Provider) Search(_ context.Context, _ string, _ modproviders.SearchParams) (modproviders.SearchResult, error) {
	return modproviders.SearchResult{}, nil
}

// GetModDetails returns the available official Minecraft server releases.
func (p *Provider) GetModDetails(ctx context.Context, sourceID string, _ modproviders.SearchParams) (*modproviders.ModDetails, error) {
	manifest, errManifest := p.getManifest(ctx)
	if errManifest != nil {
		return nil, fmt.Errorf("mojang get manifest: %w", errManifest)
	}

	versions := make([]modproviders.ModVersion, 0, len(manifest.Versions))
	for _, v := range manifest.Versions {
		if v.Type != "release" {
			continue
		}
		versions = append(versions, modproviders.ModVersion{
			VersionID:     v.ID,
			VersionString: v.ID,
			GameVersions:  []string{v.ID},
		})
	}

	return &modproviders.ModDetails{
		Source:      providerID,
		SourceID:    sourceID,
		Name:        "Vanilla",
		Description: "Official Minecraft server releases from Mojang",
		Versions:    versions,
	}, nil
}

// GetVersions returns the downloadable server jar for a specific game version.
func (p *Provider) GetVersions(ctx context.Context, _ string, gameVersion string, _ modproviders.SearchParams) ([]modproviders.ModVersion, error) {
	if gameVersion == "" {
		return nil, nil
	}

	versionURL, errVersionURL := p.versionManifestURL(ctx, gameVersion)
	if errVersionURL != nil {
		return nil, errVersionURL
	}

	details, errDetails := p.getVersionDetails(ctx, versionURL)
	if errDetails != nil {
		return nil, fmt.Errorf("mojang get version details: %w", errDetails)
	}

	if details.Downloads.Server.URL == "" {
		return nil, fmt.Errorf("mojang version %s has no server download", gameVersion)
	}

	return []modproviders.ModVersion{
		{
			VersionID:     gameVersion,
			VersionString: gameVersion,
			GameVersions:  []string{gameVersion},
			DownloadURL:   details.Downloads.Server.URL,
			FileSize:      details.Downloads.Server.Size,
			FileHashSHA1:  details.Downloads.Server.Sha1,
		},
	}, nil
}

// Download fetches the Mojang server jar for the requested version.
func (p *Provider) Download(ctx context.Context, _ string, versionID string, targetDir string) ([]modproviders.DownloadedFile, error) {
	versionURL, errVersionURL := p.versionManifestURL(ctx, versionID)
	if errVersionURL != nil {
		return nil, errVersionURL
	}

	details, errDetails := p.getVersionDetails(ctx, versionURL)
	if errDetails != nil {
		return nil, fmt.Errorf("mojang get version details: %w", errDetails)
	}
	if details.Downloads.Server.URL == "" {
		return nil, fmt.Errorf("mojang version %s has no server download", versionID)
	}

	destPath := filepath.Join(targetDir, serverJarName)
	written, hash, errDownload := providerhttp.DownloadToFile(ctx, p.httpClient, details.Downloads.Server.URL, destPath, providerID)
	if errDownload != nil {
		return nil, fmt.Errorf("mojang download server jar: %w", errDownload)
	}

	return []modproviders.DownloadedFile{
		{
			Path:      serverJarName,
			Hash:      hash,
			Size:      written,
			IsPrimary: true,
		},
	}, nil
}

// CheckForUpdate returns the latest available official Minecraft release.
func (p *Provider) CheckForUpdate(ctx context.Context, sourceID string, _ string) (*modproviders.ModVersion, error) {
	details, errDetails := p.GetModDetails(ctx, sourceID, nil)
	if errDetails != nil {
		return nil, errDetails
	}
	if details == nil || len(details.Versions) == 0 {
		return nil, modproviders.ErrNoUpdateAvailable
	}
	latest := details.Versions[0]
	return &latest, nil
}

func (p *Provider) getManifest(ctx context.Context) (*globalManifest, error) {
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, p.manifestURL, nil)
	if errReq != nil {
		return nil, fmt.Errorf("build manifest request: %w", errReq)
	}

	resp, errDo := p.httpClient.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("GET manifest: %w", errDo)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Warn().Err(errClose).Msg("mojang: failed to close manifest response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET manifest: unexpected status %d", resp.StatusCode)
	}

	var manifest globalManifest
	if errDecode := json.NewDecoder(resp.Body).Decode(&manifest); errDecode != nil {
		return nil, fmt.Errorf("decode manifest: %w", errDecode)
	}
	return &manifest, nil
}

func (p *Provider) versionManifestURL(ctx context.Context, versionID string) (string, error) {
	manifest, errManifest := p.getManifest(ctx)
	if errManifest != nil {
		return "", errManifest
	}
	for _, v := range manifest.Versions {
		if v.ID == versionID {
			return v.URL, nil
		}
	}
	return "", fmt.Errorf("mojang version %s not found", versionID)
}

func (p *Provider) getVersionDetails(ctx context.Context, url string) (*versionDetails, error) {
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if errReq != nil {
		return nil, fmt.Errorf("build version details request: %w", errReq)
	}

	resp, errDo := p.httpClient.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("GET version details: %w", errDo)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Warn().Err(errClose).Msg("mojang: failed to close version details response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET version details: unexpected status %d", resp.StatusCode)
	}

	var details versionDetails
	if errDecode := json.NewDecoder(resp.Body).Decode(&details); errDecode != nil {
		return nil, fmt.Errorf("decode version details: %w", errDecode)
	}
	return &details, nil
}
