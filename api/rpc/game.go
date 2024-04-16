package rpc

import (
	"context"
	"database/sql"
	"errors"

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
