package versiontracker

import (
	"context"
	"strings"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// UpdateInfo describes the result of an update check for a game server.
type UpdateInfo struct {
	InstalledVersion      string
	LatestVersion         string
	UpdateAvailable       bool
	InstalledVersionLabel string
	LatestVersionLabel    string
	InstalledBranch       string
	LatestBranch          string
}

// VersionTracker detects installed and latest versions for a game server.
type VersionTracker interface {
	// GetInstalledVersion returns the currently installed version.
	// Returns empty string if version cannot be determined.
	GetInstalledVersion(ctx context.Context, gameServer *models.GameServer) (string, error)

	// GetLatestVersion returns the latest available version.
	GetLatestVersion(ctx context.Context, gameServer *models.GameServer) (string, error)

	// CheckForUpdate compares installed vs latest and returns update info.
	// UpdateAvailable reports whether a newer version was found.
	CheckForUpdate(ctx context.Context, gameServer *models.GameServer) (*UpdateInfo, error)
}

func normalizeVersion(version string) string {
	return strings.TrimSpace(version)
}

// NormalizeSteamBranch returns the default public branch when no branch is configured.
func NormalizeSteamBranch(branch string) string {
	normalized := strings.TrimSpace(branch)
	if normalized == "" {
		return "public"
	}
	return normalized
}

// IsSafeSteamBranch reports whether branch is a single SteamCMD beta identifier.
func IsSafeSteamBranch(branch string) bool {
	if branch == "" {
		return false
	}
	for _, r := range branch {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
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
