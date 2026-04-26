package rpc

import (
	"context"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/controller/launchenv"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type serverEnvironmentView struct {
	gameDefaultEnv   []launchenv.Variable
	serverEnv        []launchenv.Variable
	effectiveEnv     []launchenv.Variable
	secretEnv        []launchenv.SecretState
	validationIssues []launchenv.ValidationIssue
}

// GetGameServerEnvironment returns visible server env plus safe secret metadata.
func (xs *XylonaService) GetGameServerEnvironment(
	_ context.Context,
	request *connect.Request[xylona.GetGameServerEnvironmentRequest],
) (*connect.Response[xylona.GetGameServerEnvironmentResponse], error) {
	_, gameServer, errAccess := xs.getEnvironmentGameServer(request.Header(), request.Msg.GetServerId())
	if errAccess != nil {
		return nil, errAccess
	}

	view, errView := xs.loadServerEnvironmentView(gameServer)
	if errView != nil {
		return nil, errView
	}
	return connect.NewResponse(&xylona.GetGameServerEnvironmentResponse{
		GameDefaultEnv:   environmentVariablesToProto(view.gameDefaultEnv),
		ServerEnv:        environmentVariablesToProto(view.serverEnv),
		EffectiveEnv:     environmentVariablesToProto(view.effectiveEnv),
		SecretEnv:        secretStatesToProto(view.secretEnv),
		ValidationIssues: validationIssuesToProto(view.validationIssues),
	}), nil
}

// UpdateGameServerEnvironment replaces the visible per-server env var list.
func (xs *XylonaService) UpdateGameServerEnvironment(
	_ context.Context,
	request *connect.Request[xylona.UpdateGameServerEnvironmentRequest],
) (*connect.Response[xylona.UpdateGameServerEnvironmentResponse], error) {
	user, gameServer, errAccess := xs.getEnvironmentGameServer(request.Header(), request.Msg.GetServerId())
	if errAccess != nil {
		return nil, errAccess
	}
	errMutation := ensureEnvironmentMutationAllowed(user, gameServer)
	if errMutation != nil {
		return nil, errMutation
	}

	serverEnv := environmentVariablesFromProto(request.Msg.GetEnvVars())
	encoded, errMarshal := launchenv.MarshalStored(serverEnv)
	if errMarshal != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errMarshal)
	}

	effectiveEnv, errEffectiveEnv := effectiveServerEnvironment(gameServer, serverEnv)
	if errEffectiveEnv != nil {
		return nil, errEffectiveEnv
	}

	secretStates, errSecretStates := xs.listLaunchSecretStates(gameServer.ID)
	if errSecretStates != nil {
		return nil, errSecretStates
	}
	errIssues := environmentValidationError(launchenv.ValidateSecretStates(secretStates, effectiveEnv))
	if errIssues != nil {
		return nil, errIssues
	}

	_, errUpdate := xs.db.UpdateGameServer(xs.db.DB, &models.GameServerSetter{
		ID:      omit.From(gameServer.ID),
		EnvVars: omit.From(encoded),
	})
	if errUpdate != nil {
		return nil, internalErr()
	}

	return connect.NewResponse(&xylona.UpdateGameServerEnvironmentResponse{
		ServerEnv:        environmentVariablesToProto(serverEnv),
		EffectiveEnv:     environmentVariablesToProto(effectiveEnv),
		ValidationIssues: []*xylona.EnvironmentValidationIssue{},
	}), nil
}

// SetGameServerSecretEnv creates or replaces one secret env value.
func (xs *XylonaService) SetGameServerSecretEnv(
	_ context.Context,
	request *connect.Request[xylona.SetGameServerSecretEnvRequest],
) (*connect.Response[xylona.SetGameServerSecretEnvResponse], error) {
	user, gameServer, errAccess := xs.getEnvironmentGameServer(request.Header(), request.Msg.GetServerId())
	if errAccess != nil {
		return nil, errAccess
	}
	errMutation := ensureEnvironmentMutationAllowed(user, gameServer)
	if errMutation != nil {
		return nil, errMutation
	}

	name := strings.TrimSpace(request.Msg.GetName())
	errIssues := environmentValidationError(launchenv.ValidateSecretInput(name, request.Msg.GetValue()))
	if errIssues != nil {
		return nil, errIssues
	}

	serverEnv, errServerEnv := parseStoredServerEnvironment(gameServer)
	if errServerEnv != nil {
		return nil, errServerEnv
	}
	effectiveEnv, errEffectiveEnv := effectiveServerEnvironment(gameServer, serverEnv)
	if errEffectiveEnv != nil {
		return nil, errEffectiveEnv
	}

	secretStates, errSecretStates := xs.listLaunchSecretStates(gameServer.ID)
	if errSecretStates != nil {
		return nil, errSecretStates
	}
	errIssues = environmentValidationError(validateSecretNameCanBeSet(name, secretStates, effectiveEnv))
	if errIssues != nil {
		return nil, errIssues
	}

	errSet := xs.db.SetGameServerSecretEnv(gameServer.ID, name, request.Msg.GetValue(), user.ID)
	if errSet != nil {
		return nil, internalErrf("failed to save secret environment variable")
	}

	updatedStates, errUpdatedStates := xs.listLaunchSecretStates(gameServer.ID)
	if errUpdatedStates != nil {
		return nil, errUpdatedStates
	}
	return connect.NewResponse(&xylona.SetGameServerSecretEnvResponse{
		SecretEnv:        secretStatesToProto(updatedStates),
		ValidationIssues: []*xylona.EnvironmentValidationIssue{},
	}), nil
}

// ClearGameServerSecretEnv removes one configured secret env value.
func (xs *XylonaService) ClearGameServerSecretEnv(
	_ context.Context,
	request *connect.Request[xylona.ClearGameServerSecretEnvRequest],
) (*connect.Response[xylona.ClearGameServerSecretEnvResponse], error) {
	user, gameServer, errAccess := xs.getEnvironmentGameServer(request.Header(), request.Msg.GetServerId())
	if errAccess != nil {
		return nil, errAccess
	}
	errMutation := ensureEnvironmentMutationAllowed(user, gameServer)
	if errMutation != nil {
		return nil, errMutation
	}

	name := strings.TrimSpace(request.Msg.GetName())
	errIssues := environmentValidationError(launchenv.ValidateName(name))
	if errIssues != nil {
		return nil, errIssues
	}

	errClear := xs.db.ClearGameServerSecretEnv(gameServer.ID, name)
	if errClear != nil {
		return nil, internalErrf("failed to clear secret environment variable")
	}

	updatedStates, errUpdatedStates := xs.listLaunchSecretStates(gameServer.ID)
	if errUpdatedStates != nil {
		return nil, errUpdatedStates
	}
	return connect.NewResponse(&xylona.ClearGameServerSecretEnvResponse{
		SecretEnv:        secretStatesToProto(updatedStates),
		ValidationIssues: []*xylona.EnvironmentValidationIssue{},
	}), nil
}

func (xs *XylonaService) getEnvironmentGameServer(header http.Header, serverID string) (*models.User, *models.GameServer, error) {
	user, errUser := xs.getUserFromHeader(header)
	if errUser != nil {
		return nil, nil, unauthenticated()
	}

	gameServer, errLookup := xs.db.GetGameServerByID(serverID)
	if errLookup != nil {
		return nil, nil, dbLookup(errLookup)
	}

	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.settings")
	if errPermission != nil {
		return nil, nil, errPermission
	}
	return user, gameServer, nil
}

func ensureEnvironmentMutationAllowed(user *models.User, gameServer *models.GameServer) error {
	if gameServer.R.Game == nil {
		return internalErrf("game relation missing")
	}
	if !user.SuperUser && !gameServer.R.Game.AllowStartArgEditing {
		return permissionDenied("environment editing is disabled for this game")
	}
	return nil
}

func (xs *XylonaService) loadServerEnvironmentView(gameServer *models.GameServer) (*serverEnvironmentView, error) {
	gameDefaultEnv, errDefaultEnv := parseStoredGameDefaultEnvironment(gameServer)
	if errDefaultEnv != nil {
		return nil, errDefaultEnv
	}
	serverEnv, errServerEnv := parseStoredServerEnvironment(gameServer)
	if errServerEnv != nil {
		return nil, errServerEnv
	}

	effectiveEnv, issues := launchenv.MergeNormal(gameDefaultEnv, serverEnv)
	secretStates, errSecretStates := xs.listLaunchSecretStates(gameServer.ID)
	if errSecretStates != nil {
		return nil, errSecretStates
	}
	issues = append(issues, launchenv.ValidateSecretStates(secretStates, effectiveEnv)...)

	return &serverEnvironmentView{
		gameDefaultEnv:   gameDefaultEnv,
		serverEnv:        serverEnv,
		effectiveEnv:     effectiveEnv,
		secretEnv:        secretStates,
		validationIssues: issues,
	}, nil
}

func parseStoredGameDefaultEnvironment(gameServer *models.GameServer) ([]launchenv.Variable, error) {
	if gameServer.R.Game == nil {
		return nil, internalErrf("game relation missing")
	}
	gameDefaultEnv, errParse := launchenv.ParseStored(gameServer.R.Game.DefaultEnvVars)
	if errParse != nil {
		return nil, internalErrf("stored game default environment variables are invalid")
	}
	return gameDefaultEnv, nil
}

func parseStoredServerEnvironment(gameServer *models.GameServer) ([]launchenv.Variable, error) {
	serverEnv, errParse := launchenv.ParseStored(gameServer.EnvVars)
	if errParse != nil {
		return nil, internalErrf("stored environment variables are invalid")
	}
	return serverEnv, nil
}

func effectiveServerEnvironment(gameServer *models.GameServer, serverEnv []launchenv.Variable) ([]launchenv.Variable, error) {
	gameDefaultEnv, errDefaultEnv := parseStoredGameDefaultEnvironment(gameServer)
	if errDefaultEnv != nil {
		return nil, errDefaultEnv
	}
	effectiveEnv, issues := launchenv.MergeNormal(gameDefaultEnv, serverEnv)
	errIssues := environmentValidationError(issues)
	if errIssues != nil {
		return nil, errIssues
	}
	return effectiveEnv, nil
}

func (xs *XylonaService) listLaunchSecretStates(gameServerID string) ([]launchenv.SecretState, error) {
	dbStates, errList := xs.db.ListGameServerSecretEnvStates(gameServerID)
	if errList != nil {
		return nil, internalErr()
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

func validateSecretNameCanBeSet(name string, existingStates []launchenv.SecretState, effectiveEnv []launchenv.Variable) []launchenv.ValidationIssue {
	states := make([]launchenv.SecretState, 0, len(existingStates)+1)
	replaced := false
	for _, state := range existingStates {
		if strings.EqualFold(state.Name, name) {
			replaced = true
		}
		states = append(states, state)
	}
	if !replaced {
		states = append(states, launchenv.SecretState{Name: name, Configured: true})
	}
	return launchenv.ValidateSecretStates(states, effectiveEnv)
}

func environmentValidationError(issues []launchenv.ValidationIssue) error {
	if len(issues) == 0 {
		return nil
	}
	return connect.NewError(connect.CodeInvalidArgument, launchenv.NewValidationError(issues))
}

func environmentVariablesFromProto(protoVars []*xylona.EnvironmentVariable) []launchenv.Variable {
	variables := make([]launchenv.Variable, len(protoVars))
	for i, variable := range protoVars {
		variables[i] = launchenv.Variable{
			Name:  strings.TrimSpace(variable.GetName()),
			Value: variable.GetValue(),
		}
	}
	return variables
}

func environmentVariablesToProto(variables []launchenv.Variable) []*xylona.EnvironmentVariable {
	protoVars := make([]*xylona.EnvironmentVariable, len(variables))
	for i, variable := range variables {
		protoVars[i] = &xylona.EnvironmentVariable{
			Name:  variable.Name,
			Value: variable.Value,
		}
	}
	return protoVars
}

func secretStatesToProto(states []launchenv.SecretState) []*xylona.SecretEnvironmentVariableState {
	protoStates := make([]*xylona.SecretEnvironmentVariableState, len(states))
	for i, state := range states {
		protoStates[i] = &xylona.SecretEnvironmentVariableState{
			Name:       state.Name,
			Configured: state.Configured,
			UpdatedAt:  timestamppb.New(state.UpdatedAt),
		}
	}
	return protoStates
}

func validationIssuesToProto(issues []launchenv.ValidationIssue) []*xylona.EnvironmentValidationIssue {
	protoIssues := make([]*xylona.EnvironmentValidationIssue, len(issues))
	for i, issue := range issues {
		protoIssues[i] = &xylona.EnvironmentValidationIssue{
			Name:    issue.Name,
			Message: issue.Message,
		}
	}
	return protoIssues
}
