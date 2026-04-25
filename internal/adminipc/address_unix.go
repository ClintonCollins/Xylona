//go:build !windows

package adminipc

import (
	"path/filepath"
)

func endpointForResolvedDatabasePath(resolvedDBPath string) string {
	return filepath.Join(filepath.Dir(resolvedDBPath), `.xyadm-`+endpointHash(resolvedDBPath))
}

func endpointHashInput(resolvedDBPath string) string {
	return filepath.Clean(resolvedDBPath)
}
