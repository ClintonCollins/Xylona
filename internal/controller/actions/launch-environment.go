package actions

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ClintonCollins/Xylona/internal/controller/readiness"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/launchenv"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const steamGSLTPlaceholder = "STEAM_GSLT"

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
	effectiveEnv, errNormalEnv := mergeNormalLaunchEnvironment(gameServer.R.Game, gameServer.EnvVars)
	if errNormalEnv != nil {
		return nil, nil, errNormalEnv
	}
	if inst.db == nil {
		return effectiveEnv, nil, nil
	}

	secretStates, errSecrets := inst.listStartLaunchSecretStates(gameServer.ID)
	if errSecrets != nil {
		return nil, nil, errSecrets
	}
	issues := launchenv.ValidateSecretStates(secretStates, effectiveEnv)
	if len(issues) > 0 {
		errValidation := launchenv.NewValidationError(issues)
		return nil, nil, fmt.Errorf("validate secret launch environment: %w", errValidation)
	}
	return effectiveEnv, secretStates, nil
}

func mergeNormalLaunchEnvironment(game *models.Game, storedServerEnvironment string) ([]launchenv.Variable, error) {
	gameDefaultEnv := []launchenv.Variable{}
	if game != nil {
		var errDefaultEnv error
		gameDefaultEnv, errDefaultEnv = launchenv.ParseStored(game.DefaultEnvVars)
		if errDefaultEnv != nil {
			return nil, fmt.Errorf("parse game default launch environment: %w", errDefaultEnv)
		}
	}

	serverEnv, errParse := launchenv.ParseStored(storedServerEnvironment)
	if errParse != nil {
		return nil, fmt.Errorf("parse server launch environment: %w", errParse)
	}

	effectiveEnv, issues := launchenv.MergeNormal(gameDefaultEnv, serverEnv)
	if len(issues) > 0 {
		errValidation := launchenv.NewValidationError(issues)
		return nil, fmt.Errorf("validate normal launch environment: %w", errValidation)
	}
	return effectiveEnv, nil
}

func buildNormalLaunchEnvironment(variables []launchenv.Variable) (map[string]string, error) {
	environment, issues := launchenv.BuildLaunchEnv(variables, nil)
	if len(issues) > 0 {
		errValidation := launchenv.NewValidationError(issues)
		return nil, fmt.Errorf("build normal launch environment: %w", errValidation)
	}
	return environment, nil
}

func addNormalEnvironmentPlaceholders(variables []launchenv.Variable, placeholders map[string]string) {
	for _, variable := range variables {
		name := strings.ToUpper(variable.Name)
		_, builtIn := placeholders[name]
		if builtIn {
			continue
		}
		placeholders[name] = variable.Value
	}
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

func (inst *Instance) secretStartPlaceholderVars(gameServer *models.GameServer) (map[string]string, error) {
	if gameServer == nil || gameServer.R.Game == nil || !gameServer.R.Game.RequiresSteamGameServerLoginToken {
		return map[string]string{}, nil
	}
	if inst.db == nil {
		return nil, errors.New("load Steam GSLT: database is missing")
	}

	token, configured, errToken := inst.db.DecryptGameServerSecret(
		gameServer.ID,
		db.GameServerSecretKindSteamGSLT,
		db.GameServerSecretNameSteamGSLT,
	)
	if errToken != nil {
		return nil, fmt.Errorf("load Steam GSLT: %w", errToken)
	}
	if !configured || strings.TrimSpace(token) == "" {
		return nil, errors.New("load Steam GSLT: token is not configured")
	}

	return map[string]string{steamGSLTPlaceholder: token}, nil
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

func (inst *Instance) prepareLaunchSecrets(gameServer *models.GameServer, _ nodeclient.NodeClient, launchEnv map[string]string) (map[string]string, error) {
	if !readiness.RequiresHytaleAccount(gameServer) {
		if launchEnv == nil {
			return map[string]string{}, nil
		}
		return launchEnv, nil
	}

	if inst.hytaleLaunchLocks == nil {
		inst.hytaleLaunchLocks = readiness.NewHytaleLaunchLocks()
	}
	unlock := inst.hytaleLaunchLocks.Lock(gameServer.ID)
	defer unlock()
	prepared, errPrepare := readiness.PrepareHytaleLaunchSecrets(inst.ctx, inst.db, gameServer, inst.hytaleClient, launchEnv)
	if errPrepare != nil {
		return nil, fmt.Errorf("prepare hytale launch secrets: %w", errPrepare)
	}
	return prepared, nil
}
