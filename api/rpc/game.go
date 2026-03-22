package rpc

import (
	"context"
	"database/sql"
	"errors"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (xs *XylonaService) GetGame(ctx context.Context, request *connect.Request[xylona.GetGameRequest]) (*connect.Response[xylona.GetGameResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	game, errGetGame := xs.db.GetGameByID(request.Msg.GetId())
	if errGetGame != nil {
		if errors.Is(errGetGame, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("game not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	resp := &connect.Response[xylona.GetGameResponse]{
		Msg: &xylona.GetGameResponse{
			Game: helpers.GameModelToProto(game),
		},
	}
	return resp, nil
}

func (xs *XylonaService) ListGames(ctx context.Context, request *connect.Request[xylona.ListGamesRequest]) (*connect.Response[xylona.ListGamesResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	games, errGetGames := xs.db.GetGames()
	if errGetGames != nil {
		if errors.Is(errGetGames, sql.ErrNoRows) {
			return &connect.Response[xylona.ListGamesResponse]{
				Msg: &xylona.ListGamesResponse{
					Games: []*xylona.Game{},
				},
			}, nil
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	gamesProto := make([]*xylona.Game, len(games))
	for i, game := range games {
		gameProto := helpers.GameModelToProto(game)
		gamesProto[i] = gameProto
	}
	resp := &connect.Response[xylona.ListGamesResponse]{
		Msg: &xylona.ListGamesResponse{
			Games: gamesProto,
		},
	}
	return resp, nil
}

func (xs *XylonaService) AddGame(_ context.Context, request *connect.Request[xylona.AddGameRequest]) (*connect.Response[xylona.AddGameResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}
	gameProto := request.Msg.Game
	if gameProto.Id == "" {
		gameProto.Id = uuid.NewString()
	}
	gameModel := helpers.GameProtoToModel(gameProto)
	gameSetter := helpers.GameModelToGameSetter(gameModel)
	game, errInsertGame := xs.db.InsertGame(xs.db.DB, gameSetter)
	if errInsertGame != nil {
		return nil, connect.NewError(connect.CodeInternal, errInsertGame)
	}
	resp := &connect.Response[xylona.AddGameResponse]{
		Msg: &xylona.AddGameResponse{
			Game: helpers.GameModelToProto(game),
		},
	}
	return resp, nil
}

func (xs *XylonaService) EditGame(_ context.Context, request *connect.Request[xylona.EditGameRequest]) (*connect.Response[xylona.EditGameResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}
	gameProto := request.Msg.Game
	gameModel, errGetGameModel := xs.db.GetGameByID(gameProto.GetId())
	if errGetGameModel != nil {
		if errors.Is(errGetGameModel, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("game not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))

	}
	updatedGameModel := helpers.GameProtoToModel(gameProto)
	gameSetter := helpers.GameModelToGameSetter(updatedGameModel)
	gameSetter.ID = omit.From(gameModel.ID)

	game, errUpdateGame := xs.db.UpdateGame(xs.db.DB, gameModel, gameSetter)
	if errUpdateGame != nil {
		return nil, connect.NewError(connect.CodeInternal, errUpdateGame)
	}
	resp := &connect.Response[xylona.EditGameResponse]{
		Msg: &xylona.EditGameResponse{
			Game: helpers.GameModelToProto(game),
		},
	}
	return resp, nil
}

func (xs *XylonaService) RemoveGame(ctx context.Context, request *connect.Request[xylona.RemoveGameRequest]) (*connect.Response[xylona.RemoveGameResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if !user.SuperUser {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("superuser required"))
	}
	game, errGetGame := xs.db.GetGameByID(request.Msg.GetGameId())
	if errGetGame != nil {
		if errors.Is(errGetGame, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("game not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	// Check if any servers use the game.
	gameServers, errGetGameServers := xs.db.GetGameServersByGameID(game.ID)
	if errGetGameServers != nil {
		return nil, connect.NewError(connect.CodeInternal, errGetGameServers)
	}
	if len(gameServers) > 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("game is currently used by game servers"))
	}
	errDeleteGame := xs.db.DeleteGameByID(game.ID)
	if errDeleteGame != nil {
		return nil, connect.NewError(connect.CodeInternal, errDeleteGame)
	}
	return &connect.Response[xylona.RemoveGameResponse]{Msg: &xylona.RemoveGameResponse{}}, nil
}

func (xs *XylonaService) ImportGame(_ context.Context, _ *connect.Request[xylona.ImportGameRequest]) (*connect.Response[xylona.ImportGameResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
}

func (xs *XylonaService) ExportGame(_ context.Context, _ *connect.Request[xylona.ExportGameRequest]) (*connect.Response[xylona.ExportGameResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
}

func (xs *XylonaService) GetBranches(_ context.Context, _ *connect.Request[xylona.GetBranchesRequest]) (*connect.Response[xylona.GetBranchesResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
}
