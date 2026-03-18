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
		backendURL := fs.String("backend-url", "http://localhost:8080", "Backend URL")
		adminUsername := fs.String("admin-username", "admin", "Admin username")
		adminPassword := fs.String("admin-password", "admin", "Admin password")
		e2eDir := fs.String("e2e-dir", "frontend/e2e", "E2E test directory")
		projectRoot := fs.String("project-root", ".", "Project root directory")
		errParse := fs.Parse(os.Args[2:])
		if errParse != nil {
			log.Fatal().Err(errParse).Msg("Failed to parse flags")
		}
		errRun := runSingleSetup(ctx, *backendURL, *adminUsername, *adminPassword, *e2eDir, *projectRoot)
		if errRun != nil {
			log.Fatal().Err(errRun).Msg("Single setup failed")
		}

	case "single-teardown":
		fs := flag.NewFlagSet("single-teardown", flag.ExitOnError)
		backendURL := fs.String("backend-url", "http://localhost:8080", "Backend URL")
		adminUsername := fs.String("admin-username", "admin", "Admin username")
		adminPassword := fs.String("admin-password", "admin", "Admin password")
		e2eDir := fs.String("e2e-dir", "frontend/e2e", "E2E test directory")
		errParse := fs.Parse(os.Args[2:])
		if errParse != nil {
			log.Fatal().Err(errParse).Msg("Failed to parse flags")
		}
		errRun := runSingleTeardown(ctx, *backendURL, *adminUsername, *adminPassword, *e2eDir)
		if errRun != nil {
			log.Fatal().Err(errRun).Msg("Single teardown failed")
		}

	case "federation-setup":
		fs := flag.NewFlagSet("federation-setup", flag.ExitOnError)
		e2eDir := fs.String("e2e-dir", "frontend/e2e", "E2E test directory")
		projectRoot := fs.String("project-root", ".", "Project root directory")
		nodeAPort := fs.Int("node-a-port", 9081, "Node A HTTP port")
		nodeBPort := fs.Int("node-b-port", 9082, "Node B HTTP port")
		nodeAFedPort := fs.Int("node-a-fed-port", 9444, "Node A federation port")
		nodeBFedPort := fs.Int("node-b-fed-port", 9445, "Node B federation port")
		errParse := fs.Parse(os.Args[2:])
		if errParse != nil {
			log.Fatal().Err(errParse).Msg("Failed to parse flags")
		}
		errRun := runFederationSetup(ctx, *e2eDir, *projectRoot, *nodeAPort, *nodeBPort, *nodeAFedPort, *nodeBFedPort)
		if errRun != nil {
			log.Fatal().Err(errRun).Msg("Federation setup failed")
		}

	case "federation-teardown":
		fs := flag.NewFlagSet("federation-teardown", flag.ExitOnError)
		e2eDir := fs.String("e2e-dir", "frontend/e2e", "E2E test directory")
		keepData := fs.Bool("keep-data", false, "Preserve federation data for debugging")
		nodeAPort := fs.Int("node-a-port", 9081, "Node A HTTP port")
		nodeBPort := fs.Int("node-b-port", 9082, "Node B HTTP port")
		errParse := fs.Parse(os.Args[2:])
		if errParse != nil {
			log.Fatal().Err(errParse).Msg("Failed to parse flags")
		}
		errRun := runFederationTeardown(ctx, *e2eDir, *keepData, *nodeAPort, *nodeBPort)
		if errRun != nil {
			log.Fatal().Err(errRun).Msg("Federation teardown failed")
		}

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
	fmt.Fprintln(os.Stderr, "  federation-setup     Set up federation E2E test environment")
	fmt.Fprintln(os.Stderr, "  federation-teardown  Tear down federation E2E test environment")
	fmt.Fprintln(os.Stderr, "  seed                 Seed a database with an admin user")
}
