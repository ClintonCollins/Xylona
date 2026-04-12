//go:build !windows

package db

import (
	"fmt"
	"math"
	"os"

	"golang.org/x/sys/unix"
)

func lockFileExclusiveNonBlocking(file *os.File) error {
	fd, errFileDescriptor := fileDescriptor(file)
	if errFileDescriptor != nil {
		return errFileDescriptor
	}

	errLock := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
	if errLock != nil {
		return fmt.Errorf(`lock file: %w`, errLock)
	}
	return nil
}

func unlockFile(file *os.File) error {
	fd, errFileDescriptor := fileDescriptor(file)
	if errFileDescriptor != nil {
		return errFileDescriptor
	}

	errUnlock := unix.Flock(fd, unix.LOCK_UN)
	if errUnlock != nil {
		return fmt.Errorf(`unlock file: %w`, errUnlock)
	}
	return nil
}

func fileDescriptor(file *os.File) (int, error) {
	fd := file.Fd()
	if fd > uintptr(math.MaxInt) {
		return 0, fmt.Errorf(`file descriptor %d exceeds int range`, fd)
	}

	return int(fd), nil
}
