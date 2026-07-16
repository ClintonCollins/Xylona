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

var minecraftPlayerNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,16}$`)

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
		errSend := n.SendConsoleInput(req.ProcessID, command)
		if errSend != nil {
			return fmt.Errorf("node: perform Minecraft player action: %w", errSend)
		}
		return nil
	case GameServerQueryKindPalworld:
		return performPalworldPlayerAction(ctx, req, reason)
	case GameServerQueryKindSource:
		return ErrPlayerActionUnsupported
	default:
		return ErrInvalidPlayerAction
	}
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
