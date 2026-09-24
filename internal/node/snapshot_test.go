package node

import "testing"

// GetNodeSnapshot copies each metric's validity from sysinfo. A dropped flag
// defaults to false, which would silently mute that metric's node alerts.
func TestGetNodeSnapshotCarriesValidity(t *testing.T) {
	snapshot, errSnapshot := (&Node{}).GetNodeSnapshot(t.Context())
	if errSnapshot != nil {
		t.Fatalf("GetNodeSnapshot() error = %v", errSnapshot)
	}
	if !snapshot.MemoryValid {
		t.Error("MemoryValid = false after a successful host read")
	}
	if !snapshot.DiskValid {
		t.Error("DiskValid = false after a successful host read")
	}
}
