package actions

import (
	"fmt"

	"github.com/ClintonCollins/Xylona/internal/controller/launchenv"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (inst *Instance) reloadGameServerForStart(gameServer *models.GameServer) (*models.GameServer, error) {
	if inst.db == nil {
		return gameServer, nil
	}
	if gameServer == nil || gameServer.ID == "" {
		return gameServer, nil
	}

	reloaded, errReload := inst.db.GetGameServerByID(gameServer.ID)
	if errReload != nil {
		return nil, fmt.Errorf("reload game server for start: %w", errReload)
	}
	return reloaded, nil
}

func (inst *Instance) loadStartLaunchEnvMetadata(gameServer *models.GameServer) ([]launchenv.Variable, []launchenv.SecretState, error) {
	serverEnv, errParse := launchenv.ParseStored(gameServer.EnvVars)
	if errParse != nil {
		return nil, nil, fmt.Errorf("parse server launch environment: %w", errParse)
	}

	effectiveEnv, issues := launchenv.MergeNormal(nil, serverEnv)
	if len(issues) > 0 {
		errValidation := launchenv.NewValidationError(issues)
		return nil, nil, fmt.Errorf("validate normal launch environment: %w", errValidation)
	}
	if inst.db == nil {
		return effectiveEnv, nil, nil
	}

	secretStates, errSecrets := inst.listStartLaunchSecretStates(gameServer.ID)
	if errSecrets != nil {
		return nil, nil, errSecrets
	}
	issues = launchenv.ValidateSecretStates(secretStates, effectiveEnv)
	if len(issues) > 0 {
		errValidation := launchenv.NewValidationError(issues)
		return nil, nil, fmt.Errorf("validate secret launch environment: %w", errValidation)
	}
	return effectiveEnv, secretStates, nil
}

func (inst *Instance) listStartLaunchSecretStates(gameServerID string) ([]launchenv.SecretState, error) {
	dbStates, errStates := inst.db.ListGameServerSecretEnvStates(gameServerID)
	if errStates != nil {
		return nil, fmt.Errorf("list launch secret environment: %w", errStates)
	}

	states := make([]launchenv.SecretState, len(dbStates))
	for i, state := range dbStates {
		states[i] = launchenv.SecretState{
			Name:       state.Name,
			Configured: state.Configured,
			UpdatedAt:  state.UpdatedAt,
		}
	}
	return states, nil
}

func startLaunchEnvRequired(normalEnv []launchenv.Variable, secretStates []launchenv.SecretState) bool {
	return len(normalEnv) > 0 || len(secretStates) > 0
}

func (inst *Instance) decryptStartLaunchEnv(gameServer *models.GameServer, normalEnv []launchenv.Variable) (map[string]string, error) {
	secretEnv := map[string]string{}
	if inst.db != nil {
		var errSecrets error
		secretEnv, errSecrets = inst.db.DecryptGameServerSecretEnv(gameServer.ID)
		if errSecrets != nil {
			return nil, fmt.Errorf("decrypt launch secret environment: %w", errSecrets)
		}
	}

	launchEnv, issues := launchenv.BuildLaunchEnv(normalEnv, secretEnv)
	if len(issues) > 0 {
		errValidation := launchenv.NewValidationError(issues)
		return nil, fmt.Errorf("build launch environment: %w", errValidation)
	}
	return launchEnv, nil
}
