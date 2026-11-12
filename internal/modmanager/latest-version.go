package modmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ClintonCollins/Xylona/internal/updateconfig"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/modproviders/thunderstore"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// ErrLatestUnsupported is returned for mod sources that cannot resolve and
// install their latest version.
var ErrLatestUnsupported = errors.New("modmanager: mod source does not support updating to the latest version")

// versionListTTL is how long a provider's version list answers update checks
// before the provider is asked again.
const versionListTTL = 30 * time.Minute

// Steam Workshop items have no version history and download through a SteamCMD
// path Xylona does not configure. PaperMC and Mojang supply server software.
var latestUnsupportedSources = map[string]bool{
	"steam_workshop": true,
	"papermc":        true,
	"mojang":         true,
}

// UpdateScope holds what "latest" means for one game server's mods: the
// provider filters and game version that installing from Browse uses. Params
// has an entry, possibly nil, for each source in the server's mod profile.
type UpdateScope struct {
	GameID      string
	GameVersion string
	Params      map[string]modproviders.SearchParams
}

type versionListEntry struct {
	versions []modproviders.ModVersion
	expires  time.Time
}

// NewUpdateScope resolves the update scope for a game server from its game's
// update configuration.
func NewUpdateScope(game *models.Game, server *models.GameServer) (UpdateScope, error) {
	resolved, errResolve := updateconfig.ResolveModelConfig(game, server)
	if errResolve != nil {
		return UpdateScope{}, fmt.Errorf("modmanager: resolve update scope: %w", errResolve)
	}

	scope := UpdateScope{
		GameID: server.GameID,
		Params: make(map[string]modproviders.SearchParams),
	}
	// Only Paper and Mojang targets are game versions; Steam targets are branches.
	if resolved.Provider.Kind == updateproviders.ProviderKindPaperMC || resolved.Provider.Kind == updateproviders.ProviderKindMojang {
		scope.GameVersion = resolved.Target
	}
	if resolved.ModProfile != nil {
		for _, source := range resolved.ModProfile.Sources {
			scope.Params[source.ID] = updateproviders.SearchParams(source)
		}
	}
	return scope, nil
}

// LatestVersion returns the newest version of an installed mod that its game
// server can use, or modproviders.ErrNoUpdateAvailable when there is none.
func (m *ModManager) LatestVersion(ctx context.Context, mod *models.InstalledMod, scope UpdateScope) (*modproviders.ModVersion, error) {
	provider, ok := modproviders.GetProvider(mod.Source)
	if !ok || latestUnsupportedSources[mod.Source] {
		return nil, fmt.Errorf("%w: %s", ErrLatestUnsupported, mod.Source)
	}
	if scope.GameID == "valheim" && mod.Source == "thunderstore" && mod.SourceID == thunderstore.ValheimLoaderID {
		// Valheim servers run only the verified BepInEx release.
		return &modproviders.ModVersion{
			VersionID:     thunderstore.ValheimLoaderVersion,
			VersionString: thunderstore.ValheimLoaderVersion,
		}, nil
	}

	params, inProfile := scope.Params[mod.Source]
	if !inProfile {
		// Without the profile's filters, providers would offer builds for other loaders.
		return nil, fmt.Errorf("%w: %s is not a mod source for this server", ErrLatestUnsupported, mod.Source)
	}

	versions, errVersions := m.providerVersions(ctx, provider, mod.SourceID, scope.GameVersion, params)
	if errVersions != nil {
		return nil, errVersions
	}
	latest, found := newestCompatibleVersion(versions, mod.InstalledVersionID, scope.GameVersion == "")
	if !found {
		return nil, modproviders.ErrNoUpdateAvailable
	}
	return &latest, nil
}

// newestCompatibleVersion picks the newest version from a newest-first list.
// Without a known server game version, it keeps to versions that share a game
// version with the installed one, so an update cannot move a mod to another
// game release.
func newestCompatibleVersion(versions []modproviders.ModVersion, installedVersionID string, gameVersionUnknown bool) (modproviders.ModVersion, bool) {
	if len(versions) == 0 {
		return modproviders.ModVersion{}, false
	}
	if !gameVersionUnknown {
		return versions[0], true
	}

	installed := slices.IndexFunc(versions, func(version modproviders.ModVersion) bool {
		return version.VersionID == installedVersionID
	})
	if installed < 0 || len(versions[installed].GameVersions) == 0 {
		return versions[0], true
	}
	supported := versions[installed].GameVersions
	for _, version := range versions[:installed] {
		if slices.ContainsFunc(version.GameVersions, func(gameVersion string) bool {
			return slices.Contains(supported, gameVersion)
		}) {
			return version, true
		}
	}
	return versions[installed], true
}

// providerVersions lists a mod's versions through a short-lived cache so page
// loads and update checks do not ask the provider every time.
func (m *ModManager) providerVersions(
	ctx context.Context,
	provider modproviders.ModProvider,
	sourceID, gameVersion string,
	params modproviders.SearchParams,
) ([]modproviders.ModVersion, error) {
	encodedParams, errEncode := json.Marshal(params)
	if errEncode != nil {
		return nil, fmt.Errorf("modmanager: encode provider params: %w", errEncode)
	}
	key := strings.Join([]string{provider.ID(), sourceID, gameVersion, string(encodedParams)}, "\x00")

	m.versionListsMu.Lock()
	entry, cached := m.versionLists[key]
	m.versionListsMu.Unlock()
	if cached && time.Now().Before(entry.expires) {
		return entry.versions, nil
	}

	versions, errVersions := provider.GetVersions(ctx, sourceID, gameVersion, params)
	if errVersions != nil {
		return nil, fmt.Errorf("modmanager: list %s versions for %s: %w", provider.ID(), sourceID, errVersions)
	}
	// Keep only what update checks read; changelogs can be large.
	trimmed := make([]modproviders.ModVersion, 0, len(versions))
	for _, version := range versions {
		trimmed = append(trimmed, modproviders.ModVersion{
			VersionID:     version.VersionID,
			VersionString: version.VersionString,
			GameVersions:  version.GameVersions,
			PublishedAt:   version.PublishedAt,
		})
	}
	// Providers list newest first, but some APIs put pinned or featured versions
	// on top; publish times, where reported, decide.
	slices.SortStableFunc(trimmed, func(a, b modproviders.ModVersion) int {
		return b.PublishedAt.Compare(a.PublishedAt)
	})

	m.versionListsMu.Lock()
	m.versionLists[key] = versionListEntry{versions: trimmed, expires: time.Now().Add(versionListTTL)}
	m.versionListsMu.Unlock()
	return trimmed, nil
}
