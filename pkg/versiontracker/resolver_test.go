package versiontracker

import (
	"testing"
)

func TestResolveTracker(t *testing.T) {
	dummy := NewDummyTracker()
	cfg := ResolverConfig{
		DummyTracker: dummy,
		DummyGameID:  "test-dummy-game",
	}

	tests := []struct {
		name           string
		gameID         string
		updateCommand  string
		serverSoftware string
		wantType       string
	}{
		{
			name:     "dummy game ID returns DummyTracker",
			gameID:   "test-dummy-game",
			wantType: "dummy",
		},
		{
			name:          "steamcmd update command returns SteamTracker",
			gameID:        "valheim",
			updateCommand: "steamcmd +app_update 896660",
			wantType:      "steam",
		},
		{
			name:           "minecraft with server software returns MinecraftTracker",
			gameID:         "minecraft",
			serverSoftware: `[{"id":"paper"}]`,
			wantType:       "minecraft",
		},
		{
			name:     "unknown game with no steamcmd returns nil",
			gameID:   "custom-game",
			wantType: "nil",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tracker := ResolveTracker(cfg, tc.gameID, tc.updateCommand, tc.serverSoftware)
			switch tc.wantType {
			case "dummy":
				if _, ok := tracker.(*DummyTracker); !ok {
					t.Errorf("expected *DummyTracker, got %T", tracker)
				}
			case "steam":
				if _, ok := tracker.(*SteamTracker); !ok {
					t.Errorf("expected *SteamTracker, got %T", tracker)
				}
			case "minecraft":
				if _, ok := tracker.(*MinecraftTracker); !ok {
					t.Errorf("expected *MinecraftTracker, got %T", tracker)
				}
			case "nil":
				if tracker != nil {
					t.Errorf("expected nil, got %T", tracker)
				}
			}
		})
	}
}
