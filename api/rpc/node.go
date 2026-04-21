package rpc

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetNode returns a node by ID.
func (xs *XylonaService) GetNode(ctx context.Context, request *connect.Request[xylona.GetNodeRequest]) (*connect.Response[xylona.GetNodeResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	nodeID := request.Msg.GetNodeId()
	node, err := xs.db.GetNodeByID(nodeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&xylona.GetNodeResponse{Node: xs.nodeProtoWithRuntime(ctx, node)}), nil
}

// ListNodes returns all configured nodes.
func (xs *XylonaService) ListNodes(ctx context.Context, request *connect.Request[xylona.ListNodesRequest]) (*connect.Response[xylona.ListNodesResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	nodes, err := xs.db.GetAllNodes()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	selfNodeID := xs.selfNodeID()
	runtimeState := xs.collectNodeRuntimeState(ctx, nodes)

	resp := &xylona.ListNodesResponse{}
	for _, node := range nodes {
		resp.Nodes = append(resp.Nodes, xs.nodeProtoWithRuntimeState(node, selfNodeID, runtimeState))
	}
	return connect.NewResponse(resp), nil
}

// GenerateNodePairingObject issues a one-shot bootstrap token a remote node
// binary can consume to register itself with this controller. The plaintext
// token is returned only here; the DB stores a SHA-256 hash and marks it
// consumed after the node calls POST /api/node/bootstrap.
func (xs *XylonaService) GenerateNodePairingObject(
	_ context.Context,
	request *connect.Request[xylona.GenerateNodePairingObjectRequest],
) (*connect.Response[xylona.GenerateNodePairingObjectResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}

	token, _, errGenerate := xs.db.GenerateNodeJoinToken("", 0)
	if errGenerate != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to generate join token"))
	}

	return connect.NewResponse(&xylona.GenerateNodePairingObjectResponse{
		BaseUrl:      strings.TrimSpace(request.Msg.GetTargetUrl()),
		PairingToken: token,
	}), nil
}

// RemoveNode removes a configured node. This intentionally rejects any request
// referencing the controller's embedded self-node (the hub is not a removable
// peer in the new model).
func (xs *XylonaService) RemoveNode(_ context.Context, request *connect.Request[xylona.RemoveNodeRequest]) (*connect.Response[xylona.RemoveNodeResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}

	nodeID := request.Msg.GetNodeId()
	_, errGetNode := xs.db.GetNodeByID(nodeID)
	if errGetNode != nil {
		return nil, notFoundErr()
	}
	if nodeID == xs.selfNodeID() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("the embedded self node cannot be removed"))
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
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}
	nodeModel := helpers.NodeProtoToModel(request.Msg.GetNode())
	node, err := xs.db.UpdateNode(nodeModel, helpers.NodeModelToSetter(nodeModel))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&xylona.EditNodeResponse{Node: xs.nodeProtoWithRuntime(context.Background(), node)}), nil
}

func (xs *XylonaService) aggregatedServerStatus(ctx context.Context, gameServer *models.GameServer) xylona.Status {
	if gameServer == nil {
		return xylona.Status_UNKNOWN
	}

	snapCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	status, _, errSnapshot := xs.resolveGameServerRuntimeState(snapCtx, gameServer)
	if errSnapshot != nil {
		log.Debug().Err(errSnapshot).Str("game_server_id", gameServer.ID).
			Msg("ListAggregatedGameServers: snapshot unavailable; using offline status")
	}
	return status
}

func (xs *XylonaService) remoteSummaryFromGameServer(ctx context.Context, gameServer *models.GameServer) *xylona.RemoteServerSummary {
	if gameServer == nil {
		return &xylona.RemoteServerSummary{}
	}

	var lastSeenAt *time.Time
	if gameServer.R.Node != nil {
		nodeLastSeen, ok := gameServer.R.Node.LastSeenAt.Get()
		if ok {
			lastSeenAt = &nodeLastSeen
		}
	}

	protoStatus := xs.aggregatedServerStatus(ctx, gameServer)
	out := &xylona.RemoteServerSummary{
		Id:             gameServer.ID,
		SourceNodeId:   gameServer.NodeID,
		NodeId:         gameServer.NodeID,
		RemoteServerId: gameServer.ID,
		DisplayName:    gameServer.Name,
		Status:         protoStatus,
		GameId:         gameServer.GameID,
		IpAddress:      gameServer.IP,
		Port:           gameServer.Port,
		QueryPort:      gameServer.QueryPort,
		MaxPlayers:     gameServer.MaxPlayers,
		CurrentPlayers: gameServer.SetPlayers,
		MapName:        gameServer.Map,
		Version:        gameServer.Version,
		IsStale:        false,
	}
	if gameServer.R.Game != nil {
		out.GameName = gameServer.R.Game.Name
	}
	if gameServer.R.Node != nil {
		out.NodeName = gameServer.R.Node.Name
		out.NodeHost = gameServer.R.Node.ListenURL
	}
	if lastSeenAt != nil {
		out.LastRemoteUpdate = timestamppb.New(*lastSeenAt)
		out.LastSyncedAt = timestamppb.New(*lastSeenAt)
	}
	return out
}

// ListAggregatedGameServers returns the servers the caller can access across
// both the embedded node and any configured remote nodes.
func (xs *XylonaService) ListAggregatedGameServers(ctx context.Context, request *connect.Request[xylona.ListAggregatedGameServersRequest]) (*connect.Response[xylona.ListAggregatedGameServersResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, errUser
	}

	resp := &xylona.ListAggregatedGameServersResponse{}

	var localServers []*models.GameServer
	var errLocal error
	if user.SuperUser {
		localServers, errLocal = xs.db.GetAllGameServers()
	} else {
		localServers, errLocal = xs.db.GetGameServersAccessibleByUser(user.ID)
	}
	if errLocal != nil {
		log.Warn().Err(errLocal).Msg("ListAggregatedGameServers: local server lookup failed")
		return connect.NewResponse(resp), nil
	}

	bulkPerms := map[string][]string{}
	if !user.SuperUser {
		grantedServerIDs := []string{}
		for _, gs := range localServers {
			if gs.UserID != user.ID {
				grantedServerIDs = append(grantedServerIDs, gs.ID)
			}
		}
		if len(grantedServerIDs) > 0 {
			var errBulkPerms error
			bulkPerms, errBulkPerms = xs.db.GetUserPermissionIDsForServers(user.ID, grantedServerIDs)
			if errBulkPerms != nil {
				log.Error().Err(errBulkPerms).Msg("ListAggregatedGameServers: failed to get bulk permissions")
				bulkPerms = map[string][]string{}
			}
		}
	}

	selfNodeID := xs.selfNodeID()
	for _, gs := range localServers {
		if strings.TrimSpace(gs.NodeID) != "" && gs.NodeID != selfNodeID {
			resp.Servers = append(resp.Servers, &xylona.AggregatedGameServer{
				IsLocal:      false,
				RemoteServer: xs.remoteSummaryFromGameServer(ctx, gs),
			})
			continue
		}

		gsProto := helpers.GameServerModelToProto(gs, xs.versionState)
		gsProto.Status = xs.getLocalGameServerStatus(gs)
		if user.SuperUser || gs.UserID == user.ID {
			gsProto.EffectivePermissions = xs.allPermissionIDs
		} else {
			perms, ok := bulkPerms[gs.ID]
			if ok {
				gsProto.EffectivePermissions = perms
			}
		}
		if !user.SuperUser {
			redactGameServerForNonSuperuser(gsProto)
		}
		resp.Servers = append(resp.Servers, &xylona.AggregatedGameServer{
			IsLocal:     true,
			LocalServer: gsProto,
		})
	}

	return connect.NewResponse(resp), nil
}
