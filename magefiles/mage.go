//go:build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"

	"github.com/rs/zerolog/log"
)

var validDeployParam = regexp.MustCompile(`^[a-zA-Z0-9._\-/]+$`)

func validateDeployParam(name, value string) error {
	if !validDeployParam.MatchString(value) {
		return fmt.Errorf("invalid %s: %q contains disallowed characters", name, value)
	}
	return nil
}

func Build() {
	// Build the frontend
	buildFrontend()

	// Run Goreleaser
	cmdGoReleaser := exec.Command("goreleaser", "release", "--snapshot", "--clean")
	cmdGoReleaser.Dir = "."
	cmdGoReleaser.Stdout = os.Stdout
	cmdGoReleaser.Stderr = os.Stderr
	errRunGoReleaser := cmdGoReleaser.Run()
	if errRunGoReleaser != nil {
		log.Error().Err(errRunGoReleaser).Msg("Failed to run goreleaser")
		os.Exit(1)
	}
}

func GenerateProto() {
	cmdGenProto := exec.Command("buf", "generate")
	cmdGenProto.Dir = "proto"
	cmdGenProto.Stdout = os.Stdout
	cmdGenProto.Stderr = os.Stderr
	errRun := cmdGenProto.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("Failed to generate proto")
		os.Exit(1)
	}
}

func GenerateModels() {
	cmdGenModels := exec.Command("go", "run", "github.com/stephenafamo/bob/gen/bobgen-sqlite@v0.25.0", "-c", "./bobgen.yaml")
	cmdGenModels.Dir = "sql"
	cmdGenModels.Stdout = os.Stdout
	cmdGenModels.Stderr = os.Stderr
	errRun := cmdGenModels.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("Failed to generate models")
		os.Exit(1)
	}
}

func SQLMigrateNew(migrationName string) {
	if migrationName == "" {
		log.Fatal().Msg("Migration name cannot be empty")
	}
	cmdMigrateNew := exec.Command("sql-migrate", "new", migrationName)
	cmdMigrateNew.Dir = "sql"
	cmdMigrateNew.Stdout = os.Stdout
	cmdMigrateNew.Stderr = os.Stderr
	errRun := cmdMigrateNew.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("Failed to create new migration")
	}
}

func SQLMigrateDown() {
	cmdMigrateDown := exec.Command("sql-migrate", "down")
	cmdMigrateDown.Dir = "sql"
	cmdMigrateDown.Stdout = os.Stdout
	cmdMigrateDown.Stderr = os.Stderr
	errRun := cmdMigrateDown.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("Failed to migrate down")
		os.Exit(1)
	}
}

func SQLMigrateUp() {
	cmdMigrateUp := exec.Command("sql-migrate", "up")
	cmdMigrateUp.Dir = "sql"
	cmdMigrateUp.Stdout = os.Stdout
	cmdMigrateUp.Stderr = os.Stderr
	errRun := cmdMigrateUp.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("Failed to migrate up")
		os.Exit(1)
	}
}

// Deploy builds the Linux binary and deploys it to a remote server.
// Usage: mage deploy <remote_host> [remote_user] [service_name] [remote_path]
func Deploy(host string, user string, service string, path string) {
	if host == "" {
		log.Fatal().Msg("Remote host is required. Usage: mage deploy <host> [user] [service] [path]")
	}
	if user == "" {
		user = os.Getenv("USER")
		if user == "" {
			user = os.Getenv("USERNAME")
		}
	}
	if service == "" {
		service = "xylona"
	}
	if path == "" {
		path = "/usr/local/bin/xylona"
	}

	params := map[string]string{
		"host":    host,
		"user":    user,
		"service": service,
		"path":    path,
	}
	for paramName, paramValue := range params {
		if errValidate := validateDeployParam(paramName, paramValue); errValidate != nil {
			log.Fatal().Err(errValidate).Msg("Invalid deploy parameter")
		}
	}

	remoteAddr := user + "@" + host

	// 0. Build Frontend
	buildFrontend()

	// 1. Build
	log.Info().Msg("Building Xylona for Linux/amd64...")
	cmdBuild := exec.Command("goreleaser", "build", "--snapshot", "--clean", "--single-target")
	cmdBuild.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
	cmdBuild.Stdout = os.Stdout
	cmdBuild.Stderr = os.Stderr
	if err := cmdBuild.Run(); err != nil {
		log.Fatal().Err(err).Msg("Build failed")
	}

	localBinary := "dist/Xylona_linux_amd64_v1/Xylona"
	if _, err := os.Stat(localBinary); os.IsNotExist(err) {
		log.Fatal().Msgf("Binary not found at %s. Please check goreleaser output structure.", localBinary)
	}

	// 2. Stop Service
	log.Info().Str("host", host).Msg("Stopping service...")
	cmdStop := exec.Command("ssh", "--", remoteAddr, "sudo systemctl stop "+service)
	cmdStop.Stdout = os.Stdout
	cmdStop.Stderr = os.Stderr
	if err := cmdStop.Run(); err != nil {
		log.Warn().Err(err).Msg("Failed to stop service (it might not be running)")
	}

	// 3. Copy Binary
	log.Info().Str("host", host).Msg("Uploading binary...")
	cmdScp := exec.Command("scp", "--", localBinary, remoteAddr+":"+path)
	cmdScp.Stdout = os.Stdout
	cmdScp.Stderr = os.Stderr
	if err := cmdScp.Run(); err != nil {
		log.Fatal().Err(err).Msg("Upload failed")
	}

	// 4. Start Service
	log.Info().Str("host", host).Msg("Starting service...")
	cmdStart := exec.Command("ssh", "--", remoteAddr, "sudo systemctl start "+service)
	cmdStart.Stdout = os.Stdout
	cmdStart.Stderr = os.Stderr
	if err := cmdStart.Run(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start service")
	}

	log.Info().Msg("Deployment successful!")
}

// E2E runs the single-node Playwright E2E tests.
// Requires a running backend on :8080. The Vite dev server starts automatically.
func E2E() {
	cmdE2E := exec.Command("pnpm", "run", "e2e")
	cmdE2E.Dir = "frontend"
	cmdE2E.Stdout = os.Stdout
	cmdE2E.Stderr = os.Stderr
	errRun := cmdE2E.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("E2E tests failed")
		os.Exit(1)
	}
}

// E2EHeaded runs single-node E2E tests in headed browser mode.
func E2EHeaded() {
	cmdE2E := exec.Command("pnpm", "run", "e2e:headed")
	cmdE2E.Dir = "frontend"
	cmdE2E.Stdout = os.Stdout
	cmdE2E.Stderr = os.Stderr
	errRun := cmdE2E.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("E2E headed tests failed")
		os.Exit(1)
	}
}

// E2EUI opens the Playwright interactive UI for single-node tests.
func E2EUI() {
	cmdE2E := exec.Command("pnpm", "run", "e2e:ui")
	cmdE2E.Dir = "frontend"
	cmdE2E.Stdout = os.Stdout
	cmdE2E.Stderr = os.Stderr
	errRun := cmdE2E.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("E2E UI failed")
		os.Exit(1)
	}
}

// E2EReport opens the last Playwright HTML report for single-node tests.
func E2EReport() {
	cmdE2E := exec.Command("pnpm", "run", "e2e:report")
	cmdE2E.Dir = "frontend"
	cmdE2E.Stdout = os.Stdout
	cmdE2E.Stderr = os.Stderr
	errRun := cmdE2E.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("E2E report failed")
		os.Exit(1)
	}
}

// E2EFederation runs the two-node federation Playwright E2E tests.
// Fully self-contained — builds binaries, starts two nodes, pairs them, runs tests, tears down.
func E2EFederation() {
	cmdE2E := exec.Command("pnpm", "run", "e2e:federation")
	cmdE2E.Dir = "frontend"
	cmdE2E.Stdout = os.Stdout
	cmdE2E.Stderr = os.Stderr
	errRun := cmdE2E.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("Federation E2E tests failed")
		os.Exit(1)
	}
}

// E2EFederationHeaded runs federation E2E tests in headed browser mode.
func E2EFederationHeaded() {
	cmdE2E := exec.Command("pnpm", "run", "e2e:federation:headed")
	cmdE2E.Dir = "frontend"
	cmdE2E.Stdout = os.Stdout
	cmdE2E.Stderr = os.Stderr
	errRun := cmdE2E.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("Federation E2E headed tests failed")
		os.Exit(1)
	}
}

// E2EFederationReport opens the last federation Playwright HTML report.
func E2EFederationReport() {
	cmdE2E := exec.Command("pnpm", "run", "e2e:federation:report")
	cmdE2E.Dir = "frontend"
	cmdE2E.Stdout = os.Stdout
	cmdE2E.Stderr = os.Stderr
	errRun := cmdE2E.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("Federation E2E report failed")
		os.Exit(1)
	}
}

// E2ESeed bootstraps a fresh SQLite database with an admin user.
// Usage: mage e2eSeed <db_path> [username] [password]
func E2ESeed(dbPath string, username string, password string) {
	if dbPath == "" {
		log.Fatal().Msg("db_path is required. Usage: mage e2eSeed <db_path> [username] [password]")
	}
	if username == "" {
		username = "admin"
	}
	if password == "" {
		password = "admin"
	}
	cmdSeed := exec.Command("go", "run", "./cmd/e2e", "seed", "-db", dbPath, "-username", username, "-password", password)
	cmdSeed.Stdout = os.Stdout
	cmdSeed.Stderr = os.Stderr
	errRun := cmdSeed.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("E2E seed failed")
		os.Exit(1)
	}
}

func buildFrontend() {
	// Build the frontend
	cmdPnpm := exec.Command("pnpm", "run", "build")
	cmdPnpm.Dir = "frontend"
	cmdPnpm.Stdout = os.Stdout
	cmdPnpm.Stderr = os.Stderr
	errRun := cmdPnpm.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("Failed to build frontend")
		os.Exit(1)
	}
}
