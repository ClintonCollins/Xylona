package rpc

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/supervisor"
)

// ExchangePeerList exchanges known peer metadata with another federation node.
func (fs FederationService) ExchangePeerList(
	ctx context.Context,
	request *connect.Request[xylona.ExchangePeerListRequest],
) (*connect.Response[xylona.ExchangePeerListResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	localPeers, errBuild := fs.actionsInst.BuildLocalPeerList()
	if errBuild != nil {
		log.Error().Err(errBuild).Msg("Failed to build local peer list for exchange")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to build peer list"))
	}

	// Trigger auto-pairing for unknown peers in the background.
	go fs.actionsInst.ProcessReceivedPeerList(request.Msg.GetPeers(), request.Msg.GetSenderNodeId())

	return connect.NewResponse(&xylona.ExchangePeerListResponse{
		Peers: localPeers,
	}), nil
}

// NotifyPeerChange processes a peer change broadcast from another federation node.
func (fs FederationService) NotifyPeerChange(
	ctx context.Context,
	request *connect.Request[xylona.NotifyPeerChangeRequest],
) (*connect.Response[xylona.NotifyPeerChangeResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	go fs.actionsInst.HandlePeerChange(request.Msg)

	return connect.NewResponse(&xylona.NotifyPeerChangeResponse{}), nil
}

// NotifyDeparture records a remote node departure notification.
func (fs FederationService) NotifyDeparture(
	ctx context.Context,
	request *connect.Request[xylona.NotifyDepartureRequest],
) (*connect.Response[xylona.NotifyDepartureResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	go fs.actionsInst.HandleNodeDeparture(request.Msg.GetNodeId(), request.Msg.GetReason())

	return connect.NewResponse(&xylona.NotifyDepartureResponse{}), nil
}

// StartRemoteServer starts a local game server on behalf of a federated peer.
func (fs FederationService) StartRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationRemoteActionResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	if serverID == "" {
		return nil, invalidArg("server_id is required")
	}
	errPermission := fs.authorizeFederatedPermission(
		ctx,
		request.Header(),
		request.Msg.GetActingUserId(),
		request.Msg.GetOriginNodeId(),
		serverID,
		"game_server.start",
	)
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, dbLookup(errGet)
	}

	fs.actionsInst.StartGameServer(gs)

	return connect.NewResponse(&xylona.FederationRemoteActionResponse{
		Success: true,
	}), nil
}

// StopRemoteServer stops a local game server on behalf of a federated peer.
func (fs FederationService) StopRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationRemoteActionResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	if serverID == "" {
		return nil, invalidArg("server_id is required")
	}
	errPermission := fs.authorizeFederatedPermission(
		ctx,
		request.Header(),
		request.Msg.GetActingUserId(),
		request.Msg.GetOriginNodeId(),
		serverID,
		"game_server.stop",
	)
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, dbLookup(errGet)
	}

	fs.actionsInst.StopGameServer(gs)

	return connect.NewResponse(&xylona.FederationRemoteActionResponse{
		Success: true,
	}), nil
}

// RestartRemoteServer restarts a local game server on behalf of a federated peer.
func (fs FederationService) RestartRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationRemoteActionResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	if serverID == "" {
		return nil, invalidArg("server_id is required")
	}
	errPermission := fs.authorizeFederatedPermission(
		ctx,
		request.Header(),
		request.Msg.GetActingUserId(),
		request.Msg.GetOriginNodeId(),
		serverID,
		"game_server.restart",
	)
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, dbLookup(errGet)
	}

	fs.actionsInst.StopGameServer(gs)
	fs.actionsInst.StartGameServer(gs)

	return connect.NewResponse(&xylona.FederationRemoteActionResponse{
		Success: true,
	}), nil
}

// buildServerSnapshot constructs a FederationServerSnapshot containing the current
// state of all local game servers, including live status and metrics from the supervisor.
func (fs FederationService) buildServerSnapshot() (*xylona.FederationServerSnapshot, error) {
	gameServers, errGetServers := fs.db.GetAllGameServers()
	if errGetServers != nil {
		if errors.Is(errGetServers, sql.ErrNoRows) {
			return &xylona.FederationServerSnapshot{}, nil
		}
		return nil, internalErrf("failed to get game servers")
	}

	snapshot := &xylona.FederationServerSnapshot{
		Servers: make([]*xylona.FederationServerState, 0, len(gameServers)),
	}

	for _, gs := range gameServers {
		status := fs.resolveFederatedServerStatus(gs)

		gameName := ""
		if gs.R.Game != nil {
			gameName = gs.R.Game.Name
		}

		serverState := &xylona.FederationServerState{
			ServerId:       gs.ID,
			Status:         status,
			DisplayName:    gs.Name,
			GameName:       gameName,
			GameId:         gs.GameID,
			IpAddress:      gs.IP,
			Port:           helpers.ClampInt32FromInt64(gs.Port),
			QueryPort:      helpers.ClampInt32FromInt64(gs.QueryPort),
			MaxPlayers:     helpers.ClampInt32FromInt64(gs.MaxPlayers),
			CurrentPlayers: 0,
			MapName:        gs.Map,
			Version:        resolveGameServerVersion(gs),
		}
		serverState.Version, serverState.VersionInfo = fs.resolveLocalVersionData(fs.ctx, gs, actions.VersionResolveOptions{
			AllowAsync: true,
		})

		serverState.Metrics = buildServerStateMetrics(fs.supervisorInst, gs.ID)
		snapshot.Servers = append(snapshot.Servers, serverState)
	}

	return snapshot, nil
}

// buildServerStateMetrics returns a GameServerMetrics proto populated from the
// supervisor command for the given server, or nil if no command is running.
func buildServerStateMetrics(supervisorInst *supervisor.Instance, serverID string) *xylona.GameServerMetrics {
	if supervisorInst == nil {
		return nil
	}
	cmd, errGetCmd := supervisorInst.GetCommandByID(serverID)
	if errGetCmd != nil {
		return nil
	}
	cpuPct, memRSS, memVMS, memPct, cpuCores, threads, diskBytes, ioRead, ioWrite, connCount := cmd.Metrics()
	metrics := &xylona.GameServerMetrics{
		CpuPercent:            cpuPct,
		MemoryBytes:           helpers.ClampInt64FromUint64(memVMS),
		MemoryWorkingSetBytes: helpers.ClampInt64FromUint64(memRSS),
		MemoryPercent:         float64(memPct),
		CpuCores:              cpuCores,
		NumberOfThreads:       threads,
		DiskUsageBytes:        helpers.ClampInt64FromUint64(diskBytes),
		IoReadRate:            ioRead,
		IoWriteRate:           ioWrite,
		ConnectionCount:       connCount,
	}
	startedAt := cmd.UnixStartedAt()
	if startedAt > 0 {
		metrics.UptimeSeconds = time.Now().Unix() - startedAt
	}
	return metrics
}
