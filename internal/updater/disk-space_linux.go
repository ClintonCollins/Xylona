//go:build linux

package updater

import (
	"errors"
	"fmt"
	"math"
	"math/bits"

	"golang.org/x/sys/unix"
)

func availableDiskBytes(path string) (uint64, error) {
	var stat unix.Statfs_t
	errStat := unix.Statfs(path, &stat)
	if errStat != nil {
		return 0, fmt.Errorf("stat filesystem: %w", errStat)
	}
	if stat.Bsize <= 0 {
		return 0, errors.New("filesystem reported an invalid block size")
	}
	// #nosec G115 -- Statfs block size is verified positive before conversion.
	high, low := bits.Mul64(stat.Bavail, uint64(stat.Bsize))
	if high != 0 {
		return math.MaxUint64, nil
	}
	return low, nil
}

func sameFilesystemDevice(firstPath string, secondPath string) (bool, error) {
	var first unix.Stat_t
	errFirst := unix.Stat(firstPath, &first)
	if errFirst != nil {
		return false, fmt.Errorf("stat first volume path: %w", errFirst)
	}
	var second unix.Stat_t
	errSecond := unix.Stat(secondPath, &second)
	if errSecond != nil {
		return false, fmt.Errorf("stat second volume path: %w", errSecond)
	}
	return first.Dev == second.Dev, nil
}
