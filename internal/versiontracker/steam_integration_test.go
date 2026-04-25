//go:build integration

package versiontracker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// TestSteamTracker_Integration_RealAPI tests GetLatestVersion against the live Steam API
// using Counter-Strike 2 Dedicated Server (App ID 740), which is always publicly available.
// Run with: go test -tags=integration ./internal/versiontracker/ -run TestSteamTracker_Integration_RealAPI -v
func TestSteamTracker_Integration_RealAPI(t *testing.T) {
	dir := t.TempDir()

	// Write a minimal appmanifest for CS2 DS (App ID 740).
	acfContent := `"AppState"
{
	"appid"		"740"
	"name"		"Counter-Strike 2 Dedicated Server"
	"buildid"		"0"
}
`
	errWrite := os.WriteFile(filepath.Join(dir, "appmanifest_740.acf"), []byte(acfContent), 0o600)
	if errWrite != nil {
		t.Fatalf("failed to write ACF: %v", errWrite)
	}

	tracker := NewSteamTracker()
	gs := &models.GameServer{Directory: dir}

	latest, errLatest := tracker.GetLatestVersion(context.Background(), gs)
	if errLatest != nil {
		t.Fatalf("GetLatestVersion error: %v", errLatest)
	}
	if latest == "" {
		t.Error("expected a non-empty latest version from Steam API")
	}
	t.Logf("Steam latest version for App 740: %s", latest)
}
