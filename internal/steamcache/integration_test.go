//go:build integration

package steamcache

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Integration tests that hit the real api.steamcmd.net API.

func TestIntegration_FetchDetails_KnownServers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	c := New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tests := []struct {
		name        string
		appID       string
		wantName    string
		wantType    string
		wantWindows bool
		wantLinux   bool
		wantInstDir bool
		wantParent  string
	}{
		{
			name:        "Valheim Dedicated Server",
			appID:       "896660",
			wantName:    "Valheim Dedicated Server",
			wantType:    "Tool",
			wantWindows: true,
			wantLinux:   true,
			wantInstDir: true,
			wantParent:  "892970",
		},
		{
			name:        "TF2 Dedicated Server",
			appID:       "232250",
			wantName:    "Team Fortress 2 Dedicated Server",
			wantType:    "Tool",
			wantWindows: true,
			wantLinux:   true,
			wantInstDir: true,
			wantParent:  "440",
		},
		{
			name:        "7 Days to Die Dedicated Server",
			appID:       "294420",
			wantName:    "7 Days to Die Dedicated Server",
			wantType:    "Tool",
			wantWindows: true,
			wantLinux:   true,
			wantInstDir: true,
			wantParent:  "251570",
		},
		{
			name:        "Counter-Strike 2 Dedicated Server",
			appID:       "740",
			wantName:    "Counter-Strike Global Offensive - Dedicated Server",
			wantType:    "Tool",
			wantWindows: true,
			wantLinux:   true,
			wantInstDir: true,
			wantParent:  "730",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details, err := c.FetchDetails(ctx, tt.appID)
			if err != nil {
				t.Fatalf("FetchDetails(%q) error: %v", tt.appID, err)
			}

			t.Logf("Name=%q Type=%q Windows=%v Linux=%v InstallDir=%q Parent=%q LaunchConfigs=%d",
				details.Name, details.Type, details.WindowsSupport, details.LinuxSupport,
				details.InstallDirectory, details.ParentAppID, len(details.LaunchConfigs))

			if details.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", details.Name, tt.wantName)
			}
			if details.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", details.Type, tt.wantType)
			}
			if details.WindowsSupport != tt.wantWindows {
				t.Errorf("WindowsSupport = %v, want %v", details.WindowsSupport, tt.wantWindows)
			}
			if details.LinuxSupport != tt.wantLinux {
				t.Errorf("LinuxSupport = %v, want %v", details.LinuxSupport, tt.wantLinux)
			}
			if tt.wantInstDir && details.InstallDirectory == "" {
				t.Error("InstallDirectory is empty, want non-empty")
			}
			if details.ParentAppID != tt.wantParent {
				t.Errorf("ParentAppID = %q, want %q", details.ParentAppID, tt.wantParent)
			}
		})
	}
}

func TestIntegration_FetchDetails_GameAppID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	c := New()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Fetch a base game (not a server tool) to verify it works too
	details, err := c.FetchDetails(ctx, "892970") // Valheim (the game)
	if err != nil {
		t.Fatalf("FetchDetails() error: %v", err)
	}

	t.Logf("Game: Name=%q Type=%q", details.Name, details.Type)

	if details.Name != "Valheim" {
		t.Errorf("Name = %q, want %q", details.Name, "Valheim")
	}
	if details.Type != "Game" {
		t.Errorf("Type = %q, want %q", details.Type, "Game")
	}
}

func TestIntegration_RefreshRecorded294420Fixture(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("XYLONA_REFRESH_STEAMCACHE_FIXTURES") == "" {
		t.Skip("set XYLONA_REFRESH_STEAMCACHE_FIXTURES=1 to refresh recorded steamcache fixtures")
	}

	c := New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, errRequest := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.steamcmd.net/v1/info/294420", nil)
	if errRequest != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", errRequest)
	}

	resp, errDo := c.client().Do(req)
	if errDo != nil {
		t.Fatalf("client.Do() error = %v", errDo)
	}
	defer resp.Body.Close()

	body, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		t.Fatalf("io.ReadAll() error = %v", errRead)
	}

	fixturePath := filepath.Join("testdata", "steamcmd-294420.json")
	errWrite := os.WriteFile(fixturePath, body, 0o644)
	if errWrite != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", fixturePath, errWrite)
	}
}
