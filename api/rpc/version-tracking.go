package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetVersionInfo returns cached or resolved version metadata for a game server.
func (xs *XylonaService) GetVersionInfo(ctx context.Context, req *connect.Request[xylona.GetVersionInfoRequest]) (*connect.Response[xylona.GetVersionInfoResponse], error) {
	user, errUser := xs.getUserFromHeader(req.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	gameServerID := req.Msg.GetGameServerId()
	return dispatchGameServerRequest(
		xs,
		gameServerID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.GetVersionInfoResponse], error) {
			errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.view")
			if errPermission != nil {
				return nil, errPermission
			}

			_, versionInfo := xs.resolveLocalVersionData(ctx, gameServer, actions.VersionResolveOptions{})

			return connect.NewResponse(&xylona.GetVersionInfoResponse{
				VersionInfo: versionInfo,
			}), nil
		},
		func() (*connect.Response[xylona.GetVersionInfoResponse], error) {
			return xs.getRemoteVersionInfo(ctx, gameServerID, user)
		},
	)
}

// CheckForUpdate refreshes version metadata for a game server.
func (xs *XylonaService) CheckForUpdate(ctx context.Context, req *connect.Request[xylona.CheckForUpdateRequest]) (*connect.Response[xylona.CheckForUpdateResponse], error) {
	user, errUser := xs.getUserFromHeader(req.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	gameServerID := req.Msg.GetGameServerId()
	return dispatchGameServerRequest(
		xs,
		gameServerID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.CheckForUpdateResponse], error) {
			errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.view")
			if errPermission != nil {
				return nil, errPermission
			}

			_, versionInfo := xs.resolveLocalVersionData(ctx, gameServer, actions.VersionResolveOptions{
				ForceRefresh: true,
			})

			return connect.NewResponse(&xylona.CheckForUpdateResponse{
				VersionInfo: versionInfo,
			}), nil
		},
		func() (*connect.Response[xylona.CheckForUpdateResponse], error) {
			return xs.checkRemoteServerForUpdate(ctx, gameServerID, user)
		},
	)
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
		// Reset the dummy tracker before failure-mode tests so E2E update cases do
		// not inherit a previously "updated" version from an earlier test.
		xs.dummyTracker.Reset()
		if xs.versionState != nil {
			clearDummyVersionStates(xs.versionState)
		}
	}
	xs.dummyTracker.SetSimulateFailure(req.Msg.GetSimulateFailure())
	return connect.NewResponse(&xylona.SetDummyUpdateFailureResponse{}), nil
}

func clearDummyVersionStates(states *versiontracker.VersionStateMap) {
	// Reset only dummy-tracked entries so E2E failure toggles do not wipe unrelated
	// version state cached for other server types.
	for serverID, state := range states.GetAll() {
		if state.TrackerType == "dummy" {
			states.Delete(serverID)
		}
	}
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
