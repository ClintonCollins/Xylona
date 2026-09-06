package diagnosis

import (
	"errors"

	"golang.org/x/sys/windows"
)

func platformErrorCategory(err error) string {
	switch {
	case errors.Is(err, windows.ERROR_DISK_FULL), errors.Is(err, windows.ERROR_HANDLE_DISK_FULL):
		return CategoryDiskFull
	case errors.Is(err, windows.WSAEADDRINUSE):
		return CategoryPortInUse
	default:
		return CategoryUnknown
	}
}
