//go:build integration

package versiontracker

import (
	"context"
	"testing"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// TestMinecraftTracker_Integration_GetLatestVersion_PaperMC tests against the real PaperMC API.
// Run with: go test -tags=integration ./internal/versiontracker/ -run TestMinecraftTracker_Integration
func TestMinecraftTracker_Integration_GetLatestVersion_PaperMC(t *testing.T) {
	tracker := NewMinecraftTracker()
	gs := &models.GameServer{
		ServerSoftware: null.From(serverSoftwareJSON("paper")),
	}
	version, errLatest := tracker.GetLatestVersion(context.Background(), gs)
	if errLatest != nil {
		t.Fatalf("GetLatestVersion error: %v", errLatest)
	}
	if version == "" {
		t.Error("expected a non-empty version from live PaperMC API")
	}
	t.Logf("PaperMC latest version: %s", version)
}

// TestMinecraftTracker_Integration_GetLatestVersion_Folia tests Folia against the real PaperMC API.
func TestMinecraftTracker_Integration_GetLatestVersion_Folia(t *testing.T) {
	tracker := NewMinecraftTracker()
	gs := &models.GameServer{
		ServerSoftware: null.From(serverSoftwareJSON("folia")),
	}
	version, errLatest := tracker.GetLatestVersion(context.Background(), gs)
	if errLatest != nil {
		t.Fatalf("GetLatestVersion error: %v", errLatest)
	}
	if version == "" {
		t.Error("expected a non-empty version from live PaperMC API for folia")
	}
	t.Logf("Folia latest version: %s", version)
}

// TestMinecraftTracker_Integration_CheckForUpdate tests CheckForUpdate with a known old version
// against the live PaperMC API.
func TestMinecraftTracker_Integration_CheckForUpdate(t *testing.T) {
	tracker := NewMinecraftTracker()
	// 1.8.8 is a very old version — there will always be a newer one.
	gs := &models.GameServer{
		Version:        "1.8.8",
		ServerSoftware: null.From(serverSoftwareJSON("paper")),
	}
	info, errCheck := tracker.CheckForUpdate(context.Background(), gs)
	if errCheck != nil {
		t.Fatalf("CheckForUpdate error: %v", errCheck)
	}
	if info == nil {
		t.Fatal("expected non-nil UpdateInfo for very old version 1.8.8")
	}
	if !info.UpdateAvailable {
		t.Error("expected UpdateAvailable to be true for 1.8.8")
	}
	t.Logf("Installed: %s, Latest: %s", info.InstalledVersion, info.LatestVersion)
}
