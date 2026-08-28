//go:build unix

package node

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCopyFilesPreservesPermissions(t *testing.T) {
	const childEnv = "XYLONA_COPY_PERMISSIONS_CHILD"
	if os.Getenv(childEnv) == "" {
		command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestCopyFilesPreservesPermissions$") //nolint:gosec // Re-exec the current test binary with a controlled argument.
		command.Env = append(os.Environ(), childEnv+"=1")
		output, errRun := command.CombinedOutput()
		if errRun != nil {
			t.Fatalf("permission test subprocess error = %v\n%s", errRun, output)
		}
		return
	}

	previousUmask := syscall.Umask(0o077)
	defer syscall.Umask(previousUmask)

	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "source")
	errWrite := os.WriteFile(sourcePath, []byte("content"), 0o770) //nolint:gosec // The test verifies executable group permissions are preserved.
	if errWrite != nil {
		t.Fatalf("WriteFile() error = %v", errWrite)
	}
	errChmod := os.Chmod(sourcePath, 0o770) //nolint:gosec // The test verifies executable group permissions are preserved.
	if errChmod != nil {
		t.Fatalf("Chmod() source error = %v", errChmod)
	}

	_, errCopy := (&Node{}).CopyFiles(t.Context(), directory, []CopyFileOperation{{
		SourceRelativePath:      "source",
		DestinationRelativePath: "destination",
	}}, ProtectionPolicy{})
	if errCopy != nil {
		t.Fatalf("CopyFiles() error = %v", errCopy)
	}
	info, errStat := os.Stat(filepath.Join(directory, "destination"))
	if errStat != nil {
		t.Fatalf("Stat() destination error = %v", errStat)
	}
	if info.Mode().Perm() != 0o770 {
		t.Fatalf("destination permissions = %o, want 770", info.Mode().Perm())
	}
}
