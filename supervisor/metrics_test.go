package supervisor

import (
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
		tt := tt
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
	cpu, memRSS, memVMS, memPct, _, threads, _, _, _, _ := cmd.Metrics()
	if cpu != 0 || memRSS != 0 || memVMS != 0 || memPct != 0 || threads != 0 {
		t.Errorf("expected zero metrics for nil process, got cpu=%v memRSS=%v memVMS=%v memPct=%v threads=%v", cpu, memRSS, memVMS, memPct, threads)
	}
}

func TestCollectProcessMetrics_RealProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-process metrics test in short mode")
	}

	var sleepExec *exec.Cmd
	if runtime.GOOS == "windows" {
		// ping works without stdin in non-TTY contexts; -n 10 runs for ~10 seconds.
		sleepExec = exec.Command("ping", "-n", "10", "127.0.0.1")
	} else {
		sleepExec = exec.Command("sleep", "30")
	}

	if errStart := sleepExec.Start(); errStart != nil {
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
	cpu, memRSS, memVMS, _, _, threads, _, _, _, _ := supervisorCmd.Metrics()

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
