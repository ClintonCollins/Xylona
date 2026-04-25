package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func changeToTestDirectory(t *testing.T, directory string) {
	t.Helper()

	t.Chdir(directory)
}

func TestResolveDefaultCLIUserDBPathPrefersProcessEnvironment(t *testing.T) {
	t.Setenv("DB_FILE_PATH", filepath.Join("custom", "users.sqlite"))

	workingDirectory := t.TempDir()
	changeToTestDirectory(t, workingDirectory)

	originalResolveCLIExecutableDir := resolveCLIExecutableDir
	resolveCLIExecutableDir = func() (string, error) {
		return filepath.Join(t.TempDir(), "xylona"), nil
	}
	t.Cleanup(func() {
		resolveCLIExecutableDir = originalResolveCLIExecutableDir
	})

	resolvedDBPath, errResolveDBPath := resolveDefaultCLIUserDBPath(context.Background())
	if errResolveDBPath != nil {
		t.Fatalf("resolveDefaultCLIUserDBPath() error = %v", errResolveDBPath)
	}

	expectedDBPath := filepath.Join(workingDirectory, "custom", "users.sqlite")
	if resolvedDBPath != expectedDBPath {
		t.Fatalf("resolveDefaultCLIUserDBPath() = %q, want %q", resolvedDBPath, expectedDBPath)
	}
}

func TestResolveDefaultCLIUserDBPathUsesExecutableEnvFile(t *testing.T) {
	t.Setenv("DB_FILE_PATH", "")

	workingDirectory := t.TempDir()
	executableDirectory := t.TempDir()
	changeToTestDirectory(t, workingDirectory)

	envContents := "DB_FILE_PATH=./state/data.sqlite\n"
	errWriteEnv := os.WriteFile(filepath.Join(executableDirectory, ".env"), []byte(envContents), 0o600)
	if errWriteEnv != nil {
		t.Fatalf("WriteFile(.env) error = %v", errWriteEnv)
	}

	originalResolveCLIExecutableDir := resolveCLIExecutableDir
	resolveCLIExecutableDir = func() (string, error) {
		return filepath.Join(executableDirectory, "xylona"), nil
	}
	t.Cleanup(func() {
		resolveCLIExecutableDir = originalResolveCLIExecutableDir
	})

	resolvedDBPath, errResolveDBPath := resolveDefaultCLIUserDBPath(context.Background())
	if errResolveDBPath != nil {
		t.Fatalf("resolveDefaultCLIUserDBPath() error = %v", errResolveDBPath)
	}

	expectedDBPath := filepath.Join(executableDirectory, "state", "data.sqlite")
	if resolvedDBPath != expectedDBPath {
		t.Fatalf("resolveDefaultCLIUserDBPath() = %q, want %q", resolvedDBPath, expectedDBPath)
	}
}

func TestResolveDefaultCLIUserDBPathUsesExecutableSiblingDatabase(t *testing.T) {
	t.Setenv("DB_FILE_PATH", "")

	workingDirectory := t.TempDir()
	executableDirectory := t.TempDir()
	changeToTestDirectory(t, workingDirectory)

	errWriteDatabase := os.WriteFile(filepath.Join(executableDirectory, "data.sqlite"), []byte(""), 0o600)
	if errWriteDatabase != nil {
		t.Fatalf("WriteFile(data.sqlite) error = %v", errWriteDatabase)
	}

	originalResolveCLIExecutableDir := resolveCLIExecutableDir
	resolveCLIExecutableDir = func() (string, error) {
		return filepath.Join(executableDirectory, "xylona"), nil
	}
	t.Cleanup(func() {
		resolveCLIExecutableDir = originalResolveCLIExecutableDir
	})

	resolvedDBPath, errResolveDBPath := resolveDefaultCLIUserDBPath(context.Background())
	if errResolveDBPath != nil {
		t.Fatalf("resolveDefaultCLIUserDBPath() error = %v", errResolveDBPath)
	}

	expectedDBPath := filepath.Join(executableDirectory, "data.sqlite")
	if resolvedDBPath != expectedDBPath {
		t.Fatalf("resolveDefaultCLIUserDBPath() = %q, want %q", resolvedDBPath, expectedDBPath)
	}
}
