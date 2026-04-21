package main

import (
	"fmt"

	"github.com/ClintonCollins/Xylona/cmd/e2e/seedutil"
)

func runSeed(dbPath, username, password, migrationsDir string) error {
	errRun := seedutil.Run(dbPath, username, password, migrationsDir)
	if errRun != nil {
		return fmt.Errorf("run seed utility: %w", errRun)
	}
	return nil
}
