package actions

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/ClintonCollins/Xylona/internal/controller/protomap"
	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/internal/updateconfig"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func readVersionDurationEnv(envKey string, fallback time.Duration) time.Duration {
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

// ResolveVersionData returns the raw version string plus cached or refreshed version metadata.
func (inst *Instance) ResolveVersionData(ctx context.Context, gs *models.GameServer, opts VersionResolveOptions) (string, versiontracker.VersionState) {
	rawVersion := versiontracker.ResolveCurrentVersion(gs)
	if inst.versionState == nil {
		return rawVersion, versiontracker.VersionState{}
	}

	trackerInfo := inst.resolveTrackerContextForServer(gs)
	tracker := versiontracker.ResolveTrackerWithContext(inst.resolverConfig, trackerInfo)
	if tracker == nil {
		inst.versionState.InitNoTracker(gs.ID)
		return rawVersion, inst.versionState.Get(gs.ID)
	}

	trackerType := versiontracker.TrackerTypeName(tracker)
	contextKey := trackerInfo.CacheKey()

	for {
		state, ok := inst.versionState.GetWithOK(gs.ID)
		refreshInstalled, refreshLatest := inst.versionRefreshNeeds(state, ok, opts.ForceRefresh, trackerType, contextKey)
		if !refreshInstalled && !refreshLatest {
			return rawVersion, state
		}

		if opts.AllowAsync {
			inst.startVersionRefresh(gs, tracker, trackerType, refreshInstalled, refreshLatest)
			if !ok || state.Status == versiontracker.VersionStatusUnchecked || state.Status == versiontracker.VersionStatusError {
				checkingState := state
				checkingState.Status = versiontracker.VersionStatusChecking
				checkingState.TrackerType = trackerType
				inst.versionState.Set(gs.ID, checkingState)
				return rawVersion, checkingState
			}
			return rawVersion, state
		}

		inst.runOrJoinVersionRefresh(ctx, gs, tracker, trackerType, refreshInstalled, refreshLatest)
	}
}

func (inst *Instance) resolveTrackerContextForServer(gs *models.GameServer) versiontracker.TrackerContext {
	if gs == nil {
		return versiontracker.TrackerContext{}
	}

	updateCommand := ""
	if gs.R.Game != nil {
		updateCommand = gameUpdateCommand(gs.R.Game, inst.resolveNodeOS(inst.ctx, gs.NodeID))
	}
	info := versiontracker.TrackerContext{
		GameID:        gs.GameID,
		UpdateCommand: updateCommand,
	}

	if gs.R.Game != nil {
		info.ServerSoftware = gs.R.Game.ServerSoftware.GetOr("")
		info.SteamAppID = strings.TrimSpace(gs.R.Game.SteamAppID)
	}

	if gs.R.Game != nil {
		resolved, errResolve := updateconfig.ResolveModelConfig(gs.R.Game, gs)
		if errResolve == nil {
			info.ProviderKind = string(resolved.Provider.Kind)
			info.ProviderSourceID = resolved.Provider.SourceID
			info.Target = resolved.Target
		}
	}

	return info
}

func (inst *Instance) resolveTrackerForServer(gs *models.GameServer) versiontracker.VersionTracker {
	trackerInfo := inst.resolveTrackerContextForServer(gs)
	return versiontracker.ResolveTrackerWithContext(inst.resolverConfig, trackerInfo)
}

func (inst *Instance) versionRefreshNeeds(
	state versiontracker.VersionState,
	ok bool,
	force bool,
	currentTrackerType string,
	currentContextKey string,
) (bool, bool) {
	if force || !ok {
		return true, true
	}
	if strings.TrimSpace(state.TrackerType) != strings.TrimSpace(currentTrackerType) {
		return true, true
	}
	if strings.TrimSpace(state.ContextKey) != strings.TrimSpace(currentContextKey) {
		return true, true
	}

	switch state.Status {
	case versiontracker.VersionStatusUnchecked, versiontracker.VersionStatusError:
		return true, true
	case versiontracker.VersionStatusChecking:
		return false, false
	case versiontracker.VersionStatusChecked:
		refreshInstalled := state.InstalledVersion == "" ||
			(!state.InstalledCheckTime.IsZero() && time.Since(state.InstalledCheckTime) > inst.versionInstalledTTL)
		refreshLatest := state.LatestVersion == "" ||
			(!state.LatestCheckTime.IsZero() && time.Since(state.LatestCheckTime) > inst.versionLatestTTL)
		return refreshInstalled, refreshLatest
	default:
		return true, true
	}
}

func (inst *Instance) runOrJoinVersionRefresh(ctx context.Context, gs *models.GameServer, tracker versiontracker.VersionTracker, trackerType string, refreshInstalled bool, refreshLatest bool) {
	inst.versionRefreshMu.Lock()
	if inst.versionRefreshCalls == nil {
		inst.versionRefreshCalls = make(map[string]*versionRefreshCall)
	}
	existing := inst.versionRefreshCalls[gs.ID]
	if existing == nil {
		call := &versionRefreshCall{done: make(chan struct{})}
		inst.versionRefreshCalls[gs.ID] = call
		inst.versionRefreshMu.Unlock()

		inst.refreshVersionState(ctx, gs, tracker, trackerType, refreshInstalled, refreshLatest)

		inst.versionRefreshMu.Lock()
		delete(inst.versionRefreshCalls, gs.ID)
		close(call.done)
		inst.versionRefreshMu.Unlock()
		return
	}
	inst.versionRefreshMu.Unlock()

	select {
	case <-ctx.Done():
	case <-existing.done:
	}
}

func (inst *Instance) startVersionRefresh(gs *models.GameServer, tracker versiontracker.VersionTracker, trackerType string, refreshInstalled bool, refreshLatest bool) {
	inst.versionRefreshMu.Lock()
	if inst.versionRefreshCalls == nil {
		inst.versionRefreshCalls = make(map[string]*versionRefreshCall)
	}
	if _, exists := inst.versionRefreshCalls[gs.ID]; exists {
		inst.versionRefreshMu.Unlock()
		return
	}
	call := &versionRefreshCall{done: make(chan struct{})}
	inst.versionRefreshCalls[gs.ID] = call
	inst.versionRefreshMu.Unlock()

	go func() {
		inst.refreshVersionState(inst.ctx, gs, tracker, trackerType, refreshInstalled, refreshLatest)
		inst.versionRefreshMu.Lock()
		delete(inst.versionRefreshCalls, gs.ID)
		close(call.done)
		inst.versionRefreshMu.Unlock()
	}()
}

func (inst *Instance) refreshVersionState(ctx context.Context, gs *models.GameServer, tracker versiontracker.VersionTracker, trackerType string, refreshInstalled bool, refreshLatest bool) {
	state, _ := inst.versionState.GetWithOK(gs.ID)

	installedVersion := strings.TrimSpace(state.InstalledVersion)
	latestVersion := strings.TrimSpace(state.LatestVersion)
	installedCheckTime := state.InstalledCheckTime
	latestCheckTime := state.LatestCheckTime

	now := time.Now()
	var errInstalled error
	var errLatest error

	if refreshInstalled {
		installed, getInstalledErr := tracker.GetInstalledVersion(ctx, gs)
		errInstalled = getInstalledErr
		if errInstalled == nil {
			installedVersion = strings.TrimSpace(installed)
			installedCheckTime = now
		}
	}

	if refreshLatest {
		latest, getLatestErr := tracker.GetLatestVersion(ctx, gs)
		errLatest = getLatestErr
		if errLatest == nil {
			latestVersion = strings.TrimSpace(latest)
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
		LastCheckTime:      maxVersionTime(installedCheckTime, latestCheckTime),
		InstalledCheckTime: installedCheckTime,
		LatestCheckTime:    latestCheckTime,
		TrackerType:        trackerType,
	}
	trackerInfo := inst.resolveTrackerContextForServer(gs)
	newState.ContextKey = trackerInfo.CacheKey()
	versiontracker.EnrichVersionState(ctx, tracker, gs, &newState)

	if newState.UpdateAvailable && !state.UpdateAvailable {
		eventbus.Get().Publish("server.update_available", gs.ID)
	}

	inst.versionState.Set(gs.ID, newState)
	inst.publishVersionState(gs, newState)
}

func maxVersionTime(a time.Time, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func (inst *Instance) publishVersionState(gs *models.GameServer, state versiontracker.VersionState) {
	protoState := protomap.VersionStateToProto(state)
	rawVersion := versiontracker.ResolveCurrentVersion(gs)

	if inst.versionBroadcaster != nil {
		inst.versionBroadcaster.BroadcastGameServerVersion(gs.ID, rawVersion, protoState)
	}
}
