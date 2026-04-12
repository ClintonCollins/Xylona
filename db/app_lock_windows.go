//go:build windows

package db

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func lockFileExclusiveNonBlocking(file *os.File) error {
	overlapped := windows.Overlapped{}
	errLock := windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&overlapped,
	)
	if errLock != nil {
		return fmt.Errorf(`lock file: %w`, errLock)
	}
	return nil
}

func unlockFile(file *os.File) error {
	overlapped := windows.Overlapped{}
	errUnlock := windows.UnlockFileEx(
		windows.Handle(file.Fd()),
		0,
		1,
		0,
		&overlapped,
	)
	if errUnlock != nil {
		return fmt.Errorf(`unlock file: %w`, errUnlock)
	}
	return nil
}
