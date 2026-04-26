package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
)

func runTeardown(e2eDir string, mode e2eMode) {
	log.Info().Str("mode", string(mode)).Msg("[E2E Teardown] Starting cleanup")

	paths := resolveSetupPaths(e2eDir, mode)
	killByPIDFile(filepath.Join(paths.nodeDir, "xylona-node.pid"), "remote xylona-node")
	killByPIDFile(filepath.Join(paths.controllerDir, "xylona.pid"), "controller")

	errRemove := removeAllWithRetry(paths.dataDir, 10, 300*time.Millisecond)
	if errRemove != nil {
		log.Warn().Err(errRemove).Str("path", paths.dataDir).Msg("[E2E Teardown] Could not remove E2E data dir")
	}

	cleanFrontendDist(e2eDir)
	releaseLock(e2eDir, string(mode))
	log.Info().Str("mode", string(mode)).Msg("[E2E Teardown] Teardown complete")
}

func removeAllWithRetry(path string, attempts int, delay time.Duration) error {
	var lastErr error
	for range attempts {
		lastErr = os.RemoveAll(path)
		if lastErr == nil {
			return nil
		}
		if os.IsNotExist(lastErr) {
			return nil
		}
		time.Sleep(delay)
	}
	return fmt.Errorf("remove %s: %w", path, lastErr)
}
