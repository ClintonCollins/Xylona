package rpc

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/xycrypt"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	nodeModel := helpers.NodeProtoToModel(request.Msg.GetNode())
	node, err := xs.db.InsertNode(helpers.NodeModelToSetter(nodeModel))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&xylona.AddNodeResponse{Node: helpers.NodeModelToProto(node)}), nil
}

func (xs XylonaService) RemoveNode(ctx context.Context, request *connect.Request[xylona.RemoveNodeRequest]) (*connect.Response[xylona.RemoveNodeResponse], error) {
	nodeID := request.Msg.GetNodeId()
	err := xs.db.DeleteNodeByID(nodeID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
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
	secretKeyHash, err := xycrypt.GenerateHashFromString(key, xycrypt.DefaultHashParameters)
	if err != nil {
		log.Error().Err(err).Msg("Unable to generate hash from string.")
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	newKeySetter := &models.LocalSecretKeySetter{
		Name:             omit.From(request.Msg.GetName()),
		SecretKeyHash:    omit.From(string(secretKeyHash)),
		LastAccessedFrom: omitnull.From(""),
		LastUsedAt:       omitnull.From(time.Now()),
		CreatedAt:        omit.From(time.Now()),
	}
	LocalSecretKey, err := xs.db.InsertSecretKey(newKeySetter)
	if err != nil {
		log.Error().Err(err).Msg("Unable to insert secret key.")
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := &xylona.CreateLocalSecretKeyResponse{
		Id:        LocalSecretKey.ID,
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
