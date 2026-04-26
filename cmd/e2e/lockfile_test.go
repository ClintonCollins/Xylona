package main

import (
	"os"
	"strings"
	"testing"
)

func TestAcquireLockBlocksOnRuntimeControllerPIDFromState(t *testing.T) {
	e2eDir := t.TempDir()
	suite := string(e2eModeLocalController)

	state := &testState{
		Mode:          suite,
		ControllerPID: os.Getpid(),
	}
	errSave := saveTestState(e2eDir, state)
	if errSave != nil {
		t.Fatalf("saveTestState() error = %v", errSave)
	}

	info := lockInfo{
		PID:       0,
		Suite:     suite,
		Ports:     map[string]int{"http": 9091},
		StartedAt: "2026-04-25T00:00:00Z",
	}
	errWrite := writeLockInfo(lockFilePath(e2eDir, suite), info)
	if errWrite != nil {
		t.Fatalf("writeLockInfo() error = %v", errWrite)
	}

	errAcquire := acquireLock(e2eDir, suite, map[string]int{"http": 9092})
	if errAcquire == nil {
		t.Fatal("acquireLock() error = nil, want running controller lock error")
	}
	if !strings.Contains(errAcquire.Error(), "controller PID") {
		t.Fatalf("acquireLock() error = %q, want controller PID context", errAcquire.Error())
	}
}
