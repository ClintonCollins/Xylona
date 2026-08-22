package versiontracker

import "testing"

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
