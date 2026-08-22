package actions

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	minecraftGameID        = "minecraft"
	sevenDaysToDieGameID   = "7_days_to_die"
	factorioGameID         = "factorio"
	hytaleGameID           = "hytale"
	projectZomboidGameID   = "project_zomboid"
	terrariaGameID         = "terraria"
	counterStrikeTwoGameID = "counter_strike_2"
	garrysModGameID        = "garrys_mod"
	teamFortressTwoGameID  = "team_fortress_2"
	rustGameID             = "rust"

	expandedPlayerActionsProtocolVersion int64 = 4
	managedAdminInputUnavailableReason         = "This game definition does not configure the managed admin console required for player actions."
)

// PlayerManagement describes the roster, runtime state, and typed actions
// available for one game server.
type PlayerManagement struct {
	Players           []node.SevenDaysToDiePlayer
	RosterState       node.SevenDaysToDieWebAPIValueState
	ActionsSupported  bool
	UnavailableReason string
	IdentifierLabel   string
	SupportedActions  []node.GameServerPlayerAction
	Status            xylona.Status
}

type playerManagementProfile struct {
	queryKind         node.GameServerQueryKind
	actionKind        node.GameServerQueryKind
	identifierLabel   string
	supportedActions  []node.GameServerPlayerAction
	unavailableReason string
}

// GetPlayerManagement resolves node capabilities and actively queries the
// owning node for the current roster. Stable player IDs stay on this
// permission-gated path instead of entering the broad query WebSocket feed.
func (inst *Instance) GetPlayerManagement(ctx context.Context, gameServer *models.GameServer) (PlayerManagement, error) {
	profile := playerManagementProfileForServer(gameServer)
	management := PlayerManagement{
		Players:           make([]node.SevenDaysToDiePlayer, 0),
		UnavailableReason: profile.unavailableReason,
		IdentifierLabel:   profile.identifierLabel,
		SupportedActions:  append([]node.GameServerPlayerAction(nil), profile.supportedActions...),
		Status:            statusFromModel(gameServer),
	}
	if gameServer == nil {
		return management, errors.New("actions: game server is nil")
	}

	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		return management, fmt.Errorf("actions: resolve player-management node: %w", errClient)
	}
	management.Status = currentPlayerManagementStatus(ctx, client, gameServer)

	if len(profile.supportedActions) > 0 {
		if !gameServerDefinitionSupportsPlayerActionProfile(gameServer, profile) {
			management.UnavailableReason = managedAdminInputUnavailableReason
		} else {
			caps, errCaps := client.GetRuntimeCapabilities(ctx)
			switch {
			case errCaps != nil:
				management.UnavailableReason = "The target node's player-action capabilities are unavailable."
			case !caps.PlayerActions:
				management.UnavailableReason = "The target node does not support player actions. Upgrade the node to enable management."
			case !runtimeSupportsPlayerActionProfile(caps, profile):
				management.UnavailableReason = "The target node does not support this game's player actions. Upgrade the node to enable management."
			default:
				management.ActionsSupported = true
				management.UnavailableReason = ""
			}
		}
	}

	if gameServer.GameID == sevenDaysToDieGameID {
		if management.Status != xylona.Status_ONLINE {
			management.RosterState = node.SevenDaysToDieWebAPIValueStateUnavailable
			return management, nil
		}
		tokenName, tokenSecret, errCredentials := inst.SevenDaysToDieMapCredentials(gameServer)
		if errCredentials != nil {
			management.RosterState = node.SevenDaysToDieWebAPIValueStateUnavailable
			return management, nil //nolint:nilerr // Credential failure is represented as an unavailable roster.
		}
		result, errQuery := client.QuerySevenDaysToDiePlayers(ctx, node.SevenDaysToDiePlayersQueryRequest{
			WorkingDirectory: gameServer.Directory,
			TokenName:        tokenName,
			TokenSecret:      tokenSecret,
		})
		if errQuery != nil {
			if errors.Is(errQuery, context.Canceled) || errors.Is(errQuery, context.DeadlineExceeded) {
				return management, fmt.Errorf("actions: query native player roster: %w", errQuery)
			}
			management.RosterState = node.SevenDaysToDieWebAPIValueStateUnavailable
			return management, nil
		}
		if result == nil {
			management.RosterState = node.SevenDaysToDieWebAPIValueStateUnavailable
			return management, nil
		}
		management.RosterState = result.State
		if result.ConnectionState == node.SevenDaysToDieWebAPIConnectionStateAuthenticationDenied {
			management.RosterState = node.SevenDaysToDieWebAPIValueStatePermissionDenied
		}
		management.Players = append(management.Players, result.Players...)
		return management, nil
	}

	queryRequest := node.GameServerQueryRequest{
		Kind:       profile.queryKind,
		IP:         gameServer.IP,
		QueryPort:  gameServerQueryPort(gameServer),
		MaxPlayers: gameServer.MaxPlayers,
	}
	if profile.queryKind == node.GameServerQueryKindUnknown || management.Status != xylona.Status_ONLINE {
		return management, nil
	}
	if profile.queryKind == node.GameServerQueryKindPalworld {
		username, password, errCredentials := inst.palworldQueryCredentials(gameServer)
		if errCredentials != nil {
			management.ActionsSupported = false
			management.UnavailableReason = "Palworld REST API credentials are not configured for this server."
			return management, nil //nolint:nilerr // Missing credentials are represented as a read-only capability state.
		}
		queryRequest.Username = username
		queryRequest.Password = password
	}

	result, errQuery := client.QueryGameServer(ctx, queryRequest)
	if errQuery != nil {
		return management, errors.Join(
			node.ErrPlayerActionUnavailable,
			fmt.Errorf("actions: query player roster: %w", errQuery),
		)
	}
	management.Players = playersFromQueryResult(result)
	return management, nil
}

// PerformPlayerAction verifies that the target game and node advertise the
// requested typed action, then delegates execution to the owning node.
func (inst *Instance) PerformPlayerAction(
	ctx context.Context,
	gameServer *models.GameServer,
	action node.GameServerPlayerAction,
	playerID string,
	reason string,
) error {
	if gameServer == nil {
		return errors.New("actions: game server is nil")
	}
	profile := playerManagementProfileForServer(gameServer)
	if !profileSupportsAction(profile, action) {
		return node.ErrPlayerActionUnsupported
	}
	if !gameServerDefinitionSupportsPlayerActionProfile(gameServer, profile) {
		return node.ErrPlayerActionUnsupported
	}

	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		return fmt.Errorf("actions: resolve player-management node: %w", errClient)
	}
	caps, errCaps := client.GetRuntimeCapabilities(ctx)
	if errCaps != nil {
		return errors.Join(
			node.ErrPlayerActionUnavailable,
			fmt.Errorf("actions: get player-action capabilities: %w", errCaps),
		)
	}
	if !caps.PlayerActions {
		return node.ErrPlayerActionUnsupported
	}
	if !runtimeSupportsPlayerActionProfile(caps, profile) {
		return node.ErrPlayerActionUnsupported
	}

	actionRequest := node.GameServerPlayerActionRequest{
		Kind:      profile.actionKind,
		Action:    action,
		ProcessID: gameServer.ID,
		IP:        gameServer.IP,
		QueryPort: gameServerQueryPort(gameServer),
		PlayerID:  strings.TrimSpace(playerID),
		Reason:    strings.TrimSpace(reason),
	}
	if profile.actionKind == node.GameServerQueryKindPalworld {
		username, password, errCredentials := inst.palworldQueryCredentials(gameServer)
		if errCredentials != nil {
			return errors.Join(node.ErrPlayerActionUnavailable, errCredentials)
		}
		actionRequest.Username = username
		actionRequest.Password = password
	}

	errAction := client.PerformGameServerPlayerAction(ctx, actionRequest)
	if errAction != nil {
		return fmt.Errorf("actions: perform player action: %w", errAction)
	}
	return nil
}

func runtimeSupportsPlayerActionProfile(
	caps node.RuntimeCapabilities,
	profile playerManagementProfile,
) bool {
	switch profile.actionKind {
	case node.GameServerQueryKindMinecraft, node.GameServerQueryKindPalworld:
		return true
	default:
		return caps.ProtocolVersion >= expandedPlayerActionsProtocolVersion
	}
}

func gameServerDefinitionSupportsPlayerActionProfile(
	gameServer *models.GameServer,
	profile playerManagementProfile,
) bool {
	switch profile.actionKind {
	case node.GameServerQueryKindFactorio, node.GameServerQueryKindSourceRCON, node.GameServerQueryKindRust:
		return GameServerDefinitionSupportsAdminInput(gameServer)
	default:
		return true
	}
}

func playerManagementProfileForServer(gameServer *models.GameServer) playerManagementProfile {
	if gameServer == nil {
		return playerManagementProfile{unavailableReason: "Player management is not supported for this game."}
	}
	switch gameServer.GameID {
	case minecraftGameID:
		return playerManagementProfile{
			queryKind:       node.GameServerQueryKindMinecraft,
			actionKind:      node.GameServerQueryKindMinecraft,
			identifierLabel: "Player name",
			supportedActions: []node.GameServerPlayerAction{
				node.GameServerPlayerActionKick,
				node.GameServerPlayerActionBan,
				node.GameServerPlayerActionUnban,
				node.GameServerPlayerActionAllowlistAdd,
				node.GameServerPlayerActionAllowlistRemove,
			},
		}
	case palworldGameID:
		return playerManagementProfile{
			queryKind:       node.GameServerQueryKindPalworld,
			actionKind:      node.GameServerQueryKindPalworld,
			identifierLabel: "User ID",
			supportedActions: []node.GameServerPlayerAction{
				node.GameServerPlayerActionKick,
				node.GameServerPlayerActionBan,
				node.GameServerPlayerActionUnban,
			},
		}
	case sevenDaysToDieGameID:
		return playerManagementProfile{
			actionKind:      node.GameServerQueryKindSevenDaysToDie,
			identifierLabel: "Platform, cross-platform, or entity ID",
			supportedActions: []node.GameServerPlayerAction{
				node.GameServerPlayerActionKick,
				node.GameServerPlayerActionBan,
				node.GameServerPlayerActionUnban,
				node.GameServerPlayerActionAllowlistAdd,
				node.GameServerPlayerActionAllowlistRemove,
			},
		}
	case factorioGameID:
		return playerManagementProfile{
			actionKind:      node.GameServerQueryKindFactorio,
			identifierLabel: "Factorio player name",
			supportedActions: []node.GameServerPlayerAction{
				node.GameServerPlayerActionKick,
				node.GameServerPlayerActionBan,
				node.GameServerPlayerActionUnban,
				node.GameServerPlayerActionAllowlistAdd,
				node.GameServerPlayerActionAllowlistRemove,
			},
		}
	case hytaleGameID:
		return playerManagementProfile{
			actionKind:      node.GameServerQueryKindHytale,
			identifierLabel: "Player name",
			supportedActions: []node.GameServerPlayerAction{
				node.GameServerPlayerActionKick,
				node.GameServerPlayerActionBan,
				node.GameServerPlayerActionUnban,
				node.GameServerPlayerActionAllowlistAdd,
				node.GameServerPlayerActionAllowlistRemove,
			},
		}
	case projectZomboidGameID:
		return playerManagementProfile{
			queryKind:       node.GameServerQueryKindSource,
			actionKind:      node.GameServerQueryKindProjectZomboid,
			identifierLabel: "Project Zomboid username",
			supportedActions: []node.GameServerPlayerAction{
				node.GameServerPlayerActionKick,
				node.GameServerPlayerActionBan,
				node.GameServerPlayerActionUnban,
				node.GameServerPlayerActionAllowlistRemove,
			},
		}
	case terrariaGameID:
		return playerManagementProfile{
			actionKind:      node.GameServerQueryKindTerraria,
			identifierLabel: "Player name",
			supportedActions: []node.GameServerPlayerAction{
				node.GameServerPlayerActionKick,
				node.GameServerPlayerActionBan,
			},
		}
	case counterStrikeTwoGameID, garrysModGameID, teamFortressTwoGameID:
		return playerManagementProfile{
			queryKind:       node.GameServerQueryKindSource,
			actionKind:      node.GameServerQueryKindSourceRCON,
			identifierLabel: "Steam ID or server user ID",
			supportedActions: []node.GameServerPlayerAction{
				node.GameServerPlayerActionKick,
				node.GameServerPlayerActionBan,
				node.GameServerPlayerActionUnban,
			},
		}
	case rustGameID:
		return playerManagementProfile{
			queryKind:       node.GameServerQueryKindSource,
			actionKind:      node.GameServerQueryKindRust,
			identifierLabel: "Steam64 ID",
			supportedActions: []node.GameServerPlayerAction{
				node.GameServerPlayerActionKick,
				node.GameServerPlayerActionBan,
				node.GameServerPlayerActionUnban,
			},
		}
	}
	if gameServer.R.Game != nil && gameServer.R.Game.UsesSourceQuery {
		return playerManagementProfile{
			queryKind:         node.GameServerQueryKindSource,
			unavailableReason: "This game exposes a read-only player roster, but not a stable identifier for safe player actions.",
		}
	}
	return playerManagementProfile{unavailableReason: "Player management is not supported for this game."}
}

func profileSupportsAction(profile playerManagementProfile, action node.GameServerPlayerAction) bool {
	return slices.Contains(profile.supportedActions, action)
}

func currentPlayerManagementStatus(ctx context.Context, client interface {
	GetProcessSnapshot(context.Context, string) (*node.ProcessSnapshot, bool, error)
}, gameServer *models.GameServer) xylona.Status {
	snapshot, found, errSnapshot := client.GetProcessSnapshot(ctx, gameServer.ID)
	if errSnapshot != nil {
		return statusFromModel(gameServer)
	}
	if !found || snapshot == nil {
		return xylona.Status_OFFLINE
	}
	statusValue, known := xylona.Status_value[snapshot.Status]
	if !known {
		return xylona.Status_UNKNOWN
	}
	return xylona.Status(statusValue)
}

func statusFromModel(gameServer *models.GameServer) xylona.Status {
	if gameServer == nil {
		return xylona.Status_UNKNOWN
	}
	statusValue, known := xylona.Status_value[gameServer.Status]
	if !known {
		return xylona.Status_UNKNOWN
	}
	return xylona.Status(statusValue)
}

func playersFromQueryResult(result node.GameServerQueryResult) []node.SevenDaysToDiePlayer {
	switch result.Kind {
	case node.GameServerQueryKindMinecraft:
		if result.Minecraft == nil {
			return nil
		}
		if len(result.Minecraft.PlayerDetails) > 0 {
			return managementPlayers(result.Minecraft.PlayerDetails)
		}
		return playerNamesToDetails(result.Minecraft.PlayerList, true)
	case node.GameServerQueryKindSource:
		if result.Source == nil {
			return nil
		}
		return playerNamesToDetails(result.Source.PlayerList, false)
	case node.GameServerQueryKindPalworld:
		if result.Palworld == nil {
			return nil
		}
		if len(result.Palworld.PlayerDetails) > 0 {
			return managementPlayers(result.Palworld.PlayerDetails)
		}
		return playerNamesToDetails(result.Palworld.PlayerList, false)
	default:
		return nil
	}
}

func playerNamesToDetails(names []string, useNameAsID bool) []node.SevenDaysToDiePlayer {
	players := make([]node.SevenDaysToDiePlayer, 0, len(names))
	for _, name := range names {
		player := node.SevenDaysToDiePlayer{Name: name}
		if useNameAsID {
			player.ActionID = name
		}
		players = append(players, player)
	}
	return players
}

func managementPlayers(players []node.GameServerPlayer) []node.SevenDaysToDiePlayer {
	result := make([]node.SevenDaysToDiePlayer, 0, len(players))
	for _, player := range players {
		result = append(result, node.SevenDaysToDiePlayer{Name: player.Name, ActionID: player.ID})
	}
	return result
}
