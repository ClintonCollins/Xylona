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

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/xycrypt"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetNode returns a node by ID.
func (xs *XylonaService) GetNode(_ context.Context, request *connect.Request[xylona.GetNodeRequest]) (*connect.Response[xylona.GetNodeResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	nodeID := request.Msg.GetNodeId()
	node, err := xs.db.GetNodeByID(nodeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&xylona.GetNodeResponse{Node: helpers.NodeModelToProto(node)}), nil
}

// ListNodes returns all configured nodes.
func (xs *XylonaService) ListNodes(_ context.Context, request *connect.Request[xylona.ListNodesRequest]) (*connect.Response[xylona.ListNodesResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
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

// AddNode adds a remote node using a pairing token.
func (xs *XylonaService) AddNode(_ context.Context, request *connect.Request[xylona.AddNodeRequest]) (*connect.Response[xylona.AddNodeResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}
	nodeProto := request.Msg.GetNode()

	baseURL := strings.TrimSpace(nodeProto.GetBaseUrl())
	if baseURL == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("base URL is required; local nodes are created automatically"))
	}

	pairingToken := strings.TrimSpace(nodeProto.GetSecretKey())
	if pairingToken == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pairing token is required"))
	}

	return xs.addRemoteNode(nodeProto.GetName(), baseURL, nodeProto.GetAllowInsecureTls(), pairingToken, "", int(nodeProto.GetPort()))
}

// GenerateNodePairingObject creates a pairing token and local metadata for node pairing.
func (xs *XylonaService) GenerateNodePairingObject(
	_ context.Context,
	request *connect.Request[xylona.GenerateNodePairingObjectRequest],
) (*connect.Response[xylona.GenerateNodePairingObjectResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}
	if xs.federationMTLS == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("federation mTLS is not configured"))
	}

	normalizedTargetURL := ""
	targetURL := strings.TrimSpace(request.Msg.GetTargetUrl())
	if targetURL != "" {
		var errNormalizeTargetURL error
		normalizedTargetURL, errNormalizeTargetURL = normalizeBaseURL(targetURL)
		if errNormalizeTargetURL != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid target URL"))
		}
	}

	localSettings, errSettings := xs.db.GetLocalSettings()
	if errSettings != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get local settings"))
	}

	localNode, errLocalNode := xs.db.GetNodeByID(localSettings.NodeID)
	if errLocalNode != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get local node"))
	}

	normalizedLocalBaseURL, errNormalizeLocalBaseURL := normalizeBaseURL(localNode.BaseURL)
	if errNormalizeLocalBaseURL != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("local node base URL is not configured"))
	}

	pairingToken, errGeneratePairingToken := xs.db.GeneratePairingToken(normalizedTargetURL)
	if errGeneratePairingToken != nil {
		log.Error().Err(errGeneratePairingToken).Msg("Failed to generate federation pairing token")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to generate pairing token"))
	}

	return connect.NewResponse(&xylona.GenerateNodePairingObjectResponse{
		BaseUrl:      normalizedLocalBaseURL,
		PairingToken: formatPairingToken(pairingToken),
		MtlsPort:     int64(xs.federationMTLS.FederationPort()),
	}), nil
}

// PairNode performs reciprocal pairing between two panels.
func (xs *XylonaService) PairNode(ctx context.Context, request *connect.Request[xylona.PairNodeRequest]) (*connect.Response[xylona.PairNodeResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}
	remoteBaseURL, errNormalizeRemoteBaseURL := normalizeBaseURL(request.Msg.GetRemoteBaseUrl())
	if errNormalizeRemoteBaseURL != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid remote base URL"))
	}

	remoteFederationPort, errNormalizeRemoteFederationPort := normalizeFederationPort(request.Msg.GetRemoteMtlsPort())
	if errNormalizeRemoteFederationPort != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errNormalizeRemoteFederationPort)
	}

	localBaseURL, errNormalizeLocalBaseURL := normalizeBaseURL(request.Msg.GetLocalBaseUrl())
	if errNormalizeLocalBaseURL != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid local base URL"))
	}

	if remoteBaseURL == localBaseURL {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("remote and local URLs must be different"))
	}

	remoteNodeName := strings.TrimSpace(request.Msg.GetRemoteName())
	localNodeName := strings.TrimSpace(request.Msg.GetLocalName())
	remoteAllowInsecureTLS := request.Msg.GetRemoteAllowInsecureTls()
	localAllowInsecureTLS := request.Msg.GetLocalAllowInsecureTls()

	remotePairingToken := strings.TrimSpace(request.Msg.GetRemoteSecretKey())
	if remotePairingToken == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("remote pairing token is required"))
	}

	localPairingToken, errGenerateLocalPairingToken := xs.db.GeneratePairingToken(remoteBaseURL)
	if errGenerateLocalPairingToken != nil {
		log.Error().
			Err(errGenerateLocalPairingToken).
			Str("remote_base_url", remoteBaseURL).
			Msg("Failed to generate local pairing token for reciprocal pair")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to generate local pairing token"))
	}

	httpClient := helpers.NewFederationHTTPClient(federationRequestTimeout, remoteAllowInsecureTLS)
	remoteClient := xylonaconnect.NewXylonaClient(httpClient, remoteBaseURL)

	localFederationPort := int64(0)
	if xs.federationMTLS != nil {
		localFederationPort = int64(xs.federationMTLS.FederationPort())
	}

	remoteAddRequest := connect.NewRequest(&xylona.AddNodeRequest{
		Node: &xylona.Node{
			Name:             localNodeName,
			BaseUrl:          localBaseURL,
			SecretKey:        localPairingToken,
			AllowInsecureTls: localAllowInsecureTLS,
			Port:             localFederationPort,
		},
	})

	remoteCtx, cancelRemote := context.WithTimeout(ctx, federationRequestTimeout)
	defer cancelRemote()

	remoteAlreadyHadNode := false
	var reciprocalNode *xylona.Node
	remoteAddResp, errRemoteAdd := remoteClient.AddNode(remoteCtx, remoteAddRequest)
	if errRemoteAdd != nil {
		if connect.CodeOf(errRemoteAdd) != connect.CodeAlreadyExists {
			log.Warn().
				Err(errRemoteAdd).
				Str("remote_base_url", remoteBaseURL).
				Str("local_base_url", localBaseURL).
				Msg("Pairing failed while adding this panel to remote node")
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("failed to add this panel on remote node"))
		}
		remoteAlreadyHadNode = true
	} else {
		reciprocalNode = remoteAddResp.Msg.GetNode()
	}

	localAddResp, errLocalAdd := xs.addRemoteNode(
		remoteNodeName,
		remoteBaseURL,
		remoteAllowInsecureTLS,
		remotePairingToken,
		localBaseURL,
		remoteFederationPort,
	)
	var localNode *xylona.Node
	if errLocalAdd != nil {
		if connect.CodeOf(errLocalAdd) != connect.CodeAlreadyExists {
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
		existingRemoteNode, errGetExistingRemoteNode := xs.db.GetRemoteNodeByBaseURL(remoteBaseURL)
		if errGetExistingRemoteNode != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load existing remote node"))
		}
		localNode = helpers.NodeModelToProto(existingRemoteNode)
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

func (xs *XylonaService) addRemoteNode(
	name string,
	baseURL string,
	allowInsecureTLS bool,
	pairingToken string,
	localBaseURL string,
	remoteFederationPort int,
) (*connect.Response[xylona.AddNodeResponse], error) {
	normalizedBaseURL, errNormalizeBaseURL := normalizeBaseURL(baseURL)
	if errNormalizeBaseURL != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid base URL"))
	}

	if xs.federationMTLS == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("federation mTLS is not configured"))
	}

	pairingToken = normalizePairingToken(pairingToken)
	if pairingToken == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pairing token is required"))
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

	resolvedLocalBaseURL, errResolveLocalBaseURL := xs.resolveLocalPairingBaseURL(localBaseURL)
	if errResolveLocalBaseURL != nil {
		log.Error().Err(errResolveLocalBaseURL).Msg("Failed to resolve local base URL for pairing token verification")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to resolve local base URL"))
	}

	// Use the pairing token to authenticate with the remote peer and exchange fingerprints.
	pairingResp, remoteFingerprint, errPairing := probePeerAndCompletePairing(
		xs.federationMTLS,
		normalizedBaseURL,
		pairingToken,
		resolvedLocalBaseURL,
		remoteFederationPort,
	)
	if errPairing != nil {
		log.Error().
			Err(errPairing).
			Str("base_url", normalizedBaseURL).
			Msg("Failed to complete pairing with peer node")
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("failed to pair with peer node: "+errPairing.Error()))
	}

	// Prevent self-registration.
	localSettings, errSettings := xs.db.GetLocalSettings()
	if errSettings != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get local settings"))
	}
	if pairingResp.NodeID == localSettings.NodeID {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cannot add self as a peer node"))
	}

	newID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to generate ID"))
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = pairingResp.NodeName
	}

	resolvedRemoteFederationPort := int64(pairingResp.FederationPort)
	if resolvedRemoteFederationPort <= 0 && remoteFederationPort > 0 {
		resolvedRemoteFederationPort = int64(remoteFederationPort)
	}
	if resolvedRemoteFederationPort <= 0 {
		resolvedRemoteFederationPort = int64(xs.federationMTLS.FederationPort())
	}

	now := time.Now()
	setter := &models.NodeSetter{
		ID:               omit.From(newID.String()),
		Name:             omit.From(name),
		IsLocal:          omit.From(false),
		Host:             omit.From(""),
		Port:             omit.From(resolvedRemoteFederationPort),
		BaseURL:          omit.From(normalizedBaseURL),
		AllowInsecureTLS: omit.From(allowInsecureTLS),
		Enabled:          omit.From(true),
		HealthStatus:     omit.From("healthy"),
		Version:          omit.From(""),
		ProtocolVersion:  omit.From(int64(0)),
		Capabilities:     omit.From(""),
		CreatedAt:        omitnull.From(now),
		UpdatedAt:        omitnull.From(now),
	}

	node, errInsert := xs.db.InsertRemoteNode(setter)
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Failed to insert remote node")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save node"))
	}

	errTrust := xs.db.UpsertFederationTrustedPeer(node.ID, pairingResp.NodeID, remoteFingerprint, true, false)
	if errTrust != nil {
		log.Error().Err(errTrust).Str("node_id", node.ID).Msg("Failed to save federation trusted peer")
		errDeleteNode := xs.db.DeleteRemoteNodeByID(node.ID)
		if errDeleteNode != nil {
			log.Warn().Err(errDeleteNode).Str("node_id", node.ID).Msg("Failed to rollback remote node insert after federation trust save failure")
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save federation trust"))
	}

	log.Info().
		Str("node_id", node.ID).
		Str("peer_node_id", pairingResp.NodeID).
		Str("peer_fingerprint", remoteFingerprint).
		Str("base_url", normalizedBaseURL).
		Msg("Remote node added via pairing token — peer certificate fingerprint trusted")

	// Create sync state for this node.
	_, errSyncState := xs.db.GetOrCreatePeerSyncState(node.ID)
	if errSyncState != nil {
		log.Warn().Err(errSyncState).Str("node_id", node.ID).Msg("Failed to create peer sync state")
	}

	// Trigger initial sync if sync engine is available.
	if xs.syncEngine != nil {
		go xs.syncEngine.SyncPeer(node.ID)
	}

	// Peer exchange: notify existing peers about the new node and exchange lists.
	if xs.syncEngine != nil {
		localNode, errLocalNode := xs.db.GetNodeByID(localSettings.NodeID)
		localName := "this node"
		if errLocalNode == nil {
			localName = localNode.Name
		}

		newPeerInfo := &xylona.PeerInfo{
			NodeId:          node.ID,
			Name:            node.Name,
			BaseUrl:         node.BaseURL,
			CertFingerprint: remoteFingerprint,
			FederationPort:  helpers.ClampInt32FromInt64(node.Port),
		}

		// Notify existing peers about the new node.
		xs.syncEngine.BroadcastPeerChange(
			xylona.PeerChangeType_PEER_CHANGE_TYPE_NODE_JOINED,
			newPeerInfo,
			localSettings.NodeID,
			localName,
		)

		// Exchange peer list with the new node so it learns about existing peers.
		go xs.actionsInst.ExchangePeerListWithNode(node.ID)
	}

	return connect.NewResponse(&xylona.AddNodeResponse{
		Node: helpers.NodeModelToProto(node),
	}), nil
}

func (xs *XylonaService) resolveLocalPairingBaseURL(preferredBaseURL string) (string, error) {
	preferredBaseURL = strings.TrimSpace(preferredBaseURL)
	if preferredBaseURL != "" {
		return normalizeBaseURL(preferredBaseURL)
	}

	localSettings, errSettings := xs.db.GetLocalSettings()
	if errSettings != nil {
		if errors.Is(errSettings, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("rpc: load local settings for pairing base URL: %w", errSettings)
	}

	localNode, errLocalNode := xs.db.GetNodeByID(localSettings.NodeID)
	if errLocalNode != nil {
		if errors.Is(errLocalNode, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("rpc: load local node for pairing base URL: %w", errLocalNode)
	}

	baseURL := strings.TrimSpace(localNode.BaseURL)
	if baseURL == "" {
		return "", nil
	}
	return normalizeBaseURL(baseURL)
}

// RemoveNode removes a configured node and its cached data.
func (xs *XylonaService) RemoveNode(_ context.Context, request *connect.Request[xylona.RemoveNodeRequest]) (*connect.Response[xylona.RemoveNodeResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}
	nodeID := request.Msg.GetNodeId()

	// Get node info before deletion for the broadcast.
	node, errGetNode := xs.db.GetNodeByID(nodeID)
	if errGetNode != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("node not found"))
	}

	// Broadcast NODE_REVOKED to all remaining peers BEFORE local deletion.
	if xs.syncEngine != nil && !node.IsLocal {
		localSettings, errSettings := xs.db.GetLocalSettings()
		localName := "this node"
		if errSettings == nil {
			localNode, errLocal := xs.db.GetNodeByID(localSettings.NodeID)
			if errLocal == nil {
				localName = localNode.Name
			}
		}

		trust, _ := xs.db.GetFederationTrustedPeerByNodeID(nodeID)
		fingerprint := ""
		if trust != nil {
			fingerprint = trust.PeerFingerprint
		}

		xs.syncEngine.BroadcastPeerChange(
			xylona.PeerChangeType_PEER_CHANGE_TYPE_NODE_REVOKED,
			&xylona.PeerInfo{
				NodeId:          nodeID,
				Name:            node.Name,
				BaseUrl:         node.BaseURL,
				CertFingerprint: fingerprint,
				FederationPort:  helpers.ClampInt32FromInt64(node.Port),
			},
			localSettings.NodeID,
			localName,
		)
	}

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

// EditNode updates a configured node.
func (xs *XylonaService) EditNode(_ context.Context, request *connect.Request[xylona.EditNodeRequest]) (*connect.Response[xylona.EditNodeResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}
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

// SyncNode triggers an immediate sync for a remote node.
func (xs *XylonaService) SyncNode(_ context.Context, request *connect.Request[xylona.SyncNodeRequest]) (*connect.Response[xylona.SyncNodeResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}
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

// ListAggregatedGameServers lists local and remote servers visible to the caller.
func (xs *XylonaService) ListAggregatedGameServers(ctx context.Context, request *connect.Request[xylona.ListAggregatedGameServersRequest]) (*connect.Response[xylona.ListAggregatedGameServersResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, errUser
	}

	errCleanup := xs.db.DeleteOrphanedRemoteServerCacheByNodeReferences()
	if errCleanup != nil {
		log.Warn().Err(errCleanup).Msg("Failed to clean orphaned remote server cache rows")
	}

	resp := &xylona.ListAggregatedGameServersResponse{}

	// Get local game servers (owned + RBAC-granted for non-super users).
	var localServers []*models.GameServer
	var errLocal error
	if user.SuperUser {
		localServers, errLocal = xs.db.GetAllGameServers()
	} else {
		localServers, errLocal = xs.db.GetGameServersAccessibleByUser(user.ID)
	}
	if errLocal != nil && !errors.Is(errLocal, sql.ErrNoRows) {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get local game servers"))
	}

	localBulkPerms := map[string][]string{}
	if !user.SuperUser {
		var grantedLocalServerIDs []string
		for _, gs := range localServers {
			if gs.UserID != user.ID {
				grantedLocalServerIDs = append(grantedLocalServerIDs, gs.ID)
			}
		}
		if len(grantedLocalServerIDs) > 0 {
			var errBulkPerms error
			localBulkPerms, errBulkPerms = xs.db.GetUserPermissionIDsForServers(user.ID, grantedLocalServerIDs)
			if errBulkPerms != nil {
				log.Error().Err(errBulkPerms).Msg("Failed to get bulk permissions for aggregated list")
				localBulkPerms = map[string][]string{}
			}
		}
	}

	for _, gs := range localServers {
		gsProto := helpers.GameServerModelToProto(gs, xs.versionState)
		gameServerCmd, errGetCommand := xs.supervisorInst.GetCommandByID(gs.ID)
		if errGetCommand == nil {
			gsProto.Status = gameServerCmd.Status()
		}
		gsProto.Version, gsProto.VersionInfo = xs.resolveLocalVersionData(ctx, gs, actions.VersionResolveOptions{
			AllowAsync: true,
		})
		if user.SuperUser || gs.UserID == user.ID {
			gsProto.EffectivePermissions = xs.allPermissionIDs
		} else if perms, ok := localBulkPerms[gs.ID]; ok {
			gsProto.EffectivePermissions = perms
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
		workerGroup.Go(func() {
			summaries, usedStaleData, errSummaries := xs.listRemoteNodeSummaries(ctx, node, user)
			results <- remoteNodeResult{
				nodeID:        node.ID,
				summaries:     summaries,
				usedStaleData: usedStaleData,
				err:           errSummaries,
			}
		})
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
			compositeKey := summary.GetSourceNodeId() + "/" + summary.GetRemoteServerId()
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

func (xs *XylonaService) listRemoteNodeSummaries(ctx context.Context, node *models.Node, actingUser *models.User) ([]*xylona.RemoteServerSummary, bool, error) {
	summaries, errFetch := xs.fetchRemoteNodeSummaries(ctx, node, actingUser)
	if errFetch == nil {
		if actingUser == nil || actingUser.SuperUser {
			if xs.listCache != nil {
				xs.listCache.set(node.ID, summaries, time.Now())
			}
			xs.syncRemoteServerCacheSummaries(node, summaries)
		}
		return summaries, false, nil
	}

	if actingUser != nil && !actingUser.SuperUser {
		return nil, false, errFetch
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

func (xs *XylonaService) fetchRemoteNodeSummaries(ctx context.Context, node *models.Node, actingUser *models.User) ([]*xylona.RemoteServerSummary, error) {
	client, errClient := xs.newRemoteFederationClient(node)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationListServerSummariesRequest{})
	if actingUser == nil || !actingUser.SuperUser {
		errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
		if errIdentity != nil {
			return nil, errIdentity
		}
	}

	remoteCtx, cancel := context.WithTimeout(ctx, federationRequestTimeout)
	defer cancel()

	resp, errList := client.ListServerSummaries(remoteCtx, req)
	if errList != nil {
		return nil, fmt.Errorf("rpc: list remote server summaries: %w", errList)
	}

	lastSyncedAt := timestamppb.New(time.Now())
	summaries := make([]*xylona.RemoteServerSummary, 0, len(resp.Msg.GetServers()))
	for _, server := range resp.Msg.GetServers() {
		if server == nil {
			continue
		}
		summaries = append(summaries, &xylona.RemoteServerSummary{
			Id:               node.ID + "/" + server.GetServerId(),
			SourceNodeId:     node.ID,
			NodeId:           node.ID,
			RemoteServerId:   server.GetServerId(),
			DisplayName:      server.GetDisplayName(),
			Status:           server.GetStatus(),
			GameName:         server.GetGameName(),
			GameId:           server.GetGameId(),
			IpAddress:        server.GetIpAddress(),
			Port:             server.GetPort(),
			QueryPort:        server.GetQueryPort(),
			MaxPlayers:       server.GetMaxPlayers(),
			CurrentPlayers:   server.GetCurrentPlayers(),
			MapName:          server.GetMapName(),
			Version:          server.GetVersion(),
			NodeName:         node.Name,
			NodeHost:         node.BaseURL,
			LastRemoteUpdate: server.GetUpdatedAt(),
			LastSyncedAt:     lastSyncedAt,
			IsStale:          false,
			VersionInfo:      server.GetVersionInfo(),
		})
	}

	return summaries, nil
}

func (xs *XylonaService) syncRemoteServerCacheSummaries(node *models.Node, summaries []*xylona.RemoteServerSummary) {
	for _, summary := range summaries {
		if summary == nil {
			continue
		}

		newID, errID := helpers.GenerateUniqueID()
		if errID != nil {
			log.Warn().
				Err(errID).
				Str("node_id", node.ID).
				Str("remote_server_id", summary.GetRemoteServerId()).
				Msg("Failed to generate remote cache ID while syncing remote summaries")
			continue
		}

		sourceNodeID := strings.TrimSpace(summary.GetSourceNodeId())
		if sourceNodeID == "" {
			sourceNodeID = node.ID
		}

		cacheNodeID := strings.TrimSpace(summary.GetNodeId())
		if cacheNodeID == "" {
			cacheNodeID = node.ID
		}

		nodeName := strings.TrimSpace(summary.GetNodeName())
		if nodeName == "" {
			nodeName = node.Name
		}

		nodeHost := strings.TrimSpace(summary.GetNodeHost())
		if nodeHost == "" {
			nodeHost = node.BaseURL
		}

		lastRemoteUpdate := time.Now()
		if summary.GetLastRemoteUpdate() != nil && !summary.GetLastRemoteUpdate().AsTime().IsZero() {
			lastRemoteUpdate = summary.GetLastRemoteUpdate().AsTime()
		}

		errUpsertCache := xs.db.UpsertRemoteServerCache(
			newID.String(),
			sourceNodeID,
			cacheNodeID,
			summary.GetRemoteServerId(),
			summary.GetDisplayName(),
			summary.GetStatus().String(),
			summary.GetGameName(),
			summary.GetGameId(),
			summary.GetIpAddress(),
			helpers.ClampInt32FromInt64(summary.GetPort()),
			helpers.ClampInt32FromInt64(summary.GetQueryPort()),
			helpers.ClampInt32FromInt64(summary.GetMaxPlayers()),
			helpers.ClampInt32FromInt64(summary.GetCurrentPlayers()),
			summary.GetMapName(),
			summary.GetVersion(),
			nodeName,
			nodeHost,
			lastRemoteUpdate,
		)
		if errUpsertCache != nil {
			log.Warn().
				Err(errUpsertCache).
				Str("node_id", node.ID).
				Str("remote_server_id", summary.GetRemoteServerId()).
				Msg("Failed to upsert remote server cache from live remote summary")
		}
	}
}

// VerifyNode verifies a node secret key and returns the local node identity.
func (xs *XylonaService) VerifyNode(ctx context.Context, request *connect.Request[xylona.VerifyNodeRequest]) (*connect.Response[xylona.VerifyNodeResponse], error) {
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

// ListLocalSecretKeys lists locally managed secret keys.
func (xs *XylonaService) ListLocalSecretKeys(_ context.Context, request *connect.Request[xylona.ListLocalSecretKeysRequest]) (*connect.Response[xylona.ListLocalSecretKeysResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}
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

// CreateLocalSecretKey creates a new local secret key.
func (xs *XylonaService) CreateLocalSecretKey(_ context.Context, request *connect.Request[xylona.CreateLocalSecretKeyRequest]) (*connect.Response[xylona.CreateLocalSecretKeyResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}
	localSecretKey, key, errCreateSecretKey := xs.buildLocalSecretKey(request.Msg.GetName())
	if errCreateSecretKey != nil {
		log.Error().Err(errCreateSecretKey).Msg("Unable to create local secret key.")
		return nil, connect.NewError(connect.CodeInternal, errCreateSecretKey)
	}

	mtlsPort := int64(0)
	if xs.federationMTLS != nil {
		mtlsPort = int64(xs.federationMTLS.FederationPort())
	}

	resp := &xylona.CreateLocalSecretKeyResponse{
		Id:        localSecretKey.ID,
		SecretKey: key,
		MtlsPort:  mtlsPort,
	}
	return connect.NewResponse(resp), nil
}

// DeleteLocalSecretKey deletes a local secret key.
func (xs *XylonaService) DeleteLocalSecretKey(_ context.Context, request *connect.Request[xylona.DeleteLocalSecretKeyRequest]) (*connect.Response[xylona.DeleteLocalSecretKeyResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}
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

func normalizeFederationPort(rawPort int64) (int, error) {
	if rawPort < 0 {
		return 0, errors.New("remote mTLS port cannot be negative")
	}
	if rawPort == 0 {
		return 0, nil
	}
	if rawPort > 65535 {
		return 0, errors.New("remote mTLS port must be between 1 and 65535")
	}

	return int(rawPort), nil
}

func (xs *XylonaService) buildLocalSecretKey(name string) (*models.LocalSecretKey, string, error) {
	key, errGenerateKey := helpers.GenerateRandomString(64)
	if errGenerateKey != nil {
		return nil, "", fmt.Errorf("rpc: generate local secret key: %w", errGenerateKey)
	}

	secretKeyHash, errHashSecret := xycrypt.GenerateHashFromString(key, xycrypt.DefaultHashParameters)
	if errHashSecret != nil {
		return nil, "", fmt.Errorf("rpc: hash local secret key: %w", errHashSecret)
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
		return nil, "", fmt.Errorf("rpc: insert local secret key: %w", errInsertKey)
	}

	return localSecretKey, key, nil
}
