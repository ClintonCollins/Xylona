package main

import (
	"os/exec"
	"strconv"
)

// killProcess terminates cmd and its descendants. On Windows the Go
// stdlib's cmd.Process.Kill() only signals the top-level PID, which leaves
// child processes orphaned; taskkill /T walks the process tree. We fall
// back to Kill() when taskkill is unavailable.
func killProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	pid := cmd.Process.Pid
	if pid <= 0 {
		return
	}

	//nolint:gosec,noctx // PID comes from os.Process and taskkill has no context-aware API on Windows.
	killCmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F", "/T")
	errKill := killCmd.Run()
	if errKill != nil {
		_ = cmd.Process.Kill()
	}
}
