//go:build windows

package supervisor

func processRunning(pid int) bool {
	running, errRunning := windowsProcessRunning(pid)
	return errRunning == nil && running
}
