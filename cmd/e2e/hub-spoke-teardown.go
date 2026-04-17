package main

import (
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

// runHubSpokeTeardown kills the controller and xylona-node processes that
// runHubSpokeSetup started, then removes the data directory tree. Safe to
// call even if setup failed halfway through — missing PID files are
// ignored.
func runHubSpokeTeardown(e2eDir string) {
	log.Info().Msg("[Hub-Spoke Teardown] Starting cleanup...")

	dataDir := filepath.Join(e2eDir, ".e2e-data-hub-spoke")
	controllerDir := filepath.Join(dataDir, "controller")
	nodeDir := filepath.Join(dataDir, "node")

	killByPIDFile(filepath.Join(nodeDir, "xylona-node.pid"), "xylona-node")
	killByPIDFile(filepath.Join(controllerDir, "xylona.pid"), "Controller")

	errRemove := os.RemoveAll(dataDir)
	if errRemove != nil {
		log.Warn().Err(errRemove).Msg("[Hub-Spoke Teardown] Warning: could not fully clean data dir")
	}

	cleanFrontendDist(e2eDir)
	releaseLock(e2eDir, "hub-spoke")

	log.Info().Msg("[Hub-Spoke Teardown] Teardown complete")
}
