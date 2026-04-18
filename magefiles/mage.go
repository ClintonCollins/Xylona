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

type deployConfig struct {
	host        string
	user        string
	service     string
	remotePath  string
	localBinary string
	buildArgs   []string
}

func validateDeployParam(name, value string) error {
	if !validDeployParam.MatchString(value) {
		return fmt.Errorf("invalid %s: %q contains disallowed characters", name, value)
	}
	return nil
}

func resolveDeployConfig(host string, user string, service string, path string) deployConfig {
	if user == "" {
		user = os.Getenv("USER")
		if user == "" {
			user = os.Getenv("USERNAME")
		}
	}
	if service == "" {
		service = "xylona-node"
	}
	if path == "" {
		path = "/usr/local/bin/xylona-node"
	}

	return deployConfig{
		host:        host,
		user:        user,
		service:     service,
		remotePath:  path,
		localBinary: "dist/xylona-node-linux-amd64",
		buildArgs:   []string{"build", "-o", "dist/xylona-node-linux-amd64", "./cmd/xylona-node/"},
	}
}

// Lint runs golangci-lint on the Go codebase.
func Lint() {
	cmdLint := exec.Command("golangci-lint", "run", "./...")
	cmdLint.Stdout = os.Stdout
	cmdLint.Stderr = os.Stderr
	errLint := cmdLint.Run()
	if errLint != nil {
		log.Error().Err(errLint).Msg("Lint failed")
		os.Exit(1)
	}
}

// LintFix runs golangci-lint with auto-fix for fixable issues.
func LintFix() {
	cmdLint := exec.Command("golangci-lint", "run", "--fix", "./...")
	cmdLint.Stdout = os.Stdout
	cmdLint.Stderr = os.Stderr
	errLint := cmdLint.Run()
	if errLint != nil {
		log.Error().Err(errLint).Msg("Lint fix failed")
		os.Exit(1)
	}
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

// BuildNode compiles the xylona-node agent binary. The controller (xylona)
// and the node binary are independent — this target only builds the node so
// developers can iterate on it without paying for a full goreleaser run.
//
// Note: `mage Build` currently drives goreleaser, which is configured to
// release the controller binary only. Once the node binary ships to end
// users, goreleaser should be extended to include it; until then, use
// BuildNode during development.
func BuildNode() {
	cmdBuild := exec.Command("go", "build", "-o", "xylona-node", "./cmd/xylona-node/")
	cmdBuild.Stdout = os.Stdout
	cmdBuild.Stderr = os.Stderr
	errRun := cmdBuild.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("Failed to build xylona-node")
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
	cmdGenModels := exec.Command("go", "run", "github.com/stephenafamo/bob/gen/bobgen-sqlite@v0.42.0", "-c", "./bobgen.yaml")
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
// Usage: mage deploy <remote_host> [remote_user] [service_name] [remote_path].
func Deploy(host string, user string, service string, path string) {
	if host == "" {
		log.Fatal().Msg("Remote host is required. Usage: mage deploy <host> [user] [service] [path]")
	}

	config := resolveDeployConfig(host, user, service, path)

	params := map[string]string{
		"host":    config.host,
		"user":    config.user,
		"service": config.service,
		"path":    config.remotePath,
	}
	for paramName, paramValue := range params {
		errValidate := validateDeployParam(paramName, paramValue)
		if errValidate != nil {
			log.Fatal().Err(errValidate).Msg("Invalid deploy parameter")
		}
	}

	remoteAddr := config.user + "@" + config.host

	// 0. Build Frontend
	buildFrontend()

	// 1. Build
	log.Info().Msg("Building xylona-node for Linux/amd64...")
	errMkdir := os.MkdirAll("dist", 0o750)
	if errMkdir != nil {
		log.Fatal().Err(errMkdir).Msg("Failed to prepare dist directory")
	}

	// #nosec G204 -- build args are fixed for the local xylona-node build target.
	cmdBuild := exec.Command("go", config.buildArgs...)
	cmdBuild.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
	cmdBuild.Stdout = os.Stdout
	cmdBuild.Stderr = os.Stderr
	errBuild := cmdBuild.Run()
	if errBuild != nil {
		log.Fatal().Err(errBuild).Msg("Build failed")
	}

	_, errStat := os.Stat(config.localBinary)
	if os.IsNotExist(errStat) {
		log.Fatal().Msgf("Binary not found at %s after build.", config.localBinary)
	}
	if errStat != nil {
		log.Fatal().Err(errStat).Msg("Failed to verify built binary")
	}

	// 2. Stop Service
	log.Info().Str("host", config.host).Msg("Stopping service...")
	// #nosec G204 -- remote host, user, service, and path are validated against a strict allowlist.
	cmdStop := exec.Command("ssh", "--", remoteAddr, "sudo systemctl stop "+config.service)
	cmdStop.Stdout = os.Stdout
	cmdStop.Stderr = os.Stderr
	errStop := cmdStop.Run()
	if errStop != nil {
		log.Warn().Err(errStop).Msg("Failed to stop service (it might not be running)")
	}

	// 3. Copy Binary
	log.Info().Str("host", config.host).Msg("Uploading binary...")
	// #nosec G204 -- remote host, user, service, and path are validated against a strict allowlist.
	cmdScp := exec.Command("scp", "--", config.localBinary, remoteAddr+":"+config.remotePath)
	cmdScp.Stdout = os.Stdout
	cmdScp.Stderr = os.Stderr
	errUpload := cmdScp.Run()
	if errUpload != nil {
		log.Fatal().Err(errUpload).Msg("Upload failed")
	}

	// 4. Start Service
	log.Info().Str("host", config.host).Msg("Starting service...")
	// #nosec G204 -- remote host, user, service, and path are validated against a strict allowlist.
	cmdStart := exec.Command("ssh", "--", remoteAddr, "sudo systemctl start "+config.service)
	cmdStart.Stdout = os.Stdout
	cmdStart.Stderr = os.Stderr
	errStart := cmdStart.Run()
	if errStart != nil {
		log.Fatal().Err(errStart).Msg("Failed to start service")
	}

	log.Info().Msg("Deployment successful!")
}

// E2E runs the single-node Playwright E2E tests.
// Fully self-contained -- builds backend, seeds DB, starts on :9091, runs tests, tears down.
func E2E() {
	cmdE2E := exec.Command("bun", "run", "e2e")
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
	cmdE2E := exec.Command("bun", "run", "e2e:headed")
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
	cmdE2E := exec.Command("bun", "run", "e2e:ui")
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
	cmdE2E := exec.Command("bun", "run", "e2e:report")
	cmdE2E.Dir = "frontend"
	cmdE2E.Stdout = os.Stdout
	cmdE2E.Stderr = os.Stderr
	errRun := cmdE2E.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("E2E report failed")
		os.Exit(1)
	}
}

// E2EFederation, E2EFederationHeaded, and E2EFederationReport were removed
// alongside the federation mesh harness. The hub-spoke multi-node E2E
// (controller + xylona-node) lands in step 11 of the hub-spoke migration.

// E2ESeed bootstraps a fresh SQLite database with an admin user.
// Usage: mage e2eSeed <db_path> [username] [password].
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
	cmdBun := exec.Command("bun", "run", "build")
	cmdBun.Dir = "frontend"
	cmdBun.Stdout = os.Stdout
	cmdBun.Stderr = os.Stderr
	errRun := cmdBun.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("Failed to build frontend")
		os.Exit(1)
	}
}
