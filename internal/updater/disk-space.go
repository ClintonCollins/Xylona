package updater

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// ErrInsufficientDiskSpace is returned when an update volume cannot hold the
// files required by the next update phase.
var ErrInsufficientDiskSpace = errors.New("updater: insufficient disk space")

// EnsureFreeSpace verifies that path's volume has room for every requested
// allocation. Requirements are added with overflow checking.
func EnsureFreeSpace(path string, requiredBytes ...int64) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("updater: disk space path is required")
	}

	var required int64
	for _, value := range requiredBytes {
		if value < 0 {
			return fmt.Errorf("updater: disk space requirement cannot be negative: %d", value)
		}
		if value > math.MaxInt64-required {
			return errors.New("updater: disk space requirement overflow")
		}
		required += value
	}
	if required == 0 {
		return nil
	}

	available, errAvailable := availableDiskBytes(path)
	if errAvailable != nil {
		return fmt.Errorf("updater: inspect free disk space for %q: %w", path, errAvailable)
	}
	if available < uint64(required) {
		return fmt.Errorf(
			"%w on %q: %d bytes available, %d bytes required",
			ErrInsufficientDiskSpace,
			path,
			available,
			required,
		)
	}
	return nil
}

// PathsShareVolume reports whether both existing paths are backed by the same
// filesystem volume.
func PathsShareVolume(firstPath string, secondPath string) (bool, error) {
	firstPath = strings.TrimSpace(firstPath)
	secondPath = strings.TrimSpace(secondPath)
	if firstPath == "" || secondPath == "" {
		return false, errors.New("updater: both volume paths are required")
	}
	sameVolume, errVolume := sameFilesystemDevice(firstPath, secondPath)
	if errVolume != nil {
		return false, fmt.Errorf("updater: compare filesystem volumes: %w", errVolume)
	}
	return sameVolume, nil
}
