package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/controller/readiness"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// GetGameServerReadiness returns public setup state for a game server.
func (xs *XylonaService) GetGameServerReadiness(
	ctx context.Context,
	request *connect.Request[xylona.GetGameServerReadinessRequest],
) (*connect.Response[xylona.GetGameServerReadinessResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.view")
	if errPermission != nil {
		return nil, errPermission
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}

	items, errItems := readiness.List(ctx, xs.db, gameServer, client)
	if errItems != nil {
		return nil, connect.NewError(connect.CodeInternal, errItems)
	}
	return connect.NewResponse(&xylona.GetGameServerReadinessResponse{
		Items: readinessItemsToProto(items),
	}), nil
}

// AcceptMinecraftEula accepts the Minecraft EULA for a game server.
func (xs *XylonaService) AcceptMinecraftEula(
	ctx context.Context,
	request *connect.Request[xylona.AcceptMinecraftEulaRequest],
) (*connect.Response[xylona.AcceptMinecraftEulaResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}
	if gameServer.GameID != "minecraft" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("game server is not a minecraft server"))
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}
	errAccept := readiness.AcceptMinecraftEULA(ctx, xs.db, gameServer, client, user.ID)
	if errAccept != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errAccept)
	}

	items, errItems := readiness.List(ctx, xs.db, gameServer, client)
	if errItems != nil {
		return nil, connect.NewError(connect.CodeInternal, errItems)
	}
	return connect.NewResponse(&xylona.AcceptMinecraftEulaResponse{
		Items: readinessItemsToProto(items),
	}), nil
}

func readinessItemsToProto(items []readiness.Item) []*xylona.GameServerReadinessItem {
	protoItems := make([]*xylona.GameServerReadinessItem, len(items))
	for i, item := range items {
		protoItems[i] = &xylona.GameServerReadinessItem{
			Kind:           item.Kind,
			Required:       item.Required,
			Complete:       item.Complete,
			Blocking:       item.Blocking,
			Message:        item.Message,
			PublicDataJson: item.PublicData,
		}
	}
	return protoItems
}
