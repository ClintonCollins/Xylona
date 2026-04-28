package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/controller/readiness"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetGameServerReadiness returns public setup state for a game server.
func (xs *XylonaService) GetGameServerReadiness(
	ctx context.Context,
	request *connect.Request[xylona.GetGameServerReadinessRequest],
) (*connect.Response[xylona.GetGameServerReadinessResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.view")
	if errPermission != nil {
		return nil, errPermission
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}

	items, errItems := readiness.List(ctx, xs.db, gameServer, client)
	if errItems != nil {
		return nil, connect.NewError(connect.CodeInternal, errItems)
	}
	return connect.NewResponse(&xylona.GetGameServerReadinessResponse{
		Items: readinessItemsToProto(items),
	}), nil
}

// AcceptMinecraftEula accepts the Minecraft EULA for a game server.
func (xs *XylonaService) AcceptMinecraftEula(
	ctx context.Context,
	request *connect.Request[xylona.AcceptMinecraftEulaRequest],
) (*connect.Response[xylona.AcceptMinecraftEulaResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}
	if gameServer.GameID != "minecraft" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("game server is not a minecraft server"))
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}
	errAccept := readiness.AcceptMinecraftEULA(ctx, xs.db, gameServer, client, user.ID)
	if errAccept != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errAccept)
	}

	items, errItems := readiness.List(ctx, xs.db, gameServer, client)
	if errItems != nil {
		return nil, connect.NewError(connect.CodeInternal, errItems)
	}
	return connect.NewResponse(&xylona.AcceptMinecraftEulaResponse{
		Items: readinessItemsToProto(items),
	}), nil
}

// SetSteamGSLT stores a Steam Game Server Login Token for a game server.
func (xs *XylonaService) SetSteamGSLT(
	ctx context.Context,
	request *connect.Request[xylona.SetSteamGSLTRequest],
) (*connect.Response[xylona.SetSteamGSLTResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}
	if gameServer.R.Game == nil || !gameServer.R.Game.RequiresSteamGameServerLoginToken {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("game server does not require Steam GSLT"))
	}

	errSet := readiness.SetSteamGSLT(xs.db, gameServer.ID, request.Msg.GetToken(), user.ID)
	if errSet != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errSet)
	}

	items, errItems := readiness.List(ctx, xs.db, gameServer, nil)
	if errItems != nil {
		return nil, connect.NewError(connect.CodeInternal, errItems)
	}
	return connect.NewResponse(&xylona.SetSteamGSLTResponse{
		Items: readinessItemsToProto(items),
	}), nil
}

// ClearSteamGSLT removes a configured Steam Game Server Login Token.
func (xs *XylonaService) ClearSteamGSLT(
	ctx context.Context,
	request *connect.Request[xylona.ClearSteamGSLTRequest],
) (*connect.Response[xylona.ClearSteamGSLTResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}
	if gameServer.R.Game == nil || !gameServer.R.Game.RequiresSteamGameServerLoginToken {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("game server does not require Steam GSLT"))
	}

	errClear := readiness.ClearSteamGSLT(xs.db, gameServer.ID)
	if errClear != nil {
		return nil, connect.NewError(connect.CodeInternal, errClear)
	}

	items, errItems := readiness.List(ctx, xs.db, gameServer, nil)
	if errItems != nil {
		return nil, connect.NewError(connect.CodeInternal, errItems)
	}
	return connect.NewResponse(&xylona.ClearSteamGSLTResponse{
		Items: readinessItemsToProto(items),
	}), nil
}

// StartHytaleDeviceAuth starts the Hytale device authorization flow.
func (xs *XylonaService) StartHytaleDeviceAuth(
	ctx context.Context,
	request *connect.Request[xylona.StartHytaleDeviceAuthRequest],
) (*connect.Response[xylona.StartHytaleDeviceAuthResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}
	errHytale := ensureHytaleGameServer(gameServer)
	if errHytale != nil {
		return nil, errHytale
	}

	flowID, auth, expiresAt, errStart := xs.hytaleAuthManager().Start(ctx, gameServer.ID, user.ID)
	if errStart != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errStart)
	}
	return connect.NewResponse(&xylona.StartHytaleDeviceAuthResponse{
		FlowId:                  flowID,
		UserCode:                auth.UserCode,
		VerificationUri:         auth.VerificationURI,
		VerificationUriComplete: auth.VerificationURIComplete,
		ExpiresAtUnix:           expiresAt.Unix(),
		PollAfterSeconds:        auth.Interval,
	}), nil
}

// PollHytaleDeviceAuth polls the Hytale device authorization flow.
func (xs *XylonaService) PollHytaleDeviceAuth(
	ctx context.Context,
	request *connect.Request[xylona.PollHytaleDeviceAuthRequest],
) (*connect.Response[xylona.PollHytaleDeviceAuthResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	result, errPoll := xs.hytaleAuthManager().Poll(ctx, request.Msg.GetFlowId(), user.ID)
	if errPoll != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errPoll)
	}
	return connect.NewResponse(&xylona.PollHytaleDeviceAuthResponse{
		Status:           string(result.Status),
		Message:          result.Message,
		Profiles:         hytaleProfilesToProto(result.Profiles),
		PollAfterSeconds: result.PollAfterSeconds,
	}), nil
}

// SelectHytaleProfile links the selected Hytale profile to a server.
func (xs *XylonaService) SelectHytaleProfile(
	ctx context.Context,
	request *connect.Request[xylona.SelectHytaleProfileRequest],
) (*connect.Response[xylona.SelectHytaleProfileResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}
	errHytale := ensureHytaleGameServer(gameServer)
	if errHytale != nil {
		return nil, errHytale
	}

	_, errSelect := xs.hytaleAuthManager().SelectProfile(xs.db, request.Msg.GetFlowId(), gameServer.ID, user.ID, request.Msg.GetProfileUuid())
	if errSelect != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errSelect)
	}

	items, errItems := readiness.List(ctx, xs.db, gameServer, nil)
	if errItems != nil {
		return nil, connect.NewError(connect.CodeInternal, errItems)
	}
	return connect.NewResponse(&xylona.SelectHytaleProfileResponse{
		Items: readinessItemsToProto(items),
	}), nil
}

// ClearHytaleAccount removes the linked Hytale account for a server.
func (xs *XylonaService) ClearHytaleAccount(
	ctx context.Context,
	request *connect.Request[xylona.ClearHytaleAccountRequest],
) (*connect.Response[xylona.ClearHytaleAccountResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}
	errHytale := ensureHytaleGameServer(gameServer)
	if errHytale != nil {
		return nil, errHytale
	}

	errClear := readiness.ClearHytaleAccount(xs.db, gameServer.ID)
	if errClear != nil {
		return nil, connect.NewError(connect.CodeInternal, errClear)
	}

	items, errItems := readiness.List(ctx, xs.db, gameServer, nil)
	if errItems != nil {
		return nil, connect.NewError(connect.CodeInternal, errItems)
	}
	return connect.NewResponse(&xylona.ClearHytaleAccountResponse{
		Items: readinessItemsToProto(items),
	}), nil
}

func readinessItemsToProto(items []readiness.Item) []*xylona.GameServerReadinessItem {
	protoItems := make([]*xylona.GameServerReadinessItem, len(items))
	for i, item := range items {
		protoItems[i] = &xylona.GameServerReadinessItem{
			Kind:           item.Kind,
			Required:       item.Required,
			Complete:       item.Complete,
			Blocking:       item.Blocking,
			Message:        item.Message,
			PublicDataJson: item.PublicData,
		}
	}
	return protoItems
}

func hytaleProfilesToProto(profiles []readiness.HytaleProfile) []*xylona.HytaleProfile {
	protoProfiles := make([]*xylona.HytaleProfile, len(profiles))
	for i, profile := range profiles {
		protoProfiles[i] = &xylona.HytaleProfile{
			Uuid:     profile.UUID,
			Username: profile.Username,
		}
	}
	return protoProfiles
}

func ensureHytaleGameServer(gameServer *models.GameServer) error {
	if readiness.RequiresHytaleAccount(gameServer) {
		return nil
	}
	return connect.NewError(connect.CodeFailedPrecondition, errors.New("game server is not a hytale server"))
}
