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
		info           TrackerContext
		wantType       string
	}{
		{
			name: "dummy game ID returns DummyTracker",
			info: TrackerContext{
				GameID: "test-dummy-game",
			},
			wantType: "dummy",
		},
		{
			name: "steamcmd provider returns SteamTracker",
			info: TrackerContext{
				GameID:        "valheim",
				UpdateCommand: "steamcmd +app_update 896660",
				ProviderKind:  "steamcmd",
				SteamAppID:    "896660",
			},
			wantType:      "steam",
		},
		{
			name: "typed papermc provider returns MinecraftTracker",
			info: TrackerContext{
				GameID:           "minecraft",
				ProviderKind:     "papermc",
				ProviderSourceID: "paper",
				Target:           "1.21.4",
			},
			wantType: "minecraft",
		},
		{
			name: "unknown game with no tracker returns nil",
			info: TrackerContext{
				GameID: "custom-game",
			},
			wantType: "nil",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tracker := ResolveTrackerWithContext(cfg, tc.info)
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
