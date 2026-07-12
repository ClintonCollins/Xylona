package rpc

import (
	"fmt"

	"github.com/ClintonCollins/Xylona/internal/controller/launchenv"
	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func prepareCreateGameServerEnvironment(
	game *models.Game,
	protoVariables []*xylona.EnvironmentVariable,
) (string, error) {
	serverVariables := environmentVariablesFromProto(protoVariables)
	encoded, errMarshal := launchenv.MarshalStored(serverVariables)
	if errMarshal != nil {
		return "", fmt.Errorf("validate server environment: %w", errMarshal)
	}

	defaultVariables, errDefaults := launchenv.ParseStored(game.DefaultEnvVars)
	if errDefaults != nil {
		return "", fmt.Errorf("parse game default environment: %w", errDefaults)
	}
	effectiveVariables, issues := launchenv.MergeNormal(defaultVariables, serverVariables)
	if len(issues) > 0 {
		return "", fmt.Errorf("validate effective environment: %w", launchenv.NewValidationError(issues))
	}
	effectiveEnvironment, issues := launchenv.BuildLaunchEnv(effectiveVariables, nil)
	if len(issues) > 0 {
		return "", fmt.Errorf("build effective environment: %w", launchenv.NewValidationError(issues))
	}

	if game.ID == gameintegrations.StarboundGameID {
		_, errUsername := gameintegrations.StarboundSteamUsername(effectiveEnvironment)
		if errUsername != nil {
			return "", fmt.Errorf("validate Starbound Steam account: %w", errUsername)
		}
	}
	return encoded, nil
}
