package versiontracker

import (
	"regexp"
	"strings"
)

var reSteamCMDAppUpdate = regexp.MustCompile(`(?i)\+app_update\s+(\d+)`)

type TrackerContext struct {
	GameID           string
	UpdateCommand    string
	ServerSoftware   string
	ProviderKind     string
	ProviderSourceID string
	Target           string
	SteamAppID       string
}

func (info TrackerContext) CacheKey() string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(info.GameID)),
		strings.TrimSpace(info.UpdateCommand),
		strings.ToLower(strings.TrimSpace(info.ServerSoftware)),
		normalizeProviderKind(info.ProviderKind),
		strings.ToLower(strings.TrimSpace(info.ProviderSourceID)),
		strings.TrimSpace(info.Target),
		strings.TrimSpace(info.SteamAppID),
	}
	return strings.Join(parts, "\x1f")
}

// ResolverConfig holds configuration for the tracker resolver.
type ResolverConfig struct {
	// DummyTracker is returned for the DummyGameID. Used for testing and development.
	DummyTracker *DummyTracker
	// DummyGameID is the game ID that triggers returning the DummyTracker.
	DummyGameID string
	// CustomTrackerFactory allows tests and special callers to override tracker resolution.
	CustomTrackerFactory func(info TrackerContext) VersionTracker
}

// ResolveTracker selects the appropriate VersionTracker for a game server based on
// its game ID, update command, and server software configuration.
// Returns nil if no suitable tracker can be determined.
func ResolveTracker(cfg ResolverConfig, gameID string, updateCommand string, serverSoftware string) VersionTracker {
	return ResolveTrackerWithContext(cfg, TrackerContext{
		GameID:         gameID,
		UpdateCommand:  updateCommand,
		ServerSoftware: serverSoftware,
	})
}

func ResolveTrackerWithContext(cfg ResolverConfig, info TrackerContext) VersionTracker {
	if cfg.CustomTrackerFactory != nil {
		if tracker := cfg.CustomTrackerFactory(info); tracker != nil {
			return tracker
		}
	}

	// Check for dummy tracker first (testing/development).
	if cfg.DummyTracker != nil && info.GameID == cfg.DummyGameID {
		return cfg.DummyTracker
	}

	switch normalizeProviderKind(info.ProviderKind) {
	case "papermc", "mojang":
		return NewConfiguredMinecraftTracker(info.ProviderKind, info.ProviderSourceID, info.Target)
	case "steamcmd":
		appID := strings.TrimSpace(info.SteamAppID)
		if appID == "" {
			appID = steamCMDAppID(info.UpdateCommand)
		}
		return NewSteamTrackerWithAppID(appID)
	}

	// Check for Minecraft with server software.
	if isMinecraft(info.GameID) && info.ServerSoftware != "" {
		return NewMinecraftTracker()
	}

	// Check for SteamCMD-based update command.
	if isSteamCMDCommand(info.UpdateCommand) {
		return NewSteamTrackerWithAppID(steamCMDAppID(info.UpdateCommand))
	}

	return nil
}

func normalizeProviderKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
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
