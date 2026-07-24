package rpc

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"

	"github.com/ClintonCollins/Xylona/internal/controller/protomap"
	"github.com/ClintonCollins/Xylona/internal/gamedefinitions"
	"github.com/ClintonCollins/Xylona/internal/launchenv"
	"github.com/ClintonCollins/Xylona/pkg/version"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
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
	gamedefinitions.ClearOfficialMetadata(gameModel)
	validationErrors := gamedefinitions.ValidateModel(gameModel)
	if len(validationErrors) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New(strings.Join(validationErrors, "; ")))
	}
	gameSetter := gamedefinitions.GameSetterForModel(gameModel)
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
	updatedGameModel.DefaultEnvVars = gameModel.DefaultEnvVars
	if gameModel.XylonaOfficial {
		gamedefinitions.MarkImportedOfficialEdit(updatedGameModel, gameModel)
	} else {
		gamedefinitions.ClearOfficialMetadata(updatedGameModel)
	}
	validationErrors := gamedefinitions.ValidateModel(updatedGameModel)
	if len(validationErrors) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New(strings.Join(validationErrors, "; ")))
	}
	gameSetter := gamedefinitions.GameSetterForModel(updatedGameModel)
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

// ImportGame validates or imports a browser-provided game definition JSON document.
func (xs *XylonaService) ImportGame(_ context.Context, request *connect.Request[xylona.ImportGameRequest]) (*connect.Response[xylona.ImportGameResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}

	parsed, parseValidationErrors := parseImportGameDefinition(request.Msg.GetGameDefinitionJson())
	if len(parseValidationErrors) > 0 {
		return connect.NewResponse(&xylona.ImportGameResponse{
			Success:          false,
			ValidationErrors: parseValidationErrors,
		}), nil
	}

	validationErrors := gamedefinitions.ValidateModel(parsed.Model)
	warnings := append([]string{}, parsed.Warnings...)
	mode := request.Msg.GetMode()
	existingGame, existingFound, errExisting := xs.lookupImportGameConflict(parsed.Model.ID)
	if errExisting != nil {
		return nil, errExisting
	}
	if existingFound && len(validationErrors) == 0 && mode != xylona.GameImportMode_GAME_IMPORT_MODE_IMPORT_COPY {
		defaultEnv, errDefaultEnv := launchenv.ParseStored(parsed.Model.DefaultEnvVars)
		if errDefaultEnv != nil {
			validationErrors = append(validationErrors, errDefaultEnv.Error())
		} else {
			defaultEnvValidationErrors, errValidate := xs.validateGameDefaultEnvironmentAgainstServers(
				parsed.Model.ID,
				defaultEnv,
			)
			if errValidate != nil {
				return nil, errValidate
			}
			if mode == xylona.GameImportMode_GAME_IMPORT_MODE_PREVIEW {
				for _, validationError := range defaultEnvValidationErrors {
					warnings = append(warnings, "Updating the existing game would fail: "+validationError)
				}
			} else {
				validationErrors = append(validationErrors, defaultEnvValidationErrors...)
			}
		}
	}
	affectedNames, errAffected := xs.affectedGameServerNames(parsed.Model.ID, existingFound)
	if errAffected != nil {
		return nil, errAffected
	}
	changes := []*xylona.GameImportChange{}
	if existingFound && len(validationErrors) == 0 {
		var errChanges error
		changes, errChanges = importGameChanges(existingGame, parsed.Model)
		if errChanges != nil {
			return nil, connect.NewError(connect.CodeInternal, errChanges)
		}
	}

	response := &xylona.ImportGameResponse{
		Game:                    protomap.GameModelToProto(parsed.Model),
		Success:                 len(validationErrors) == 0,
		IdConflict:              existingFound,
		UpdatesExisting:         existingFound && mode == xylona.GameImportMode_GAME_IMPORT_MODE_APPLY,
		AffectedGameServerCount: int64(len(affectedNames)),
		AffectedGameServerNames: affectedNames,
		Warnings:                warnings,
		ValidationErrors:        validationErrors,
		ImportedGameId:          parsed.Model.ID,
		Changes:                 changes,
	}
	if mode == xylona.GameImportMode_GAME_IMPORT_MODE_PREVIEW {
		return connect.NewResponse(response), nil
	}
	if len(validationErrors) > 0 {
		return connect.NewResponse(response), nil
	}

	switch mode {
	case xylona.GameImportMode_GAME_IMPORT_MODE_APPLY:
		importedGame, errApply := xs.applyImportedGame(parsed.Model, existingGame, existingFound)
		if errApply != nil {
			return nil, errApply
		}
		response.Game = protomap.GameModelToProto(importedGame)
		response.ImportedGameId = importedGame.ID
		response.Success = true
	case xylona.GameImportMode_GAME_IMPORT_MODE_IMPORT_COPY:
		importedGame, errCopy := xs.copyImportedGame(parsed.Model)
		if errCopy != nil {
			return nil, errCopy
		}
		response.Game = protomap.GameModelToProto(importedGame)
		response.IdConflict = false
		response.UpdatesExisting = false
		response.AffectedGameServerCount = 0
		response.AffectedGameServerNames = nil
		response.Changes = nil
		response.ImportedGameId = importedGame.ID
		response.Success = true
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("unsupported game import mode"))
	}

	return connect.NewResponse(response), nil
}

// ExportGame returns a browser-downloadable game definition JSON document.
func (xs *XylonaService) ExportGame(_ context.Context, request *connect.Request[xylona.ExportGameRequest]) (*connect.Response[xylona.ExportGameResponse], error) {
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

	definitionJSON, _, errExport := gamedefinitions.ExportModel(game, version.SoftwareVersion, time.Now())
	if errExport != nil {
		return nil, connect.NewError(connect.CodeInternal, errExport)
	}

	return connect.NewResponse(&xylona.ExportGameResponse{
		GameDefinitionJson: definitionJSON,
		FileName:           gamedefinitions.ExportFileName(game),
	}), nil
}

// ResetGameToOfficialDefinition discards local edits on an official game and
// re-applies the bundled definition, restamping sync metadata.
func (xs *XylonaService) ResetGameToOfficialDefinition(_ context.Context, request *connect.Request[xylona.ResetGameToOfficialDefinitionRequest]) (*connect.Response[xylona.ResetGameToOfficialDefinitionResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}

	updated, errReset := gamedefinitions.ResetGameToOfficialDefinition(xs.db, request.Msg.GetGameId())
	if errReset != nil {
		if errors.Is(errReset, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("game not found"))
		}
		if errors.Is(errReset, gamedefinitions.ErrGameNotOfficial) ||
			errors.Is(errReset, gamedefinitions.ErrNoBundledDefinition) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errReset)
		}
		return nil, connect.NewError(connect.CodeInternal, errReset)
	}

	return connect.NewResponse(&xylona.ResetGameToOfficialDefinitionResponse{
		Game: protomap.GameModelToProto(updated),
	}), nil
}

func (xs *XylonaService) lookupImportGameConflict(gameID string) (*models.Game, bool, error) {
	existingGame, errExisting := xs.db.GetGameByID(gameID)
	if errExisting != nil {
		if errors.Is(errExisting, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, dbLookup(errExisting)
	}
	return existingGame, true, nil
}

func parseImportGameDefinition(definitionJSON string) (*gamedefinitions.ParsedDefinition, []string) {
	parsed, errParse := gamedefinitions.Parse([]byte(definitionJSON))
	if errParse != nil {
		return nil, []string{errParse.Error()}
	}
	return parsed, nil
}

func (xs *XylonaService) affectedGameServerNames(gameID string, existingFound bool) ([]string, error) {
	if !existingFound {
		return []string{}, nil
	}
	gameServers, errServers := xs.db.GetGameServersByGameID(gameID)
	if errServers != nil {
		return nil, connect.NewError(connect.CodeInternal, errServers)
	}
	names := make([]string, 0, len(gameServers))
	for _, gameServer := range gameServers {
		names = append(names, gameServer.Name)
	}
	return names, nil
}

func (xs *XylonaService) applyImportedGame(imported *models.Game, existing *models.Game, existingFound bool) (*models.Game, error) {
	if existingFound {
		imported.ID = existing.ID
		if existing.XylonaOfficial {
			gamedefinitions.MarkImportedOfficialEdit(imported, existing)
		} else {
			gamedefinitions.ClearOfficialMetadata(imported)
		}
		updated, errUpdate := xs.db.UpdateGame(xs.db.DB, existing, gamedefinitions.GameSetterForModel(imported))
		if errUpdate != nil {
			return nil, connect.NewError(connect.CodeInternal, errUpdate)
		}
		return updated, nil
	}

	gamedefinitions.ClearOfficialMetadata(imported)
	inserted, errInsert := xs.db.InsertGame(xs.db.DB, gamedefinitions.GameSetterForModel(imported))
	if errInsert != nil {
		return nil, connect.NewError(connect.CodeInternal, errInsert)
	}
	return inserted, nil
}

func (xs *XylonaService) copyImportedGame(imported *models.Game) (*models.Game, error) {
	newID, errID := gamedefinitions.CopyID(xs.db, imported.ID)
	if errID != nil {
		return nil, connect.NewError(connect.CodeInternal, errID)
	}
	imported.ID = newID
	gamedefinitions.ClearOfficialMetadata(imported)
	inserted, errInsert := xs.db.InsertGame(xs.db.DB, gamedefinitions.GameSetterForModel(imported))
	if errInsert != nil {
		return nil, connect.NewError(connect.CodeInternal, errInsert)
	}
	return inserted, nil
}
