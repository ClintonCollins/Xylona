package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"
)

const (
	helperParentExitTimeout = 15 * time.Minute
	helperOperationAttempts = 120
	helperOperationDelay    = 500 * time.Millisecond
)

var errServiceRestartRollbackUnsafe = errors.New("service state is not safe for executable rollback")

type helperDependencies struct {
	waitForParent  func(pid int, timeout time.Duration) error
	startProcess   func(pending pendingUpdate) (bool, error)
	restartService func(serviceName string) error
}

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
	return runHelperWithDependencies(markerPath, helperDependencies{
		waitForParent:  waitForProcessExit,
		startProcess:   startReplacementProcess,
		restartService: restartWindowsService,
	})
}

func runHelperWithDependencies(markerPath string, deps helperDependencies) (resultErr error) {
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
	if pending.ParentPID <= 0 {
		pending.ParentPID = os.Getppid()
	}
	if pending.RestartMode == "" {
		pending.RestartMode = RestartModeSelf
	}
	if deps.waitForParent == nil {
		return errors.New("selfupdate helper: parent wait function is required")
	}
	if deps.startProcess == nil {
		return errors.New("selfupdate helper: process starter is required")
	}
	if pending.HelperReadyPath == "" {
		return errors.New("selfupdate helper: ready marker path is required")
	}

	errPreverify := verifyFileSHA256(pending.StagedPath, pending.ExpectedSHA256)
	if errPreverify != nil {
		return errPreverify
	}
	errSignalReady := signalHelperReady(pending.HelperReadyPath)
	if errSignalReady != nil {
		return errSignalReady
	}
	defer func() {
		resultErr = errors.Join(resultErr, removeHelperReadyFiles(pending.HelperReadyPath))
	}()

	errWait := deps.waitForParent(pending.ParentPID, helperParentExitTimeout)
	if errWait != nil {
		return fmt.Errorf("selfupdate helper: wait for current process to exit: %w", errWait)
	}

	errVerify := verifyFileSHA256(pending.StagedPath, pending.ExpectedSHA256)
	if errVerify != nil {
		return recoverPreviousExecutable(markerPath, pending, deps, errVerify)
	}

	errReplace := replaceExecutable(pending)
	if errReplace != nil {
		return recoverPreviousExecutable(markerPath, pending, deps, errReplace)
	}

	if pending.RestartMode == RestartModeServiceManager {
		return removePendingMarker(markerPath)
	}
	if pending.RestartMode == RestartModeWindowsService {
		if deps.restartService == nil {
			errRestart := errors.New("selfupdate helper: Windows service restarter is required")
			return recoverPreviousExecutable(markerPath, pending, deps, errRestart)
		}
		errRestart := deps.restartService(pending.ServiceName)
		if errRestart != nil {
			errRestart = fmt.Errorf("selfupdate helper: restart updated Windows service: %w", errRestart)
			if errors.Is(errRestart, errServiceRestartRollbackUnsafe) {
				return errRestart
			}
			return recoverPreviousExecutable(markerPath, pending, deps, errRestart)
		}
		return removePendingMarker(markerPath)
	}
	if pending.RestartMode != RestartModeSelf {
		errMode := fmt.Errorf("selfupdate helper: unsupported restart mode %q", pending.RestartMode)
		return recoverPreviousExecutable(markerPath, pending, deps, errMode)
	}

	started, errStart := deps.startProcess(pending)
	if !started {
		if errStart == nil {
			errStart = errors.New("replacement process did not start")
		}
		errStart = fmt.Errorf("selfupdate helper: start updated executable: %w", errStart)
		return recoverPreviousExecutable(markerPath, pending, deps, errStart)
	}

	errRemoveMarker := removePendingMarker(markerPath)
	if errStart != nil {
		errStart = fmt.Errorf("selfupdate helper: release updated process: %w", errStart)
	}
	return errors.Join(errStart, errRemoveMarker)
}

func replaceExecutable(pending pendingUpdate) error {
	backupExists, errBackupExists := pathExists(pending.BackupPath)
	if errBackupExists != nil {
		return fmt.Errorf("inspect backup executable: %w", errBackupExists)
	}
	executableExists, errExecutableExists := pathExists(pending.ExecutablePath)
	if errExecutableExists != nil {
		return fmt.Errorf("inspect current executable: %w", errExecutableExists)
	}
	stagedExists, errStagedExists := pathExists(pending.StagedPath)
	if errStagedExists != nil {
		return fmt.Errorf("inspect staged executable: %w", errStagedExists)
	}

	if !backupExists {
		if !executableExists {
			return errors.New("backup current executable: current executable is missing")
		}
		if !stagedExists {
			return errors.New("install new executable: staged executable is missing")
		}
		errRenameBackup := retryHelperOperation("backup current executable", func() error {
			return os.Rename(pending.ExecutablePath, pending.BackupPath)
		})
		if errRenameBackup != nil {
			return errRenameBackup
		}
		backupTime := time.Now()
		errBackupTime := os.Chtimes(pending.BackupPath, backupTime, backupTime)
		if errBackupTime != nil {
			return fmt.Errorf("timestamp rollback backup: %w", errBackupTime)
		}
		executableExists = false
	}

	if executableExists && stagedExists {
		return errors.New("install new executable: current, staged, and backup executables all exist")
	}
	if !executableExists {
		if !stagedExists {
			return errors.New("install new executable: both current and staged executables are missing")
		}
		errRenameNew := retryHelperOperation("install new executable", func() error {
			return os.Rename(pending.StagedPath, pending.ExecutablePath)
		})
		if errRenameNew != nil {
			return errRenameNew
		}
	}

	errVerify := verifyFileSHA256(pending.ExecutablePath, pending.ExpectedSHA256)
	if errVerify != nil {
		return fmt.Errorf("verify installed executable: %w", errVerify)
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

func recoverPreviousExecutable(markerPath string, pending pendingUpdate, deps helperDependencies, cause error) error {
	errRestore := restorePreviousExecutable(pending)
	if errRestore != nil {
		return errors.Join(cause, fmt.Errorf("selfupdate helper: restore previous executable: %w", errRestore))
	}
	if pending.RestartMode == RestartModeServiceManager {
		errRemoveMarker := removePendingMarker(markerPath)
		return errors.Join(cause, errRemoveMarker)
	}
	if pending.RestartMode == RestartModeWindowsService {
		if deps.restartService == nil {
			return errors.Join(cause, errors.New("selfupdate helper: Windows service restarter is required after rollback"))
		}
		errRestart := deps.restartService(pending.ServiceName)
		if errRestart != nil {
			errRestart = fmt.Errorf("selfupdate helper: restart restored Windows service: %w", errRestart)
			return errors.Join(cause, errRestart)
		}
		errRemoveMarker := removePendingMarker(markerPath)
		return errors.Join(cause, errRemoveMarker)
	}

	started, errStart := deps.startProcess(pending)
	if !started && errStart == nil {
		errStart = errors.New("restored process did not start")
	}
	if errStart != nil {
		errStart = fmt.Errorf("selfupdate helper: start restored executable: %w", errStart)
	}
	if !started {
		return errors.Join(cause, errStart)
	}
	errRemoveMarker := removePendingMarker(markerPath)
	return errors.Join(cause, errRemoveMarker, errStart)
}

func restorePreviousExecutable(pending pendingUpdate) error {
	backupExists, errBackupExists := pathExists(pending.BackupPath)
	if errBackupExists != nil {
		return fmt.Errorf("inspect backup executable: %w", errBackupExists)
	}
	executableExists, errExecutableExists := pathExists(pending.ExecutablePath)
	if errExecutableExists != nil {
		return fmt.Errorf("inspect current executable: %w", errExecutableExists)
	}
	if !backupExists {
		if executableExists {
			return nil
		}
		return errors.New("backup and current executables are missing")
	}
	if executableExists {
		errRemove := retryHelperOperation("remove failed replacement", func() error {
			err := os.Remove(pending.ExecutablePath)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("remove executable: %w", err)
		})
		if errRemove != nil {
			return errRemove
		}
	}
	errRestore := retryHelperOperation("restore backup executable", func() error {
		return os.Rename(pending.BackupPath, pending.ExecutablePath)
	})
	return errRestore
}

func retryHelperOperation(name string, operation func() error) error {
	var lastErr error
	for attempt := 1; attempt <= helperOperationAttempts; attempt++ {
		lastErr = operation()
		if lastErr == nil {
			return nil
		}
		if attempt < helperOperationAttempts {
			time.Sleep(helperOperationDelay)
		}
	}
	return fmt.Errorf("%s after retries: %w", name, lastErr)
}

func pathExists(pathValue string) (bool, error) {
	_, errStat := os.Stat(pathValue)
	if errStat == nil {
		return true, nil
	}
	if errors.Is(errStat, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat path: %w", errStat)
}

func removePendingMarker(markerPath string) error {
	errRemove := os.Remove(markerPath)
	if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
		return fmt.Errorf("selfupdate helper: remove pending marker: %w", errRemove)
	}
	return nil
}

func signalHelperReady(readyPath string) error {
	tempPath := helperReadyTempPath(readyPath)
	file, errCreate := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errCreate != nil {
		return fmt.Errorf("selfupdate helper: create temporary ready marker: %w", errCreate)
	}
	errClose := file.Close()
	if errClose != nil {
		errRemove := os.Remove(tempPath)
		return errors.Join(
			fmt.Errorf("selfupdate helper: close temporary ready marker: %w", errClose),
			wrapReadyMarkerRemovalError(tempPath, errRemove),
		)
	}
	errRename := os.Rename(tempPath, readyPath)
	if errRename != nil {
		errRemove := os.Remove(tempPath)
		return errors.Join(
			fmt.Errorf("selfupdate helper: publish ready marker: %w", errRename),
			wrapReadyMarkerRemovalError(tempPath, errRemove),
		)
	}
	return nil
}

func helperReadyTempPath(readyPath string) string {
	return readyPath + ".tmp"
}

func removeHelperReadyFiles(readyPath string) error {
	errRemoveReady := os.Remove(readyPath)
	errRemoveTemp := os.Remove(helperReadyTempPath(readyPath))
	return errors.Join(
		wrapReadyMarkerRemovalError(readyPath, errRemoveReady),
		wrapReadyMarkerRemovalError(helperReadyTempPath(readyPath), errRemoveTemp),
	)
}

func wrapReadyMarkerRemovalError(pathValue string, errRemove error) error {
	if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
		return fmt.Errorf("selfupdate helper: remove ready marker %q: %w", pathValue, errRemove)
	}
	return nil
}

func startReplacementProcess(pending pendingUpdate) (bool, error) {
	// #nosec G204 -- the executable and arguments come from Xylona's
	// owner-readable update marker and intentionally define the restart command.
	cmd := exec.CommandContext(context.Background(), pending.ExecutablePath, pending.RestartArgs...)
	cmd.Dir = pending.WorkingDirectory
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	errStart := cmd.Start()
	if errStart != nil {
		return false, fmt.Errorf("start process: %w", errStart)
	}
	errRelease := cmd.Process.Release()
	if errRelease != nil {
		return true, fmt.Errorf("release process: %w", errRelease)
	}
	return true, nil
}
