package websocket

import (
	"testing"

	"github.com/ClintonCollins/Xylona/internal/node"
)

func TestBuildNodeResourceSnapshotFlagsUnavailableReadings(t *testing.T) {
	tests := []struct {
		name                          string
		snapshot                      node.NodeSnapshot
		wantCPU, wantMemory, wantDisk bool
	}{
		{
			name:     "every reading taken",
			snapshot: node.NodeSnapshot{CPUPercent: 12, CPUValid: true, TotalMemory: 1024, MemoryValid: true, DiskTotal: 100, DiskValid: true},
		},
		{
			name:     "failed readings are flagged",
			snapshot: node.NodeSnapshot{TotalMemory: 1024, DiskTotal: 100, DiskValid: true},
			wantCPU:  true, wantMemory: true,
		},
		{
			name:       "older node zero totals are flagged",
			snapshot:   node.NodeSnapshot{CPUValid: true, MemoryValid: true, DiskValid: true},
			wantMemory: true, wantDisk: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := buildNodeResourceSnapshot(&test.snapshot, nil, 0)
			if got.GetCpuUnavailable() != test.wantCPU || got.GetMemoryUnavailable() != test.wantMemory || got.GetDiskUnavailable() != test.wantDisk {
				t.Fatalf("unavailable = (cpu %t, memory %t, disk %t), want (%t, %t, %t)",
					got.GetCpuUnavailable(), got.GetMemoryUnavailable(), got.GetDiskUnavailable(),
					test.wantCPU, test.wantMemory, test.wantDisk)
			}
		})
	}
}

// A reading that fails while its zeroed value rounds the same as before must
// still be sent, or the browser keeps showing the stale reading.
func TestNodeSnapshotEqualComparesAvailability(t *testing.T) {
	readings := node.NodeSnapshot{TotalMemory: 1024, MemoryValid: true, DiskTotal: 100, DiskValid: true}
	cpuTaken := readings
	cpuTaken.CPUPercent, cpuTaken.CPUValid = 0.4, true
	available := buildNodeResourceSnapshot(&cpuTaken, nil, 0)
	cpuFailed := buildNodeResourceSnapshot(&readings, nil, 0)
	if nodeSnapshotEqual(available, cpuFailed) {
		t.Fatal("nodeSnapshotEqual() = true for a CPU reading that became unavailable")
	}
	if !nodeSnapshotEqual(cpuFailed, buildNodeResourceSnapshot(&readings, nil, 0)) {
		t.Fatal("nodeSnapshotEqual() = false for identical snapshots")
	}
}
