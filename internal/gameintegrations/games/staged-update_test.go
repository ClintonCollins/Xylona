package games

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
)

func TestStagedUpdateApply(t *testing.T) {
	t.Run("commits nested overlay and retains rollback data", func(t *testing.T) {
		root := t.TempDir()
		writeStagedUpdateTestFile(t, filepath.Join(root, "bin", "server"), "old")
		writeStagedUpdateTestFile(t, filepath.Join(root, "unrelated.txt"), "user")
		update := newStagedUpdateForTest(t, root)
		writeStagedUpdateTestFile(t, filepath.Join(update.payload, "bin", "server"), "new")
		writeStagedUpdateTestFile(t, filepath.Join(update.payload, "data", "version.txt"), "2")

		errApply := update.Apply(nil, nil)
		if errApply != nil {
			t.Fatalf("Apply() error = %v", errApply)
		}
		assertStagedUpdateTestFile(t, filepath.Join(root, "bin", "server"), "new")
		assertStagedUpdateTestFile(t, filepath.Join(root, "data", "version.txt"), "2")
		assertStagedUpdateTestFile(t, filepath.Join(root, "unrelated.txt"), "user")
		assertStagedUpdateTestFile(
			t,
			filepath.Join(root, filepath.FromSlash(gameintegrations.InternalUpdateFilesDirectory), "bin", "server"),
			"old",
		)
		assertStagedUpdateTestPathExists(
			t,
			filepath.Join(root, filepath.FromSlash(gameintegrations.InternalUpdateCommittedPath)),
		)
	})

	t.Run("rolls back exact changes after mid-commit failure", func(t *testing.T) {
		root := t.TempDir()
		writeStagedUpdateTestFile(t, filepath.Join(root, "a.txt"), "old-a")
		writeStagedUpdateTestFile(t, filepath.Join(root, "b.txt"), "old-b")
		update := newStagedUpdateForTest(t, root)
		writeStagedUpdateTestFile(t, filepath.Join(update.payload, "a.txt"), "new-a")
		writeStagedUpdateTestFile(t, filepath.Join(update.payload, "b.txt"), "new-b")
		update.renamePath = func(oldPath string, newPath string) error {
			if strings.HasPrefix(oldPath, update.payload) && filepath.Base(oldPath) == "b.txt" {
				return errors.New("injected promotion failure")
			}
			return os.Rename(oldPath, newPath)
		}

		errApply := update.Apply(nil, nil)
		if errApply == nil || !strings.Contains(errApply.Error(), "injected promotion failure") {
			t.Fatalf("Apply() error = %v, want injected failure", errApply)
		}
		assertStagedUpdateTestFile(t, filepath.Join(root, "a.txt"), "old-a")
		assertStagedUpdateTestFile(t, filepath.Join(root, "b.txt"), "old-b")
		assertStagedUpdateTestPathMissing(
			t,
			filepath.Join(root, filepath.FromSlash(gameintegrations.InternalUpdateManifestPath)),
		)
	})

	t.Run("retains recovery data when rollback is incomplete", func(t *testing.T) {
		root := t.TempDir()
		writeStagedUpdateTestFile(t, filepath.Join(root, "server.bin"), "old")
		update := newStagedUpdateForTest(t, root)
		writeStagedUpdateTestFile(t, filepath.Join(update.payload, "server.bin"), "new")
		update.renamePath = func(oldPath string, newPath string) error {
			if strings.HasPrefix(oldPath, update.payload) {
				return errors.New("injected promotion failure")
			}
			if strings.Contains(filepath.ToSlash(oldPath), gameintegrations.InternalUpdateFilesDirectory) {
				return errors.New("injected restore failure")
			}
			return os.Rename(oldPath, newPath)
		}

		errApply := update.Apply(nil, nil)
		if errApply == nil || !strings.Contains(errApply.Error(), "injected restore failure") {
			t.Fatalf("Apply() error = %v, want rollback failure", errApply)
		}
		assertStagedUpdateTestPathExists(
			t,
			filepath.Join(root, filepath.FromSlash(gameintegrations.InternalUpdateManifestPath)),
		)
		assertStagedUpdateTestFile(
			t,
			filepath.Join(root, filepath.FromSlash(gameintegrations.InternalUpdateFilesDirectory), "server.bin"),
			"old",
		)
	})

	t.Run("rejects links before live mutation", func(t *testing.T) {
		root := t.TempDir()
		update := newStagedUpdateForTest(t, root)
		target := filepath.Join(update.payload, "target")
		writeStagedUpdateTestFile(t, target, "payload")
		errLink := os.Symlink(target, filepath.Join(update.payload, "link"))
		if errLink != nil {
			t.Skipf("symlinks unavailable: %v", errLink)
		}

		errApply := update.Apply(nil, nil)
		if errApply == nil || !strings.Contains(errApply.Error(), "symlink") {
			t.Fatalf("Apply() error = %v, want symlink rejection", errApply)
		}
		assertStagedUpdateTestPathMissing(
			t,
			filepath.Join(root, filepath.FromSlash(gameintegrations.InternalUpdateManifestPath)),
		)
	})
}

func TestStagedUpdateRefusesUnresolvedTransaction(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, filepath.FromSlash(gameintegrations.InternalUpdateManifestPath))
	writeStagedUpdateTestFile(t, manifestPath, "{}")

	_, errUpdate := newStagedUpdate(root, "factorio", stagedUpdateRetainRollback)
	if errUpdate == nil || !strings.Contains(errUpdate.Error(), "unresolved") {
		t.Fatalf("newStagedUpdate() error = %v, want unresolved transaction", errUpdate)
	}
}

func newStagedUpdateForTest(t *testing.T, root string) *stagedUpdate {
	t.Helper()
	update, errUpdate := newStagedUpdate(root, "test-game", stagedUpdateRetainRollback)
	if errUpdate != nil {
		t.Fatalf("newStagedUpdate() error = %v", errUpdate)
	}
	t.Cleanup(func() {
		errCleanup := update.CleanupTransient()
		if errCleanup != nil {
			t.Errorf("CleanupTransient() error = %v", errCleanup)
		}
	})
	return update
}

func writeStagedUpdateTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	errMkdir := os.MkdirAll(filepath.Dir(path), 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, errMkdir)
	}
	errWrite := os.WriteFile(path, []byte(contents), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, errWrite)
	}
}

func assertStagedUpdateTestFile(t *testing.T, path string, want string) {
	t.Helper()
	contents, errRead := os.ReadFile(path)
	if errRead != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, errRead)
	}
	if string(contents) != want {
		t.Errorf("ReadFile(%q) = %q, want %q", path, contents, want)
	}
}

func assertStagedUpdateTestPathExists(t *testing.T, path string) {
	t.Helper()
	_, errStat := os.Lstat(path)
	if errStat != nil {
		t.Fatalf("Lstat(%q) error = %v", path, errStat)
	}
}

func assertStagedUpdateTestPathMissing(t *testing.T, path string) {
	t.Helper()
	_, errStat := os.Lstat(path)
	if !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Lstat(%q) error = %v, want not exist", path, errStat)
	}
}
