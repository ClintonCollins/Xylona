package supervisor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestCalculateDirSize(t *testing.T) {
	tests := []struct {
		name        string
		useEmptyDir bool
		setup       func(dir string) uint64
	}{
		{
			name: "empty directory",
			setup: func(_ string) uint64 {
				return 0
			},
		},
		{
			name: "single file",
			setup: func(dir string) uint64 {
				data := []byte("hello world")
				_ = os.WriteFile(filepath.Join(dir, "test.txt"), data, 0o600)
				return uint64(len(data))
			},
		},
		{
			name: "nested files",
			setup: func(dir string) uint64 {
				subDir := filepath.Join(dir, "sub")
				_ = os.MkdirAll(subDir, 0o700)
				data1 := []byte("file one content")
				data2 := []byte("file two content here")
				_ = os.WriteFile(filepath.Join(dir, "a.txt"), data1, 0o600)
				_ = os.WriteFile(filepath.Join(subDir, "b.txt"), data2, 0o600)
				return uint64(len(data1) + len(data2))
			},
		},
		{
			name:        "empty string returns zero",
			useEmptyDir: true,
			setup:       func(_ string) uint64 { return 0 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dir string
			if tt.useEmptyDir {
				dir = ""
			} else {
				dir = t.TempDir()
			}
			expectedSize := tt.setup(dir)
			got, errCalc := calculateDirSize(dir)
			if errCalc != nil {
				t.Fatalf("calculateDirSize() unexpected error: %v", errCalc)
			}
			if got != expectedSize {
				t.Errorf("calculateDirSize() = %d, want %d", got, expectedSize)
			}
		})
	}
}

func TestCollectProcessMetrics_NilProcess(t *testing.T) {
	cmd := &Command{
		RWMutex: &sync.RWMutex{},
	}
	cmd.collectProcessMetrics()
	cpu, cpuValid, metricsValid, memRSS, memVMS, memPct, _, threads, _, _, _, _, _, _, _, _, _, _, _ := cmd.Metrics()
	if cpu != 0 || memRSS != 0 || memVMS != 0 || memPct != 0 || threads != 0 {
		t.Errorf("expected zero metrics for nil process, got cpu=%v memRSS=%v memVMS=%v memPct=%v threads=%v", cpu, memRSS, memVMS, memPct, threads)
	}
	if cpuValid || metricsValid {
		t.Errorf("expected nil process metrics to be invalid, got cpuValid=%t metricsValid=%t", cpuValid, metricsValid)
	}
}

func TestIntervalCPUPercent(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 3, 0, time.UTC)
	previousPoll := now.Add(-3 * time.Second)
	tests := []struct {
		name            string
		cpuDeltaSeconds float64
		deltaValid      bool
		previousPoll    time.Time
		numCPU          int
		want            float64
		wantValid       bool
	}{
		{
			name:            "one core fully busy across four core host",
			cpuDeltaSeconds: 3,
			deltaValid:      true,
			previousPoll:    previousPoll,
			numCPU:          4,
			want:            25,
			wantValid:       true,
		},
		{
			name:            "first poll has no interval",
			cpuDeltaSeconds: 3,
			deltaValid:      true,
			numCPU:          4,
			want:            0,
		},
		{
			name:            "invalid aggregate has no usage",
			cpuDeltaSeconds: 3,
			previousPoll:    previousPoll,
			numCPU:          4,
			want:            0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, valid := intervalCPUPercent(tt.cpuDeltaSeconds, tt.deltaValid, now, tt.previousPoll, tt.numCPU)
			if got != tt.want {
				t.Errorf("intervalCPUPercent() = %v, want %v", got, tt.want)
			}
			if valid != tt.wantValid {
				t.Errorf("intervalCPUPercent() valid = %t, want %t", valid, tt.wantValid)
			}
		})
	}
}

func TestIntervalIORates(t *testing.T) {
	now := time.Date(2026, time.July, 17, 12, 0, 3, 0, time.UTC)
	previousPoll := now.Add(-3 * time.Second)
	tests := []struct {
		name          string
		readDelta     uint64
		writeDelta    uint64
		deltaValid    bool
		previousPoll  time.Time
		wantReadRate  float64
		wantWriteRate float64
		wantValid     bool
	}{
		{
			name:          "valid interval reports bytes per second",
			readDelta:     300,
			writeDelta:    150,
			deltaValid:    true,
			previousPoll:  previousPoll,
			wantReadRate:  100,
			wantWriteRate: 50,
			wantValid:     true,
		},
		{
			name:         "first poll has no interval",
			readDelta:    300,
			writeDelta:   150,
			deltaValid:   true,
			previousPoll: time.Time{},
		},
		{
			name:         "invalid aggregate is unavailable",
			readDelta:    300,
			writeDelta:   150,
			previousPoll: previousPoll,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readRate, writeRate, valid := intervalIORates(tt.readDelta, tt.writeDelta, tt.deltaValid, now, tt.previousPoll)
			if readRate != tt.wantReadRate {
				t.Errorf("read rate = %v, want %v", readRate, tt.wantReadRate)
			}
			if writeRate != tt.wantWriteRate {
				t.Errorf("write rate = %v, want %v", writeRate, tt.wantWriteRate)
			}
			if valid != tt.wantValid {
				t.Errorf("valid = %t, want %t", valid, tt.wantValid)
			}
		})
	}
}

func TestAggregateProcessMetricSamples(t *testing.T) {
	validSample := func(pid int32, cpuSeconds float64, memoryRSS uint64) processMetricSample {
		return processMetricSample{
			pid:                 pid,
			cpuSeconds:          cpuSeconds,
			cpuReadOK:           true,
			memoryRSS:           memoryRSS,
			memoryVMS:           memoryRSS * 2,
			memoryPercent:       float32(memoryRSS) / 10,
			memoryInfoReadOK:    true,
			memoryPercentReadOK: true,
		}
	}
	tests := []struct {
		name                string
		samples             []processMetricSample
		previousCPUSeconds  map[int32]float64
		treeComplete        bool
		wantCPUDelta        float64
		wantCPUValid        bool
		wantCPUSecondsByPID map[int32]float64
		wantMemoryRSS       uint64
		wantMemoryValid     bool
	}{
		{
			name: "child entry invalidates the interval while establishing baselines",
			samples: []processMetricSample{
				validSample(100, 13, 100),
				validSample(200, 5, 50),
			},
			previousCPUSeconds:  map[int32]float64{100: 10},
			treeComplete:        true,
			wantCPUDelta:        3,
			wantCPUSecondsByPID: map[int32]float64{100: 13, 200: 5},
			wantMemoryRSS:       150,
			wantMemoryValid:     true,
		},
		{
			name: "child exit invalidates the interval and prunes its PID",
			samples: []processMetricSample{
				validSample(100, 13, 100),
			},
			previousCPUSeconds:  map[int32]float64{100: 10, 200: 5},
			treeComplete:        true,
			wantCPUDelta:        3,
			wantCPUSecondsByPID: map[int32]float64{100: 13},
			wantMemoryRSS:       100,
			wantMemoryValid:     true,
		},
		{
			name: "counter reset becomes a fresh baseline",
			samples: []processMetricSample{
				validSample(100, 4, 100),
			},
			previousCPUSeconds:  map[int32]float64{100: 10},
			treeComplete:        true,
			wantCPUSecondsByPID: map[int32]float64{100: 4},
			wantMemoryRSS:       100,
			wantMemoryValid:     true,
		},
		{
			name: "partial CPU read invalidates interval and drops failed baseline",
			samples: []processMetricSample{
				validSample(100, 13, 100),
				{
					pid:                 200,
					memoryRSS:           50,
					memoryVMS:           100,
					memoryPercent:       5,
					memoryInfoReadOK:    true,
					memoryPercentReadOK: true,
				},
			},
			previousCPUSeconds:  map[int32]float64{100: 10, 200: 5},
			treeComplete:        true,
			wantCPUDelta:        3,
			wantCPUSecondsByPID: map[int32]float64{100: 13},
			wantMemoryRSS:       150,
			wantMemoryValid:     true,
		},
		{
			name: "partial memory read invalidates and clears aggregate memory",
			samples: []processMetricSample{
				validSample(100, 13, 100),
				{
					pid:           200,
					cpuSeconds:    8,
					cpuReadOK:     true,
					memoryRSS:     50,
					memoryVMS:     100,
					memoryPercent: 5,
				},
			},
			previousCPUSeconds:  map[int32]float64{100: 10, 200: 5},
			treeComplete:        true,
			wantCPUDelta:        6,
			wantCPUValid:        true,
			wantCPUSecondsByPID: map[int32]float64{100: 13, 200: 8},
		},
		{
			name: "incomplete process tree invalidates CPU and memory",
			samples: []processMetricSample{
				validSample(100, 13, 100),
			},
			previousCPUSeconds:  map[int32]float64{100: 10},
			wantCPUDelta:        3,
			wantCPUSecondsByPID: map[int32]float64{100: 13},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := aggregateProcessMetricSamples(tt.samples, tt.previousCPUSeconds, nil, tt.treeComplete)
			if got.cpuDeltaSeconds != tt.wantCPUDelta {
				t.Errorf("CPU delta = %v, want %v", got.cpuDeltaSeconds, tt.wantCPUDelta)
			}
			if got.cpuDeltaValid != tt.wantCPUValid {
				t.Errorf("CPU valid = %t, want %t", got.cpuDeltaValid, tt.wantCPUValid)
			}
			if !cpuSecondsByPIDEqual(got.cpuSecondsByPID, tt.wantCPUSecondsByPID) {
				t.Errorf("CPU baselines = %#v, want %#v", got.cpuSecondsByPID, tt.wantCPUSecondsByPID)
			}
			if got.memoryRSS != tt.wantMemoryRSS {
				t.Errorf("memory RSS = %d, want %d", got.memoryRSS, tt.wantMemoryRSS)
			}
			if got.memoryValid != tt.wantMemoryValid {
				t.Errorf("memory valid = %t, want %t", got.memoryValid, tt.wantMemoryValid)
			}
		})
	}
}

func cpuSecondsByPIDEqual(got, want map[int32]float64) bool {
	if len(got) != len(want) {
		return false
	}
	for pid, wantSeconds := range want {
		gotSeconds, exists := got[pid]
		if !exists || gotSeconds != wantSeconds {
			return false
		}
	}
	return true
}

func TestAggregateProcessMetricSamplesIOAndConnections(t *testing.T) {
	metricSample := func(pid int32, readBytes, writeBytes uint64, connections int32) processMetricSample {
		return processMetricSample{
			pid:               pid,
			ioRead:            readBytes,
			ioWrite:           writeBytes,
			ioReadOK:          true,
			connections:       connections,
			connectionsReadOK: true,
		}
	}
	tests := []struct {
		name                 string
		samples              []processMetricSample
		previousIOByPID      map[int32]processIOCounters
		treeComplete         bool
		wantReadDelta        uint64
		wantWriteDelta       uint64
		wantIOValid          bool
		wantIOByPID          map[int32]processIOCounters
		wantConnections      int32
		wantConnectionsValid bool
	}{
		{
			name:                 "first poll establishes baselines and preserves valid zero connections",
			samples:              []processMetricSample{metricSample(100, 100, 200, 0)},
			treeComplete:         true,
			wantIOByPID:          map[int32]processIOCounters{100: {read: 100, write: 200}},
			wantConnectionsValid: true,
		},
		{
			name:                 "stable process tree reports summed deltas and connections",
			samples:              []processMetricSample{metricSample(100, 160, 230, 2), metricSample(200, 40, 80, 3)},
			previousIOByPID:      map[int32]processIOCounters{100: {read: 100, write: 200}, 200: {read: 10, write: 20}},
			treeComplete:         true,
			wantReadDelta:        90,
			wantWriteDelta:       90,
			wantIOValid:          true,
			wantIOByPID:          map[int32]processIOCounters{100: {read: 160, write: 230}, 200: {read: 40, write: 80}},
			wantConnections:      5,
			wantConnectionsValid: true,
		},
		{
			name:                 "child entry invalidates one interval without adding lifetime counters",
			samples:              []processMetricSample{metricSample(100, 160, 230, 1), metricSample(200, 9000, 8000, 1)},
			previousIOByPID:      map[int32]processIOCounters{100: {read: 100, write: 200}},
			treeComplete:         true,
			wantReadDelta:        60,
			wantWriteDelta:       30,
			wantIOByPID:          map[int32]processIOCounters{100: {read: 160, write: 230}, 200: {read: 9000, write: 8000}},
			wantConnections:      2,
			wantConnectionsValid: true,
		},
		{
			name:                 "child exit invalidates one interval and prunes its baseline",
			samples:              []processMetricSample{metricSample(100, 160, 230, 1)},
			previousIOByPID:      map[int32]processIOCounters{100: {read: 100, write: 200}, 200: {read: 10, write: 20}},
			treeComplete:         true,
			wantReadDelta:        60,
			wantWriteDelta:       30,
			wantIOByPID:          map[int32]processIOCounters{100: {read: 160, write: 230}},
			wantConnections:      1,
			wantConnectionsValid: true,
		},
		{
			name: "counter regression and connection read failure are unavailable",
			samples: []processMetricSample{
				{
					pid:         100,
					ioRead:      90,
					ioWrite:     190,
					ioReadOK:    true,
					connections: 4,
				},
			},
			previousIOByPID: map[int32]processIOCounters{100: {read: 100, write: 200}},
			treeComplete:    true,
			wantIOByPID:     map[int32]processIOCounters{100: {read: 90, write: 190}},
		},
		{
			name: "partial IO read drops the failed baseline and invalidates the interval",
			samples: []processMetricSample{
				metricSample(100, 160, 230, 1),
				{pid: 200, connectionsReadOK: true},
			},
			previousIOByPID:      map[int32]processIOCounters{100: {read: 100, write: 200}, 200: {read: 10, write: 20}},
			treeComplete:         true,
			wantReadDelta:        60,
			wantWriteDelta:       30,
			wantIOByPID:          map[int32]processIOCounters{100: {read: 160, write: 230}},
			wantConnections:      1,
			wantConnectionsValid: true,
		},
		{
			name:            "incomplete process tree invalidates IO and connections",
			samples:         []processMetricSample{metricSample(100, 160, 230, 1)},
			previousIOByPID: map[int32]processIOCounters{100: {read: 100, write: 200}},
			wantReadDelta:   60,
			wantWriteDelta:  30,
			wantIOByPID:     map[int32]processIOCounters{100: {read: 160, write: 230}},
			wantConnections: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := aggregateProcessMetricSamples(tt.samples, nil, tt.previousIOByPID, tt.treeComplete)
			if got.ioReadDelta != tt.wantReadDelta {
				t.Errorf("I/O read delta = %d, want %d", got.ioReadDelta, tt.wantReadDelta)
			}
			if got.ioWriteDelta != tt.wantWriteDelta {
				t.Errorf("I/O write delta = %d, want %d", got.ioWriteDelta, tt.wantWriteDelta)
			}
			if got.ioDeltaValid != tt.wantIOValid {
				t.Errorf("I/O valid = %t, want %t", got.ioDeltaValid, tt.wantIOValid)
			}
			if !processIOByPIDEqual(got.ioByPID, tt.wantIOByPID) {
				t.Errorf("I/O baselines = %#v, want %#v", got.ioByPID, tt.wantIOByPID)
			}
			if got.connections != tt.wantConnections {
				t.Errorf("connections = %d, want %d", got.connections, tt.wantConnections)
			}
			if got.connectionsValid != tt.wantConnectionsValid {
				t.Errorf("connections valid = %t, want %t", got.connectionsValid, tt.wantConnectionsValid)
			}
		})
	}
}

func processIOByPIDEqual(got, want map[int32]processIOCounters) bool {
	if len(got) != len(want) {
		return false
	}
	for pid, wantCounters := range want {
		gotCounters, exists := got[pid]
		if !exists || gotCounters != wantCounters {
			return false
		}
	}
	return true
}

func TestSelectDiskMetricScanTargets(t *testing.T) {
	commands := []*Command{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	first, nextIndex := selectDiskMetricScanTargets(commands, 0, 2)
	if len(first) != 2 || first[0].ID != "a" || first[1].ID != "b" {
		t.Fatalf("first disk scan targets = %#v, want a then b", first)
	}
	if nextIndex != 2 {
		t.Fatalf("first next index = %d, want 2", nextIndex)
	}

	second, nextIndex := selectDiskMetricScanTargets(commands, nextIndex, 2)
	if len(second) != 2 || second[0].ID != "c" || second[1].ID != "a" {
		t.Fatalf("second disk scan targets = %#v, want c then a", second)
	}
	if nextIndex != 1 {
		t.Fatalf("second next index = %d, want 1", nextIndex)
	}
}

func TestNormalizeMetricsPollerOptions(t *testing.T) {
	defaults := DefaultMetricsPollerOptions()
	tests := []struct {
		name  string
		input MetricsPollerOptions
		want  MetricsPollerOptions
	}{
		{
			name: "zero values use defaults",
			want: defaults,
		},
		{
			name: "custom values are preserved",
			input: MetricsPollerOptions{
				ProcessInterval:  7 * time.Second,
				DiskInterval:     11 * time.Second,
				DiskScansPerTick: 4,
			},
			want: MetricsPollerOptions{
				ProcessInterval:  7 * time.Second,
				DiskInterval:     11 * time.Second,
				DiskScansPerTick: 4,
			},
		},
		{
			name: "extreme values are bounded",
			input: MetricsPollerOptions{
				ProcessInterval:  time.Millisecond,
				DiskInterval:     2 * time.Hour,
				DiskScansPerTick: 100,
			},
			want: MetricsPollerOptions{
				ProcessInterval:  minProcessMetricsInterval,
				DiskInterval:     maxDiskMetricsInterval,
				DiskScansPerTick: maxDiskMetricsScansPerTick,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeMetricsPollerOptions(test.input)
			if got != test.want {
				t.Errorf("normalizeMetricsPollerOptions() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestDefaultMetricsPollerOptions(t *testing.T) {
	options := DefaultMetricsPollerOptions()
	if options.ProcessInterval != 3*time.Second {
		t.Errorf("ProcessInterval = %v, want 3s", options.ProcessInterval)
	}
	if options.DiskInterval != 30*time.Second {
		t.Errorf("DiskInterval = %v, want 30s", options.DiskInterval)
	}
	if options.DiskScansPerTick != 2 {
		t.Errorf("DiskScansPerTick = %d, want 2", options.DiskScansPerTick)
	}
}

func TestCollectDiskMetrics(t *testing.T) {
	directory := t.TempDir()
	contents := []byte("disk metrics")
	errWrite := os.WriteFile(filepath.Join(directory, "metrics.txt"), contents, 0o600)
	if errWrite != nil {
		t.Fatalf("write metrics fixture: %v", errWrite)
	}

	command := &Command{RWMutex: &sync.RWMutex{}, workingDir: directory}
	command.collectDiskMetrics()
	metrics := command.DiskMetrics()
	if !metrics.Valid {
		t.Fatal("disk metrics should be valid for a temporary directory")
	}
	if metrics.UsageBytes != uint64(len(contents)) {
		t.Errorf("disk usage = %d, want %d", metrics.UsageBytes, len(contents))
	}
	if metrics.TotalBytes == 0 {
		t.Error("disk total bytes should be populated")
	}
	if metrics.MeasuredAt.IsZero() {
		t.Error("disk measured time should be populated")
	}
}

func TestCollectProcessMetrics_RealProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-process metrics test in short mode")
	}

	var sleepExec *exec.Cmd
	if runtime.GOOS == "windows" {
		// ping works without stdin in non-TTY contexts; -n 10 runs for ~10 seconds.
		sleepExec = exec.CommandContext(context.Background(), "ping", "-n", "10", "127.0.0.1")
	} else {
		sleepExec = exec.CommandContext(context.Background(), "sleep", "30")
	}

	errStart := sleepExec.Start()
	if errStart != nil {
		t.Skipf("could not start sleep process: %v", errStart)
	}
	t.Cleanup(func() {
		_ = sleepExec.Process.Kill()
		_ = sleepExec.Wait()
	})

	supervisorCmd := &Command{
		RWMutex:    &sync.RWMutex{},
		currentCMD: sleepExec,
	}

	time.Sleep(200 * time.Millisecond)

	supervisorCmd.collectProcessMetrics()
	cpu, _, _, memRSS, memVMS, _, _, threads, _, _, _, _, _, _, _, _, _, _, _ := supervisorCmd.Metrics()

	if memRSS == 0 {
		t.Errorf("expected non-zero working set (RSS) for running process, got %d", memRSS)
	}
	if memVMS == 0 {
		t.Errorf("expected non-zero private memory (VMS) for running process, got %d", memVMS)
	}
	if threads < 1 {
		t.Errorf("expected at least 1 thread, got %d", threads)
	}
	if cpu > 100 {
		t.Errorf("expected normalized CPU <= 100%%, got %v", cpu)
	}
}
