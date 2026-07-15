//go:build windows

package selfupdate

import "errors"

func platformUsesInProcessRestart() bool {
	return false
}

func restartCurrentProcess(string, pendingUpdate) error {
	return errors.New("selfupdate: in-process restart is unavailable on Windows")
}
