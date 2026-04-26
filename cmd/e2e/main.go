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
	case "setup":
		fs := flag.NewFlagSet("setup", flag.ExitOnError)
		modeValue := fs.String("mode", string(e2eModeLocalController), "E2E mode: local-controller or remote-node")
		httpPort := fs.Int("http-port", 9091, "Backend HTTP port for E2E")
		nodePort := fs.Int("node-port", 9501, "Remote xylona-node HTTPS port")
		adminUsername := fs.String("admin-username", "admin", "Seed admin username")
		adminPassword := fs.String("admin-password", "admin", "Seed admin password")
		e2eDir := fs.String("e2e-dir", "frontend/e2e", "E2E test directory")
		projectRoot := fs.String("project-root", ".", "Project root directory")
		errParse := fs.Parse(os.Args[2:])
		if errParse != nil {
			log.Fatal().Err(errParse).Msg("Failed to parse flags")
		}
		mode, errMode := parseE2EMode(*modeValue)
		if errMode != nil {
			log.Fatal().Err(errMode).Msg("Invalid mode")
		}
		_, errRun := runSetup(ctx, setupConfig{
			mode:          mode,
			httpPort:      *httpPort,
			nodePort:      *nodePort,
			adminUsername: *adminUsername,
			adminPassword: *adminPassword,
			e2eDir:        *e2eDir,
			projectRoot:   *projectRoot,
		})
		if errRun != nil {
			log.Fatal().Err(errRun).Msg("Setup failed")
		}

	case "teardown":
		fs := flag.NewFlagSet("teardown", flag.ExitOnError)
		modeValue := fs.String("mode", string(e2eModeLocalController), "E2E mode: local-controller or remote-node")
		e2eDir := fs.String("e2e-dir", "frontend/e2e", "E2E test directory")
		errParse := fs.Parse(os.Args[2:])
		if errParse != nil {
			log.Fatal().Err(errParse).Msg("Failed to parse flags")
		}
		mode, errMode := parseE2EMode(*modeValue)
		if errMode != nil {
			log.Fatal().Err(errMode).Msg("Invalid mode")
		}
		runTeardown(*e2eDir, mode)

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

	case "status":
		fs := flag.NewFlagSet("status", flag.ExitOnError)
		modeValue := fs.String("mode", string(e2eModeLocalController), "E2E mode: local-controller or remote-node")
		e2eDir := fs.String("e2e-dir", "frontend/e2e", "E2E test directory")
		errParse := fs.Parse(os.Args[2:])
		if errParse != nil {
			log.Fatal().Err(errParse).Msg("Failed to parse flags")
		}
		mode, errMode := parseE2EMode(*modeValue)
		if errMode != nil {
			log.Fatal().Err(errMode).Msg("Invalid mode")
		}
		errRun := runStatus(*e2eDir, mode)
		if errRun != nil {
			log.Fatal().Err(errRun).Msg("Status failed")
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
	fmt.Fprintln(os.Stderr, "  setup                Set up local-controller or remote-node E2E environment")
	fmt.Fprintln(os.Stderr, "  teardown             Tear down an E2E environment")
	fmt.Fprintln(os.Stderr, "  seed                 Seed a database with an admin user")
	fmt.Fprintln(os.Stderr, "  status               Print current E2E environment status")
}
