//go:build !windows

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	pty "github.com/aymanbagabas/go-pty"
)

func configureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func prepareProcessInvocation(baseCommand string, args []string) (string, []string, error) {
	return baseCommand, append([]string(nil), args...), nil
}

func requiresPseudoTerminal(serviceID string) bool {
	return serviceID == "terraria"
}

func preparePseudoTerminalDrain(terminal pty.Pty) (bool, error) {
	unixTerminal, isUnixTerminal := terminal.(pty.UnixPty)
	if !isUnixTerminal {
		return false, errors.New("pseudo-terminal does not expose Unix handles")
	}
	// The child retains its inherited slave descriptor after Start. Release the
	// parent's duplicate so master reads can finish after the child exits.
	errCloseSlave := unixTerminal.Slave().Close()
	if errCloseSlave != nil && !errors.Is(errCloseSlave, os.ErrClosed) {
		return false, fmt.Errorf("close parent pseudo-terminal slave: %w", errCloseSlave)
	}
	return true, nil
}

func closePseudoTerminal(terminal pty.Pty, slaveReleased bool) error {
	if !slaveReleased {
		errClose := terminal.Close()
		if errClose != nil {
			return fmt.Errorf("close pseudo-terminal: %w", errClose)
		}
		return nil
	}
	unixTerminal, isUnixTerminal := terminal.(pty.UnixPty)
	if !isUnixTerminal {
		return errors.New("pseudo-terminal does not expose Unix handles")
	}
	errCloseMaster := unixTerminal.Master().Close()
	if errCloseMaster != nil && !errors.Is(errCloseMaster, os.ErrClosed) {
		return fmt.Errorf("close pseudo-terminal master: %w", errCloseMaster)
	}
	return nil
}

func preparePseudoTerminalCommand(
	ctx context.Context,
	terminal pty.Pty,
	baseCommand string,
	args []string,
) *pty.Cmd {
	return terminal.CommandContext(ctx, baseCommand, args...)
}

func watchPseudoTerminalCancellation(_ context.Context, _ *pty.Cmd) func() {
	return func() {}
}

// RunConsoleSignalHelper is a no-op outside Windows.
func RunConsoleSignalHelper() (bool, int) {
	return false, 0
}

func mirrorProcessConsoleInput(_ *exec.Cmd, _ string) (bool, error) {
	return false, nil
}

func interruptProcessTree(process *os.Process) error {
	if process == nil {
		return os.ErrProcessDone
	}
	errInterrupt := syscall.Kill(-process.Pid, syscall.SIGINT)
	if errInterrupt == nil {
		return nil
	}
	if errors.Is(errInterrupt, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	errTerminate := syscall.Kill(-process.Pid, syscall.SIGTERM)
	if errTerminate == nil {
		return nil
	}
	if errors.Is(errTerminate, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return errors.Join(errInterrupt, errTerminate)
}

func terminateProcessTree(process *os.Process) error {
	if process == nil {
		return os.ErrProcessDone
	}
	errKill := syscall.Kill(-process.Pid, syscall.SIGKILL)
	if errKill == nil {
		return nil
	}
	if errors.Is(errKill, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return fmt.Errorf("terminate process group for PID %d: %w", process.Pid, errKill)
}
