package actions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func makeTestGameServer(t *testing.T, dir string) *models.GameServer {
	t.Helper()
	gs := &models.GameServer{}
	gs.ID = "test-server-id"
	gs.Directory = dir
	return gs
}

func TestBackupServerFiles_CreatesBackupDir(t *testing.T) {
	dir := t.TempDir()
	gs := makeTestGameServer(t, dir)

	configPath := filepath.Join(dir, "server.toml")
	errWrite := os.WriteFile(configPath, []byte("port=25565\n"), 0644)
	if errWrite != nil {
		t.Fatalf("failed to create test config: %v", errWrite)
	}

	errBackup := backupServerFiles(gs)
	if errBackup != nil {
		t.Fatalf("backupServerFiles() error = %v", errBackup)
	}

	backupDir := filepath.Join(dir, ".update-backup")
	_, errStat := os.Stat(backupDir)
	if os.IsNotExist(errStat) {
		t.Fatal("expected .update-backup directory to exist")
	}

	backupConfig := filepath.Join(backupDir, "server.toml")
	_, errStatConfig := os.Stat(backupConfig)
	if os.IsNotExist(errStatConfig) {
		t.Fatal("expected server.toml to be backed up")
	}
}

func TestBackupServerFiles_CopiesExecutable(t *testing.T) {
	dir := t.TempDir()
	gs := makeTestGameServer(t, dir)

	// Use a name that is recognized as an executable on all platforms:
	// .exe extension is detected on Windows; 0755 bits are detected on Unix.
	execName := "server.exe"
	execPath := filepath.Join(dir, execName)
	errWrite := os.WriteFile(execPath, []byte("binary\n"), 0755)
	if errWrite != nil {
		t.Fatalf("failed to create test executable: %v", errWrite)
	}

	errBackup := backupServerFiles(gs)
	if errBackup != nil {
		t.Fatalf("backupServerFiles() error = %v", errBackup)
	}

	backupExec := filepath.Join(dir, ".update-backup", execName)
	_, errStat := os.Stat(backupExec)
	if os.IsNotExist(errStat) {
		t.Fatal("expected executable to be backed up")
	}
}

func TestBackupServerFiles_NamedExecutable(t *testing.T) {
	dir := t.TempDir()
	gs := makeTestGameServer(t, dir)
	gs.ServerExecutable = null.From("myserver.exe")

	execPath := filepath.Join(dir, "myserver.exe")
	errWrite := os.WriteFile(execPath, []byte("binary\n"), 0644)
	if errWrite != nil {
		t.Fatalf("failed to create named executable: %v", errWrite)
	}

	// Another executable that should NOT be backed up when ServerExecutable is set.
	otherExec := filepath.Join(dir, "other.exe")
	errWrite2 := os.WriteFile(otherExec, []byte("other\n"), 0755)
	if errWrite2 != nil {
		t.Fatalf("failed to create other executable: %v", errWrite2)
	}

	errBackup := backupServerFiles(gs)
	if errBackup != nil {
		t.Fatalf("backupServerFiles() error = %v", errBackup)
	}

	backupExec := filepath.Join(dir, ".update-backup", "myserver.exe")
	_, errStat := os.Stat(backupExec)
	if os.IsNotExist(errStat) {
		t.Fatal("expected named executable to be backed up")
	}
}

func TestBackupServerFiles_SkipsSubdirectories(t *testing.T) {
	dir := t.TempDir()
	gs := makeTestGameServer(t, dir)

	// Config in root — should be backed up.
	rootConfig := filepath.Join(dir, "server.json")
	errWrite := os.WriteFile(rootConfig, []byte("{}"), 0644)
	if errWrite != nil {
		t.Fatalf("failed to create root config: %v", errWrite)
	}

	// Config in subdir — should NOT be backed up.
	subDir := filepath.Join(dir, "config")
	errMkdir := os.MkdirAll(subDir, 0755)
	if errMkdir != nil {
		t.Fatalf("failed to create subdir: %v", errMkdir)
	}
	subConfig := filepath.Join(subDir, "sub.json")
	errWriteSub := os.WriteFile(subConfig, []byte("{}"), 0644)
	if errWriteSub != nil {
		t.Fatalf("failed to create subdir config: %v", errWriteSub)
	}

	errBackup := backupServerFiles(gs)
	if errBackup != nil {
		t.Fatalf("backupServerFiles() error = %v", errBackup)
	}

	// Root config should be backed up.
	_, errStatRoot := os.Stat(filepath.Join(dir, ".update-backup", "server.json"))
	if os.IsNotExist(errStatRoot) {
		t.Fatal("expected root config to be backed up")
	}

	// Subdirectory config should NOT be backed up.
	_, errStatSub := os.Stat(filepath.Join(dir, ".update-backup", "config", "sub.json"))
	if errStatSub == nil {
		t.Fatal("expected subdir config to NOT be backed up")
	}
}

func TestRestoreServerFiles_RestoresAndRemovesBackup(t *testing.T) {
	dir := t.TempDir()
	gs := makeTestGameServer(t, dir)

	backupDir := filepath.Join(dir, ".update-backup")
	errMkdir := os.MkdirAll(backupDir, 0755)
	if errMkdir != nil {
		t.Fatalf("failed to create backup dir: %v", errMkdir)
	}
	backupFile := filepath.Join(backupDir, "server.toml")
	errWrite := os.WriteFile(backupFile, []byte("port=25565\n"), 0644)
	if errWrite != nil {
		t.Fatalf("failed to create backup file: %v", errWrite)
	}

	errRestore := restoreServerFiles(gs)
	if errRestore != nil {
		t.Fatalf("restoreServerFiles() error = %v", errRestore)
	}

	// Backup dir should be removed.
	_, errStatBackup := os.Stat(backupDir)
	if !os.IsNotExist(errStatBackup) {
		t.Fatal("expected .update-backup directory to be removed after restore")
	}

	// File should be in server dir.
	restored := filepath.Join(dir, "server.toml")
	_, errStatRestored := os.Stat(restored)
	if os.IsNotExist(errStatRestored) {
		t.Fatal("expected server.toml to be restored")
	}
}

func TestRestoreServerFiles_OverwritesExistingFiles(t *testing.T) {
	dir := t.TempDir()
	gs := makeTestGameServer(t, dir)

	// Write an "old" version of the file in the server dir.
	serverFile := filepath.Join(dir, "server.toml")
	errWrite := os.WriteFile(serverFile, []byte("old content\n"), 0644)
	if errWrite != nil {
		t.Fatalf("failed to create server file: %v", errWrite)
	}

	// Write the backup with different content.
	backupDir := filepath.Join(dir, ".update-backup")
	errMkdir := os.MkdirAll(backupDir, 0755)
	if errMkdir != nil {
		t.Fatalf("failed to create backup dir: %v", errMkdir)
	}
	errWriteBackup := os.WriteFile(filepath.Join(backupDir, "server.toml"), []byte("backup content\n"), 0644)
	if errWriteBackup != nil {
		t.Fatalf("failed to create backup file: %v", errWriteBackup)
	}

	errRestore := restoreServerFiles(gs)
	if errRestore != nil {
		t.Fatalf("restoreServerFiles() error = %v", errRestore)
	}

	content, errRead := os.ReadFile(serverFile)
	if errRead != nil {
		t.Fatalf("failed to read restored file: %v", errRead)
	}
	if string(content) != "backup content\n" {
		t.Errorf("restored file content = %q, want %q", string(content), "backup content\n")
	}
}

func TestRestoreServerFiles_NoBackupDirIsNoop(t *testing.T) {
	dir := t.TempDir()
	gs := makeTestGameServer(t, dir)

	// No backup dir exists — should not error.
	errRestore := restoreServerFiles(gs)
	if errRestore != nil {
		t.Fatalf("restoreServerFiles() with no backup dir error = %v", errRestore)
	}
}

func TestCleanupBackup_RemovesBackupDir(t *testing.T) {
	dir := t.TempDir()
	gs := makeTestGameServer(t, dir)

	backupDir := filepath.Join(dir, ".update-backup")
	errMkdir := os.MkdirAll(backupDir, 0755)
	if errMkdir != nil {
		t.Fatalf("failed to create backup dir: %v", errMkdir)
	}

	cleanupBackup(gs)

	_, errStat := os.Stat(backupDir)
	if !os.IsNotExist(errStat) {
		t.Fatal("expected .update-backup directory to be removed")
	}
}

func TestCleanupBackup_NoBackupDirIsNoop(t *testing.T) {
	dir := t.TempDir()
	gs := makeTestGameServer(t, dir)

	// Should not panic or error when there's nothing to remove.
	cleanupBackup(gs)
}
