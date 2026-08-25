package rpc

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// GetSevenDaysToDieSandboxSettings returns a read-only comparison of the saved
// SandboxCode and the effective settings reported by the running game.
func (xs *XylonaService) GetSevenDaysToDieSandboxSettings(
	ctx context.Context,
	request *connect.Request[xylona.GetSevenDaysToDieSandboxSettingsRequest],
) (*connect.Response[xylona.GetSevenDaysToDieSandboxSettingsResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errServer := xs.getGameServerFromID(strings.TrimSpace(request.Msg.GetGameServerId()))
	if errServer != nil {
		return nil, errServer
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, db.PermissionGameServerConfig)
	if errPermission != nil {
		return nil, errPermission
	}
	if gameServer.GameID != sevenDaysToDieGameID {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("sandbox settings are only available for 7 Days to Die servers"))
	}
	access, outcome, errAccess := xs.prepareSevenDaysToDiePrivateRead(ctx, gameServer)
	if errAccess != nil {
		return nil, errAccess
	}
	switch outcome {
	case sevenDaysToDiePrivateReadReady:
	case sevenDaysToDiePrivateReadNodeUnavailable, sevenDaysToDiePrivateReadRuntimeUnavailable:
		return sevenDaysToDieSandboxSettingsStateResponse(node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable), nil
	case sevenDaysToDiePrivateReadServerOffline:
		return sevenDaysToDieSandboxSettingsStateResponse(node.SevenDaysToDieWebAPIConnectionStateServerOffline), nil
	case sevenDaysToDiePrivateReadUnsupported:
		return connect.NewResponse(&xylona.GetSevenDaysToDieSandboxSettingsResponse{
			State: xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED,
		}), nil
	default:
		return nil, internalErrf("invalid 7 Days to Die private read outcome")
	}
	result, errQuery := access.client.QuerySevenDaysToDieSandboxSettings(ctx, node.SevenDaysToDieSandboxSettingsQueryRequest{
		WorkingDirectory: access.workingDirectory,
		TokenName:        access.tokenName,
		TokenSecret:      access.tokenSecret,
	})
	if errQuery != nil {
		if errors.Is(errQuery, context.Canceled) || errors.Is(errQuery, context.DeadlineExceeded) {
			return nil, connect.NewError(contextConnectCode(errQuery), errQuery)
		}
		return sevenDaysToDieSandboxSettingsStateResponse(node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable), nil
	}
	errValidate := node.ValidateSevenDaysToDieSandboxSettings(result)
	if errValidate != nil {
		return nil, internalErrf("node returned invalid 7 Days to Die sandbox settings")
	}

	settings := make([]*xylona.SevenDaysToDieSandboxSetting, 0, len(result.Settings))
	for _, setting := range result.Settings {
		settings = append(settings, &xylona.SevenDaysToDieSandboxSetting{
			Key:            setting.Key,
			Label:          setting.Label,
			Description:    setting.Description,
			Group:          setting.Group,
			EffectiveValue: setting.EffectiveValue,
			EffectiveLabel: setting.EffectiveLabel,
		})
	}
	response := &xylona.GetSevenDaysToDieSandboxSettingsResponse{
		ConnectionState: publicSevenDaysToDieWebAPIConnectionState(result.ConnectionState),
		State:           publicSevenDaysToDieWebAPIValueState(result.State),
		ComparisonState: publicSevenDaysToDieSandboxComparisonState(result.ComparisonState),
		ConfiguredCode:  result.ConfiguredCode,
		EffectiveCode:   result.EffectiveCode,
		Settings:        settings,
	}
	if !result.ObservedAt.IsZero() {
		response.ObservedAt = timestamppb.New(result.ObservedAt)
	}
	return connect.NewResponse(response), nil
}

func sevenDaysToDieSandboxSettingsStateResponse(state node.SevenDaysToDieWebAPIConnectionState) *connect.Response[xylona.GetSevenDaysToDieSandboxSettingsResponse] {
	return connect.NewResponse(&xylona.GetSevenDaysToDieSandboxSettingsResponse{
		ConnectionState: publicSevenDaysToDieWebAPIConnectionState(state),
		State:           xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
	})
}

func publicSevenDaysToDieSandboxComparisonState(state node.SevenDaysToDieSandboxComparisonState) xylona.SevenDaysToDieSandboxComparisonState {
	switch state {
	case node.SevenDaysToDieSandboxComparisonStateMatch:
		return xylona.SevenDaysToDieSandboxComparisonState_SEVEN_DAYS_TO_DIE_SANDBOX_COMPARISON_STATE_MATCH
	case node.SevenDaysToDieSandboxComparisonStateMismatch:
		return xylona.SevenDaysToDieSandboxComparisonState_SEVEN_DAYS_TO_DIE_SANDBOX_COMPARISON_STATE_MISMATCH
	case node.SevenDaysToDieSandboxComparisonStateStale:
		return xylona.SevenDaysToDieSandboxComparisonState_SEVEN_DAYS_TO_DIE_SANDBOX_COMPARISON_STATE_STALE
	default:
		return xylona.SevenDaysToDieSandboxComparisonState_SEVEN_DAYS_TO_DIE_SANDBOX_COMPARISON_STATE_UNSPECIFIED
	}
}
