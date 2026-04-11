//go:build !windows

package actions

import "os"

func currentProcessIsElevated() bool {
	return os.Geteuid() == 0
}
