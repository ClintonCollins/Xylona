//go:build mage

package main

import (
	"os"
	"os/exec"

	"github.com/rs/zerolog/log"
)

func Build() {
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

	remoteAddr := user + "@" + host

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
	cmdStop := exec.Command("ssh", remoteAddr, "sudo systemctl stop "+service)
	cmdStop.Stdout = os.Stdout
	cmdStop.Stderr = os.Stderr
	if err := cmdStop.Run(); err != nil {
		log.Warn().Err(err).Msg("Failed to stop service (it might not be running)")
	}

	// 3. Copy Binary
	log.Info().Str("host", host).Msg("Uploading binary...")
	cmdScp := exec.Command("scp", localBinary, remoteAddr+":"+path)
	cmdScp.Stdout = os.Stdout
	cmdScp.Stderr = os.Stderr
	if err := cmdScp.Run(); err != nil {
		log.Fatal().Err(err).Msg("Upload failed")
	}

	// 4. Start Service
	log.Info().Str("host", host).Msg("Starting service...")
	cmdStart := exec.Command("ssh", remoteAddr, "sudo systemctl start "+service)
	cmdStart.Stdout = os.Stdout
	cmdStart.Stderr = os.Stderr
	if err := cmdStart.Run(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start service")
	}

	log.Info().Msg("Deployment successful!")
}
