package rpc

import (
	"context"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (xs *XylonaService) GetVersionInfo(ctx context.Context, req *connect.Request[xylona.GetVersionInfoRequest]) (*connect.Response[xylona.GetVersionInfoResponse], error) {
	gameServerID := req.Msg.GetGameServerId()

	state := xs.versionState.Get(gameServerID)

	// If unchecked, trigger an immediate check.
	if state.Status == versiontracker.VersionStatusUnchecked {
		xs.triggerVersionCheck(ctx, gameServerID)
		state = xs.versionState.Get(gameServerID)
	}

	return connect.NewResponse(&xylona.GetVersionInfoResponse{
		VersionInfo: versionStateToProto(state),
	}), nil
}

func (xs *XylonaService) CheckForUpdate(ctx context.Context, req *connect.Request[xylona.CheckForUpdateRequest]) (*connect.Response[xylona.CheckForUpdateResponse], error) {
	gameServerID := req.Msg.GetGameServerId()

	xs.triggerVersionCheck(ctx, gameServerID)
	state := xs.versionState.Get(gameServerID)

	return connect.NewResponse(&xylona.CheckForUpdateResponse{
		VersionInfo: versionStateToProto(state),
	}), nil
}

func (xs *XylonaService) SetDummyUpdateFailure(_ context.Context, req *connect.Request[xylona.SetDummyUpdateFailureRequest]) (*connect.Response[xylona.SetDummyUpdateFailureResponse], error) {
	if xs.dummyTracker == nil {
		return connect.NewResponse(&xylona.SetDummyUpdateFailureResponse{}), nil
	}
	xs.dummyTracker.SetSimulateFailure(req.Msg.GetSimulateFailure())
	return connect.NewResponse(&xylona.SetDummyUpdateFailureResponse{}), nil
}

func (xs *XylonaService) triggerVersionCheck(ctx context.Context, gameServerID string) {
	xs.actionsInst.CheckServerVersionByID(ctx, gameServerID)
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
		InstalledVersion: state.InstalledVersion,
		LatestVersion:    state.LatestVersion,
		UpdateAvailable:  state.UpdateAvailable,
		LastCheckTime:    lastCheckUnix,
		TrackerType:      state.TrackerType,
		Status:           protoStatus,
	}
}
