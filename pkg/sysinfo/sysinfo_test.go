package sysinfo

import (
	"testing"
)

func TestCollectSystemInfo(t *testing.T) {
	info, errCollect := CollectSystemInfo()
	if errCollect != nil {
		t.Fatalf("CollectSystemInfo() error = %v", errCollect)
	}
	if info == nil {
		t.Fatal("CollectSystemInfo() returned nil")
	}
	if info.OS == "" {
		t.Error("CollectSystemInfo() OS is empty")
	}
	if info.Architecture == "" {
		t.Error("CollectSystemInfo() Architecture is empty")
	}
	if info.XylonaVersion == "" {
		t.Error("CollectSystemInfo() XylonaVersion is empty")
	}
	if info.CPUThreads <= 0 {
		t.Error("CollectSystemInfo() CPUThreads should be > 0")
	}
}

func TestCollectResourceSnapshot(t *testing.T) {
	snapshot, errCollect := CollectResourceSnapshot()
	if errCollect != nil {
		t.Fatalf("CollectResourceSnapshot() error = %v", errCollect)
	}
	if snapshot == nil {
		t.Fatal("CollectResourceSnapshot() returned nil")
	}
	if snapshot.MemoryTotal == 0 {
		t.Error("CollectResourceSnapshot() MemoryTotal should be > 0")
	}
}
