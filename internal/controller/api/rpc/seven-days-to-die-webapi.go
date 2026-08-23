package rpc

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const sevenDaysToDieNonTacticalStatusNodeProtocol int64 = 10

// GetSevenDaysToDieWebAPIStatus returns bounded native WebAPI diagnostics for
// an authenticated 7 Days to Die game server.
func (xs *XylonaService) GetSevenDaysToDieWebAPIStatus(
	ctx context.Context,
	request *connect.Request[xylona.GetSevenDaysToDieWebAPIStatusRequest],
) (*connect.Response[xylona.GetSevenDaysToDieWebAPIStatusResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errServer := xs.getGameServerFromID(strings.TrimSpace(request.Msg.GetGameServerId()))
	if errServer != nil {
		return nil, errServer
	}

	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerView)
	if errPermission != nil {
		return nil, errPermission
	}
	if gameServer.GameID != sevenDaysToDieGameID {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("web API diagnostics are only available for 7 Days to Die servers"))
	}
	includeTactical := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerSettings) == nil

	if gameServer.Status == xylona.Status_OFFLINE.String() {
		return sevenDaysToDieWebAPIStateResponse(node.SevenDaysToDieWebAPIConnectionStateServerOffline, includeTactical), nil
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return sevenDaysToDieWebAPIStateResponse(node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable, includeTactical), nil //nolint:nilerr // Node reachability is a typed operational state.
	}
	process, found, errProcess := client.GetProcessSnapshot(ctx, gameServer.ID)
	if errProcess != nil {
		if errors.Is(errProcess, context.Canceled) || errors.Is(errProcess, context.DeadlineExceeded) {
			return nil, connect.NewError(contextConnectCode(errProcess), errProcess)
		}
		return sevenDaysToDieWebAPIStateResponse(node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable, includeTactical), nil
	}
	if !found || process == nil || process.Status != xylona.Status_ONLINE.String() {
		return sevenDaysToDieWebAPIStateResponse(node.SevenDaysToDieWebAPIConnectionStateServerOffline, includeTactical), nil
	}
	if !includeTactical {
		capabilities, errCapabilities := client.GetRuntimeCapabilities(ctx)
		if errCapabilities != nil {
			if errors.Is(errCapabilities, context.Canceled) || errors.Is(errCapabilities, context.DeadlineExceeded) {
				return nil, connect.NewError(contextConnectCode(errCapabilities), errCapabilities)
			}
			return sevenDaysToDieWebAPIStateResponse(node.SevenDaysToDieWebAPIConnectionStateUnspecified, false), nil
		}
		// Older nodes ignore the tactical query flag and fetch Blood Moon data.
		// Do not call their legacy status RPC for view-only requests.
		if capabilities.ProtocolVersion < sevenDaysToDieNonTacticalStatusNodeProtocol {
			return sevenDaysToDieWebAPIStateResponse(node.SevenDaysToDieWebAPIConnectionStateUnspecified, false), nil
		}
	}
	if xs.actionsInst == nil {
		return nil, internalErrf("7 Days to Die WebAPI credentials are unavailable")
	}

	tokenName, tokenSecret, errCredentials := xs.actionsInst.SevenDaysToDieMapCredentials(gameServer)
	if errCredentials != nil {
		return nil, internalErrf("failed to resolve 7 Days to Die WebAPI credentials")
	}

	status, errQuery := client.QuerySevenDaysToDieWebAPIStatus(ctx, node.SevenDaysToDieWebAPIStatusQueryRequest{
		WorkingDirectory: gameServer.Directory,
		TokenName:        tokenName,
		TokenSecret:      tokenSecret,
		IncludeTactical:  includeTactical,
	})
	if errQuery != nil {
		if errors.Is(errQuery, context.Canceled) || errors.Is(errQuery, context.DeadlineExceeded) {
			return nil, connect.NewError(contextConnectCode(errQuery), errQuery)
		}
		return sevenDaysToDieWebAPIStateResponse(node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable, includeTactical), nil
	}
	if status == nil {
		return nil, internalErrf("node returned an empty 7 Days to Die WebAPI status")
	}

	return connect.NewResponse(&xylona.GetSevenDaysToDieWebAPIStatusResponse{
		Status: publicSevenDaysToDieWebAPIStatus(status, includeTactical),
	}), nil
}

func sevenDaysToDieWebAPIStateResponse(
	state node.SevenDaysToDieWebAPIConnectionState,
	includeTactical bool,
) *connect.Response[xylona.GetSevenDaysToDieWebAPIStatusResponse] {
	status := &xylona.SevenDaysToDieWebAPIStatus{
		ConnectionState: publicSevenDaysToDieWebAPIConnectionState(state),
		Capabilities:    &xylona.SevenDaysToDieWebAPICapabilities{},
		WorldTimeState:  xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
	}
	if includeTactical {
		status.BloodMoonState = xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE
	}
	return connect.NewResponse(&xylona.GetSevenDaysToDieWebAPIStatusResponse{
		Status: status,
	})
}

func publicSevenDaysToDieWebAPIStatus(status *node.SevenDaysToDieWebAPIStatus, includeTactical bool) *xylona.SevenDaysToDieWebAPIStatus {
	result := &xylona.SevenDaysToDieWebAPIStatus{
		ConnectionState: publicSevenDaysToDieWebAPIConnectionState(status.ConnectionState),
		ApiVersion:      status.APIVersion,
		Capabilities: &xylona.SevenDaysToDieWebAPICapabilities{
			PlayerData:      status.Capabilities.PlayerData,
			RuntimeSettings: status.Capabilities.RuntimeSettings,
			NativeLog:       status.Capabilities.NativeLog,
			WorldPopulation: status.Capabilities.WorldPopulation,
			AccessControl:   status.Capabilities.AccessControl,
			GamePermissions: status.Capabilities.GamePermissions,
			ReportedMods:    status.Capabilities.ReportedMods,
		},
		WorldTimeState: publicSevenDaysToDieWebAPIValueState(status.WorldTimeState),
		WorldTime:      publicSevenDaysToDieGameTime(status.WorldTime),
	}
	if includeTactical {
		result.Capabilities.HostileAndAnimalPositions = status.Capabilities.HostileAndAnimalPositions
		result.Capabilities.HostilePositions = status.Capabilities.HostilePositions
		result.Capabilities.AnimalPositions = status.Capabilities.AnimalPositions
		result.BloodMoonState = publicSevenDaysToDieWebAPIValueState(status.BloodMoonState)
		result.NextBloodMoon = publicSevenDaysToDieGameTime(status.NextBloodMoon)
		result.NextBloodMoonEnd = publicSevenDaysToDieGameTime(status.NextBloodMoonEnd)
	}
	if includeTactical && status.BloodMoonActive != nil {
		active := *status.BloodMoonActive
		result.BloodMoonActive = &active
	}
	if !status.ObservedAt.IsZero() {
		result.ObservedAt = timestamppb.New(status.ObservedAt.UTC())
	}
	return result
}

func publicSevenDaysToDieGameTime(value *node.SevenDaysToDieGameTime) *xylona.SevenDaysToDieGameTime {
	if value == nil {
		return nil
	}
	return &xylona.SevenDaysToDieGameTime{Day: value.Day, Hour: value.Hour, Minute: value.Minute}
}

func publicSevenDaysToDieWebAPIConnectionState(state node.SevenDaysToDieWebAPIConnectionState) xylona.SevenDaysToDieWebAPIConnectionState {
	switch state {
	case node.SevenDaysToDieWebAPIConnectionStateAvailable:
		return xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE
	case node.SevenDaysToDieWebAPIConnectionStateServerOffline:
		return xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE
	case node.SevenDaysToDieWebAPIConnectionStateDashboardDisabled:
		return xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DASHBOARD_DISABLED
	case node.SevenDaysToDieWebAPIConnectionStateMisconfigured:
		return xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_MISCONFIGURED
	case node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable:
		return xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_NODE_UNAVAILABLE
	case node.SevenDaysToDieWebAPIConnectionStateUnreachable:
		return xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_WEB_API_UNREACHABLE
	case node.SevenDaysToDieWebAPIConnectionStateDiscoveryUnsupported:
		return xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DISCOVERY_UNSUPPORTED
	case node.SevenDaysToDieWebAPIConnectionStateAuthenticationDenied:
		return xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AUTHENTICATION_DENIED
	case node.SevenDaysToDieWebAPIConnectionStateInvalidResponse:
		return xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_INVALID_RESPONSE
	default:
		return xylona.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_UNSPECIFIED
	}
}

func publicSevenDaysToDieWebAPIValueState(state node.SevenDaysToDieWebAPIValueState) xylona.SevenDaysToDieWebAPIValueState {
	switch state {
	case node.SevenDaysToDieWebAPIValueStateAvailable:
		return xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE
	case node.SevenDaysToDieWebAPIValueStateUnsupported:
		return xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED
	case node.SevenDaysToDieWebAPIValueStatePermissionDenied:
		return xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED
	case node.SevenDaysToDieWebAPIValueStateUnavailable:
		return xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE
	default:
		return xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED
	}
}
