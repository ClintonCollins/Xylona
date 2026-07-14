//go:build !windows

package selfupdate

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

const processExitPollInterval = 50 * time.Millisecond

func waitForProcessExit(pid int, timeout time.Duration) error {
	if pid <= 0 {
		return fmt.Errorf("invalid process ID %d", pid)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(processExitPollInterval)
	defer ticker.Stop()

	for {
		errSignal := unix.Kill(pid, 0)
		if errors.Is(errSignal, unix.ESRCH) {
			return nil
		}
		if errSignal != nil && !errors.Is(errSignal, unix.EPERM) {
			return fmt.Errorf("inspect process %d: %w", pid, errSignal)
		}

		select {
		case <-timer.C:
			return fmt.Errorf("timed out after %s waiting for process %d", timeout, pid)
		case <-ticker.C:
		}
	}
}
