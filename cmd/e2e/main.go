package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Caller().Logger()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	ctx := context.Background()
	subcommand := os.Args[1]

	switch subcommand {
	case "single-setup":
		fs := flag.NewFlagSet("single-setup", flag.ExitOnError)
		httpPort := fs.Int("http-port", 9091, "Backend HTTP port for E2E")
		fedPort := fs.Int("fed-port", 9446, "Backend federation port for E2E")
		adminUsername := fs.String("admin-username", "admin", "Admin username")
		adminPassword := fs.String("admin-password", "admin", "Admin password")
		e2eDir := fs.String("e2e-dir", "frontend/e2e", "E2E test directory")
		projectRoot := fs.String("project-root", ".", "Project root directory")
		errParse := fs.Parse(os.Args[2:])
		if errParse != nil {
			log.Fatal().Err(errParse).Msg("Failed to parse flags")
		}
		errRun := runSingleSetup(ctx, *httpPort, *fedPort, *adminUsername, *adminPassword, *e2eDir, *projectRoot)
		if errRun != nil {
			log.Fatal().Err(errRun).Msg("Single setup failed")
		}

	case "single-teardown":
		fs := flag.NewFlagSet("single-teardown", flag.ExitOnError)
		fs.Int("http-port", 9091, "Backend HTTP port for E2E")
		fs.String("admin-username", "admin", "Admin username")
		fs.String("admin-password", "admin", "Admin password")
		e2eDir := fs.String("e2e-dir", "frontend/e2e", "E2E test directory")
		errParse := fs.Parse(os.Args[2:])
		if errParse != nil {
			log.Fatal().Err(errParse).Msg("Failed to parse flags")
		}
		runSingleTeardown(*e2eDir)

	case "hub-spoke-setup":
		fs := flag.NewFlagSet("hub-spoke-setup", flag.ExitOnError)
		httpPort := fs.Int("http-port", 9091, "Controller HTTP port")
		nodePort := fs.Int("node-port", 9501, "Remote xylona-node HTTPS port")
		adminUsername := fs.String("admin-username", "admin", "Admin username")
		adminPassword := fs.String("admin-password", "admin", "Admin password")
		e2eDir := fs.String("e2e-dir", "frontend/e2e", "E2E test directory")
		projectRoot := fs.String("project-root", ".", "Project root directory")
		errParse := fs.Parse(os.Args[2:])
		if errParse != nil {
			log.Fatal().Err(errParse).Msg("Failed to parse flags")
		}
		errRun := runHubSpokeSetup(ctx, *httpPort, *nodePort, *adminUsername, *adminPassword, *e2eDir, *projectRoot)
		if errRun != nil {
			log.Fatal().Err(errRun).Msg("Hub-spoke setup failed")
		}

	case "hub-spoke-teardown":
		fs := flag.NewFlagSet("hub-spoke-teardown", flag.ExitOnError)
		e2eDir := fs.String("e2e-dir", "frontend/e2e", "E2E test directory")
		errParse := fs.Parse(os.Args[2:])
		if errParse != nil {
			log.Fatal().Err(errParse).Msg("Failed to parse flags")
		}
		runHubSpokeTeardown(*e2eDir)

	case "seed":
		fs := flag.NewFlagSet("seed", flag.ExitOnError)
		dbPath := fs.String("db", "", "Path to SQLite database file (required)")
		username := fs.String("username", "admin", "Admin username")
		password := fs.String("password", "admin", "Admin password")
		migrationsDir := fs.String("migrations", "sql/migrations", "Path to SQL migrations directory")
		errParse := fs.Parse(os.Args[2:])
		if errParse != nil {
			log.Fatal().Err(errParse).Msg("Failed to parse flags")
		}
		if *dbPath == "" {
			fmt.Fprintln(os.Stderr, "error: -db flag is required")
			fs.Usage()
			os.Exit(1)
		}
		errRun := runSeed(*dbPath, *username, *password, *migrationsDir)
		if errRun != nil {
			log.Fatal().Err(errRun).Msg("Seed failed")
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: e2e <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Subcommands:")
	fmt.Fprintln(os.Stderr, "  single-setup         Set up single-node E2E test environment")
	fmt.Fprintln(os.Stderr, "  single-teardown      Tear down single-node E2E test environment")
	fmt.Fprintln(os.Stderr, "  hub-spoke-setup      Spawn controller + remote xylona-node for E2E")
	fmt.Fprintln(os.Stderr, "  hub-spoke-teardown   Tear down hub-spoke E2E environment")
	fmt.Fprintln(os.Stderr, "  seed                 Seed a database with an admin user")
}
