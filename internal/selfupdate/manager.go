// Package selfupdate stages and applies Xylona binary updates.
package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/updater"
	"github.com/ClintonCollins/Xylona/pkg/version"
)

const (
	// ProtocolVersion is the current self-update handoff protocol.
	ProtocolVersion    = 1
	shutdownDelay      = 750 * time.Millisecond
	helperReadyTimeout = 10 * time.Second
	helperReadyPoll    = 25 * time.Millisecond
	updateSpaceReserve = 16 * 1024 * 1024
	// RestartModeEnvironment selects whether the update helper or an external
	// service manager owns the restart after replacing the binary.
	RestartModeEnvironment = "XYLONA_UPDATE_RESTART_MODE"
	helperArg              = "--xylona-update-helper"
)

// RestartMode identifies who starts the replacement process.
type RestartMode string

const (
	// RestartModeSelf makes the update helper start the replacement directly.
	RestartModeSelf RestartMode = "self"
	// RestartModeServiceManager leaves restart ownership to an external manager.
	RestartModeServiceManager RestartMode = "service-manager"
)

var (
	// ErrInvalidStage is returned when a staged artifact is missing or invalid.
	ErrInvalidStage = errors.New("selfupdate: invalid staged artifact")
	// ErrApplyInProgress is returned when an update handoff already owns the executable.
	ErrApplyInProgress  = errors.New("selfupdate: an update handoff is already in progress")
	applyingExecutables sync.Map
)

// Config controls a Manager.
type Config struct {
	Component        string
	StageDir         string
	ExecutablePath   string
	RestartArgs      []string
	WorkingDirectory string
	RestartMode      RestartMode
	ShutdownFunc     func()
}

type helperProcess interface {
	Release() error
	Stop() error
}

type helperStarter func(helperPath string, pendingPath string) (helperProcess, error)
type helperReadyWaiter func(readyPath string, timeout time.Duration) error
type freeSpaceChecker func(path string, requiredBytes ...int64) error

type execHelperProcess struct {
	cmd *exec.Cmd
}

// Manager stages and hands off updates for one running binary.
type Manager struct {
	artifactMu       sync.Mutex
	component        string
	stageDir         string
	executablePath   string
	restartArgs      []string
	workingDirectory string
	restartMode      RestartMode
	shutdownFunc     func()
	shutdownDelay    time.Duration
	startHelper      helperStarter
	waitHelperReady  helperReadyWaiter
	helperReadyWait  time.Duration
	ensureFreeSpace  freeSpaceChecker
	now              func() time.Time
	inProcessRestart bool
	pendingRestart   string
	restartProcess   restartProcessFunc
}

// NewManager creates a Manager.
func NewManager(cfg Config) (*Manager, error) {
	component := strings.TrimSpace(cfg.Component)
	if component == "" {
		return nil, errors.New("selfupdate: component is required")
	}
	executablePath := strings.TrimSpace(cfg.ExecutablePath)
	if executablePath == "" {
		exe, errExe := os.Executable()
		if errExe != nil {
			return nil, fmt.Errorf("selfupdate: resolve executable: %w", errExe)
		}
		executablePath = exe
	}
	absExe, errAbsExe := filepath.Abs(executablePath)
	if errAbsExe != nil {
		return nil, fmt.Errorf("selfupdate: resolve executable path: %w", errAbsExe)
	}

	stageDir := strings.TrimSpace(cfg.StageDir)
	if stageDir == "" {
		stageDir = filepath.Join(filepath.Dir(absExe), ".xylona-updates")
	}
	absStageDir, errAbsStage := filepath.Abs(stageDir)
	if errAbsStage != nil {
		return nil, fmt.Errorf("selfupdate: resolve stage dir: %w", errAbsStage)
	}

	restartArgs := cfg.RestartArgs
	if restartArgs == nil {
		restartArgs = os.Args[1:]
	}
	restartArgs = append([]string(nil), restartArgs...)

	workingDirectory := strings.TrimSpace(cfg.WorkingDirectory)
	if workingDirectory == "" {
		currentDirectory, errWorkingDirectory := os.Getwd()
		if errWorkingDirectory != nil {
			return nil, fmt.Errorf("selfupdate: resolve working directory: %w", errWorkingDirectory)
		}
		workingDirectory = currentDirectory
	}
	absWorkingDirectory, errAbsWorkingDirectory := filepath.Abs(workingDirectory)
	if errAbsWorkingDirectory != nil {
		return nil, fmt.Errorf("selfupdate: resolve working directory path: %w", errAbsWorkingDirectory)
	}

	restartMode := RestartMode(strings.ToLower(strings.TrimSpace(string(cfg.RestartMode))))
	if restartMode == "" {
		restartMode = RestartModeSelf
	}
	switch restartMode {
	case RestartModeSelf, RestartModeServiceManager:
	default:
		return nil, fmt.Errorf("selfupdate: unsupported restart mode %q", restartMode)
	}

	shutdownFunc := cfg.ShutdownFunc
	if shutdownFunc == nil {
		return nil, errors.New("selfupdate: shutdown function is required")
	}

	manager := &Manager{
		component:        component,
		stageDir:         absStageDir,
		executablePath:   absExe,
		restartArgs:      restartArgs,
		workingDirectory: absWorkingDirectory,
		restartMode:      restartMode,
		shutdownFunc:     shutdownFunc,
		shutdownDelay:    shutdownDelay,
		startHelper:      startHelperProcess,
		waitHelperReady:  waitForHelperReady,
		helperReadyWait:  helperReadyTimeout,
		ensureFreeSpace:  updater.EnsureFreeSpace,
		now:              time.Now,
		inProcessRestart: platformUsesInProcessRestart(),
		restartProcess:   restartCurrentProcess,
	}
	_, errReconcile := manager.reconcileArtifacts(maxRetainedStagedUpdates)
	if errReconcile != nil {
		log.Warn().Err(errReconcile).Str("stage_dir", manager.stageDir).Msg("selfupdate: startup artifact reconciliation was incomplete")
	}
	return manager, nil
}

// Capabilities reports the runtime's self-update support.
func (m *Manager) Capabilities() node.UpdateCapabilities {
	caps := node.UpdateCapabilities{
		Supported:       false,
		Component:       m.component,
		CurrentVersion:  version.SoftwareVersion,
		OS:              runtime.GOOS,
		Architecture:    runtime.GOARCH,
		ProtocolVersion: ProtocolVersion,
		// Generic supervisor behavior cannot be verified from inside the process.
		ServiceManagerSupported: false,
		InstallPath:             m.executablePath,
	}
	caps.InstallPathWritable = m.installPathWritable()
	if !caps.InstallPathWritable {
		caps.Reason = "install path is not writable"
		return caps
	}
	caps.Supported = true
	return caps
}

// Stage writes and verifies an update artifact.
func (m *Manager) Stage(ctx context.Context, req node.StageSelfUpdateRequest) (node.StageSelfUpdateResult, error) {
	if m == nil {
		return node.StageSelfUpdateResult{}, errors.New("selfupdate: manager is nil")
	}
	m.artifactMu.Lock()
	defer m.artifactMu.Unlock()

	if strings.TrimSpace(req.Component) != "" && req.Component != m.component {
		return node.StageSelfUpdateResult{}, fmt.Errorf("selfupdate: component %q cannot update %q", req.Component, m.component)
	}
	if req.OS != "" && req.OS != runtime.GOOS {
		return node.StageSelfUpdateResult{}, fmt.Errorf("selfupdate: artifact OS %q does not match %q", req.OS, runtime.GOOS)
	}
	if req.Architecture != "" && req.Architecture != runtime.GOARCH {
		return node.StageSelfUpdateResult{}, fmt.Errorf("selfupdate: artifact architecture %q does not match %q", req.Architecture, runtime.GOARCH)
	}
	if strings.TrimSpace(req.ExpectedSHA256) == "" {
		return node.StageSelfUpdateResult{}, errors.New("selfupdate: expected SHA-256 is required")
	}
	if req.Reader == nil {
		return node.StageSelfUpdateResult{}, errors.New("selfupdate: artifact reader is required")
	}
	if req.ExpectedSize <= 0 {
		return node.StageSelfUpdateResult{}, errors.New("selfupdate: expected artifact size must be positive")
	}

	errMkdir := os.MkdirAll(m.stageDir, 0o750)
	if errMkdir != nil {
		return node.StageSelfUpdateResult{}, fmt.Errorf("selfupdate: create stage dir: %w", errMkdir)
	}
	pending, errReconcile := m.reconcileArtifacts(maxRetainedStagedUpdates - 1)
	if errReconcile != nil {
		return node.StageSelfUpdateResult{}, fmt.Errorf("selfupdate: reconcile artifacts before staging: %w", errReconcile)
	}
	if pending {
		return node.StageSelfUpdateResult{}, ErrApplyInProgress
	}
	errStageSpace := m.checkFreeSpace(m.stageDir, req.ExpectedSize, updateSpaceReserve)
	if errStageSpace != nil {
		return node.StageSelfUpdateResult{}, fmt.Errorf("selfupdate: stage capacity preflight: %w", errStageSpace)
	}

	stageID := uuid.NewString()
	tmpPath := filepath.Join(m.stageDir, stageID+".tmp")
	stagePath := filepath.Join(m.stageDir, stageID+".bin")
	file, errCreate := os.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errCreate != nil {
		return node.StageSelfUpdateResult{}, fmt.Errorf("selfupdate: create stage file: %w", errCreate)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(file, hasher)
	written, errCopy := copyContext(ctx, writer, req.Reader)
	var errSync error
	if errCopy == nil {
		errSync = file.Sync()
	}
	errClose := file.Close()
	if errCopy != nil {
		errRemove := removeFileIfExists(tmpPath)
		errResult := fmt.Errorf("selfupdate: write stage file: %w", errCopy)
		if errClose != nil {
			errResult = errors.Join(errResult, fmt.Errorf("selfupdate: close partial stage file: %w", errClose))
		}
		return node.StageSelfUpdateResult{}, errors.Join(errResult, errRemove)
	}
	var errPersist error
	if errSync != nil {
		errPersist = errors.Join(errPersist, fmt.Errorf("selfupdate: sync stage file: %w", errSync))
	}
	if errClose != nil {
		errPersist = errors.Join(errPersist, fmt.Errorf("selfupdate: close stage file: %w", errClose))
	}
	if errPersist != nil {
		errRemove := removeFileIfExists(tmpPath)
		return node.StageSelfUpdateResult{}, errors.Join(errPersist, errRemove)
	}
	if req.ExpectedSize > 0 && written != req.ExpectedSize {
		errRemove := removeFileIfExists(tmpPath)
		return node.StageSelfUpdateResult{}, errors.Join(fmt.Errorf("selfupdate: staged size %d does not match %d", written, req.ExpectedSize), errRemove)
	}
	sum := hex.EncodeToString(hasher.Sum(nil))
	expected := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(req.ExpectedSHA256), "sha256:"))
	if sum != expected {
		errRemove := removeFileIfExists(tmpPath)
		return node.StageSelfUpdateResult{}, errors.Join(fmt.Errorf("selfupdate: staged SHA-256 %s does not match %s", sum, expected), errRemove)
	}
	errRename := os.Rename(tmpPath, stagePath)
	if errRename != nil {
		errRemove := removeFileIfExists(tmpPath)
		return node.StageSelfUpdateResult{}, errors.Join(fmt.Errorf("selfupdate: finalize stage file: %w", errRename), errRemove)
	}
	errInstallSpace := m.ensureInstallCapacity(written)
	if errInstallSpace != nil {
		errRemove := removeFileIfExists(stagePath)
		return node.StageSelfUpdateResult{}, errors.Join(fmt.Errorf("selfupdate: install capacity preflight: %w", errInstallSpace), errRemove)
	}
	meta := stagedMetadata{
		StageID:        stageID,
		Component:      m.component,
		TargetVersion:  req.TargetVersion,
		StagedPath:     stagePath,
		ExecutablePath: m.executablePath,
		ExpectedSHA256: sum,
		CreatedAt:      m.currentTime().UTC(),
	}
	errMeta := writeJSON(filepath.Join(m.stageDir, stageID+".json"), meta)
	if errMeta != nil {
		errRemoveStage := removeFileIfExists(stagePath)
		errRemoveMetadata := removeFileIfExists(filepath.Join(m.stageDir, stageID+".json"))
		return node.StageSelfUpdateResult{}, errors.Join(errMeta, errRemoveStage, errRemoveMetadata)
	}
	return node.StageSelfUpdateResult{
		StageID:      stageID,
		BytesWritten: written,
		SHA256:       sum,
	}, nil
}

// Apply starts the restart handoff for a staged artifact.
func (m *Manager) Apply(_ context.Context, req node.ApplySelfUpdateRequest) (node.ApplySelfUpdateResult, error) {
	if m == nil {
		return node.ApplySelfUpdateResult{}, errors.New("selfupdate: manager is nil")
	}
	m.artifactMu.Lock()
	defer m.artifactMu.Unlock()

	stageID := strings.TrimSpace(req.StageID)
	if !validStageID(stageID) {
		return node.ApplySelfUpdateResult{}, fmt.Errorf("%w: stage ID is required", ErrInvalidStage)
	}
	handoffPending, errReconcile := m.reconcileArtifactsPreserving(maxRetainedStagedUpdates, stageID)
	if errReconcile != nil {
		return node.ApplySelfUpdateResult{}, fmt.Errorf("selfupdate: reconcile artifacts before apply: %w", errReconcile)
	}
	if handoffPending {
		return node.ApplySelfUpdateResult{}, ErrApplyInProgress
	}
	meta, errMeta := m.readMetadata(stageID)
	if errMeta != nil {
		return node.ApplySelfUpdateResult{}, errMeta
	}
	if req.TargetVersion != "" && meta.TargetVersion != req.TargetVersion {
		return node.ApplySelfUpdateResult{}, fmt.Errorf("%w: target version mismatch", ErrInvalidStage)
	}
	if req.ExpectedSHA256 != "" && meta.ExpectedSHA256 != strings.TrimPrefix(strings.ToLower(req.ExpectedSHA256), "sha256:") {
		return node.ApplySelfUpdateResult{}, fmt.Errorf("%w: checksum mismatch", ErrInvalidStage)
	}
	errVerify := verifyFileSHA256(meta.StagedPath, meta.ExpectedSHA256)
	if errVerify != nil {
		return node.ApplySelfUpdateResult{}, errVerify
	}
	stageInfo, errStageInfo := os.Stat(meta.StagedPath)
	if errStageInfo != nil {
		return node.ApplySelfUpdateResult{}, fmt.Errorf("selfupdate: inspect staged artifact before apply: %w", errStageInfo)
	}
	errInstallSpace := m.ensureInstallCapacity(stageInfo.Size())
	if errInstallSpace != nil {
		return node.ApplySelfUpdateResult{}, fmt.Errorf("selfupdate: apply capacity preflight: %w", errInstallSpace)
	}

	_, applyLoaded := applyingExecutables.LoadOrStore(m.executablePath, struct{}{})
	if applyLoaded {
		return node.ApplySelfUpdateResult{}, ErrApplyInProgress
	}
	handoffStarted := false
	defer func() {
		if !handoffStarted {
			applyingExecutables.Delete(m.executablePath)
		}
	}()

	installPath, errInstallPath := m.prepareInstallCandidate(meta)
	if errInstallPath != nil {
		return node.ApplySelfUpdateResult{}, errInstallPath
	}

	pendingPath := filepath.Join(m.stageDir, meta.StageID+"-pending.json")
	helperReadyPath := filepath.Join(m.stageDir, meta.StageID+"-"+uuid.NewString()+"-ready")
	pending := pendingUpdate{
		StageID:          meta.StageID,
		Component:        meta.Component,
		TargetVersion:    meta.TargetVersion,
		StagedPath:       installPath,
		ExecutablePath:   meta.ExecutablePath,
		BackupPath:       meta.ExecutablePath + ".bak-" + meta.StageID,
		ExpectedSHA256:   meta.ExpectedSHA256,
		ParentPID:        os.Getpid(),
		RestartArgs:      append([]string(nil), m.restartArgs...),
		WorkingDirectory: m.workingDirectory,
		RestartMode:      m.restartMode,
		HelperReadyPath:  helperReadyPath,
		CreatedAt:        m.currentTime().UTC(),
	}
	errPending := writeJSON(pendingPath, pending)
	if errPending != nil {
		return node.ApplySelfUpdateResult{}, removeHandoffArtifactsOnError(
			[]string{installPath, pendingPath, helperReadyPath, helperReadyTempPath(helperReadyPath)},
			errPending,
		)
	}
	if m.inProcessRestart && m.restartMode == RestartModeSelf {
		m.pendingRestart = pendingPath
		handoffStarted = true
		go func() {
			time.Sleep(m.shutdownDelay)
			m.shutdownFunc()
		}()
		return node.ApplySelfUpdateResult{
			Accepted: true,
			Message:  "update staged; process will replace itself after graceful shutdown",
		}, nil
	}

	helperPath, errHelper := m.prepareHelperExecutable(stageID)
	if errHelper != nil {
		return node.ApplySelfUpdateResult{}, removeHandoffArtifactsOnError(
			[]string{installPath, pendingPath, helperReadyPath, helperReadyTempPath(helperReadyPath)},
			errHelper,
		)
	}
	startHelper := m.startHelper
	if startHelper == nil {
		startHelper = startHelperProcess
	}
	runningHelper, errStart := startHelper(helperPath, pendingPath)
	if errStart != nil || runningHelper == nil {
		if errStart == nil {
			errStart = errors.New("selfupdate: helper process was not returned after start")
		}
		return node.ApplySelfUpdateResult{}, removeHandoffArtifactsOnError(
			[]string{installPath, helperPath, pendingPath, helperReadyPath, helperReadyTempPath(helperReadyPath)},
			errStart,
		)
	}
	waitHelperReady := m.waitHelperReady
	if waitHelperReady == nil {
		waitHelperReady = waitForHelperReady
	}
	errReady := waitHelperReady(helperReadyPath, m.helperReadyWait)
	if errReady != nil {
		errStop := runningHelper.Stop()
		return node.ApplySelfUpdateResult{}, removeHandoffArtifactsOnError(
			[]string{installPath, helperPath, pendingPath, helperReadyPath, helperReadyTempPath(helperReadyPath)},
			errors.Join(errReady, errStop),
		)
	}
	errRelease := runningHelper.Release()
	handoffStarted = true

	go func() {
		time.Sleep(m.shutdownDelay)
		m.shutdownFunc()
	}()

	message := "update helper started; process will restart itself"
	if m.restartMode == RestartModeServiceManager {
		message = "update helper started; service manager should restart this process"
	}
	if errRelease != nil {
		message += "; helper process handle could not be released cleanly: " + errRelease.Error()
	}
	return node.ApplySelfUpdateResult{
		Accepted: true,
		Message:  message,
	}, nil
}

// CompleteSelfUpdate replaces and restarts the current process after its
// service cleanup has completed. It is a no-op when Apply did not select the
// in-process restart path.
func (m *Manager) CompleteSelfUpdate() error {
	if m == nil {
		return nil
	}
	m.artifactMu.Lock()
	defer m.artifactMu.Unlock()

	pendingPath := m.pendingRestart
	if pendingPath == "" {
		return nil
	}
	pending, errPending := readPendingUpdate(pendingPath)
	if errPending != nil {
		return fmt.Errorf("selfupdate: read pending in-process restart: %w", errPending)
	}
	stageID, validPendingPath := pendingStageID(filepath.Base(pendingPath))
	if !validPendingPath {
		return errors.New("selfupdate: pending in-process restart path is invalid")
	}
	errValidate := m.validatePendingUpdate(pending, stageID)
	if errValidate != nil {
		return fmt.Errorf("selfupdate: validate pending in-process restart: %w", errValidate)
	}
	restartProcess := m.restartProcess
	if restartProcess == nil {
		restartProcess = restartCurrentProcess
	}
	errRestart := restartProcess(pendingPath, pending)
	if errRestart != nil {
		return fmt.Errorf("selfupdate: complete in-process restart: %w", errRestart)
	}
	return nil
}

func startHelperProcess(helperPath string, pendingPath string) (helperProcess, error) {
	cmd := exec.CommandContext(context.Background(), helperPath, helperArg, pendingPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	errStart := cmd.Start()
	if errStart != nil {
		return nil, fmt.Errorf("selfupdate: start helper: %w", errStart)
	}
	return &execHelperProcess{cmd: cmd}, nil
}

func (p *execHelperProcess) Release() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return errors.New("selfupdate: helper process is unavailable")
	}
	errRelease := p.cmd.Process.Release()
	if errRelease != nil {
		return fmt.Errorf("selfupdate: release helper process: %w", errRelease)
	}
	return nil
}

func (p *execHelperProcess) Stop() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return errors.New("selfupdate: helper process is unavailable")
	}
	errKill := p.cmd.Process.Kill()
	if errKill != nil && !errors.Is(errKill, os.ErrProcessDone) {
		return fmt.Errorf("selfupdate: stop unready helper: %w", errKill)
	}
	errWait := p.cmd.Wait()
	var exitErr *exec.ExitError
	if errWait != nil && !errors.As(errWait, &exitErr) && !errors.Is(errWait, os.ErrProcessDone) {
		return fmt.Errorf("selfupdate: wait for unready helper: %w", errWait)
	}
	return nil
}

func waitForHelperReady(readyPath string, timeout time.Duration) error {
	if strings.TrimSpace(readyPath) == "" {
		return errors.New("selfupdate: helper ready path is required")
	}
	if timeout <= 0 {
		return errors.New("selfupdate: helper ready timeout must be positive")
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(helperReadyPoll)
	defer ticker.Stop()
	for {
		ready, errReady := pathExists(readyPath)
		if errReady != nil {
			return fmt.Errorf("selfupdate: inspect helper ready marker: %w", errReady)
		}
		if ready {
			return nil
		}

		select {
		case <-timer.C:
			return fmt.Errorf("selfupdate: helper did not become ready within %s", timeout)
		case <-ticker.C:
		}
	}
}

func (m *Manager) prepareInstallCandidate(meta stagedMetadata) (string, error) {
	installPath := m.installCandidatePath(meta.StageID)
	errCopy := copyFile(meta.StagedPath, installPath, 0o700)
	if errCopy != nil {
		return "", fmt.Errorf("selfupdate: prepare install candidate: %w", errCopy)
	}
	errVerify := verifyFileSHA256(installPath, meta.ExpectedSHA256)
	if errVerify != nil {
		errRemove := os.Remove(installPath)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			return "", errors.Join(errVerify, fmt.Errorf("remove invalid install candidate: %w", errRemove))
		}
		return "", errVerify
	}
	return installPath, nil
}

func (m *Manager) installCandidatePath(stageID string) string {
	name := ".xylona-update-" + stageID
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(m.executablePath), name)
}

func removeHandoffArtifactsOnError(paths []string, cause error) error {
	result := cause
	for _, pathValue := range paths {
		errRemove := os.Remove(pathValue)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			result = errors.Join(result, fmt.Errorf("remove handoff artifact %q: %w", pathValue, errRemove))
		}
	}
	return result
}

func (m *Manager) prepareHelperExecutable(stageID string) (string, error) {
	exe, errExe := os.Executable()
	if errExe != nil {
		return "", fmt.Errorf("selfupdate: resolve helper executable: %w", errExe)
	}
	name := stageID + "-" + uuid.NewString() + "-helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	helperPath := filepath.Join(m.stageDir, name)
	errCopy := copyFile(exe, helperPath, 0o700)
	if errCopy != nil {
		return "", fmt.Errorf("selfupdate: prepare helper executable: %w", errCopy)
	}
	return helperPath, nil
}

type stagedMetadata struct {
	StageID        string    `json:"stage_id"`
	Component      string    `json:"component"`
	TargetVersion  string    `json:"target_version"`
	StagedPath     string    `json:"staged_path"`
	ExecutablePath string    `json:"executable_path"`
	ExpectedSHA256 string    `json:"expected_sha256"`
	CreatedAt      time.Time `json:"created_at"`
}

type pendingUpdate struct {
	StageID          string      `json:"stage_id"`
	Component        string      `json:"component"`
	TargetVersion    string      `json:"target_version"`
	StagedPath       string      `json:"staged_path"`
	ExecutablePath   string      `json:"executable_path"`
	BackupPath       string      `json:"backup_path"`
	ExpectedSHA256   string      `json:"expected_sha256"`
	ParentPID        int         `json:"parent_pid"`
	RestartArgs      []string    `json:"restart_args"`
	WorkingDirectory string      `json:"working_directory"`
	RestartMode      RestartMode `json:"restart_mode"`
	HelperReadyPath  string      `json:"helper_ready_path"`
	CreatedAt        time.Time   `json:"created_at"`
}

func (m *Manager) readMetadata(stageID string) (stagedMetadata, error) {
	var meta stagedMetadata
	if !validStageID(stageID) {
		return meta, fmt.Errorf("%w: invalid stage ID", ErrInvalidStage)
	}
	metaPath := filepath.Join(m.stageDir, stageID+".json")
	file, errOpen := os.Open(metaPath)
	if errOpen != nil {
		return meta, errors.Join(ErrInvalidStage, fmt.Errorf("open metadata: %w", errOpen))
	}
	errDecode := json.NewDecoder(file).Decode(&meta)
	errClose := file.Close()
	if errDecode != nil {
		errResult := errors.Join(ErrInvalidStage, fmt.Errorf("decode metadata: %w", errDecode))
		if errClose != nil {
			errResult = errors.Join(errResult, fmt.Errorf("close invalid metadata: %w", errClose))
		}
		return meta, errResult
	}
	if errClose != nil {
		return meta, fmt.Errorf("close metadata: %w", errClose)
	}
	if meta.StageID != stageID {
		return meta, fmt.Errorf("%w: metadata stage ID mismatch", ErrInvalidStage)
	}
	if filepath.Clean(meta.StagedPath) != filepath.Join(m.stageDir, stageID+".bin") {
		return meta, fmt.Errorf("%w: metadata staged path mismatch", ErrInvalidStage)
	}
	if filepath.Clean(meta.ExecutablePath) != m.executablePath {
		return meta, fmt.Errorf("%w: metadata executable path mismatch", ErrInvalidStage)
	}
	return meta, nil
}

func (m *Manager) checkFreeSpace(pathValue string, requiredBytes ...int64) error {
	checker := m.ensureFreeSpace
	if checker == nil {
		checker = updater.EnsureFreeSpace
	}
	return checker(pathValue, requiredBytes...)
}

func (m *Manager) ensureInstallCapacity(candidateSize int64) error {
	if candidateSize <= 0 {
		return errors.New("selfupdate: install candidate size must be positive")
	}
	currentInfo, errCurrent := os.Stat(m.executablePath)
	if errCurrent != nil {
		return fmt.Errorf("selfupdate: inspect current executable: %w", errCurrent)
	}
	if !currentInfo.Mode().IsRegular() {
		return errors.New("selfupdate: current executable is not a regular file")
	}
	helperSource, errHelperSource := os.Executable()
	if errHelperSource != nil {
		return fmt.Errorf("selfupdate: resolve helper source executable: %w", errHelperSource)
	}
	helperInfo, errHelperInfo := os.Stat(helperSource)
	if errHelperInfo != nil {
		return fmt.Errorf("selfupdate: inspect helper source executable: %w", errHelperInfo)
	}
	if !helperInfo.Mode().IsRegular() {
		return errors.New("selfupdate: helper source executable is not a regular file")
	}
	installDir := filepath.Dir(m.executablePath)
	sameVolume, errVolume := updater.PathsShareVolume(m.stageDir, installDir)
	if errVolume != nil {
		return fmt.Errorf("selfupdate: compare staging and install volumes: %w", errVolume)
	}
	if sameVolume {
		return m.checkFreeSpace(installDir, candidateSize, helperInfo.Size(), updateSpaceReserve)
	}
	errStageSpace := m.checkFreeSpace(m.stageDir, helperInfo.Size(), updateSpaceReserve)
	if errStageSpace != nil {
		return errStageSpace
	}
	return m.checkFreeSpace(installDir, candidateSize, updateSpaceReserve)
}

func (m *Manager) installPathWritable() bool {
	dir := filepath.Dir(m.executablePath)
	errMkdir := os.MkdirAll(m.stageDir, 0o750)
	if errMkdir != nil {
		return false
	}
	probe, errCreate := os.CreateTemp(dir, ".xylona-write-probe-*")
	if errCreate != nil {
		return false
	}
	name := probe.Name()
	errClose := probe.Close()
	errRemove := os.Remove(name)
	return errClose == nil && errRemove == nil
}

func copyContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 64*1024)
	var written int64
	for {
		errCtx := ctx.Err()
		if errCtx != nil {
			return written, fmt.Errorf("selfupdate: copy canceled: %w", errCtx)
		}
		n, errRead := src.Read(buf)
		if n > 0 {
			w, errWrite := dst.Write(buf[:n])
			written += int64(w)
			if errWrite != nil {
				return written, fmt.Errorf("selfupdate: write copy chunk: %w", errWrite)
			}
			if w != n {
				return written, io.ErrShortWrite
			}
		}
		if errors.Is(errRead, io.EOF) {
			return written, nil
		}
		if errRead != nil {
			return written, fmt.Errorf("selfupdate: read copy chunk: %w", errRead)
		}
	}
}

func copyFile(srcPath string, dstPath string, mode os.FileMode) error {
	src, errOpen := os.Open(srcPath)
	if errOpen != nil {
		return fmt.Errorf("open source file: %w", errOpen)
	}

	// #nosec G302 -- helper copies must be executable so they can perform the handoff.
	dst, errCreate := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if errCreate != nil {
		errCloseSource := src.Close()
		if errCloseSource != nil {
			return errors.Join(
				fmt.Errorf("create destination file: %w", errCreate),
				fmt.Errorf("close source file after destination create failure: %w", errCloseSource),
			)
		}
		return fmt.Errorf("create destination file: %w", errCreate)
	}
	_, errCopy := io.Copy(dst, src)
	var errSync error
	if errCopy == nil {
		errSync = dst.Sync()
	}
	errCloseDestination := dst.Close()
	errCloseSource := src.Close()
	var resultErr error
	if errCopy != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("copy file: %w", errCopy))
	}
	if errSync != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("sync destination file: %w", errSync))
	}
	if errCloseDestination != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("close destination file: %w", errCloseDestination))
	}
	if errCloseSource != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("close source file: %w", errCloseSource))
	}
	if resultErr == nil {
		return nil
	}
	errRemove := removeFileIfExists(dstPath)
	return errors.Join(resultErr, errRemove)
}

func writeJSON(pathValue string, v any) error {
	file, errCreate := os.OpenFile(pathValue, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if errCreate != nil {
		return fmt.Errorf("selfupdate: write metadata: %w", errCreate)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	errEncode := encoder.Encode(v)
	var errSync error
	if errEncode == nil {
		errSync = file.Sync()
	}
	errClose := file.Close()
	if errEncode != nil {
		errResult := fmt.Errorf("selfupdate: encode metadata: %w", errEncode)
		if errClose != nil {
			errResult = errors.Join(errResult, fmt.Errorf("selfupdate: close metadata after encode failure: %w", errClose))
		}
		return errResult
	}
	var resultErr error
	if errSync != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("selfupdate: sync metadata: %w", errSync))
	}
	if errClose != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("selfupdate: close metadata: %w", errClose))
	}
	return resultErr
}

func verifyFileSHA256(pathValue string, expected string) error {
	file, errOpen := os.Open(pathValue)
	if errOpen != nil {
		return errors.Join(ErrInvalidStage, fmt.Errorf("open staged file: %w", errOpen))
	}
	sum, errHash := hashFile(file)
	errClose := file.Close()
	if errHash != nil {
		if errClose != nil {
			return errors.Join(errHash, fmt.Errorf("selfupdate: close staged file after checksum failure: %w", errClose))
		}
		return errHash
	}
	if errClose != nil {
		return fmt.Errorf("selfupdate: close staged file after checksum: %w", errClose)
	}
	expected = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(expected)), "sha256:")
	if sum != expected {
		return fmt.Errorf("%w: staged checksum %s does not match %s", ErrInvalidStage, sum, expected)
	}
	return nil
}

func hashFile(r io.Reader) (string, error) {
	hasher := sha256.New()
	_, errCopy := io.Copy(hasher, r)
	if errCopy != nil {
		return "", fmt.Errorf("selfupdate: hash file: %w", errCopy)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
