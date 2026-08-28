//go:build !windows

package main

import "os"

func ensureIdentityDataDir(dataDir string) error {
	return os.MkdirAll(dataDir, 0o700)
}

func protectIdentityPathSecurity(_ string, _ bool) error {
	return nil
}
