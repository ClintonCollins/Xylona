package rpc

import (
	"context"
	"database/sql"
	"errors"

	"github.com/aarondl/opt/omit"
	connect_go "github.com/bufbuild/connect-go"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (xs XylonaService) GetGame(ctx context.Context, request *connect_go.Request[xylona.GetGameRequest]) (*connect_go.Response[xylona.GetGameResponse], error) {
	game, errGetGame := xs.db.GetGameByID(request.Msg.GetId())
	if errGetGame != nil {
		if errors.Is(errGetGame, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("game not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	resp := &connect_go.Response[xylona.GetGameResponse]{
		Msg: &xylona.GetGameResponse{
			Game: helpers.GameModelToProto(game),
		},
	}
	return resp, nil
}

func (xs XylonaService) ListGames(ctx context.Context, request *connect_go.Request[xylona.ListGamesRequest]) (*connect_go.Response[xylona.ListGamesResponse], error) {
	games, errGetGames := xs.db.GetGames()
	if errGetGames != nil {
		if errors.Is(errGetGames, sql.ErrNoRows) {
			return &connect_go.Response[xylona.ListGamesResponse]{
				Msg: &xylona.ListGamesResponse{
					Games: []*xylona.Game{},
				},
			}, nil
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	gamesProto := make([]*xylona.Game, len(games))
	for i, game := range games {
		gameProto := helpers.GameModelToProto(game)
		gamesProto[i] = gameProto
	}
	resp := &connect_go.Response[xylona.ListGamesResponse]{
		Msg: &xylona.ListGamesResponse{
			Games: gamesProto,
		},
	}
	return resp, nil
}

func (xs XylonaService) AddGame(_ context.Context, request *connect_go.Request[xylona.AddGameRequest]) (*connect_go.Response[xylona.AddGameResponse], error) {
	gameProto := request.Msg.Game
	gameModel := helpers.GameProtoToModel(gameProto)
	gameSetter := helpers.GameModelToGameSetter(gameModel)
	game, errInsertGame := xs.db.InsertGame(xs.db.DB, gameSetter)
	if errInsertGame != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errInsertGame)
	}
	resp := &connect_go.Response[xylona.AddGameResponse]{
		Msg: &xylona.AddGameResponse{
			Game: helpers.GameModelToProto(game),
		},
	}
	return resp, nil
}

func (xs XylonaService) EditGame(_ context.Context, request *connect_go.Request[xylona.EditGameRequest]) (*connect_go.Response[xylona.EditGameResponse], error) {
	gameProto := request.Msg.Game
	gameModel, errGetGameModel := xs.db.GetGameByID(gameProto.GetId())
	if errGetGameModel != nil {
		if errors.Is(errGetGameModel, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("game not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))

	}
	updatedGameModel := helpers.GameProtoToModel(gameProto)
	gameSetter := helpers.GameModelToGameSetter(updatedGameModel)
	gameSetter.ID = omit.From(gameModel.ID)

	game, errUpdateGame := xs.db.UpdateGame(xs.db.DB, gameModel, gameSetter)
	if errUpdateGame != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errUpdateGame)
	}
	resp := &connect_go.Response[xylona.EditGameResponse]{
		Msg: &xylona.EditGameResponse{
			Game: helpers.GameModelToProto(game),
		},
	}
	return resp, nil
}

func (xs XylonaService) RemoveGame(ctx context.Context, request *connect_go.Request[xylona.RemoveGameRequest]) (*connect_go.Response[xylona.RemoveGameResponse], error) {
	game, errGetGame := xs.db.GetGameByID(request.Msg.GetGameId())
	if errGetGame != nil {
		if errors.Is(errGetGame, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("game not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	// Check if any servers use the game.
	gameServers, errGetGameServers := xs.db.GetGameServersByGameID(game.ID)
	if errGetGameServers != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errGetGameServers)
	}
	if len(gameServers) > 0 {
		return nil, connect_go.NewError(connect_go.CodeFailedPrecondition, errors.New("game is currently used by game servers"))
	}
	errDeleteGame := xs.db.DeleteGameByID(game.ID)
	if errDeleteGame != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errDeleteGame)
	}
	return &connect_go.Response[xylona.RemoveGameResponse]{Msg: &xylona.RemoveGameResponse{}}, nil
}

func (xs XylonaService) ImportGame(ctx context.Context, request *connect_go.Request[xylona.ImportGameRequest]) (*connect_go.Response[xylona.ImportGameResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) ExportGame(ctx context.Context, request *connect_go.Request[xylona.ExportGameRequest]) (*connect_go.Response[xylona.ExportGameResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) GetBranches(ctx context.Context, c *connect_go.Request[xylona.GetBranchesRequest]) (*connect_go.Response[xylona.GetBranchesResponse], error) {
	//TODO implement me
	panic("implement me")
}
