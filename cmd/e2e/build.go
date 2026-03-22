package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

func buildDummyGameServer(projectRoot, outputPath string) error {
	// Only rebuild if stale or missing.
	srcPath := filepath.Join(projectRoot, "cmd", "dummy_game_server", "main.go")
	srcStat, errSrc := os.Stat(srcPath)
	if errSrc != nil {
		return fmt.Errorf("stat dummy_game_server source: %w", errSrc)
	}

	exeStat, errExe := os.Stat(outputPath)
	if errExe == nil && exeStat.ModTime().After(srcStat.ModTime()) {
		log.Info().Msg("[E2E Setup] Dummy game server binary is up to date")
		return nil
	}

	// Ensure output directory exists.
	errMkdir := os.MkdirAll(filepath.Dir(outputPath), 0o755)
	if errMkdir != nil {
		return fmt.Errorf("create output dir: %w", errMkdir)
	}

	log.Info().Msg("[E2E Setup] Building dummy game server binary...")
	cmd := exec.Command("go", "build", "-o", outputPath, "./cmd/dummy_game_server") //nolint:noctx // build commands don't need cancellation context
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildXylona(projectRoot, outputPath string) error {
	log.Info().Msg("[Federation Setup] Building Xylona binary...")
	errMkdir := os.MkdirAll(filepath.Dir(outputPath), 0o755)
	if errMkdir != nil {
		return fmt.Errorf("create output dir: %w", errMkdir)
	}
	cmd := exec.Command("go", "build", "-o", outputPath, ".") //nolint:noctx // build commands don't need cancellation context
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildFrontend(projectRoot string) error {
	log.Info().Msg("[Federation Setup] Building frontend SPA...")
	cmd := exec.Command("pnpm", "run", "build") //nolint:noctx // build commands don't need cancellation context
	cmd.Dir = filepath.Join(projectRoot, "frontend")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// cleanFrontendDist removes the built SPA files from frontend/dist so stale
// E2E build artifacts don't linger after teardown. It leaves a .gitkeep file
// so the embed directive in embed.go doesn't break go build.
func cleanFrontendDist(e2eDir string) {
	distDir := filepath.Join(e2eDir, "..", "dist")
	entries, errRead := os.ReadDir(distDir)
	if errRead != nil {
		// dist doesn't exist or can't be read — nothing to clean.
		return
	}

	for _, entry := range entries {
		_ = os.RemoveAll(filepath.Join(distDir, entry.Name()))
	}

	// Write a .gitkeep so embed.go's "all:frontend/dist" has at least one file.
	errKeep := os.WriteFile(filepath.Join(distDir, ".gitkeep"), []byte(""), 0o644)
	if errKeep != nil {
		log.Warn().Err(errKeep).Msg("Could not write .gitkeep to frontend/dist")
		return
	}

	log.Info().Msg("Cleaned frontend/dist (left .gitkeep for embed directive)")
}
