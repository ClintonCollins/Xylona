package supervisor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	processTreeHelperEnv  = "XYLONA_PROCESS_TREE_HELPER"
	processTreePIDFileEnv = "XYLONA_PROCESS_TREE_PID_FILE"
)

func TestProcessTreeCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping process-tree integration test in short mode")
	}

	executable, errExecutable := os.Executable()
	if errExecutable != nil {
		t.Fatalf("os.Executable() error = %v", errExecutable)
	}
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestProcessTreeHelper$")
	cmd.Env = append(os.Environ(), processTreeHelperEnv+"=parent", processTreePIDFileEnv+"="+pidFile)
	configureProcessTree(cmd)
	cmd.Cancel = func() error {
		return terminateProcessTree(cmd)
	}
	errStart := cmd.Start()
	if errStart != nil {
		cancel()
		t.Fatalf("start process-tree helper: %v", errStart)
	}

	childPID := waitForProcessTreeChild(t, pidFile)
	cancel()
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("process-tree root did not stop after context cancellation")
	}

	deadline := time.Now().Add(5 * time.Second)
	for processRunning(childPID) && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if processRunning(childPID) {
		t.Fatalf("child process %d remained alive after process-tree cancellation", childPID)
	}
}

func TestProcessTreeHelper(_ *testing.T) {
	role := os.Getenv(processTreeHelperEnv)
	if role == "" {
		return
	}
	if role == "child" {
		for {
			time.Sleep(time.Hour)
		}
	}
	if role != "parent" {
		os.Exit(2)
	}

	executable, errExecutable := os.Executable()
	if errExecutable != nil {
		os.Exit(3)
	}
	child := exec.CommandContext(context.Background(), executable, "-test.run=^TestProcessTreeHelper$")
	child.Env = append(os.Environ(), processTreeHelperEnv+"=child")
	errStart := child.Start()
	if errStart != nil {
		os.Exit(4)
	}
	pidFile := os.Getenv(processTreePIDFileEnv)
	errWrite := os.WriteFile(pidFile, []byte(strconv.Itoa(child.Process.Pid)), 0o600) //nolint:gosec // Parent supplies this test-only path from t.TempDir.
	if errWrite != nil {
		os.Exit(5)
	}
	errWait := child.Wait()
	if errWait != nil {
		os.Exit(6)
	}
	os.Exit(0)
}

func waitForProcessTreeChild(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, errRead := os.ReadFile(pidFile)
		if errRead == nil {
			pid, errPID := strconv.Atoi(strings.TrimSpace(string(data)))
			if errPID != nil {
				t.Fatalf("parse child PID %q: %v", data, errPID)
			}
			return pid
		}
		if !os.IsNotExist(errRead) {
			t.Fatalf("read child PID file: %v", errRead)
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("child PID file %q was not created", pidFile)
	return 0
}
