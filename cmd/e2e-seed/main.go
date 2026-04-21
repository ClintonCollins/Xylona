// Command e2e-seed seeds a Xylona database for end-to-end tests.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/cmd/e2e/seedutil"
)

func main() {
	dbPath := flag.String("db", "", "Path to the SQLite database file (required)")
	username := flag.String("username", "admin", "Admin username to create")
	password := flag.String("password", "admin", "Admin password")
	migrationsDir := flag.String("migrations", "sql/migrations", "Path to SQL migrations directory")
	flag.Parse()

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Caller().Logger()

	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "error: -db flag is required")
		flag.Usage()
		os.Exit(1)
	}

	errRun := runSeedCommand(*dbPath, *username, *password, *migrationsDir)
	if errRun != nil {
		log.Fatal().Err(errRun).Msg("Failed to seed database")
	}
}

func runSeedCommand(dbPath string, username string, password string, migrationsDir string) error {
	errRun := seedutil.Run(dbPath, username, password, migrationsDir)
	if errRun != nil {
		return fmt.Errorf("run seed utility: %w", errRun)
	}
	return nil
}
