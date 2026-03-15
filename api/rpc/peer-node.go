package rpc

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (xs XylonaService) ListPeerNodes(ctx context.Context, request *connect.Request[xylona.ListPeerNodesRequest]) (*connect.Response[xylona.ListPeerNodesResponse], error) {
	peerNodes, err := xs.db.GetAllPeerNodes()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list peer nodes"))
	}
	resp := &xylona.ListPeerNodesResponse{}
	for _, pn := range peerNodes {
		resp.PeerNodes = append(resp.PeerNodes, helpers.PeerNodeModelToProto(pn))
	}
	return connect.NewResponse(resp), nil
}

func (xs XylonaService) GetPeerNode(ctx context.Context, request *connect.Request[xylona.GetPeerNodeRequest]) (*connect.Response[xylona.GetPeerNodeResponse], error) {
	peerNode, err := xs.db.GetPeerNodeByID(request.Msg.GetPeerNodeId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("peer node not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get peer node"))
	}
	return connect.NewResponse(&xylona.GetPeerNodeResponse{
		PeerNode: helpers.PeerNodeModelToProto(peerNode),
	}), nil
}

func (xs XylonaService) AddPeerNode(ctx context.Context, request *connect.Request[xylona.AddPeerNodeRequest]) (*connect.Response[xylona.AddPeerNodeResponse], error) {
	baseURL := strings.TrimRight(request.Msg.GetBaseUrl(), "/")
	name := request.Msg.GetName()
	secretKey := request.Msg.GetSecretKey()

	// Validate base URL.
	parsedURL, errParse := url.Parse(baseURL)
	if errParse != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid base URL"))
	}

	// Check for duplicate base URL.
	_, errExisting := xs.db.GetPeerNodeByBaseURL(baseURL)
	if errExisting == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("peer node with this URL already exists"))
	}

	// Perform handshake to verify connectivity and get peer identity.
	handshakeResp, errHandshake := performHandshake(ctx, baseURL, secretKey)
	if errHandshake != nil {
		log.Error().Err(errHandshake).Str("base_url", baseURL).Msg("Failed to handshake with peer node")
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("failed to connect to peer node: "+errHandshake.Error()))
	}

	// Prevent self-registration.
	localSettings, errSettings := xs.db.GetLocalSettings()
	if errSettings != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get local settings"))
	}
	if handshakeResp.NodeId == localSettings.NodeID {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cannot add self as a peer node"))
	}

	// Check for duplicate node ID.
	if handshakeResp.NodeId != "" {
		_, errDupe := xs.db.GetPeerNodeByNodeID(handshakeResp.NodeId)
		if errDupe == nil {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("peer node with this node ID already exists"))
		}
	}

	newID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to generate ID"))
	}

	if name == "" {
		name = handshakeResp.NodeName
	}

	now := time.Now()
	setter := &models.PeerNodeSetter{
		ID:              omit.From(newID.String()),
		NodeID:          omit.From(handshakeResp.NodeId),
		Name:            omit.From(name),
		BaseURL:         omit.From(baseURL),
		Enabled:         omit.From(true),
		SecretKey:       omit.From(secretKey),
		HealthStatus:    omit.From("healthy"),
		Version:         omit.From(handshakeResp.Version),
		ProtocolVersion: omit.From(handshakeResp.ProtocolVersion),
		Capabilities:    omit.From(handshakeResp.Capabilities),
		CreatedAt:       omit.From(now),
		UpdatedAt:       omit.From(now),
	}

	peerNode, errInsert := xs.db.InsertPeerNode(setter)
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Failed to insert peer node")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save peer node"))
	}

	// Create sync state for this peer.
	_, errSyncState := xs.db.GetOrCreatePeerSyncState(peerNode.ID)
	if errSyncState != nil {
		log.Warn().Err(errSyncState).Str("peer_node_id", peerNode.ID).Msg("Failed to create peer sync state")
	}

	// Trigger initial sync if sync engine is available.
	if xs.syncEngine != nil {
		go xs.syncEngine.SyncPeer(peerNode.ID)
	}

	return connect.NewResponse(&xylona.AddPeerNodeResponse{
		PeerNode: helpers.PeerNodeModelToProto(peerNode),
	}), nil
}

func (xs XylonaService) RemovePeerNode(ctx context.Context, request *connect.Request[xylona.RemovePeerNodeRequest]) (*connect.Response[xylona.RemovePeerNodeResponse], error) {
	peerNodeID := request.Msg.GetPeerNodeId()

	// Stop syncing this peer.
	if xs.syncEngine != nil {
		xs.syncEngine.RemovePeer(peerNodeID)
	}

	// Delete cached remote servers for this peer.
	errDeleteCache := xs.db.DeleteRemoteServerCacheByPeerNodeID(peerNodeID)
	if errDeleteCache != nil {
		log.Warn().Err(errDeleteCache).Str("peer_node_id", peerNodeID).Msg("Failed to delete remote server cache")
	}

	errDelete := xs.db.DeletePeerNodeByID(peerNodeID)
	if errDelete != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to remove peer node"))
	}

	return connect.NewResponse(&xylona.RemovePeerNodeResponse{}), nil
}

func (xs XylonaService) EditPeerNode(ctx context.Context, request *connect.Request[xylona.EditPeerNodeRequest]) (*connect.Response[xylona.EditPeerNodeResponse], error) {
	peerNode, err := xs.db.GetPeerNodeByID(request.Msg.GetPeerNodeId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("peer node not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get peer node"))
	}

	setter := &models.PeerNodeSetter{
		ID:      omit.From(peerNode.ID),
		Name:    omit.From(request.Msg.GetName()),
		BaseURL: omit.From(strings.TrimRight(request.Msg.GetBaseUrl(), "/")),
		Enabled: omit.From(request.Msg.GetEnabled()),
	}
	if request.Msg.GetSecretKey() != "" {
		setter.SecretKey = omit.From(request.Msg.GetSecretKey())
	}

	updatedPeerNode, errUpdate := xs.db.UpdatePeerNode(peerNode, setter)
	if errUpdate != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update peer node"))
	}

	return connect.NewResponse(&xylona.EditPeerNodeResponse{
		PeerNode: helpers.PeerNodeModelToProto(updatedPeerNode),
	}), nil
}

func (xs XylonaService) SyncPeerNode(ctx context.Context, request *connect.Request[xylona.SyncPeerNodeRequest]) (*connect.Response[xylona.SyncPeerNodeResponse], error) {
	peerNodeID := request.Msg.GetPeerNodeId()

	_, err := xs.db.GetPeerNodeByID(peerNodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("peer node not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get peer node"))
	}

	if xs.syncEngine != nil {
		go xs.syncEngine.SyncPeer(peerNodeID)
	}

	return connect.NewResponse(&xylona.SyncPeerNodeResponse{
		Success: true,
	}), nil
}

func (xs XylonaService) ListAggregatedGameServers(ctx context.Context, request *connect.Request[xylona.ListAggregatedGameServersRequest]) (*connect.Response[xylona.ListAggregatedGameServersResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, errUser
	}

	resp := &xylona.ListAggregatedGameServersResponse{}

	// Get local game servers.
	var localServers []*models.GameServer
	var errLocal error
	if user.SuperUser {
		localServers, errLocal = xs.db.GetAllGameServers()
	} else {
		localServers, errLocal = xs.db.GetGameServersByUser(user.ID)
	}
	if errLocal != nil && !errors.Is(errLocal, sql.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get local game servers"))
	}

	for _, gs := range localServers {
		gsProto := helpers.GameServerModelToProto(gs)
		gameServerCmd, errGetCommand := xs.supervisorInst.GetCommandByID(gs.ID)
		if errGetCommand == nil {
			gsProto.Status = helpers.GameServerModelStatusToProtoStatus(string(gameServerCmd.Status()))
		}
		resp.Servers = append(resp.Servers, &xylona.AggregatedGameServer{
			IsLocal:     true,
			LocalServer: gsProto,
		})
	}

	// Get remote cached servers.
	remoteServers, errRemote := xs.db.GetAllRemoteServerCaches()
	if errRemote != nil && !errors.Is(errRemote, sql.ErrNoRows) {
		log.Warn().Err(errRemote).Msg("Failed to get remote server caches")
	}
	for _, rsc := range remoteServers {
		resp.Servers = append(resp.Servers, &xylona.AggregatedGameServer{
			IsLocal:      false,
			RemoteServer: helpers.RemoteServerCacheModelToProto(rsc),
		})
	}

	return connect.NewResponse(resp), nil
}
