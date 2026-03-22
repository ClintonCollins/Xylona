package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

func runSingleTeardown(_ context.Context, _ int, _, _, e2eDir string) error {
	log.Info().Msg("[E2E Teardown] Starting cleanup...")

	dataDir := filepath.Join(e2eDir, ".e2e-data")

	// Kill the backend process.
	pidFile := filepath.Join(dataDir, "xylona.pid")
	killByPIDFile(pidFile, "E2E Backend")

	// Clean up data directory.
	log.Info().Msg("[E2E Teardown] Cleaning up E2E data...")
	errRemove := os.RemoveAll(dataDir)
	if errRemove != nil {
		log.Warn().Err(errRemove).Msg("[E2E Teardown] Warning: could not fully clean data dir")
	}

	// Clean the frontend dist built during setup so stale artifacts don't linger.
	// Leave a .gitkeep so the embed directive in embed.go doesn't break go build.
	cleanFrontendDist(e2eDir)

	// Release the suite lock.
	releaseLock(e2eDir, "single")

	log.Info().Msg("[E2E Teardown] Teardown complete")
	return nil
}
