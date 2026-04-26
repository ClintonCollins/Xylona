package rpc

import (
	"context"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/internal/controller/launchenv"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetGameEnvironment returns visible default env vars for one game definition.
func (xs *XylonaService) GetGameEnvironment(
	_ context.Context,
	request *connect.Request[xylona.GetGameEnvironmentRequest],
) (*connect.Response[xylona.GetGameEnvironmentResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}

	game, errGame := xs.db.GetGameByID(request.Msg.GetGameId())
	if errGame != nil {
		return nil, dbLookup(errGame)
	}

	defaultEnv, errDefaultEnv := parseStoredGameEnvironment(game)
	if errDefaultEnv != nil {
		return nil, errDefaultEnv
	}
	issues := launchenv.ValidateVariables(defaultEnv)

	return connect.NewResponse(&xylona.GetGameEnvironmentResponse{
		DefaultEnv:       environmentVariablesToProto(defaultEnv),
		ValidationIssues: validationIssuesToProto(issues),
	}), nil
}

// UpdateGameEnvironment replaces visible default env vars for one game definition.
func (xs *XylonaService) UpdateGameEnvironment(
	_ context.Context,
	request *connect.Request[xylona.UpdateGameEnvironmentRequest],
) (*connect.Response[xylona.UpdateGameEnvironmentResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}

	game, errGame := xs.db.GetGameByID(request.Msg.GetGameId())
	if errGame != nil {
		return nil, dbLookup(errGame)
	}

	defaultEnv := environmentVariablesFromProto(request.Msg.GetDefaultEnv())
	encoded, errMarshal := launchenv.MarshalStored(defaultEnv)
	if errMarshal != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errMarshal)
	}

	updatedGame := *game
	updatedGame.DefaultEnvVars = encoded
	if updatedGame.XylonaOfficial {
		updatedGame.OfficialDefinitionDiverged = true
	}

	updated, errUpdate := xs.db.UpdateGame(xs.db.DB, game, &models.GameSetter{
		ID:                         omit.From(game.ID),
		DefaultEnvVars:             omit.From(encoded),
		OfficialDefinitionDiverged: omit.From(updatedGame.OfficialDefinitionDiverged),
	})
	if errUpdate != nil {
		return nil, connect.NewError(connect.CodeInternal, errUpdate)
	}

	updatedDefaultEnv, errDefaultEnv := parseStoredGameEnvironment(updated)
	if errDefaultEnv != nil {
		return nil, errDefaultEnv
	}

	return connect.NewResponse(&xylona.UpdateGameEnvironmentResponse{
		DefaultEnv:       environmentVariablesToProto(updatedDefaultEnv),
		ValidationIssues: []*xylona.EnvironmentValidationIssue{},
	}), nil
}

func parseStoredGameEnvironment(game *models.Game) ([]launchenv.Variable, error) {
	defaultEnv, errParse := launchenv.ParseStored(game.DefaultEnvVars)
	if errParse != nil {
		return nil, internalErrf("stored game default environment variables are invalid")
	}
	return defaultEnv, nil
}
