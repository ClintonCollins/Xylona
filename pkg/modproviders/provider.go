// Package modproviders defines provider interfaces and shared types for mod sources.
package modproviders

import (
	"context"
	"errors"
	"maps"
)

// MaxModDownloadSize is the maximum number of bytes allowed for a single mod
// download. Downloads that exceed this limit are rejected.
const MaxModDownloadSize = 500 * 1024 * 1024 // 500 MB

// UnknownTotalHits marks provider search results whose real total count is not
// known or not pagination-safe.
const UnknownTotalHits = -1

var (
	// ErrDownloadTooLarge is returned when a download exceeds MaxModDownloadSize.
	ErrDownloadTooLarge = errors.New("modproviders: download exceeds maximum allowed size")
	// ErrNoUpdateAvailable indicates no compatible version is available to compare or install.
	ErrNoUpdateAvailable = errors.New("modproviders: no update available")
	// ErrMissingIntegrityMetadata is returned when a provider that should supply
	// integrity metadata omits it for a downloadable artifact.
	ErrMissingIntegrityMetadata = errors.New("modproviders: missing integrity metadata")
	// ErrIntegrityMismatch is returned when a downloaded artifact does not match
	// the provider-advertised integrity metadata.
	ErrIntegrityMismatch = errors.New("modproviders: integrity verification failed")
)

// Well-known SearchParams keys. Providers read these from the map alongside
// any game-specific params configured in the server software definition.
const (
	ParamSortBy      = "sort_by"      // string: "downloads", "updated", "newest", "relevance"
	ParamGameVersion = "game_version" // string: e.g. "1.21.4"
	ParamCategories  = "categories"   // []string: category/tag names
	ParamLimit       = "limit"        // int: page size
	ParamOffset      = "offset"       // int: pagination offset
)

// SearchParams holds passthrough parameters from the game's server software config.
// Each provider interprets its own params (e.g., Modrinth uses "facets", Hangar uses "platform").
type SearchParams map[string]any

// SearchResult contains a page of search results with the total hit count.
type SearchResult struct {
	Results   []ModSearchResult
	TotalHits int
}

// ModSearchResult is a single search result from a provider.
type ModSearchResult struct {
	Source             string
	SourceID           string
	Name               string
	Author             string
	Description        string
	IconURL            string
	Downloads          int64
	LatestVersion      string
	CompatibleVersions []string
	Categories         []string
	DateModified       string
}

// ModDetails contains full information about a mod.
type ModDetails struct {
	Source        string
	SourceID      string
	Name          string
	Author        string
	Description   string
	Body          string // Full description, usually markdown
	IconURL       string
	Downloads     int64
	GalleryImages []string
	Categories    []string
	License       string
	SourceURL     string
	Versions      []ModVersion
}

// ModVersion represents a specific version of a mod.
type ModVersion struct {
	VersionID     string
	VersionString string
	GameVersions  []string
	DownloadURL   string
	FileSize      int64
	Dependencies  []ModDependency
	Changelog     string
}

// ModDependency represents a mod dependency.
type ModDependency struct {
	SourceID string
	Name     string
	Required bool
}

// DownloadedFile represents a file downloaded by a provider.
type DownloadedFile struct {
	Path      string // Relative path where the file was written
	Hash      string // SHA-256 hash
	Size      int64
	IsPrimary bool
}

// ModProvider is the interface that all mod source providers must implement.
type ModProvider interface {
	// ID returns the provider identifier (e.g., "modrinth", "hangar").
	ID() string

	// Search queries the source with the given query and game-defined search params.
	Search(ctx context.Context, query string, params SearchParams) (SearchResult, error)

	// GetModDetails returns full details for a specific mod.
	GetModDetails(ctx context.Context, sourceID string, params SearchParams) (*ModDetails, error)

	// GetVersions lists available versions, filterable by game version.
	GetVersions(ctx context.Context, sourceID string, gameVersion string, params SearchParams) ([]ModVersion, error)

	// Download fetches the mod file(s) to the target directory.
	Download(ctx context.Context, sourceID string, versionID string, targetDir string) ([]DownloadedFile, error)

	// CheckForUpdate returns the newest compatible version or ErrNoUpdateAvailable.
	CheckForUpdate(ctx context.Context, sourceID string, gameVersion string) (*ModVersion, error)

	// RequiresAPIKey reports whether this provider needs an API key to function.
	RequiresAPIKey() bool
}

// registry holds all registered providers.
var registry = map[string]ModProvider{}

// RegisterProvider registers a provider. Panics if a provider with the same ID is already registered.
func RegisterProvider(p ModProvider) {
	id := p.ID()
	if _, exists := registry[id]; exists {
		panic("modproviders: provider already registered: " + id)
	}
	registry[id] = p
}

// GetProvider returns the provider with the given ID and whether it was found.
func GetProvider(id string) (ModProvider, bool) {
	p, ok := registry[id]
	return p, ok
}

// AllProviders returns all registered providers.
func AllProviders() map[string]ModProvider {
	result := make(map[string]ModProvider, len(registry))
	maps.Copy(result, registry)
	return result
}
