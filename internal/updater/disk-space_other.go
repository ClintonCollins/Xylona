//go:build !linux && !windows

package updater

import (
	"errors"
	"runtime"
)

func availableDiskBytes(string) (uint64, error) {
	return 0, errors.New("free disk space checks are unsupported on " + runtime.GOOS)
}

func sameFilesystemDevice(string, string) (bool, error) {
	return false, errors.New("filesystem volume comparison is unsupported on " + runtime.GOOS)
}
