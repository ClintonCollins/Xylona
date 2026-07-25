package selfupdate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	replacementProcessChildEnvironment  = "XYLONA_SELFUPDATE_TEST_CHILD"
	replacementProcessResultEnvironment = "XYLONA_SELFUPDATE_TEST_RESULT"
	parentWaitChildEnvironment          = "XYLONA_SELFUPDATE_WAIT_TEST_CHILD"
)

type replacementProcessReport struct {
	Arguments        []string `json:"arguments"`
	WorkingDirectory string   `json:"working_directory"`
}

func TestRunHelperRestartsReplacement(t *testing.T) {
	t.Parallel()

	pending, markerPath, oldContent, newContent := prepareHelperTest(t, RestartModeSelf)
	waited := false
	started := false

	deps := helperDependencies{
		waitForParent: func(pid int, timeout time.Duration) error {
			if pid != pending.ParentPID {
				t.Fatalf("wait pid = %d, want %d", pid, pending.ParentPID)
			}
			if timeout != helperParentExitTimeout {
				t.Fatalf("wait timeout = %s, want %s", timeout, helperParentExitTimeout)
			}
			waited = true
			return nil
		},
		startProcess: func(got pendingUpdate) (bool, error) {
			if !waited {
				t.Fatal("replacement started before the parent exit wait completed")
			}
			if !slices.Equal(got.RestartArgs, pending.RestartArgs) {
				t.Fatalf("restart args = %q, want %q", got.RestartArgs, pending.RestartArgs)
			}
			if got.WorkingDirectory != pending.WorkingDirectory {
				t.Fatalf("working directory = %q, want %q", got.WorkingDirectory, pending.WorkingDirectory)
			}
			assertFileContent(t, pending.ExecutablePath, newContent)
			assertFileContent(t, pending.BackupPath, oldContent)
			_, errStat := os.Stat(markerPath)
			if errStat != nil {
				t.Fatalf("marker must exist until replacement starts: %v", errStat)
			}
			_, errStat = os.Stat(pending.HelperReadyPath)
			if errStat != nil {
				t.Fatalf("ready marker must exist during replacement: %v", errStat)
			}
			started = true
			return true, nil
		},
	}

	errRun := runHelperWithDependencies(markerPath, deps)
	if errRun != nil {
		t.Fatalf("runHelperWithDependencies() error = %v", errRun)
	}
	if !started {
		t.Fatal("replacement was not started")
	}
	_, errStat := os.Stat(markerPath)
	if !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("marker still exists after restart, stat error = %v", errStat)
	}
	_, errStat = os.Stat(pending.HelperReadyPath)
	if !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("ready marker still exists after restart, stat error = %v", errStat)
	}
}

func TestRunHelperRestoresPreviousExecutableWhenReplacementFails(t *testing.T) {
	t.Parallel()

	pending, markerPath, oldContent, newContent := prepareHelperTest(t, RestartModeSelf)
	startCalls := 0
	deps := helperDependencies{
		waitForParent: func(int, time.Duration) error {
			return nil
		},
		startProcess: func(pending pendingUpdate) (bool, error) {
			startCalls++
			if startCalls == 1 {
				assertFileContent(t, pending.ExecutablePath, newContent)
				return false, errors.New("replacement launch failed")
			}
			assertFileContent(t, pending.ExecutablePath, oldContent)
			return true, nil
		},
	}

	errRun := runHelperWithDependencies(markerPath, deps)
	if errRun == nil || !strings.Contains(errRun.Error(), "replacement launch failed") {
		t.Fatalf("runHelperWithDependencies() error = %v, want replacement launch failure", errRun)
	}
	if startCalls != 2 {
		t.Fatalf("start calls = %d, want 2", startCalls)
	}
	assertFileContent(t, pending.ExecutablePath, oldContent)
	_, errStat := os.Stat(pending.BackupPath)
	if !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("backup remains after rollback, stat error = %v", errStat)
	}
	_, errStat = os.Stat(markerPath)
	if !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("marker remains after restored process starts, stat error = %v", errStat)
	}
}

func TestRunHelperKeepsRecoveryMarkerWhenRestoredProcessFails(t *testing.T) {
	t.Parallel()

	pending, markerPath, oldContent, _ := prepareHelperTest(t, RestartModeSelf)
	startCalls := 0
	deps := helperDependencies{
		waitForParent: func(int, time.Duration) error {
			return nil
		},
		startProcess: func(pendingUpdate) (bool, error) {
			startCalls++
			return false, errors.New("process launch failed")
		},
	}

	errRun := runHelperWithDependencies(markerPath, deps)
	if errRun == nil || !strings.Contains(errRun.Error(), "process launch failed") {
		t.Fatalf("runHelperWithDependencies() error = %v, want process launch failure", errRun)
	}
	if startCalls != 2 {
		t.Fatalf("start calls = %d, want 2", startCalls)
	}
	assertFileContent(t, pending.ExecutablePath, oldContent)
	_, errStat := os.Stat(markerPath)
	if errStat != nil {
		t.Fatalf("recovery marker was removed after both launches failed: %v", errStat)
	}
}

func TestRunHelperLeavesReplacementToServiceManager(t *testing.T) {
	t.Parallel()

	pending, markerPath, oldContent, newContent := prepareHelperTest(t, RestartModeServiceManager)
	deps := helperDependencies{
		waitForParent: func(int, time.Duration) error {
			return nil
		},
		startProcess: func(pendingUpdate) (bool, error) {
			t.Fatal("service-manager mode started a process")
			return false, nil
		},
	}

	errRun := runHelperWithDependencies(markerPath, deps)
	if errRun != nil {
		t.Fatalf("runHelperWithDependencies() error = %v", errRun)
	}
	assertFileContent(t, pending.ExecutablePath, newContent)
	assertFileContent(t, pending.BackupPath, oldContent)
	_, errStat := os.Stat(markerPath)
	if !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("marker still exists after service-manager handoff, stat error = %v", errStat)
	}
}

func TestRunHelperRestartsWindowsService(t *testing.T) {
	t.Parallel()

	t.Run("replacement service starts", func(t *testing.T) {
		t.Parallel()

		pending, markerPath, oldContent, newContent := prepareHelperTest(t, RestartModeWindowsService)
		pending.ServiceName = "XylonaNode"
		errMarker := writeJSON(markerPath, pending)
		if errMarker != nil {
			t.Fatalf("rewrite pending marker: %v", errMarker)
		}
		restartCalls := 0
		deps := helperDependencies{
			waitForParent: func(int, time.Duration) error {
				return nil
			},
			startProcess: func(pendingUpdate) (bool, error) {
				t.Fatal("Windows-service mode started a replacement process directly")
				return false, nil
			},
			restartService: func(serviceName string) error {
				restartCalls++
				if serviceName != "XylonaNode" {
					t.Fatalf("service name = %q, want XylonaNode", serviceName)
				}
				assertFileContent(t, pending.ExecutablePath, newContent)
				return nil
			},
		}

		errRun := runHelperWithDependencies(markerPath, deps)
		if errRun != nil {
			t.Fatalf("runHelperWithDependencies() error = %v", errRun)
		}
		if restartCalls != 1 {
			t.Fatalf("restart calls = %d, want 1", restartCalls)
		}
		assertFileContent(t, pending.ExecutablePath, newContent)
		assertFileContent(t, pending.BackupPath, oldContent)
		_, errStat := os.Stat(markerPath)
		if !errors.Is(errStat, os.ErrNotExist) {
			t.Fatalf("marker still exists after Windows service restart, stat error = %v", errStat)
		}
	})

	t.Run("restart failure restores previous executable and retries service", func(t *testing.T) {
		t.Parallel()

		pending, markerPath, oldContent, newContent := prepareHelperTest(t, RestartModeWindowsService)
		pending.ServiceName = "Xylona"
		errMarker := writeJSON(markerPath, pending)
		if errMarker != nil {
			t.Fatalf("rewrite pending marker: %v", errMarker)
		}
		restartCalls := 0
		deps := helperDependencies{
			waitForParent: func(int, time.Duration) error {
				return nil
			},
			startProcess: func(pendingUpdate) (bool, error) {
				t.Fatal("Windows-service mode started a replacement process directly")
				return false, nil
			},
			restartService: func(serviceName string) error {
				restartCalls++
				if serviceName != "Xylona" {
					t.Fatalf("service name = %q, want Xylona", serviceName)
				}
				if restartCalls == 1 {
					assertFileContent(t, pending.ExecutablePath, newContent)
					return errors.New("SCM start failed")
				}
				assertFileContent(t, pending.ExecutablePath, oldContent)
				return nil
			},
		}

		errRun := runHelperWithDependencies(markerPath, deps)
		if errRun == nil || !strings.Contains(errRun.Error(), "SCM start failed") {
			t.Fatalf("runHelperWithDependencies() error = %v, want SCM start failure", errRun)
		}
		if restartCalls != 2 {
			t.Fatalf("restart calls = %d, want 2", restartCalls)
		}
		assertFileContent(t, pending.ExecutablePath, oldContent)
		_, errStat := os.Stat(markerPath)
		if !errors.Is(errStat, os.ErrNotExist) {
			t.Fatalf("marker still exists after restored Windows service restart, stat error = %v", errStat)
		}
	})

	t.Run("uncertain service state preserves replacement for safe recovery", func(t *testing.T) {
		t.Parallel()

		pending, markerPath, oldContent, newContent := prepareHelperTest(t, RestartModeWindowsService)
		pending.ServiceName = "XylonaNode"
		errMarker := writeJSON(markerPath, pending)
		if errMarker != nil {
			t.Fatalf("rewrite pending marker: %v", errMarker)
		}
		restartCalls := 0
		deps := helperDependencies{
			waitForParent: func(int, time.Duration) error {
				return nil
			},
			startProcess: func(pendingUpdate) (bool, error) {
				t.Fatal("Windows-service mode started a replacement process directly")
				return false, nil
			},
			restartService: func(serviceName string) error {
				restartCalls++
				if serviceName != "XylonaNode" {
					t.Fatalf("service name = %q, want XylonaNode", serviceName)
				}
				return errors.Join(
					errServiceRestartRollbackUnsafe,
					errors.New("service stop timed out"),
				)
			},
		}

		errRun := runHelperWithDependencies(markerPath, deps)
		if !errors.Is(errRun, errServiceRestartRollbackUnsafe) {
			t.Fatalf("runHelperWithDependencies() error = %v, want unsafe-rollback marker", errRun)
		}
		if restartCalls != 1 {
			t.Fatalf("restart calls = %d, want 1", restartCalls)
		}
		assertFileContent(t, pending.ExecutablePath, newContent)
		assertFileContent(t, pending.BackupPath, oldContent)
		_, errStat := os.Stat(markerPath)
		if errStat != nil {
			t.Fatalf("pending marker was removed after uncertain service stop: %v", errStat)
		}
	})
}

func TestRunHelperDoesNotReplaceBeforeParentExits(t *testing.T) {
	t.Parallel()

	pending, markerPath, oldContent, newContent := prepareHelperTest(t, RestartModeSelf)
	deps := helperDependencies{
		waitForParent: func(int, time.Duration) error {
			return errors.New("parent still running")
		},
		startProcess: func(pendingUpdate) (bool, error) {
			t.Fatal("process started while parent was still running")
			return false, nil
		},
	}

	errRun := runHelperWithDependencies(markerPath, deps)
	if errRun == nil || !strings.Contains(errRun.Error(), "parent still running") {
		t.Fatalf("runHelperWithDependencies() error = %v, want parent wait failure", errRun)
	}
	assertFileContent(t, pending.ExecutablePath, oldContent)
	assertFileContent(t, pending.StagedPath, newContent)
	_, errStat := os.Stat(markerPath)
	if errStat != nil {
		t.Fatalf("marker was removed before parent exit: %v", errStat)
	}
}

func TestRunHelperRejectsInvalidCandidateBeforeReady(t *testing.T) {
	t.Parallel()

	pending, markerPath, oldContent, _ := prepareHelperTest(t, RestartModeSelf)
	corruptContent := []byte("corrupt candidate")
	errWrite := os.WriteFile(pending.StagedPath, corruptContent, 0o600)
	if errWrite != nil {
		t.Fatalf("corrupt staged executable: %v", errWrite)
	}
	deps := helperDependencies{
		waitForParent: func(int, time.Duration) error {
			t.Fatal("helper waited for parent before validating the candidate")
			return nil
		},
		startProcess: func(pendingUpdate) (bool, error) {
			t.Fatal("helper started a process for an invalid candidate")
			return false, nil
		},
	}

	errRun := runHelperWithDependencies(markerPath, deps)
	if errRun == nil || !errors.Is(errRun, ErrInvalidStage) {
		t.Fatalf("runHelperWithDependencies() error = %v, want %v", errRun, ErrInvalidStage)
	}
	assertFileContent(t, pending.ExecutablePath, oldContent)
	assertFileContent(t, pending.StagedPath, corruptContent)
	_, errStat := os.Stat(pending.HelperReadyPath)
	if !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("ready marker exists for invalid candidate, stat error = %v", errStat)
	}
	_, errStat = os.Stat(markerPath)
	if errStat != nil {
		t.Fatalf("pending marker was removed after preflight failure: %v", errStat)
	}
}

func TestStartReplacementProcess(t *testing.T) {
	if os.Getenv(replacementProcessChildEnvironment) == "1" {
		workingDirectory, errWorkingDirectory := os.Getwd()
		if errWorkingDirectory != nil {
			t.Fatalf("get child working directory: %v", errWorkingDirectory)
		}
		report := replacementProcessReport{
			Arguments:        os.Args[1:],
			WorkingDirectory: workingDirectory,
		}
		resultPath := os.Getenv(replacementProcessResultEnvironment)
		data, errMarshal := json.Marshal(report)
		if errMarshal != nil {
			t.Fatalf("marshal child report: %v", errMarshal)
		}
		// #nosec G703 -- the parent test supplies an isolated temporary result path.
		errWrite := os.WriteFile(resultPath, data, 0o600)
		if errWrite != nil {
			t.Fatalf("write child report: %v", errWrite)
		}
		return
	}

	executablePath, errExecutable := os.Executable()
	if errExecutable != nil {
		t.Fatalf("resolve test executable: %v", errExecutable)
	}
	workingDirectory := t.TempDir()
	resultPath := filepath.Join(t.TempDir(), "replacement-process.json")
	t.Setenv(replacementProcessChildEnvironment, "1")
	t.Setenv(replacementProcessResultEnvironment, resultPath)
	restartArgs := []string{"-test.run=^TestStartReplacementProcess$", "--", "restart-value"}

	started, errStart := startReplacementProcess(pendingUpdate{
		ExecutablePath:   executablePath,
		RestartArgs:      restartArgs,
		WorkingDirectory: workingDirectory,
	})
	if errStart != nil {
		t.Fatalf("startReplacementProcess() error = %v", errStart)
	}
	if !started {
		t.Fatal("startReplacementProcess() started = false, want true")
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	var report replacementProcessReport
	for {
		data, errRead := os.ReadFile(resultPath)
		if errRead == nil {
			errUnmarshal := json.Unmarshal(data, &report)
			if errUnmarshal == nil {
				break
			}
		} else if !errors.Is(errRead, os.ErrNotExist) {
			t.Fatalf("read child report: %v", errRead)
		}

		select {
		case <-ticker.C:
		case <-timer.C:
			t.Fatal("timed out waiting for replacement process report")
		}
	}

	if !slices.Equal(report.Arguments, restartArgs) {
		t.Fatalf("replacement arguments = %q, want %q", report.Arguments, restartArgs)
	}
	if report.WorkingDirectory != workingDirectory {
		t.Fatalf("replacement working directory = %q, want %q", report.WorkingDirectory, workingDirectory)
	}
}

func TestWaitForProcessExit(t *testing.T) {
	if os.Getenv(parentWaitChildEnvironment) == "1" {
		return
	}

	executablePath, errExecutable := os.Executable()
	if errExecutable != nil {
		t.Fatalf("resolve test executable: %v", errExecutable)
	}
	// #nosec G204 -- this test launches its own executable in a constrained child mode.
	cmd := exec.CommandContext(t.Context(), executablePath, "-test.run=^TestWaitForProcessExit$")
	cmd.Env = append(os.Environ(), parentWaitChildEnvironment+"=1")
	errStart := cmd.Start()
	if errStart != nil {
		t.Fatalf("start wait test child: %v", errStart)
	}
	waitResult := make(chan error, 1)
	go func() {
		waitResult <- cmd.Wait()
	}()

	errWait := waitForProcessExit(cmd.Process.Pid, 5*time.Second)
	if errWait != nil {
		t.Fatalf("waitForProcessExit(child) error = %v", errWait)
	}
	errChild := <-waitResult
	if errChild != nil {
		t.Fatalf("wait test child error = %v", errChild)
	}

	errTimeout := waitForProcessExit(os.Getpid(), 20*time.Millisecond)
	if errTimeout == nil || !strings.Contains(errTimeout.Error(), "timed out") {
		t.Fatalf("waitForProcessExit(current process) error = %v, want timeout", errTimeout)
	}
}

func prepareHelperTest(t *testing.T, restartMode RestartMode) (pendingUpdate, string, []byte, []byte) {
	t.Helper()

	dir := t.TempDir()
	oldContent := []byte("old executable")
	newContent := []byte("new executable")
	executablePath := filepath.Join(dir, "xylona")
	stagedPath := filepath.Join(dir, "candidate")
	backupPath := filepath.Join(dir, "xylona.backup")
	markerPath := filepath.Join(dir, "pending-update.json")
	helperReadyPath := filepath.Join(dir, "helper-ready")

	errWriteOld := os.WriteFile(executablePath, oldContent, 0o600)
	if errWriteOld != nil {
		t.Fatalf("write old executable: %v", errWriteOld)
	}
	errWriteNew := os.WriteFile(stagedPath, newContent, 0o600)
	if errWriteNew != nil {
		t.Fatalf("write staged executable: %v", errWriteNew)
	}
	sumBytes := sha256.Sum256(newContent)
	pending := pendingUpdate{
		StageID:          "stage-1",
		Component:        "node",
		TargetVersion:    "1.2.3",
		StagedPath:       stagedPath,
		ExecutablePath:   executablePath,
		BackupPath:       backupPath,
		ExpectedSHA256:   hex.EncodeToString(sumBytes[:]),
		ParentPID:        4242,
		RestartArgs:      []string{"--listen", ":9500", "--data-dir", dir},
		WorkingDirectory: dir,
		RestartMode:      restartMode,
		HelperReadyPath:  helperReadyPath,
		CreatedAt:        time.Now().UTC(),
	}
	errMarker := writeJSON(markerPath, pending)
	if errMarker != nil {
		t.Fatalf("write pending marker: %v", errMarker)
	}
	return pending, markerPath, oldContent, newContent
}

func assertFileContent(t *testing.T, pathValue string, expected []byte) {
	t.Helper()

	content, errRead := os.ReadFile(pathValue)
	if errRead != nil {
		t.Fatalf("read %q: %v", pathValue, errRead)
	}
	if !bytes.Equal(content, expected) {
		t.Fatalf("content of %q = %q, want %q", pathValue, content, expected)
	}
}
