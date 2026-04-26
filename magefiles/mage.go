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
	cmdGoReleaser := exec.Command("goreleaser", "release", "--snapshot", "--clean", "--skip=sign")
	cmdGoReleaser.Dir = "."
	cmdGoReleaser.Stdout = os.Stdout
	cmdGoReleaser.Stderr = os.Stderr
	errRunGoReleaser := cmdGoReleaser.Run()
	if errRunGoReleaser != nil {
		log.Error().Err(errRunGoReleaser).Msg("Failed to run goreleaser")
		os.Exit(1)
	}
}

// BuildNode compiles the local xylona-node agent binary for development.
// Release and deploy packaging use their own platform-specific output paths,
// so this target stays intentionally small for fast node iteration.
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

func runFrontendScript(script string, failureMessage string) {
	cmdE2E := exec.Command("bun", "run", script)
	cmdE2E.Dir = "frontend"
	cmdE2E.Stdout = os.Stdout
	cmdE2E.Stderr = os.Stderr
	errRun := cmdE2E.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg(failureMessage)
		os.Exit(1)
	}
}

// E2ESmoke runs the smallest browser suite against a fresh local-controller environment.
func E2ESmoke() {
	runFrontendScript("e2e:smoke", "E2E smoke tests failed")
}

// E2E runs the browser E2E suite against a fresh local-controller environment.
func E2E() {
	runFrontendScript("e2e", "E2E tests failed")
}

// E2ERemoteNode runs the browser remote-node smoke suite.
func E2ERemoteNode() {
	runFrontendScript("e2e:remote-node", "E2E remote-node tests failed")
}

// E2EHeaded runs E2E tests in headed browser mode.
func E2EHeaded() {
	runFrontendScript("e2e:headed", "E2E headed tests failed")
}

// E2EUI opens the Playwright interactive UI for single-node tests.
func E2EUI() {
	runFrontendScript("e2e:ui", "E2E UI failed")
}

// E2EReport opens the last Playwright HTML report for single-node tests.
func E2EReport() {
	runFrontendScript("e2e:report", "E2E report failed")
}

// IntegrationLocal runs local process integration tests with controller + xylona-node.
func IntegrationLocal() {
	cmdTest := exec.Command("go", "test", "-race", "-count=1", "-tags=local_integration", "./cmd/e2e")
	cmdTest.Stdout = os.Stdout
	cmdTest.Stderr = os.Stderr
	errRun := cmdTest.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("Local integration tests failed")
		os.Exit(1)
	}
}

// IntegrationLive runs opt-in tests that call external provider APIs.
func IntegrationLive() {
	cmdTest := exec.Command("go", "test", "-race", "-count=1", "-tags=integration", "./...")
	cmdTest.Stdout = os.Stdout
	cmdTest.Stderr = os.Stderr
	errRun := cmdTest.Run()
	if errRun != nil {
		log.Error().Err(errRun).Msg("Live integration tests failed")
		os.Exit(1)
	}
}

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
