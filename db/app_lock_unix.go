//go:build !windows

package db

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func lockFileExclusiveNonBlocking(file *os.File) error {
	errLock := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errLock != nil {
		return fmt.Errorf(`lock file: %w`, errLock)
	}
	return nil
}

func unlockFile(file *os.File) error {
	errUnlock := unix.Flock(int(file.Fd()), unix.LOCK_UN)
	if errUnlock != nil {
		return fmt.Errorf(`unlock file: %w`, errUnlock)
	}
	return nil
}
