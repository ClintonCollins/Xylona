//go:build windows

// Package adminipc provides the host-local user-management transport used by
// `xylona user` live mode.
package adminipc

import (
	"path/filepath"
	"strings"
)

func endpointForResolvedDatabasePath(resolvedDBPath string) string {
	return `\\.\pipe\xylona-admin-` + endpointHash(resolvedDBPath)
}

func endpointHashInput(resolvedDBPath string) string {
	return strings.ToLower(filepath.Clean(resolvedDBPath))
}
