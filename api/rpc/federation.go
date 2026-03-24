package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/pkg/version"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

const (
	federationRequestTimeout = 15 * time.Second
)

// streamMetricsInterval and streamHeartbeatInterval control the tick rates for
// the metrics and heartbeat events sent by StreamServerUpdates. They are
// package-level variables so that tests can override them to avoid long waits.
var (
	streamMetricsInterval   = 3 * time.Second
	streamHeartbeatInterval = 30 * time.Second
)

// FederationService handles node-to-node federation API calls.
type FederationService struct {
	ctx              context.Context
	db               *db.Connection
	actionsInst      *actions.Instance
	supervisorInst   *supervisor.Instance
	versionState     *versiontracker.VersionStateMap
	allPermissionIDs []string
}

func NewFederationService(
	ctx context.Context,
	dbInst *db.Connection,
	actionsInst *actions.Instance,
	supervisorInst *supervisor.Instance,
	versionState *versiontracker.VersionStateMap,
) *FederationService {
	allPerms, errPerms := dbInst.GetAllPermissions()
	if errPerms != nil {
		log.Fatal().Err(errPerms).Msg("Failed to load permission IDs")
	}
	permIDs := make([]string, len(allPerms))
	for i, p := range allPerms {
		permIDs[i] = p.ID
	}

	return &FederationService{
		ctx:              ctx,
		db:               dbInst,
		actionsInst:      actionsInst,
		supervisorInst:   supervisorInst,
		versionState:     versionState,
		allPermissionIDs: permIDs,
	}
}

func (fs FederationService) authenticateRequest(ctx context.Context) (FederationPeerIdentity, error) {
	identity, okIdentity := federationPeerIdentityFromContext(ctx)
	if !okIdentity {
		return FederationPeerIdentity{}, errors.New("federation peer identity is required")
	}
	if strings.TrimSpace(identity.NodeID) == "" {
		return FederationPeerIdentity{}, errors.New("federation peer node configuration is required")
	}
	return identity, nil
}

func (fs FederationService) resolveFederatedServerStatus(gameServer *models.GameServer) xylona.Status {
	status := helpers.GameServerModelStatusToProtoStatus(gameServer.Status)
	if fs.supervisorInst == nil {
		if status == xylona.Status_ONLINE {
			return xylona.Status_OFFLINE
		}
		return status
	}

	gameServerCmd, errGetCommand := fs.supervisorInst.GetCommandByID(gameServer.ID)
	if errGetCommand == nil {
		return gameServerCmd.Status()
	}

	// Prevent stale persisted ONLINE from surviving process restarts.
	if status == xylona.Status_ONLINE {
		return xylona.Status_OFFLINE
	}

	return status
}

func (fs FederationService) authorizeFederatedPermission(
	ctx context.Context,
	header http.Header,
	actingUserID string,
	originNodeID string,
	serverID string,
	permissionID string,
) error {
	peerIdentity, okIdentity := federationPeerIdentityFromContext(ctx)
	if !okIdentity || strings.TrimSpace(peerIdentity.NodeID) == "" {
		log.Warn().
			Str("server_id", serverID).
			Str("permission_id", permissionID).
			Msg("federation request missing authenticated peer identity")
		return connect.NewError(connect.CodePermissionDenied, errors.New("authenticated federation peer identity is required"))
	}

	if strings.TrimSpace(actingUserID) == "" || strings.TrimSpace(originNodeID) == "" {
		headerUserID, headerOriginNodeID := helpers.GetFederatedActingIdentity(header)
		actingUserID = strings.TrimSpace(headerUserID)
		originNodeID = strings.TrimSpace(headerOriginNodeID)
	}

	if actingUserID == "" {
		log.Warn().
			Str("server_id", serverID).
			Str("permission_id", permissionID).
			Msg("federation request missing acting user identity, denying by default")
		return connect.NewError(connect.CodePermissionDenied, errors.New("acting user identity is required for federated actions"))
	}

	if originNodeID != "" && originNodeID != peerIdentity.NodeID && originNodeID != peerIdentity.PeerNodeID {
		log.Warn().
			Str("server_id", serverID).
			Str("permission_id", permissionID).
			Str("origin_node_id", originNodeID).
			Str("authenticated_node_id", peerIdentity.NodeID).
			Str("authenticated_peer_node_id", peerIdentity.PeerNodeID).
			Msg("federation request acting origin does not match authenticated peer")
		return connect.NewError(connect.CodePermissionDenied, errors.New("acting origin node is invalid"))
	}

	if helpers.FederatedActingIsSuperUser(header) {
		if originNodeID == "" {
			log.Warn().
				Str("server_id", serverID).
				Str("permission_id", permissionID).
				Str("acting_user_id", actingUserID).
				Msg("federation super-user request missing origin node")
			return connect.NewError(connect.CodePermissionDenied, errors.New("acting origin node is required for super-user federated actions"))
		}
		return nil
	}

	allowed, errPermission := fs.db.FederatedUserHasPermissionOnServer(peerIdentity.NodeID, actingUserID, serverID, permissionID)
	if errPermission != nil {
		log.Error().
			Err(errPermission).
			Str("server_id", serverID).
			Str("origin_node_id", originNodeID).
			Str("authenticated_node_id", peerIdentity.NodeID).
			Str("acting_user_id", actingUserID).
			Str("permission_id", permissionID).
			Msg("failed to verify federated permission")
		return connect.NewError(connect.CodeInternal, errors.New("failed to verify federated permission"))
	}
	if !allowed {
		return connect.NewError(connect.CodePermissionDenied, errors.New("federated user does not have permission"))
	}

	return nil
}

func (fs FederationService) Handshake(ctx context.Context, request *connect.Request[xylona.FederationHandshakeRequest]) (*connect.Response[xylona.FederationHandshakeResponse], error) {
	peerIdentity, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		log.Warn().Str("peer", request.Peer().Addr).Msg("Federation handshake authentication failed")
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	localSettings, errSettings := fs.db.GetLocalSettings()
	if errSettings != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get local settings"))
	}

	localNode, errNode := fs.db.GetNodeByID(localSettings.NodeID)
	if errNode != nil && !errors.Is(errNode, sql.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get local node"))
	}

	nodeName := "Xylona Node"
	if localNode != nil {
		nodeName = localNode.Name
	}

	// If this node has departed the federation, signal it to the peer.
	if localNode != nil && localNode.Departed {
		resp := &xylona.FederationHandshakeResponse{
			NodeId:   localSettings.NodeID,
			NodeName: nodeName,
			Departed: true,
		}
		return connect.NewResponse(resp), nil
	}

	resp := &xylona.FederationHandshakeResponse{
		NodeId:          localSettings.NodeID,
		NodeName:        nodeName,
		Version:         version.SystemVersion,
		ProtocolVersion: version.FederationProtocolVersion,
		Capabilities:    version.FederationCapabilities,
		ServerTime:      timestamppb.New(time.Now()),
	}

	log.Info().
		Str("peer", request.Peer().Addr).
		Str("peer_node_id", peerIdentity.PeerNodeID).
		Msg("Federation handshake successful")
	return connect.NewResponse(resp), nil
}

func (fs FederationService) ListServerSummaries(ctx context.Context, request *connect.Request[xylona.FederationListServerSummariesRequest]) (*connect.Response[xylona.FederationListServerSummariesResponse], error) {
	peerIdentity, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	actingUserID, originNodeID := helpers.GetFederatedActingIdentity(request.Header())
	actingUserID = strings.TrimSpace(actingUserID)
	originNodeID = strings.TrimSpace(originNodeID)
	actingIsSuperUser := helpers.FederatedActingIsSuperUser(request.Header())
	if (actingUserID != "" || originNodeID != "") && (actingUserID == "" || originNodeID == "") {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("federated acting identity is invalid"))
	}
	if actingIsSuperUser && (actingUserID == "" || originNodeID == "") {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("super-user federated identity is invalid"))
	}
	if originNodeID != "" && originNodeID != peerIdentity.NodeID && originNodeID != peerIdentity.PeerNodeID {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("federated acting origin is invalid"))
	}

	gameServers, errGetServers := fs.db.GetAllGameServers()
	if errGetServers != nil {
		if errors.Is(errGetServers, sql.ErrNoRows) {
			return connect.NewResponse(&xylona.FederationListServerSummariesResponse{}), nil
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get game servers"))
	}

	resp := &xylona.FederationListServerSummariesResponse{}
	for _, gs := range gameServers {
		if actingUserID != "" && !actingIsSuperUser {
			allowed, errPermission := fs.db.FederatedUserHasPermissionOnServer(peerIdentity.NodeID, actingUserID, gs.ID, "game_server.view")
			if errPermission != nil {
				log.Error().
					Err(errPermission).
					Str("server_id", gs.ID).
					Str("origin_node_id", originNodeID).
					Str("authenticated_node_id", peerIdentity.NodeID).
					Str("acting_user_id", actingUserID).
					Msg("failed to evaluate federated view permission for server summary")
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to evaluate permissions"))
			}
			if !allowed {
				continue
			}
		}

		status := fs.resolveFederatedServerStatus(gs)

		gameName := ""
		if gs.R.Game != nil {
			gameName = gs.R.Game.Name
		}

		summary := &xylona.FederationServerSummary{
			ServerId:       gs.ID,
			DisplayName:    gs.Name,
			Status:         status,
			GameName:       gameName,
			GameId:         gs.GameID,
			IpAddress:      gs.IP,
			Port:           gs.Port,
			QueryPort:      gs.QueryPort,
			MaxPlayers:     gs.MaxPlayers,
			CurrentPlayers: 0,
			MapName:        gs.Map,
			Version:        resolveGameServerVersion(gs),
			UpdatedAt:      timestamppb.New(gs.UpdatedAt),
		}
		summary.Version, summary.VersionInfo = fs.resolveLocalVersionData(ctx, gs, actions.VersionResolveOptions{
			AllowAsync: true,
		})
		populateFederationSummaryMetrics(summary, fs.supervisorInst, gs.ID)
		resp.Servers = append(resp.Servers, summary)
	}

	return connect.NewResponse(resp), nil
}

func (fs FederationService) ListUserSummaries(ctx context.Context, request *connect.Request[xylona.FederationListUserSummariesRequest]) (*connect.Response[xylona.FederationListUserSummariesResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	users, errUsers := fs.db.GetAllUsers()
	if errUsers != nil {
		if errors.Is(errUsers, sql.ErrNoRows) {
			return connect.NewResponse(&xylona.FederationListUserSummariesResponse{}), nil
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list users"))
	}

	limit := int(request.Msg.GetLimit())
	limit = max(limit, 0)

	resp := &xylona.FederationListUserSummariesResponse{}
	for _, user := range users {
		if user.SuperUser {
			continue
		}
		if limit > 0 && len(resp.Users) >= limit {
			break
		}

		resp.Users = append(resp.Users, &xylona.FederationUserSummary{
			UserId:    user.ID,
			UserName:  user.UserName,
			Email:     user.Email,
			FirstName: user.FirstName,
			LastName:  user.LastName,
			CreatedAt: timestamppb.New(user.CreatedAt),
			UpdatedAt: timestamppb.New(user.UpdatedAt),
		})
	}

	return connect.NewResponse(resp), nil
}

func (fs FederationService) GetServerDetail(ctx context.Context, request *connect.Request[xylona.FederationGetServerDetailRequest]) (*connect.Response[xylona.FederationGetServerDetailResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.view")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get server"))
	}

	status := fs.resolveFederatedServerStatus(gs)

	gameName := ""
	if gs.R.Game != nil {
		gameName = gs.R.Game.Name
	}

	summary := &xylona.FederationServerSummary{
		ServerId:    gs.ID,
		DisplayName: gs.Name,
		Status:      status,
		GameName:    gameName,
		GameId:      gs.GameID,
		IpAddress:   gs.IP,
		Port:        gs.Port,
		QueryPort:   gs.QueryPort,
		MaxPlayers:  gs.MaxPlayers,
		MapName:     gs.Map,
		Version:     resolveGameServerVersion(gs),
		UpdatedAt:   timestamppb.New(gs.UpdatedAt),
	}
	summary.Version, summary.VersionInfo = fs.resolveLocalVersionData(ctx, gs, actions.VersionResolveOptions{
		AllowAsync: true,
	})

	populateFederationSummaryMetrics(summary, fs.supervisorInst, gs.ID)

	actingUserID, originNodeID := helpers.GetFederatedActingIdentity(request.Header())
	isSuperUser := helpers.FederatedActingIsSuperUser(request.Header())

	var effectivePerms []string
	if isSuperUser {
		effectivePerms = fs.allPermissionIDs
	} else if actingUserID != "" && originNodeID != "" {
		effectivePerms, _ = fs.db.GetFederatedUserPermissionIDsForServer(originNodeID, actingUserID, serverID)
	}

	return connect.NewResponse(&xylona.FederationGetServerDetailResponse{
		Server:               summary,
		EffectivePermissions: effectivePerms,
	}), nil
}

// populateFederationSummaryMetrics fills metric fields on a FederationServerSummary
// from the supervisor's running command for the given serverID.
// It is a no-op when supervisorInst is nil or no command exists for the server.
func populateFederationSummaryMetrics(summary *xylona.FederationServerSummary, supervisorInst *supervisor.Instance, serverID string) {
	if supervisorInst == nil {
		return
	}
	cmd, errGetCmd := supervisorInst.GetCommandByID(serverID)
	if errGetCmd != nil {
		return
	}
	cpuPct, memRSS, memVMS, memPct, cpuCores, threads, diskBytes, ioRead, ioWrite, connCount := cmd.Metrics()
	summary.CpuPercent = int64(cpuPct)
	summary.MemoryBytes = int64(memVMS)
	summary.MemoryWorkingSetBytes = int64(memRSS)
	summary.MemoryPercent = float64(memPct)
	summary.CpuCores = cpuCores
	summary.NumberOfThreads = threads
	summary.DiskUsageBytes = int64(diskBytes)
	summary.IoReadRate = ioRead
	summary.IoWriteRate = ioWrite
	summary.ConnectionCount = connCount
	startedAt := cmd.UnixStartedAt()
	if startedAt > 0 {
		summary.UptimeSeconds = time.Now().Unix() - startedAt
	}
}

func (fs FederationService) StartRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationRemoteActionResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
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
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	fs.actionsInst.StartGameServer(gs)

	return connect.NewResponse(&xylona.FederationRemoteActionResponse{
		Success: true,
	}), nil
}

func (fs FederationService) StopRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationRemoteActionResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
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
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	fs.actionsInst.StopGameServer(gs)

	return connect.NewResponse(&xylona.FederationRemoteActionResponse{
		Success: true,
	}), nil
}

func (fs FederationService) RestartRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationRemoteActionResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
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
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	fs.actionsInst.StopGameServer(gs)
	fs.actionsInst.StartGameServer(gs)

	return connect.NewResponse(&xylona.FederationRemoteActionResponse{
		Success: true,
	}), nil
}

func (fs FederationService) StreamConsoleOutput(ctx context.Context, request *connect.Request[xylona.FederationStreamConsoleRequest], stream *connect.ServerStream[xylona.FederationConsoleOutputChunk]) error {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.console")
	if errPermission != nil {
		return errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return connect.NewError(connect.CodeNotFound, errors.New("server not found"))
		}
		return connect.NewError(connect.CodeInternal, errors.New("failed to get server"))
	}

	command := fs.supervisorInst.GetCommandByIDOrCreateShell(gs.ID)
	listenerID := fmt.Sprintf("federation-%s", uuid.New().String())
	outputChan := make(chan *xylona.Message, 64)
	command.AddOutputListener(listenerID, outputChan)
	defer command.RemoveOutputListener(listenerID)

	log.Debug().Str("server_id", serverID).Str("listener_id", listenerID).Msg("Federation console stream started")

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-fs.ctx.Done():
			return nil
		case msg := <-outputChan:
			if msg.GameServerConsoleOutput == nil {
				continue
			}
			errSend := stream.Send(&xylona.FederationConsoleOutputChunk{
				ServerId: serverID,
				Output:   msg.GameServerConsoleOutput.Output,
			})
			if errSend != nil {
				log.Debug().Err(errSend).Str("server_id", serverID).Msg("Federation console stream send failed")
				return nil
			}
		}
	}
}

func (fs FederationService) SendConsoleInput(ctx context.Context, request *connect.Request[xylona.FederationSendConsoleInputRequest]) (*connect.Response[xylona.FederationSendConsoleInputResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.console")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get server"))
	}

	command, errGetCommand := fs.supervisorInst.GetCommandByID(gs.ID)
	if errGetCommand != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("game server not running"))
	}

	status := command.Status()
	if status == xylona.Status_OFFLINE || status == xylona.Status_UNKNOWN {
		return connect.NewResponse(&xylona.FederationSendConsoleInputResponse{
			Success: false,
			Error:   "game server is not running",
		}), nil
	}

	errSend := command.SendInput(request.Msg.GetInput())
	if errSend != nil {
		log.Error().Err(errSend).Str("server_id", serverID).Msg("Failed to send federation console input")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to send input"))
	}

	return connect.NewResponse(&xylona.FederationSendConsoleInputResponse{
		Success: true,
	}), nil
}

func (fs FederationService) ReadConsoleBuffer(ctx context.Context, request *connect.Request[xylona.FederationReadConsoleBufferRequest]) (*connect.Response[xylona.FederationReadConsoleBufferResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.console")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get server"))
	}

	output := fs.actionsInst.ReadGameServerBuffer(gs)
	return connect.NewResponse(&xylona.FederationReadConsoleBufferResponse{
		Output: output,
	}), nil
}

func (fs FederationService) StreamServerUpdates(ctx context.Context, request *connect.Request[xylona.FederationStreamServerUpdatesRequest], stream *connect.ServerStream[xylona.FederationServerUpdateEvent]) error {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	snapshot, errSnapshot := fs.buildServerSnapshot()
	if errSnapshot != nil {
		return errSnapshot
	}

	errSend := stream.Send(&xylona.FederationServerUpdateEvent{
		Event: &xylona.FederationServerUpdateEvent_Snapshot{
			Snapshot: snapshot,
		},
	})
	if errSend != nil {
		return errSend
	}

	// Register a status listener on each game server's supervisor command so we
	// can push status change events to the remote peer in real time.
	statusChan := make(chan *xylona.GameServerStatusUpdate, 64)
	listenerID := fmt.Sprintf("federation-stream-%s", uuid.New().String())

	gameServers, errGetServers := fs.db.GetAllGameServers()
	if errGetServers != nil && !errors.Is(errGetServers, sql.ErrNoRows) {
		return connect.NewError(connect.CodeInternal, errors.New("failed to get game servers for status listeners"))
	}

	for _, gs := range gameServers {
		cmd := fs.supervisorInst.GetCommandByIDOrCreateShell(gs.ID)
		cmd.AddStatusListener(listenerID, statusChan)
	}
	defer func() {
		for _, gs := range gameServers {
			cmd := fs.supervisorInst.GetCommandByIDOrCreateShell(gs.ID)
			cmd.RemoveStatusListener(listenerID)
		}
	}()

	// Initialize previous metrics from the snapshot so the first tick only
	// sends updates for servers whose metrics actually changed.
	previousMetrics := make(map[string]*xylona.GameServerMetrics, len(snapshot.Servers))
	for _, srv := range snapshot.Servers {
		if srv.Metrics != nil {
			previousMetrics[srv.ServerId] = srv.Metrics
		}
	}

	serverCreatedCh := eventbus.Get().Subscribe(eventbus.TopicGameServerCreated)
	defer eventbus.Get().Unsubscribe(eventbus.TopicGameServerCreated, serverCreatedCh)

	serverRemovedCh := eventbus.Get().Subscribe(eventbus.TopicGameServerRemoved)
	defer eventbus.Get().Unsubscribe(eventbus.TopicGameServerRemoved, serverRemovedCh)

	serverVersionChangedCh := eventbus.Get().SubscribeReliable(eventbus.TopicGameServerVersionChanged)
	defer eventbus.Get().Unsubscribe(eventbus.TopicGameServerVersionChanged, serverVersionChangedCh)

	metricsTicker := time.NewTicker(streamMetricsInterval)
	defer metricsTicker.Stop()

	heartbeatTicker := time.NewTicker(streamHeartbeatInterval)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-fs.ctx.Done():
			return nil
		case <-serverCreatedCh:
			newServers, errGetNew := fs.db.GetAllGameServers()
			if errGetNew != nil && !errors.Is(errGetNew, sql.ErrNoRows) {
				log.Error().Err(errGetNew).Msg("failed to get game servers after create event")
				continue
			}
			for _, gs := range newServers {
				cmd := fs.supervisorInst.GetCommandByIDOrCreateShell(gs.ID)
				cmd.AddStatusListener(listenerID, statusChan)
			}
			gameServers = newServers

			newSnapshot, errNewSnapshot := fs.buildServerSnapshot()
			if errNewSnapshot != nil {
				log.Error().Err(errNewSnapshot).Msg("failed to build snapshot after server create")
				continue
			}
			previousMetrics = make(map[string]*xylona.GameServerMetrics, len(newSnapshot.Servers))
			for _, srv := range newSnapshot.Servers {
				if srv.Metrics != nil {
					previousMetrics[srv.ServerId] = srv.Metrics
				}
			}
			errSendCreate := stream.Send(&xylona.FederationServerUpdateEvent{
				Event: &xylona.FederationServerUpdateEvent_Snapshot{
					Snapshot: newSnapshot,
				},
			})
			if errSendCreate != nil {
				return nil
			}
		case <-serverRemovedCh:
			newServers, errGetNew := fs.db.GetAllGameServers()
			if errGetNew != nil && !errors.Is(errGetNew, sql.ErrNoRows) {
				log.Error().Err(errGetNew).Msg("failed to get game servers after remove event")
				continue
			}
			gameServers = newServers

			newSnapshot, errNewSnapshot := fs.buildServerSnapshot()
			if errNewSnapshot != nil {
				log.Error().Err(errNewSnapshot).Msg("failed to build snapshot after server remove")
				continue
			}
			previousMetrics = make(map[string]*xylona.GameServerMetrics, len(newSnapshot.Servers))
			for _, srv := range newSnapshot.Servers {
				if srv.Metrics != nil {
					previousMetrics[srv.ServerId] = srv.Metrics
				}
			}
			errSendRemove := stream.Send(&xylona.FederationServerUpdateEvent{
				Event: &xylona.FederationServerUpdateEvent_Snapshot{
					Snapshot: newSnapshot,
				},
			})
			if errSendRemove != nil {
				return nil
			}
		case rawEvent := <-serverVersionChangedCh:
			versionEvent, ok := rawEvent.(eventbus.VersionChangedEvent)
			if !ok {
				continue
			}
			errVersion := stream.Send(&xylona.FederationServerUpdateEvent{
				Event: &xylona.FederationServerUpdateEvent_VersionChange{
					VersionChange: &xylona.FederationServerVersionChange{
						ServerId:    versionEvent.ServerID,
						Version:     versionEvent.Version,
						VersionInfo: versionEvent.VersionInfo,
					},
				},
			})
			if errVersion != nil {
				return nil
			}
		case update := <-statusChan:
			errStatus := stream.Send(&xylona.FederationServerUpdateEvent{
				Event: &xylona.FederationServerUpdateEvent_StatusChange{
					StatusChange: &xylona.FederationServerStatusChange{
						ServerId: update.GameServerId,
						Status:   update.Status,
					},
				},
			})
			if errStatus != nil {
				return nil
			}
		case <-metricsTicker.C:
			for _, gs := range gameServers {
				current := buildServerStateMetrics(fs.supervisorInst, gs.ID)
				if current == nil {
					continue
				}
				prev := previousMetrics[gs.ID]
				if !helpers.MetricsChanged(prev, current) {
					continue
				}
				previousMetrics[gs.ID] = current
				errMetrics := stream.Send(&xylona.FederationServerUpdateEvent{
					Event: &xylona.FederationServerUpdateEvent_MetricsUpdate{
						MetricsUpdate: &xylona.FederationServerMetricsUpdate{
							ServerId: gs.ID,
							Metrics:  current,
						},
					},
				})
				if errMetrics != nil {
					return nil
				}
			}
		case <-heartbeatTicker.C:
			errHeartbeat := stream.Send(&xylona.FederationServerUpdateEvent{
				Event: &xylona.FederationServerUpdateEvent_Heartbeat{
					Heartbeat: &xylona.FederationStreamHeartbeat{},
				},
			})
			if errHeartbeat != nil {
				return nil
			}
		}
	}
}

// buildServerSnapshot constructs a FederationServerSnapshot containing the current
// state of all local game servers, including live status and metrics from the supervisor.
func (fs FederationService) buildServerSnapshot() (*xylona.FederationServerSnapshot, error) {
	gameServers, errGetServers := fs.db.GetAllGameServers()
	if errGetServers != nil {
		if errors.Is(errGetServers, sql.ErrNoRows) {
			return &xylona.FederationServerSnapshot{}, nil
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get game servers"))
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
			Port:           int32(gs.Port),
			QueryPort:      int32(gs.QueryPort),
			MaxPlayers:     int32(gs.MaxPlayers),
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
		MemoryBytes:           int64(memVMS),
		MemoryWorkingSetBytes: int64(memRSS),
		MemoryPercent:         float64(memPct),
		CpuCores:              cpuCores,
		NumberOfThreads:       threads,
		DiskUsageBytes:        int64(diskBytes),
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

func (fs FederationService) getVersionState(serverID string) versiontracker.VersionState {
	if fs.versionState == nil {
		return versiontracker.VersionState{}
	}
	return fs.versionState.Get(serverID)
}

func (fs FederationService) UpdateRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationRemoteActionResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
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
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	errUpdate := fs.actionsInst.UpdateGameServer(gs)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("server_id", serverID).Msg("Failed to update game server")
		return nil, connect.NewError(federationUpdateErrorCode(errUpdate), errUpdate)
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

func (fs FederationService) GetRemoteVersionInfo(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationVersionInfoResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
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
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
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

func (fs FederationService) CheckRemoteServerForUpdate(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationVersionInfoResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
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
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	fs.actionsInst.CheckServerVersionByID(ctx, serverID)
	state := fs.getVersionState(serverID)

	return connect.NewResponse(&xylona.FederationVersionInfoResponse{
		VersionInfo: versionStateToProto(state),
	}), nil
}

func (fs FederationService) EditRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationEditServerRequest]) (*connect.Response[xylona.FederationEditServerResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	existingGS, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	incomingGameServer := helpers.GameServerProtoToModel(request.Msg.GetGameServer())
	gameServerModel := mergeEditableGameServerUpdate(
		existingGS,
		incomingGameServer,
		helpers.FederatedActingIsSuperUser(request.Header()),
	)
	setter := helpers.GameServerModelToSetter(gameServerModel)
	_, errUpdate := fs.db.UpdateGameServer(fs.db.DB, setter)
	if errUpdate != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update game server"))
	}

	return connect.NewResponse(&xylona.FederationEditServerResponse{
		Success:    true,
		GameServer: helpers.GameServerModelToProto(gameServerModel, nil),
	}), nil
}

func (fs FederationService) RemoveRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationRemoteActionResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(
		ctx,
		request.Header(),
		request.Msg.GetActingUserId(),
		request.Msg.GetOriginNodeId(),
		serverID,
		"game_server.delete",
	)
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	fs.actionsInst.StopGameServer(gs)
	errRemove := fs.actionsInst.RemoveGameServer(gs, true)
	if errRemove != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to remove game server"))
	}

	return connect.NewResponse(&xylona.FederationRemoteActionResponse{
		Success: true,
	}), nil
}

func (fs FederationService) ListRemoteDirectoryFiles(ctx context.Context, request *connect.Request[xylona.FederationListDirectoryFilesRequest]) (*connect.Response[xylona.FederationListDirectoryFilesResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.files.view")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	files, errList := fs.actionsInst.ListGameServerFiles(gs, request.Msg.GetPath())
	if errList != nil {
		if errors.Is(errList, actions.ErrInvalidPath) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid path"))
		}
		if errors.Is(errList, os.ErrNotExist) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("path not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list files"))
	}

	return connect.NewResponse(&xylona.FederationListDirectoryFilesResponse{
		Files: files,
	}), nil
}

func (fs FederationService) EditRemoteFile(ctx context.Context, request *connect.Request[xylona.FederationEditFileRequest]) (*connect.Response[xylona.FederationEditFileResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	errEdit := fs.actionsInst.EditFile(gs, request.Msg.GetFullFilePath(), request.Msg.GetContent())
	if errEdit != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to edit file"))
	}

	return connect.NewResponse(&xylona.FederationEditFileResponse{
		Success: true,
	}), nil
}

func (fs FederationService) DeleteRemoteFiles(ctx context.Context, request *connect.Request[xylona.FederationDeleteFilesRequest]) (*connect.Response[xylona.FederationDeleteFilesResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	results, errDelete := fs.actionsInst.DeleteFiles(ctx, gs, request.Msg.GetFullFilePaths())
	if errDelete != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete files"))
	}

	return connect.NewResponse(&xylona.FederationDeleteFilesResponse{
		Success:       true,
		FullFilePaths: results,
	}), nil
}

func (fs FederationService) RenameRemoteFile(ctx context.Context, request *connect.Request[xylona.FederationRenameFileRequest]) (*connect.Response[xylona.FederationRenameFileResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	newPath, errRename := fs.actionsInst.RenameFile(gs, request.Msg.GetOldPath(), request.Msg.GetNewPath())
	if errRename != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to rename file"))
	}

	return connect.NewResponse(&xylona.FederationRenameFileResponse{
		Success: true,
		NewPath: newPath,
	}), nil
}

func (fs FederationService) MoveRemoteFiles(ctx context.Context, request *connect.Request[xylona.FederationMoveFilesRequest]) (*connect.Response[xylona.FederationMoveFilesResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	results, errMove := fs.actionsInst.MoveFiles(ctx, gs, request.Msg.GetFullFilePaths(), request.Msg.GetDestinationBasePath())
	if errMove != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to move files"))
	}

	return connect.NewResponse(&xylona.FederationMoveFilesResponse{
		Success:       true,
		FullFilePaths: results,
	}), nil
}

func (fs FederationService) CreateRemoteFileOrDirectory(ctx context.Context, request *connect.Request[xylona.FederationCreateFileOrDirectoryRequest]) (*connect.Response[xylona.FederationCreateFileOrDirectoryResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	errCreate := fs.actionsInst.CreateFileOrDirectory(gs, request.Msg.GetFullFilePath(), request.Msg.GetContent(), request.Msg.GetIsDirectory())
	if errCreate != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create file or directory"))
	}

	return connect.NewResponse(&xylona.FederationCreateFileOrDirectoryResponse{
		Success: true,
	}), nil
}

func (fs FederationService) DownloadRemoteFileFromURL(ctx context.Context, request *connect.Request[xylona.FederationDownloadFileFromURLRequest]) (*connect.Response[xylona.FederationDownloadFileFromURLResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	filePath, errDownload := fs.actionsInst.DownloadFileFromURL(ctx, gs, request.Msg.GetUrl(), request.Msg.GetDestinationBasePath())
	if errDownload != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to download file"))
	}

	return connect.NewResponse(&xylona.FederationDownloadFileFromURLResponse{
		Success:  true,
		FilePath: filePath,
	}), nil
}

func (fs FederationService) QueryRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationQueryServerRequest]) (*connect.Response[xylona.FederationQueryServerResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.view")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get server"))
	}

	allServerQueries := fs.actionsInst.GetServerQueries()
	queryInfo, exists := allServerQueries.Servers[gs.ID]
	if !exists {
		var queryType xylona.ServerQuery_Type
		if gs.GameID == "minecraft" {
			queryType = xylona.ServerQuery_Minecraft
		} else {
			queryType = xylona.ServerQuery_Source
		}
		queryInfo = &xylona.ServerQuery{
			ServerId:   gs.ID,
			ServerName: gs.Name,
			Type:       queryType,
			Minecraft:  &xylona.MinecraftQueryInfo{NumberOfPlayers: 0, MaxPlayers: uint32(gs.MaxPlayers)},
			Source:     &xylona.SourceQueryInfo{Players: 0, MaxPlayers: uint32(gs.MaxPlayers)},
		}
	}

	return connect.NewResponse(&xylona.FederationQueryServerResponse{
		QueryInfo: queryInfo,
	}), nil
}

func (fs FederationService) ListRemoteGameServerAccessGrants(ctx context.Context, request *connect.Request[xylona.FederationListGameServerAccessGrantsRequest]) (*connect.Response[xylona.FederationListGameServerAccessGrantsResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := strings.TrimSpace(request.Msg.GetGameServerId())
	if serverID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("game_server_id is required"))
	}

	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	assignments, errGetAssignments := fs.db.GetUserRoleAssignmentsForServer(serverID)
	if errGetAssignments != nil {
		log.Error().Err(errGetAssignments).Str("server_id", serverID).Msg("failed to list remote game server access grants")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list access grants"))
	}

	resp := &xylona.FederationListGameServerAccessGrantsResponse{}
	for _, assignment := range assignments {
		grant, errBuild := fs.buildFederationGameServerAccessGrant(assignment)
		if errBuild != nil {
			return nil, errBuild
		}
		resp.Grants = append(resp.Grants, grant)
	}

	return connect.NewResponse(resp), nil
}

func (fs FederationService) GrantRemoteGameServerAccess(ctx context.Context, request *connect.Request[xylona.FederationGrantGameServerAccessRequest]) (*connect.Response[xylona.FederationGrantGameServerAccessResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := strings.TrimSpace(request.Msg.GetGameServerId())
	targetUserID := strings.TrimSpace(request.Msg.GetUserId())
	roleID := strings.TrimSpace(request.Msg.GetRoleId())
	if serverID == "" || targetUserID == "" || roleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("game_server_id, user_id, and role_id are required"))
	}

	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	_, errGetServer := fs.db.GetGameServerByID(serverID)
	if errGetServer != nil {
		if errors.Is(errGetServer, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
		}
		log.Error().Err(errGetServer).Str("server_id", serverID).Msg("failed to load game server for remote grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	_, errGetUser := fs.db.GetUserByID(targetUserID)
	if errGetUser != nil {
		if errors.Is(errGetUser, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("target user not found"))
		}
		log.Error().Err(errGetUser).Str("user_id", targetUserID).Msg("failed to verify target user for remote grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	_, errGetRole := fs.db.GetRoleByID(roleID)
	if errGetRole != nil {
		if errors.Is(errGetRole, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("role not found"))
		}
		log.Error().Err(errGetRole).Str("role_id", roleID).Msg("failed to verify role for remote grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	grantorUserID, errGrantor := fs.resolveGrantorUserIDForServer(request.Header(), serverID)
	if errGrantor != nil {
		if errors.Is(errGrantor, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
		}
		log.Error().Err(errGrantor).Str("server_id", serverID).Msg("failed to resolve grantor for remote grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	newID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		log.Error().Err(errID).Msg("failed to generate remote game server access grant id")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	errCreateGrant := fs.db.CreateUserRoleAssignment(newID.String(), targetUserID, roleID, serverID, grantorUserID)
	if errCreateGrant != nil {
		if isSQLiteUniqueConstraintError(errCreateGrant) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("grant already exists"))
		}
		log.Error().
			Err(errCreateGrant).
			Str("server_id", serverID).
			Str("user_id", targetUserID).
			Str("role_id", roleID).
			Msg("failed to create remote game server access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	assignment, errGetAssignment := fs.db.GetUserRoleAssignmentByID(newID.String())
	if errGetAssignment != nil {
		log.Error().Err(errGetAssignment).Str("grant_id", newID.String()).Msg("failed to fetch created remote game server grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
	}

	grant, errBuild := fs.buildFederationGameServerAccessGrant(assignment)
	if errBuild != nil {
		return nil, errBuild
	}

	return connect.NewResponse(&xylona.FederationGrantGameServerAccessResponse{
		Grant: grant,
	}), nil
}

func (fs FederationService) RevokeRemoteGameServerAccess(ctx context.Context, request *connect.Request[xylona.FederationRevokeGameServerAccessRequest]) (*connect.Response[xylona.FederationRevokeGameServerAccessResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	grantID := strings.TrimSpace(request.Msg.GetGrantId())
	if grantID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("grant_id is required"))
	}

	assignment, errGetAssignment := fs.db.GetUserRoleAssignmentByID(grantID)
	if errGetAssignment != nil {
		if errors.Is(errGetAssignment, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("grant not found"))
		}
		log.Error().Err(errGetAssignment).Str("grant_id", grantID).Msg("failed to fetch remote game server access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke access"))
	}
	if !assignment.GameServerID.IsValue() || assignment.GameServerID.IsNull() {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("grant is not scoped to a game server"))
	}

	serverID := assignment.GameServerID.MustGet()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	errDeleteGrant := fs.db.DeleteUserRoleAssignment(grantID)
	if errDeleteGrant != nil {
		log.Error().Err(errDeleteGrant).Str("grant_id", grantID).Msg("failed to revoke remote game server access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke access"))
	}

	return connect.NewResponse(&xylona.FederationRevokeGameServerAccessResponse{}), nil
}

func (fs FederationService) ListRemoteFederatedAccessGrants(ctx context.Context, request *connect.Request[xylona.FederationListFederatedAccessGrantsRequest]) (*connect.Response[xylona.FederationListFederatedAccessGrantsResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := strings.TrimSpace(request.Msg.GetGameServerId())
	if serverID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("game_server_id is required"))
	}

	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	grants, errGetGrants := fs.db.GetFederatedAccessGrantsForServer(serverID)
	if errGetGrants != nil {
		log.Error().Err(errGetGrants).Str("server_id", serverID).Msg("failed to list remote federated access grants")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list federated grants"))
	}

	resp := &xylona.FederationListFederatedAccessGrantsResponse{}
	for _, grantModel := range grants {
		grantInfo, errBuild := fs.buildFederationFederatedAccessGrantInfo(grantModel)
		if errBuild != nil {
			return nil, errBuild
		}
		resp.Grants = append(resp.Grants, grantInfo)
	}

	return connect.NewResponse(resp), nil
}

func (fs FederationService) GrantRemoteFederatedAccess(ctx context.Context, request *connect.Request[xylona.FederationGrantFederatedAccessRequest]) (*connect.Response[xylona.FederationGrantFederatedAccessResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := strings.TrimSpace(request.Msg.GetGameServerId())
	remoteNodeID := strings.TrimSpace(request.Msg.GetRemoteNodeId())
	remoteUserID := strings.TrimSpace(request.Msg.GetRemoteUserId())
	roleID := strings.TrimSpace(request.Msg.GetRoleId())
	if serverID == "" || remoteNodeID == "" || remoteUserID == "" || roleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("game_server_id, remote_node_id, remote_user_id, and role_id are required"))
	}

	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	_, errGetServer := fs.db.GetGameServerByID(serverID)
	if errGetServer != nil {
		if errors.Is(errGetServer, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
		}
		log.Error().Err(errGetServer).Str("server_id", serverID).Msg("failed to load game server for remote federated grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
	}

	_, errGetNode := fs.db.GetRemoteNodeByID(remoteNodeID)
	if errGetNode != nil {
		if errors.Is(errGetNode, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("remote node not found"))
		}
		log.Error().Err(errGetNode).Str("remote_node_id", remoteNodeID).Msg("failed to verify remote node for federated grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
	}

	_, errGetRole := fs.db.GetRoleByID(roleID)
	if errGetRole != nil {
		if errors.Is(errGetRole, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("role not found"))
		}
		log.Error().Err(errGetRole).Str("role_id", roleID).Msg("failed to verify role for federated grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
	}

	remoteUserName := strings.TrimSpace(request.Msg.GetRemoteUserName())
	if remoteUserName == "" {
		remoteUserName = remoteUserID
	}

	grantorUserID, errGrantor := fs.resolveGrantorUserIDForServer(request.Header(), serverID)
	if errGrantor != nil {
		if errors.Is(errGrantor, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
		}
		log.Error().Err(errGrantor).Str("server_id", serverID).Msg("failed to resolve grantor for remote federated grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
	}

	newID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		log.Error().Err(errID).Msg("failed to generate remote federated access grant id")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
	}

	errCreateGrant := fs.db.CreateFederatedAccessGrant(
		newID.String(),
		serverID,
		remoteNodeID,
		remoteUserID,
		remoteUserName,
		roleID,
		grantorUserID,
	)
	if errCreateGrant != nil {
		if isSQLiteUniqueConstraintError(errCreateGrant) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("grant already exists"))
		}
		log.Error().
			Err(errCreateGrant).
			Str("server_id", serverID).
			Str("remote_node_id", remoteNodeID).
			Str("remote_user_id", remoteUserID).
			Str("role_id", roleID).
			Msg("failed to create remote federated access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
	}

	grantModel, errGetGrant := fs.db.GetFederatedAccessGrantByID(newID.String())
	if errGetGrant != nil {
		log.Error().Err(errGetGrant).Str("grant_id", newID.String()).Msg("failed to fetch created remote federated grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant federated access"))
	}

	grantInfo, errBuild := fs.buildFederationFederatedAccessGrantInfo(grantModel)
	if errBuild != nil {
		return nil, errBuild
	}

	return connect.NewResponse(&xylona.FederationGrantFederatedAccessResponse{
		Grant: grantInfo,
	}), nil
}

func (fs FederationService) RevokeRemoteFederatedAccess(ctx context.Context, request *connect.Request[xylona.FederationRevokeFederatedAccessRequest]) (*connect.Response[xylona.FederationRevokeFederatedAccessResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	grantID := strings.TrimSpace(request.Msg.GetGrantId())
	if grantID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("grant_id is required"))
	}

	grantModel, errGetGrant := fs.db.GetFederatedAccessGrantByID(grantID)
	if errGetGrant != nil {
		if errors.Is(errGetGrant, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("grant not found"))
		}
		log.Error().Err(errGetGrant).Str("grant_id", grantID).Msg("failed to fetch remote federated access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke federated access"))
	}

	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", grantModel.GameServerID, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	errDeleteGrant := fs.db.DeleteFederatedAccessGrant(grantID)
	if errDeleteGrant != nil {
		log.Error().Err(errDeleteGrant).Str("grant_id", grantID).Msg("failed to revoke remote federated access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to revoke federated access"))
	}

	return connect.NewResponse(&xylona.FederationRevokeFederatedAccessResponse{}), nil
}

func (fs FederationService) buildFederationGameServerAccessGrant(assignment *models.UserRoleAssignment) (*xylona.FederationGameServerAccessGrant, error) {
	role, errGetRole := fs.db.GetRoleByID(assignment.RoleID)
	if errGetRole != nil {
		log.Error().Err(errGetRole).Str("role_id", assignment.RoleID).Msg("failed to fetch role for federation game server access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list access grants"))
	}

	targetUser, errGetUser := fs.db.GetUserByID(assignment.UserID)
	if errGetUser != nil {
		log.Error().Err(errGetUser).Str("user_id", assignment.UserID).Msg("failed to fetch user for federation game server access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list access grants"))
	}

	grantedByUser, errGetGrantedBy := fs.db.GetUserByID(assignment.GrantedBy)
	if errGetGrantedBy != nil {
		log.Error().Err(errGetGrantedBy).Str("granted_by", assignment.GrantedBy).Msg("failed to fetch grantor for federation game server access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list access grants"))
	}

	gameServerID := ""
	if assignment.GameServerID.IsValue() && !assignment.GameServerID.IsNull() {
		gameServerID = assignment.GameServerID.MustGet()
	}

	return &xylona.FederationGameServerAccessGrant{
		Id:                assignment.ID,
		UserId:            assignment.UserID,
		UserName:          targetUser.UserName,
		RoleId:            assignment.RoleID,
		RoleName:          role.Name,
		GameServerId:      gameServerID,
		GrantedByUserId:   assignment.GrantedBy,
		GrantedByUserName: grantedByUser.UserName,
		CreatedAt:         timestamppb.New(assignment.CreatedAt),
	}, nil
}

func (fs FederationService) buildFederationFederatedAccessGrantInfo(grantModel *models.FederatedAccessGrant) (*xylona.FederationFederatedAccessGrantInfo, error) {
	role, errGetRole := fs.db.GetRoleByID(grantModel.RoleID)
	if errGetRole != nil {
		log.Error().Err(errGetRole).Str("role_id", grantModel.RoleID).Msg("failed to fetch role for federation federated access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list federated grants"))
	}

	grantedByUser, errGetGrantedBy := fs.db.GetUserByID(grantModel.GrantedBy)
	if errGetGrantedBy != nil {
		log.Error().Err(errGetGrantedBy).Str("granted_by", grantModel.GrantedBy).Msg("failed to fetch grantor for federation federated access grant")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list federated grants"))
	}

	nodeName := ""
	node, errGetNode := fs.db.GetRemoteNodeByID(grantModel.RemoteNodeID)
	if errGetNode == nil && node != nil {
		nodeName = node.Name
	}

	return &xylona.FederationFederatedAccessGrantInfo{
		Id:                grantModel.ID,
		GameServerId:      grantModel.GameServerID,
		RemoteNodeId:      grantModel.RemoteNodeID,
		RemoteNodeName:    nodeName,
		RemoteUserId:      grantModel.RemoteUserID,
		RemoteUserName:    grantModel.RemoteUserName,
		RoleId:            grantModel.RoleID,
		RoleName:          role.Name,
		GrantedByUserId:   grantModel.GrantedBy,
		GrantedByUserName: grantedByUser.UserName,
		CreatedAt:         timestamppb.New(grantModel.CreatedAt),
	}, nil
}

func (fs FederationService) resolveGrantorUserIDForServer(header http.Header, serverID string) (string, error) {
	actingUserID, _ := helpers.GetFederatedActingIdentity(header)
	actingUserID = strings.TrimSpace(actingUserID)
	if actingUserID != "" {
		_, errGetUser := fs.db.GetUserByID(actingUserID)
		if errGetUser == nil {
			return actingUserID, nil
		}
		if !errors.Is(errGetUser, sql.ErrNoRows) {
			return "", errGetUser
		}
	}

	gameServer, errGetServer := fs.db.GetGameServerByID(serverID)
	if errGetServer != nil {
		return "", errGetServer
	}

	return gameServer.UserID, nil
}
