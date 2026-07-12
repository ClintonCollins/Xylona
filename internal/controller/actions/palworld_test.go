package actions

import (
	"errors"
	"strings"
	"testing"
)

func TestPatchPalworldSettings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		password string
		port     int64
		players  int64
		want     []string
		wantErr  error
	}{
		{
			name:     "updates existing values and preserves nested lists",
			input:    "[/Script/Pal.PalGameWorldSettings]\nOptionSettings=(ServerName=\"Pal, World\",CrossplayPlatforms=(Steam,Xbox),AdminPassword=\"old\",RESTAPIEnabled=False,RESTAPIPort=8212,ServerPlayerMaxNum=32)\n",
			password: "generated-secret",
			port:     27015,
			players:  48,
			want: []string{
				`ServerName="Pal, World"`,
				`CrossplayPlatforms=(Steam,Xbox)`,
				`AdminPassword="generated-secret"`,
				`RESTAPIEnabled=True`,
				`RESTAPIPort=27015`,
				`ServerPlayerMaxNum=48`,
			},
		},
		{
			name:     "adds missing managed values",
			input:    "[/Script/Pal.PalGameWorldSettings]\nOptionSettings=(Difficulty=None)\n",
			password: "generated-secret",
			port:     8212,
			players:  32,
			want: []string{
				`Difficulty=None`,
				`AdminPassword="generated-secret"`,
				`RESTAPIEnabled=True`,
				`RESTAPIPort=8212`,
				`ServerPlayerMaxNum=32`,
			},
		},
		{
			name:     "rejects missing option settings",
			input:    "[/Script/Pal.PalGameWorldSettings]\n",
			password: "generated-secret",
			port:     8212,
			players:  32,
			wantErr:  errPalworldOptionSettingsMissing,
		},
		{
			name:     "rejects unsafe password",
			input:    "OptionSettings=(Difficulty=None)",
			password: `bad"password`,
			port:     8212,
			players:  32,
			wantErr:  errors.New("unsupported characters"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, errPatch := patchPalworldSettings([]byte(tc.input), tc.password, tc.port, tc.players)
			if tc.wantErr != nil {
				if errPatch == nil {
					t.Fatalf("patchPalworldSettings() error = nil, want %v", tc.wantErr)
				}
				if !errors.Is(errPatch, tc.wantErr) && !strings.Contains(errPatch.Error(), tc.wantErr.Error()) {
					t.Fatalf("patchPalworldSettings() error = %v, want %v", errPatch, tc.wantErr)
				}
				return
			}
			if errPatch != nil {
				t.Fatalf("patchPalworldSettings() error = %v", errPatch)
			}
			output := string(result)
			for _, expected := range tc.want {
				if !strings.Contains(output, expected) {
					t.Errorf("patchPalworldSettings() output missing %q: %s", expected, output)
				}
			}
		})
	}
}

func TestPalworldSettingsPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		nodeOS OSType
		want   string
	}{
		{name: "windows", nodeOS: Windows, want: "Pal/Saved/Config/WindowsServer/PalWorldSettings.ini"},
		{name: "linux", nodeOS: Linux, want: "Pal/Saved/Config/LinuxServer/PalWorldSettings.ini"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := palworldSettingsPath(tc.nodeOS)
			if got != tc.want {
				t.Fatalf("palworldSettingsPath(%q) = %q, want %q", tc.nodeOS, got, tc.want)
			}
		})
	}
}
