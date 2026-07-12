//go:build !windows

package supervisor

import (
	"errors"
	"syscall"
)

func processRunning(pid int) bool {
	errSignal := syscall.Kill(pid, 0)
	return errSignal == nil || errors.Is(errSignal, syscall.EPERM)
}
