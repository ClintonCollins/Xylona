package selfupdate

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestReplaceAndRestartCurrentProcess(t *testing.T) {
	t.Run("executes replacement in current process", func(t *testing.T) {
		pending, markerPath, oldContent, newContent := prepareHelperTest(t, RestartModeSelf)
		execCalls := 0
		errRestart := replaceAndRestartCurrentProcess(markerPath, pending, func(got pendingUpdate) error {
			execCalls++
			assertFileContent(t, got.ExecutablePath, newContent)
			assertFileContent(t, got.BackupPath, oldContent)
			return nil
		})
		if errRestart != nil {
			t.Fatalf("replaceAndRestartCurrentProcess() error = %v", errRestart)
		}
		if execCalls != 1 {
			t.Fatalf("exec calls = %d, want 1", execCalls)
		}
		_, errMarker := os.Stat(markerPath)
		if errMarker != nil {
			t.Fatalf("pending marker must remain for replacement startup reconciliation: %v", errMarker)
		}
	})

	t.Run("restores previous executable when exec fails", func(t *testing.T) {
		pending, markerPath, oldContent, newContent := prepareHelperTest(t, RestartModeSelf)
		execCalls := 0
		errRestart := replaceAndRestartCurrentProcess(markerPath, pending, func(got pendingUpdate) error {
			execCalls++
			if execCalls == 1 {
				assertFileContent(t, got.ExecutablePath, newContent)
				return errors.New("replacement exec failed")
			}
			assertFileContent(t, got.ExecutablePath, oldContent)
			return errors.New("restored exec failed")
		})
		if errRestart == nil || !strings.Contains(errRestart.Error(), "replacement exec failed") || !strings.Contains(errRestart.Error(), "restored exec failed") {
			t.Fatalf("replaceAndRestartCurrentProcess() error = %v, want both exec failures", errRestart)
		}
		if execCalls != 2 {
			t.Fatalf("exec calls = %d, want 2", execCalls)
		}
		assertFileContent(t, pending.ExecutablePath, oldContent)
		_, errMarker := os.Stat(markerPath)
		if !errors.Is(errMarker, os.ErrNotExist) {
			t.Fatalf("pending marker remains after rollback, stat error = %v", errMarker)
		}
	})

	t.Run("restarts previous executable when candidate verification fails", func(t *testing.T) {
		pending, markerPath, oldContent, _ := prepareHelperTest(t, RestartModeSelf)
		errCorrupt := os.WriteFile(pending.StagedPath, []byte("corrupt candidate"), 0o600)
		if errCorrupt != nil {
			t.Fatalf("corrupt candidate: %v", errCorrupt)
		}
		execCalls := 0
		errRestart := replaceAndRestartCurrentProcess(markerPath, pending, func(got pendingUpdate) error {
			execCalls++
			assertFileContent(t, got.ExecutablePath, oldContent)
			return errors.New("restored exec failed")
		})
		if errRestart == nil || !errors.Is(errRestart, ErrInvalidStage) || !strings.Contains(errRestart.Error(), "restored exec failed") {
			t.Fatalf("replaceAndRestartCurrentProcess() error = %v, want verification and restored exec failures", errRestart)
		}
		if execCalls != 1 {
			t.Fatalf("exec calls = %d, want 1", execCalls)
		}
		_, errMarker := os.Stat(markerPath)
		if !errors.Is(errMarker, os.ErrNotExist) {
			t.Fatalf("pending marker remains after verification rollback, stat error = %v", errMarker)
		}
	})
}
