package node

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ClintonCollins/Xylona/pkg/query"
)

const (
	maxPlayerActionIdentifierRunes = 256
	maxPlayerActionReasonRunes     = 256
)

var (
	minecraftPlayerNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,16}$`)
	simplePlayerNamePattern    = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	sourcePlayerIDPattern      = regexp.MustCompile(`^(?:[0-9]+|STEAM_[0-5]:[01]:[0-9]+|\[U:1:[0-9]+\])$`)
	steam64IDPattern           = regexp.MustCompile(`^[0-9]{17}$`)
)

// PerformGameServerPlayerAction executes a typed game-specific player action.
// Raw command text and REST endpoint paths are selected locally so untrusted
// player input cannot change the operation being performed.
func (n *Node) PerformGameServerPlayerAction(ctx context.Context, req GameServerPlayerActionRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	errCtx := ctx.Err()
	if errCtx != nil {
		return fmt.Errorf("node: player action canceled: %w", errCtx)
	}

	reason, errReason := normalizePlayerActionReason(req.Reason)
	if errReason != nil {
		return errReason
	}

	switch req.Kind {
	case GameServerQueryKindMinecraft:
		command, errCommand := minecraftPlayerActionCommand(req.Action, req.PlayerID, reason)
		if errCommand != nil {
			return errCommand
		}
		return n.sendConsolePlayerAction(ctx, req.ProcessID, "Minecraft", command)
	case GameServerQueryKindPalworld:
		return performPalworldPlayerAction(ctx, req, reason)
	case GameServerQueryKindSevenDaysToDie:
		command, errCommand := sevenDaysToDiePlayerActionCommand(req.Action, req.PlayerID, reason)
		if errCommand != nil {
			return errCommand
		}
		return n.sendConsolePlayerAction(ctx, req.ProcessID, "7 Days to Die", command)
	case GameServerQueryKindFactorio:
		command, errCommand := factorioPlayerActionCommand(req.Action, req.PlayerID, reason)
		if errCommand != nil {
			return errCommand
		}
		return n.sendConsolePlayerAction(ctx, req.ProcessID, "Factorio", command)
	case GameServerQueryKindHytale:
		command, errCommand := hytalePlayerActionCommand(req.Action, req.PlayerID, reason)
		if errCommand != nil {
			return errCommand
		}
		return n.sendConsolePlayerAction(ctx, req.ProcessID, "Hytale", command)
	case GameServerQueryKindProjectZomboid:
		command, errCommand := projectZomboidPlayerActionCommand(req.Action, req.PlayerID, reason)
		if errCommand != nil {
			return errCommand
		}
		return n.sendConsolePlayerAction(ctx, req.ProcessID, "Project Zomboid", command)
	case GameServerQueryKindTerraria:
		command, errCommand := terrariaPlayerActionCommand(req.Action, req.PlayerID)
		if errCommand != nil {
			return errCommand
		}
		return n.sendConsolePlayerAction(ctx, req.ProcessID, "Terraria", command)
	case GameServerQueryKindSourceRCON:
		command, errCommand := sourcePlayerActionCommand(req.Action, req.PlayerID, reason)
		if errCommand != nil {
			return errCommand
		}
		return n.sendConsolePlayerAction(ctx, req.ProcessID, "Source RCON", command)
	case GameServerQueryKindRust:
		command, errCommand := rustPlayerActionCommand(req.Action, req.PlayerID, reason)
		if errCommand != nil {
			return errCommand
		}
		return n.sendConsolePlayerAction(ctx, req.ProcessID, "Rust", command)
	default:
		return ErrInvalidPlayerAction
	}
}

func (n *Node) sendConsolePlayerAction(ctx context.Context, processID string, gameName string, command string) error {
	errSend := n.SendConsoleInputContext(ctx, processID, command)
	if errSend != nil {
		return fmt.Errorf("node: perform %s player action: %w", gameName, errSend)
	}
	return nil
}

func minecraftPlayerActionCommand(action GameServerPlayerAction, playerID string, reason string) (string, error) {
	playerID = strings.TrimSpace(playerID)
	if !minecraftPlayerNamePattern.MatchString(playerID) {
		return "", fmt.Errorf("%w: invalid Minecraft player name", ErrInvalidPlayerAction)
	}

	var command string
	switch action {
	case GameServerPlayerActionKick:
		command = "kick " + playerID
	case GameServerPlayerActionBan:
		command = "ban " + playerID
	case GameServerPlayerActionUnban:
		command = "pardon " + playerID
	case GameServerPlayerActionAllowlistAdd:
		command = "whitelist add " + playerID
	case GameServerPlayerActionAllowlistRemove:
		command = "whitelist remove " + playerID
	default:
		return "", ErrInvalidPlayerAction
	}
	if reason != "" && (action == GameServerPlayerActionKick || action == GameServerPlayerActionBan) {
		command += " " + reason
	}
	return command, nil
}

func sevenDaysToDiePlayerActionCommand(action GameServerPlayerAction, playerID string, reason string) (string, error) {
	identifier, errIdentifier := quotedPlayerIdentifier(playerID, "7 Days to Die")
	if errIdentifier != nil {
		return "", errIdentifier
	}
	switch action {
	case GameServerPlayerActionKick:
		return appendQuotedReason("kick "+identifier, reason)
	case GameServerPlayerActionBan:
		if reason == "" {
			reason = "Banned by Xylona"
		}
		return appendQuotedReason("ban add "+identifier+" 0 minutes", reason)
	case GameServerPlayerActionUnban:
		return "ban remove " + identifier, nil
	case GameServerPlayerActionAllowlistAdd:
		return "whitelist add " + identifier, nil
	case GameServerPlayerActionAllowlistRemove:
		return "whitelist remove " + identifier, nil
	default:
		return "", ErrInvalidPlayerAction
	}
}

func factorioPlayerActionCommand(action GameServerPlayerAction, playerID string, reason string) (string, error) {
	playerID = strings.TrimSpace(playerID)
	if !simplePlayerNamePattern.MatchString(playerID) {
		return "", fmt.Errorf("%w: invalid Factorio player name", ErrInvalidPlayerAction)
	}
	switch action {
	case GameServerPlayerActionKick:
		return appendUnquotedReason("/kick "+playerID, reason), nil
	case GameServerPlayerActionBan:
		return appendUnquotedReason("/ban "+playerID, reason), nil
	case GameServerPlayerActionUnban:
		return "/unban " + playerID, nil
	case GameServerPlayerActionAllowlistAdd:
		return "/whitelist add " + playerID, nil
	case GameServerPlayerActionAllowlistRemove:
		return "/whitelist remove " + playerID, nil
	default:
		return "", ErrInvalidPlayerAction
	}
}

func hytalePlayerActionCommand(action GameServerPlayerAction, playerID string, reason string) (string, error) {
	playerID = strings.TrimSpace(playerID)
	if !simplePlayerNamePattern.MatchString(playerID) {
		return "", fmt.Errorf("%w: invalid Hytale player name", ErrInvalidPlayerAction)
	}
	switch action {
	case GameServerPlayerActionKick:
		return appendUnquotedReason("kick "+playerID, reason), nil
	case GameServerPlayerActionBan:
		return appendUnquotedReason("ban "+playerID, reason), nil
	case GameServerPlayerActionUnban:
		return "unban " + playerID, nil
	case GameServerPlayerActionAllowlistAdd:
		return "whitelist add " + playerID, nil
	case GameServerPlayerActionAllowlistRemove:
		return "whitelist remove " + playerID, nil
	default:
		return "", ErrInvalidPlayerAction
	}
}

func projectZomboidPlayerActionCommand(action GameServerPlayerAction, playerID string, reason string) (string, error) {
	identifier, errIdentifier := quotedPlayerIdentifier(playerID, "Project Zomboid")
	if errIdentifier != nil {
		return "", errIdentifier
	}
	switch action {
	case GameServerPlayerActionKick:
		return appendFlaggedReason("kickuser "+identifier, reason)
	case GameServerPlayerActionBan:
		return appendFlaggedReason("banuser "+identifier, reason)
	case GameServerPlayerActionUnban:
		return "unbanuser " + identifier, nil
	case GameServerPlayerActionAllowlistRemove:
		return "removeuserfromwhitelist " + identifier, nil
	case GameServerPlayerActionAllowlistAdd:
		return "", ErrPlayerActionUnsupported
	default:
		return "", ErrInvalidPlayerAction
	}
}

func terrariaPlayerActionCommand(action GameServerPlayerAction, playerID string) (string, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" || utf8.RuneCountInString(playerID) > maxPlayerActionIdentifierRunes ||
		strings.ContainsFunc(playerID, unicode.IsControl) {
		return "", fmt.Errorf("%w: invalid Terraria player name", ErrInvalidPlayerAction)
	}
	switch action {
	case GameServerPlayerActionKick:
		return "kick " + playerID, nil
	case GameServerPlayerActionBan:
		return "ban " + playerID, nil
	case GameServerPlayerActionUnban, GameServerPlayerActionAllowlistAdd, GameServerPlayerActionAllowlistRemove:
		return "", ErrPlayerActionUnsupported
	default:
		return "", ErrInvalidPlayerAction
	}
}

func sourcePlayerActionCommand(action GameServerPlayerAction, playerID string, reason string) (string, error) {
	playerID = strings.TrimSpace(playerID)
	if !sourcePlayerIDPattern.MatchString(playerID) {
		return "", fmt.Errorf("%w: invalid Source player identifier", ErrInvalidPlayerAction)
	}
	switch action {
	case GameServerPlayerActionKick:
		return appendQuotedReason("kickid "+playerID, reason)
	case GameServerPlayerActionBan:
		return "banid 0 " + playerID + "; writeid", nil
	case GameServerPlayerActionUnban:
		return "removeid " + playerID + "; writeid", nil
	case GameServerPlayerActionAllowlistAdd, GameServerPlayerActionAllowlistRemove:
		return "", ErrPlayerActionUnsupported
	default:
		return "", ErrInvalidPlayerAction
	}
}

func rustPlayerActionCommand(action GameServerPlayerAction, playerID string, reason string) (string, error) {
	playerID = strings.TrimSpace(playerID)
	switch action {
	case GameServerPlayerActionKick:
		identifier, errIdentifier := quotedPlayerIdentifier(playerID, "Rust")
		if errIdentifier != nil {
			return "", errIdentifier
		}
		return appendQuotedReason("kick "+identifier, reason)
	case GameServerPlayerActionBan:
		if !steam64IDPattern.MatchString(playerID) {
			return "", fmt.Errorf("%w: Rust bans require a Steam64 ID", ErrInvalidPlayerAction)
		}
		if reason == "" {
			reason = "Banned by Xylona"
		}
		quotedReason, errReason := quoteCommandArgument(reason)
		if errReason != nil {
			return "", errReason
		}
		return "banid " + playerID + " " + playerID + " " + quotedReason, nil
	case GameServerPlayerActionUnban:
		if !steam64IDPattern.MatchString(playerID) {
			return "", fmt.Errorf("%w: Rust unbans require a Steam64 ID", ErrInvalidPlayerAction)
		}
		return "unban " + playerID, nil
	case GameServerPlayerActionAllowlistAdd, GameServerPlayerActionAllowlistRemove:
		return "", ErrPlayerActionUnsupported
	default:
		return "", ErrInvalidPlayerAction
	}
}

func quotedPlayerIdentifier(playerID string, gameName string) (string, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" || utf8.RuneCountInString(playerID) > maxPlayerActionIdentifierRunes {
		return "", fmt.Errorf("%w: invalid %s player identifier", ErrInvalidPlayerAction, gameName)
	}
	quoted, errQuote := quoteCommandArgument(playerID)
	if errQuote != nil {
		return "", fmt.Errorf("%w: invalid %s player identifier", ErrInvalidPlayerAction, gameName)
	}
	return quoted, nil
}

func quoteCommandArgument(value string) (string, error) {
	if strings.ContainsRune(value, rune(34)) || strings.ContainsRune(value, rune(92)) ||
		strings.ContainsFunc(value, unicode.IsControl) {
		return "", fmt.Errorf("%w: command argument contains unsupported characters", ErrInvalidPlayerAction)
	}
	return string(rune(34)) + value + string(rune(34)), nil
}

func appendQuotedReason(command string, reason string) (string, error) {
	if reason == "" {
		return command, nil
	}
	quoted, errQuote := quoteCommandArgument(reason)
	if errQuote != nil {
		return "", errQuote
	}
	return command + " " + quoted, nil
}

func appendFlaggedReason(command string, reason string) (string, error) {
	if reason == "" {
		return command, nil
	}
	quoted, errQuote := quoteCommandArgument(reason)
	if errQuote != nil {
		return "", errQuote
	}
	return command + " -r " + quoted, nil
}

func appendUnquotedReason(command string, reason string) string {
	if reason == "" {
		return command
	}
	return command + " " + reason
}

func performPalworldPlayerAction(ctx context.Context, req GameServerPlayerActionRequest, reason string) error {
	playerID := strings.TrimSpace(req.PlayerID)
	if playerID == "" || utf8.RuneCountInString(playerID) > maxPlayerActionIdentifierRunes || strings.ContainsFunc(playerID, unicode.IsControl) {
		return fmt.Errorf("%w: invalid Palworld user ID", ErrInvalidPlayerAction)
	}

	var action query.PalworldPlayerAction
	switch req.Action {
	case GameServerPlayerActionKick:
		action = query.PalworldPlayerActionKick
	case GameServerPlayerActionBan:
		action = query.PalworldPlayerActionBan
	case GameServerPlayerActionUnban:
		action = query.PalworldPlayerActionUnban
	case GameServerPlayerActionAllowlistAdd, GameServerPlayerActionAllowlistRemove:
		return ErrPlayerActionUnsupported
	default:
		return ErrInvalidPlayerAction
	}

	errAction := query.PerformPalworldPlayerAction(
		ctx,
		req.IP,
		int(req.QueryPort),
		req.Username,
		req.Password,
		action,
		playerID,
		reason,
	)
	if errAction != nil {
		return fmt.Errorf("%w: %w", ErrPlayerActionUnavailable, errAction)
	}
	return nil
}

func normalizePlayerActionReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", nil
	}
	if utf8.RuneCountInString(reason) > maxPlayerActionReasonRunes || strings.ContainsFunc(reason, unicode.IsControl) {
		return "", fmt.Errorf("%w: invalid player action reason", ErrInvalidPlayerAction)
	}
	return reason, nil
}
