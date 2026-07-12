//go:build windows

package supervisor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

const windowsProcessStillActive = 259

func configureProcessTree(_ *exec.Cmd) {}

func terminateProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}

	pid := cmd.Process.Pid
	running, errRunning := windowsProcessRunning(pid)
	if errRunning == nil && !running {
		return os.ErrProcessDone
	}
	// taskkill /T walks the descendant tree before terminating the root process.
	//nolint:gosec,noctx // PID comes from os.Process and taskkill has no context-aware API.
	taskkill := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
	errTaskkill := taskkill.Run()
	if errTaskkill == nil {
		return nil
	}

	errKill := cmd.Process.Kill()
	if errors.Is(errKill, os.ErrProcessDone) || errors.Is(errKill, syscall.EINVAL) {
		return os.ErrProcessDone
	}
	if errKill != nil {
		return errors.Join(
			fmt.Errorf("terminate process tree %d: %w", pid, errTaskkill),
			fmt.Errorf("terminate root process %d: %w", pid, errKill),
		)
	}
	return fmt.Errorf("terminate process tree %d: %w", pid, errTaskkill)
}

func windowsProcessRunning(pid int) (bool, error) {
	if pid <= 0 || int64(pid) > int64(^uint32(0)) {
		return false, fmt.Errorf("invalid process ID %d", pid)
	}
	// The bounds check above guarantees that this conversion cannot truncate.
	processID := uint32(pid)
	handle, errOpen := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, processID)
	if errors.Is(errOpen, windows.ERROR_INVALID_PARAMETER) {
		return false, nil
	}
	if errOpen != nil {
		return false, fmt.Errorf("open process %d: %w", pid, errOpen)
	}
	var exitCode uint32
	errExitCode := windows.GetExitCodeProcess(handle, &exitCode)
	errClose := windows.CloseHandle(handle)
	if errExitCode != nil || errClose != nil {
		return false, errors.Join(errExitCode, errClose)
	}
	return exitCode == windowsProcessStillActive, nil
}
