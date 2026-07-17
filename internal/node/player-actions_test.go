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

func TestConsolePlayerActionCommands(t *testing.T) {
	t.Parallel()

	quote := func(value string) string {
		return string(rune(34)) + value + string(rune(34))
	}
	tests := []struct {
		name    string
		build   func() (string, error)
		want    string
		wantErr error
	}{
		{
			name: "7 Days to Die permanent ban",
			build: func() (string, error) {
				return sevenDaysToDiePlayerActionCommand(GameServerPlayerActionBan, "Steam_1", "Abuse")
			},
			want: "ban add " + quote("Steam_1") + " 0 minutes " + quote("Abuse"),
		},
		{
			name: "Factorio allowlist",
			build: func() (string, error) {
				return factorioPlayerActionCommand(GameServerPlayerActionAllowlistAdd, "Engineer_1", "")
			},
			want: "/whitelist add Engineer_1",
		},
		{
			name:  "Hytale unban",
			build: func() (string, error) { return hytalePlayerActionCommand(GameServerPlayerActionUnban, "Builder", "") },
			want:  "unban Builder",
		},
		{
			name: "Project Zomboid kick reason",
			build: func() (string, error) {
				return projectZomboidPlayerActionCommand(GameServerPlayerActionKick, "Survivor", "AFK")
			},
			want: "kickuser " + quote("Survivor") + " -r " + quote("AFK"),
		},
		{
			name:  "Terraria ban",
			build: func() (string, error) { return terrariaPlayerActionCommand(GameServerPlayerActionBan, "Guide Fan") },
			want:  "ban Guide Fan",
		},
		{
			name: "Source ban persists ID list",
			build: func() (string, error) {
				return sourcePlayerActionCommand(GameServerPlayerActionBan, "STEAM_1:0:123", "")
			},
			want: "banid 0 STEAM_1:0:123; writeid",
		},
		{
			name: "Rust ban",
			build: func() (string, error) {
				return rustPlayerActionCommand(GameServerPlayerActionBan, "76561198000000000", "Cheating")
			},
			want: "banid 76561198000000000 76561198000000000 " + quote("Cheating"),
		},
		{
			name: "quoted command injection is rejected",
			build: func() (string, error) {
				return projectZomboidPlayerActionCommand(
					GameServerPlayerActionKick,
					"bad"+string(rune(34))+"name",
					"",
				)
			},
			wantErr: ErrInvalidPlayerAction,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			command, errCommand := tc.build()
			if tc.wantErr != nil {
				if !errors.Is(errCommand, tc.wantErr) {
					t.Fatalf("command error = %v, want %v", errCommand, tc.wantErr)
				}
				return
			}
			if errCommand != nil {
				t.Fatalf("command error = %v", errCommand)
			}
			if command != tc.want {
				t.Fatalf("command = %q, want %q", command, tc.want)
			}
		})
	}
}
