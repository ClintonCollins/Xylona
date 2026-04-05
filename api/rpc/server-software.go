package rpc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/modmanager"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetUpdateTargets lists selectable update targets for a server variant.
func (xs *XylonaService) GetUpdateTargets(
	ctx context.Context,
	request *connect.Request[xylona.GetUpdateTargetsRequest],
) (*connect.Response[xylona.GetUpdateTargetsResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	gameServer, errGetServer := xs.getGameServerFromID(request.Msg.GetGameServerId())
	if errGetServer != nil {
		return nil, errGetServer
	}

	errPerm := xs.ensureLocalServerPermission(user, gameServer, "game_server.view")
	if errPerm != nil {
		return nil, errPerm
	}

	if gameServer.R.Game == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("game relation not loaded"))
	}

	gameConfig, errConfig := updateproviders.LoadGameConfigFromModel(gameServer.R.Game)
	if errConfig != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load update configuration"))
	}

	serverConfig := updateproviders.LoadServerConfigFromModel(gameServer)
	requestedVariantID := strings.TrimSpace(request.Msg.GetVariantId())
	if requestedVariantID != "" {
		currentVariantID := serverConfig.VariantID
		serverConfig.VariantID = requestedVariantID
		if requestedVariantID != currentVariantID {
			resetTarget, errReset := updateproviders.ResetTargetForVariant(gameConfig, requestedVariantID)
			if errReset != nil {
				return nil, connect.NewError(connect.CodeNotFound, errReset)
			}
			serverConfig.Target = resetTarget
		}
	}

	resolved, errResolve := updateproviders.ResolveConfig(gameConfig, serverConfig)
	if errResolve != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to resolve update configuration"))
	}

	targets, errTargets := xs.listUpdateTargets(ctx, gameServer, resolved)
	if errTargets != nil {
		return nil, errTargets
	}

	return connect.NewResponse(&xylona.GetUpdateTargetsResponse{
		Targets:       targets,
		CurrentTarget: resolved.Target,
	}), nil
}

// GetVariantOperationStatus returns the current variant install operation state.
func (xs *XylonaService) GetVariantOperationStatus(
	_ context.Context,
	request *connect.Request[xylona.GetVariantOperationStatusRequest],
) (*connect.Response[xylona.GetVariantOperationStatusResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	gameServer, errGetServer := xs.getGameServerFromID(request.Msg.GetGameServerId())
	if errGetServer != nil {
		return nil, errGetServer
	}

	errPerm := xs.ensureLocalServerPermission(user, gameServer, PermissionGameServerMods)
	if errPerm != nil {
		return nil, errPerm
	}

	state, _ := xs.installTracker.Get(request.Msg.GetGameServerId())

	return connect.NewResponse(&xylona.GetVariantOperationStatusResponse{
		Status:    state.Status,
		Error:     state.Error,
		VariantId: state.SoftwareID,
	}), nil
}

// SetServerVariant changes the selected server software variant for a server.
func (xs *XylonaService) SetServerVariant(
	_ context.Context,
	request *connect.Request[xylona.SetServerVariantRequest],
) (*connect.Response[xylona.SetServerVariantResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	gameServer, errGetServer := xs.getGameServerFromID(request.Msg.GetGameServerId())
	if errGetServer != nil {
		return nil, errGetServer
	}

	errPerm := xs.ensureLocalServerPermission(user, gameServer, PermissionGameServerMods)
	if errPerm != nil {
		return nil, errPerm
	}

	if xs.getLocalGameServerStatus(gameServer) != xylona.Status_OFFLINE {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server must be stopped before changing variant"))
	}

	if xs.installTracker.IsInstalling(gameServer.ID) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("variant change already in progress"))
	}

	game := gameServer.R.Game
	if game == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("game relation not loaded"))
	}

	gameConfig, errConfig := updateproviders.LoadGameConfigFromModel(game)
	if errConfig != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load variant configuration"))
	}

	variantID := strings.TrimSpace(request.Msg.GetVariantId())
	target := strings.TrimSpace(request.Msg.GetTarget())
	pinTarget := request.Msg.GetPinTarget()
	if target == "" {
		var errTarget error
		target, errTarget = updateproviders.ResetTargetForVariant(gameConfig, variantID)
		if errTarget != nil {
			return nil, connect.NewError(connect.CodeNotFound, errTarget)
		}
	}

	resolved, errResolve := updateproviders.ResolveConfig(gameConfig, updateproviders.ServerConfig{
		VariantID:    variantID,
		Target:       target,
		TargetPinned: true,
	})
	if errResolve != nil {
		return nil, connect.NewError(connect.CodeNotFound, errResolve)
	}

	persistedTarget, persistedTargetPinned := persistedVariantTarget(resolved.Provider.Kind, resolved.Target, pinTarget)

	installedMods, errMods := xs.db.GetInstalledModsByGameServerID(gameServer.ID)
	if errMods != nil {
		log.Error().Err(errMods).Str("server_id", gameServer.ID).Msg("Failed to list installed mods")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list installed mods"))
	}
	modCount := helpers.ClampInt32FromInt(len(installedMods))

	if !variantRequiresDownload(resolved.Provider.Kind) {
		updated, errUpdate := xs.persistVariantSelection(
			gameServer,
			variantID,
			persistedTarget,
			persistedTargetPinned,
			omitnull.Val[string]{},
		)
		if errUpdate != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update variant"))
		}
		if xs.actionsInst != nil {
			xs.actionsInst.CheckServerVersionByID(xs.ctx, gameServer.ID)
		}
		return connect.NewResponse(&xylona.SetServerVariantResponse{
			GameServer:        helpers.GameServerModelToProto(updated, xs.versionState),
			Status:            modmanager.InstallStatusComplete,
			InstalledModCount: modCount,
		}), nil
	}

	logConsoleOutput := func(message string) {
		if xs.supervisorInst != nil {
			xs.supervisorInst.SendConsoleOutput(gameServer.ID, message)
		}
	}

	xs.installTracker.SetInstalling(gameServer.ID, variantID)
	logConsoleOutput(fmt.Sprintf("Starting variant change to %s", variantDisplayName(resolved)))

	if xs.installBroadcast != nil {
		xs.installBroadcast.BroadcastServerSoftwareInstall(gameServer.ID, modmanager.InstallStatusInstalling, variantID, "")
	}

	go xs.applyVariantDownload(gameServer, variantID, resolved, persistedTarget, persistedTargetPinned, logConsoleOutput)

	return connect.NewResponse(&xylona.SetServerVariantResponse{
		GameServer:        helpers.GameServerModelToProto(gameServer, xs.versionState),
		Status:            modmanager.InstallStatusInstalling,
		InstalledModCount: modCount,
	}), nil
}

func (xs *XylonaService) listUpdateTargets(
	ctx context.Context,
	gameServer *models.GameServer,
	resolved updateproviders.ResolvedConfig,
) ([]*xylona.UpdateTargetOption, error) {
	switch resolved.Provider.Kind {
	case updateproviders.ProviderKindSteamCMD:
		if gameServer.R.Game == nil || strings.TrimSpace(gameServer.R.Game.SteamAppID) == "" {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("steam update targets are unavailable"))
		}
		if xs.steamCache == nil {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("steam cache is unavailable"))
		}

		releases, errReleases := xs.steamCache.FetchReleases(ctx, gameServer.R.Game.SteamAppID)
		if errReleases != nil {
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to load steam release metadata"))
		}

		targets := make([]*xylona.UpdateTargetOption, 0, len(releases))
		for _, release := range releases {
			label := strings.TrimSpace(release.DisplayLabel)
			if label == "" {
				label = strings.TrimSpace(release.Name)
			}
			description := strings.TrimSpace(release.Description)
			if description == "" && release.BuildID != "" {
				description = fmt.Sprintf("Build %s", release.BuildID)
			}
			targets = append(targets, &xylona.UpdateTargetOption{
				Id:            release.Name,
				Label:         label,
				Description:   description,
				LatestVersion: release.BuildID,
				IsSelected:    updateproviders.NormalizeSteamTarget(release.Name) == updateproviders.NormalizeSteamTarget(resolved.Target),
			})
		}
		return targets, nil
	case updateproviders.ProviderKindPaperMC, updateproviders.ProviderKindMojang:
		provider, ok := providerForVariant(resolved.Provider)
		if !ok {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("update provider is unavailable"))
		}

		details, errDetails := provider.GetModDetails(ctx, resolved.Provider.SourceID, nil)
		if errDetails != nil {
			log.Error().Err(errDetails).Str("provider", provider.ID()).Str("source_id", resolved.Provider.SourceID).Msg("Failed to load update targets")
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to load update targets"))
		}

		targets := make([]*xylona.UpdateTargetOption, 0, len(details.Versions))
		for _, version := range details.Versions {
			targetID := strings.TrimSpace(version.VersionID)
			if targetID == "" {
				targetID = strings.TrimSpace(version.VersionString)
			}
			label := strings.TrimSpace(version.VersionString)
			if label == "" {
				label = targetID
			}
			targets = append(targets, &xylona.UpdateTargetOption{
				Id:            targetID,
				Label:         label,
				LatestVersion: strings.TrimSpace(version.VersionString),
				IsSelected:    targetID == strings.TrimSpace(resolved.Target),
			})
		}
		return targets, nil
	default:
		return []*xylona.UpdateTargetOption{}, nil
	}
}

func (xs *XylonaService) applyVariantDownload(
	gameServer *models.GameServer,
	variantID string,
	resolved updateproviders.ResolvedConfig,
	persistedTarget string,
	persistedTargetPinned bool,
	logConsoleOutput func(message string),
) {
	downloadCtx, cancel := context.WithTimeout(xs.ctx, 10*time.Minute)
	defer cancel()

	provider, ok := providerForVariant(resolved.Provider)
	if !ok {
		errMsg := "variant update provider not found"
		xs.installTracker.SetFailed(gameServer.ID, errMsg)
		if xs.installBroadcast != nil {
			xs.installBroadcast.BroadcastServerSoftwareInstall(gameServer.ID, modmanager.InstallStatusFailed, variantID, errMsg)
		}
		return
	}

	downloadVersionID, errDownloadID := resolveVariantDownloadVersion(downloadCtx, provider, resolved)
	if errDownloadID != nil {
		xs.installTracker.SetFailed(gameServer.ID, errDownloadID.Error())
		logConsoleOutput(fmt.Sprintf("Variant change failed: %s", errDownloadID.Error()))
		if xs.installBroadcast != nil {
			xs.installBroadcast.BroadcastServerSoftwareInstall(gameServer.ID, modmanager.InstallStatusFailed, variantID, errDownloadID.Error())
		}
		return
	}

	logConsoleOutput(fmt.Sprintf("Downloading files for %s", variantDisplayName(resolved)))
	files, errDownload := provider.Download(downloadCtx, resolved.Provider.SourceID, downloadVersionID, gameServer.Directory)
	if errDownload != nil {
		xs.installTracker.SetFailed(gameServer.ID, errDownload.Error())
		logConsoleOutput(fmt.Sprintf("Variant change failed: %s", errDownload.Error()))
		if xs.installBroadcast != nil {
			xs.installBroadcast.BroadcastServerSoftwareInstall(gameServer.ID, modmanager.InstallStatusFailed, variantID, errDownload.Error())
		}
		return
	}

	newExecutable := primaryVariantDownloadedFile(files)
	oldExecutable := gameServer.ServerExecutable.GetOr("")
	if oldExecutable != "" && oldExecutable != newExecutable {
		oldPath := filepath.Join(gameServer.Directory, oldExecutable)
		errRemove := os.Remove(oldPath)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			log.Warn().Err(errRemove).Str("path", oldPath).Msg("Failed to remove superseded executable")
		}
	}

	logConsoleOutput(fmt.Sprintf("Applying variant %s", variantDisplayName(resolved)))
	updated, errUpdate := xs.persistVariantSelection(
		gameServer,
		variantID,
		persistedTarget,
		persistedTargetPinned,
		executableSetterValue(newExecutable),
	)
	if errUpdate != nil {
		xs.installTracker.SetFailed(gameServer.ID, errUpdate.Error())
		logConsoleOutput(fmt.Sprintf("Variant change failed: %s", errUpdate.Error()))
		if xs.installBroadcast != nil {
			xs.installBroadcast.BroadcastServerSoftwareInstall(gameServer.ID, modmanager.InstallStatusFailed, variantID, errUpdate.Error())
		}
		return
	}

	gameServer.ServerExecutable = updated.ServerExecutable
	gameServer.ServerSoftware = updated.ServerSoftware
	gameServer.Branch = updated.Branch
	gameServer.TargetPinned = updated.TargetPinned

	xs.installTracker.SetComplete(gameServer.ID)
	logConsoleOutput(fmt.Sprintf("Variant changed to %s", variantDisplayName(resolved)))
	if xs.installBroadcast != nil {
		xs.installBroadcast.BroadcastServerSoftwareInstall(gameServer.ID, modmanager.InstallStatusComplete, variantID, "")
	}
	if xs.actionsInst != nil {
		xs.actionsInst.CheckServerVersionByID(xs.ctx, gameServer.ID)
	}
}

func (xs *XylonaService) persistVariantSelection(
	gameServer *models.GameServer,
	variantID string,
	target string,
	targetPinned bool,
	executable omitnull.Val[string],
) (*models.GameServer, error) {
	setter := &models.GameServerSetter{
		ID:             omit.From(gameServer.ID),
		ServerSoftware: omitnull.FromNull(null.FromCond(strings.TrimSpace(variantID), strings.TrimSpace(variantID) != "")),
		Branch:         omit.From(strings.TrimSpace(target)),
		TargetPinned:   omit.From(targetPinned),
	}
	if !executable.IsUnset() {
		setter.ServerExecutable = executable
	}

	updated, errUpdate := xs.db.UpdateGameServer(xs.db.DB, setter)
	if errUpdate != nil {
		return nil, fmt.Errorf("rpc: persist variant selection: %w", errUpdate)
	}
	return updated, nil
}

func executableSetterValue(executable string) omitnull.Val[string] {
	if strings.TrimSpace(executable) == "" {
		return omitnull.FromNull(null.Val[string]{})
	}
	return omitnull.From(strings.TrimSpace(executable))
}

func persistedVariantTarget(kind updateproviders.ProviderKind, target string, pinTarget bool) (string, bool) {
	normalizedTarget := strings.TrimSpace(target)
	if kind == updateproviders.ProviderKindPaperMC || kind == updateproviders.ProviderKindMojang {
		if !pinTarget {
			return "", false
		}
		if normalizedTarget == "" {
			return "", false
		}
		return normalizedTarget, true
	}
	return normalizedTarget, false
}

func variantRequiresDownload(kind updateproviders.ProviderKind) bool {
	return kind == updateproviders.ProviderKindPaperMC || kind == updateproviders.ProviderKindMojang
}

func providerForVariant(cfg updateproviders.ProviderConfig) (modproviders.ModProvider, bool) {
	switch cfg.Kind {
	case updateproviders.ProviderKindPaperMC:
		return modproviders.GetProvider("papermc")
	case updateproviders.ProviderKindMojang:
		return modproviders.GetProvider("mojang")
	default:
		return nil, false
	}
}

func resolveVariantDownloadVersion(
	ctx context.Context,
	provider modproviders.ModProvider,
	resolved updateproviders.ResolvedConfig,
) (string, error) {
	target := strings.TrimSpace(resolved.Target)
	if target == "" {
		details, errDetails := provider.GetModDetails(ctx, resolved.Provider.SourceID, nil)
		if errDetails != nil {
			return "", fmt.Errorf("resolve target version: %w", errDetails)
		}
		if details == nil || len(details.Versions) == 0 {
			return "", errors.New("variant target is not configured")
		}
		target = strings.TrimSpace(details.Versions[0].VersionID)
		if target == "" {
			target = strings.TrimSpace(details.Versions[0].VersionString)
		}
		if target == "" {
			return "", errors.New("variant target is not configured")
		}
	}

	versions, errVersions := provider.GetVersions(ctx, resolved.Provider.SourceID, target, nil)
	if errVersions != nil {
		return "", fmt.Errorf("resolve target version: %w", errVersions)
	}
	if len(versions) == 0 {
		return target, nil
	}

	selected := versions[len(versions)-1]
	if strings.TrimSpace(selected.VersionID) != "" {
		return strings.TrimSpace(selected.VersionID), nil
	}
	if strings.TrimSpace(selected.VersionString) != "" {
		return strings.TrimSpace(selected.VersionString), nil
	}
	return target, nil
}

func variantDisplayName(resolved updateproviders.ResolvedConfig) string {
	name := strings.TrimSpace(resolved.VariantName)
	if name == "" {
		name = strings.TrimSpace(resolved.VariantID)
	}
	if name == "" {
		name = strings.TrimSpace(resolved.Target)
	}
	if name == "" {
		name = "selected variant"
	}
	if strings.TrimSpace(resolved.Target) == "" {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, resolved.Target)
}

func primaryVariantDownloadedFile(files []modproviders.DownloadedFile) string {
	for _, file := range files {
		if file.IsPrimary {
			return file.Path
		}
	}
	return ""
}
