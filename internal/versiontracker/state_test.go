package versiontracker

import (
	"testing"
	"time"
)

func TestVersionStateMap_GetReturnsNoTrackerForUnknownServer(t *testing.T) {
	m := NewVersionStateMap()
	state := m.Get("nonexistent")
	if state.Status != VersionStatusNoTracker {
		t.Fatalf("expected NoTracker status, got %d", state.Status)
	}
}

func TestVersionStateMap_SetAndGet(t *testing.T) {
	m := NewVersionStateMap()
	m.Set("server1", VersionState{
		Status:           VersionStatusChecked,
		InstalledVersion: "1.0.0",
		LatestVersion:    "2.0.0",
		UpdateAvailable:  true,
		TrackerType:      "dummy",
		LastCheckTime:    time.Now(),
	})
	state := m.Get("server1")
	if state.Status != VersionStatusChecked {
		t.Fatalf("expected Checked status, got %d", state.Status)
	}
	if state.InstalledVersion != "1.0.0" {
		t.Errorf("expected installed version 1.0.0, got %s", state.InstalledVersion)
	}
	if !state.UpdateAvailable {
		t.Error("expected update available to be true")
	}
}

func TestVersionStateMap_InitUnchecked(t *testing.T) {
	m := NewVersionStateMap()
	m.InitUnchecked("server1", "steam")
	state := m.Get("server1")
	if state.Status != VersionStatusUnchecked {
		t.Fatalf("expected Unchecked status, got %d", state.Status)
	}
	if state.TrackerType != "steam" {
		t.Errorf("expected tracker type steam, got %s", state.TrackerType)
	}
}

func TestVersionStateMap_InitNoTracker(t *testing.T) {
	m := NewVersionStateMap()
	m.InitNoTracker("server1")
	state := m.Get("server1")
	if state.Status != VersionStatusNoTracker {
		t.Fatalf("expected NoTracker status, got %d", state.Status)
	}
}

func TestVersionStateMap_GetAll(t *testing.T) {
	m := NewVersionStateMap()
	m.Set("s1", VersionState{Status: VersionStatusChecked, UpdateAvailable: true})
	m.Set("s2", VersionState{Status: VersionStatusChecked, UpdateAvailable: false})
	all := m.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}
}

func TestVersionStateMap_Delete(t *testing.T) {
	m := NewVersionStateMap()
	m.Set("server1", VersionState{Status: VersionStatusChecked})
	m.Delete("server1")
	state := m.Get("server1")
	if state.Status != VersionStatusNoTracker {
		t.Fatalf("expected NoTracker after delete, got %d", state.Status)
	}
}
