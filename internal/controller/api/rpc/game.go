package rpc

import (
	"context"
	"database/sql"
	"errors"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"

	"github.com/ClintonCollins/Xylona/internal/controller/protomap"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// GetGame returns a single game definition by ID.
func (xs *XylonaService) GetGame(_ context.Context, request *connect.Request[xylona.GetGameRequest]) (*connect.Response[xylona.GetGameResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	game, errGetGame := xs.db.GetGameByID(request.Msg.GetId())
	if errGetGame != nil {
		return nil, dbLookup(errGetGame)
	}
	gameProto := protomap.GameModelToProto(game)
	if !user.SuperUser {
		redactGameForNonSuperuser(gameProto)
	}

	resp := &connect.Response[xylona.GetGameResponse]{
		Msg: &xylona.GetGameResponse{
			Game: gameProto,
		},
	}
	return resp, nil
}

// ListGames returns all game definitions visible to the caller.
func (xs *XylonaService) ListGames(_ context.Context, request *connect.Request[xylona.ListGamesRequest]) (*connect.Response[xylona.ListGamesResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
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
		return nil, internalErr()
	}
	gamesProto := make([]*xylona.Game, len(games))
	for i, game := range games {
		gameProto := protomap.GameModelToProto(game)
		if !user.SuperUser {
			redactGameForNonSuperuser(gameProto)
		}
		gamesProto[i] = gameProto
	}
	resp := &connect.Response[xylona.ListGamesResponse]{
		Msg: &xylona.ListGamesResponse{
			Games: gamesProto,
		},
	}
	return resp, nil
}

// AddGame creates a new game definition.
func (xs *XylonaService) AddGame(_ context.Context, request *connect.Request[xylona.AddGameRequest]) (*connect.Response[xylona.AddGameResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}
	gameProto := request.Msg.GetGame()
	if gameProto.GetId() == "" {
		gameProto.Id = uuid.NewString()
	}
	gameModel := protomap.GameProtoToModel(gameProto)
	errValidateStartArgs := validateStructuredStartArgsGameConfig(gameModel)
	if errValidateStartArgs != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errValidateStartArgs)
	}
	gameSetter := protomap.GameModelToGameSetter(gameModel)
	game, errInsertGame := xs.db.InsertGame(xs.db.DB, gameSetter)
	if errInsertGame != nil {
		return nil, connect.NewError(connect.CodeInternal, errInsertGame)
	}
	resp := &connect.Response[xylona.AddGameResponse]{
		Msg: &xylona.AddGameResponse{
			Game: protomap.GameModelToProto(game),
		},
	}
	return resp, nil
}

// EditGame updates an existing game definition.
func (xs *XylonaService) EditGame(_ context.Context, request *connect.Request[xylona.EditGameRequest]) (*connect.Response[xylona.EditGameResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}
	gameProto := request.Msg.GetGame()
	gameModel, errGetGameModel := xs.db.GetGameByID(gameProto.GetId())
	if errGetGameModel != nil {
		return nil, dbLookup(errGetGameModel)
	}
	updatedGameModel := protomap.GameProtoToModel(gameProto)
	errValidateStartArgs := validateStructuredStartArgsGameConfig(updatedGameModel)
	if errValidateStartArgs != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errValidateStartArgs)
	}
	gameSetter := protomap.GameModelToGameSetter(updatedGameModel)
	gameSetter.ID = omit.From(gameModel.ID)

	game, errUpdateGame := xs.db.UpdateGame(xs.db.DB, gameModel, gameSetter)
	if errUpdateGame != nil {
		return nil, connect.NewError(connect.CodeInternal, errUpdateGame)
	}
	resp := &connect.Response[xylona.EditGameResponse]{
		Msg: &xylona.EditGameResponse{
			Game: protomap.GameModelToProto(game),
		},
	}
	return resp, nil
}

// RemoveGame deletes a game definition when no servers still use it.
func (xs *XylonaService) RemoveGame(_ context.Context, request *connect.Request[xylona.RemoveGameRequest]) (*connect.Response[xylona.RemoveGameResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}
	game, errGetGame := xs.db.GetGameByID(request.Msg.GetGameId())
	if errGetGame != nil {
		return nil, dbLookup(errGetGame)
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

// ImportGame is reserved for future game import support.
func (xs *XylonaService) ImportGame(_ context.Context, _ *connect.Request[xylona.ImportGameRequest]) (*connect.Response[xylona.ImportGameResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
}

// ExportGame is reserved for future game export support.
func (xs *XylonaService) ExportGame(_ context.Context, _ *connect.Request[xylona.ExportGameRequest]) (*connect.Response[xylona.ExportGameResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
}
