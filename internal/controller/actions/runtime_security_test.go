package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestAssessRuntimeSecurityLayoutCases(t *testing.T) {
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home", "clinton"))
	t.Setenv("USER", "clinton")
	t.Setenv("USERPROFILE", filepath.Join(t.TempDir(), "Users", "clinton"))

	defaultInstallRoot, errDefaultInstallPath := DefaultInstallPath()
	if errDefaultInstallPath != nil {
		t.Fatalf("DefaultInstallPath() error = %v", errDefaultInstallPath)
	}
	defaultBackupRoot, errDefaultBackupPath := DefaultBackupDirectory()
	if errDefaultBackupPath != nil {
		t.Fatalf("DefaultBackupDirectory() error = %v", errDefaultBackupPath)
	}

	root := t.TempDir()
	safeServerDir := filepath.Join(defaultInstallRoot, "safe-default")
	safeBackupDir := filepath.Join(defaultBackupRoot, "safe-default")
	customServerDir := filepath.Join(root, "custom-install", "custom-layout")
	customBackupDir := filepath.Join(root, "custom-backups", "custom-layout")
	dbOutsidePaths := filepath.Join(root, "data.sqlite")

	mustMkdirAll(t, safeServerDir)
	mustMkdirAll(t, safeBackupDir)
	mustMkdirAll(t, customServerDir)
	mustMkdirAll(t, customBackupDir)
	mustWriteFile(t, dbOutsidePaths, []byte("sqlite"))

	tests := []struct {
		name                  string
		input                 RuntimeSecurityAssessmentInput
		wantBlockingCount     int
		wantWarningSubstrings []string
	}{
		{
			name: "db inside server directory blocks startup",
			input: RuntimeSecurityAssessmentInput{
				DBFilePath: dbPathWithin(safeServerDir),
				Servers: []*models.GameServer{
					runtimeSecurityServer("safe-default", safeServerDir, safeBackupDir),
				},
				CurrentUser: "clinton",
			},
			wantBlockingCount: 1,
		},
		{
			name: "db inside backup directory blocks startup",
			input: RuntimeSecurityAssessmentInput{
				DBFilePath: dbPathWithin(safeBackupDir),
				Servers: []*models.GameServer{
					runtimeSecurityServer("safe-default", safeServerDir, safeBackupDir),
				},
				CurrentUser: "clinton",
			},
			wantBlockingCount: 1,
		},
		{
			name: "safe default layout does not block",
			input: RuntimeSecurityAssessmentInput{
				DBFilePath: dbOutsidePaths,
				Servers: []*models.GameServer{
					runtimeSecurityServer("safe-default", safeServerDir, safeBackupDir),
				},
				CurrentUser: "clinton",
			},
			wantBlockingCount: 0,
		},
		{
			name: "custom layout warns but does not block",
			input: RuntimeSecurityAssessmentInput{
				DBFilePath: dbOutsidePaths,
				Servers: []*models.GameServer{
					runtimeSecurityServer("custom-layout", customServerDir, customBackupDir),
				},
				CurrentUser: "clinton",
			},
			wantBlockingCount:     0,
			wantWarningSubstrings: []string{"custom layout"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := AssessRuntimeSecurity(tt.input)
			if got := len(assessment.BlockingErrors); got != tt.wantBlockingCount {
				t.Fatalf("len(BlockingErrors) = %d, want %d", got, tt.wantBlockingCount)
			}
			for _, wantWarning := range tt.wantWarningSubstrings {
				if !containsWarning(assessment.Warnings, wantWarning) {
					t.Fatalf("warnings = %v, want substring %q", assessment.Warnings, wantWarning)
				}
			}
		})
	}
}

func TestAssessRuntimeSecurityWarnings(t *testing.T) {
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home", "clinton"))
	t.Setenv("USER", "clinton")
	t.Setenv("USERPROFILE", filepath.Join(t.TempDir(), "Users", "clinton"))

	defaultInstallRoot, errDefaultInstallPath := DefaultInstallPath()
	if errDefaultInstallPath != nil {
		t.Fatalf("DefaultInstallPath() error = %v", errDefaultInstallPath)
	}
	defaultBackupRoot, errDefaultBackupPath := DefaultBackupDirectory()
	if errDefaultBackupPath != nil {
		t.Fatalf("DefaultBackupDirectory() error = %v", errDefaultBackupPath)
	}

	server := runtimeSecurityServer(
		"safe-default",
		filepath.Join(defaultInstallRoot, "safe-default"),
		filepath.Join(defaultBackupRoot, "safe-default"),
	)

	tests := []struct {
		name                  string
		input                 RuntimeSecurityAssessmentInput
		wantWarningSubstrings []string
	}{
		{
			name: "same-user trust model warning",
			input: RuntimeSecurityAssessmentInput{
				DBFilePath:  dbPathWithin(t.TempDir()),
				Servers:     []*models.GameServer{server},
				CurrentUser: "clinton",
			},
			wantWarningSubstrings: []string{"same-user"},
		},
		{
			name: "elevated runtime warning",
			input: RuntimeSecurityAssessmentInput{
				DBFilePath:  dbPathWithin(t.TempDir()),
				Servers:     []*models.GameServer{server},
				CurrentUser: "clinton",
				Elevated:    true,
			},
			wantWarningSubstrings: []string{"elevated"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := AssessRuntimeSecurity(tt.input)
			if len(assessment.BlockingErrors) != 0 {
				t.Fatalf("len(BlockingErrors) = %d, want 0", len(assessment.BlockingErrors))
			}
			for _, wantWarning := range tt.wantWarningSubstrings {
				if !containsWarning(assessment.Warnings, wantWarning) {
					t.Fatalf("warnings = %v, want substring %q", assessment.Warnings, wantWarning)
				}
			}
		})
	}
}

func runtimeSecurityServer(id string, directory string, backupDirectory string) *models.GameServer {
	return &models.GameServer{
		ID:              id,
		Directory:       directory,
		BackupDirectory: backupDirectory,
	}
}

func dbPathWithin(root string) string {
	return filepath.Join(root, "data.sqlite")
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()

	if filepath.Clean(path) == "" {
		t.Fatalf("invalid empty path")
	}

	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatalf("mkdirAll(%q) error = %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, contents []byte) {
	t.Helper()

	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("writeFile(%q) error = %v", path, err)
	}
}

func containsWarning(warnings []string, wantSubstring string) bool {
	for _, warning := range warnings {
		if strings.Contains(strings.ToLower(warning), strings.ToLower(wantSubstring)) {
			return true
		}
	}
	return false
}
