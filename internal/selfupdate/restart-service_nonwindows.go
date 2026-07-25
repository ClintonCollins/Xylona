//go:build !windows

package selfupdate

import "errors"

func restartWindowsService(string) error {
	return errors.New("windows service restart is unavailable on this operating system")
}
