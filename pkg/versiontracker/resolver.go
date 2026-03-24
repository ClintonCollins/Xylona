package versiontracker

import (
	"regexp"
	"strings"
)

var reSteamCMDAppUpdate = regexp.MustCompile(`(?i)\+app_update\s+(\d+)`)

// ResolverConfig holds configuration for the tracker resolver.
type ResolverConfig struct {
	// DummyTracker is returned for the DummyGameID. Used for testing and development.
	DummyTracker *DummyTracker
	// DummyGameID is the game ID that triggers returning the DummyTracker.
	DummyGameID string
	// CustomTrackerFactory allows tests and special callers to override tracker resolution.
	CustomTrackerFactory func(gameID string, updateCommand string, serverSoftware string) VersionTracker
}

// ResolveTracker selects the appropriate VersionTracker for a game server based on
// its game ID, update command, and server software configuration.
// Returns nil if no suitable tracker can be determined.
func ResolveTracker(cfg ResolverConfig, gameID string, updateCommand string, serverSoftware string) VersionTracker {
	if cfg.CustomTrackerFactory != nil {
		if tracker := cfg.CustomTrackerFactory(gameID, updateCommand, serverSoftware); tracker != nil {
			return tracker
		}
	}

	// Check for dummy tracker first (testing/development).
	if cfg.DummyTracker != nil && gameID == cfg.DummyGameID {
		return cfg.DummyTracker
	}

	// Check for Minecraft with server software.
	if isMinecraft(gameID) && serverSoftware != "" {
		return NewMinecraftTracker()
	}

	// Check for SteamCMD-based update command.
	if isSteamCMDCommand(updateCommand) {
		return NewSteamTrackerWithAppID(steamCMDAppID(updateCommand))
	}

	return nil
}

// isSteamCMDCommand returns true if the command string appears to use SteamCMD.
func isSteamCMDCommand(command string) bool {
	return strings.Contains(strings.ToLower(command), "steamcmd")
}

func steamCMDAppID(command string) string {
	match := reSteamCMDAppUpdate.FindStringSubmatch(command)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

// isMinecraft returns true if the game ID indicates a Minecraft server.
func isMinecraft(gameID string) bool {
	return strings.EqualFold(gameID, "minecraft")
}
