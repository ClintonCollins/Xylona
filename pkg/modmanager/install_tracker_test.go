package modmanager

import (
	"testing"
	"time"
)

func TestInstallTracker_SetAndGet(t *testing.T) {
	tracker := NewInstallTracker()

	tracker.SetInstalling("server-1", "paper")
	state, ok := tracker.Get("server-1")
	if !ok {
		t.Fatal("expected state to exist")
	}
	if state.Status != InstallStatusInstalling {
		t.Errorf("status = %q, want %q", state.Status, InstallStatusInstalling)
	}
	if state.SoftwareID != "paper" {
		t.Errorf("softwareID = %q, want %q", state.SoftwareID, "paper")
	}
}

func TestInstallTracker_IsInstalling(t *testing.T) {
	tracker := NewInstallTracker()

	if tracker.IsInstalling("server-1") {
		t.Error("expected not installing")
	}

	tracker.SetInstalling("server-1", "paper")
	if !tracker.IsInstalling("server-1") {
		t.Error("expected installing")
	}

	tracker.SetComplete("server-1")
	if tracker.IsInstalling("server-1") {
		t.Error("expected not installing after complete")
	}
}

func TestInstallTracker_SetFailed(t *testing.T) {
	tracker := NewInstallTracker()

	tracker.SetInstalling("server-1", "paper")
	tracker.SetFailed("server-1", "download error")

	state, ok := tracker.Get("server-1")
	if !ok {
		t.Fatal("expected state to exist")
	}
	if state.Status != InstallStatusFailed {
		t.Errorf("status = %q, want %q", state.Status, InstallStatusFailed)
	}
	if state.Error != "download error" {
		t.Errorf("error = %q, want %q", state.Error, "download error")
	}
}

func TestInstallTracker_GetIdle(t *testing.T) {
	tracker := NewInstallTracker()

	state, ok := tracker.Get("nonexistent")
	if ok {
		t.Error("expected ok=false for nonexistent server")
	}
	if state.Status != InstallStatusIdle {
		t.Errorf("status = %q, want %q", state.Status, InstallStatusIdle)
	}
}

func TestInstallTracker_Cleanup(t *testing.T) {
	tracker := NewInstallTracker()
	tracker.ttl = 50 * time.Millisecond

	tracker.SetInstalling("server-1", "paper")
	tracker.SetComplete("server-1")

	_, ok := tracker.Get("server-1")
	if !ok {
		t.Fatal("expected state to exist immediately after completion")
	}

	time.Sleep(100 * time.Millisecond)
	tracker.Cleanup()

	_, ok = tracker.Get("server-1")
	if ok {
		t.Error("expected state to be cleaned up after TTL")
	}
}

func TestInstallTracker_CleanupKeepsInstalling(t *testing.T) {
	tracker := NewInstallTracker()
	tracker.ttl = 50 * time.Millisecond

	tracker.SetInstalling("server-1", "paper")

	time.Sleep(100 * time.Millisecond)
	tracker.Cleanup()

	if !tracker.IsInstalling("server-1") {
		t.Error("expected installing state to survive cleanup")
	}
}
