package versiontracker

import (
	"strings"
)

// ResolverConfig holds configuration for the tracker resolver.
type ResolverConfig struct {
	// DummyTracker is returned for the DummyGameID. Used for testing and development.
	DummyTracker *DummyTracker
	// DummyGameID is the game ID that triggers returning the DummyTracker.
	DummyGameID string
}

// ResolveTracker selects the appropriate VersionTracker for a game server based on
// its game ID, update command, and server software configuration.
// Returns nil if no suitable tracker can be determined.
func ResolveTracker(cfg ResolverConfig, gameID string, updateCommand string, serverSoftware string) VersionTracker {
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
		return NewSteamTracker()
	}

	return nil
}

// isSteamCMDCommand returns true if the command string appears to use SteamCMD.
func isSteamCMDCommand(command string) bool {
	return strings.Contains(strings.ToLower(command), "steamcmd")
}

// isMinecraft returns true if the game ID indicates a Minecraft server.
func isMinecraft(gameID string) bool {
	return strings.EqualFold(gameID, "minecraft")
}
