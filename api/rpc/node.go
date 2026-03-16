package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
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
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
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

	baseURL := strings.TrimSpace(nodeProto.GetBaseUrl())
	if baseURL == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("base URL is required; local nodes are created automatically"))
	}

	return xs.addRemoteNode(ctx, nodeProto.GetName(), baseURL, nodeProto.GetSecretKey(), nodeProto.GetAllowInsecureTls())
}

func (xs XylonaService) PairNode(ctx context.Context, request *connect.Request[xylona.PairNodeRequest]) (*connect.Response[xylona.PairNodeResponse], error) {
	remoteBaseURL, errNormalizeRemoteBaseURL := normalizeBaseURL(request.Msg.GetRemoteBaseUrl())
	if errNormalizeRemoteBaseURL != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid remote base URL"))
	}

	localBaseURL, errNormalizeLocalBaseURL := normalizeBaseURL(request.Msg.GetLocalBaseUrl())
	if errNormalizeLocalBaseURL != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid local base URL"))
	}

	if remoteBaseURL == localBaseURL {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("remote and local URLs must be different"))
	}

	remoteSecretKey := strings.TrimSpace(request.Msg.GetRemoteSecretKey())
	if remoteSecretKey == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("remote secret key is required"))
	}

	remoteNodeName := strings.TrimSpace(request.Msg.GetRemoteName())
	localNodeName := strings.TrimSpace(request.Msg.GetLocalName())
	remoteAllowInsecureTLS := request.Msg.GetRemoteAllowInsecureTls()
	localAllowInsecureTLS := request.Msg.GetLocalAllowInsecureTls()

	secretKeyName := fmt.Sprintf("pairing:%s:%s", remoteBaseURL, time.Now().UTC().Format(time.RFC3339))
	_, localSecretKey, errCreateKey := xs.createLocalSecretKey(secretKeyName)
	if errCreateKey != nil {
		log.Error().Err(errCreateKey).Str("remote_base_url", remoteBaseURL).Msg("Failed to create local secret key for reciprocal node add")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create local secret key"))
	}

	httpClient := helpers.NewFederationHTTPClient(federationRequestTimeout, remoteAllowInsecureTLS)
	remoteClient := xylonaconnect.NewXylonaClient(httpClient, remoteBaseURL)
	remoteAddRequest := connect.NewRequest(&xylona.AddNodeRequest{
		Node: &xylona.Node{
			Name:             localNodeName,
			BaseUrl:          localBaseURL,
			SecretKey:        localSecretKey,
			AllowInsecureTls: localAllowInsecureTLS,
		},
	})

	remoteCtx, cancelRemote := context.WithTimeout(ctx, federationRequestTimeout)
	defer cancelRemote()

	remoteAlreadyHadNode := false
	var reciprocalNode *xylona.Node
	remoteAddResp, errRemoteAdd := remoteClient.AddNode(remoteCtx, remoteAddRequest)
	if errRemoteAdd != nil {
		if connect.CodeOf(errRemoteAdd) == connect.CodeAlreadyExists {
			remoteAlreadyHadNode = true
		} else {
			log.Warn().
				Err(errRemoteAdd).
				Str("remote_base_url", remoteBaseURL).
				Str("local_base_url", localBaseURL).
				Msg("Pairing failed while adding this panel to remote node")

			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("failed to add this panel on remote node"))
		}
	} else {
		reciprocalNode = remoteAddResp.Msg.GetNode()
	}

	localAddResp, errLocalAdd := xs.addRemoteNode(ctx, remoteNodeName, remoteBaseURL, remoteSecretKey, remoteAllowInsecureTLS)
	var localNode *xylona.Node
	if errLocalAdd != nil {
		if connect.CodeOf(errLocalAdd) == connect.CodeAlreadyExists {
			existingRemoteNode, errGetExistingRemoteNode := xs.db.GetRemoteNodeByBaseURL(remoteBaseURL)
			if errGetExistingRemoteNode != nil {
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load existing remote node"))
			}
			localNode = helpers.NodeModelToProto(existingRemoteNode)
		} else {
			// Best-effort rollback if this request added the local node to the remote panel.
			if reciprocalNode != nil && reciprocalNode.GetId() != "" {
				rollbackCtx, cancelRollback := context.WithTimeout(ctx, federationRequestTimeout)
				defer cancelRollback()
				_, errRollback := remoteClient.RemoveNode(rollbackCtx, connect.NewRequest(&xylona.RemoveNodeRequest{
					NodeId: reciprocalNode.GetId(),
				}))
				if errRollback != nil {
					log.Warn().
						Err(errRollback).
						Str("remote_base_url", remoteBaseURL).
						Str("remote_node_id", reciprocalNode.GetId()).
						Msg("Failed to rollback reciprocal node add after local add failure")
				}
			}
			return nil, errLocalAdd
		}
	} else {
		localNode = localAddResp.Msg.GetNode()
	}

	reciprocalError := ""
	if remoteAlreadyHadNode {
		reciprocalError = "remote panel already had this node configured"
	}
	reciprocalAdded := reciprocalNode != nil && !remoteAlreadyHadNode

	return connect.NewResponse(&xylona.PairNodeResponse{
		Node:            localNode,
		ReciprocalAdded: reciprocalAdded,
		ReciprocalError: reciprocalError,
		ReciprocalNode:  reciprocalNode,
	}), nil
}

func (xs XylonaService) addRemoteNode(ctx context.Context, name string, baseURL string, secretKey string, allowInsecureTLS bool) (*connect.Response[xylona.AddNodeResponse], error) {
	normalizedBaseURL, errNormalizeBaseURL := normalizeBaseURL(baseURL)
	if errNormalizeBaseURL != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid base URL"))
	}

	secretKey = strings.TrimSpace(secretKey)
	if secretKey == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("secret key is required"))
	}

	// Check for duplicate base URL.
	_, errExisting := xs.db.GetRemoteNodeByBaseURL(normalizedBaseURL)
	if errExisting == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("node with this URL already exists"))
	}
	if !errors.Is(errExisting, sql.ErrNoRows) {
		log.Error().Err(errExisting).Str("base_url", normalizedBaseURL).Msg("Failed to check existing remote node by base URL")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check existing nodes"))
	}

	// Perform handshake to verify connectivity and get peer identity.
	handshakeResp, errHandshake := performHandshake(ctx, normalizedBaseURL, secretKey, allowInsecureTLS)
	if errHandshake != nil {
		log.Error().Err(errHandshake).Str("base_url", normalizedBaseURL).Msg("Failed to handshake with peer node")
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

	name = strings.TrimSpace(name)
	if name == "" {
		name = handshakeResp.NodeName
	}

	now := time.Now()
	setter := &models.NodeSetter{
		ID:               omit.From(newID.String()),
		Name:             omit.From(name),
		IsLocal:          omit.From(false),
		Host:             omit.From(""),
		Port:             omit.From(int32(0)),
		BaseURL:          omit.From(normalizedBaseURL),
		AllowInsecureTLS: omit.From(allowInsecureTLS),
		Enabled:          omit.From(true),
		SecretKey:        omitnull.From(secretKey),
		HealthStatus:     omit.From("healthy"),
		Version:          omit.From(handshakeResp.Version),
		ProtocolVersion:  omit.From(handshakeResp.ProtocolVersion),
		Capabilities:     omit.From(handshakeResp.Capabilities),
		CreatedAt:        omitnull.From(now),
		UpdatedAt:        omitnull.From(now),
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
	if xs.listCache != nil {
		xs.listCache.invalidate(nodeID)
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
	if !node.IsLocal && xs.listCache != nil {
		xs.listCache.invalidate(node.ID)
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

	errCleanup := xs.db.DeleteOrphanedRemoteServerCacheByNodeReferences()
	if errCleanup != nil {
		log.Warn().Err(errCleanup).Msg("Failed to clean orphaned remote server cache rows")
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
			gsProto.Status = gameServerCmd.Status()
		}
		resp.Servers = append(resp.Servers, &xylona.AggregatedGameServer{
			IsLocal:     true,
			LocalServer: gsProto,
		})
	}

	remoteNodes, errRemoteNodes := xs.db.GetEnabledRemoteNodes()
	if errRemoteNodes != nil && !errors.Is(errRemoteNodes, sql.ErrNoRows) {
		log.Warn().Err(errRemoteNodes).Msg("Failed to get enabled remote nodes")
		return connect.NewResponse(resp), nil
	}

	type remoteNodeResult struct {
		nodeID        string
		summaries     []*xylona.RemoteServerSummary
		usedStaleData bool
		err           error
	}

	results := make(chan remoteNodeResult, len(remoteNodes))
	var workerGroup sync.WaitGroup
	for _, remoteNode := range remoteNodes {
		node := remoteNode
		workerGroup.Add(1)
		go func() {
			defer workerGroup.Done()
			summaries, usedStaleData, errSummaries := xs.listRemoteNodeSummaries(ctx, node)
			results <- remoteNodeResult{
				nodeID:        node.ID,
				summaries:     summaries,
				usedStaleData: usedStaleData,
				err:           errSummaries,
			}
		}()
	}

	go func() {
		workerGroup.Wait()
		close(results)
	}()

	resultByNodeID := make(map[string]remoteNodeResult, len(remoteNodes))
	for result := range results {
		resultByNodeID[result.nodeID] = result
	}

	seenRemote := make(map[string]struct{})
	for _, remoteNode := range remoteNodes {
		result, exists := resultByNodeID[remoteNode.ID]
		if !exists {
			continue
		}
		if result.err != nil {
			log.Warn().
				Err(result.err).
				Str("node_id", remoteNode.ID).
				Str("base_url", remoteNode.BaseURL).
				Msg("Failed to load remote node server list")
			continue
		}
		if result.usedStaleData {
			log.Warn().
				Str("node_id", remoteNode.ID).
				Str("base_url", remoteNode.BaseURL).
				Msg("Using stale in-memory server list for remote node")
		}

		for _, summary := range result.summaries {
			if summary == nil {
				continue
			}
			compositeKey := summary.SourceNodeId + "/" + summary.RemoteServerId
			if _, duplicate := seenRemote[compositeKey]; duplicate {
				continue
			}
			seenRemote[compositeKey] = struct{}{}

			resp.Servers = append(resp.Servers, &xylona.AggregatedGameServer{
				IsLocal:      false,
				RemoteServer: summary,
			})
		}
	}

	return connect.NewResponse(resp), nil
}

func (xs XylonaService) listRemoteNodeSummaries(ctx context.Context, node *models.Node) ([]*xylona.RemoteServerSummary, bool, error) {
	summaries, errFetch := xs.fetchRemoteNodeSummaries(ctx, node)
	if errFetch == nil {
		if xs.listCache != nil {
			xs.listCache.set(node.ID, summaries, time.Now())
		}
		xs.syncRemoteServerCacheSummaries(node, summaries)
		return summaries, false, nil
	}

	if xs.listCache == nil {
		return nil, false, errFetch
	}

	staleSummaries, staleFetchedAt, hasStale := xs.listCache.getAny(node.ID)
	if !hasStale {
		return nil, false, errFetch
	}

	return markRemoteServerSummariesStale(staleSummaries, staleFetchedAt), true, nil
}

func (xs XylonaService) fetchRemoteNodeSummaries(ctx context.Context, node *models.Node) ([]*xylona.RemoteServerSummary, error) {
	client, secretKey := newRemoteFederationClient(node)
	req := connect.NewRequest(&xylona.FederationListServerSummariesRequest{})
	req.Header().Set("X-Federation-Key", secretKey)

	remoteCtx, cancel := context.WithTimeout(ctx, federationRequestTimeout)
	defer cancel()

	resp, errList := client.ListServerSummaries(remoteCtx, req)
	if errList != nil {
		return nil, errList
	}

	lastSyncedAt := timestamppb.New(time.Now())
	summaries := make([]*xylona.RemoteServerSummary, 0, len(resp.Msg.Servers))
	for _, server := range resp.Msg.Servers {
		if server == nil {
			continue
		}
		summaries = append(summaries, &xylona.RemoteServerSummary{
			Id:               node.ID + "/" + server.ServerId,
			SourceNodeId:     node.ID,
			NodeId:           node.ID,
			RemoteServerId:   server.ServerId,
			DisplayName:      server.DisplayName,
			Status:           server.Status,
			GameName:         server.GameName,
			GameId:           server.GameId,
			IpAddress:        server.IpAddress,
			Port:             server.Port,
			QueryPort:        server.QueryPort,
			MaxPlayers:       server.MaxPlayers,
			CurrentPlayers:   server.CurrentPlayers,
			MapName:          server.MapName,
			Version:          server.Version,
			NodeName:         node.Name,
			NodeHost:         node.BaseURL,
			LastRemoteUpdate: server.UpdatedAt,
			LastSyncedAt:     lastSyncedAt,
			IsStale:          false,
		})
	}

	return summaries, nil
}

func (xs XylonaService) syncRemoteServerCacheSummaries(node *models.Node, summaries []*xylona.RemoteServerSummary) {
	for _, summary := range summaries {
		if summary == nil {
			continue
		}

		newID, errID := helpers.GenerateUniqueID()
		if errID != nil {
			log.Warn().
				Err(errID).
				Str("node_id", node.ID).
				Str("remote_server_id", summary.RemoteServerId).
				Msg("Failed to generate remote cache ID while syncing remote summaries")
			continue
		}

		sourceNodeID := strings.TrimSpace(summary.SourceNodeId)
		if sourceNodeID == "" {
			sourceNodeID = node.ID
		}

		cacheNodeID := strings.TrimSpace(summary.NodeId)
		if cacheNodeID == "" {
			cacheNodeID = node.ID
		}

		nodeName := strings.TrimSpace(summary.NodeName)
		if nodeName == "" {
			nodeName = node.Name
		}

		nodeHost := strings.TrimSpace(summary.NodeHost)
		if nodeHost == "" {
			nodeHost = node.BaseURL
		}

		lastRemoteUpdate := time.Now()
		if summary.LastRemoteUpdate != nil && !summary.LastRemoteUpdate.AsTime().IsZero() {
			lastRemoteUpdate = summary.LastRemoteUpdate.AsTime()
		}

		errUpsertCache := xs.db.UpsertRemoteServerCache(
			newID.String(),
			sourceNodeID,
			cacheNodeID,
			summary.RemoteServerId,
			summary.DisplayName,
			summary.Status.String(),
			summary.GameName,
			summary.GameId,
			summary.IpAddress,
			int32(summary.Port),
			int32(summary.QueryPort),
			int32(summary.MaxPlayers),
			int32(summary.CurrentPlayers),
			summary.MapName,
			summary.Version,
			nodeName,
			nodeHost,
			lastRemoteUpdate,
		)
		if errUpsertCache != nil {
			log.Warn().
				Err(errUpsertCache).
				Str("node_id", node.ID).
				Str("remote_server_id", summary.RemoteServerId).
				Msg("Failed to upsert remote server cache from live remote summary")
		}
	}
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
	localSecretKey, key, errCreateSecretKey := xs.createLocalSecretKey(request.Msg.GetName())
	if errCreateSecretKey != nil {
		log.Error().Err(errCreateSecretKey).Msg("Unable to create local secret key.")
		return nil, connect.NewError(connect.CodeInternal, errCreateSecretKey)
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

func normalizeBaseURL(baseURL string) (string, error) {
	normalizedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalizedBaseURL == "" {
		return "", errors.New("base URL is required")
	}

	parsedURL, errParseURL := url.Parse(normalizedBaseURL)
	if errParseURL != nil {
		return "", errors.New("invalid base URL")
	}
	if (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return "", errors.New("invalid base URL")
	}

	return normalizedBaseURL, nil
}

func (xs XylonaService) createLocalSecretKey(name string) (*models.LocalSecretKey, string, error) {
	key, errGenerateKey := helpers.GenerateRandomString(64)
	if errGenerateKey != nil {
		return nil, "", errGenerateKey
	}

	secretKeyHash, errHashSecret := xycrypt.GenerateHashFromString(key, xycrypt.DefaultHashParameters)
	if errHashSecret != nil {
		return nil, "", errHashSecret
	}

	now := time.Now()
	newKeySetter := &models.LocalSecretKeySetter{
		Name:             omit.From(strings.TrimSpace(name)),
		SecretKeyHash:    omit.From(string(secretKeyHash)),
		LastAccessedFrom: omitnull.From(""),
		LastUsedAt:       omitnull.From(now),
		CreatedAt:        omit.From(now),
	}

	localSecretKey, errInsertKey := xs.db.InsertSecretKey(newKeySetter)
	if errInsertKey != nil {
		return nil, "", errInsertKey
	}

	return localSecretKey, key, nil
}
