//go:build !windows

package supervisor

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	errKill := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(errKill, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return errKill
}
