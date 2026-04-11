//go:build windows

package actions

import "golang.org/x/sys/windows"

func currentProcessIsElevated() bool {
	processToken := windows.GetCurrentProcessToken()
	return processToken.IsElevated()
}
