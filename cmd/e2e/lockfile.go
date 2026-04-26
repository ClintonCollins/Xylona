package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// lockInfo is the JSON structure written to lock files.
type lockInfo struct {
	PID           int            `json:"pid"`
	ControllerPID int            `json:"controllerPid,omitempty"`
	RemoteNodePID int            `json:"remoteNodePid,omitempty"`
	Suite         string         `json:"suite"`
	Ports         map[string]int `json:"ports"`
	StartedAt     string         `json:"startedAt"`
}

// lockFilePath returns the canonical lock file path for a given suite.
func lockFilePath(e2eDir, suite string) string {
	return filepath.Join(e2eDir, ".e2e-"+suite+".lock")
}

// acquireLock attempts to create a lock file for the given suite.
// If a lock already exists and the owning process is still alive, it returns
// an error with a descriptive message. If the lock is stale (process dead),
// it cleans up and proceeds.
func acquireLock(e2eDir, suite string, ports map[string]int) error {
	lockPath := lockFilePath(e2eDir, suite)

	data, errRead := os.ReadFile(lockPath)
	if errRead == nil {
		// Lock file exists — check if the process is still alive.
		var existing lockInfo
		errParse := json.Unmarshal(data, &existing)
		if errParse == nil {
			existing.mergeStatePIDs(e2eDir, suite)
			activePID, activeLabel, active := existing.activeProcess()
			if active {
				return fmt.Errorf(
					"E2E suite %q is already running (%s PID %d, started %s, ports: %s). "+
						"Wait for it to finish or kill PID %d to proceed",
					existing.Suite,
					activeLabel,
					activePID,
					existing.StartedAt,
					formatPorts(existing.Ports),
					activePID,
				)
			}
			// Process is dead — stale lock.
			log.Warn().
				Int("setup_pid", existing.PID).
				Int("controller_pid", existing.ControllerPID).
				Int("remote_node_pid", existing.RemoteNodePID).
				Str("suite", existing.Suite).
				Msg("Found stale lock file (no tracked processes are running); cleaning up")
		}
		// Lock file is unparseable or stale — remove it.
		_ = os.Remove(lockPath)
	}

	// Write the new lock file.
	info := lockInfo{
		PID:       os.Getpid(),
		Suite:     suite,
		Ports:     ports,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	errWriteLock := writeLockInfo(lockPath, info)
	if errWriteLock != nil {
		return errWriteLock
	}

	log.Info().Str("path", lockPath).Msg("Acquired E2E lock")
	return nil
}

func updateLockRuntimePIDs(e2eDir, suite string, state *testState) error {
	lockPath := lockFilePath(e2eDir, suite)
	data, errRead := os.ReadFile(lockPath)
	if errRead != nil {
		return fmt.Errorf("read lock file: %w", errRead)
	}

	info := lockInfo{}
	errUnmarshal := json.Unmarshal(data, &info)
	if errUnmarshal != nil {
		return fmt.Errorf("unmarshal lock info: %w", errUnmarshal)
	}

	info.ControllerPID = state.ControllerPID
	info.RemoteNodePID = state.RemoteNodePID
	errWrite := writeLockInfo(lockPath, info)
	if errWrite != nil {
		return errWrite
	}
	return nil
}

func writeLockInfo(lockPath string, info lockInfo) error {
	encoded, errMarshal := json.MarshalIndent(info, "", "  ")
	if errMarshal != nil {
		return fmt.Errorf("marshal lock info: %w", errMarshal)
	}
	errWrite := os.WriteFile(lockPath, encoded, 0o600)
	if errWrite != nil {
		return fmt.Errorf("write lock file: %w", errWrite)
	}
	return nil
}

func (info *lockInfo) mergeStatePIDs(e2eDir, suite string) {
	state, errState := loadTestState(e2eDir)
	if errState != nil {
		return
	}
	if state.Mode != suite {
		return
	}
	if info.ControllerPID == 0 {
		info.ControllerPID = state.ControllerPID
	}
	if info.RemoteNodePID == 0 {
		info.RemoteNodePID = state.RemoteNodePID
	}
}

func (info *lockInfo) activeProcess() (int, string, bool) {
	candidates := []struct {
		pid   int
		label string
	}{
		{pid: info.ControllerPID, label: "controller"},
		{pid: info.RemoteNodePID, label: "remote node"},
		{pid: info.PID, label: "setup"},
	}

	for _, candidate := range candidates {
		if candidate.pid <= 0 {
			continue
		}
		if processIsAlive(candidate.pid) {
			return candidate.pid, candidate.label, true
		}
	}

	return 0, "", false
}

// releaseLock removes the lock file for the given suite.
// It is safe to call even if the lock file does not exist.
func releaseLock(e2eDir, suite string) {
	lockPath := lockFilePath(e2eDir, suite)
	errRemove := os.Remove(lockPath)
	if errRemove != nil && !os.IsNotExist(errRemove) {
		log.Warn().Err(errRemove).Str("path", lockPath).Msg("Could not remove lock file")
		return
	}
	if errRemove == nil {
		log.Info().Str("path", lockPath).Msg("Released E2E lock")
	}
}

// processIsAlive checks whether a process with the given PID is still running.
func processIsAlive(pid int) bool {
	if runtime.GOOS == "windows" {
		return processIsAliveWindows(pid)
	}
	// On Linux/macOS, check if /proc/<pid> exists or use kill -0.
	_, errStat := os.Stat("/proc/" + strconv.Itoa(pid))
	return errStat == nil
}

// processIsAliveWindows uses tasklist to check if a PID exists on Windows.
func processIsAliveWindows(pid int) bool {
	if pid <= 0 {
		return false
	}

	//nolint:gosec,noctx // PID is validated numeric input and tasklist has no context-aware API on Windows.
	cmd := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/NH")
	out, errRun := cmd.Output()
	if errRun != nil {
		return false
	}
	// tasklist outputs "INFO: No tasks are running..." when the PID doesn't exist.
	return !strings.Contains(string(out), "No tasks")
}

// formatPorts renders a port map as a human-readable string.
func formatPorts(ports map[string]int) string {
	if len(ports) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(ports))
	for name, port := range ports {
		parts = append(parts, fmt.Sprintf("%s=%d", name, port))
	}
	return strings.Join(parts, ", ")
}
