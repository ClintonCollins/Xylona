package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/helpers/federation"
	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/pkg/sysinfo"
	"github.com/ClintonCollins/Xylona/pkg/version"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
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

// NewFederationService constructs the federation RPC service implementation.
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

// Handshake returns local node metadata to an authenticated federation peer.
func (fs FederationService) Handshake(ctx context.Context, request *connect.Request[xylona.FederationHandshakeRequest]) (*connect.Response[xylona.FederationHandshakeResponse], error) {
	peerIdentity, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		log.Warn().Str("peer", request.Peer().Addr).Msg("Federation handshake authentication failed")
		return nil, permissionDenied("authentication failed")
	}

	localSettings, errSettings := fs.db.GetLocalSettings()
	if errSettings != nil {
		return nil, internalErrf("failed to get local settings")
	}

	localNode, errNode := fs.db.GetNodeByID(localSettings.NodeID)
	if errNode != nil && !errors.Is(errNode, sql.ErrNoRows) {
		return nil, internalErrf("failed to get local node")
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
	systemInfo, errSystemInfo := sysinfo.CollectSystemInfo()
	if errSystemInfo != nil {
		log.Warn().Err(errSystemInfo).Msg("Failed to collect system info for federation handshake")
	} else {
		resp.SystemInfo = sysInfoToProto(systemInfo)
	}

	log.Info().
		Str("peer", request.Peer().Addr).
		Str("peer_node_id", peerIdentity.PeerNodeID).
		Msg("Federation handshake successful")
	return connect.NewResponse(resp), nil
}

// ListServerSummaries lists local server summaries for an authenticated federation peer.
func (fs FederationService) ListServerSummaries(ctx context.Context, request *connect.Request[xylona.FederationListServerSummariesRequest]) (*connect.Response[xylona.FederationListServerSummariesResponse], error) {
	peerIdentity, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	actingUserID, originNodeID := federation.GetActingIdentity(request.Header())
	actingUserID = strings.TrimSpace(actingUserID)
	originNodeID = strings.TrimSpace(originNodeID)
	actingIsSuperUser := federation.ActingIsSuperUser(request.Header())
	if (actingUserID != "" || originNodeID != "") && (actingUserID == "" || originNodeID == "") {
		return nil, permissionDenied("federated acting identity is invalid")
	}
	if actingIsSuperUser && (actingUserID == "" || originNodeID == "") {
		return nil, permissionDenied("super-user federated identity is invalid")
	}
	if originNodeID != "" && originNodeID != peerIdentity.NodeID && originNodeID != peerIdentity.PeerNodeID {
		return nil, permissionDenied("federated acting origin is invalid")
	}

	gameServers, errGetServers := fs.db.GetAllGameServers()
	if errGetServers != nil {
		if errors.Is(errGetServers, sql.ErrNoRows) {
			return connect.NewResponse(&xylona.FederationListServerSummariesResponse{}), nil
		}
		return nil, internalErrf("failed to get game servers")
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

// ListUserSummaries lists local user summaries for an authenticated federation peer.
func (fs FederationService) ListUserSummaries(ctx context.Context, request *connect.Request[xylona.FederationListUserSummariesRequest]) (*connect.Response[xylona.FederationListUserSummariesResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	users, errUsers := fs.db.GetAllUsers()
	if errUsers != nil {
		if errors.Is(errUsers, sql.ErrNoRows) {
			return connect.NewResponse(&xylona.FederationListUserSummariesResponse{}), nil
		}
		return nil, internalErrf("failed to list users")
	}

	limit := int(request.Msg.GetLimit())
	limit = max(limit, 0)

	resp := &xylona.FederationListUserSummariesResponse{}
	for _, user := range users {
		if user.SuperUser {
			continue
		}
		if limit > 0 && len(resp.GetUsers()) >= limit {
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

// GetServerDetail returns detailed information for a federated game server.
func (fs FederationService) GetServerDetail(ctx context.Context, request *connect.Request[xylona.FederationGetServerDetailRequest]) (*connect.Response[xylona.FederationGetServerDetailResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.view")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, dbLookupMsg(errGet, "failed to get server")
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

	actingUserID, originNodeID := federation.GetActingIdentity(request.Header())
	isSuperUser := federation.ActingIsSuperUser(request.Header())

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
	summary.MemoryBytes = helpers.ClampInt64FromUint64(memVMS)
	summary.MemoryWorkingSetBytes = helpers.ClampInt64FromUint64(memRSS)
	summary.MemoryPercent = float64(memPct)
	summary.CpuCores = cpuCores
	summary.NumberOfThreads = threads
	summary.DiskUsageBytes = helpers.ClampInt64FromUint64(diskBytes)
	summary.IoReadRate = ioRead
	summary.IoWriteRate = ioWrite
	summary.ConnectionCount = connCount
	startedAt := cmd.UnixStartedAt()
	if startedAt > 0 {
		summary.UptimeSeconds = time.Now().Unix() - startedAt
	}
}

// StreamConsoleOutput streams console output for a local game server to a federated peer.
func (fs FederationService) StreamConsoleOutput(ctx context.Context, request *connect.Request[xylona.FederationStreamConsoleRequest], stream *connect.ServerStream[xylona.FederationConsoleOutputChunk]) error {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.console")
	if errPermission != nil {
		return errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return dbLookupMsg(errGet, "failed to get server")
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
			if msg.GetGameServerConsoleOutput() == nil {
				continue
			}
			errSend := stream.Send(&xylona.FederationConsoleOutputChunk{
				ServerId: serverID,
				Output:   msg.GetGameServerConsoleOutput().GetOutput(),
			})
			if errSend != nil {
				log.Debug().Err(errSend).Str("server_id", serverID).Msg("Federation console stream send failed")
				return nil
			}
		}
	}
}

// SendConsoleInput forwards console input from a federated peer to a local server.
func (fs FederationService) SendConsoleInput(ctx context.Context, request *connect.Request[xylona.FederationSendConsoleInputRequest]) (*connect.Response[xylona.FederationSendConsoleInputResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.console")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, dbLookupMsg(errGet, "failed to get server")
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

// ReadConsoleBuffer returns buffered console output for a local game server.
func (fs FederationService) ReadConsoleBuffer(ctx context.Context, request *connect.Request[xylona.FederationReadConsoleBufferRequest]) (*connect.Response[xylona.FederationReadConsoleBufferResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.console")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, dbLookupMsg(errGet, "failed to get server")
	}

	output := fs.actionsInst.ReadGameServerBuffer(gs)
	return connect.NewResponse(&xylona.FederationReadConsoleBufferResponse{
		Output: output,
	}), nil
}

// StreamServerUpdates streams local server state changes to an authenticated federation peer.
func (fs FederationService) StreamServerUpdates(ctx context.Context, _ *connect.Request[xylona.FederationStreamServerUpdatesRequest], stream *connect.ServerStream[xylona.FederationServerUpdateEvent]) error {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return permissionDenied("authentication failed")
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
		return fmt.Errorf("rpc: send federation snapshot: %w", errSend)
	}

	// Register a status listener on each game server's supervisor command so we
	// can push status change events to the remote peer in real time.
	statusChan := make(chan *xylona.GameServerStatusUpdate, 64)
	listenerID := fmt.Sprintf("federation-stream-%s", uuid.New().String())

	gameServers, errGetServers := fs.db.GetAllGameServers()
	if errGetServers != nil && !errors.Is(errGetServers, sql.ErrNoRows) {
		return internalErrf("failed to get game servers for status listeners")
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
	previousMetrics := make(map[string]*xylona.GameServerMetrics, len(snapshot.GetServers()))
	for _, srv := range snapshot.GetServers() {
		if srv.GetMetrics() != nil {
			previousMetrics[srv.GetServerId()] = srv.GetMetrics()
		}
	}

	serverCreatedCh := eventbus.Get().Subscribe(eventbus.TopicGameServerCreated)
	defer eventbus.Get().Unsubscribe(eventbus.TopicGameServerCreated, serverCreatedCh)

	serverRemovedCh := eventbus.Get().Subscribe(eventbus.TopicGameServerRemoved)
	defer eventbus.Get().Unsubscribe(eventbus.TopicGameServerRemoved, serverRemovedCh)

	serverVersionChangedCh := eventbus.Get().SubscribeReliable(eventbus.TopicGameServerVersionChanged)
	defer eventbus.Get().Unsubscribe(eventbus.TopicGameServerVersionChanged, serverVersionChangedCh)

	// Subscribe to all alert event topics and fan them into a single channel
	// so we can forward alert events to the federated peer.
	type alertMsg struct {
		topic string
		msg   any
	}
	alertCh := make(chan alertMsg, 256)
	type alertSubscription struct {
		topic string
		ch    chan any
	}
	subscriptions := make([]alertSubscription, 0, len(allAlertTopics))
	for _, topic := range allAlertTopics {
		topicCh := eventbus.Get().SubscribeReliable(topic)
		subscriptions = append(subscriptions, alertSubscription{topic: topic, ch: topicCh})
		capturedTopic := topic
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case raw, ok := <-topicCh:
					if !ok {
						return
					}
					select {
					case alertCh <- alertMsg{topic: capturedTopic, msg: raw}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	defer func() {
		for _, subscription := range subscriptions {
			eventbus.Get().Unsubscribe(subscription.topic, subscription.ch)
		}
	}()

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
			previousMetrics = make(map[string]*xylona.GameServerMetrics, len(newSnapshot.GetServers()))
			for _, srv := range newSnapshot.GetServers() {
				if srv.GetMetrics() != nil {
					previousMetrics[srv.GetServerId()] = srv.GetMetrics()
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
			previousMetrics = make(map[string]*xylona.GameServerMetrics, len(newSnapshot.GetServers()))
			for _, srv := range newSnapshot.GetServers() {
				if srv.GetMetrics() != nil {
					previousMetrics[srv.GetServerId()] = srv.GetMetrics()
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
						ServerId:    update.GetGameServerId(),
						Status:      update.GetStatus(),
						DisplayName: update.GetGameServerName(),
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
		case alert := <-alertCh:
			protoEvt, ok := serializeAlertEvent(alert.topic, alert.msg)
			if !ok {
				continue
			}
			errAlert := stream.Send(&xylona.FederationServerUpdateEvent{
				Event: &xylona.FederationServerUpdateEvent_AlertEvent{
					AlertEvent: protoEvt,
				},
			})
			if errAlert != nil {
				return nil
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

// QueryRemoteServer performs a live query against a local server for a federated peer.
func (fs FederationService) QueryRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationQueryServerRequest]) (*connect.Response[xylona.FederationQueryServerResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.view")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, dbLookupMsg(errGet, "failed to get server")
	}

	allServerQueries := fs.actionsInst.GetServerQueries()
	queryInfo, exists := allServerQueries.GetServers()[gs.ID]
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
			Minecraft:  &xylona.MinecraftQueryInfo{NumberOfPlayers: 0, MaxPlayers: helpers.ClampUint32FromInt64(gs.MaxPlayers)},
			Source:     &xylona.SourceQueryInfo{Players: 0, MaxPlayers: helpers.ClampUint32FromInt64(gs.MaxPlayers)},
		}
	}

	return connect.NewResponse(&xylona.FederationQueryServerResponse{
		QueryInfo: queryInfo,
	}), nil
}

// EditRemoteServer edits a local game server on behalf of a federated peer.
func (fs FederationService) EditRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationEditServerRequest]) (*connect.Response[xylona.FederationEditServerResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	existingGS, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, notFoundErr()
	}

	incomingGameServer := helpers.GameServerProtoToModel(request.Msg.GetGameServer())
	gameServerModel := mergeEditableGameServerUpdate(
		existingGS,
		incomingGameServer,
		federation.ActingIsSuperUser(request.Header()),
	)
	setter := helpers.GameServerModelToSetter(gameServerModel)
	_, errUpdate := fs.db.UpdateGameServer(fs.db.DB, setter)
	if errUpdate != nil {
		return nil, internalErrf("failed to update game server")
	}

	return connect.NewResponse(&xylona.FederationEditServerResponse{
		Success:    true,
		GameServer: helpers.GameServerModelToProto(gameServerModel, nil),
	}), nil
}

// RemoveRemoteServer deletes a local game server on behalf of a federated peer.
func (fs FederationService) RemoveRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationRemoteActionResponse], error) {
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
		"game_server.delete",
	)
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, notFoundErr()
	}

	fs.actionsInst.StopGameServer(gs)
	errRemove := fs.actionsInst.RemoveGameServer(gs, true)
	if errRemove != nil {
		return nil, internalErrf("failed to remove game server")
	}

	return connect.NewResponse(&xylona.FederationRemoteActionResponse{
		Success: true,
	}), nil
}

func (fs FederationService) resolveGrantorUserIDForServer(header http.Header, serverID string) (string, error) {
	actingUserID, _ := federation.GetActingIdentity(header)
	actingUserID = strings.TrimSpace(actingUserID)
	if actingUserID != "" {
		_, errGetUser := fs.db.GetUserByID(actingUserID)
		if errGetUser == nil {
			return actingUserID, nil
		}
		if !errors.Is(errGetUser, sql.ErrNoRows) {
			return "", fmt.Errorf("rpc: load acting grantor user: %w", errGetUser)
		}
	}

	gameServer, errGetServer := fs.db.GetGameServerByID(serverID)
	if errGetServer != nil {
		return "", fmt.Errorf("rpc: load game server for grantor resolution: %w", errGetServer)
	}

	return gameServer.UserID, nil
}
