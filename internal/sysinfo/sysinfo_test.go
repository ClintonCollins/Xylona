package sysinfo

import (
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
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
	if !snapshot.MemoryValid {
		t.Error("CollectResourceSnapshot() MemoryValid = false after a successful read")
	}
}

func TestCPUSamplerValidity(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	idle := cpu.TimesStat{User: 100, System: 50, Idle: 850}
	busy := cpu.TimesStat{User: 130, System: 60, Idle: 910} // 40% busy since idle

	type reading struct {
		times       cpu.TimesStat
		after       time.Duration
		wantPercent float64
		wantValid   bool
	}
	tests := []struct {
		name     string
		readings []reading
	}{
		{
			name:     "first reading only primes",
			readings: []reading{{times: idle, wantValid: false}},
		},
		{
			name: "readings inside the first window stay invalid",
			readings: []reading{
				{times: idle},
				{times: busy, after: time.Second, wantValid: false},
			},
		},
		{
			name: "full window yields a valid reading",
			readings: []reading{
				{times: idle},
				{times: busy, after: cpuSampleWindow, wantPercent: 40, wantValid: true},
			},
		},
		{
			name: "calls inside a later window reuse the valid reading",
			readings: []reading{
				{times: idle},
				{times: busy, after: cpuSampleWindow, wantPercent: 40, wantValid: true},
				{times: busy, after: cpuSampleWindow + time.Second, wantPercent: 40, wantValid: true},
			},
		},
		{
			name: "window with no elapsed CPU time stays invalid",
			readings: []reading{
				{times: idle},
				{times: idle, after: cpuSampleWindow, wantValid: false},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var sampler cpuSampler
			for i, r := range tc.readings {
				percent, valid := sampler.sample(r.times, start.Add(r.after))
				if percent != r.wantPercent || valid != r.wantValid {
					t.Fatalf("reading %d: sample() = (%v, %v), want (%v, %v)", i, percent, valid, r.wantPercent, r.wantValid)
				}
			}
		})
	}
}

func TestCPUBusyPercent(t *testing.T) {
	previous := cpu.TimesStat{User: 100, System: 50, Idle: 850}
	current := cpu.TimesStat{User: 130, System: 60, Idle: 910}
	percent, ok := cpuBusyPercent(previous, current)
	if !ok {
		t.Fatal("cpuBusyPercent() ok = false, want true")
	}
	if percent != 40 {
		t.Errorf("cpuBusyPercent() = %v, want 40", percent)
	}
	if _, idleOK := cpuBusyPercent(current, current); idleOK {
		t.Error("cpuBusyPercent() with no elapsed time should not be ok")
	}
}
