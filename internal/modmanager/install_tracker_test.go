package modmanager

import (
	"testing"
	"time"
)

func TestInstallTracker_Lifecycle(t *testing.T) {
	tracker := NewInstallTracker()

	idleState, ok := tracker.Get("missing")
	if ok {
		t.Fatal("Get(missing) ok = true, want false")
	}
	if idleState.Status != InstallStatusIdle {
		t.Errorf("Get(missing).Status = %q, want %q", idleState.Status, InstallStatusIdle)
	}
	if tracker.IsInstalling("server-1") {
		t.Fatal("IsInstalling() = true before install starts")
	}

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
	if !tracker.IsInstalling("server-1") {
		t.Error("IsInstalling() = false after SetInstalling")
	}

	tracker.SetComplete("server-1")
	if tracker.IsInstalling("server-1") {
		t.Error("IsInstalling() = true after SetComplete")
	}
	state, ok = tracker.Get("server-1")
	if !ok {
		t.Fatal("expected completed state to exist")
	}
	if state.Status != InstallStatusComplete {
		t.Errorf("status = %q, want %q", state.Status, InstallStatusComplete)
	}

	tracker.SetInstalling("server-2", "fabric")
	tracker.SetFailed("server-2", "download error")

	state, ok = tracker.Get("server-2")
	if !ok {
		t.Fatal("expected failed state to exist")
	}
	if state.Status != InstallStatusFailed {
		t.Errorf("status = %q, want %q", state.Status, InstallStatusFailed)
	}
	if state.Error != "download error" {
		t.Errorf("error = %q, want %q", state.Error, "download error")
	}
}

func TestInstallTracker_Cleanup(t *testing.T) {
	tests := []struct {
		name           string
		complete       bool
		fail           bool
		wantStateAfter bool
	}{
		{name: "removes complete state", complete: true, wantStateAfter: false},
		{name: "removes failed state", fail: true, wantStateAfter: false},
		{name: "keeps installing state", wantStateAfter: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := NewInstallTracker()
			tracker.ttl = 50 * time.Millisecond
			tracker.SetInstalling("server-1", "paper")
			switch {
			case tt.complete:
				tracker.SetComplete("server-1")
			case tt.fail:
				tracker.SetFailed("server-1", "download error")
			}

			time.Sleep(100 * time.Millisecond)
			tracker.Cleanup()

			_, ok := tracker.Get("server-1")
			if ok != tt.wantStateAfter {
				t.Fatalf("Get() ok = %v, want %v", ok, tt.wantStateAfter)
			}
			if tt.wantStateAfter && !tracker.IsInstalling("server-1") {
				t.Fatal("installing state should survive cleanup")
			}
		})
	}
}
