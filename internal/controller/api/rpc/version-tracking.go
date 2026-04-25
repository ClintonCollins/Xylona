package rpc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/updateconfig"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
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
	if xs.versionState == nil {
		return "", nil, nil
	}
	if opts.AllowAsync {
		rawVersion, versionInfo := xs.resolveRemoteVersionDataAsync(gs, opts)
		return rawVersion, versionInfo, nil
	}

	state, errResolve := xs.resolveRemoteVersionState(ctx, gs, opts.ForceRefresh)
	if errResolve != nil {
		return "", nil, errResolve
	}
	return state.InstalledVersion, versionStateToProto(state), nil
}

func (xs *XylonaService) resolveRemoteVersionDataAsync(gs *models.GameServer, opts actions.VersionResolveOptions) (string, *xylona.VersionInfo) {
	state, ok := xs.versionState.GetWithOK(gs.ID)
	rawVersion := resolveRemoteDisplayVersion(gs, state, ok)

	trackerInfo := xs.remoteVersionTrackerContext(gs)
	tracker := versiontracker.ResolveTrackerWithContext(versiontracker.ResolverConfig{}, trackerInfo)
	if tracker == nil {
		xs.versionState.InitNoTracker(gs.ID)
		state = xs.versionState.Get(gs.ID)
		return rawVersion, versionStateToProto(state)
	}

	trackerType := versiontracker.TrackerTypeName(tracker)
	contextKey := trackerInfo.CacheKey()
	refreshInstalled, refreshLatest := remoteVersionRefreshNeeds(state, ok, opts.ForceRefresh, trackerType, contextKey)
	if !refreshInstalled && !refreshLatest {
		return rawVersion, versionStateToProto(state)
	}

	xs.startRemoteVersionRefresh(gs, opts.ForceRefresh)

	if !ok || state.Status == versiontracker.VersionStatusUnchecked || state.Status == versiontracker.VersionStatusError {
		checkingState := state
		checkingState.Status = versiontracker.VersionStatusChecking
		checkingState.TrackerType = trackerType
		checkingState.ContextKey = contextKey
		xs.versionState.Set(gs.ID, checkingState)
		return rawVersion, versionStateToProto(checkingState)
	}

	return rawVersion, versionStateToProto(state)
}

func resolveRemoteDisplayVersion(gs *models.GameServer, state versiontracker.VersionState, ok bool) string {
	if ok && strings.TrimSpace(state.InstalledVersion) != "" {
		return strings.TrimSpace(state.InstalledVersion)
	}

	return strings.TrimSpace(gs.Version)
}

func remoteVersionRefreshNeeds(
	state versiontracker.VersionState,
	ok bool,
	force bool,
	trackerType string,
	contextKey string,
) (bool, bool) {
	if force || !ok {
		return true, true
	}
	if strings.TrimSpace(state.TrackerType) != strings.TrimSpace(trackerType) {
		return true, true
	}
	if strings.TrimSpace(state.ContextKey) != strings.TrimSpace(contextKey) {
		return true, true
	}

	switch state.Status {
	case versiontracker.VersionStatusNoTracker:
		return false, false
	case versiontracker.VersionStatusUnchecked, versiontracker.VersionStatusError:
		latestTTL := readRemoteVersionDurationEnv("XYLONA_VERSION_LATEST_TTL", 2*time.Minute)
		refreshLatest := state.LatestVersion == "" ||
			(!state.LatestCheckTime.IsZero() && time.Since(state.LatestCheckTime) > latestTTL)
		return state.InstalledVersion == "", refreshLatest
	case versiontracker.VersionStatusChecking:
		return state.InstalledVersion == "", state.LatestVersion == ""
	case versiontracker.VersionStatusChecked:
		installedTTL := readRemoteVersionDurationEnv("XYLONA_VERSION_INSTALLED_TTL", 15*time.Second)
		latestTTL := readRemoteVersionDurationEnv("XYLONA_VERSION_LATEST_TTL", 2*time.Minute)

		refreshInstalled := state.InstalledVersion == "" ||
			(!state.InstalledCheckTime.IsZero() && time.Since(state.InstalledCheckTime) > installedTTL)
		refreshLatest := state.LatestVersion == "" ||
			(!state.LatestCheckTime.IsZero() && time.Since(state.LatestCheckTime) > latestTTL)
		return refreshInstalled, refreshLatest
	default:
		return true, true
	}
}

func readRemoteVersionDurationEnv(envKey string, fallback time.Duration) time.Duration {
	envValue := strings.TrimSpace(os.Getenv(envKey))
	if envValue == "" {
		return fallback
	}

	parsed, errParse := time.ParseDuration(envValue)
	if errParse != nil || parsed <= 0 {
		return fallback
	}

	return parsed
}

func (xs *XylonaService) startRemoteVersionRefresh(gs *models.GameServer, force bool) {
	xs.remoteVersionRefreshMu.Lock()
	if xs.remoteVersionRefreshCalls == nil {
		xs.remoteVersionRefreshCalls = make(map[string]*remoteVersionRefreshCall)
	}
	if _, exists := xs.remoteVersionRefreshCalls[gs.ID]; exists {
		xs.remoteVersionRefreshMu.Unlock()
		return
	}

	call := &remoteVersionRefreshCall{done: make(chan struct{})}
	xs.remoteVersionRefreshCalls[gs.ID] = call
	xs.remoteVersionRefreshMu.Unlock()

	go func() {
		xs.refreshRemoteVersionState(gs, force)

		xs.remoteVersionRefreshMu.Lock()
		delete(xs.remoteVersionRefreshCalls, gs.ID)
		close(call.done)
		xs.remoteVersionRefreshMu.Unlock()
	}()
}

func (xs *XylonaService) refreshRemoteVersionState(gs *models.GameServer, force bool) {
	_, errRefresh := xs.resolveRemoteVersionState(xs.ctx, gs, force)
	if errRefresh != nil {
		xs.setRemoteVersionRefreshError(gs)
	}
}

func (xs *XylonaService) setRemoteVersionRefreshError(gs *models.GameServer) {
	if xs.versionState == nil || gs == nil {
		return
	}

	state, _ := xs.versionState.GetWithOK(gs.ID)
	state.Status = versiontracker.VersionStatusError
	xs.versionState.Set(gs.ID, state)
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

func (xs *XylonaService) resolveRemoteVersionState(ctx context.Context, gs *models.GameServer, force bool) (versiontracker.VersionState, error) {
	trackerInfo := xs.remoteVersionTrackerContext(gs)
	tracker := versiontracker.ResolveTrackerWithContext(versiontracker.ResolverConfig{}, trackerInfo)
	if tracker == nil {
		xs.versionState.InitNoTracker(gs.ID)
		return xs.versionState.Get(gs.ID), nil
	}

	trackerType := versiontracker.TrackerTypeName(tracker)
	contextKey := trackerInfo.CacheKey()
	state, ok := xs.versionState.GetWithOK(gs.ID)
	refreshInstalled, refreshLatest := remoteVersionRefreshNeeds(state, ok, force, trackerType, contextKey)
	if !refreshInstalled && !refreshLatest {
		return state, nil
	}

	client, errClient := xs.resolveNodeClient(gs)
	if errClient != nil {
		return versiontracker.VersionState{}, errClient
	}

	installedVersion := strings.TrimSpace(state.InstalledVersion)
	latestVersion := strings.TrimSpace(state.LatestVersion)
	installedCheckTime := state.InstalledCheckTime
	latestCheckTime := state.LatestCheckTime
	now := time.Now()
	var errInstalled error
	var errLatest error

	if refreshInstalled {
		installedVersion, errInstalled = xs.readRemoteInstalledVersion(ctx, client, tracker, trackerType, trackerInfo, gs)
		installedVersion = strings.TrimSpace(installedVersion)
		if errInstalled == nil {
			installedCheckTime = now
		}
	}

	if refreshLatest {
		latestVersion, errLatest = tracker.GetLatestVersion(ctx, gs)
		latestVersion = strings.TrimSpace(latestVersion)
		if errLatest == nil {
			latestCheckTime = now
		}
	}

	status := versiontracker.VersionStatusChecked
	if errInstalled != nil || errLatest != nil {
		status = versiontracker.VersionStatusError
	}

	newState := versiontracker.VersionState{
		Status:             status,
		InstalledVersion:   installedVersion,
		LatestVersion:      latestVersion,
		UpdateAvailable:    installedVersion != "" && latestVersion != "" && installedVersion != latestVersion,
		LastCheckTime:      maxRemoteVersionTime(installedCheckTime, latestCheckTime),
		InstalledCheckTime: installedCheckTime,
		LatestCheckTime:    latestCheckTime,
		TrackerType:        trackerType,
		ContextKey:         contextKey,
	}
	enrichRemoteVersionState(ctx, tracker, trackerInfo, trackerType, gs, &newState)

	xs.versionState.Set(gs.ID, newState)
	return newState, nil
}

func (xs *XylonaService) remoteVersionTrackerContext(gs *models.GameServer) versiontracker.TrackerContext {
	info := versiontracker.TrackerContext{GameID: gs.GameID}
	if gs.R.Game == nil {
		return info
	}

	info.UpdateCommand = updateCommandForNodeGOOS(gs.R.Game, xs.resolveNodeGOOS(gs.NodeID))
	info.ServerSoftware = gs.R.Game.ServerSoftware.GetOr("")
	info.SteamAppID = strings.TrimSpace(gs.R.Game.SteamAppID)

	resolved, errResolve := updateconfig.ResolveModelConfig(gs.R.Game, gs)
	if errResolve == nil {
		info.ProviderKind = string(resolved.Provider.Kind)
		info.ProviderSourceID = resolved.Provider.SourceID
		info.Target = resolved.Target
	}
	return info
}

func updateCommandForNodeGOOS(game *models.Game, nodeGOOS string) string {
	if game == nil {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(nodeGOOS), "windows") {
		return game.WindowsUpdateCommand
	}
	return game.LinuxUpdateCommand
}

func (xs *XylonaService) readRemoteInstalledVersion(
	ctx context.Context,
	client nodeclient.NodeClient,
	tracker versiontracker.VersionTracker,
	trackerType string,
	trackerInfo versiontracker.TrackerContext,
	gs *models.GameServer,
) (string, error) {
	switch trackerType {
	case "dummy":
		version, errInstalled := tracker.GetInstalledVersion(ctx, gs)
		if errInstalled != nil {
			return "", fmt.Errorf("read remote dummy installed version: %w", errInstalled)
		}
		return version, nil
	case "minecraft":
		return probeRemoteMinecraftInstalledVersion(ctx, client, gs)
	case "steam":
		return probeRemoteSteamInstalledVersion(ctx, client, trackerInfo, gs)
	default:
		return "", nil
	}
}

func probeRemoteMinecraftInstalledVersion(ctx context.Context, client nodeclient.NodeClient, gs *models.GameServer) (string, error) {
	candidates := []string{}
	executable := strings.TrimSpace(gs.ServerExecutable.GetOr(""))
	if executable != "" {
		candidates = append(candidates, executable)
	}
	if executable != "minecraft_server.jar" {
		candidates = append(candidates, "minecraft_server.jar")
	}

	result, errProbe := client.ProbeInstalledVersion(ctx, node.InstalledVersionProbeRequest{
		Directory:     gs.Directory,
		Kind:          node.InstalledVersionProbeKindMinecraftJar,
		RelativePaths: candidates,
	})
	if errProbe != nil {
		return strings.TrimSpace(gs.Version), fmt.Errorf("probe remote minecraft installed version: %w", errProbe)
	}
	if result.Found {
		return result.Version, nil
	}
	return strings.TrimSpace(gs.Version), nil
}

func probeRemoteSteamInstalledVersion(ctx context.Context, client nodeclient.NodeClient, trackerInfo versiontracker.TrackerContext, gs *models.GameServer) (string, error) {
	result, errProbe := client.ProbeInstalledVersion(ctx, node.InstalledVersionProbeRequest{
		Directory:           gs.Directory,
		Kind:                node.InstalledVersionProbeKindSteamManifest,
		PreferredSteamAppID: strings.TrimSpace(trackerInfo.SteamAppID),
	})
	if errProbe != nil {
		return "", fmt.Errorf("probe remote steam installed version: %w", errProbe)
	}
	if result.Found {
		return result.Version, nil
	}
	return "", nil
}

func enrichRemoteVersionState(
	ctx context.Context,
	tracker versiontracker.VersionTracker,
	trackerInfo versiontracker.TrackerContext,
	trackerType string,
	gs *models.GameServer,
	state *versiontracker.VersionState,
) {
	state.InstalledVersionLabel = state.InstalledVersion
	state.LatestVersionLabel = state.LatestVersion
	if trackerType == "minecraft" || (trackerType == "steam" && strings.TrimSpace(trackerInfo.SteamAppID) != "") {
		versiontracker.EnrichVersionState(ctx, tracker, gs, state)
	}
}

func maxRemoteVersionTime(a time.Time, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
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
