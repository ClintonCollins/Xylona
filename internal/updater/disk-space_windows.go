//go:build windows

package updater

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

func availableDiskBytes(path string) (uint64, error) {
	pathPointer, errPointer := windows.UTF16PtrFromString(path)
	if errPointer != nil {
		return 0, fmt.Errorf("encode disk space path: %w", errPointer)
	}
	var available uint64
	errSpace := windows.GetDiskFreeSpaceEx(pathPointer, &available, nil, nil)
	if errSpace != nil {
		return 0, fmt.Errorf("get disk free space: %w", errSpace)
	}
	return available, nil
}

func sameFilesystemDevice(firstPath string, secondPath string) (bool, error) {
	firstVolume, errFirst := volumePath(firstPath)
	if errFirst != nil {
		return false, errFirst
	}
	secondVolume, errSecond := volumePath(secondPath)
	if errSecond != nil {
		return false, errSecond
	}
	return strings.EqualFold(firstVolume, secondVolume), nil
}

func volumePath(path string) (string, error) {
	pathPointer, errPointer := windows.UTF16PtrFromString(path)
	if errPointer != nil {
		return "", fmt.Errorf("encode volume path: %w", errPointer)
	}
	const bufferLength = windows.MAX_PATH + 1
	buffer := make([]uint16, bufferLength)
	errVolume := windows.GetVolumePathName(pathPointer, &buffer[0], bufferLength)
	if errVolume != nil {
		return "", fmt.Errorf("get volume path: %w", errVolume)
	}
	return windows.UTF16ToString(buffer), nil
}
