package rpc

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const permissionPlayerManage = "game_server.players.manage"

// GetGameServerPlayerManagement returns the permission-gated current roster
// and the typed actions supported by the game and owning node.
func (xs *XylonaService) GetGameServerPlayerManagement(
	ctx context.Context,
	request *connect.Request[xylona.GetGameServerPlayerManagementRequest],
) (*connect.Response[xylona.GetGameServerPlayerManagementResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServerID := request.Msg.GetGameServerId()
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}
	gameServer, errLookup := xs.db.GetGameServerByID(gameServerID)
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionPlayerManage)
	if errPermission != nil {
		return nil, errPermission
	}
	if xs.actionsInst == nil {
		return nil, internalErr()
	}

	management, errManagement := xs.actionsInst.GetPlayerManagement(ctx, gameServer)
	if errManagement != nil {
		if errors.Is(errManagement, context.Canceled) || errors.Is(errManagement, context.DeadlineExceeded) {
			return nil, connect.NewError(contextConnectCode(errManagement), fmt.Errorf("get player management: %w", errManagement))
		}
		if errors.Is(errManagement, node.ErrPlayerActionUnavailable) {
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("player roster is unavailable"))
		}
		log.Warn().Err(errManagement).Str("game_server_id", gameServer.ID).Msg("Failed to get player management")
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("player management is unavailable"))
	}

	players := make([]*xylona.GameServerPlayer, 0, len(management.Players))
	managementPlayers := make([]*xylona.GameServerManagementPlayer, 0, len(management.Players))
	for _, player := range management.Players {
		legacyPlayer := &xylona.GameServerPlayer{Name: player.Name}
		if gameServer.GameID != sevenDaysToDieGameID && player.ActionID != "" {
			playerID := player.ActionID
			legacyPlayer.Id = &playerID
		}
		players = append(players, legacyPlayer)

		playerProto := &xylona.GameServerManagementPlayer{
			Name:             player.Name,
			ActionIdentifier: player.ActionID,
			EntityId:         player.EntityID,
			PlatformId:       player.PlatformID,
			CrossPlatformId:  player.CrossPlatformID,
			Online:           player.Online,
			Ping:             player.Ping,
			Level:            player.Level,
			Health:           player.Health,
			Stamina:          player.Stamina,
			Score:            player.Score,
			Deaths:           player.Deaths,
			ZombieKills:      player.ZombieKills,
			PlayerKills:      player.PlayerKills,
			Banned:           player.Banned,
		}
		managementPlayers = append(managementPlayers, playerProto)
	}
	supportedActions := make([]xylona.GameServerPlayerAction, 0, len(management.SupportedActions))
	for _, action := range management.SupportedActions {
		supportedActions = append(supportedActions, publicPlayerAction(action))
	}
	return connect.NewResponse(&xylona.GetGameServerPlayerManagementResponse{
		Capabilities: &xylona.GameServerPlayerManagementCapabilities{
			ActionsSupported:  management.ActionsSupported,
			UnavailableReason: management.UnavailableReason,
			IdentifierLabel:   management.IdentifierLabel,
			SupportedActions:  supportedActions,
			RosterState:       publicPlayerManagementRosterState(management.RosterState),
		},
		Players:           players,
		Status:            management.Status,
		ManagementPlayers: managementPlayers,
	}), nil
}

func publicPlayerManagementRosterState(state node.SevenDaysToDieWebAPIValueState) xylona.GameServerPlayerManagementRosterState {
	switch state {
	case node.SevenDaysToDieWebAPIValueStateAvailable:
		return xylona.GameServerPlayerManagementRosterState_GAME_SERVER_PLAYER_MANAGEMENT_ROSTER_STATE_AVAILABLE
	case node.SevenDaysToDieWebAPIValueStateUnsupported:
		return xylona.GameServerPlayerManagementRosterState_GAME_SERVER_PLAYER_MANAGEMENT_ROSTER_STATE_UNSUPPORTED
	case node.SevenDaysToDieWebAPIValueStatePermissionDenied:
		return xylona.GameServerPlayerManagementRosterState_GAME_SERVER_PLAYER_MANAGEMENT_ROSTER_STATE_PERMISSION_DENIED
	case node.SevenDaysToDieWebAPIValueStateUnavailable:
		return xylona.GameServerPlayerManagementRosterState_GAME_SERVER_PLAYER_MANAGEMENT_ROSTER_STATE_UNAVAILABLE
	default:
		return xylona.GameServerPlayerManagementRosterState_GAME_SERVER_PLAYER_MANAGEMENT_ROSTER_STATE_UNSPECIFIED
	}
}

// PerformGameServerPlayerAction executes one capability-gated typed action.
func (xs *XylonaService) PerformGameServerPlayerAction(
	ctx context.Context,
	request *connect.Request[xylona.PerformGameServerPlayerActionRequest],
) (*connect.Response[xylona.PerformGameServerPlayerActionResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServerID := request.Msg.GetGameServerId()
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}
	playerID := request.Msg.GetPlayerId()
	if playerID == "" {
		return nil, invalidArg("player_id is required")
	}
	action := nodePlayerAction(request.Msg.GetAction())
	if action == node.GameServerPlayerActionUnknown {
		return nil, invalidArg("action is required")
	}

	gameServer, errLookup := xs.db.GetGameServerByID(gameServerID)
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionPlayerManage)
	if errPermission != nil {
		return nil, errPermission
	}
	if xs.actionsInst == nil {
		return nil, internalErr()
	}

	errAction := xs.actionsInst.PerformPlayerAction(ctx, gameServer, action, playerID, request.Msg.GetReason())
	if errAction != nil {
		switch {
		case errors.Is(errAction, node.ErrInvalidPlayerAction):
			return nil, invalidArg("invalid player identifier or reason")
		case errors.Is(errAction, node.ErrPlayerActionUnsupported):
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("this player action is not supported by the game server"))
		case errors.Is(errAction, node.ErrProcessNotFound):
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("game server process is not running"))
		case errors.Is(errAction, node.ErrConsoleInputUnavailable):
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("game server console input is unavailable; retry shortly"))
		case errors.Is(errAction, node.ErrPlayerActionUnavailable):
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("the game server could not complete the player action"))
		case errors.Is(errAction, context.Canceled), errors.Is(errAction, context.DeadlineExceeded):
			return nil, connect.NewError(contextConnectCode(errAction), fmt.Errorf("perform player action: %w", errAction))
		}
		code := connect.CodeOf(errAction)
		if code == connect.CodeCanceled || code == connect.CodeDeadlineExceeded || code == connect.CodeUnavailable {
			return nil, connect.NewError(code, fmt.Errorf("perform player action: %w", errAction))
		}
		log.Error().Err(errAction).Str("game_server_id", gameServer.ID).Msg("Failed to perform player action")
		return nil, internalErr()
	}
	return connect.NewResponse(&xylona.PerformGameServerPlayerActionResponse{}), nil
}

func nodePlayerAction(action xylona.GameServerPlayerAction) node.GameServerPlayerAction {
	switch action {
	case xylona.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_KICK:
		return node.GameServerPlayerActionKick
	case xylona.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_BAN:
		return node.GameServerPlayerActionBan
	case xylona.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_UNBAN:
		return node.GameServerPlayerActionUnban
	case xylona.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_ALLOWLIST_ADD:
		return node.GameServerPlayerActionAllowlistAdd
	case xylona.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_ALLOWLIST_REMOVE:
		return node.GameServerPlayerActionAllowlistRemove
	default:
		return node.GameServerPlayerActionUnknown
	}
}

func publicPlayerAction(action node.GameServerPlayerAction) xylona.GameServerPlayerAction {
	switch action {
	case node.GameServerPlayerActionKick:
		return xylona.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_KICK
	case node.GameServerPlayerActionBan:
		return xylona.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_BAN
	case node.GameServerPlayerActionUnban:
		return xylona.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_UNBAN
	case node.GameServerPlayerActionAllowlistAdd:
		return xylona.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_ALLOWLIST_ADD
	case node.GameServerPlayerActionAllowlistRemove:
		return xylona.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_ALLOWLIST_REMOVE
	default:
		return xylona.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_UNSPECIFIED
	}
}
