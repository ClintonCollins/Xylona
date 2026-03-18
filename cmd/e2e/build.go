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
	cmd := exec.Command("go", "build", "-o", outputPath, "./cmd/dummy_game_server")
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
	cmd := exec.Command("go", "build", "-o", outputPath, ".")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildFrontend(projectRoot string) error {
	log.Info().Msg("[Federation Setup] Building frontend SPA...")
	cmd := exec.Command("pnpm", "run", "build")
	cmd.Dir = filepath.Join(projectRoot, "frontend")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
