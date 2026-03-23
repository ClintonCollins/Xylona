package versiontracker

import (
	"context"
	"strings"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// UpdateInfo describes an available update for a game server.
type UpdateInfo struct {
	InstalledVersion string
	LatestVersion    string
	UpdateAvailable  bool
}

// VersionTracker detects installed and latest versions for a game server.
type VersionTracker interface {
	// GetInstalledVersion returns the currently installed version.
	// Returns empty string if version cannot be determined.
	GetInstalledVersion(ctx context.Context, gameServer *models.GameServer) (string, error)

	// GetLatestVersion returns the latest available version.
	GetLatestVersion(ctx context.Context, gameServer *models.GameServer) (string, error)

	// CheckForUpdate compares installed vs latest and returns update info.
	// Returns nil if up-to-date or version cannot be determined.
	CheckForUpdate(ctx context.Context, gameServer *models.GameServer) (*UpdateInfo, error)
}

func normalizeVersion(version string) string {
	return strings.TrimSpace(version)
}

func versionsEqual(installed string, latest string) bool {
	return normalizeVersion(installed) == normalizeVersion(latest)
}

// TrackerTypeName returns the stable API tracker type for a resolved tracker.
func TrackerTypeName(tracker VersionTracker) string {
	switch tracker.(type) {
	case *DummyTracker:
		return "dummy"
	case *MinecraftTracker:
		return "minecraft"
	case *SteamTracker:
		return "steam"
	default:
		return ""
	}
}
