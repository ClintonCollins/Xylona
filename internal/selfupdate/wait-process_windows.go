//go:build windows

package selfupdate

import (
	"errors"
	"fmt"
	"math"
	"time"

	"golang.org/x/sys/windows"
)

func waitForProcessExit(pid int, timeout time.Duration) error {
	if pid <= 0 {
		return fmt.Errorf("invalid process ID %d", pid)
	}
	if uint64(pid) > math.MaxUint32 {
		return fmt.Errorf("process ID %d exceeds the Windows process ID range", pid)
	}
	waitMilliseconds := timeout.Milliseconds()
	if waitMilliseconds <= 0 || waitMilliseconds > math.MaxUint32 {
		return fmt.Errorf("wait timeout %s is outside the Windows wait range", timeout)
	}

	// #nosec G115 -- pid is range-checked against MaxUint32 above.
	handle, errOpen := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if errors.Is(errOpen, windows.ERROR_INVALID_PARAMETER) {
		return nil
	}
	if errOpen != nil {
		return fmt.Errorf("open process %d: %w", pid, errOpen)
	}

	// #nosec G115 -- waitMilliseconds is range-checked against MaxUint32 above.
	event, errWait := windows.WaitForSingleObject(handle, uint32(waitMilliseconds))
	errClose := windows.CloseHandle(handle)
	if errWait != nil {
		return errors.Join(fmt.Errorf("wait for process %d: %w", pid, errWait), errClose)
	}
	if event == uint32(windows.WAIT_TIMEOUT) {
		return errors.Join(fmt.Errorf("timed out after %s waiting for process %d", timeout, pid), errClose)
	}
	if event != windows.WAIT_OBJECT_0 {
		return errors.Join(fmt.Errorf("wait for process %d returned status %#x", pid, event), errClose)
	}
	if errClose != nil {
		return fmt.Errorf("close process %d handle: %w", pid, errClose)
	}
	return nil
}
