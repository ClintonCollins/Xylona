package rpc

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// GetSevenDaysToDieReportedMods returns the read-only mod inventory reported by
// an authenticated 7 Days to Die game server.
func (xs *XylonaService) GetSevenDaysToDieReportedMods(
	ctx context.Context,
	request *connect.Request[xylona.GetSevenDaysToDieReportedModsRequest],
) (*connect.Response[xylona.GetSevenDaysToDieReportedModsResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errServer := xs.getGameServerFromID(strings.TrimSpace(request.Msg.GetGameServerId()))
	if errServer != nil {
		return nil, errServer
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, PermissionGameServerMods)
	if errPermission != nil {
		return nil, errPermission
	}
	if gameServer.GameID != sevenDaysToDieGameID {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("reported mods are only available for 7 Days to Die servers"))
	}
	if gameServer.Status == xylona.Status_OFFLINE.String() {
		return sevenDaysToDieReportedModsStateResponse(node.SevenDaysToDieWebAPIConnectionStateServerOffline), nil
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return sevenDaysToDieReportedModsStateResponse(node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable), nil //nolint:nilerr // Node reachability is a typed operational state.
	}
	process, found, errProcess := client.GetProcessSnapshot(ctx, gameServer.ID)
	if errProcess != nil {
		if errors.Is(errProcess, context.Canceled) || errors.Is(errProcess, context.DeadlineExceeded) {
			return nil, connect.NewError(contextConnectCode(errProcess), errProcess)
		}
		return sevenDaysToDieReportedModsStateResponse(node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable), nil
	}
	if !found || process == nil || process.Status != xylona.Status_ONLINE.String() {
		return sevenDaysToDieReportedModsStateResponse(node.SevenDaysToDieWebAPIConnectionStateServerOffline), nil
	}
	capabilities, errCapabilities := client.GetRuntimeCapabilities(ctx)
	if errCapabilities != nil {
		if errors.Is(errCapabilities, context.Canceled) || errors.Is(errCapabilities, context.DeadlineExceeded) {
			return nil, connect.NewError(contextConnectCode(errCapabilities), errCapabilities)
		}
		return sevenDaysToDieReportedModsStateResponse(node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable), nil
	}
	if capabilities.ProtocolVersion < sevenDaysToDiePrivateWebAPINodeProtocol {
		return connect.NewResponse(&xylona.GetSevenDaysToDieReportedModsResponse{
			State: xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED,
		}), nil
	}
	if xs.actionsInst == nil {
		return nil, internalErrf("7 Days to Die WebAPI credentials are unavailable")
	}

	tokenName, tokenSecret, errCredentials := xs.actionsInst.SevenDaysToDieMapCredentials(gameServer)
	if errCredentials != nil {
		return nil, internalErrf("failed to resolve 7 Days to Die WebAPI credentials")
	}
	reportedMods, errQuery := client.QuerySevenDaysToDieReportedMods(ctx, node.SevenDaysToDieReportedModsQueryRequest{
		WorkingDirectory: gameServer.Directory,
		TokenName:        tokenName,
		TokenSecret:      tokenSecret,
	})
	if errQuery != nil {
		if errors.Is(errQuery, context.Canceled) || errors.Is(errQuery, context.DeadlineExceeded) {
			return nil, connect.NewError(contextConnectCode(errQuery), errQuery)
		}
		return sevenDaysToDieReportedModsStateResponse(node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable), nil
	}
	if reportedMods == nil {
		return nil, internalErrf("node returned an empty 7 Days to Die reported mod result")
	}
	errValidate := node.ValidateSevenDaysToDieReportedMods(reportedMods.Mods)
	if errValidate != nil {
		return nil, internalErrf("node returned an invalid 7 Days to Die reported mod result")
	}

	mods := make([]*xylona.SevenDaysToDieReportedMod, 0, len(reportedMods.Mods))
	for _, reportedMod := range reportedMods.Mods {
		mods = append(mods, &xylona.SevenDaysToDieReportedMod{
			Name:        reportedMod.Name,
			DisplayName: reportedMod.DisplayName,
			Description: reportedMod.Description,
			Author:      reportedMod.Author,
			Version:     reportedMod.Version,
		})
	}

	return connect.NewResponse(&xylona.GetSevenDaysToDieReportedModsResponse{
		ConnectionState: publicSevenDaysToDieWebAPIConnectionState(reportedMods.ConnectionState),
		State:           publicSevenDaysToDieWebAPIValueState(reportedMods.State),
		Mods:            mods,
	}), nil
}

func sevenDaysToDieReportedModsStateResponse(state node.SevenDaysToDieWebAPIConnectionState) *connect.Response[xylona.GetSevenDaysToDieReportedModsResponse] {
	return connect.NewResponse(&xylona.GetSevenDaysToDieReportedModsResponse{
		ConnectionState: publicSevenDaysToDieWebAPIConnectionState(state),
		State:           xylona.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
	})
}
