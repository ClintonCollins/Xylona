package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
)

const addAdministratorOperationID = "player_access.add_administrator"

var gameOperationPlayerIdentityPattern = regexp.MustCompile(gameintegrations.PlayerIdentityPattern)

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

// ExecuteGameOperation validates and executes one integration-owned operation.
func (n *Node) ExecuteGameOperation(ctx context.Context, request GameOperationRequest) GameOperationResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if request.OperationID != addAdministratorOperationID {
		return failedGameOperation("Unknown game operation.")
	}

	values, validationFailure := validateAddAdministratorValues(request.Values)
	if validationFailure != "" {
		return failedGameOperation(validationFailure)
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
		sevenDaysToDieOpenAPIOperation{path: "/api/userpermissions/user/{id}", method: http.MethodPost},
		sevenDaysToDieOpenAPIOperation{path: "/api/userpermissions", method: http.MethodGet},
	)
	if !supported || discovery.resolver.failed {
		return failedGameOperationWithDetails("The node could not verify native Add administrator support.", details)
	}

	path := "/api/userpermissions/user/" + url.PathEscape(values.playerIdentity)
	statusCode, errPost := access.postWebAPIJSON(discovery.ctx, discovery.settings, path, struct {
		PermissionLevel int64 `json:"permissionLevel"`
	}{PermissionLevel: values.permissionLevel})
	if errPost != nil {
		return failedGameOperationWithDetails(gameOperationTransportFailure(ctx, errPost), details)
	}
	if statusCode != http.StatusCreated {
		return failedGameOperationWithDetails(fmt.Sprintf("The server rejected Add administrator (status %d).", statusCode), details)
	}

	statusCode, body, errReadBack := access.getWebAPI(discovery.ctx, discovery.settings, "/api/userpermissions")
	if errReadBack != nil || statusCode != http.StatusOK {
		return acceptedUnverifiedGameOperation(details)
	}
	var permissions sevenDaysToDieUserPermissionsEnvelope
	errDecode := json.Unmarshal(body, &permissions)
	if errDecode != nil {
		return acceptedUnverifiedGameOperation(details)
	}
	for _, user := range permissions.Data.Users {
		if user.UserID.CombinedString == values.playerIdentity && user.PermissionLevel == values.permissionLevel {
			return GameOperationResult{
				Classification:   GameOperationResultConfirmed,
				Message:          "Administrator access was confirmed by native user permission read-back.",
				TransportDetails: details,
			}
		}
	}
	return failedGameOperationWithDetails("The server did not report the requested administrator state.", details)
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
		return "The native dashboard does not advertise Add administrator support."
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

func acceptedUnverifiedGameOperation(details GameOperationTransportDetails) GameOperationResult {
	return GameOperationResult{
		Classification:   GameOperationResultAcceptedButUnverified,
		Message:          "The server accepted Add administrator, but the final state could not be verified.",
		TransportDetails: details,
	}
}
