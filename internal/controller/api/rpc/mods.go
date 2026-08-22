package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/controller/protomap"
	"github.com/ClintonCollins/Xylona/internal/modmanager"
	"github.com/ClintonCollins/Xylona/internal/updateconfig"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// PermissionGameServerMods is the RBAC permission key for managing game server mods.
const PermissionGameServerMods = "game_server.mods"

// serverModInfo holds the information needed by mod handlers after resolving
// a game server's mod configuration.
type serverModInfo struct {
	gameServer *models.GameServer
	game       *models.Game
	variantID  string
	modProfile *updateproviders.ModProfile
}

// getServerModInfo fetches the game server, resolves the typed variant/update
// config, and returns the effective mod profile for the server.
func getServerModInfo(xs *XylonaService, gameServerID string) (*serverModInfo, error) {
	gameServer, errGetServer := xs.db.GetGameServerByID(gameServerID)
	if errGetServer != nil {
		if errors.Is(errGetServer, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("game server not found"))
		}
		return nil, internalErrf("failed to get game server")
	}

	game, errGetGame := xs.db.GetGameByID(gameServer.GameID)
	if errGetGame != nil {
		return nil, internalErrf("failed to get game")
	}

	resolved, errResolve := updateconfig.ResolveModelConfig(game, gameServer)
	if errResolve != nil {
		return nil, internalErrf("failed to resolve server configuration")
	}

	return &serverModInfo{
		gameServer: gameServer,
		game:       game,
		variantID:  resolved.VariantID,
		modProfile: resolved.ModProfile,
	}, nil
}

// getInstallPath returns the install path from the effective mod profile.
func getInstallPath(profile *updateproviders.ModProfile) string {
	if profile == nil || profile.InstallPath == "" {
		return "mods"
	}
	return profile.InstallPath
}

func (xs *XylonaService) remoteModClient(gameServer *models.GameServer) (modmanager.FileClient, bool, error) {
	if gameServer == nil {
		return nil, false, nil
	}

	selfNodeID := xs.selfNodeID()
	if gameServer.NodeID == "" || gameServer.NodeID == selfNodeID {
		return nil, false, nil
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, true, errClient
	}
	return client, true, nil
}

// modSearchResultToProto converts a provider search result to proto.
func modSearchResultToProto(r modproviders.ModSearchResult) *xylona.ModSearchResult {
	return &xylona.ModSearchResult{
		Source:             r.Source,
		SourceId:           r.SourceID,
		Name:               r.Name,
		Author:             r.Author,
		Description:        r.Description,
		IconUrl:            r.IconURL,
		Downloads:          r.Downloads,
		LatestVersion:      r.LatestVersion,
		CompatibleVersions: r.CompatibleVersions,
		Categories:         r.Categories,
		DateModified:       r.DateModified,
	}
}

// modVersionToProto converts a provider mod version to proto.
func modVersionToProto(v modproviders.ModVersion) *xylona.ModVersion {
	var deps []*xylona.ModDependency
	for _, d := range v.Dependencies {
		deps = append(deps, &xylona.ModDependency{
			SourceId: d.SourceID,
			Name:     d.Name,
			Required: d.Required,
		})
	}
	return &xylona.ModVersion{
		VersionId:     v.VersionID,
		VersionString: v.VersionString,
		GameVersions:  v.GameVersions,
		DownloadUrl:   v.DownloadURL,
		FileSize:      v.FileSize,
		Dependencies:  deps,
		Changelog:     v.Changelog,
	}
}

// modDetailsToProto converts a provider mod details to proto.
func modDetailsToProto(d *modproviders.ModDetails) *xylona.ModDetails {
	var versions []*xylona.ModVersion
	for _, v := range d.Versions {
		versions = append(versions, modVersionToProto(v))
	}
	return &xylona.ModDetails{
		Source:        d.Source,
		SourceId:      d.SourceID,
		Name:          d.Name,
		Author:        d.Author,
		Description:   d.Description,
		Body:          d.Body,
		IconUrl:       d.IconURL,
		Downloads:     d.Downloads,
		GalleryImages: d.GalleryImages,
		Categories:    d.Categories,
		License:       d.License,
		SourceUrl:     d.SourceURL,
		Versions:      versions,
	}
}

// SearchMods searches for mods across configured providers.
func (xs *XylonaService) SearchMods(
	ctx context.Context,
	request *connect.Request[xylona.SearchModsRequest],
) (*connect.Response[xylona.SearchModsResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	info, errInfo := getServerModInfo(xs, request.Msg.GetGameServerId())
	if errInfo != nil {
		return nil, errInfo
	}
	errPerm := xs.ensureLocalServerPermission(user, info.gameServer, PermissionGameServerMods)
	if errPerm != nil {
		return nil, errPerm
	}

	if info.modProfile == nil {
		return &connect.Response[xylona.SearchModsResponse]{
			Msg: &xylona.SearchModsResponse{},
		}, nil
	}

	// Build source configs for the search.
	var sources []modmanager.SourceConfig
	requestedSource := request.Msg.GetSource()
	for _, src := range info.modProfile.Sources {
		if requestedSource != "" && src.ID != requestedSource {
			continue
		}
		sources = append(sources, modmanager.SourceConfig{
			ID:           src.ID,
			SearchParams: updateproviders.SearchParams(src),
		})
	}

	// Determine sort default based on whether a query is present.
	sortBy := request.Msg.GetSortBy()
	if sortBy == "" {
		if request.Msg.GetQuery() != "" {
			sortBy = "relevance"
		} else {
			sortBy = "downloads"
		}
	}

	pageSize := int(request.Msg.GetPageSize())
	if pageSize <= 0 {
		pageSize = 20
	}

	page := int(request.Msg.GetPage())
	offset := 0
	if page > 0 {
		offset = (page - 1) * pageSize
	}

	results, totalHits, errSearch := xs.modManager.SearchAll(
		ctx,
		request.Msg.GetQuery(),
		sources,
		sortBy,
		request.Msg.GetGameVersion(),
		request.Msg.GetCategories(),
		pageSize,
		offset,
	)
	if errSearch != nil {
		log.Error().Err(errSearch).Msg("Failed to search mods")
		return nil, internalErrf("search failed")
	}

	// Check which mods are already installed.
	installed, errInstalled := xs.db.GetInstalledModsByGameServerID(request.Msg.GetGameServerId())
	if errInstalled != nil {
		log.Warn().Err(errInstalled).Msg("Failed to get installed mods for search result enrichment")
	}
	installedMap := make(map[string]bool)
	for _, m := range installed {
		key := m.Source + ":" + m.SourceID
		installedMap[key] = true
	}

	var protoResults []*xylona.ModSearchResult
	for _, r := range results {
		pr := modSearchResultToProto(r)
		key := r.Source + ":" + r.SourceID
		pr.IsInstalled = installedMap[key]
		protoResults = append(protoResults, pr)
	}

	return &connect.Response[xylona.SearchModsResponse]{
		Msg: &xylona.SearchModsResponse{
			Results:    protoResults,
			TotalCount: clampSearchTotalCount(totalHits),
		},
	}, nil
}

func clampSearchTotalCount(totalHits int) int32 {
	if totalHits == modproviders.UnknownTotalHits {
		return int32(modproviders.UnknownTotalHits)
	}
	if totalHits < 0 {
		return 0
	}
	if totalHits > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(totalHits)
}

// GetModDetails returns detailed information about a specific mod.
func (xs *XylonaService) GetModDetails(
	ctx context.Context,
	request *connect.Request[xylona.GetModDetailsRequest],
) (*connect.Response[xylona.GetModDetailsResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	source := request.Msg.GetSource()
	provider, ok := modproviders.GetProvider(source)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("provider not found: %s", source))
	}

	// Get search params from server's mod config if available.
	var params modproviders.SearchParams
	info, errInfo := getServerModInfo(xs, request.Msg.GetGameServerId())
	if errInfo == nil && info.modProfile != nil {
		for _, src := range info.modProfile.Sources {
			if src.ID == source {
				params = updateproviders.SearchParams(src)
				break
			}
		}
	}

	details, errDetails := provider.GetModDetails(ctx, request.Msg.GetSourceId(), params)
	if errDetails != nil {
		log.Error().Err(errDetails).Str("source", source).Str("source_id", request.Msg.GetSourceId()).Msg("Failed to get mod details")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get mod details"))
	}

	return &connect.Response[xylona.GetModDetailsResponse]{
		Msg: &xylona.GetModDetailsResponse{
			Details: modDetailsToProto(details),
		},
	}, nil
}

// GetModVersions returns available versions for a mod.
func (xs *XylonaService) GetModVersions(
	ctx context.Context,
	request *connect.Request[xylona.GetModVersionsRequest],
) (*connect.Response[xylona.GetModVersionsResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	source := request.Msg.GetSource()
	provider, ok := modproviders.GetProvider(source)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("provider not found: %s", source))
	}

	// Get search params from server's mod config if available.
	var params modproviders.SearchParams
	info, errInfo := getServerModInfo(xs, request.Msg.GetGameServerId())
	if errInfo == nil && info.modProfile != nil {
		for _, src := range info.modProfile.Sources {
			if src.ID == source {
				params = updateproviders.SearchParams(src)
				break
			}
		}
	}

	versions, errVersions := provider.GetVersions(ctx, request.Msg.GetSourceId(), request.Msg.GetGameVersion(), params)
	if errVersions != nil {
		log.Error().Err(errVersions).Str("source", source).Msg("Failed to get mod versions")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get mod versions"))
	}

	var protoVersions []*xylona.ModVersion
	for _, v := range versions {
		protoVersions = append(protoVersions, modVersionToProto(v))
	}

	return &connect.Response[xylona.GetModVersionsResponse]{
		Msg: &xylona.GetModVersionsResponse{
			Versions: protoVersions,
		},
	}, nil
}

// InstallMod installs a mod on a game server.
func (xs *XylonaService) InstallMod(
	ctx context.Context,
	request *connect.Request[xylona.InstallModRequest],
) (*connect.Response[xylona.InstallModResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	info, errInfo := getServerModInfo(xs, request.Msg.GetGameServerId())
	if errInfo != nil {
		return nil, errInfo
	}

	errPerm := xs.ensureLocalServerPermission(user, info.gameServer, PermissionGameServerMods)
	if errPerm != nil {
		return nil, errPerm
	}

	if info.modProfile == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("mod support is not configured for this variant"))
	}

	installPath := getInstallPath(info.modProfile)

	remoteClient, isRemote, errRemoteClient := xs.remoteModClient(info.gameServer)
	if errRemoteClient != nil {
		return nil, errRemoteClient
	}

	var mod *models.InstalledMod
	var errInstall error
	if isRemote {
		mod, errInstall = xs.modManager.InstallRemote(
			ctx,
			remoteClient,
			info.gameServer.ID,
			request.Msg.GetSource(),
			request.Msg.GetSourceId(),
			request.Msg.GetVersionId(),
			info.gameServer.Directory,
			installPath,
		)
	} else {
		mod, errInstall = xs.modManager.Install(
			ctx,
			info.gameServer.ID,
			request.Msg.GetSource(),
			request.Msg.GetSourceId(),
			request.Msg.GetVersionId(),
			info.gameServer.Directory,
			installPath,
		)
	}
	if errInstall != nil {
		log.Error().Err(errInstall).Msg("Failed to install mod")
		return nil, internalErrf("failed to install mod")
	}

	return &connect.Response[xylona.InstallModResponse]{
		Msg: &xylona.InstallModResponse{
			InstalledMod: protomap.InstalledModModelToProto(mod),
		},
	}, nil
}

// UninstallMod removes a mod from a game server.
func (xs *XylonaService) UninstallMod(
	ctx context.Context,
	request *connect.Request[xylona.UninstallModRequest],
) (*connect.Response[xylona.UninstallModResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errGetServer := xs.getGameServerFromID(request.Msg.GetGameServerId())
	if errGetServer != nil {
		return nil, errGetServer
	}

	errPerm := xs.ensureLocalServerPermission(user, gameServer, PermissionGameServerMods)
	if errPerm != nil {
		return nil, errPerm
	}

	nodeClient, errNodeClient := xs.resolveNodeClient(gameServer)
	if errNodeClient != nil {
		return nil, errNodeClient
	}

	errUninstall := xs.modManager.Uninstall(ctx, nodeClient, request.Msg.GetInstalledModId(), gameServer.Directory)
	if errUninstall != nil {
		log.Error().Err(errUninstall).Msg("Failed to uninstall mod")
		return nil, internalErrf("failed to uninstall mod")
	}

	return &connect.Response[xylona.UninstallModResponse]{
		Msg: &xylona.UninstallModResponse{},
	}, nil
}

// UpdateMod updates an installed mod to a new version.
func (xs *XylonaService) UpdateMod(
	ctx context.Context,
	request *connect.Request[xylona.UpdateModRequest],
) (*connect.Response[xylona.UpdateModResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errGetServer := xs.getGameServerFromID(request.Msg.GetGameServerId())
	if errGetServer != nil {
		return nil, errGetServer
	}

	errPerm := xs.ensureLocalServerPermission(user, gameServer, PermissionGameServerMods)
	if errPerm != nil {
		return nil, errPerm
	}

	remoteClient, isRemote, errRemoteClient := xs.remoteModClient(gameServer)
	if errRemoteClient != nil {
		return nil, errRemoteClient
	}

	var updated *models.InstalledMod
	var errUpdate error
	if isRemote {
		updated, errUpdate = xs.modManager.UpdateRemote(ctx, remoteClient, request.Msg.GetInstalledModId(), request.Msg.GetVersionId(), gameServer.Directory)
	} else {
		updated, errUpdate = xs.modManager.Update(ctx, request.Msg.GetInstalledModId(), request.Msg.GetVersionId(), gameServer.Directory)
	}
	if errUpdate != nil {
		log.Error().Err(errUpdate).Msg("Failed to update mod")
		return nil, internalErrf("failed to update mod")
	}

	return &connect.Response[xylona.UpdateModResponse]{
		Msg: &xylona.UpdateModResponse{
			InstalledMod: protomap.InstalledModModelToProto(updated),
		},
	}, nil
}

// ListInstalledMods returns all installed mods for a game server.
func (xs *XylonaService) ListInstalledMods(
	_ context.Context,
	request *connect.Request[xylona.ListInstalledModsRequest],
) (*connect.Response[xylona.ListInstalledModsResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errGetServer := xs.getGameServerFromID(request.Msg.GetGameServerId())
	if errGetServer != nil {
		return nil, errGetServer
	}
	errPerm := xs.ensureLocalServerPermission(user, gameServer, PermissionGameServerMods)
	if errPerm != nil {
		return nil, errPerm
	}

	mods, errGet := xs.db.GetInstalledModsByGameServerID(request.Msg.GetGameServerId())
	if errGet != nil {
		log.Error().Err(errGet).Msg("Failed to get installed mods")
		return nil, internalErrf("failed to list installed mods")
	}

	var protoMods []*xylona.InstalledMod
	for _, m := range mods {
		protoMods = append(protoMods, protomap.InstalledModModelToProto(m))
	}

	return &connect.Response[xylona.ListInstalledModsResponse]{
		Msg: &xylona.ListInstalledModsResponse{
			InstalledMods: protoMods,
		},
	}, nil
}

// SetModAutoUpdate toggles auto-update for an installed mod.
func (xs *XylonaService) SetModAutoUpdate(
	_ context.Context,
	request *connect.Request[xylona.SetModAutoUpdateRequest],
) (*connect.Response[xylona.SetModAutoUpdateResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errGetServer := xs.getGameServerFromID(request.Msg.GetGameServerId())
	if errGetServer != nil {
		return nil, errGetServer
	}

	errPerm := xs.ensureLocalServerPermission(user, gameServer, PermissionGameServerMods)
	if errPerm != nil {
		return nil, errPerm
	}

	mod, errGet := xs.db.GetInstalledModByID(request.Msg.GetInstalledModId())
	if errGet != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("installed mod not found"))
	}

	autoUpdateVal := int64(0)
	if request.Msg.GetEnabled() {
		autoUpdateVal = 1
	}

	setter := &models.InstalledModSetter{
		AutoUpdate: omit.From(autoUpdateVal),
		UpdatedAt:  omit.From(time.Now().UTC()),
	}

	updated, errUpdate := xs.db.UpdateInstalledMod(xs.db.DB, mod, setter)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Msg("Failed to set mod auto-update")
		return nil, internalErrf("failed to update mod")
	}

	return &connect.Response[xylona.SetModAutoUpdateResponse]{
		Msg: &xylona.SetModAutoUpdateResponse{
			InstalledMod: protomap.InstalledModModelToProto(updated),
		},
	}, nil
}

// SetModEnabled enables or disables an installed mod.
func (xs *XylonaService) SetModEnabled(
	ctx context.Context,
	request *connect.Request[xylona.SetModEnabledRequest],
) (*connect.Response[xylona.SetModEnabledResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	info, errInfo := getServerModInfo(xs, request.Msg.GetGameServerId())
	if errInfo != nil {
		return nil, errInfo
	}

	errPerm := xs.ensureLocalServerPermission(user, info.gameServer, PermissionGameServerMods)
	if errPerm != nil {
		return nil, errPerm
	}

	if info.modProfile == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("mod support is not configured for this variant"))
	}

	installPath := getInstallPath(info.modProfile)

	nodeClient, errNodeClient := xs.resolveNodeClient(info.gameServer)
	if errNodeClient != nil {
		return nil, errNodeClient
	}

	if request.Msg.GetEnabled() {
		errEnable := xs.modManager.Enable(ctx, nodeClient, request.Msg.GetInstalledModId(), info.gameServer.Directory, installPath)
		if errEnable != nil {
			log.Error().Err(errEnable).Msg("Failed to enable mod")
			return nil, internalErrf("failed to enable mod")
		}
	} else {
		errDisable := xs.modManager.Disable(ctx, nodeClient, request.Msg.GetInstalledModId(), info.gameServer.Directory, installPath)
		if errDisable != nil {
			log.Error().Err(errDisable).Msg("Failed to disable mod")
			return nil, internalErrf("failed to disable mod")
		}
	}

	// Re-fetch the updated mod.
	mod, errGet := xs.db.GetInstalledModByID(request.Msg.GetInstalledModId())
	if errGet != nil {
		return nil, internalErrf("failed to fetch updated mod")
	}

	return &connect.Response[xylona.SetModEnabledResponse]{
		Msg: &xylona.SetModEnabledResponse{
			InstalledMod: protomap.InstalledModModelToProto(mod),
		},
	}, nil
}

// GetModCategories returns the available mod categories for browsing.
// Uses a hardcoded list of well-known Modrinth categories that apply broadly
// across mod platforms. Can be enhanced later to query provider APIs with caching.
func (xs *XylonaService) GetModCategories(
	_ context.Context,
	request *connect.Request[xylona.GetModCategoriesRequest],
) (*connect.Response[xylona.GetModCategoriesResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	categories := []string{
		"adventure", "cursed", "decoration", "economy", "equipment",
		"food", "game-mechanics", "library", "magic", "management",
		"minigame", "mobs", "optimization", "social", "storage",
		"technology", "transportation", "utility", "worldgen",
	}

	return &connect.Response[xylona.GetModCategoriesResponse]{
		Msg: &xylona.GetModCategoriesResponse{
			Categories: categories,
		},
	}, nil
}

// PinModVersion pins or unpins a mod to a specific version.
func (xs *XylonaService) PinModVersion(
	_ context.Context,
	request *connect.Request[xylona.PinModVersionRequest],
) (*connect.Response[xylona.PinModVersionResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errGetServer := xs.getGameServerFromID(request.Msg.GetGameServerId())
	if errGetServer != nil {
		return nil, errGetServer
	}

	errPerm := xs.ensureLocalServerPermission(user, gameServer, PermissionGameServerMods)
	if errPerm != nil {
		return nil, errPerm
	}

	mod, errGet := xs.db.GetInstalledModByID(request.Msg.GetInstalledModId())
	if errGet != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("installed mod not found"))
	}

	version := request.Msg.GetVersion()
	pinnedVersion := omitnull.FromNull(null.Val[string]{})
	if version != "" {
		pinnedVersion = omitnull.From(version)
	}
	setter := &models.InstalledModSetter{
		PinnedVersion: pinnedVersion,
		UpdatedAt:     omit.From(time.Now().UTC()),
	}

	updated, errUpdate := xs.db.UpdateInstalledMod(xs.db.DB, mod, setter)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Msg("Failed to pin mod version")
		return nil, internalErrf("failed to update mod")
	}

	return &connect.Response[xylona.PinModVersionResponse]{
		Msg: &xylona.PinModVersionResponse{
			InstalledMod: protomap.InstalledModModelToProto(updated),
		},
	}, nil
}
