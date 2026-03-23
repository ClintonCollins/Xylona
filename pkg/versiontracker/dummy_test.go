package versiontracker

import (
	"context"
	"testing"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestDummyTracker_GetInstalledVersion(t *testing.T) {
	tracker := NewDummyTracker()
	version, err := tracker.GetInstalledVersion(context.Background(), &models.GameServer{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != "1.0.0" {
		t.Errorf("expected 1.0.0, got %s", version)
	}
}

func TestDummyTracker_GetLatestVersion(t *testing.T) {
	tracker := NewDummyTracker()
	version, err := tracker.GetLatestVersion(context.Background(), &models.GameServer{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != "2.0.0" {
		t.Errorf("expected 2.0.0, got %s", version)
	}
}

func TestDummyTracker_CheckForUpdate_ReturnsUpdateAvailable(t *testing.T) {
	tracker := NewDummyTracker()
	info, err := tracker.CheckForUpdate(context.Background(), &models.GameServer{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil UpdateInfo")
	}
	if !info.UpdateAvailable {
		t.Error("expected update available")
	}
	if info.InstalledVersion != "1.0.0" {
		t.Errorf("expected installed 1.0.0, got %s", info.InstalledVersion)
	}
	if info.LatestVersion != "2.0.0" {
		t.Errorf("expected latest 2.0.0, got %s", info.LatestVersion)
	}
}

func TestDummyTracker_SimulateFailure_Toggle(t *testing.T) {
	tracker := NewDummyTracker()
	if tracker.SimulateFailure() {
		t.Error("expected simulateFailure to be false by default")
	}
	tracker.SetSimulateFailure(true)
	if !tracker.SimulateFailure() {
		t.Error("expected simulateFailure to be true after setting")
	}
	tracker.SetSimulateFailure(false)
	if tracker.SimulateFailure() {
		t.Error("expected simulateFailure to be false after unsetting")
	}
}

func TestDummyTracker_AfterUpdate_VersionChanges(t *testing.T) {
	tracker := NewDummyTracker()
	tracker.MarkUpdated()
	version, err := tracker.GetInstalledVersion(context.Background(), &models.GameServer{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != "2.0.0" {
		t.Errorf("expected 2.0.0 after update, got %s", version)
	}
	info, errCheck := tracker.CheckForUpdate(context.Background(), &models.GameServer{})
	if errCheck != nil {
		t.Fatalf("unexpected error: %v", errCheck)
	}
	if info != nil {
		t.Error("expected nil UpdateInfo after update (up to date)")
	}
}
