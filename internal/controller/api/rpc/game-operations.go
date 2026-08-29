package rpc

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const gameOperationsNodeProtocol int64 = 12

type operationAvailability struct {
	available bool
	reason    xylona.GameOperationAvailabilityReason
	text      string
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

	response.Operations = make([]*xylona.GameOperationDescriptor, 0, len(authorized))
	for _, operation := range authorized {
		availability, access, status, errAvailability := xs.gameOperationAvailability(ctx, gameServer, operation.ID)
		if errAvailability != nil {
			return nil, errAvailability
		}
		playerOptions := []*xylona.GameOperationFieldOption(nil)
		if availability.available && status.Capabilities.PlayerData && operation.ID == "player_access.add_administrator" {
			playerOptions, errAvailability = gameOperationPlayerOptions(ctx, access)
			if errAvailability != nil {
				return nil, errAvailability
			}
		}
		response.Operations = append(response.Operations, publicGameOperation(operation, availability, playerOptions))
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

	availability, access, _, errAvailability := xs.gameOperationAvailability(ctx, gameServer, operationID)
	if errAvailability != nil {
		return nil, errAvailability
	}
	if !availability.available {
		return failedPublicGameOperation(availability.text), nil
	}
	values := nodeGameOperationValues(request.Msg.GetValues())
	playerTarget := ""
	for _, value := range values {
		if value.FieldID == "player" && value.StringValue != nil {
			playerTarget = *value.StringValue
			break
		}
	}
	operationKey := gameServer.ID + "\x00" + operationID + "\x00" + playerTarget
	xs.gameOperationMu.Lock()
	if xs.gameOperationsInFlight == nil {
		xs.gameOperationsInFlight = make(map[string]struct{})
	}
	_, alreadyInFlight := xs.gameOperationsInFlight[operationKey]
	if !alreadyInFlight {
		xs.gameOperationsInFlight[operationKey] = struct{}{}
	}
	xs.gameOperationMu.Unlock()
	if alreadyInFlight {
		return failedPublicGameOperation("This operation is already in progress for this game server."), nil
	}
	defer func() {
		xs.gameOperationMu.Lock()
		delete(xs.gameOperationsInFlight, operationKey)
		xs.gameOperationMu.Unlock()
	}()

	result, errExecute := access.client.ExecuteGameOperation(ctx, node.GameOperationRequest{
		WorkingDirectory: access.workingDirectory,
		TokenName:        access.tokenName,
		TokenSecret:      access.tokenSecret,
		OperationID:      operationID,
		Values:           values,
	})
	if errExecute != nil {
		if errors.Is(errExecute, context.Canceled) || errors.Is(errExecute, context.DeadlineExceeded) {
			return nil, connect.NewError(contextConnectCode(errExecute), errExecute)
		}
		return failedPublicGameOperation("The server node could not execute the operation."), nil
	}

	return connect.NewResponse(&xylona.ExecuteGameServerOperationResponse{
		Result: publicGameOperationResult(result, access.tokenName, access.tokenSecret),
	}), nil
}

func (xs *XylonaService) gameOperationAvailability(
	ctx context.Context,
	gameServer *models.GameServer,
	operationID string,
) (operationAvailability, sevenDaysToDiePrivateReadAccess, *node.SevenDaysToDieWebAPIStatus, error) {
	var access sevenDaysToDiePrivateReadAccess
	status := &node.SevenDaysToDieWebAPIStatus{}
	disabled := func(reason xylona.GameOperationAvailabilityReason, text string) (operationAvailability, sevenDaysToDiePrivateReadAccess, *node.SevenDaysToDieWebAPIStatus, error) {
		return operationAvailability{reason: reason, text: text}, access, status, nil
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return disabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_UNAVAILABLE,
			"The server node is not currently reachable.",
		)
	}
	process, found, errProcess := client.GetProcessSnapshot(ctx, gameServer.ID)
	if errProcess != nil {
		if errors.Is(errProcess, context.Canceled) || errors.Is(errProcess, context.DeadlineExceeded) {
			return operationAvailability{}, access, status, connect.NewError(contextConnectCode(errProcess), errProcess)
		}
		return disabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_UNAVAILABLE,
			"The server node could not report the live process state.",
		)
	}
	if !found || process == nil || process.Status != xylona.Status_ONLINE.String() {
		return disabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_SERVER_OFFLINE,
			"Start the game server to make this operation available.",
		)
	}

	capabilities, errCapabilities := client.GetRuntimeCapabilities(ctx)
	if errCapabilities != nil {
		if errors.Is(errCapabilities, context.Canceled) || errors.Is(errCapabilities, context.DeadlineExceeded) {
			return operationAvailability{}, access, status, connect.NewError(contextConnectCode(errCapabilities), errCapabilities)
		}
		return disabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_CAPABILITY_UNAVAILABLE,
			"The node could not report its supported game operations.",
		)
	}
	if capabilities.ProtocolVersion < gameOperationsNodeProtocol ||
		!capabilities.SupportsGameOperation(gameServer.GameID, operationID) {
		return disabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_UNSUPPORTED,
			"Update the node to a version that supports game operations.",
		)
	}
	if xs.actionsInst == nil {
		return disabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_SERVER_CONFIGURATION_INVALID,
			"The native dashboard credentials are not configured.",
		)
	}

	tokenName, tokenSecret, errCredentials := xs.actionsInst.SevenDaysToDieMapCredentials(gameServer)
	if errCredentials != nil {
		return disabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_SERVER_CONFIGURATION_INVALID,
			"The native dashboard credentials could not be resolved.",
		)
	}
	access = sevenDaysToDiePrivateReadAccess{
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
			return operationAvailability{}, access, status, connect.NewError(contextConnectCode(errStatus), errStatus)
		}
		return disabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_DASHBOARD_UNREACHABLE,
			"The native dashboard could not be reached.",
		)
	}
	if queriedStatus == nil {
		return disabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_DASHBOARD_UNREACHABLE,
			"The native dashboard returned no capability information.",
		)
	}
	status = queriedStatus

	switch status.ConnectionState {
	case node.SevenDaysToDieWebAPIConnectionStateAvailable:
		if !status.Capabilities.GamePermissions {
			return disabled(
				xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_GAME_PERMISSION_UNSUPPORTED,
				"This game version does not expose native game-permission management.",
			)
		}
		return operationAvailability{available: true}, access, status, nil
	case node.SevenDaysToDieWebAPIConnectionStateServerOffline:
		return disabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_SERVER_OFFLINE,
			"Start the game server to make this operation available.",
		)
	case node.SevenDaysToDieWebAPIConnectionStateDashboardDisabled:
		return disabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_DASHBOARD_DISABLED,
			"Enable the native dashboard for this game server.",
		)
	case node.SevenDaysToDieWebAPIConnectionStateAuthenticationDenied:
		return disabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_AUTHENTICATION_DENIED,
			"The native dashboard rejected the configured credentials.",
		)
	case node.SevenDaysToDieWebAPIConnectionStateMisconfigured, node.SevenDaysToDieWebAPIConnectionStateInvalidResponse:
		return disabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_SERVER_CONFIGURATION_INVALID,
			"The native dashboard configuration is invalid.",
		)
	case node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable:
		return disabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NODE_UNAVAILABLE,
			"The server node is not currently reachable.",
		)
	default:
		return disabled(
			xylona.GameOperationAvailabilityReason_GAME_OPERATION_AVAILABILITY_REASON_NATIVE_DASHBOARD_UNREACHABLE,
			"The native dashboard capability could not be confirmed.",
		)
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
	playerOptions []*xylona.GameOperationFieldOption,
) *xylona.GameOperationDescriptor {
	fields := make([]*xylona.GameOperationField, 0, len(operation.Fields))
	for _, field := range operation.Fields {
		options := make([]*xylona.GameOperationFieldOption, 0, len(field.Options)+len(playerOptions))
		for _, option := range field.Options {
			options = append(options, &xylona.GameOperationFieldOption{
				Label: option.Label, Value: option.Value, Description: option.Description,
			})
		}
		if field.Type == gameintegrations.OperationFieldPlayerIdentity {
			options = append(options, playerOptions...)
		}
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
