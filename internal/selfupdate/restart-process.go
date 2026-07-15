package selfupdate

import (
	"errors"
	"fmt"
)

type restartProcessFunc func(markerPath string, pending pendingUpdate) error
type execCurrentProcessFunc func(pending pendingUpdate) error

func replaceAndRestartCurrentProcess(markerPath string, pending pendingUpdate, execCurrent execCurrentProcessFunc) error {
	if execCurrent == nil {
		return errors.New("selfupdate: current process exec function is required")
	}
	errVerify := verifyFileSHA256(pending.StagedPath, pending.ExpectedSHA256)
	if errVerify != nil {
		return restartPreviousExecutable(markerPath, pending, errVerify, false, execCurrent)
	}
	errReplace := replaceExecutable(pending)
	if errReplace != nil {
		return restartPreviousExecutable(markerPath, pending, errReplace, true, execCurrent)
	}
	errExec := execCurrent(pending)
	if errExec == nil {
		return nil
	}
	errExec = fmt.Errorf("exec updated executable: %w", errExec)
	return restartPreviousExecutable(markerPath, pending, errExec, true, execCurrent)
}

func restartPreviousExecutable(
	markerPath string,
	pending pendingUpdate,
	cause error,
	restore bool,
	execCurrent execCurrentProcessFunc,
) error {
	if restore {
		errRestore := restorePreviousExecutable(pending)
		if errRestore != nil {
			return errors.Join(cause, fmt.Errorf("restore previous executable: %w", errRestore))
		}
	}
	errRemoveMarker := removePendingMarker(markerPath)
	errExecRestored := execCurrent(pending)
	if errExecRestored != nil {
		errExecRestored = fmt.Errorf("exec restored executable: %w", errExecRestored)
	}
	return errors.Join(cause, errRemoveMarker, errExecRestored)
}
