package rpc

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/xycrypt"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (xs XylonaService) GetNode(ctx context.Context, request *connect.Request[xylona.GetNodeRequest]) (*connect.Response[xylona.GetNodeResponse], error) {
	nodeID := request.Msg.GetNodeId()
	node, err := xs.db.GetNodeByID(nodeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&xylona.GetNodeResponse{Node: helpers.NodeModelToProto(node)}), nil
}

func (xs XylonaService) ListNodes(ctx context.Context, request *connect.Request[xylona.ListNodesRequest]) (*connect.Response[xylona.ListNodesResponse], error) {
	nodes, err := xs.db.GetAllNodes()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := &xylona.ListNodesResponse{}
	for _, node := range nodes {
		resp.Nodes = append(resp.Nodes, helpers.NodeModelToProto(node))
	}
	return connect.NewResponse(resp), nil
}

func (xs XylonaService) AddNode(ctx context.Context, request *connect.Request[xylona.AddNodeRequest]) (*connect.Response[xylona.AddNodeResponse], error) {
	nodeProto := request.Msg.GetNode()

	// If a base_url is provided, treat this as a remote/peer node add with handshake.
	baseURL := strings.TrimRight(nodeProto.GetBaseUrl(), "/")
	if baseURL != "" {
		return xs.addRemoteNode(ctx, nodeProto.GetName(), baseURL, nodeProto.GetSecretKey())
	}

	// Otherwise, add a simple local node.
	nodeModel := helpers.NodeProtoToModel(nodeProto)
	if nodeModel.ID == "" {
		newID, errID := helpers.GenerateUniqueID()
		if errID != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to generate node ID"))
		}
		nodeModel.ID = newID.String()
	}
	node, err := xs.db.InsertNode(helpers.NodeModelToSetter(nodeModel))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&xylona.AddNodeResponse{Node: helpers.NodeModelToProto(node)}), nil
}

func (xs XylonaService) addRemoteNode(ctx context.Context, name string, baseURL string, secretKey string) (*connect.Response[xylona.AddNodeResponse], error) {
	// Validate base URL.
	parsedURL, errParse := url.Parse(baseURL)
	if errParse != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid base URL"))
	}

	// Check for duplicate base URL.
	_, errExisting := xs.db.GetRemoteNodeByBaseURL(baseURL)
	if errExisting == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("node with this URL already exists"))
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

	newID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to generate ID"))
	}

	if name == "" {
		name = handshakeResp.NodeName
	}

	now := time.Now()
	setter := &models.NodeSetter{
		ID:              omit.From(newID.String()),
		Name:            omit.From(name),
		IsLocal:         omit.From(false),
		Host:            omit.From(""),
		Port:            omit.From(int32(0)),
		BaseURL:         omit.From(baseURL),
		Enabled:         omit.From(true),
		SecretKey:       omitnull.From(secretKey),
		HealthStatus:    omit.From("healthy"),
		Version:         omit.From(handshakeResp.Version),
		ProtocolVersion: omit.From(handshakeResp.ProtocolVersion),
		Capabilities:    omit.From(handshakeResp.Capabilities),
		CreatedAt:       omitnull.From(now),
		UpdatedAt:       omitnull.From(now),
	}

	node, errInsert := xs.db.InsertRemoteNode(setter)
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Failed to insert remote node")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save node"))
	}

	// Create sync state for this node.
	_, errSyncState := xs.db.GetOrCreatePeerSyncState(node.ID)
	if errSyncState != nil {
		log.Warn().Err(errSyncState).Str("node_id", node.ID).Msg("Failed to create peer sync state")
	}

	// Trigger initial sync if sync engine is available.
	if xs.syncEngine != nil {
		go xs.syncEngine.SyncPeer(node.ID)
	}

	return connect.NewResponse(&xylona.AddNodeResponse{
		Node: helpers.NodeModelToProto(node),
	}), nil
}

func (xs XylonaService) RemoveNode(ctx context.Context, request *connect.Request[xylona.RemoveNodeRequest]) (*connect.Response[xylona.RemoveNodeResponse], error) {
	nodeID := request.Msg.GetNodeId()

	// Stop syncing this node if it's a remote peer.
	if xs.syncEngine != nil {
		xs.syncEngine.RemovePeer(nodeID)
	}

	// Delete cached remote servers for this node.
	errDeleteCache := xs.db.DeleteRemoteServerCacheByNodeID(nodeID)
	if errDeleteCache != nil {
		log.Warn().Err(errDeleteCache).Str("node_id", nodeID).Msg("Failed to delete remote server cache")
	}

	errDelete := xs.db.DeleteNodeByID(nodeID)
	if errDelete != nil {
		return nil, connect.NewError(connect.CodeInternal, errDelete)
	}
	return connect.NewResponse(&xylona.RemoveNodeResponse{}), nil
}

func (xs XylonaService) EditNode(ctx context.Context, request *connect.Request[xylona.EditNodeRequest]) (*connect.Response[xylona.EditNodeResponse], error) {
	nodeModel := helpers.NodeProtoToModel(request.Msg.GetNode())
	node, err := xs.db.UpdateNode(nodeModel, helpers.NodeModelToSetter(nodeModel))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&xylona.EditNodeResponse{Node: helpers.NodeModelToProto(node)}), nil
}

func (xs XylonaService) SyncNode(ctx context.Context, request *connect.Request[xylona.SyncNodeRequest]) (*connect.Response[xylona.SyncNodeResponse], error) {
	nodeID := request.Msg.GetNodeId()

	_, err := xs.db.GetRemoteNodeByID(nodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("node not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get node"))
	}

	if xs.syncEngine != nil {
		go xs.syncEngine.SyncPeer(nodeID)
	}

	return connect.NewResponse(&xylona.SyncNodeResponse{
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

func (xs XylonaService) VerifyNode(ctx context.Context, request *connect.Request[xylona.VerifyNodeRequest]) (*connect.Response[xylona.VerifyNodeResponse], error) {
	secretKey := request.Msg.GetSecretKey()
	secretKeyHash, err := xycrypt.GenerateHashFromString(secretKey, xycrypt.DefaultHashParameters)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	key, errGetSecretKey := xs.db.GetSecretKeyByHash(string(secretKeyHash))
	if errGetSecretKey != nil {
		if !errors.Is(errGetSecretKey, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeInternal, errGetSecretKey)
		}
		return nil, connect.NewError(connect.CodePermissionDenied, errGetSecretKey)
	}

	key.LastUsedAt = null.From(time.Now())
	key.LastAccessedFrom = null.From(request.Peer().Addr)
	log.Debug().Msgf("%+v", request.Header())

	node, errNode := models.Nodes.Query(models.SelectWhere.Nodes.IsLocal.EQ(true)).One(ctx, xs.db.DB)
	if errNode != nil {
		return nil, connect.NewError(connect.CodeInternal, errNode)
	}
	return connect.NewResponse(&xylona.VerifyNodeResponse{
		Node: helpers.NodeModelToProto(node),
	}), nil
}

func (xs XylonaService) ListLocalSecretKeys(ctx context.Context, request *connect.Request[xylona.ListLocalSecretKeysRequest]) (*connect.Response[xylona.ListLocalSecretKeysResponse], error) {
	secretKeys, err := xs.db.GetAllSecretKeys()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := &xylona.ListLocalSecretKeysResponse{}
	for _, secretKey := range secretKeys {
		resp.SecretKeys = append(resp.SecretKeys, &xylona.SecretKey{
			Id:               secretKey.ID,
			Name:             secretKey.Name,
			CreatedAt:        timestamppb.New(secretKey.CreatedAt),
			LastUsed:         timestamppb.New(secretKey.LastUsedAt.GetOr(time.Time{})),
			LastAccessedFrom: secretKey.LastAccessedFrom.GetOr(""),
		})
	}
	return connect.NewResponse(resp), nil
}

func (xs XylonaService) CreateLocalSecretKey(ctx context.Context, request *connect.Request[xylona.CreateLocalSecretKeyRequest]) (*connect.Response[xylona.CreateLocalSecretKeyResponse], error) {
	key, err := helpers.GenerateRandomString(64)
	if err != nil {
		log.Error().Err(err).Msg("Unable to generate random string.")
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	secretKeyHash, errHash := xycrypt.GenerateHashFromString(key, xycrypt.DefaultHashParameters)
	if errHash != nil {
		log.Error().Err(errHash).Msg("Unable to generate hash from string.")
		return nil, connect.NewError(connect.CodeInternal, errHash)
	}
	newKeySetter := &models.LocalSecretKeySetter{
		Name:             omit.From(request.Msg.GetName()),
		SecretKeyHash:    omit.From(string(secretKeyHash)),
		LastAccessedFrom: omitnull.From(""),
		LastUsedAt:       omitnull.From(time.Now()),
		CreatedAt:        omit.From(time.Now()),
	}
	localSecretKey, errInsert := xs.db.InsertSecretKey(newKeySetter)
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Unable to insert secret key.")
		return nil, connect.NewError(connect.CodeInternal, errInsert)
	}
	resp := &xylona.CreateLocalSecretKeyResponse{
		Id:        localSecretKey.ID,
		SecretKey: key,
	}
	return connect.NewResponse(resp), nil
}

func (xs XylonaService) DeleteLocalSecretKey(ctx context.Context, request *connect.Request[xylona.DeleteLocalSecretKeyRequest]) (*connect.Response[xylona.DeleteLocalSecretKeyResponse], error) {
	id := request.Msg.GetId()
	err := xs.db.DeleteSecretKeyByID(id)
	if err != nil {
		log.Error().Err(err).Msg("Unable to delete secret key.")
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&xylona.DeleteLocalSecretKeyResponse{}), nil
}
