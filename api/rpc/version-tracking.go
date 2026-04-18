package rpc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (xs *XylonaService) resolveVersionData(ctx context.Context, gs *models.GameServer, opts actions.VersionResolveOptions) (string, *xylona.VersionInfo, error) {
	if strings.TrimSpace(gs.NodeID) == "" || strings.TrimSpace(gs.NodeID) == xs.selfNodeID() {
		return xs.resolveLocalVersionData(ctx, gs, opts)
	}
	return xs.resolveRemoteVersionData(ctx, gs, opts)
}

func (xs *XylonaService) resolveLocalVersionData(ctx context.Context, gs *models.GameServer, opts actions.VersionResolveOptions) (string, *xylona.VersionInfo, error) {
	if xs.actionsInst == nil || xs.versionState == nil {
		return "", nil, nil
	}
	_, state := xs.actionsInst.ResolveVersionData(ctx, gs, opts)
	return state.InstalledVersion, versionStateToProto(state), nil
}

func (xs *XylonaService) resolveRemoteVersionData(ctx context.Context, gs *models.GameServer, opts actions.VersionResolveOptions) (string, *xylona.VersionInfo, error) {
	if xs.actionsInst == nil || xs.versionState == nil {
		return "", nil, nil
	}

	client, errClient := xs.resolveNodeClient(gs)
	if errClient != nil {
		return "", nil, errClient
	}

	stagedServer, cleanup, errStage := stageRemoteVersionServer(ctx, client, gs)
	if errStage != nil {
		return "", nil, internalErrf("failed to stage remote version data")
	}
	defer cleanup()

	_, state := xs.actionsInst.ResolveVersionData(ctx, stagedServer, opts)
	return state.InstalledVersion, versionStateToProto(state), nil
}

// GetVersionInfo returns cached or resolved version metadata for a game server.
func (xs *XylonaService) GetVersionInfo(ctx context.Context, req *connect.Request[xylona.GetVersionInfoRequest]) (*connect.Response[xylona.GetVersionInfoResponse], error) {
	user, errUser := xs.getUserFromHeader(req.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	gameServer, errLookup := xs.db.GetGameServerByID(req.Msg.GetGameServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.view")
	if errPermission != nil {
		return nil, errPermission
	}

	_, versionInfo, errResolve := xs.resolveVersionData(ctx, gameServer, actions.VersionResolveOptions{})
	if errResolve != nil {
		return nil, errResolve
	}

	return connect.NewResponse(&xylona.GetVersionInfoResponse{
		VersionInfo: versionInfo,
	}), nil
}

// CheckForUpdate refreshes version metadata for a game server.
func (xs *XylonaService) CheckForUpdate(ctx context.Context, req *connect.Request[xylona.CheckForUpdateRequest]) (*connect.Response[xylona.CheckForUpdateResponse], error) {
	user, errUser := xs.getUserFromHeader(req.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	gameServer, errLookup := xs.db.GetGameServerByID(req.Msg.GetGameServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.view")
	if errPermission != nil {
		return nil, errPermission
	}

	_, versionInfo, errResolve := xs.resolveVersionData(ctx, gameServer, actions.VersionResolveOptions{
		ForceRefresh: true,
	})
	if errResolve != nil {
		return nil, errResolve
	}

	return connect.NewResponse(&xylona.CheckForUpdateResponse{
		VersionInfo: versionInfo,
	}), nil
}

// SetDummyUpdateFailure toggles simulated update failures for dummy tracker tests.
func (xs *XylonaService) SetDummyUpdateFailure(_ context.Context, req *connect.Request[xylona.SetDummyUpdateFailureRequest]) (*connect.Response[xylona.SetDummyUpdateFailureResponse], error) {
	_, errUser := xs.getUserFromHeader(req.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	if xs.dummyTracker == nil {
		return connect.NewResponse(&xylona.SetDummyUpdateFailureResponse{}), nil
	}
	if req.Msg.GetSimulateFailure() {
		xs.dummyTracker.Reset()
		if xs.versionState != nil {
			clearDummyVersionStates(xs.versionState)
		}
	}
	xs.dummyTracker.SetSimulateFailure(req.Msg.GetSimulateFailure())
	return connect.NewResponse(&xylona.SetDummyUpdateFailureResponse{}), nil
}

func clearDummyVersionStates(states *versiontracker.VersionStateMap) {
	for serverID, state := range states.GetAll() {
		if state.TrackerType == "dummy" {
			states.Delete(serverID)
		}
	}
}

func stageRemoteVersionServer(ctx context.Context, client nodeclient.NodeClient, gs *models.GameServer) (*models.GameServer, func(), error) {
	tempDir, errMkdirTemp := os.MkdirTemp("", "xylona-remote-version-*")
	if errMkdirTemp != nil {
		return nil, nil, fmt.Errorf("create remote version temp dir: %w", errMkdirTemp)
	}

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	stagedExecutable, errStageExecutable := stageOptionalRemoteVersionFile(ctx, client, gs.Directory, tempDir, gs.ServerExecutable.GetOr(""))
	if errStageExecutable != nil {
		cleanup()
		return nil, nil, errStageExecutable
	}

	if !stagedExecutable && gs.ServerExecutable.GetOr("") != "minecraft_server.jar" {
		_, errStageFallback := stageOptionalRemoteVersionFile(ctx, client, gs.Directory, tempDir, "minecraft_server.jar")
		if errStageFallback != nil {
			cleanup()
			return nil, nil, errStageFallback
		}
	}

	errStageManifests := stageRemoteVersionManifests(ctx, client, gs.Directory, tempDir)
	if errStageManifests != nil {
		cleanup()
		return nil, nil, errStageManifests
	}

	stagedServer := *gs
	stagedServer.Directory = tempDir
	return &stagedServer, cleanup, nil
}

func stageRemoteVersionManifests(ctx context.Context, client nodeclient.NodeClient, remoteDir string, tempDir string) error {
	for _, relativeDir := range []string{"", "steamapps"} {
		entries, errList := client.ListFiles(ctx, remoteDir, relativeDir)
		if errList != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDirectory || !strings.HasPrefix(entry.Name, "appmanifest_") || !strings.HasSuffix(entry.Name, ".acf") {
				continue
			}

			relativePath := entry.Name
			if relativeDir != "" {
				relativePath = filepath.ToSlash(filepath.Join(relativeDir, entry.Name))
			}

			_, errStage := stageOptionalRemoteVersionFile(ctx, client, remoteDir, tempDir, relativePath)
			if errStage != nil {
				return errStage
			}
		}
	}

	return nil
}

func stageOptionalRemoteVersionFile(ctx context.Context, client nodeclient.NodeClient, remoteDir string, tempDir string, relativePath string) (bool, error) {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(relativePath))
	if normalizedPath == "" || !filepath.IsLocal(normalizedPath) {
		return false, nil
	}

	fileData, errRead := client.ReadFile(ctx, remoteDir, normalizedPath)
	if errRead != nil {
		if errors.Is(errRead, node.ErrInvalidPath) || errors.Is(errRead, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read remote version file %q: %w", normalizedPath, errRead)
	}

	stagedFilePath := filepath.Join(tempDir, filepath.FromSlash(normalizedPath))
	errMkdirAll := os.MkdirAll(filepath.Dir(stagedFilePath), 0o750)
	if errMkdirAll != nil {
		return false, fmt.Errorf("create remote version staging directory for %q: %w", normalizedPath, errMkdirAll)
	}

	errWrite := os.WriteFile(stagedFilePath, fileData, 0o600)
	if errWrite != nil {
		return false, fmt.Errorf("write remote version staging file %q: %w", normalizedPath, errWrite)
	}

	return true, nil
}

func versionStateToProto(state versiontracker.VersionState) *xylona.VersionInfo {
	protoStatus := xylona.VersionStatus_VERSION_STATUS_NO_TRACKER
	switch state.Status {
	case versiontracker.VersionStatusUnchecked:
		protoStatus = xylona.VersionStatus_VERSION_STATUS_UNCHECKED
	case versiontracker.VersionStatusChecking:
		protoStatus = xylona.VersionStatus_VERSION_STATUS_CHECKING
	case versiontracker.VersionStatusChecked:
		protoStatus = xylona.VersionStatus_VERSION_STATUS_CHECKED
	case versiontracker.VersionStatusError:
		protoStatus = xylona.VersionStatus_VERSION_STATUS_ERROR
	}

	var lastCheckUnix int64
	if !state.LastCheckTime.IsZero() {
		lastCheckUnix = state.LastCheckTime.Unix()
	}

	return &xylona.VersionInfo{
		InstalledVersion:      state.InstalledVersion,
		LatestVersion:         state.LatestVersion,
		UpdateAvailable:       state.UpdateAvailable,
		LastCheckTime:         lastCheckUnix,
		TrackerType:           state.TrackerType,
		Status:                protoStatus,
		InstalledVersionLabel: state.InstalledVersionLabel,
		LatestVersionLabel:    state.LatestVersionLabel,
		InstalledBranch:       state.InstalledBranch,
		LatestBranch:          state.LatestBranch,
	}
}
