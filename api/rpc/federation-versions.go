package rpc

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (fs FederationService) getVersionState(serverID string) versiontracker.VersionState {
	if fs.versionState == nil {
		return versiontracker.VersionState{}
	}
	return fs.versionState.Get(serverID)
}

// UpdateRemoteServer updates a local game server on behalf of a federated peer.
func (fs FederationService) UpdateRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationRemoteActionResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(
		ctx,
		request.Header(),
		request.Msg.GetActingUserId(),
		request.Msg.GetOriginNodeId(),
		serverID,
		"game_server.settings",
	)
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, notFoundErr()
	}

	selectedTarget := strings.TrimSpace(request.Msg.GetTarget())
	if selectedTarget != "" {
		gs.Branch = selectedTarget
	}

	errUpdate := fs.actionsInst.UpdateGameServer(gs)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("server_id", serverID).Msg("Failed to update game server")
		return nil, federationUpdateError(errUpdate)
	}

	return connect.NewResponse(&xylona.FederationRemoteActionResponse{
		Success: true,
	}), nil
}

func federationUpdateErrorCode(err error) connect.Code {
	switch {
	case errors.Is(err, actions.ErrMinecraftVariantUpdateNotSupported),
		errors.Is(err, actions.ErrGameUpdateNotConfigured),
		errors.Is(err, actions.ErrInternalGameUpdateMissing):
		return connect.CodeFailedPrecondition
	default:
		return connect.CodeInternal
	}
}

func federationUpdateError(err error) error {
	code := federationUpdateErrorCode(err)
	if code == connect.CodeInternal {
		return connect.NewError(code, errors.New("failed to update game server"))
	}
	return connect.NewError(code, err)
}

// GetRemoteVersionInfo returns version metadata for a local server to a federated peer.
func (fs FederationService) GetRemoteVersionInfo(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationVersionInfoResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(
		ctx,
		request.Header(),
		request.Msg.GetActingUserId(),
		request.Msg.GetOriginNodeId(),
		serverID,
		"game_server.view",
	)
	if errPermission != nil {
		return nil, errPermission
	}

	_, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, notFoundErr()
	}

	state := fs.getVersionState(serverID)
	if state.Status == versiontracker.VersionStatusUnchecked || state.Status == versiontracker.VersionStatusNoTracker {
		fs.actionsInst.CheckServerVersionByID(ctx, serverID)
		state = fs.getVersionState(serverID)
	}

	return connect.NewResponse(&xylona.FederationVersionInfoResponse{
		VersionInfo: versionStateToProto(state),
	}), nil
}

// CheckRemoteServerForUpdate refreshes version metadata for a local server.
func (fs FederationService) CheckRemoteServerForUpdate(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationVersionInfoResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(
		ctx,
		request.Header(),
		request.Msg.GetActingUserId(),
		request.Msg.GetOriginNodeId(),
		serverID,
		"game_server.view",
	)
	if errPermission != nil {
		return nil, errPermission
	}

	_, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, notFoundErr()
	}

	fs.actionsInst.CheckServerVersionByID(ctx, serverID)
	state := fs.getVersionState(serverID)

	return connect.NewResponse(&xylona.FederationVersionInfoResponse{
		VersionInfo: versionStateToProto(state),
	}), nil
}
