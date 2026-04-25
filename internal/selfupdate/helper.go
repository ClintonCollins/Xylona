package selfupdate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"
)

// RunHelperFromArgs executes the update helper mode when args contain the
// hidden helper flag. It returns handled=true when the current process was
// invoked as an updater helper.
func RunHelperFromArgs(args []string) (bool, error) {
	if len(args) < 3 || args[1] != helperArg {
		return false, nil
	}
	errRun := runHelper(args[2])
	if errRun != nil {
		return true, errRun
	}
	return true, nil
}

func runHelper(markerPath string) error {
	file, errOpen := os.Open(markerPath)
	if errOpen != nil {
		return fmt.Errorf("selfupdate helper: open marker: %w", errOpen)
	}
	var pending pendingUpdate
	errDecode := json.NewDecoder(file).Decode(&pending)
	errClose := file.Close()
	if errDecode != nil {
		return fmt.Errorf("selfupdate helper: decode marker: %w", errDecode)
	}
	if errClose != nil {
		return fmt.Errorf("selfupdate helper: close marker: %w", errClose)
	}

	errVerify := verifyFileSHA256(pending.StagedPath, pending.ExpectedSHA256)
	if errVerify != nil {
		return errVerify
	}

	var lastErr error
	for range 120 {
		lastErr = replaceExecutable(pending)
		if lastErr == nil {
			_ = os.Remove(markerPath)
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("selfupdate helper: replace executable after retries: %w", lastErr)
}

func replaceExecutable(pending pendingUpdate) error {
	errRenameBackup := os.Rename(pending.ExecutablePath, pending.BackupPath)
	if errRenameBackup != nil {
		return fmt.Errorf("backup current executable: %w", errRenameBackup)
	}

	errRenameNew := os.Rename(pending.StagedPath, pending.ExecutablePath)
	if errRenameNew != nil {
		errRestore := os.Rename(pending.BackupPath, pending.ExecutablePath)
		if errRestore != nil {
			return errors.Join(
				fmt.Errorf("install new binary: %w", errRenameNew),
				fmt.Errorf("restore backup: %w", errRestore),
			)
		}
		return fmt.Errorf("install new binary: %w", errRenameNew)
	}

	if runtime.GOOS != "windows" {
		// #nosec G302 -- updated binaries must remain executable after replacement.
		errChmod := os.Chmod(pending.ExecutablePath, 0o755)
		if errChmod != nil {
			return fmt.Errorf("chmod updated executable: %w", errChmod)
		}
	}
	return nil
}
