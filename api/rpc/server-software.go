package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/modmanager"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetServerSoftwareOptions returns the available server software options for a game.
func (xs *XylonaService) GetServerSoftwareOptions(
	_ context.Context,
	request *connect.Request[xylona.GetServerSoftwareOptionsRequest],
) (*connect.Response[xylona.GetServerSoftwareOptionsResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	game, errGame := xs.db.GetGameByID(request.Msg.GetGameId())
	if errGame != nil {
		if errors.Is(errGame, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("game not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get game"))
	}

	softwareJSON := game.ServerSoftware.GetOr("")
	allSoftware, errParse := modmanager.ParseServerSoftware(softwareJSON)
	if errParse != nil {
		log.Error().Err(errParse).Str("game_id", game.ID).Msg("Failed to parse server software JSON")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to parse server software config"))
	}

	var options []*xylona.ServerSoftwareOption
	for _, sw := range allSoftware {
		providerID := modmanager.ProviderIDForGame(game.ID, sw)
		options = append(options, &xylona.ServerSoftwareOption{
			Id:            sw.ID,
			Name:          sw.Name,
			JarSource:     providerID,
			HasModSupport: sw.ModConfig != nil,
		})
	}

	return &connect.Response[xylona.GetServerSoftwareOptionsResponse]{
		Msg: &xylona.GetServerSoftwareOptionsResponse{
			Options: options,
		},
	}, nil
}

// GetServerSoftwareVersions returns available versions for a server software option.
func (xs *XylonaService) GetServerSoftwareVersions(
	ctx context.Context,
	request *connect.Request[xylona.GetServerSoftwareVersionsRequest],
) (*connect.Response[xylona.GetServerSoftwareVersionsResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	game, errGame := xs.db.GetGameByID(request.Msg.GetGameId())
	if errGame != nil {
		if errors.Is(errGame, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("game not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get game"))
	}

	softwareJSON := game.ServerSoftware.GetOr("")
	allSoftware, errParse := modmanager.ParseServerSoftware(softwareJSON)
	if errParse != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to parse server software config"))
	}

	sw, found := modmanager.GetSoftwareByID(allSoftware, request.Msg.GetSoftwareId())
	if !found {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("software option not found: %s", request.Msg.GetSoftwareId()))
	}

	providerID := modmanager.ProviderIDForGame(game.ID, *sw)
	if providerID == "" {
		return &connect.Response[xylona.GetServerSoftwareVersionsResponse]{
			Msg: &xylona.GetServerSoftwareVersionsResponse{},
		}, nil
	}

	// Use the jar source as the provider ID to get available game versions.
	provider, ok := modproviders.GetProvider(providerID)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("jar source provider not found: %s", providerID))
	}

	// For jar source providers like PaperMC, GetModDetails returns the project
	// with its available game versions (e.g., 1.21.4, 1.21.3). We return these
	// as SoftwareVersion entries — each represents a game version the user can
	// select, not individual builds.
	details, errDetails := provider.GetModDetails(ctx, sw.ID, nil)
	if errDetails != nil {
		log.Error().Err(errDetails).Str("jar_source", providerID).Msg("Failed to get software versions")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get software versions"))
	}

	var protoVersions []*xylona.SoftwareVersion
	// The versions in ModDetails come from the project's version list.
	// For PaperMC this is game versions; for other providers it may differ.
	if details != nil {
		for _, v := range details.Versions {
			protoVersions = append(protoVersions, &xylona.SoftwareVersion{
				VersionId:     v.VersionID,
				VersionString: v.VersionString,
			})
		}
	}

	return &connect.Response[xylona.GetServerSoftwareVersionsResponse]{
		Msg: &xylona.GetServerSoftwareVersionsResponse{
			Versions: protoVersions,
		},
	}, nil
}

// GetServerSoftwareStatus returns the current installation status for a game server's software.
func (xs *XylonaService) GetServerSoftwareStatus(
	_ context.Context,
	request *connect.Request[xylona.GetServerSoftwareStatusRequest],
) (*connect.Response[xylona.GetServerSoftwareStatusResponse], error) {
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

	return &connect.Response[xylona.GetServerSoftwareStatusResponse]{
		Msg: &xylona.GetServerSoftwareStatusResponse{
			Status:     state.Status,
			Error:      state.Error,
			SoftwareId: state.SoftwareID,
		},
	}, nil
}

// SetServerSoftware sets the active server software for a game server.
// If the software has a jar source, it kicks off a background download and
// returns status "installing". Otherwise it updates the DB immediately and
// returns status "complete".
func (xs *XylonaService) SetServerSoftware(
	_ context.Context,
	request *connect.Request[xylona.SetServerSoftwareRequest],
) (*connect.Response[xylona.SetServerSoftwareResponse], error) {
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

	// Server must be stopped before changing software.
	if gameServer.Status != xylona.Status_OFFLINE.String() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server must be stopped before changing software"))
	}

	gameServerID := gameServer.ID
	softwareID := request.Msg.GetSoftwareId()
	logConsoleOutput := func(message string) {
		if xs.supervisorInst != nil {
			xs.supervisorInst.SendConsoleOutput(gameServerID, message)
		}
	}

	// Reject if an install is already in progress.
	if xs.installTracker.IsInstalling(gameServerID) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("software installation already in progress"))
	}

	// Resolve the software definition from the game.
	game := gameServer.R.Game
	if game == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("game relation not loaded"))
	}

	softwareJSON := game.ServerSoftware.GetOr("")
	allSoftware, errParse := modmanager.ParseServerSoftware(softwareJSON)
	if errParse != nil {
		log.Error().Err(errParse).Str("game_id", game.ID).Msg("Failed to parse server software JSON")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to parse server software config"))
	}

	sw, found := modmanager.GetSoftwareByID(allSoftware, softwareID)
	if !found {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("software option not found: %s", softwareID))
	}

	// Count installed mods for the response.
	installedMods, errMods := xs.db.GetInstalledModsByGameServerID(gameServerID)
	if errMods != nil {
		log.Error().Err(errMods).Str("server_id", gameServerID).Msg("Failed to list installed mods")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list installed mods"))
	}
	modCount := int32(len(installedMods))

	providerID := modmanager.ProviderIDForGame(game.ID, *sw)
	selectedLabel := sw.Name
	if requestedVersion := request.Msg.GetVersionId(); requestedVersion != "" {
		selectedLabel = fmt.Sprintf("%s %s", sw.Name, requestedVersion)
	}

	// If the software has no jar source, update the DB immediately.
	if providerID == "" {
		logConsoleOutput(fmt.Sprintf("Changing server software to %s", selectedLabel))
		setter := &models.GameServerSetter{
			ID:               omit.From(gameServerID),
			ServerSoftware:   omitnull.From(softwareID),
			ServerExecutable: omitnull.FromNull(null.Val[string]{}),
		}
		updated, errUpdate := xs.db.UpdateGameServer(xs.db.DB, setter)
		if errUpdate != nil {
			log.Error().Err(errUpdate).Msg("Failed to update game server software")
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update server software"))
		}
		// Broadcast completion so the frontend can react (tab recalculation, etc.)
		if xs.installBroadcast != nil {
			xs.installBroadcast.BroadcastServerSoftwareInstall(gameServerID, modmanager.InstallStatusComplete, softwareID, "")
		}
		logConsoleOutput(fmt.Sprintf("Server software changed to %s", selectedLabel))
		return &connect.Response[xylona.SetServerSoftwareResponse]{
			Msg: &xylona.SetServerSoftwareResponse{
				GameServer:        helpers.GameServerModelToProto(updated, xs.versionState),
				Status:            modmanager.InstallStatusComplete,
				InstalledModCount: modCount,
			},
		}, nil
	}

	// Software has a jar source — kick off a background download.
	xs.installTracker.SetInstalling(gameServerID, softwareID)
	logConsoleOutput(fmt.Sprintf("Starting server software change to %s", selectedLabel))

	if xs.installBroadcast != nil {
		xs.installBroadcast.BroadcastServerSoftwareInstall(gameServerID, modmanager.InstallStatusInstalling, softwareID, "")
	}

	provider, ok := modproviders.GetProvider(providerID)
	if !ok {
		xs.installTracker.SetFailed(gameServerID, "jar source provider not found: "+providerID)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("jar source provider not found: %s", providerID))
	}

	requestedVersionID := request.Msg.GetVersionId()
	downloadCtx, downloadCancel := context.WithTimeout(xs.ctx, 10*time.Minute)
	go func() {
		defer downloadCancel()

		// The frontend sends a game version (e.g., "1.21.4") from GetServerSoftwareVersions,
		// but providers like PaperMC expect a version-build ID (e.g., "1.21.4-100") for
		// download. Resolve the game version to the latest build via GetVersions.
		downloadVersionID := requestedVersionID
		if requestedVersionID != "" {
			logConsoleOutput(fmt.Sprintf("Resolving latest build for %s", requestedVersionID))
		}
		builds, errBuilds := provider.GetVersions(downloadCtx, sw.ID, requestedVersionID, nil)
		if errBuilds == nil && len(builds) > 0 {
			// Use the last build (newest) — GetVersions returns oldest-first for PaperMC.
			downloadVersionID = builds[len(builds)-1].VersionID
		}

		logConsoleOutput(fmt.Sprintf("Downloading server software files for %s", selectedLabel))
		files, errDownload := provider.Download(downloadCtx, sw.ID, downloadVersionID, gameServer.Directory)
		if errDownload != nil {
			xs.installTracker.SetFailed(gameServerID, errDownload.Error())
			logConsoleOutput(fmt.Sprintf("Server software installation failed: %s", errDownload.Error()))
			if xs.installBroadcast != nil {
				xs.installBroadcast.BroadcastServerSoftwareInstall(gameServerID, modmanager.InstallStatusFailed, softwareID, errDownload.Error())
			}
			return
		}

		// Find the primary downloaded file.
		var newExecutable string
		for _, f := range files {
			if f.IsPrimary {
				newExecutable = f.Path
				break
			}
		}

		// Delete old executable if it differs from the new one.
		oldExe := gameServer.ServerExecutable.GetOr("")
		if oldExe != "" && oldExe != newExecutable {
			oldPath := filepath.Join(gameServer.Directory, oldExe)
			_ = os.Remove(oldPath) // Best effort
		}

		logConsoleOutput("Applying downloaded server software")
		// Update DB with new software and executable.
		setter := &models.GameServerSetter{
			ID:               omit.From(gameServerID),
			ServerSoftware:   omitnull.From(softwareID),
			ServerExecutable: omitnull.From(newExecutable),
		}
		_, errUpdate := xs.db.UpdateGameServer(xs.db.DB, setter)
		if errUpdate != nil {
			xs.installTracker.SetFailed(gameServerID, errUpdate.Error())
			logConsoleOutput(fmt.Sprintf("Server software installation failed: %s", errUpdate.Error()))
			if xs.installBroadcast != nil {
				xs.installBroadcast.BroadcastServerSoftwareInstall(gameServerID, modmanager.InstallStatusFailed, softwareID, errUpdate.Error())
			}
			return
		}

		xs.installTracker.SetComplete(gameServerID)
		logConsoleOutput(fmt.Sprintf("Server software changed to %s", selectedLabel))
		if xs.installBroadcast != nil {
			xs.installBroadcast.BroadcastServerSoftwareInstall(gameServerID, modmanager.InstallStatusComplete, softwareID, "")
		}
	}()

	return &connect.Response[xylona.SetServerSoftwareResponse]{
		Msg: &xylona.SetServerSoftwareResponse{
			GameServer:        helpers.GameServerModelToProto(gameServer, xs.versionState),
			Status:            modmanager.InstallStatusInstalling,
			InstalledModCount: modCount,
		},
	}, nil
}
