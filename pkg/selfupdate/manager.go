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
	"time"

	"github.com/google/uuid"

	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/version"
)

const (
	// ProtocolVersion is the current self-update handoff protocol.
	ProtocolVersion = 1
	helperArg       = "--xylona-update-helper"
)

var (
	// ErrApplyUnsupported is returned when the runtime is not configured for self-update.
	ErrApplyUnsupported = errors.New("selfupdate: apply unsupported")
	// ErrInvalidStage is returned when a staged artifact is missing or invalid.
	ErrInvalidStage = errors.New("selfupdate: invalid staged artifact")
)

// Config controls a Manager.
type Config struct {
	Component      string
	StageDir       string
	ExecutablePath string
	AllowApply     bool
	ExitFunc       func(code int)
}

type helperStarter func(helperPath string, pendingPath string) error

// Manager stages and hands off updates for one running binary.
type Manager struct {
	component      string
	stageDir       string
	executablePath string
	allowApply     bool
	exitFunc       func(code int)
	startHelper    helperStarter
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

	exitFunc := cfg.ExitFunc
	if exitFunc == nil {
		exitFunc = os.Exit
	}

	return &Manager{
		component:      component,
		stageDir:       absStageDir,
		executablePath: absExe,
		allowApply:     cfg.AllowApply,
		exitFunc:       exitFunc,
		startHelper:    startHelperProcess,
	}, nil
}

// NewDefaultManager creates a Manager using environment-controlled apply support.
func NewDefaultManager(component string, stageDir string) (*Manager, error) {
	return NewManager(Config{
		Component:  component,
		StageDir:   stageDir,
		AllowApply: envBool("XYLONA_SELF_UPDATE_ALLOW_APPLY"),
	})
}

// Capabilities reports the runtime's self-update support.
func (m *Manager) Capabilities() node.UpdateCapabilities {
	caps := node.UpdateCapabilities{
		Supported:               false,
		Component:               m.component,
		CurrentVersion:          version.SoftwareVersion,
		OS:                      runtime.GOOS,
		Architecture:            runtime.GOARCH,
		ProtocolVersion:         ProtocolVersion,
		ServiceManagerSupported: m.allowApply,
		InstallPath:             m.executablePath,
	}
	caps.InstallPathWritable = m.installPathWritable()
	if !m.allowApply {
		caps.Reason = "self-update apply is disabled; set XYLONA_SELF_UPDATE_ALLOW_APPLY=1 for service-managed installs"
		return caps
	}
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

	errMkdir := os.MkdirAll(m.stageDir, 0o750)
	if errMkdir != nil {
		return node.StageSelfUpdateResult{}, fmt.Errorf("selfupdate: create stage dir: %w", errMkdir)
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
	errClose := file.Close()
	if errCopy != nil {
		_ = os.Remove(tmpPath)
		return node.StageSelfUpdateResult{}, fmt.Errorf("selfupdate: write stage file: %w", errCopy)
	}
	if errClose != nil {
		_ = os.Remove(tmpPath)
		return node.StageSelfUpdateResult{}, fmt.Errorf("selfupdate: close stage file: %w", errClose)
	}
	if req.ExpectedSize > 0 && written != req.ExpectedSize {
		_ = os.Remove(tmpPath)
		return node.StageSelfUpdateResult{}, fmt.Errorf("selfupdate: staged size %d does not match %d", written, req.ExpectedSize)
	}
	sum := hex.EncodeToString(hasher.Sum(nil))
	expected := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(req.ExpectedSHA256), "sha256:"))
	if sum != expected {
		_ = os.Remove(tmpPath)
		return node.StageSelfUpdateResult{}, fmt.Errorf("selfupdate: staged SHA-256 %s does not match %s", sum, expected)
	}
	errRename := os.Rename(tmpPath, stagePath)
	if errRename != nil {
		_ = os.Remove(tmpPath)
		return node.StageSelfUpdateResult{}, fmt.Errorf("selfupdate: finalize stage file: %w", errRename)
	}
	meta := stagedMetadata{
		StageID:        stageID,
		Component:      m.component,
		TargetVersion:  req.TargetVersion,
		StagedPath:     stagePath,
		ExecutablePath: m.executablePath,
		ExpectedSHA256: sum,
		CreatedAt:      time.Now().UTC(),
	}
	errMeta := writeJSON(filepath.Join(m.stageDir, stageID+".json"), meta)
	if errMeta != nil {
		return node.StageSelfUpdateResult{}, errMeta
	}
	return node.StageSelfUpdateResult{
		StageID:      stageID,
		BytesWritten: written,
		SHA256:       sum,
	}, nil
}

// Apply starts the helper handoff for a staged artifact.
func (m *Manager) Apply(_ context.Context, req node.ApplySelfUpdateRequest) (node.ApplySelfUpdateResult, error) {
	if m == nil {
		return node.ApplySelfUpdateResult{}, errors.New("selfupdate: manager is nil")
	}
	if !m.allowApply {
		return node.ApplySelfUpdateResult{}, ErrApplyUnsupported
	}
	stageID := strings.TrimSpace(req.StageID)
	if stageID == "" {
		return node.ApplySelfUpdateResult{}, fmt.Errorf("%w: stage ID is required", ErrInvalidStage)
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

	installPath, errInstallPath := m.prepareInstallCandidate(meta)
	if errInstallPath != nil {
		return node.ApplySelfUpdateResult{}, errInstallPath
	}

	pendingPath := filepath.Join(m.stageDir, "pending-update.json")
	pending := pendingUpdate{
		StageID:        meta.StageID,
		Component:      meta.Component,
		TargetVersion:  meta.TargetVersion,
		StagedPath:     installPath,
		ExecutablePath: meta.ExecutablePath,
		BackupPath:     meta.ExecutablePath + ".bak-" + time.Now().UTC().Format("20060102150405"),
		ExpectedSHA256: meta.ExpectedSHA256,
		CreatedAt:      time.Now().UTC(),
	}
	errPending := writeJSON(pendingPath, pending)
	if errPending != nil {
		return node.ApplySelfUpdateResult{}, removeInstallCandidateOnError(installPath, errPending)
	}

	helperPath, errHelper := m.prepareHelperExecutable(stageID)
	if errHelper != nil {
		return node.ApplySelfUpdateResult{}, removeInstallCandidateOnError(installPath, errHelper)
	}
	startHelper := m.startHelper
	if startHelper == nil {
		startHelper = startHelperProcess
	}
	errStart := startHelper(helperPath, pendingPath)
	if errStart != nil {
		return node.ApplySelfUpdateResult{}, removeInstallCandidateOnError(installPath, errStart)
	}

	go func() {
		time.Sleep(750 * time.Millisecond)
		m.exitFunc(0)
	}()

	return node.ApplySelfUpdateResult{
		Accepted: true,
		Message:  "update helper started; service manager should restart this process",
	}, nil
}

func startHelperProcess(helperPath string, pendingPath string) error {
	cmd := exec.CommandContext(context.Background(), helperPath, helperArg, pendingPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	errStart := cmd.Start()
	if errStart != nil {
		return fmt.Errorf("selfupdate: start helper: %w", errStart)
	}
	errRelease := cmd.Process.Release()
	if errRelease != nil {
		return fmt.Errorf("selfupdate: release helper process: %w", errRelease)
	}
	return nil
}

func (m *Manager) prepareInstallCandidate(meta stagedMetadata) (string, error) {
	name := ".xylona-update-" + meta.StageID
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	installPath := filepath.Join(filepath.Dir(meta.ExecutablePath), name)
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

func removeInstallCandidateOnError(installPath string, err error) error {
	errRemove := os.Remove(installPath)
	if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
		return errors.Join(err, fmt.Errorf("remove install candidate: %w", errRemove))
	}
	return err
}

func (m *Manager) prepareHelperExecutable(stageID string) (string, error) {
	exe, errExe := os.Executable()
	if errExe != nil {
		return "", fmt.Errorf("selfupdate: resolve helper executable: %w", errExe)
	}
	name := stageID + "-helper"
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
	StageID        string    `json:"stage_id"`
	Component      string    `json:"component"`
	TargetVersion  string    `json:"target_version"`
	StagedPath     string    `json:"staged_path"`
	ExecutablePath string    `json:"executable_path"`
	BackupPath     string    `json:"backup_path"`
	ExpectedSHA256 string    `json:"expected_sha256"`
	CreatedAt      time.Time `json:"created_at"`
}

func (m *Manager) readMetadata(stageID string) (stagedMetadata, error) {
	var meta stagedMetadata
	metaPath := filepath.Join(m.stageDir, stageID+".json")
	file, errOpen := os.Open(metaPath)
	if errOpen != nil {
		return meta, errors.Join(ErrInvalidStage, fmt.Errorf("open metadata: %w", errOpen))
	}
	defer func() {
		_ = file.Close()
	}()
	errDecode := json.NewDecoder(file).Decode(&meta)
	if errDecode != nil {
		return meta, errors.Join(ErrInvalidStage, fmt.Errorf("decode metadata: %w", errDecode))
	}
	if meta.StageID != stageID {
		return meta, fmt.Errorf("%w: metadata stage ID mismatch", ErrInvalidStage)
	}
	return meta, nil
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
	defer func() {
		_ = src.Close()
	}()

	// #nosec G302 -- helper copies must be executable so they can perform the handoff.
	dst, errCreate := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if errCreate != nil {
		return fmt.Errorf("create destination file: %w", errCreate)
	}
	_, errCopy := io.Copy(dst, src)
	errClose := dst.Close()
	if errCopy != nil {
		_ = os.Remove(dstPath)
		return fmt.Errorf("copy file: %w", errCopy)
	}
	if errClose != nil {
		_ = os.Remove(dstPath)
		return fmt.Errorf("close destination file: %w", errClose)
	}
	return nil
}

func writeJSON(pathValue string, v any) error {
	file, errCreate := os.OpenFile(pathValue, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if errCreate != nil {
		return fmt.Errorf("selfupdate: write metadata: %w", errCreate)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	errEncode := encoder.Encode(v)
	errClose := file.Close()
	if errEncode != nil {
		return fmt.Errorf("selfupdate: encode metadata: %w", errEncode)
	}
	if errClose != nil {
		return fmt.Errorf("selfupdate: close metadata: %w", errClose)
	}
	return nil
}

func verifyFileSHA256(pathValue string, expected string) error {
	file, errOpen := os.Open(pathValue)
	if errOpen != nil {
		return errors.Join(ErrInvalidStage, fmt.Errorf("open staged file: %w", errOpen))
	}
	defer func() {
		_ = file.Close()
	}()
	sum, _, errHash := hashFile(file)
	if errHash != nil {
		return errHash
	}
	expected = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(expected)), "sha256:")
	if sum != expected {
		return fmt.Errorf("%w: staged checksum %s does not match %s", ErrInvalidStage, sum, expected)
	}
	return nil
}

func hashFile(r io.Reader) (string, int64, error) {
	hasher := sha256.New()
	written, errCopy := io.Copy(hasher, r)
	if errCopy != nil {
		return "", written, fmt.Errorf("selfupdate: hash file: %w", errCopy)
	}
	return hex.EncodeToString(hasher.Sum(nil)), written, nil
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
