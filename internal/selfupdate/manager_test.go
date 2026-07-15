package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/updater"
)

type fakeHelperProcess struct {
	releaseErr error
	stopErr    error
	stopped    bool
}

func writeManagerExecutable(t *testing.T, manager *Manager) {
	t.Helper()
	errWrite := os.WriteFile(manager.executablePath, []byte("current xylona executable"), 0o600)
	if errWrite != nil {
		t.Fatalf("write manager executable: %v", errWrite)
	}
}

func (p *fakeHelperProcess) Release() error {
	return p.releaseErr
}

func (p *fakeHelperProcess) Stop() error {
	p.stopped = true
	return p.stopErr
}

func TestManagerStageSelfUpdate(t *testing.T) {
	t.Parallel()

	content := []byte("new xylona-node binary")
	sumBytes := sha256.Sum256(content)
	sum := hex.EncodeToString(sumBytes[:])
	exePath := t.TempDir() + "/xylona-node"

	manager, errManager := NewManager(Config{
		Component:      "node",
		StageDir:       t.TempDir(),
		ExecutablePath: exePath,
		ShutdownFunc:   func() {},
	})
	if errManager != nil {
		t.Fatalf("NewManager() error = %v", errManager)
	}
	writeManagerExecutable(t, manager)

	result, errStage := manager.Stage(t.Context(), node.StageSelfUpdateRequest{
		Component:      "node",
		TargetVersion:  "1.2.3",
		OS:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
		ExpectedSize:   int64(len(content)),
		ExpectedSHA256: sum,
		Reader:         bytes.NewReader(content),
	})
	if errStage != nil {
		t.Fatalf("Stage() error = %v", errStage)
	}
	if result.BytesWritten != int64(len(content)) {
		t.Fatalf("Stage().BytesWritten = %d, want %d", result.BytesWritten, len(content))
	}
	if result.SHA256 != sum {
		t.Fatalf("Stage().SHA256 = %q, want %q", result.SHA256, sum)
	}
}

func TestManagerStageRejectsBadInput(t *testing.T) {
	t.Parallel()

	manager, errManager := NewManager(Config{
		Component:      "node",
		StageDir:       t.TempDir(),
		ExecutablePath: t.TempDir() + "/xylona-node",
		ShutdownFunc:   func() {},
	})
	if errManager != nil {
		t.Fatalf("NewManager() error = %v", errManager)
	}

	_, errComponent := manager.Stage(t.Context(), node.StageSelfUpdateRequest{
		Component:      "controller",
		ExpectedSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Reader:         bytes.NewReader([]byte("content")),
	})
	if errComponent == nil {
		t.Fatal("Stage(wrong component) error = nil, want error")
	}

	_, errHash := manager.Stage(t.Context(), node.StageSelfUpdateRequest{
		Component:      "node",
		OS:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
		ExpectedSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Reader:         bytes.NewReader([]byte("content")),
	})
	if errHash == nil {
		t.Fatal("Stage(bad checksum) error = nil, want error")
	}
}

func TestManagerApplyStartsHelperAfterRequestContextCanceled(t *testing.T) {
	t.Parallel()

	content := []byte("new xylona-node binary")
	sumBytes := sha256.Sum256(content)
	sum := hex.EncodeToString(sumBytes[:])
	shutdownCalls := make(chan struct{}, 1)

	manager, errManager := NewManager(Config{
		Component:      "node",
		StageDir:       t.TempDir(),
		ExecutablePath: t.TempDir() + "/xylona-node",
		ShutdownFunc: func() {
			shutdownCalls <- struct{}{}
		},
	})
	if errManager != nil {
		t.Fatalf("NewManager() error = %v", errManager)
	}
	writeManagerExecutable(t, manager)
	t.Cleanup(func() {
		applyingExecutables.Delete(manager.executablePath)
	})
	manager.shutdownDelay = 0
	manager.inProcessRestart = false
	manager.waitHelperReady = func(string, time.Duration) error {
		return nil
	}

	startCalls := make(chan [2]string, 1)
	manager.startHelper = func(helperPath string, pendingPath string) (helperProcess, error) {
		startCalls <- [2]string{helperPath, pendingPath}
		return &fakeHelperProcess{}, nil
	}

	result, errStage := manager.Stage(t.Context(), node.StageSelfUpdateRequest{
		Component:      "node",
		TargetVersion:  "1.2.3",
		OS:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
		ExpectedSize:   int64(len(content)),
		ExpectedSHA256: sum,
		Reader:         bytes.NewReader(content),
	})
	if errStage != nil {
		t.Fatalf("Stage() error = %v", errStage)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	applyResult, errApply := manager.Apply(ctx, node.ApplySelfUpdateRequest{
		StageID:        result.StageID,
		TargetVersion:  "1.2.3",
		ExpectedSHA256: result.SHA256,
	})
	if errApply != nil {
		t.Fatalf("Apply() error = %v", errApply)
	}
	if !applyResult.Accepted {
		t.Fatal("Apply().Accepted = false, want true")
	}

	select {
	case call := <-startCalls:
		if call[0] == "" || call[1] == "" {
			t.Fatalf("start helper call = %q, want helper and pending paths", call)
		}
	default:
		t.Fatal("Apply() did not start helper")
	}

	select {
	case <-shutdownCalls:
	case <-time.After(time.Second):
		t.Fatal("Apply() did not request application shutdown")
	}

	_, errSecondApply := manager.Apply(t.Context(), node.ApplySelfUpdateRequest{
		StageID:        result.StageID,
		TargetVersion:  "1.2.3",
		ExpectedSHA256: result.SHA256,
	})
	if !errors.Is(errSecondApply, ErrApplyInProgress) {
		t.Fatalf("second Apply() error = %v, want %v", errSecondApply, ErrApplyInProgress)
	}
}

func TestManagerApplyPreparesInstallCandidateNextToExecutable(t *testing.T) {
	t.Parallel()

	content := []byte("new xylona-node binary")
	sumBytes := sha256.Sum256(content)
	sum := hex.EncodeToString(sumBytes[:])
	executableDir := t.TempDir()
	executablePath := filepath.Join(executableDir, "xylona-node")
	workingDirectory := t.TempDir()
	restartArgs := []string{"--listen", ":9500", "--data-dir", "node-data"}

	manager, errManager := NewManager(Config{
		Component:        "node",
		StageDir:         t.TempDir(),
		ExecutablePath:   executablePath,
		RestartArgs:      restartArgs,
		WorkingDirectory: workingDirectory,
		RestartMode:      RestartModeSelf,
		ShutdownFunc:     func() {},
	})
	if errManager != nil {
		t.Fatalf("NewManager() error = %v", errManager)
	}
	writeManagerExecutable(t, manager)
	t.Cleanup(func() {
		applyingExecutables.Delete(manager.executablePath)
	})
	restartArgs[0] = "mutated"
	manager.inProcessRestart = false
	manager.waitHelperReady = func(string, time.Duration) error {
		return nil
	}

	startCalls := make(chan [2]string, 1)
	manager.startHelper = func(helperPath string, pendingPath string) (helperProcess, error) {
		startCalls <- [2]string{helperPath, pendingPath}
		return &fakeHelperProcess{}, nil
	}

	result, errStage := manager.Stage(t.Context(), node.StageSelfUpdateRequest{
		Component:      "node",
		TargetVersion:  "1.2.3",
		OS:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
		ExpectedSize:   int64(len(content)),
		ExpectedSHA256: sum,
		Reader:         bytes.NewReader(content),
	})
	if errStage != nil {
		t.Fatalf("Stage() error = %v", errStage)
	}

	_, errApply := manager.Apply(t.Context(), node.ApplySelfUpdateRequest{
		StageID:        result.StageID,
		TargetVersion:  "1.2.3",
		ExpectedSHA256: result.SHA256,
	})
	if errApply != nil {
		t.Fatalf("Apply() error = %v", errApply)
	}

	var call [2]string
	select {
	case call = <-startCalls:
	default:
		t.Fatal("Apply() did not start helper")
	}
	if filepath.Base(call[1]) != result.StageID+"-pending.json" {
		t.Fatalf("pending marker = %q, want stage-specific marker", call[1])
	}

	data, errRead := os.ReadFile(call[1])
	if errRead != nil {
		t.Fatalf("read pending update: %v", errRead)
	}
	var pending pendingUpdate
	errDecode := json.Unmarshal(data, &pending)
	if errDecode != nil {
		t.Fatalf("decode pending update: %v", errDecode)
	}
	if filepath.Dir(pending.StagedPath) != executableDir {
		t.Fatalf("pending staged dir = %q, want %q", filepath.Dir(pending.StagedPath), executableDir)
	}
	if pending.ParentPID != os.Getpid() {
		t.Fatalf("pending parent PID = %d, want %d", pending.ParentPID, os.Getpid())
	}
	wantRestartArgs := []string{"--listen", ":9500", "--data-dir", "node-data"}
	if !slices.Equal(pending.RestartArgs, wantRestartArgs) {
		t.Fatalf("pending restart args = %q, want %q", pending.RestartArgs, wantRestartArgs)
	}
	if pending.WorkingDirectory != workingDirectory {
		t.Fatalf("pending working directory = %q, want %q", pending.WorkingDirectory, workingDirectory)
	}
	if pending.RestartMode != RestartModeSelf {
		t.Fatalf("pending restart mode = %q, want %q", pending.RestartMode, RestartModeSelf)
	}
	if pending.HelperReadyPath == "" {
		t.Fatal("pending helper ready path is empty")
	}
	if pending.BackupPath != executablePath+".bak-"+result.StageID {
		t.Fatalf("pending backup path = %q, want stage-specific backup", pending.BackupPath)
	}
	candidateContent, errReadCandidate := os.ReadFile(pending.StagedPath)
	if errReadCandidate != nil {
		t.Fatalf("read install candidate: %v", errReadCandidate)
	}
	if !bytes.Equal(candidateContent, content) {
		t.Fatalf("install candidate content = %q, want %q", candidateContent, content)
	}
}

func TestManagerApplyCanRetryAfterHelperStartFailure(t *testing.T) {
	t.Parallel()

	content := []byte("new xylona-node binary")
	sumBytes := sha256.Sum256(content)
	sum := hex.EncodeToString(sumBytes[:])
	manager, errManager := NewManager(Config{
		Component:      "node",
		StageDir:       t.TempDir(),
		ExecutablePath: filepath.Join(t.TempDir(), "xylona-node"),
		ShutdownFunc:   func() {},
	})
	if errManager != nil {
		t.Fatalf("NewManager() error = %v", errManager)
	}
	writeManagerExecutable(t, manager)
	t.Cleanup(func() {
		applyingExecutables.Delete(manager.executablePath)
	})

	result, errStage := manager.Stage(t.Context(), node.StageSelfUpdateRequest{
		Component:      "node",
		TargetVersion:  "1.2.3",
		OS:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
		ExpectedSize:   int64(len(content)),
		ExpectedSHA256: sum,
		Reader:         bytes.NewReader(content),
	})
	if errStage != nil {
		t.Fatalf("Stage() error = %v", errStage)
	}

	startCalls := 0
	manager.inProcessRestart = false
	var firstHelperPath string
	var firstPendingPath string
	var firstCandidatePath string
	manager.waitHelperReady = func(string, time.Duration) error {
		return nil
	}
	manager.startHelper = func(helperPath string, pendingPath string) (helperProcess, error) {
		startCalls++
		if startCalls == 1 {
			firstHelperPath = helperPath
			firstPendingPath = pendingPath
			data, errRead := os.ReadFile(pendingPath)
			if errRead != nil {
				t.Fatalf("read first pending marker: %v", errRead)
			}
			var pending pendingUpdate
			errDecode := json.Unmarshal(data, &pending)
			if errDecode != nil {
				t.Fatalf("decode first pending marker: %v", errDecode)
			}
			firstCandidatePath = pending.StagedPath
			return nil, errors.New("helper launch failed")
		}
		return &fakeHelperProcess{}, nil
	}

	request := node.ApplySelfUpdateRequest{
		StageID:        result.StageID,
		TargetVersion:  "1.2.3",
		ExpectedSHA256: result.SHA256,
	}
	_, errFirstApply := manager.Apply(t.Context(), request)
	if errFirstApply == nil || !strings.Contains(errFirstApply.Error(), "helper launch failed") {
		t.Fatalf("first Apply() error = %v, want helper launch failure", errFirstApply)
	}
	for _, pathValue := range []string{firstHelperPath, firstPendingPath, firstCandidatePath} {
		_, errStat := os.Stat(pathValue)
		if !errors.Is(errStat, os.ErrNotExist) {
			t.Fatalf("handoff artifact %q remains after start failure, stat error = %v", pathValue, errStat)
		}
	}

	applyResult, errRetry := manager.Apply(t.Context(), request)
	if errRetry != nil {
		t.Fatalf("retry Apply() error = %v", errRetry)
	}
	if !applyResult.Accepted {
		t.Fatal("retry Apply().Accepted = false, want true")
	}
	if startCalls != 2 {
		t.Fatalf("helper start calls = %d, want 2", startCalls)
	}
}

func TestManagerApplyDoesNotShutdownBeforeHelperReady(t *testing.T) {
	t.Parallel()

	content := []byte("new xylona-node binary")
	sumBytes := sha256.Sum256(content)
	sum := hex.EncodeToString(sumBytes[:])
	shutdownCalls := make(chan struct{}, 1)
	manager, errManager := NewManager(Config{
		Component:      "node",
		StageDir:       t.TempDir(),
		ExecutablePath: filepath.Join(t.TempDir(), "xylona-node"),
		ShutdownFunc: func() {
			shutdownCalls <- struct{}{}
		},
	})
	if errManager != nil {
		t.Fatalf("NewManager() error = %v", errManager)
	}
	writeManagerExecutable(t, manager)
	t.Cleanup(func() {
		applyingExecutables.Delete(manager.executablePath)
	})
	manager.shutdownDelay = 0
	manager.inProcessRestart = false

	result, errStage := manager.Stage(t.Context(), node.StageSelfUpdateRequest{
		Component:      "node",
		TargetVersion:  "1.2.3",
		OS:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
		ExpectedSize:   int64(len(content)),
		ExpectedSHA256: sum,
		Reader:         bytes.NewReader(content),
	})
	if errStage != nil {
		t.Fatalf("Stage() error = %v", errStage)
	}

	fakeProcess := &fakeHelperProcess{}
	manager.startHelper = func(string, string) (helperProcess, error) {
		return fakeProcess, nil
	}
	manager.waitHelperReady = func(string, time.Duration) error {
		return errors.New("helper readiness failed")
	}
	_, errApply := manager.Apply(t.Context(), node.ApplySelfUpdateRequest{
		StageID:        result.StageID,
		TargetVersion:  "1.2.3",
		ExpectedSHA256: result.SHA256,
	})
	if errApply == nil || !strings.Contains(errApply.Error(), "helper readiness failed") {
		t.Fatalf("Apply() error = %v, want helper readiness failure", errApply)
	}
	if !fakeProcess.stopped {
		t.Fatal("unready helper process was not stopped")
	}
	select {
	case <-shutdownCalls:
		t.Fatal("application shutdown was requested before helper readiness")
	default:
	}
}

func TestManagerApplyCompletesInProcessRestartAfterShutdown(t *testing.T) {
	content := []byte("new xylona binary")
	shutdownCalls := make(chan struct{}, 1)
	manager, errManager := NewManager(Config{
		Component:      "node",
		StageDir:       t.TempDir(),
		ExecutablePath: filepath.Join(t.TempDir(), "xylona"),
		ShutdownFunc: func() {
			shutdownCalls <- struct{}{}
		},
	})
	if errManager != nil {
		t.Fatalf("NewManager() error = %v", errManager)
	}
	writeManagerExecutable(t, manager)
	t.Cleanup(func() {
		applyingExecutables.Delete(manager.executablePath)
	})
	manager.inProcessRestart = true
	manager.shutdownDelay = 0
	manager.startHelper = func(string, string) (helperProcess, error) {
		t.Fatal("in-process restart started a helper")
		return nil, errors.New("unexpected helper start")
	}
	restarted := false
	manager.restartProcess = func(markerPath string, pending pendingUpdate) error {
		restarted = true
		if markerPath != manager.pendingRestart {
			t.Fatalf("restart marker = %q, want %q", markerPath, manager.pendingRestart)
		}
		assertFileContent(t, manager.executablePath, []byte("current xylona executable"))
		assertFileContent(t, pending.StagedPath, content)
		return nil
	}

	stage := stageManagerTestArtifact(t, manager, "1.2.3", content)
	result, errApply := manager.Apply(t.Context(), node.ApplySelfUpdateRequest{
		StageID:        stage.StageID,
		TargetVersion:  "1.2.3",
		ExpectedSHA256: stage.SHA256,
	})
	if errApply != nil {
		t.Fatalf("Apply() error = %v", errApply)
	}
	if !result.Accepted {
		t.Fatal("Apply().Accepted = false, want true")
	}
	if restarted {
		t.Fatal("Apply() restarted before service cleanup")
	}
	select {
	case <-shutdownCalls:
	case <-time.After(time.Second):
		t.Fatal("Apply() did not request shutdown")
	}

	errComplete := manager.CompleteSelfUpdate()
	if errComplete != nil {
		t.Fatalf("CompleteSelfUpdate() error = %v", errComplete)
	}
	if !restarted {
		t.Fatal("CompleteSelfUpdate() did not restart the process")
	}
}

func TestManagerReconcilesAbandonedInProcessRestart(t *testing.T) {
	manager := newArtifactTestManager(t)
	manager.inProcessRestart = true
	manager.shutdownDelay = time.Hour
	t.Cleanup(func() {
		applyingExecutables.Delete(manager.executablePath)
	})

	stage := stageManagerTestArtifact(t, manager, "1.2.3", []byte("new xylona binary"))
	_, errApply := manager.Apply(t.Context(), node.ApplySelfUpdateRequest{
		StageID:        stage.StageID,
		TargetVersion:  "1.2.3",
		ExpectedSHA256: stage.SHA256,
	})
	if errApply != nil {
		t.Fatalf("Apply() error = %v", errApply)
	}
	pendingPath := manager.pendingRestart
	pending, errPending := readPendingUpdate(pendingPath)
	if errPending != nil {
		t.Fatalf("readPendingUpdate() error = %v", errPending)
	}
	pending.ParentPID = os.Getpid() + 1
	errWrite := writeJSON(pendingPath, pending)
	if errWrite != nil {
		t.Fatalf("rewrite pending handoff: %v", errWrite)
	}
	applyingExecutables.Delete(manager.executablePath)

	handoffPending, errReconcile := manager.reconcileArtifacts(maxRetainedStagedUpdates)
	if errReconcile != nil {
		t.Fatalf("reconcileArtifacts() error = %v", errReconcile)
	}
	if handoffPending {
		t.Fatal("reconcileArtifacts() retained an abandoned in-process handoff")
	}
	for _, pathValue := range []string{pendingPath, pending.StagedPath} {
		_, errStat := os.Stat(pathValue)
		if !errors.Is(errStat, os.ErrNotExist) {
			t.Fatalf("abandoned handoff artifact %q remains, stat error = %v", pathValue, errStat)
		}
	}
}

func TestManagerArtifactReconciliation(t *testing.T) {
	t.Run("bounds unapplied stages", func(t *testing.T) {
		manager := newArtifactTestManager(t)
		baseTime := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)

		manager.now = func() time.Time { return baseTime.Add(-2 * time.Minute) }
		first := stageManagerTestArtifact(t, manager, "1.1.0", []byte("first staged update"))
		manager.now = func() time.Time { return baseTime.Add(-time.Minute) }
		second := stageManagerTestArtifact(t, manager, "1.2.0", []byte("second staged update"))
		manager.now = func() time.Time { return baseTime }
		third := stageManagerTestArtifact(t, manager, "1.3.0", []byte("third staged update"))

		assertStageExists(t, manager, second.StageID)
		assertStageExists(t, manager, third.StageID)
		assertStageRemoved(t, manager, first.StageID)
	})

	t.Run("cleans a stage confirmed by the running executable", func(t *testing.T) {
		manager := newArtifactTestManager(t)
		currentContent, errRead := os.ReadFile(manager.executablePath)
		if errRead != nil {
			t.Fatalf("read current executable: %v", errRead)
		}
		stage := stageManagerTestArtifact(t, manager, "1.1.0", currentContent)

		pending, errReconcile := manager.reconcileArtifacts(maxRetainedStagedUpdates)
		if errReconcile != nil {
			t.Fatalf("reconcileArtifacts() error = %v", errReconcile)
		}
		if pending {
			t.Fatal("reconcileArtifacts() pending = true, want false")
		}
		assertStageRemoved(t, manager, stage.StageID)
	})

	t.Run("retains only the newest rollback backup", func(t *testing.T) {
		manager := newArtifactTestManager(t)
		baseTime := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
		stageIDs := []string{
			"11111111-1111-4111-8111-111111111111",
			"22222222-2222-4222-8222-222222222222",
			"33333333-3333-4333-8333-333333333333",
		}
		for idx, stageID := range stageIDs {
			pathValue := manager.executablePath + ".bak-" + stageID
			errWrite := os.WriteFile(pathValue, []byte(stageID), 0o600)
			if errWrite != nil {
				t.Fatalf("write rollback backup: %v", errWrite)
			}
			modTime := baseTime.Add(time.Duration(idx) * time.Minute)
			errTime := os.Chtimes(pathValue, modTime, modTime)
			if errTime != nil {
				t.Fatalf("set rollback backup time: %v", errTime)
			}
		}

		_, errReconcile := manager.reconcileArtifacts(maxRetainedStagedUpdates)
		if errReconcile != nil {
			t.Fatalf("reconcileArtifacts() error = %v", errReconcile)
		}
		for _, stageID := range stageIDs[:2] {
			pathValue := manager.executablePath + ".bak-" + stageID
			_, errStat := os.Stat(pathValue)
			if !errors.Is(errStat, os.ErrNotExist) {
				t.Fatalf("old rollback backup %q remains, stat error = %v", pathValue, errStat)
			}
		}
		newestPath := manager.executablePath + ".bak-" + stageIDs[2]
		_, errStat := os.Stat(newestPath)
		if errStat != nil {
			t.Fatalf("newest rollback backup stat error = %v", errStat)
		}
	})

	t.Run("restores the newest backup when the executable is missing", func(t *testing.T) {
		manager := newArtifactTestManager(t)
		errRemove := os.Remove(manager.executablePath)
		if errRemove != nil {
			t.Fatalf("remove current executable: %v", errRemove)
		}
		olderID := "44444444-4444-4444-8444-444444444444"
		newerID := "55555555-5555-4555-8555-555555555555"
		olderPath := manager.executablePath + ".bak-" + olderID
		newerPath := manager.executablePath + ".bak-" + newerID
		errWrite := os.WriteFile(olderPath, []byte("older backup"), 0o600)
		if errWrite != nil {
			t.Fatalf("write older backup: %v", errWrite)
		}
		errWrite = os.WriteFile(newerPath, []byte("newer backup"), 0o600)
		if errWrite != nil {
			t.Fatalf("write newer backup: %v", errWrite)
		}
		baseTime := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
		errTime := os.Chtimes(olderPath, baseTime, baseTime)
		if errTime != nil {
			t.Fatalf("set older backup time: %v", errTime)
		}
		errTime = os.Chtimes(newerPath, baseTime.Add(time.Minute), baseTime.Add(time.Minute))
		if errTime != nil {
			t.Fatalf("set newer backup time: %v", errTime)
		}

		_, errReconcile := manager.reconcileArtifacts(maxRetainedStagedUpdates)
		if errReconcile != nil {
			t.Fatalf("reconcileArtifacts() error = %v", errReconcile)
		}
		content, errRead := os.ReadFile(manager.executablePath)
		if errRead != nil {
			t.Fatalf("read restored executable: %v", errRead)
		}
		if string(content) != "newer backup" {
			t.Fatalf("restored executable = %q, want newer backup", content)
		}
		_, errStat := os.Stat(olderPath)
		if errStat != nil {
			t.Fatalf("retained rollback backup stat error = %v", errStat)
		}
	})

	t.Run("preserves a recent pending handoff", func(t *testing.T) {
		manager := newArtifactTestManager(t)
		stageID := "66666666-6666-4666-8666-666666666666"
		pendingPath := writeReconciliationPending(t, manager, stageID, strings.Repeat("a", 64))

		pending, errReconcile := manager.reconcileArtifacts(maxRetainedStagedUpdates)
		if errReconcile != nil {
			t.Fatalf("reconcileArtifacts() error = %v", errReconcile)
		}
		if !pending {
			t.Fatal("reconcileArtifacts() pending = false, want true")
		}
		_, errStat := os.Stat(pendingPath)
		if errStat != nil {
			t.Fatalf("recent pending handoff stat error = %v", errStat)
		}
	})

	t.Run("confirms a recent handoff running the staged executable", func(t *testing.T) {
		manager := newArtifactTestManager(t)
		currentContent, errRead := os.ReadFile(manager.executablePath)
		if errRead != nil {
			t.Fatalf("read current executable: %v", errRead)
		}
		stage := stageManagerTestArtifact(t, manager, "1.1.0", currentContent)
		pendingPath := writeReconciliationPending(t, manager, stage.StageID, stage.SHA256)

		pending, errReconcile := manager.reconcileArtifacts(maxRetainedStagedUpdates)
		if errReconcile != nil {
			t.Fatalf("reconcileArtifacts() error = %v", errReconcile)
		}
		if pending {
			t.Fatal("reconcileArtifacts() pending = true, want false")
		}
		_, errStat := os.Stat(pendingPath)
		if !errors.Is(errStat, os.ErrNotExist) {
			t.Fatalf("confirmed pending handoff remains, stat error = %v", errStat)
		}
		assertStageRemoved(t, manager, stage.StageID)
	})

	t.Run("clears an orphaned pending handoff", func(t *testing.T) {
		manager := newArtifactTestManager(t)
		stageID := "77777777-7777-4777-8777-777777777777"
		pendingPath := writeReconciliationPending(t, manager, stageID, strings.Repeat("a", 64))
		candidatePath := manager.installCandidatePath(stageID)
		errWrite := os.WriteFile(candidatePath, []byte("candidate"), 0o600)
		if errWrite != nil {
			t.Fatalf("write install candidate: %v", errWrite)
		}
		baseTime := time.Now().Add(-orphanedHandoffAge - time.Minute)
		errTime := os.Chtimes(pendingPath, baseTime, baseTime)
		if errTime != nil {
			t.Fatalf("set pending handoff time: %v", errTime)
		}

		pending, errReconcile := manager.reconcileArtifacts(maxRetainedStagedUpdates)
		if errReconcile != nil {
			t.Fatalf("reconcileArtifacts() error = %v", errReconcile)
		}
		if pending {
			t.Fatal("reconcileArtifacts() pending = true, want false")
		}
		for _, pathValue := range []string{pendingPath, candidatePath} {
			_, errStat := os.Stat(pathValue)
			if !errors.Is(errStat, os.ErrNotExist) {
				t.Fatalf("orphaned handoff artifact %q remains, stat error = %v", pathValue, errStat)
			}
		}
	})

	t.Run("discards a stale invalid handoff without deleting its retryable stage", func(t *testing.T) {
		manager := newArtifactTestManager(t)
		stage := stageManagerTestArtifact(t, manager, "1.1.0", []byte("staged update"))
		pendingPath := filepath.Join(manager.stageDir, stage.StageID+"-pending.json")
		errWrite := os.WriteFile(pendingPath, []byte("{invalid"), 0o600)
		if errWrite != nil {
			t.Fatalf("write invalid pending handoff: %v", errWrite)
		}
		candidatePath := manager.installCandidatePath(stage.StageID)
		errWrite = os.WriteFile(candidatePath, []byte("candidate"), 0o600)
		if errWrite != nil {
			t.Fatalf("write invalid handoff candidate: %v", errWrite)
		}
		helperPath := filepath.Join(manager.stageDir, stage.StageID+"-88888888-8888-4888-8888-888888888888-helper")
		errWrite = os.WriteFile(helperPath, []byte("helper"), 0o600)
		if errWrite != nil {
			t.Fatalf("write invalid handoff helper: %v", errWrite)
		}
		staleTime := time.Now().Add(-orphanedHandoffAge - time.Minute)
		errTime := os.Chtimes(pendingPath, staleTime, staleTime)
		if errTime != nil {
			t.Fatalf("set invalid pending handoff time: %v", errTime)
		}

		pending, errReconcile := manager.reconcileArtifacts(maxRetainedStagedUpdates)
		if errReconcile != nil {
			t.Fatalf("reconcileArtifacts() error = %v", errReconcile)
		}
		if pending {
			t.Fatal("reconcileArtifacts() pending = true, want false")
		}
		for _, pathValue := range []string{pendingPath, candidatePath, helperPath} {
			_, errStat := os.Stat(pathValue)
			if !errors.Is(errStat, os.ErrNotExist) {
				t.Fatalf("invalid handoff artifact %q remains, stat error = %v", pathValue, errStat)
			}
		}
		assertStageExists(t, manager, stage.StageID)
	})

	t.Run("protects recent legacy handoffs and clears stale markers", func(t *testing.T) {
		manager := newArtifactTestManager(t)
		legacyPath := filepath.Join(manager.stageDir, "pending-update.json")
		errWrite := os.WriteFile(legacyPath, []byte("{}"), 0o600)
		if errWrite != nil {
			t.Fatalf("write legacy pending marker: %v", errWrite)
		}

		pending, errRecent := manager.reconcileArtifacts(maxRetainedStagedUpdates)
		if errRecent != nil {
			t.Fatalf("reconcile recent legacy marker: %v", errRecent)
		}
		if !pending {
			t.Fatal("recent legacy marker did not protect the handoff")
		}

		staleTime := time.Now().Add(-orphanedHandoffAge - time.Minute)
		errTime := os.Chtimes(legacyPath, staleTime, staleTime)
		if errTime != nil {
			t.Fatalf("set legacy pending marker time: %v", errTime)
		}
		pending, errStale := manager.reconcileArtifacts(maxRetainedStagedUpdates)
		if errStale != nil {
			t.Fatalf("reconcile stale legacy marker: %v", errStale)
		}
		if pending {
			t.Fatal("stale legacy marker still protects the handoff")
		}
		_, errStat := os.Stat(legacyPath)
		if !errors.Is(errStat, os.ErrNotExist) {
			t.Fatalf("stale legacy marker remains, stat error = %v", errStat)
		}
	})
}

func TestManagerCapacityPreflight(t *testing.T) {
	t.Run("rejects insufficient stage volume", func(t *testing.T) {
		manager := newArtifactTestManager(t)
		manager.ensureFreeSpace = func(pathValue string, _ ...int64) error {
			if filepath.Clean(pathValue) == manager.stageDir {
				return updater.ErrInsufficientDiskSpace
			}
			return nil
		}
		_, errStage := stageManagerArtifact(manager, "1.1.0", []byte("staged update"))
		if !errors.Is(errStage, updater.ErrInsufficientDiskSpace) {
			t.Fatalf("Stage() error = %v, want ErrInsufficientDiskSpace", errStage)
		}
	})

	t.Run("removes stage when the install volume is insufficient", func(t *testing.T) {
		manager := newArtifactTestManager(t)
		manager.ensureFreeSpace = func(pathValue string, _ ...int64) error {
			if filepath.Clean(pathValue) == filepath.Dir(manager.executablePath) {
				return updater.ErrInsufficientDiskSpace
			}
			return nil
		}
		_, errStage := stageManagerArtifact(manager, "1.1.0", []byte("staged update"))
		if !errors.Is(errStage, updater.ErrInsufficientDiskSpace) {
			t.Fatalf("Stage() error = %v, want ErrInsufficientDiskSpace", errStage)
		}
		entries, errReadDir := os.ReadDir(manager.stageDir)
		if errReadDir != nil {
			t.Fatalf("read stage directory: %v", errReadDir)
		}
		if len(entries) != 0 {
			t.Fatalf("stage directory contains %d entries after capacity failure, want 0", len(entries))
		}
	})

	t.Run("rechecks install capacity before apply", func(t *testing.T) {
		manager := newArtifactTestManager(t)
		stage := stageManagerTestArtifact(t, manager, "1.1.0", []byte("staged update"))
		manager.ensureFreeSpace = func(string, ...int64) error {
			return updater.ErrInsufficientDiskSpace
		}
		_, errApply := manager.Apply(t.Context(), node.ApplySelfUpdateRequest{
			StageID:        stage.StageID,
			TargetVersion:  "1.1.0",
			ExpectedSHA256: stage.SHA256,
		})
		if !errors.Is(errApply, updater.ErrInsufficientDiskSpace) {
			t.Fatalf("Apply() error = %v, want ErrInsufficientDiskSpace", errApply)
		}
	})
}

func newArtifactTestManager(t *testing.T) *Manager {
	t.Helper()
	manager, errManager := NewManager(Config{
		Component:      "node",
		StageDir:       t.TempDir(),
		ExecutablePath: filepath.Join(t.TempDir(), "xylona-node"),
		ShutdownFunc:   func() {},
	})
	if errManager != nil {
		t.Fatalf("NewManager() error = %v", errManager)
	}
	writeManagerExecutable(t, manager)
	return manager
}

func stageManagerTestArtifact(t *testing.T, manager *Manager, targetVersion string, content []byte) node.StageSelfUpdateResult {
	t.Helper()
	result, errStage := stageManagerArtifact(manager, targetVersion, content)
	if errStage != nil {
		t.Fatalf("Stage() error = %v", errStage)
	}
	return result
}

func stageManagerArtifact(manager *Manager, targetVersion string, content []byte) (node.StageSelfUpdateResult, error) {
	sumBytes := sha256.Sum256(content)
	return manager.Stage(context.Background(), node.StageSelfUpdateRequest{
		Component:      "node",
		TargetVersion:  targetVersion,
		OS:             runtime.GOOS,
		Architecture:   runtime.GOARCH,
		ExpectedSize:   int64(len(content)),
		ExpectedSHA256: hex.EncodeToString(sumBytes[:]),
		Reader:         bytes.NewReader(content),
	})
}

func assertStageExists(t *testing.T, manager *Manager, stageID string) {
	t.Helper()
	for _, suffix := range []string{".bin", ".json"} {
		pathValue := filepath.Join(manager.stageDir, stageID+suffix)
		_, errStat := os.Stat(pathValue)
		if errStat != nil {
			t.Fatalf("staged artifact %q stat error = %v", pathValue, errStat)
		}
	}
}

func assertStageRemoved(t *testing.T, manager *Manager, stageID string) {
	t.Helper()
	for _, suffix := range []string{".bin", ".json"} {
		pathValue := filepath.Join(manager.stageDir, stageID+suffix)
		_, errStat := os.Stat(pathValue)
		if !errors.Is(errStat, os.ErrNotExist) {
			t.Fatalf("staged artifact %q remains, stat error = %v", pathValue, errStat)
		}
	}
}

func writeReconciliationPending(t *testing.T, manager *Manager, stageID string, expectedSHA256 string) string {
	t.Helper()
	readyPath := filepath.Join(manager.stageDir, stageID+"-88888888-8888-4888-8888-888888888888-ready")
	pendingPath := filepath.Join(manager.stageDir, stageID+"-pending.json")
	pending := pendingUpdate{
		StageID:         stageID,
		StagedPath:      manager.installCandidatePath(stageID),
		ExecutablePath:  manager.executablePath,
		BackupPath:      manager.executablePath + ".bak-" + stageID,
		ExpectedSHA256:  expectedSHA256,
		HelperReadyPath: readyPath,
	}
	errWrite := writeJSON(pendingPath, pending)
	if errWrite != nil {
		t.Fatalf("write pending handoff: %v", errWrite)
	}
	return pendingPath
}
