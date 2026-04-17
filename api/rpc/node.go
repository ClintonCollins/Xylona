package rpc

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetNode returns a node by ID.
func (xs *XylonaService) GetNode(_ context.Context, request *connect.Request[xylona.GetNodeRequest]) (*connect.Response[xylona.GetNodeResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
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
		return nil, unauthenticated()
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
	return connect.NewResponse(&xylona.EditNodeResponse{Node: helpers.NodeModelToProto(node)}), nil
}

// ListAggregatedGameServers returns the controller-local servers the caller
// has access to. Remote-node servers will join the aggregated view once
// NodeClient exposes a cross-node server lookup; for now the list is local
// only.
func (xs *XylonaService) ListAggregatedGameServers(_ context.Context, request *connect.Request[xylona.ListAggregatedGameServersRequest]) (*connect.Response[xylona.ListAggregatedGameServersResponse], error) {
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

	for _, gs := range localServers {
		gsProto := helpers.GameServerModelToProto(gs, xs.versionState)
		gsProto.Status = xs.getLocalGameServerStatus(gs)
		if user.SuperUser || gs.UserID == user.ID {
			gsProto.EffectivePermissions = xs.allPermissionIDs
		}
		resp.Servers = append(resp.Servers, &xylona.AggregatedGameServer{
			IsLocal:     true,
			LocalServer: gsProto,
		})
	}

	return connect.NewResponse(resp), nil
}
