package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/xycrypt"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/supervisor"
)

const (
	FederationProtocolVersion  = 1
	FederationCapabilities     = "server_list,server_detail,remote_actions,console_streaming,status_streaming"
	SoftwareVersion            = "0.1.0"
	federationRequestTimeout   = 15 * time.Second
)

// FederationService handles node-to-node federation API calls.
type FederationService struct {
	ctx            context.Context
	db             *db.Connection
	actionsInst    *actions.Instance
	supervisorInst *supervisor.Instance
}

func NewFederationService(ctx context.Context, dbInst *db.Connection, actionsInst *actions.Instance, supervisorInst *supervisor.Instance) *FederationService {
	return &FederationService{
		ctx:            ctx,
		db:             dbInst,
		actionsInst:    actionsInst,
		supervisorInst: supervisorInst,
	}
}

func (fs FederationService) authenticateRequest(secretKey string) error {
	if secretKey == "" {
		return errors.New("secret key is required")
	}

	allKeys, errGetKeys := fs.db.GetAllSecretKeys()
	if errGetKeys != nil {
		return errors.New("failed to retrieve secret keys")
	}

	for _, key := range allKeys {
		matches, errCompare := xycrypt.CompareHashAndString([]byte(key.SecretKeyHash), secretKey)
		if errCompare != nil {
			continue
		}
		if matches {
			return nil
		}
	}
	return errors.New("invalid secret key")
}

func (fs FederationService) Handshake(ctx context.Context, request *connect.Request[xylona.FederationHandshakeRequest]) (*connect.Response[xylona.FederationHandshakeResponse], error) {
	errAuth := fs.authenticateRequest(request.Msg.GetSecretKey())
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

	resp := &xylona.FederationHandshakeResponse{
		NodeId:          localSettings.NodeID,
		NodeName:        nodeName,
		Version:         SoftwareVersion,
		ProtocolVersion: FederationProtocolVersion,
		Capabilities:    FederationCapabilities,
		ServerTime:      timestamppb.New(time.Now()),
	}

	log.Info().Str("peer", request.Peer().Addr).Msg("Federation handshake successful")
	return connect.NewResponse(resp), nil
}

func (fs FederationService) ListServerSummaries(ctx context.Context, request *connect.Request[xylona.FederationListServerSummariesRequest]) (*connect.Response[xylona.FederationListServerSummariesResponse], error) {
	errAuth := fs.authenticateRequest(request.Header().Get("X-Federation-Key"))
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
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
		status := helpers.GameServerModelStatusToProtoStatus(gs.Status)
		gameServerCmd, errGetCommand := fs.supervisorInst.GetCommandByID(gs.ID)
		if errGetCommand == nil {
			status = helpers.GameServerModelStatusToProtoStatus(string(gameServerCmd.Status()))
		}

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
			Version:        gs.Version,
			UpdatedAt:      timestamppb.New(gs.UpdatedAt),
		}
		resp.Servers = append(resp.Servers, summary)
	}

	return connect.NewResponse(resp), nil
}

func (fs FederationService) GetServerDetail(ctx context.Context, request *connect.Request[xylona.FederationGetServerDetailRequest]) (*connect.Response[xylona.FederationGetServerDetailResponse], error) {
	errAuth := fs.authenticateRequest(request.Header().Get("X-Federation-Key"))
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	gs, errGet := fs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get server"))
	}

	status := helpers.GameServerModelStatusToProtoStatus(gs.Status)
	gameServerCmd, errGetCommand := fs.supervisorInst.GetCommandByID(gs.ID)
	if errGetCommand == nil {
		status = helpers.GameServerModelStatusToProtoStatus(string(gameServerCmd.Status()))
	}

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
		Version:     gs.Version,
		UpdatedAt:   timestamppb.New(gs.UpdatedAt),
	}

	return connect.NewResponse(&xylona.FederationGetServerDetailResponse{
		Server: summary,
	}), nil
}

func (fs FederationService) StartRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationRemoteActionResponse], error) {
	errAuth := fs.authenticateRequest(request.Header().Get("X-Federation-Key"))
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	gs, errGet := fs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGet != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	fs.actionsInst.StartGameServer(gs)

	return connect.NewResponse(&xylona.FederationRemoteActionResponse{
		Success: true,
	}), nil
}

func (fs FederationService) StopRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationRemoteActionResponse], error) {
	errAuth := fs.authenticateRequest(request.Header().Get("X-Federation-Key"))
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	gs, errGet := fs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGet != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	fs.actionsInst.StopGameServer(gs)

	return connect.NewResponse(&xylona.FederationRemoteActionResponse{
		Success: true,
	}), nil
}

func (fs FederationService) RestartRemoteServer(ctx context.Context, request *connect.Request[xylona.FederationRemoteActionRequest]) (*connect.Response[xylona.FederationRemoteActionResponse], error) {
	errAuth := fs.authenticateRequest(request.Header().Get("X-Federation-Key"))
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	gs, errGet := fs.db.GetGameServerByID(request.Msg.GetServerId())
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
	errAuth := fs.authenticateRequest(request.Msg.GetSecretKey())
	if errAuth != nil {
		return connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return connect.NewError(connect.CodeNotFound, errors.New("server not found"))
		}
		return connect.NewError(connect.CodeInternal, errors.New("failed to get server"))
	}

	command := fs.supervisorInst.GetCommandByIDOrCreateShell(gs.ID)
	listenerID := fmt.Sprintf("federation-%s", uuid.New().String())
	outputChan := make(chan xylona.Message, 64)
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
	errAuth := fs.authenticateRequest(request.Header().Get("X-Federation-Key"))
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
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
	errAuth := fs.authenticateRequest(request.Header().Get("X-Federation-Key"))
	if errAuth != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	serverID := request.Msg.GetServerId()
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

func (fs FederationService) StreamServerStatuses(ctx context.Context, request *connect.Request[xylona.FederationStreamServerStatusesRequest], stream *connect.ServerStream[xylona.FederationServerStatusEvent]) error {
	errAuth := fs.authenticateRequest(request.Msg.GetSecretKey())
	if errAuth != nil {
		return connect.NewError(connect.CodePermissionDenied, errors.New("authentication failed"))
	}

	log.Debug().Msg("Federation server status stream started")

	// Poll all local game servers for status changes and stream them.
	previousStatuses := make(map[string]xylona.Status)
	var statusLock sync.Mutex

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-fs.ctx.Done():
			return nil
		case <-ticker.C:
			allGameServers, errGetServers := fs.db.GetAllGameServers()
			if errGetServers != nil {
				continue
			}

			statusLock.Lock()
			for _, gs := range allGameServers {
				currentStatus := xylona.Status_OFFLINE
				gameServerCmd, errGetCommand := fs.supervisorInst.GetCommandByID(gs.ID)
				if errGetCommand == nil {
					currentStatus = gameServerCmd.Status()
				}

				prevStatus, exists := previousStatuses[gs.ID]
				if !exists || prevStatus != currentStatus {
					previousStatuses[gs.ID] = currentStatus
					errSend := stream.Send(&xylona.FederationServerStatusEvent{
						ServerId: gs.ID,
						Status:   currentStatus,
					})
					if errSend != nil {
						statusLock.Unlock()
						log.Debug().Err(errSend).Msg("Federation server status stream send failed")
						return nil
					}
				}
			}
			statusLock.Unlock()
		}
	}
}
