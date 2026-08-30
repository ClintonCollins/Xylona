package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
)

var gameOperationPlayerIdentityPattern = regexp.MustCompile(gameintegrations.PlayerIdentityPattern)
var gameOperationCommandNamePattern = regexp.MustCompile(gameintegrations.CommandNamePattern)

const (
	gameOperationMessageRuneLimit = 512
	gameOperationFilterRuneLimit  = 128
	gameOperationResultRuneLimit  = 512
)

// GameOperationValue is one typed semantic input to a trusted game operation.
type GameOperationValue struct {
	FieldID      string
	StringValue  *string
	IntegerValue *int64
	BooleanValue *bool
}

// GameOperationRequest contains transport-neutral values plus controller-owned node credentials.
type GameOperationRequest struct {
	WorkingDirectory string
	TokenName        string
	TokenSecret      string
	OperationID      string
	Values           []GameOperationValue
}

// GameOperationResultClassification describes how certain an operation result is.
type GameOperationResultClassification uint8

// Game operation result classifications.
const (
	GameOperationResultConfirmed GameOperationResultClassification = iota + 1
	GameOperationResultAcceptedButUnverified
	GameOperationResultFailed
)

// GameOperationTransportDetails contains bounded, credential-free troubleshooting context.
type GameOperationTransportDetails struct {
	Method       string
	Verification string
}

// GameOperationResult is the node-authoritative transient outcome of an operation.
type GameOperationResult struct {
	Classification   GameOperationResultClassification
	Message          string
	TransportDetails GameOperationTransportDetails
}

type addAdministratorValues struct {
	playerIdentity  string
	permissionLevel int64
}

type commandPermissionValues struct {
	command         string
	permissionLevel int64
}

type sevenDaysToDieCommandPermissionState struct {
	Command         string `json:"command"`
	Default         bool   `json:"default"`
	PermissionLevel int64  `json:"permissionLevel"`
}

type sevenDaysToDieCommandPermissionsEnvelope struct {
	Data []sevenDaysToDieCommandPermissionState `json:"data"`
}

type sevenDaysToDieUserPermissionsEnvelope struct {
	Data struct {
		Users []struct {
			PermissionLevel int64 `json:"permissionLevel"`
			UserID          struct {
				CombinedString string `json:"combinedString"`
			} `json:"userId"`
		} `json:"users"`
	} `json:"data"`
}

type sevenDaysToDieCommandResultEnvelope struct {
	Data struct {
		Command    string `json:"command"`
		Parameters string `json:"parameters"`
		Result     string `json:"result"`
	} `json:"data"`
}

type sevenDaysToDieCommandCatalogEnvelope struct {
	Data struct {
		Commands []struct {
			Command string `json:"command"`
			Allowed *bool  `json:"allowed"`
		} `json:"commands"`
	} `json:"data"`
}

type sevenDaysToDieCommandOperation struct {
	name                  string
	command               string
	parameters            string
	playerIdentity        string
	parametersAfterPlayer string
	confirmed             bool
	validationFail        string
}

// ExecuteGameOperation validates and executes one integration-owned operation.
func (n *Node) ExecuteGameOperation(ctx context.Context, request GameOperationRequest) GameOperationResult {
	if ctx == nil {
		ctx = context.Background()
	}
	switch request.OperationID {
	case gameintegrations.OperationIDSetCommandPermission, gameintegrations.OperationIDResetCommandPermission:
		return n.executeCommandPermissionOperation(ctx, request)
	case gameintegrations.OperationIDBroadcastMessage,
		gameintegrations.OperationIDMessagePlayer,
		gameintegrations.OperationIDTeleportPlayer,
		gameintegrations.OperationIDGiveItem,
		gameintegrations.OperationIDGiveExperience,
		gameintegrations.OperationIDApplyBuff,
		gameintegrations.OperationIDRemoveBuff,
		gameintegrations.OperationIDSpawnAirdrop,
		gameintegrations.OperationIDSpawnWanderingHorde,
		gameintegrations.OperationIDSetWeather,
		gameintegrations.OperationIDGamePreferences,
		gameintegrations.OperationIDGameStatistics,
		gameintegrations.OperationIDGameTime,
		gameintegrations.OperationIDDLCStatus,
		gameintegrations.OperationIDItemSearch,
		gameintegrations.OperationIDVersion:
		operation, found := sevenDaysToDieCommandOperationForRequest(request.OperationID, request.Values)
		if !found {
			return failedGameOperation("Unknown game operation.")
		}
		if operation.validationFail != "" {
			return failedGameOperation(operation.validationFail)
		}
		return executeSevenDaysToDieCatalogCommandOperation(ctx, request, operation)
	case gameintegrations.OperationIDSaveWorld:
		if len(request.Values) != 0 {
			return failedGameOperation("Save world does not accept fields.")
		}
		return executeSevenDaysToDieCommandOperation(ctx, request, "saveworld", "Save world", nil)
	case gameintegrations.OperationIDSetTemperatureUnit:
		command, validationFailure := validateSetTemperatureUnitValues(request.Values)
		if validationFailure != "" {
			return failedGameOperation(validationFailure)
		}
		return executeSevenDaysToDieCommandOperation(ctx, request, command, "Set temperature unit", nil)
	case gameintegrations.OperationIDSetGameTime:
		command, expectedWorldTime, validationFailure := validateSetGameTimeValues(request.Values)
		if validationFailure != "" {
			return failedGameOperation(validationFailure)
		}
		return executeSevenDaysToDieCommandOperation(ctx, request, command, "Set game time", expectedWorldTime)
	case gameintegrations.OperationIDShutdown:
		if len(request.Values) != 0 {
			return failedGameOperation("Shut down server does not accept fields.")
		}
		return executeSevenDaysToDieCommandOperation(ctx, request, "shutdown", "Shut down server", nil)
	case gameintegrations.OperationIDAddAdministrator, gameintegrations.OperationIDRemoveAdministrator:
	default:
		return failedGameOperation("Unknown game operation.")
	}

	values := addAdministratorValues{}
	var playerIdentity string
	mutationMethod := http.MethodPost
	operationName := "Add administrator"
	if request.OperationID == gameintegrations.OperationIDAddAdministrator {
		var validationFailure string
		values, validationFailure = validateAddAdministratorValues(request.Values)
		if validationFailure != "" {
			return failedGameOperation(validationFailure)
		}
		playerIdentity = values.playerIdentity
	} else {
		var validationFailure string
		playerIdentity, validationFailure = validateRemoveAdministratorValues(request.Values)
		if validationFailure != "" {
			return failedGameOperation(validationFailure)
		}
		mutationMethod = http.MethodDelete
		operationName = "Remove administrator"
	}

	details := GameOperationTransportDetails{
		Method:       "7 Days to Die native dashboard",
		Verification: "User permission read-back",
	}
	access := newSevenDaysToDieNativeAccess(request.WorkingDirectory, request.TokenName, request.TokenSecret)
	discovery, errDiscovery := access.discover(ctx)
	if errDiscovery != nil {
		return failedGameOperationWithDetails(gameOperationTransportFailure(ctx, errDiscovery), details)
	}
	defer discovery.cancel()
	if discovery.connectionState != SevenDaysToDieWebAPIConnectionStateAvailable {
		return failedGameOperationWithDetails(gameOperationConnectionFailure(discovery.connectionState), details)
	}

	supported := discovery.resolver.supports(
		sevenDaysToDieOpenAPIOperation{path: "/api/userpermissions/user/{id}", method: mutationMethod},
		sevenDaysToDieOpenAPIOperation{path: "/api/userpermissions", method: http.MethodGet},
	)
	if !supported || discovery.resolver.failed {
		return failedGameOperationWithDetails("The node could not verify native "+operationName+" support.", details)
	}

	path := "/api/userpermissions/user/" + url.PathEscape(playerIdentity)
	if request.OperationID == gameintegrations.OperationIDAddAdministrator {
		statusCode, _, errPost := access.postWebAPIJSON(discovery.ctx, discovery.settings, path, struct {
			PermissionLevel int64 `json:"permissionLevel"`
		}{PermissionLevel: values.permissionLevel})
		if errPost != nil {
			return failedGameOperationWithDetails(gameOperationTransportFailure(ctx, errPost), details)
		}
		if statusCode != http.StatusCreated {
			return failedGameOperationWithDetails(fmt.Sprintf("The server rejected Add administrator (status %d).", statusCode), details)
		}
	} else {
		statusCode, _, errDelete := access.requestWebAPIPath(discovery.ctx, discovery.settings, http.MethodDelete, path, "application/json", nil)
		if errDelete != nil {
			return failedGameOperationWithDetails(gameOperationTransportFailure(ctx, errDelete), details)
		}
		if statusCode != http.StatusNoContent {
			return failedGameOperationWithDetails(fmt.Sprintf("The server rejected Remove administrator (status %d).", statusCode), details)
		}
	}

	statusCode, body, errReadBack := access.getWebAPI(discovery.ctx, discovery.settings, "/api/userpermissions")
	if errReadBack != nil || statusCode != http.StatusOK {
		return acceptedUnverifiedGameOperation(operationName, details)
	}
	var permissions sevenDaysToDieUserPermissionsEnvelope
	errDecode := json.Unmarshal(body, &permissions)
	if errDecode != nil || permissions.Data.Users == nil {
		return acceptedUnverifiedGameOperation(operationName, details)
	}
	for _, user := range permissions.Data.Users {
		if user.UserID.CombinedString != playerIdentity {
			continue
		}
		if request.OperationID == gameintegrations.OperationIDRemoveAdministrator {
			return failedGameOperationWithDetails("The server still reports administrator access for the selected Player.", details)
		}
		if user.PermissionLevel == values.permissionLevel {
			return GameOperationResult{
				Classification:   GameOperationResultConfirmed,
				Message:          "Administrator access was confirmed by native user permission read-back.",
				TransportDetails: details,
			}
		}
	}
	if request.OperationID == gameintegrations.OperationIDRemoveAdministrator {
		return GameOperationResult{
			Classification:   GameOperationResultConfirmed,
			Message:          "Administrator removal was confirmed by native user permission read-back.",
			TransportDetails: details,
		}
	}
	return failedGameOperationWithDetails("The server did not report the requested administrator state.", details)
}

func sevenDaysToDieCommandOperationForRequest(
	operationID string,
	values []GameOperationValue,
) (sevenDaysToDieCommandOperation, bool) {
	operation := sevenDaysToDieCommandOperation{}
	switch operationID {
	case gameintegrations.OperationIDBroadcastMessage:
		operation.name = "Send server announcement"
		operation.command = "say"
		textValues, failure := validateCommandOperationTextValues(values, "message")
		if failure != "" {
			operation.validationFail = failure
			return operation, true
		}
		operation.parameters, operation.validationFail = requiredCommandOperationArgument(
			textValues["message"],
			"Announcement",
			gameOperationMessageRuneLimit,
		)
		return operation, true
	case gameintegrations.OperationIDMessagePlayer:
		operation.name = "Send private Player message"
		operation.command = "sayplayer"
		textValues, failure := validateCommandOperationTextValues(values, "player", "message")
		if failure != "" {
			operation.validationFail = failure
			return operation, true
		}
		playerIdentity := strings.TrimSpace(textValues["player"])
		if !gameOperationPlayerIdentityPattern.MatchString(playerIdentity) {
			operation.validationFail = "Player identity must be a valid platform identity."
			return operation, true
		}
		quotedPlayer, errPlayer := quotedPlayerIdentifier(playerIdentity, "7 Days to Die")
		if errPlayer != nil {
			operation.validationFail = "Player identity must be a valid platform identity."
			return operation, true
		}
		quotedMessage, failure := requiredCommandOperationArgument(
			textValues["message"],
			"Message",
			gameOperationMessageRuneLimit,
		)
		if failure != "" {
			operation.validationFail = failure
			return operation, true
		}
		operation.parameters = quotedPlayer + " " + quotedMessage
		return operation, true
	case gameintegrations.OperationIDTeleportPlayer:
		operation.name = "Teleport Player"
		operation.command = "teleportplayer"
		textValues, failure := validateCommandOperationTextValues(values, "player", "destination")
		if failure != "" {
			operation.validationFail = failure
			return operation, true
		}
		player, failure := playerCommandOperationArgument(textValues["player"], "Player")
		if failure != "" {
			operation.validationFail = failure
			return operation, true
		}
		destination, failure := playerCommandOperationArgument(textValues["destination"], "Destination Player")
		if failure != "" {
			operation.validationFail = failure
			return operation, true
		}
		operation.parameters = player + " " + destination
		return operation, true
	case gameintegrations.OperationIDGiveItem:
		return giveItemCommandOperation(values), true
	case gameintegrations.OperationIDGiveExperience:
		return giveExperienceCommandOperation(values), true
	case gameintegrations.OperationIDApplyBuff:
		return playerBuffCommandOperation("Apply buff", "buffplayer", values), true
	case gameintegrations.OperationIDRemoveBuff:
		return playerBuffCommandOperation("Remove buff", "debuffplayer", values), true
	case gameintegrations.OperationIDSpawnAirdrop:
		operation = noValueSevenDaysToDieCommandOperation("Spawn airdrop", "spawnairdrop", values)
		operation.confirmed = false
		return operation, true
	case gameintegrations.OperationIDSpawnWanderingHorde:
		operation = noValueSevenDaysToDieCommandOperation("Spawn wandering horde", "spawnwandering", values)
		operation.parameters = "h"
		operation.confirmed = false
		return operation, true
	case gameintegrations.OperationIDSetWeather:
		return setWeatherCommandOperation(values), true
	case gameintegrations.OperationIDGamePreferences:
		return filteredSevenDaysToDieCommandOperation("Inspect game preferences", "getgamepref", values), true
	case gameintegrations.OperationIDGameStatistics:
		return filteredSevenDaysToDieCommandOperation("Inspect game statistics", "getgamestat", values), true
	case gameintegrations.OperationIDGameTime:
		return noValueSevenDaysToDieCommandOperation("Inspect game time", "gettime", values), true
	case gameintegrations.OperationIDDLCStatus:
		return noValueSevenDaysToDieCommandOperation("Inspect DLC status", "listdlc", values), true
	case gameintegrations.OperationIDItemSearch:
		operation.name = "Search item definitions"
		operation.command = "listitems"
		operation.confirmed = true
		textValues, failure := validateCommandOperationTextValues(values, "search")
		if failure != "" {
			operation.validationFail = failure
			return operation, true
		}
		operation.parameters, operation.validationFail = requiredCommandOperationArgument(
			textValues["search"],
			"Item search",
			gameOperationFilterRuneLimit,
		)
		return operation, true
	case gameintegrations.OperationIDVersion:
		return noValueSevenDaysToDieCommandOperation("Inspect game version", "version", values), true
	default:
		return operation, false
	}
}

func giveItemCommandOperation(values []GameOperationValue) sevenDaysToDieCommandOperation {
	operation := sevenDaysToDieCommandOperation{name: "Give item", command: "give"}
	seen := make(map[string]struct{}, len(values))
	var player string
	var item string
	var amount int64
	for _, value := range values {
		_, duplicate := seen[value.FieldID]
		if duplicate {
			operation.validationFail = "Duplicate operation field: " + value.FieldID + "."
			return operation
		}
		seen[value.FieldID] = struct{}{}
		switch value.FieldID {
		case "player":
			if value.StringValue == nil || value.IntegerValue != nil || value.BooleanValue != nil {
				operation.validationFail = "Player must be text."
				return operation
			}
			player = *value.StringValue
		case "item":
			if value.StringValue == nil || value.IntegerValue != nil || value.BooleanValue != nil {
				operation.validationFail = "Item must be text."
				return operation
			}
			item = *value.StringValue
		case "amount":
			var failure string
			amount, failure = commandOperationInteger(value, "Amount", 1, 1000)
			if failure != "" {
				operation.validationFail = failure
				return operation
			}
		default:
			operation.validationFail = "Unknown operation field: " + value.FieldID + "."
			return operation
		}
	}
	_, failure := playerCommandOperationArgument(player, "Player")
	if failure != "" {
		operation.validationFail = failure
		return operation
	}
	quotedItem, failure := requiredCommandOperationArgument(item, "Item", gameOperationFilterRuneLimit)
	if failure != "" {
		operation.validationFail = failure
		return operation
	}
	_, foundAmount := seen["amount"]
	if !foundAmount {
		operation.validationFail = "Amount is required."
		return operation
	}
	operation.playerIdentity = strings.TrimSpace(player)
	operation.parametersAfterPlayer = quotedItem + " " + strconv.FormatInt(amount, 10)
	return operation
}

func giveExperienceCommandOperation(values []GameOperationValue) sevenDaysToDieCommandOperation {
	operation := sevenDaysToDieCommandOperation{name: "Give experience", command: "givexp"}
	if len(values) != 2 {
		operation.validationFail = "Give experience requires one Player and an experience amount."
		return operation
	}
	var player string
	var experience int64
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		_, duplicate := seen[value.FieldID]
		if duplicate {
			operation.validationFail = "Duplicate operation field: " + value.FieldID + "."
			return operation
		}
		seen[value.FieldID] = struct{}{}
		switch value.FieldID {
		case "player":
			if value.StringValue == nil || value.IntegerValue != nil || value.BooleanValue != nil {
				operation.validationFail = "Player must be text."
				return operation
			}
			player = *value.StringValue
		case "experience":
			var failure string
			experience, failure = commandOperationInteger(value, "Experience", 1, 1000000)
			if failure != "" {
				operation.validationFail = failure
				return operation
			}
		default:
			operation.validationFail = "Unknown operation field: " + value.FieldID + "."
			return operation
		}
	}
	quotedPlayer, failure := playerCommandOperationArgument(player, "Player")
	if failure != "" {
		operation.validationFail = failure
		return operation
	}
	_, foundExperience := seen["experience"]
	if !foundExperience {
		operation.validationFail = "Experience is required."
		return operation
	}
	operation.parameters = quotedPlayer + " " + strconv.FormatInt(experience, 10)
	return operation
}

func playerBuffCommandOperation(
	name string,
	command string,
	values []GameOperationValue,
) sevenDaysToDieCommandOperation {
	operation := sevenDaysToDieCommandOperation{name: name, command: command}
	textValues, failure := validateCommandOperationTextValues(values, "player", "buff")
	if failure != "" {
		operation.validationFail = failure
		return operation
	}
	player, failure := playerCommandOperationArgument(textValues["player"], "Player")
	if failure != "" {
		operation.validationFail = failure
		return operation
	}
	buff, failure := requiredCommandOperationArgument(textValues["buff"], "Buff", gameOperationFilterRuneLimit)
	if failure != "" {
		operation.validationFail = failure
		return operation
	}
	operation.parameters = player + " " + buff
	return operation
}

func setWeatherCommandOperation(values []GameOperationValue) sevenDaysToDieCommandOperation {
	operation := sevenDaysToDieCommandOperation{name: "Set weather", command: "weather"}
	textValues, failure := validateCommandOperationTextValues(values, "weather")
	if failure != "" {
		operation.validationFail = failure
		return operation
	}
	switch textValues["weather"] {
	case "natural":
		operation.parameters = "Defaults"
	case "rain":
		operation.parameters = "Rain 1"
	case "snow":
		operation.parameters = "SnowFall 1"
	default:
		operation.validationFail = "Weather must be natural, rain, or snow."
	}
	return operation
}

func playerCommandOperationArgument(value string, label string) (string, string) {
	value = strings.TrimSpace(value)
	if !gameOperationPlayerIdentityPattern.MatchString(value) {
		return "", label + " must be a valid platform identity."
	}
	quoted, errQuote := quotedPlayerIdentifier(value, "7 Days to Die")
	if errQuote != nil {
		return "", label + " must be a valid platform identity."
	}
	return quoted, ""
}

func commandOperationInteger(value GameOperationValue, label string, minimum int64, maximum int64) (int64, string) {
	if value.IntegerValue == nil || value.StringValue != nil || value.BooleanValue != nil {
		return 0, label + " must be a whole number."
	}
	if *value.IntegerValue < minimum || *value.IntegerValue > maximum {
		return 0, fmt.Sprintf("%s must be between %d and %d.", label, minimum, maximum)
	}
	return *value.IntegerValue, ""
}

func validateCommandOperationTextValues(
	values []GameOperationValue,
	allowedFieldIDs ...string,
) (map[string]string, string) {
	allowed := make(map[string]struct{}, len(allowedFieldIDs))
	for _, fieldID := range allowedFieldIDs {
		allowed[fieldID] = struct{}{}
	}
	result := make(map[string]string, len(values))
	for _, value := range values {
		_, found := allowed[value.FieldID]
		if !found {
			return nil, "Unknown operation field: " + value.FieldID + "."
		}
		_, duplicate := result[value.FieldID]
		if duplicate {
			return nil, "Duplicate operation field: " + value.FieldID + "."
		}
		if value.StringValue == nil || value.IntegerValue != nil || value.BooleanValue != nil {
			return nil, "Operation field " + value.FieldID + " must be text."
		}
		result[value.FieldID] = *value.StringValue
	}
	return result, ""
}

func requiredCommandOperationArgument(value string, label string, limit int) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", label + " is required."
	}
	if len([]rune(value)) > limit {
		return "", fmt.Sprintf("%s must be %d characters or fewer.", label, limit)
	}
	quoted, errQuote := quoteCommandArgument(value)
	if errQuote != nil {
		return "", label + " contains unsupported characters."
	}
	return quoted, ""
}

func filteredSevenDaysToDieCommandOperation(
	name string,
	command string,
	values []GameOperationValue,
) sevenDaysToDieCommandOperation {
	operation := sevenDaysToDieCommandOperation{name: name, command: command, confirmed: true}
	textValues, failure := validateCommandOperationTextValues(values, "filter")
	if failure != "" {
		operation.validationFail = failure
		return operation
	}
	filter := strings.TrimSpace(textValues["filter"])
	if filter == "" {
		return operation
	}
	operation.parameters, operation.validationFail = requiredCommandOperationArgument(
		filter,
		"Name filter",
		gameOperationFilterRuneLimit,
	)
	return operation
}

func noValueSevenDaysToDieCommandOperation(
	name string,
	command string,
	values []GameOperationValue,
) sevenDaysToDieCommandOperation {
	operation := sevenDaysToDieCommandOperation{name: name, command: command, confirmed: true}
	if len(values) != 0 {
		operation.validationFail = "Unknown operation field: " + values[0].FieldID + "."
	}
	return operation
}

func executeSevenDaysToDieCatalogCommandOperation(
	ctx context.Context,
	request GameOperationRequest,
	operation sevenDaysToDieCommandOperation,
) GameOperationResult {
	details := GameOperationTransportDetails{
		Method:       "7 Days to Die native dashboard",
		Verification: "Native command result",
	}
	if !operation.confirmed {
		details.Verification = "No delivery read-back"
	}
	access := newSevenDaysToDieNativeAccess(request.WorkingDirectory, request.TokenName, request.TokenSecret)
	discovery, errDiscovery := access.discover(ctx)
	if errDiscovery != nil {
		return failedGameOperationWithDetails(gameOperationTransportFailure(ctx, errDiscovery), details)
	}
	defer discovery.cancel()
	if discovery.connectionState != SevenDaysToDieWebAPIConnectionStateAvailable {
		return failedGameOperationWithDetails(gameOperationConnectionFailure(discovery.connectionState), details)
	}
	supported := discovery.resolver.supports(
		sevenDaysToDieOpenAPIOperation{path: "/api/command", method: http.MethodGet},
		sevenDaysToDieOpenAPIOperation{path: "/api/command", method: http.MethodPost},
	)
	if !supported || discovery.resolver.failed {
		return failedGameOperationWithDetails("The native dashboard does not advertise command execution support.", details)
	}
	commandState, supportedOperations, allowedOperations, _, errCommands := querySevenDaysToDieCommandOperations(ctx, discovery, access)
	if errCommands != nil {
		return failedGameOperationWithDetails(gameOperationTransportFailure(ctx, errCommands), details)
	}
	if commandState != SevenDaysToDieWebAPIValueStateAvailable {
		return failedGameOperationWithDetails("The native command permissions could not be confirmed.", details)
	}
	if !slices.Contains(supportedOperations, request.OperationID) {
		return failedGameOperationWithDetails("The running game version does not expose this native command.", details)
	}
	if !slices.Contains(allowedOperations, request.OperationID) {
		return failedGameOperationWithDetails("The configured native dashboard token is not allowed to execute this operation.", details)
	}
	if operation.playerIdentity != "" {
		playerLookupSupported := discovery.resolver.supports(
			sevenDaysToDieOpenAPIOperation{path: sevenDaysToDieWebAPIEndpointPlayer, method: http.MethodGet},
		)
		if !playerLookupSupported || discovery.resolver.failed {
			return failedGameOperationWithDetails("The native dashboard does not advertise player lookup support.", details)
		}
		playerState, playerBody, errPlayers := access.queryWebAPIResource(
			discovery.ctx,
			discovery,
			sevenDaysToDieWebAPIEndpointPlayer,
		)
		if errPlayers != nil {
			return failedGameOperationWithDetails(gameOperationTransportFailure(ctx, errPlayers), details)
		}
		players, errDecodePlayers := decodeSevenDaysToDiePlayers(playerBody)
		if playerState != SevenDaysToDieWebAPIValueStateAvailable || errDecodePlayers != nil {
			return failedGameOperationWithDetails("The native dashboard could not resolve the selected player.", details)
		}
		playerTarget := ""
		for _, player := range players {
			if operation.playerIdentity != player.ActionID && operation.playerIdentity != player.PlatformID &&
				operation.playerIdentity != player.CrossPlatformID {
				continue
			}
			playerTarget = player.EntityID
			if playerTarget == "" {
				playerTarget = player.Name
			}
			break
		}
		quotedPlayerTarget, failure := requiredCommandOperationArgument(playerTarget, "Player", gameOperationFilterRuneLimit)
		if failure != "" {
			return failedGameOperationWithDetails("The selected player is no longer available.", details)
		}
		operation.parameters = quotedPlayerTarget + " " + operation.parametersAfterPlayer
	}

	command := operation.command
	if operation.parameters != "" {
		command += " " + operation.parameters
	}
	statusCode, body, errPost := access.postWebAPIJSON(discovery.ctx, discovery.settings, "/api/command", struct {
		Command string `json:"command"`
		Format  string `json:"format"`
	}{Command: command, Format: "Full"})
	if errPost != nil {
		return failedGameOperationWithDetails(gameOperationTransportFailure(ctx, errPost), details)
	}
	if statusCode != http.StatusOK {
		return failedGameOperationWithDetails(
			fmt.Sprintf("The server rejected %s (status %d).", operation.name, statusCode),
			details,
		)
	}

	var response sevenDaysToDieCommandResultEnvelope
	errDecode := json.Unmarshal(body, &response)
	responseMatches := errDecode == nil && response.Data.Command == operation.command &&
		response.Data.Parameters == operation.parameters
	if !responseMatches {
		if operation.confirmed {
			return failedGameOperationWithDetails("The native command response could not be verified.", details)
		}
		return acceptedUnverifiedCommandOperation(operation.name, details)
	}
	if !operation.confirmed {
		return acceptedUnverifiedCommandOperation(operation.name, details)
	}
	message := boundedRedactedNodeGameOperationText(
		response.Data.Result,
		gameOperationResultRuneLimit,
		request.TokenName,
		request.TokenSecret,
	)
	if message == "" {
		return failedGameOperationWithDetails("The native command returned no diagnostic output.", details)
	}
	return GameOperationResult{
		Classification:   GameOperationResultConfirmed,
		Message:          message,
		TransportDetails: details,
	}
}

func querySevenDaysToDieCommandOperations(
	ctx context.Context,
	discovery *sevenDaysToDieWebAPIDiscovery,
	access *sevenDaysToDieNativeAccess,
) (SevenDaysToDieWebAPIValueState, []string, []string, []string, error) {
	state, body, errQuery := access.queryWebAPIResource(ctx, discovery, "/api/command")
	if errQuery != nil || state != SevenDaysToDieWebAPIValueStateAvailable {
		return state, nil, nil, nil, errQuery
	}
	supportedOperations, allowedOperations, knownCommands, valid := decodeSevenDaysToDieCommandOperations(body)
	if !valid {
		return SevenDaysToDieWebAPIValueStateUnavailable, nil, nil, nil, nil
	}
	return state, supportedOperations, allowedOperations, knownCommands, nil
}

func decodeSevenDaysToDieCommandOperations(body []byte) ([]string, []string, []string, bool) {
	var envelope sevenDaysToDieCommandCatalogEnvelope
	errDecode := json.Unmarshal(body, &envelope)
	if errDecode != nil || envelope.Data.Commands == nil || len(envelope.Data.Commands) > SevenDaysToDieOperationOptionCountLimit {
		return nil, nil, nil, false
	}
	supportedOperations := make([]string, 0, len(envelope.Data.Commands))
	allowedOperations := make([]string, 0, len(envelope.Data.Commands))
	knownCommands := make([]string, 0, len(envelope.Data.Commands))
	seenOperations := make(map[string]struct{}, len(envelope.Data.Commands))
	seenCommands := make(map[string]struct{}, len(envelope.Data.Commands))
	for _, command := range envelope.Data.Commands {
		commandName := strings.TrimSpace(command.Command)
		if commandName == "" || len(commandName) > SevenDaysToDieOperationOptionFieldByteLimit || strings.ContainsAny(commandName, "\x00\r\n") {
			return nil, nil, nil, false
		}
		if _, duplicate := seenCommands[commandName]; !duplicate {
			seenCommands[commandName] = struct{}{}
			knownCommands = append(knownCommands, commandName)
		}
		operationID, modeled := sevenDaysToDieGameOperationIDForCommand(commandName)
		if !modeled {
			continue
		}
		_, duplicate := seenOperations[operationID]
		if duplicate || command.Allowed == nil {
			return nil, nil, nil, false
		}
		seenOperations[operationID] = struct{}{}
		supportedOperations = append(supportedOperations, operationID)
		if *command.Allowed {
			allowedOperations = append(allowedOperations, operationID)
		}
	}
	slices.Sort(supportedOperations)
	slices.Sort(allowedOperations)
	slices.Sort(knownCommands)
	return supportedOperations, allowedOperations, knownCommands, true
}

func sevenDaysToDieGameOperationIDForCommand(command string) (string, bool) {
	switch command {
	case "say":
		return gameintegrations.OperationIDBroadcastMessage, true
	case "sayplayer":
		return gameintegrations.OperationIDMessagePlayer, true
	case "teleportplayer":
		return gameintegrations.OperationIDTeleportPlayer, true
	case "give":
		return gameintegrations.OperationIDGiveItem, true
	case "givexp":
		return gameintegrations.OperationIDGiveExperience, true
	case "buffplayer":
		return gameintegrations.OperationIDApplyBuff, true
	case "debuffplayer":
		return gameintegrations.OperationIDRemoveBuff, true
	case "spawnairdrop":
		return gameintegrations.OperationIDSpawnAirdrop, true
	case "spawnwandering":
		return gameintegrations.OperationIDSpawnWanderingHorde, true
	case "weather":
		return gameintegrations.OperationIDSetWeather, true
	case "getgamepref":
		return gameintegrations.OperationIDGamePreferences, true
	case "getgamestat":
		return gameintegrations.OperationIDGameStatistics, true
	case "gettime":
		return gameintegrations.OperationIDGameTime, true
	case "listdlc":
		return gameintegrations.OperationIDDLCStatus, true
	case "listitems":
		return gameintegrations.OperationIDItemSearch, true
	case "version":
		return gameintegrations.OperationIDVersion, true
	case "saveworld":
		return gameintegrations.OperationIDSaveWorld, true
	case "settempunit":
		return gameintegrations.OperationIDSetTemperatureUnit, true
	case "settime":
		return gameintegrations.OperationIDSetGameTime, true
	case "shutdown":
		return gameintegrations.OperationIDShutdown, true
	default:
		return "", false
	}
}

func acceptedUnverifiedCommandOperation(name string, details GameOperationTransportDetails) GameOperationResult {
	return GameOperationResult{
		Classification:   GameOperationResultAcceptedButUnverified,
		Message:          "The server accepted " + name + ", but delivery could not be verified.",
		TransportDetails: details,
	}
}

func boundedRedactedNodeGameOperationText(value string, limit int, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	end := limit - len("…")
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + "…"
}

func validateSetTemperatureUnitValues(values []GameOperationValue) (string, string) {
	if len(values) != 1 || values[0].FieldID != "unit" {
		return "", "Set temperature unit accepts only the temperature unit field."
	}
	value := values[0]
	if value.StringValue == nil || value.IntegerValue != nil || value.BooleanValue != nil {
		return "", "Temperature unit must be F or C."
	}
	switch *value.StringValue {
	case "F", "C":
		return "settempunit " + *value.StringValue, ""
	default:
		return "", "Temperature unit must be F or C."
	}
}

func validateSetGameTimeValues(values []GameOperationValue) (string, *SevenDaysToDieGameTime, string) {
	if len(values) != 1 || values[0].FieldID != "time" {
		return "", nil, "Set game time accepts only the world time field."
	}
	value := values[0]
	if value.StringValue == nil || value.IntegerValue != nil || value.BooleanValue != nil {
		return "", nil, "World time must be day, night, or an exact day, hour, and minute."
	}
	worldTime := strings.TrimSpace(*value.StringValue)
	switch worldTime {
	case "day":
		return "settime day", &SevenDaysToDieGameTime{Day: 1, Hour: 12}, ""
	case "night":
		return "settime night", &SevenDaysToDieGameTime{Day: 2}, ""
	}
	parts := strings.Fields(worldTime)
	if len(parts) != 3 {
		return "", nil, "World time must be day, night, or an exact day, hour, and minute."
	}
	day, errDay := strconv.ParseInt(parts[0], 10, 32)
	hour, errHour := strconv.ParseInt(parts[1], 10, 32)
	minute, errMinute := strconv.ParseInt(parts[2], 10, 32)
	if errDay != nil || errHour != nil || errMinute != nil || day < 1 || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return "", nil, "Exact world time requires a day from 1 onward, hour from 0 to 23, and minute from 0 to 59."
	}
	expected := &SevenDaysToDieGameTime{Day: int32(day), Hour: int32(hour), Minute: int32(minute)}
	return "settime " + strconv.FormatInt(day, 10) + " " + strconv.FormatInt(hour, 10) + " " + strconv.FormatInt(minute, 10), expected, ""
}

func executeSevenDaysToDieCommandOperation(
	ctx context.Context,
	request GameOperationRequest,
	command string,
	operationName string,
	expectedWorldTime *SevenDaysToDieGameTime,
) GameOperationResult {
	details := GameOperationTransportDetails{
		Method:       "7 Days to Die native dashboard",
		Verification: "No authoritative read-back available",
	}
	if expectedWorldTime != nil {
		details.Verification = "World time read-back"
	}
	access := newSevenDaysToDieNativeAccess(request.WorkingDirectory, request.TokenName, request.TokenSecret)
	discovery, errDiscovery := access.discover(ctx)
	if errDiscovery != nil {
		return failedGameOperationWithDetails(gameOperationTransportFailure(ctx, errDiscovery), details)
	}
	defer discovery.cancel()
	if discovery.connectionState != SevenDaysToDieWebAPIConnectionStateAvailable {
		return failedGameOperationWithDetails(gameOperationConnectionFailure(discovery.connectionState), details)
	}

	supported := discovery.resolver.supports(
		sevenDaysToDieOpenAPIOperation{path: "/api/command", method: http.MethodPost},
	)
	if !supported || discovery.resolver.failed {
		return failedGameOperationWithDetails("The node could not verify native command support.", details)
	}
	statusCode, _, errPost := access.postWebAPIJSON(discovery.ctx, discovery.settings, "/api/command", struct {
		Command string `json:"command"`
		Format  string `json:"format"`
	}{Command: command, Format: "Simple"})
	if errPost != nil {
		return failedGameOperationWithDetails(gameOperationTransportFailure(ctx, errPost), details)
	}
	if statusCode != http.StatusOK {
		return failedGameOperationWithDetails(fmt.Sprintf("The server rejected %s (status %d).", operationName, statusCode), details)
	}
	if expectedWorldTime != nil {
		readBackSupported := discovery.resolver.supports(
			sevenDaysToDieOpenAPIOperation{path: sevenDaysToDieWebAPIEndpointServerStats, method: http.MethodGet},
		)
		if readBackSupported && !discovery.resolver.failed {
			status := &SevenDaysToDieWebAPIStatus{WorldTimeState: SevenDaysToDieWebAPIValueStateUnsupported}
			errReadBack := querySevenDaysToDieServerStats(discovery.ctx, ctx, access, discovery.settings, status)
			if errReadBack == nil && status.WorldTimeState == SevenDaysToDieWebAPIValueStateAvailable && status.WorldTime != nil {
				if *status.WorldTime != *expectedWorldTime {
					return failedGameOperationWithDetails("The server did not report the requested world time.", details)
				}
				return GameOperationResult{
					Classification:   GameOperationResultConfirmed,
					Message:          "The world time change was confirmed by native read-back.",
					TransportDetails: details,
				}
			}
		}
	}
	return GameOperationResult{
		Classification:   GameOperationResultAcceptedButUnverified,
		Message:          "The server accepted " + operationName + ", but the final state cannot be verified.",
		TransportDetails: details,
	}
}

func (*Node) executeCommandPermissionOperation(ctx context.Context, request GameOperationRequest) GameOperationResult {
	settingPermission := request.OperationID == gameintegrations.OperationIDSetCommandPermission
	values, validationFailure := validateCommandPermissionValues(request.Values, settingPermission)
	if validationFailure != "" {
		return failedGameOperation(validationFailure)
	}

	details := GameOperationTransportDetails{
		Method:       "7 Days to Die native dashboard",
		Verification: "Command permission read-back",
	}
	access := newSevenDaysToDieNativeAccess(request.WorkingDirectory, request.TokenName, request.TokenSecret)
	discovery, errDiscovery := access.discover(ctx)
	if errDiscovery != nil {
		return failedGameOperationWithDetails(gameOperationTransportFailure(ctx, errDiscovery), details)
	}
	defer discovery.cancel()
	if discovery.connectionState != SevenDaysToDieWebAPIConnectionStateAvailable {
		return failedGameOperationWithDetails(gameOperationConnectionFailure(discovery.connectionState), details)
	}

	mutationMethod := http.MethodDelete
	operationName := "Reset command permission"
	if settingPermission {
		mutationMethod = http.MethodPost
		operationName = "Set command permission"
	}
	supported := discovery.resolver.supports(
		sevenDaysToDieOpenAPIOperation{path: "/api/commandpermissions", method: http.MethodGet},
		sevenDaysToDieOpenAPIOperation{path: "/api/commandpermissions/{command}", method: mutationMethod},
	)
	if !supported || discovery.resolver.failed {
		return failedGameOperationWithDetails("The node could not verify native "+operationName+" support.", details)
	}

	statusCode, body, errRead := access.getWebAPI(discovery.ctx, discovery.settings, "/api/commandpermissions")
	if errRead != nil {
		return failedGameOperationWithDetails(gameOperationTransportFailure(ctx, errRead), details)
	}
	if statusCode != http.StatusOK {
		return failedGameOperationWithDetails(fmt.Sprintf("The server rejected command permission read-back (status %d).", statusCode), details)
	}
	permission, found, errDecode := findCommandPermission(body, values.command)
	if errDecode != nil {
		return failedGameOperationWithDetails("The server returned an invalid command permission response.", details)
	}
	if !found {
		return failedGameOperationWithDetails("Unknown command: "+values.command+".", details)
	}
	if settingPermission && !permission.Default && permission.PermissionLevel == values.permissionLevel {
		return confirmedCommandPermissionResult("Command permission already matches the requested native value.", details)
	}
	if !settingPermission && permission.Default {
		return confirmedCommandPermissionResult("Command permission already uses the native default.", details)
	}

	path := "/api/commandpermissions/" + url.PathEscape(permission.Command)
	if settingPermission {
		mutationStatusCode, _, errPost := access.postWebAPIJSON(discovery.ctx, discovery.settings, path, struct {
			PermissionLevel int64 `json:"permissionLevel"`
		}{PermissionLevel: values.permissionLevel})
		if errPost != nil {
			return failedGameOperationWithDetails(gameOperationTransportFailure(ctx, errPost), details)
		}
		if mutationStatusCode != http.StatusCreated {
			return failedGameOperationWithDetails(fmt.Sprintf("The server rejected Set command permission (status %d).", mutationStatusCode), details)
		}
	} else {
		mutationStatusCode, _, errDelete := access.requestWebAPIPath(discovery.ctx, discovery.settings, http.MethodDelete, path, "application/json", nil)
		if errDelete != nil {
			return failedGameOperationWithDetails(gameOperationTransportFailure(ctx, errDelete), details)
		}
		if mutationStatusCode != http.StatusNoContent {
			return failedGameOperationWithDetails(fmt.Sprintf("The server rejected Reset command permission (status %d).", mutationStatusCode), details)
		}
	}

	statusCode, body, errRead = access.getWebAPI(discovery.ctx, discovery.settings, "/api/commandpermissions")
	if errRead != nil || statusCode != http.StatusOK {
		return acceptedUnverifiedGameOperation(operationName, details)
	}
	permission, found, errDecode = findCommandPermission(body, permission.Command)
	if errDecode != nil || !found {
		return acceptedUnverifiedGameOperation(operationName, details)
	}
	if settingPermission && !permission.Default && permission.PermissionLevel == values.permissionLevel {
		return confirmedCommandPermissionResult("Command permission was confirmed by native command permission read-back.", details)
	}
	if !settingPermission && permission.Default {
		return confirmedCommandPermissionResult("Command permission reset was confirmed by native command permission read-back.", details)
	}
	return failedGameOperationWithDetails("The server did not report the requested command permission state.", details)
}

func validateCommandPermissionValues(values []GameOperationValue, requirePermissionLevel bool) (commandPermissionValues, string) {
	var result commandPermissionValues
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		_, found := seen[value.FieldID]
		if found {
			return result, "Duplicate operation field: " + value.FieldID + "."
		}
		seen[value.FieldID] = struct{}{}

		switch value.FieldID {
		case "command":
			if value.StringValue == nil || value.IntegerValue != nil || value.BooleanValue != nil ||
				!gameOperationCommandNamePattern.MatchString(*value.StringValue) {
				return result, "Command must be a valid native command name."
			}
			result.command = *value.StringValue
		case "permission_level":
			if !requirePermissionLevel {
				return result, "Unknown operation field: permission_level."
			}
			if value.IntegerValue == nil || value.StringValue != nil || value.BooleanValue != nil {
				return result, "Permission level must be an integer."
			}
			if *value.IntegerValue < 0 || *value.IntegerValue > 1000 {
				return result, "Permission level must be between 0 and 1000."
			}
			result.permissionLevel = *value.IntegerValue
		default:
			return result, "Unknown operation field: " + value.FieldID + "."
		}
	}
	if result.command == "" {
		return result, "Command is required."
	}
	_, foundPermissionLevel := seen["permission_level"]
	if requirePermissionLevel && !foundPermissionLevel {
		return result, "Permission level is required."
	}
	return result, ""
}

func findCommandPermission(body []byte, command string) (sevenDaysToDieCommandPermissionState, bool, error) {
	var permissions sevenDaysToDieCommandPermissionsEnvelope
	errDecode := json.Unmarshal(body, &permissions)
	if errDecode != nil {
		return sevenDaysToDieCommandPermissionState{}, false, fmt.Errorf("decode command permissions: %w", errDecode)
	}
	for _, permission := range permissions.Data {
		if strings.EqualFold(permission.Command, command) {
			return permission, true, nil
		}
	}
	return sevenDaysToDieCommandPermissionState{}, false, nil
}

func confirmedCommandPermissionResult(message string, details GameOperationTransportDetails) GameOperationResult {
	return GameOperationResult{
		Classification:   GameOperationResultConfirmed,
		Message:          message,
		TransportDetails: details,
	}
}

func validateAddAdministratorValues(values []GameOperationValue) (addAdministratorValues, string) {
	var result addAdministratorValues
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		_, found := seen[value.FieldID]
		if found {
			return result, "Duplicate operation field: " + value.FieldID + "."
		}
		seen[value.FieldID] = struct{}{}

		switch value.FieldID {
		case "player":
			if value.StringValue == nil || value.IntegerValue != nil || value.BooleanValue != nil ||
				!gameOperationPlayerIdentityPattern.MatchString(*value.StringValue) {
				return result, "Player identity must be a valid platform identity."
			}
			result.playerIdentity = *value.StringValue
		case "permission_level":
			if value.IntegerValue == nil || value.StringValue != nil || value.BooleanValue != nil {
				return result, "Permission level must be an integer."
			}
			if *value.IntegerValue < 0 || *value.IntegerValue > 1000 {
				return result, "Permission level must be between 0 and 1000."
			}
			result.permissionLevel = *value.IntegerValue
		default:
			return result, "Unknown operation field: " + value.FieldID + "."
		}
	}
	if result.playerIdentity == "" {
		return result, "Player identity is required."
	}
	_, foundPermissionLevel := seen["permission_level"]
	if !foundPermissionLevel {
		return result, "Permission level is required."
	}
	return result, ""
}

func validateRemoveAdministratorValues(values []GameOperationValue) (string, string) {
	if len(values) != 1 || values[0].FieldID != "player" {
		if len(values) == 0 {
			return "", "Player identity is required."
		}
		return "", "Remove administrator accepts only one Player identity."
	}
	value := values[0]
	if value.StringValue == nil || value.IntegerValue != nil || value.BooleanValue != nil ||
		!gameOperationPlayerIdentityPattern.MatchString(*value.StringValue) {
		return "", "Player identity must be a valid platform identity."
	}
	return *value.StringValue, ""
}

func gameOperationTransportFailure(ctx context.Context, err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "The native dashboard request timed out."
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return "The native dashboard request was canceled."
	}
	return "The native dashboard transport failed."
}

func gameOperationConnectionFailure(state SevenDaysToDieWebAPIConnectionState) string {
	switch state {
	case SevenDaysToDieWebAPIConnectionStateDashboardDisabled:
		return "The native dashboard is disabled."
	case SevenDaysToDieWebAPIConnectionStateAuthenticationDenied:
		return "The native dashboard rejected its configured credentials."
	case SevenDaysToDieWebAPIConnectionStateDiscoveryUnsupported:
		return "The native dashboard does not advertise the required game operation support."
	case SevenDaysToDieWebAPIConnectionStateMisconfigured, SevenDaysToDieWebAPIConnectionStateInvalidResponse:
		return "The native dashboard configuration or capability response is invalid."
	default:
		return "The native dashboard is unavailable."
	}
}

func failedGameOperation(message string) GameOperationResult {
	return GameOperationResult{Classification: GameOperationResultFailed, Message: message}
}

func failedGameOperationWithDetails(message string, details GameOperationTransportDetails) GameOperationResult {
	result := failedGameOperation(message)
	result.TransportDetails = details
	return result
}

func acceptedUnverifiedGameOperation(operationName string, details GameOperationTransportDetails) GameOperationResult {
	return GameOperationResult{
		Classification:   GameOperationResultAcceptedButUnverified,
		Message:          "The server accepted " + operationName + ", but the final state could not be verified.",
		TransportDetails: details,
	}
}
