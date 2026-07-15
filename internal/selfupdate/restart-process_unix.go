//go:build !windows

package selfupdate

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func platformUsesInProcessRestart() bool {
	return true
}

func restartCurrentProcess(markerPath string, pending pendingUpdate) error {
	return replaceAndRestartCurrentProcess(markerPath, pending, execCurrentProcess)
}

func execCurrentProcess(pending pendingUpdate) error {
	errChdir := os.Chdir(pending.WorkingDirectory)
	if errChdir != nil {
		return fmt.Errorf("change working directory: %w", errChdir)
	}
	args := append([]string{pending.ExecutablePath}, pending.RestartArgs...)
	errExec := unix.Exec(pending.ExecutablePath, args, os.Environ())
	if errExec != nil {
		return fmt.Errorf("exec process: %w", errExec)
	}
	return nil
}
