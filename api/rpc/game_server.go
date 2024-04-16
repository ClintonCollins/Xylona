package rpc

import (
	"context"
	"database/sql"
	"errors"

	connect_go "github.com/bufbuild/connect-go"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (xs XylonaService) StreamGameServerOutput(ctx context.Context, request *connect_go.Request[xylona.ReadGameServerOutputRequest], stream *connect_go.ServerStream[xylona.ReadGameServerOutputResponse]) error {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}

	gameServerCommand, err := xs.supervisorInst.GetCommandByID(gameServer.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get game server command")
		return nil
	}

	streamID := uuid.NewString()
	outputChannel := make(chan string)
	gameServerCommand.AddOutputListener(streamID, outputChannel)
	defer func() {
		gameServerCommand.RemoveOutputListener(streamID)
		close(outputChannel)
	}()
	for {
		select {
		case <-xs.ctx.Done():
			return nil
		case <-ctx.Done():
			return nil
		case output := <-outputChannel:
			response := &xylona.ReadGameServerOutputResponse{
				Output: output,
			}
			errSend := stream.Send(response)
			if errSend != nil {
				log.Error().Err(errSend).Msg("Failed to send game server output")
				return errSend
			}
		}
	}
}

func (xs XylonaService) CreateGameServer(ctx context.Context, request *connect_go.Request[xylona.CreateGameServerRequest]) (*connect_go.Response[xylona.CreateGameServerResponse], error) {

	// log.Debug().Msgf("CreateGameServer request: %+v", request.Msg.GetGameServer())
	user, errGetUser := xs.db.GetUserByID(request.Msg.GetGameServer().UserId)
	if errGetUser != nil {
		if errors.Is(errGetUser, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	game, errGetGame := xs.db.GetGameByID(request.Msg.GetGameServer().GameId)
	if errGetGame != nil {
		if errors.Is(errGetGame, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}

	newGameServerModel := helpers.GameServerProtoToModel(request.Msg.GetGameServer())

	newGameServer, errInstallGameServer := xs.actionsInst.InstallGameServer(game, newGameServerModel, user)
	if errInstallGameServer != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}

	response := &xylona.CreateGameServerResponse{
		GameServer: helpers.GameServerModelToProto(newGameServer),
	}
	return connect_go.NewResponse(response), nil
}

func (xs XylonaService) EditGameServer(ctx context.Context, request *connect_go.Request[xylona.EditGameServerRequest]) (*connect_go.Response[xylona.EditGameServerResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) RemoveGameServer(ctx context.Context, request *connect_go.Request[xylona.RemoveGameServerRequest]) (*connect_go.Response[xylona.RemoveGameServerResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) StartGameServer(ctx context.Context, request *connect_go.Request[xylona.StartGameServerRequest]) (*connect_go.Response[xylona.StartGameServerResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	xs.actionsInst.StartGameServer(gameServer)
	response := &xylona.StartGameServerResponse{}
	return connect_go.NewResponse(response), nil
}

func (xs XylonaService) StopGameServer(ctx context.Context, request *connect_go.Request[xylona.StopGameServerRequest]) (*connect_go.Response[xylona.StopGameServerResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	xs.actionsInst.StopGameServer(gameServer)
	response := &xylona.StopGameServerResponse{}
	return connect_go.NewResponse(response), nil
}

func (xs XylonaService) ReadGameServerOutput(ctx context.Context, request *connect_go.Request[xylona.ReadGameServerOutputRequest]) (*connect_go.Response[xylona.ReadGameServerOutputResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	output := xs.actionsInst.ReadGameServerBuffer(gameServer)
	response := &xylona.ReadGameServerOutputResponse{
		Output: output,
	}
	return connect_go.NewResponse(response), nil
}

func (xs XylonaService) SendGameServerInput(ctx context.Context, request *connect_go.Request[xylona.SendGameServerInputRequest]) (*connect_go.Response[xylona.SendGameServerInputResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) GetGameServer(ctx context.Context, request *connect_go.Request[xylona.GetGameServerRequest]) (*connect_go.Response[xylona.GetGameServerResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	response := &xylona.GetGameServerResponse{
		GameServer: helpers.GameServerModelToProto(gameServer),
	}
	return connect_go.NewResponse(response), nil
}

func (xs XylonaService) UpdateGameServer(ctx context.Context, request *connect_go.Request[xylona.UpdateGameServerRequest]) (*connect_go.Response[xylona.UpdateGameServerResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) ListGameServers(ctx context.Context, request *connect_go.Request[xylona.ListGameServersRequest]) (*connect_go.Response[xylona.ListGameServersResponse], error) {
	user, err := xs.getUserFromHeader(request.Header())
	if err != nil {
		return nil, err
	}
	if !user.SuperUser {
		gameServers, errGetGameServers := xs.db.GetGameServersByUser(user.ID)
		if errGetGameServers != nil {
			if errors.Is(errGetGameServers, sql.ErrNoRows) {
				return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
			}
			return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
		}
		gameServersProto := make([]*xylona.GameServer, len(gameServers))
		for i, gameServer := range gameServers {
			gameServerProto := helpers.GameServerModelToProto(gameServer)
			gameServersProto[i] = gameServerProto
		}
		response := &xylona.ListGameServersResponse{
			GameServers: gameServersProto,
		}
		return connect_go.NewResponse(response), nil
	}
	gameServers, errGetGameServers := xs.db.GetAllGameServers()
	if errGetGameServers != nil {
		if errors.Is(errGetGameServers, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}

	gameServersProto := make([]*xylona.GameServer, len(gameServers))
	for i, gameServer := range gameServers {
		gameServerProto := helpers.GameServerModelToProto(gameServer)
		gameServersProto[i] = gameServerProto
	}
	response := &xylona.ListGameServersResponse{
		GameServers: gameServersProto,
	}
	return connect_go.NewResponse(response), nil
}
