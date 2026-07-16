package node

import (
	"errors"
	"strings"
	"testing"
)

func TestMinecraftPlayerActionCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		action   GameServerPlayerAction
		playerID string
		reason   string
		want     string
		wantErr  error
	}{
		{name: "kick", action: GameServerPlayerActionKick, playerID: "Player_1", reason: "AFK", want: "kick Player_1 AFK"},
		{name: "ban", action: GameServerPlayerActionBan, playerID: "Player_1", reason: "Abuse", want: "ban Player_1 Abuse"},
		{name: "unban", action: GameServerPlayerActionUnban, playerID: "Player_1", reason: "ignored", want: "pardon Player_1"},
		{name: "allowlist add", action: GameServerPlayerActionAllowlistAdd, playerID: "Player_1", want: "whitelist add Player_1"},
		{name: "allowlist remove", action: GameServerPlayerActionAllowlistRemove, playerID: "Player_1", want: "whitelist remove Player_1"},
		{name: "rejects command injection", action: GameServerPlayerActionKick, playerID: "Player_1\nban Other", wantErr: ErrInvalidPlayerAction},
		{name: "rejects unsupported action", action: GameServerPlayerActionUnknown, playerID: "Player_1", wantErr: ErrInvalidPlayerAction},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			command, errCommand := minecraftPlayerActionCommand(tc.action, tc.playerID, tc.reason)
			if tc.wantErr != nil {
				if !errors.Is(errCommand, tc.wantErr) {
					t.Fatalf("minecraftPlayerActionCommand() error = %v, want %v", errCommand, tc.wantErr)
				}
				return
			}
			if errCommand != nil {
				t.Fatalf("minecraftPlayerActionCommand() error = %v", errCommand)
			}
			if command != tc.want {
				t.Fatalf("command = %q, want %q", command, tc.want)
			}
		})
	}
}

func TestNormalizePlayerActionReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		reason  string
		want    string
		wantErr bool
	}{
		{name: "empty", reason: "  ", want: ""},
		{name: "trims", reason: "  repeated abuse  ", want: "repeated abuse"},
		{name: "rejects newline", reason: "line one\nline two", wantErr: true},
		{name: "rejects excessive length", reason: strings.Repeat("a", maxPlayerActionReasonRunes+1), wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reason, errReason := normalizePlayerActionReason(tc.reason)
			if tc.wantErr {
				if !errors.Is(errReason, ErrInvalidPlayerAction) {
					t.Fatalf("normalizePlayerActionReason() error = %v, want invalid action", errReason)
				}
				return
			}
			if errReason != nil || reason != tc.want {
				t.Fatalf("normalizePlayerActionReason() = %q, %v, want %q, nil", reason, errReason, tc.want)
			}
		})
	}
}
