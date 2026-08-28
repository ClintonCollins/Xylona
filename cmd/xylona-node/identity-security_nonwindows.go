//go:build !windows

package main

import (
	"fmt"
	"os"
)

func ensureIdentityDataDir(dataDir string) error {
	errMkdir := os.MkdirAll(dataDir, 0o700)
	if errMkdir != nil {
		return fmt.Errorf("create identity data directory: %w", errMkdir)
	}
	return nil
}

func protectIdentityPathSecurity(_ string, _ bool) error {
	return nil
}
