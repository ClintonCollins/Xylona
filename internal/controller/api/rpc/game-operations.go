package rpc

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	gameOperationsNodeProtocol        int64 = 12
	gameOperationMetadataNodeProtocol int64 = 13
)

type operationAvailability struct {
	available bool
	reason    xylona.GameOperationAvailabilityReason
	text      string
}

type gameOperationEnvironment struct {
	capabilities            node.RuntimeCapabilities
	access                  sevenDaysToDiePrivateReadAccess
	status                  *node.SevenDaysToDieWebAPIStatus
	playerActionsConfigured bool
	preconditionUnavailable operationAvailability
	nativeUnavailable       operationAvailability
}

type gameOperationInFlight struct {
	serverID      string
	operationID   string
	operationName string
	requestKey    string
	lockKey       string
	conflictsWith []string
}

// ListGameServerOperations returns authorized, transport-neutral operations for one server.
func (xs *XylonaService) ListGameServerOperations(
	ctx context.Context,
	request *connect.Request[xylona.ListGameServerOperationsRequest],
) (*connect.Response[xylona.ListGameServerOperationsResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServerID := strings.TrimSpace(request.Msg.GetGameServerId())
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}
	gameServer, errServer := xs.getGameServerFromID(gameServerID)
	if errServer != nil {
		return nil, errServer
	}
	errView := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerView)
	if errView != nil {
		return nil, errView
	}

	catalog := gameintegrations.OperationsForGame(gameServer.GameID)
	authorized := make([]gameintegrations.OperationDescriptor, 0, len(catalog))
	for _, operation := range catalog {
		allowed, errPermission := db.HasPermission(xs.db, user, gameServer.ID, gameServer.UserID, operation.PermissionID)
		if errPermission != nil {
			log.Error().Err(errPermission).
				Str("server_id", gameServer.ID).
				Str("permission_id", operation.PermissionID).
				Str("user_id", user.ID).
				Msg("failed to filter game operation permissions")
			return nil, internalErrf("failed to check game operation permissions")
		}
		if allowed {
			authorized = append(authorized, operation)
		}
	}

	response := &xylona.ListGameServerOperationsResponse{GameServerName: gameServer.Name}
	if len(authorized) == 0 {
		return connect.NewResponse(response), nil
	}

	environment, errEnvironment := xs.resolveGameOperationEnvironment(ctx, gameServer, authorized)
	if errEnvironment != nil {
		return nil, errEnvironment
	}
	metadata := new(node.SevenDaysToDieOperationMetadata)
	if gameServer.GameID == "7_days_to_die" &&
		environment.access.client != nil &&
		environment.capabilities.ProtocolVersion >= gameOperationMetadataNodeProtocol {
		queriedMetadata, errMetadata := environment.access.client.QuerySevenDaysToDieOperationMetadata(ctx, node.SevenDaysToDieOperationMetadataQueryRequest{
			WorkingDirectory: environment.access.workingDirectory,
		})
		if errors.Is(errMetadata, context.Canceled) || errors.Is(errMetadata, context.DeadlineExceeded) {
			return nil, connect.NewError(contextConnectCode(errMetadata), errMetadata)
		}
		if errMetadata != nil {
			log.Warn().Err(errMetadata).Str("server_id", gameServer.ID).Msg("failed to load game operation metadata")
		} else if queriedMetadata != nil {
			metadata = queriedMetadata
		}
	}
	for _, command := range environment.status.KnownCommands {
		metadata.Commands = append(metadata.Commands, node.SevenDaysToDieOperationOption{
			Label: command, Value: command, Description: "Native command",
		})
	}
	response.Operations = make([]*xylona.GameOperationDescriptor, 0, len(authorized))
	knownPlayerOptions := publicGameOperationOptions(metadata.Players, gameServer.ID, false)
	var onlinePlayerOptions []*xylona.GameOperationFieldOption
	if environment.status.Capabilities.PlayerData {
		var errPlayers error
		onlinePlayerOptions, errPlayers = gameOperationPlayerOptions(ctx, environment.access)
		if errPlayers != nil {
			return nil, errPlayers
		}
		knownPlayerOptions = mergeGameOperationOptions(knownPlayerOptions, onlinePlayerOptions)
	}
	for _, operation := range authorized {
		availability := gameOperationAvailability(environment, gameServer.GameID, operation)
		response.Operations = append(response.Operations, publicGameOperation(
			operation,
			availability,
			knownPlayerOptions,
			onlinePlayerOptions,
			metadata,
			gameServer.ID,
		))
	}
	return connect.NewResponse(response), nil
}

// ExecuteGameServerOperation reauthorizes and executes one structured operation on its owning node.
func (xs *XylonaService) ExecuteGameServerOperation(
	ctx context.Context,
	request *connect.Request[xylona.ExecuteGameServerOperationRequest],
) (*connect.Response[xylona.ExecuteGameServerOperationResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServerID := strings.TrimSpace(request.Msg.GetGameServerId())
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}
	operationID := strings.TrimSpace(request.Msg.GetOperationId())
	if operationID == "" {
		return nil, invalidArg("operation_id is required")
	}

	gameServer, errServer := xs.getGameServerFromID(gameServerID)
	if errServer != nil {
		return nil, errServer
	}
	errView := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerView)
	if errView != nil {
		return nil, errView
	}

	operation, found := gameOperationByID(gameServer.GameID, operationID)
	if !found {
		return nil, invalidArg("unknown game operation")
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, operation.PermissionID)
	if errPermission != nil {
		return nil, errPermission
	}

	environment, errEnvironment := xs.resolveGameOperationEnvironment(
		ctx,
		gameServer,
		[]gameintegrations.OperationDescriptor{operation},
	)
	if errEnvironment != nil {
		return nil, errEnvironment
	}
	availability := gameOperationAvailability(environment, gameServer.GameID, operation)
	if !availability.available {
		return failedPublicGameOperation(availability.text), nil
	}
	access := environment.access
	values := nodeGameOperationValues(request.Msg.GetValues())
	releaseOperation, conflictMessage := xs.beginGameOperation(gameServer.ID, operation, values)
	if conflictMessage != "" {
		return failedPublicGameOperation(conflictMessage), nil
	}
	defer releaseOperation()

	playerAction, isPlayerAction := gameOperationPlayerAction(operationID)
	var result node.GameOperationResult
	var errExecute error
	if isPlayerAction {
		player, reason, validationFailure := gameOperationPlayerActionValues(operationID, request.Msg.GetValues())
		if validationFailure != "" {
			return failedPublicGameOperation(validationFailure), nil
		}
		errExecute = xs.actionsInst.PerformPlayerAction(ctx, gameServer, playerAction, player, reason)
		result = playerActionOperationResult(operation.Name, errExecute)
	} else {
		result, errExecute = access.client.ExecuteGameOperation(ctx, node.GameOperationRequest{
			WorkingDirectory: access.workingDirectory,
			TokenName:        access.tokenName,
			TokenSecret:      access.tokenSecret,
			OperationID:      operationID,
			Values:           values,
		})
	}
	if errExecute != nil {
		if errors.Is(errExecute, context.Canceled) || errors.Is(errExecute, context.DeadlineExceeded) {
			return nil, connect.NewError(contextConnectCode(errExecute), errExecute)
		}
		if !isPlayerAction {
			return failedPublicGameOperation("The server node could not execute the operation."), nil
		}
	}

	return connect.NewResponse(&xylona.ExecuteGameServerOperationResponse{
		Result: publicGameOperationResult(result, access.tokenName, access.tokenSecret),
	}), nil
}

func (xs *XylonaService) resolveGameOperationEnvironment(
	ctx context.Context,
	gameServer *models.GameServer,
	operations []gameintegrations.OperationDescriptor,
) (gameOperationEnvironment, error) {
	environment := gameOperationEnvironment{
		status:                  &node.SevenDaysToDieWebAPIStatus{},
		playerActionsConfigured: xs.actionsInst != nil,
	}
	preconditionDisabled := func(reason xylona.GameOperationAvailabilityReason, text string) (gameOperationEnvironment, error) {
		environment.preconditionUnavailable = operationAvailability{reason: reason, text: text}
		return environment, nil
	}
	nativeDisabled := func(reason xylona.GameOperationAvailabilityReason, text string) (gameOperationEnvironment, error) {
		environment.nativeUnavailable = operationAvailability{reason: reason, text: text}
		return environment, nil
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return preconditionDisabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_UNAVAILABLE,
			"The server node is not currently reachable.",
		)
	}
	environment.access = sevenDaysToDiePrivateReadAccess{
		client:           client,
		workingDirectory: gameServer.Directory,
	}
	capabilities, errCapabilities := client.GetRuntimeCapabilities(ctx)
	if errCapabilities != nil {
		if errors.Is(errCapabilities, context.Canceled) || errors.Is(errCapabilities, context.DeadlineExceeded) {
			return environment, connect.NewError(contextConnectCode(errCapabilities), errCapabilities)
		}
		return preconditionDisabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_CAPABILITY_UNAVAILABLE,
			"The node could not report its supported game operations.",
		)
	}
	environment.capabilities = capabilities
	process, found, errProcess := client.GetProcessSnapshot(ctx, gameServer.ID)
	if errProcess != nil {
		if errors.Is(errProcess, context.Canceled) || errors.Is(errProcess, context.DeadlineExceeded) {
			return environment, connect.NewError(contextConnectCode(errProcess), errProcess)
		}
		return preconditionDisabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_UNAVAILABLE,
			"The server node could not report the live process state.",
		)
	}
	if !found || process == nil || process.Status != xylona.Status_ONLINE.String() {
		return preconditionDisabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_SERVER_OFFLINE,
			"Start the game server to make this operation available.",
		)
	}

	if capabilities.ProtocolVersion < gameOperationsNodeProtocol ||
		!slices.ContainsFunc(operations, func(operation gameintegrations.OperationDescriptor) bool {
			return capabilities.SupportsGameOperation(gameServer.GameID, operation.ID)
		}) {
		return environment, nil
	}
	if xs.actionsInst == nil {
		return nativeDisabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_SERVER_CONFIGURATION_INVALID,
			"The native dashboard credentials are not configured.",
		)
	}

	tokenName, tokenSecret, errCredentials := xs.actionsInst.SevenDaysToDieMapCredentials(gameServer)
	if errCredentials != nil {
		return nativeDisabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_SERVER_CONFIGURATION_INVALID,
			"The native dashboard credentials could not be resolved.",
		)
	}
	environment.access = sevenDaysToDiePrivateReadAccess{
		client:           client,
		workingDirectory: gameServer.Directory,
		tokenName:        tokenName,
		tokenSecret:      tokenSecret,
	}
	queriedStatus, errStatus := client.QuerySevenDaysToDieWebAPIStatus(ctx, node.SevenDaysToDieWebAPIStatusQueryRequest{
		WorkingDirectory: gameServer.Directory,
		TokenName:        tokenName,
		TokenSecret:      tokenSecret,
	})
	if errStatus != nil {
		if errors.Is(errStatus, context.Canceled) || errors.Is(errStatus, context.DeadlineExceeded) {
			return environment, connect.NewError(contextConnectCode(errStatus), errStatus)
		}
		return nativeDisabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_DASHBOARD_UNREACHABLE,
			"The native dashboard could not be reached.",
		)
	}
	if queriedStatus == nil {
		return nativeDisabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_DASHBOARD_UNREACHABLE,
			"The native dashboard returned no capability information.",
		)
	}
	environment.status = queriedStatus

	switch queriedStatus.ConnectionState {
	case node.SevenDaysToDieWebAPIConnectionStateAvailable:
		return environment, nil
	case node.SevenDaysToDieWebAPIConnectionStateServerOffline:
		return nativeDisabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_SERVER_OFFLINE,
			"Start the game server to make this operation available.",
		)
	case node.SevenDaysToDieWebAPIConnectionStateDashboardDisabled:
		return nativeDisabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_DASHBOARD_DISABLED,
			"Enable the native dashboard for this game server.",
		)
	case node.SevenDaysToDieWebAPIConnectionStateAuthenticationDenied:
		return nativeDisabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_AUTHENTICATION_DENIED,
			"The native dashboard rejected the configured credentials.",
		)
	case node.SevenDaysToDieWebAPIConnectionStateMisconfigured, node.SevenDaysToDieWebAPIConnectionStateInvalidResponse:
		return nativeDisabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_SERVER_CONFIGURATION_INVALID,
			"The native dashboard configuration is invalid.",
		)
	case node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable:
		return nativeDisabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_UNAVAILABLE,
			"The server node is not currently reachable.",
		)
	default:
		return nativeDisabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_DASHBOARD_UNREACHABLE,
			"The native dashboard capability could not be confirmed.",
		)
	}
}

func gameOperationAvailability(
	environment gameOperationEnvironment,
	gameID string,
	operation gameintegrations.OperationDescriptor,
) operationAvailability {
	if environment.preconditionUnavailable.reason != xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_UNSPECIFIED {
		return environment.preconditionUnavailable
	}
	if environment.capabilities.ProtocolVersion < gameOperationsNodeProtocol ||
		!environment.capabilities.SupportsGameOperation(gameID, operation.ID) {
		return operationAvailability{
			reason: xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_UNSUPPORTED,
			text:   "Update the node to a version that supports game operations.",
		}
	}
	if operation.NativeCapability == gameintegrations.OperationNativeCapabilityPlayerActions {
		if !environment.capabilities.PlayerActions {
			return operationAvailability{
				reason: xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_UNSUPPORTED,
				text:   "Update the node to a version that supports typed 7 Days to Die Player actions.",
			}
		}
		if !environment.playerActionsConfigured {
			return operationAvailability{
				reason: xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_SERVER_CONFIGURATION_INVALID,
				text:   "Player actions are not configured on the controller.",
			}
		}
		return operationAvailability{available: true}
	}
	if environment.nativeUnavailable.reason != xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_UNSPECIFIED {
		return environment.nativeUnavailable
	}

	switch operation.NativeCapability {
	case gameintegrations.OperationNativeCapabilityGamePermissions:
		if environment.status.Capabilities.GamePermissions {
			return operationAvailability{available: true}
		}
		return operationAvailability{
			reason: xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_GAME_PERMISSION_UNSUPPORTED,
			text:   "This game version does not expose native game-permission management.",
		}
	case gameintegrations.OperationNativeCapabilityCommand:
		if !environment.status.Capabilities.CommandExecution {
			return operationAvailability{
				reason: xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_COMMAND_UNSUPPORTED,
				text:   "This game version does not expose native command execution.",
			}
		}
		if environment.status.CommandOperationsState != node.SevenDaysToDieWebAPIValueStateAvailable {
			return operationAvailability{
				reason: xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_COMMAND_UNSUPPORTED,
				text:   "The native command permissions could not be confirmed.",
			}
		}
		if !slices.Contains(environment.status.SupportedGameOperations, operation.ID) {
			return operationAvailability{
				reason: xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_COMMAND_UNSUPPORTED,
				text:   "The running game version does not expose this native command.",
			}
		}
		if !slices.Contains(environment.status.AllowedGameOperations, operation.ID) {
			return operationAvailability{
				reason: xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_COMMAND_UNSUPPORTED,
				text:   "The configured native dashboard token is not allowed to execute this operation.",
			}
		}
		if operation.ID == gameintegrations.OperationIDGiveItem && !environment.status.Capabilities.PlayerData {
			return operationAvailability{
				reason: xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_COMMAND_UNSUPPORTED,
				text:   "This game version does not expose native player lookup.",
			}
		}
		return operationAvailability{available: true}
	case gameintegrations.OperationNativeCapabilityCommandPermissions:
		if environment.status.Capabilities.CommandPermissions {
			return operationAvailability{available: true}
		}
		return operationAvailability{
			reason: xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_GAME_PERMISSION_UNSUPPORTED,
			text:   "This game version does not expose native command-permission management.",
		}
	default:
		return operationAvailability{
			reason: xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_UNSUPPORTED,
			text:   "The operation has no supported native capability contract.",
		}
	}
}

func (xs *XylonaService) beginGameOperation(
	serverID string,
	operation gameintegrations.OperationDescriptor,
	values []node.GameOperationValue,
) (func(), string) {
	requestKey := gameOperationRequestKey(serverID, operation.ID, values)
	lockKey := gameOperationLockKey(serverID, operation.Concurrency, values)
	entry := gameOperationInFlight{
		serverID:      serverID,
		operationID:   operation.ID,
		operationName: operation.Name,
		requestKey:    requestKey,
		lockKey:       lockKey,
		conflictsWith: slices.Clone(operation.Concurrency.ConflictsWith),
	}

	xs.gameOperationMu.Lock()
	defer xs.gameOperationMu.Unlock()
	if xs.gameOperationsInFlight == nil {
		xs.gameOperationsInFlight = make(map[string]gameOperationInFlight)
	}
	for _, active := range xs.gameOperationsInFlight {
		if active.serverID != serverID {
			continue
		}
		if active.requestKey == requestKey {
			return nil, "An identical " + operation.Name + " request is already in progress for this game server."
		}
		if lockKey != "" && active.lockKey == lockKey {
			return nil, operation.Name + " is already in progress for the same target."
		}
		if slices.Contains(operation.Concurrency.ConflictsWith, active.operationID) ||
			slices.Contains(active.conflictsWith, operation.ID) {
			return nil, operation.Name + " cannot start while " + active.operationName + " is in progress for this game server."
		}
	}
	xs.gameOperationsInFlight[requestKey] = entry
	return func() {
		xs.gameOperationMu.Lock()
		delete(xs.gameOperationsInFlight, requestKey)
		xs.gameOperationMu.Unlock()
	}, ""
}

func gameOperationRequestKey(serverID string, operationID string, values []node.GameOperationValue) string {
	encodedValues := make([]string, 0, len(values))
	for _, value := range values {
		encoded := strconv.Quote(value.FieldID) + "="
		switch {
		case value.StringValue != nil:
			encoded += "s:" + strconv.Quote(*value.StringValue)
		case value.IntegerValue != nil:
			encoded += "i:" + strconv.FormatInt(*value.IntegerValue, 10)
		case value.BooleanValue != nil:
			encoded += "b:" + strconv.FormatBool(*value.BooleanValue)
		default:
			encoded += "unset"
		}
		encodedValues = append(encodedValues, encoded)
	}
	slices.Sort(encodedValues)
	return serverID + "\x00" + operationID + "\x00" + strings.Join(encodedValues, "\x00")
}

func gameOperationLockKey(
	serverID string,
	concurrency gameintegrations.OperationConcurrency,
	values []node.GameOperationValue,
) string {
	if concurrency.Lock == "" {
		return ""
	}
	target := ""
	for _, value := range values {
		if value.FieldID == concurrency.TargetField && value.StringValue != nil {
			target = *value.StringValue
			break
		}
	}
	return serverID + "\x00" + concurrency.Lock + "\x00" + target
}

func gameOperationPlayerAction(operationID string) (node.GameServerPlayerAction, bool) {
	switch operationID {
	case gameintegrations.OperationIDKickPlayer:
		return node.GameServerPlayerActionKick, true
	case gameintegrations.OperationIDBanPlayer:
		return node.GameServerPlayerActionBan, true
	case gameintegrations.OperationIDUnbanPlayer:
		return node.GameServerPlayerActionUnban, true
	case gameintegrations.OperationIDAllowlistAdd:
		return node.GameServerPlayerActionAllowlistAdd, true
	case gameintegrations.OperationIDAllowlistRemove:
		return node.GameServerPlayerActionAllowlistRemove, true
	default:
		return node.GameServerPlayerActionUnknown, false
	}
}

func gameOperationPlayerActionValues(
	operationID string,
	values []*xylona.GameOperationValue,
) (string, string, string) {
	_, isPlayerAction := gameOperationPlayerAction(operationID)
	supportsReason := isPlayerAction &&
		(operationID == gameintegrations.OperationIDKickPlayer || operationID == gameintegrations.OperationIDBanPlayer)
	seen := make(map[string]struct{}, len(values))
	player := ""
	reason := ""
	for _, value := range values {
		if value == nil {
			return "", "", "Operation fields must not be empty."
		}
		fieldID := value.GetFieldId()
		_, found := seen[fieldID]
		if found {
			return "", "", "Duplicate operation field: " + fieldID + "."
		}
		seen[fieldID] = struct{}{}

		typedValue, stringValue := value.GetValue().(*xylona.GameOperationValue_StringValue)
		if !stringValue {
			return "", "", "Operation field " + fieldID + " must be text."
		}
		switch fieldID {
		case "player":
			player = typedValue.StringValue
		case "reason":
			if !supportsReason {
				return "", "", "Unknown operation field: reason."
			}
			reason = typedValue.StringValue
		default:
			return "", "", "Unknown operation field: " + fieldID + "."
		}
	}
	if strings.TrimSpace(player) == "" {
		return "", "", "Player is required."
	}
	return player, reason, ""
}

func playerActionOperationResult(operationName string, errAction error) node.GameOperationResult {
	details := node.GameOperationTransportDetails{
		Method:       "Typed 7 Days to Die console action",
		Verification: "Console submission only",
	}
	if errAction == nil {
		return node.GameOperationResult{
			Classification:   node.GameOperationResultAcceptedButUnverified,
			Message:          operationName + " was accepted by the server console, but the final Player state could not be verified.",
			TransportDetails: details,
		}
	}

	message := "The server node could not complete the Player action."
	switch {
	case errors.Is(errAction, node.ErrInvalidPlayerAction):
		message = "The Player identity or reason was rejected by the node."
	case errors.Is(errAction, node.ErrPlayerActionUnsupported):
		message = "This Player action is not supported by the game server."
	case errors.Is(errAction, node.ErrProcessNotFound):
		message = "The game server process is not running."
	case errors.Is(errAction, node.ErrConsoleInputUnavailable):
		message = "The game server console input is unavailable."
	case errors.Is(errAction, node.ErrPlayerActionUnavailable):
		message = "The game server could not complete the Player action."
	}
	return node.GameOperationResult{
		Classification:   node.GameOperationResultFailed,
		Message:          message,
		TransportDetails: details,
	}
}

func gameOperationPlayerOptions(
	ctx context.Context,
	access sevenDaysToDiePrivateReadAccess,
) ([]*xylona.GameOperationFieldOption, error) {
	players, errPlayers := access.client.QuerySevenDaysToDiePlayers(ctx, node.SevenDaysToDiePlayersQueryRequest{
		WorkingDirectory: access.workingDirectory,
		TokenName:        access.tokenName,
		TokenSecret:      access.tokenSecret,
	})
	if errPlayers != nil {
		if errors.Is(errPlayers, context.Canceled) || errors.Is(errPlayers, context.DeadlineExceeded) {
			return nil, connect.NewError(contextConnectCode(errPlayers), errPlayers)
		}
		return nil, nil
	}
	if players == nil || players.State != node.SevenDaysToDieWebAPIValueStateAvailable {
		return nil, nil
	}

	options := make([]*xylona.GameOperationFieldOption, 0, len(players.Players))
	for _, player := range players.Players {
		if strings.TrimSpace(player.ActionID) == "" {
			continue
		}
		identities := make([]string, 0, 3)
		if player.PlatformID != "" {
			identities = append(identities, "Platform: "+player.PlatformID)
		}
		if player.CrossPlatformID != "" {
			identities = append(identities, "Cross-platform: "+player.CrossPlatformID)
		}
		if player.EntityID != "" {
			identities = append(identities, "Entity: "+player.EntityID)
		}
		label := strings.TrimSpace(player.Name)
		if label == "" {
			label = player.ActionID
		}
		options = append(options, &xylona.GameOperationFieldOption{
			Label:       label,
			Value:       player.ActionID,
			Description: strings.Join(identities, " · "),
		})
	}
	return options, nil
}

func publicGameOperation(
	operation gameintegrations.OperationDescriptor,
	availability operationAvailability,
	knownPlayerOptions []*xylona.GameOperationFieldOption,
	onlinePlayerOptions []*xylona.GameOperationFieldOption,
	metadata *node.SevenDaysToDieOperationMetadata,
	gameServerID string,
) *xylona.GameOperationDescriptor {
	fields := make([]*xylona.GameOperationField, 0, len(operation.Fields))
	for _, field := range operation.Fields {
		options := make([]*xylona.GameOperationFieldOption, 0, len(field.Options)+len(knownPlayerOptions))
		for _, option := range field.Options {
			options = append(options, &xylona.GameOperationFieldOption{
				Label: option.Label, Value: option.Value, Description: option.Description,
			})
		}
		if field.Type == gameintegrations.OperationFieldPlayerIdentity {
			playerOptions := knownPlayerOptions
			if operation.ID == gameintegrations.OperationIDTeleportPlayer && field.ID == "destination" {
				playerOptions = onlinePlayerOptions
			}
			options = append(options, playerOptions...)
		}
		options = append(options, publicGameOperationMetadataOptions(operation.ID, field.ID, metadata, gameServerID)...)
		fields = append(fields, &xylona.GameOperationField{
			Id:                field.ID,
			Label:             field.Label,
			Description:       field.Description,
			Type:              publicGameOperationFieldType(field.Type),
			Required:          field.Required,
			DefaultValue:      field.DefaultValue,
			Options:           options,
			AllowManual:       field.AllowManual,
			AllowExactValue:   field.AllowExactValue,
			ValidationPattern: field.ValidationPattern,
			MinValue:          cloneInt32(field.MinValue),
			MaxValue:          cloneInt32(field.MaxValue),
		})
	}
	return &xylona.GameOperationDescriptor{
		Id:                       operation.ID,
		Name:                     operation.Name,
		Summary:                  operation.Summary,
		Category:                 operation.Category,
		PermissionId:             operation.PermissionID,
		Risk:                     publicGameOperationRisk(operation.Risk),
		AvailabilityRequirements: operation.AvailabilityRequirements,
		Fields:                   fields,
		Review: &xylona.GameOperationReview{
			Title: operation.Review.Title, Effect: operation.Review.Effect, Caution: operation.Review.Caution,
		},
		RendererKey:            operation.RendererKey,
		Available:              availability.available,
		AvailabilityReason:     availability.reason,
		AvailabilityReasonText: availability.text,
	}
}

func publicGameOperationMetadataOptions(
	operationID string,
	fieldID string,
	metadata *node.SevenDaysToDieOperationMetadata,
	gameServerID string,
) []*xylona.GameOperationFieldOption {
	if metadata == nil {
		return nil
	}
	switch {
	case operationID == gameintegrations.OperationIDGiveItem && fieldID == "item":
		return publicGameOperationOptions(metadata.Items, gameServerID, true)
	case (operationID == gameintegrations.OperationIDApplyBuff || operationID == gameintegrations.OperationIDRemoveBuff) && fieldID == "buff":
		return publicGameOperationOptions(metadata.Buffs, gameServerID, false)
	case (operationID == gameintegrations.OperationIDSetCommandPermission || operationID == gameintegrations.OperationIDResetCommandPermission) && fieldID == "command":
		return publicGameOperationOptions(metadata.Commands, gameServerID, false)
	default:
		return nil
	}
}

func publicGameOperationOptions(
	options []node.SevenDaysToDieOperationOption,
	gameServerID string,
	includeIcons bool,
) []*xylona.GameOperationFieldOption {
	byValue := make(map[string]*xylona.GameOperationFieldOption, len(options))
	for _, option := range options {
		if _, found := byValue[option.Value]; found || option.Value == "" {
			continue
		}
		publicOption := &xylona.GameOperationFieldOption{
			Label: option.Label, Value: option.Value, Description: option.Description,
			Category: option.Category, AccentColor: option.AccentColor,
		}
		if includeIcons && option.IconName != "" {
			publicOption.IconUrl = SevenDaysToDieOperationItemIconPathPrefix + "/" +
				url.PathEscape(gameServerID) + "/" + url.PathEscape(option.IconName) + ".png"
		}
		byValue[option.Value] = publicOption
	}
	result := make([]*xylona.GameOperationFieldOption, 0, len(byValue))
	for _, option := range byValue {
		result = append(result, option)
	}
	slices.SortFunc(result, func(left, right *xylona.GameOperationFieldOption) int {
		return strings.Compare(strings.ToLower(left.GetLabel()), strings.ToLower(right.GetLabel()))
	})
	return result
}

func mergeGameOperationOptions(
	base []*xylona.GameOperationFieldOption,
	overrides []*xylona.GameOperationFieldOption,
) []*xylona.GameOperationFieldOption {
	byValue := make(map[string]*xylona.GameOperationFieldOption, len(base)+len(overrides))
	for _, option := range base {
		byValue[option.GetValue()] = option
	}
	for _, option := range overrides {
		byValue[option.GetValue()] = option
	}
	result := make([]*xylona.GameOperationFieldOption, 0, len(byValue))
	for _, option := range byValue {
		result = append(result, option)
	}
	slices.SortFunc(result, func(left, right *xylona.GameOperationFieldOption) int {
		return strings.Compare(strings.ToLower(left.GetLabel()), strings.ToLower(right.GetLabel()))
	})
	return result
}

func publicGameOperationRisk(risk gameintegrations.OperationRisk) xylona.GameOperationRisk {
	switch risk {
	case gameintegrations.OperationRiskRoutine:
		return xylona.GameOperationRisk_GAME_OPERATION_RISK_ROUTINE
	case gameintegrations.OperationRiskCaution:
		return xylona.GameOperationRisk_GAME_OPERATION_RISK_CAUTION
	case gameintegrations.OperationRiskIrreversible:
		return xylona.GameOperationRisk_GAME_OPERATION_RISK_IRREVERSIBLE
	default:
		return xylona.GameOperationRisk_GAME_OPERATION_RISK_UNSPECIFIED
	}
}

func publicGameOperationFieldType(fieldType gameintegrations.OperationFieldType) xylona.GameOperationFieldType {
	switch fieldType {
	case gameintegrations.OperationFieldText:
		return xylona.GameOperationFieldType_GAME_OPERATION_FIELD_TYPE_TEXT
	case gameintegrations.OperationFieldInteger:
		return xylona.GameOperationFieldType_GAME_OPERATION_FIELD_TYPE_INTEGER
	case gameintegrations.OperationFieldBoolean:
		return xylona.GameOperationFieldType_GAME_OPERATION_FIELD_TYPE_BOOLEAN
	case gameintegrations.OperationFieldEnum:
		return xylona.GameOperationFieldType_GAME_OPERATION_FIELD_TYPE_ENUM
	case gameintegrations.OperationFieldDuration:
		return xylona.GameOperationFieldType_GAME_OPERATION_FIELD_TYPE_DURATION
	case gameintegrations.OperationFieldPlayerIdentity:
		return xylona.GameOperationFieldType_GAME_OPERATION_FIELD_TYPE_PLAYER_IDENTITY
	default:
		return xylona.GameOperationFieldType_GAME_OPERATION_FIELD_TYPE_UNSPECIFIED
	}
}

func cloneInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func gameOperationByID(gameID string, operationID string) (gameintegrations.OperationDescriptor, bool) {
	for _, operation := range gameintegrations.OperationsForGame(gameID) {
		if operation.ID == operationID {
			return operation, true
		}
	}
	return gameintegrations.OperationDescriptor{}, false
}

func nodeGameOperationValues(values []*xylona.GameOperationValue) []node.GameOperationValue {
	converted := make([]node.GameOperationValue, 0, len(values))
	for _, value := range values {
		if value == nil {
			converted = append(converted, node.GameOperationValue{})
			continue
		}
		convertedValue := node.GameOperationValue{FieldID: value.GetFieldId()}
		switch typedValue := value.GetValue().(type) {
		case *xylona.GameOperationValue_StringValue:
			convertedValue.StringValue = new(typedValue.StringValue)
		case *xylona.GameOperationValue_IntegerValue:
			convertedValue.IntegerValue = new(typedValue.IntegerValue)
		case *xylona.GameOperationValue_BooleanValue:
			convertedValue.BooleanValue = new(typedValue.BooleanValue)
		}
		converted = append(converted, convertedValue)
	}
	return converted
}

func publicGameOperationResult(result node.GameOperationResult, secrets ...string) *xylona.GameOperationResult {
	classification := xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_FAILED
	switch result.Classification {
	case node.GameOperationResultConfirmed:
		classification = xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_CONFIRMED
	case node.GameOperationResultAcceptedButUnverified:
		classification = xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_ACCEPTED_BUT_UNVERIFIED
	case node.GameOperationResultFailed:
		classification = xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_FAILED
	default:
		result.Message = "The node returned an invalid operation result."
	}

	publicResult := &xylona.GameOperationResult{
		Classification: classification,
		Message:        boundedRedactedGameOperationText(result.Message, 512, secrets...),
	}
	method := boundedRedactedGameOperationText(result.TransportDetails.Method, 128, secrets...)
	verification := boundedRedactedGameOperationText(result.TransportDetails.Verification, 128, secrets...)
	if method != "" || verification != "" {
		publicResult.TransportDetails = &xylona.GameOperationTransportDetails{
			Method:       method,
			Verification: verification,
		}
	}
	return publicResult
}

func failedPublicGameOperation(message string) *connect.Response[xylona.ExecuteGameServerOperationResponse] {
	return connect.NewResponse(&xylona.ExecuteGameServerOperationResponse{
		Result: &xylona.GameOperationResult{
			Classification: xylona.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_FAILED,
			Message:        boundedRedactedGameOperationText(message, 512),
		},
	})
}

func boundedRedactedGameOperationText(value string, limit int, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	runes := []rune(strings.ToValidUTF8(value, "\uFFFD"))
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}
