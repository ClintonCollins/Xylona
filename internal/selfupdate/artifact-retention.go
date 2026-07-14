package selfupdate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const (
	maxRetainedStagedUpdates = 2
	stagedArtifactMaxAge     = 7 * 24 * time.Hour
	orphanedHandoffAge       = helperParentExitTimeout + time.Minute
	recentHelperGrace        = time.Minute
)

type stagedArtifactRecord struct {
	stageID   string
	createdAt time.Time
	metadata  stagedMetadata
	valid     bool
}

// reconcileArtifacts removes superseded update files while retaining the
// newest rollback executable and a small number of unapplied stages. It
// reports whether a recent helper handoff is still pending.
func (m *Manager) reconcileArtifacts(retainStages int) (bool, error) {
	return m.reconcileArtifactsPreserving(retainStages, "")
}

func (m *Manager) reconcileArtifactsPreserving(retainStages int, preserveStageID string) (bool, error) {
	entries, errReadDir := os.ReadDir(m.stageDir)
	if errors.Is(errReadDir, os.ErrNotExist) {
		return false, m.reconcileBackups(false)
	}
	if errReadDir != nil {
		return false, fmt.Errorf("read update staging directory: %w", errReadDir)
	}

	now := m.currentTime()
	legacyPending, errLegacyPending := m.reconcileLegacyPending(entries, now)
	if errLegacyPending != nil {
		return true, errLegacyPending
	}
	if legacyPending {
		return true, nil
	}
	pendingStages := make(map[string]struct{})
	var resultErr error
	for _, entry := range entries {
		stageID, ok := pendingStageID(entry.Name())
		if !ok {
			continue
		}
		pendingPath := filepath.Join(m.stageDir, entry.Name())
		info, errInfo := entry.Info()
		if errInfo != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("inspect pending update %q: %w", entry.Name(), errInfo))
			continue
		}
		pendingUpdateValue, errPendingData := readPendingUpdate(pendingPath)
		if errPendingData == nil {
			errPendingData = m.validatePendingUpdate(pendingUpdateValue, stageID)
		}
		if errPendingData != nil {
			if now.Sub(info.ModTime()) < orphanedHandoffAge {
				pendingStages[stageID] = struct{}{}
				resultErr = errors.Join(resultErr, errPendingData)
				continue
			}
			errCleanup := m.removeInvalidPendingArtifacts(pendingPath, stageID)
			if errCleanup != nil {
				resultErr = errors.Join(resultErr, errPendingData, errCleanup)
				continue
			}
			log.Warn().Err(errPendingData).Str("stage_id", stageID).Msg("selfupdate: discarded stale invalid update handoff")
			continue
		}
		if now.Sub(info.ModTime()) < orphanedHandoffAge {
			confirmed, errConfirmed := m.pendingHandoffMatchesCurrent(pendingPath, stageID)
			if errConfirmed != nil {
				pendingStages[stageID] = struct{}{}
				resultErr = errors.Join(resultErr, errConfirmed)
				continue
			}
			if !confirmed {
				pendingStages[stageID] = struct{}{}
				continue
			}
		}
		errPending := m.reconcileOrphanedPending(pendingPath, stageID)
		if errPending != nil {
			pendingStages[stageID] = struct{}{}
			resultErr = errors.Join(resultErr, errPending)
		}
	}

	currentSHA, currentExists, errCurrent := fileSHA256IfExists(m.executablePath)
	if errCurrent != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("inspect current executable during update reconciliation: %w", errCurrent))
	}

	records := make([]stagedArtifactRecord, 0)
	metadataStages := make(map[string]struct{})
	for _, entry := range entries {
		stageID, ok := metadataStageID(entry.Name())
		if !ok {
			continue
		}
		metadataStages[stageID] = struct{}{}
		info, errInfo := entry.Info()
		if errInfo != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("inspect staged metadata %q: %w", entry.Name(), errInfo))
			continue
		}
		record := stagedArtifactRecord{stageID: stageID, createdAt: info.ModTime()}
		metadataPath := filepath.Join(m.stageDir, entry.Name())
		metadata, errMetadata := m.readMetadataFile(metadataPath, stageID)
		if errMetadata == nil {
			record.metadata = metadata
			record.valid = true
			if !metadata.CreatedAt.IsZero() {
				record.createdAt = metadata.CreatedAt
			}
		}
		if stageID != preserveStageID && currentExists && record.valid && currentSHA == record.metadata.ExpectedSHA256 {
			errRemove := m.removeStageArtifacts(stageID, pendingStages)
			resultErr = errors.Join(resultErr, errRemove)
			continue
		}
		_, pending := pendingStages[stageID]
		if !pending {
			errCandidate := removeFileIfExists(m.installCandidatePath(stageID))
			resultErr = errors.Join(resultErr, errCandidate)
		}
		records = append(records, record)
	}

	sort.Slice(records, func(i int, j int) bool {
		if records[i].createdAt.Equal(records[j].createdAt) {
			return records[i].stageID > records[j].stageID
		}
		return records[i].createdAt.After(records[j].createdAt)
	})
	retained := 0
	for _, record := range records {
		if record.stageID == preserveStageID {
			continue
		}
		_, pending := pendingStages[record.stageID]
		if pending {
			continue
		}
		age := now.Sub(record.createdAt)
		keep := retained < retainStages && age <= stagedArtifactMaxAge
		if keep {
			retained++
			continue
		}
		errRemove := m.removeStageArtifacts(record.stageID, pendingStages)
		resultErr = errors.Join(resultErr, errRemove)
	}

	orphanEntries, errReadOrphans := os.ReadDir(m.stageDir)
	if errReadOrphans != nil && !errors.Is(errReadOrphans, os.ErrNotExist) {
		resultErr = errors.Join(resultErr, fmt.Errorf("read update staging directory for orphan cleanup: %w", errReadOrphans))
	} else {
		errOrphans := m.removeOrphanedStageArtifacts(orphanEntries, metadataStages, pendingStages, now)
		resultErr = errors.Join(resultErr, errOrphans)
	}
	errBackups := m.reconcileBackups(len(pendingStages) > 0)
	resultErr = errors.Join(resultErr, errBackups)
	return len(pendingStages) > 0, resultErr
}

func (m *Manager) reconcileLegacyPending(entries []os.DirEntry, now time.Time) (bool, error) {
	for _, entry := range entries {
		if entry.Name() != "pending-update.json" {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			return true, fmt.Errorf("inspect legacy pending update: %w", errInfo)
		}
		if now.Sub(info.ModTime()) < orphanedHandoffAge {
			return true, nil
		}
		errRemove := removeFileIfExists(filepath.Join(m.stageDir, entry.Name()))
		if errRemove != nil {
			return true, errRemove
		}
		return false, nil
	}
	return false, nil
}

func (m *Manager) removeInvalidPendingArtifacts(pendingPath string, stageID string) error {
	resultErr := removeFileIfExists(m.installCandidatePath(stageID))
	entries, errReadDir := os.ReadDir(m.stageDir)
	if errReadDir != nil && !errors.Is(errReadDir, os.ErrNotExist) {
		resultErr = errors.Join(resultErr, fmt.Errorf("read staging directory for invalid handoff cleanup: %w", errReadDir))
	} else {
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), stageID+"-") || !isHelperOrReadyArtifact(entry.Name()) {
				continue
			}
			errRemove := removeFileIfExists(filepath.Join(m.stageDir, entry.Name()))
			resultErr = errors.Join(resultErr, errRemove)
		}
	}
	errRemovePending := removeFileIfExists(pendingPath)
	return errors.Join(resultErr, errRemovePending)
}

func (m *Manager) reconcileOrphanedPending(pathValue string, stageID string) error {
	pending, errPending := readPendingUpdate(pathValue)
	if errPending != nil {
		return errPending
	}
	errValidate := m.validatePendingUpdate(pending, stageID)
	if errValidate != nil {
		return errValidate
	}

	_, currentExists, errCurrent := fileSHA256IfExists(m.executablePath)
	if errCurrent != nil {
		return fmt.Errorf("inspect executable for orphaned handoff: %w", errCurrent)
	}
	if !currentExists {
		backupExists, errBackup := pathExists(pending.BackupPath)
		if errBackup != nil {
			return fmt.Errorf("inspect orphaned update backup: %w", errBackup)
		}
		if !backupExists {
			return errors.New("reconcile orphaned update handoff: current executable and rollback backup are missing")
		}
		errRestore := os.Rename(pending.BackupPath, m.executablePath)
		if errRestore != nil {
			return fmt.Errorf("restore executable from orphaned update backup: %w", errRestore)
		}
	}

	errRemoveCandidate := removeFileIfExists(pending.StagedPath)
	errRemoveReady := removeHelperReadyFiles(pending.HelperReadyPath)
	errRemovePending := removeFileIfExists(pathValue)
	return errors.Join(errRemoveCandidate, errRemoveReady, errRemovePending)
}

func (m *Manager) pendingHandoffMatchesCurrent(pathValue string, stageID string) (bool, error) {
	pending, errPending := readPendingUpdate(pathValue)
	if errPending != nil {
		return false, errPending
	}
	errValidate := m.validatePendingUpdate(pending, stageID)
	if errValidate != nil {
		return false, errValidate
	}
	currentSHA, currentExists, errCurrent := fileSHA256IfExists(m.executablePath)
	if errCurrent != nil {
		return false, fmt.Errorf("inspect current executable for pending handoff: %w", errCurrent)
	}
	expectedSHA := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(pending.ExpectedSHA256)), "sha256:")
	return currentExists && expectedSHA != "" && currentSHA == expectedSHA, nil
}

func readPendingUpdate(pathValue string) (pendingUpdate, error) {
	file, errOpen := os.Open(pathValue)
	if errOpen != nil {
		return pendingUpdate{}, fmt.Errorf("open pending update handoff %q: %w", pathValue, errOpen)
	}
	var pending pendingUpdate
	errDecode := json.NewDecoder(file).Decode(&pending)
	errClose := file.Close()
	if errDecode != nil {
		errResult := fmt.Errorf("decode pending update handoff %q: %w", pathValue, errDecode)
		if errClose != nil {
			errResult = errors.Join(errResult, fmt.Errorf("close invalid pending update handoff %q: %w", pathValue, errClose))
		}
		return pendingUpdate{}, errResult
	}
	if errClose != nil {
		return pendingUpdate{}, fmt.Errorf("close pending update handoff %q: %w", pathValue, errClose)
	}
	return pending, nil
}

func (m *Manager) validatePendingUpdate(pending pendingUpdate, stageID string) error {
	if pending.StageID != stageID {
		return fmt.Errorf("reconcile orphaned update handoff: stage ID mismatch for %q", stageID)
	}
	if filepath.Clean(pending.ExecutablePath) != m.executablePath {
		return fmt.Errorf("reconcile orphaned update handoff: executable path mismatch for %q", stageID)
	}
	if filepath.Clean(pending.BackupPath) != m.executablePath+".bak-"+stageID {
		return fmt.Errorf("reconcile orphaned update handoff: backup path mismatch for %q", stageID)
	}
	if filepath.Clean(pending.StagedPath) != m.installCandidatePath(stageID) {
		return fmt.Errorf("reconcile orphaned update handoff: candidate path mismatch for %q", stageID)
	}
	readyDir := filepath.Dir(filepath.Clean(pending.HelperReadyPath))
	readyName := filepath.Base(filepath.Clean(pending.HelperReadyPath))
	if readyDir != m.stageDir || !strings.HasPrefix(readyName, stageID+"-") || !strings.HasSuffix(readyName, "-ready") {
		return fmt.Errorf("reconcile orphaned update handoff: ready path mismatch for %q", stageID)
	}
	return nil
}

func (m *Manager) removeStageArtifacts(stageID string, pendingStages map[string]struct{}) error {
	_, pending := pendingStages[stageID]
	if pending {
		return nil
	}
	resultErr := removeFileIfExists(m.installCandidatePath(stageID))
	entries, errReadDir := os.ReadDir(m.stageDir)
	if errors.Is(errReadDir, os.ErrNotExist) {
		return resultErr
	}
	if errReadDir != nil {
		return errors.Join(resultErr, fmt.Errorf("read update staging directory for cleanup: %w", errReadDir))
	}

	now := m.currentTime()
	for _, entry := range entries {
		if !stageArtifactNameMatches(entry.Name(), stageID) {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("inspect update artifact %q: %w", entry.Name(), errInfo))
			continue
		}
		if isHelperOrReadyArtifact(entry.Name()) && now.Sub(info.ModTime()) < recentHelperGrace {
			continue
		}
		errRemove := removeFileIfExists(filepath.Join(m.stageDir, entry.Name()))
		if errRemove != nil {
			resultErr = errors.Join(resultErr, errRemove)
		}
	}
	return resultErr
}

func (m *Manager) removeOrphanedStageArtifacts(entries []os.DirEntry, metadataStages map[string]struct{}, pendingStages map[string]struct{}, now time.Time) error {
	var resultErr error
	for _, entry := range entries {
		stageID, ok := artifactStageID(entry.Name())
		if !ok {
			continue
		}
		_, hasMetadata := metadataStages[stageID]
		if hasMetadata {
			continue
		}
		_, pending := pendingStages[stageID]
		if pending {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("inspect orphaned update artifact %q: %w", entry.Name(), errInfo))
			continue
		}
		if now.Sub(info.ModTime()) < orphanedHandoffAge {
			continue
		}
		errRemove := removeFileIfExists(filepath.Join(m.stageDir, entry.Name()))
		if errRemove != nil {
			resultErr = errors.Join(resultErr, errRemove)
		}
	}
	return resultErr
}

func (m *Manager) reconcileBackups(handoffPending bool) error {
	if handoffPending {
		return nil
	}
	dir := filepath.Dir(m.executablePath)
	entries, errReadDir := os.ReadDir(dir)
	if errReadDir != nil {
		return fmt.Errorf("read executable directory for rollback retention: %w", errReadDir)
	}
	prefix := filepath.Base(m.executablePath) + ".bak-"
	type backupRecord struct {
		path    string
		name    string
		modTime time.Time
	}
	backups := make([]backupRecord, 0)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		stageID := strings.TrimPrefix(entry.Name(), prefix)
		legacyTime, errLegacyTime := time.Parse("20060102150405", stageID)
		if !validStageID(stageID) && errLegacyTime != nil {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			return fmt.Errorf("inspect rollback backup %q: %w", entry.Name(), errInfo)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		modTime := info.ModTime()
		if errLegacyTime == nil {
			modTime = legacyTime
		}
		backups = append(backups, backupRecord{
			path:    filepath.Join(dir, entry.Name()),
			name:    entry.Name(),
			modTime: modTime,
		})
	}
	if len(backups) == 0 {
		return nil
	}
	sort.Slice(backups, func(i int, j int) bool {
		if backups[i].modTime.Equal(backups[j].modTime) {
			return backups[i].name > backups[j].name
		}
		return backups[i].modTime.After(backups[j].modTime)
	})

	currentExists, errCurrent := pathExists(m.executablePath)
	if errCurrent != nil {
		return fmt.Errorf("inspect current executable for rollback retention: %w", errCurrent)
	}
	keepIndex := 0
	if !currentExists {
		errRestore := os.Rename(backups[0].path, m.executablePath)
		if errRestore != nil {
			return fmt.Errorf("restore newest rollback backup: %w", errRestore)
		}
		keepIndex = 1
	}

	var resultErr error
	for idx := keepIndex + 1; idx < len(backups); idx++ {
		errRemove := removeFileIfExists(backups[idx].path)
		if errRemove != nil {
			resultErr = errors.Join(resultErr, errRemove)
		}
	}
	return resultErr
}

func (m *Manager) currentTime() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

func (m *Manager) readMetadataFile(pathValue string, stageID string) (stagedMetadata, error) {
	var metadata stagedMetadata
	file, errOpen := os.Open(pathValue)
	if errOpen != nil {
		return metadata, fmt.Errorf("open staged metadata: %w", errOpen)
	}
	errDecode := json.NewDecoder(file).Decode(&metadata)
	errClose := file.Close()
	if errDecode != nil {
		errResult := fmt.Errorf("decode staged metadata: %w", errDecode)
		if errClose != nil {
			errResult = errors.Join(errResult, fmt.Errorf("close invalid staged metadata: %w", errClose))
		}
		return metadata, errResult
	}
	if errClose != nil {
		return metadata, fmt.Errorf("close staged metadata: %w", errClose)
	}
	if metadata.StageID != stageID {
		return metadata, errors.New("metadata stage ID mismatch")
	}
	return metadata, nil
}

func fileSHA256IfExists(pathValue string) (string, bool, error) {
	file, errOpen := os.Open(pathValue)
	if errors.Is(errOpen, os.ErrNotExist) {
		return "", false, nil
	}
	if errOpen != nil {
		return "", false, fmt.Errorf("open file for checksum: %w", errOpen)
	}
	sum, errHash := hashFile(file)
	errClose := file.Close()
	if errHash != nil {
		if errClose != nil {
			return "", true, errors.Join(errHash, fmt.Errorf("close file after checksum failure: %w", errClose))
		}
		return "", true, errHash
	}
	if errClose != nil {
		return "", true, fmt.Errorf("close file after checksum: %w", errClose)
	}
	return sum, true, nil
}

func removeFileIfExists(pathValue string) error {
	errRemove := os.Remove(pathValue)
	if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
		return fmt.Errorf("remove update artifact %q: %w", pathValue, errRemove)
	}
	return nil
}

func metadataStageID(name string) (string, bool) {
	if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, "-pending.json") {
		return "", false
	}
	stageID := strings.TrimSuffix(name, ".json")
	return stageID, validStageID(stageID)
}

func pendingStageID(name string) (string, bool) {
	if !strings.HasSuffix(name, "-pending.json") {
		return "", false
	}
	stageID := strings.TrimSuffix(name, "-pending.json")
	return stageID, validStageID(stageID)
}

func artifactStageID(name string) (string, bool) {
	if len(name) < 36 {
		return "", false
	}
	stageID := name[:36]
	if !validStageID(stageID) || !stageArtifactNameMatches(name, stageID) {
		return "", false
	}
	return stageID, true
}

func validStageID(stageID string) bool {
	parsed, errParse := uuid.Parse(stageID)
	return errParse == nil && parsed.String() == stageID
}

func stageArtifactNameMatches(name string, stageID string) bool {
	if name == stageID+".bin" || name == stageID+".tmp" || name == stageID+".json" || name == stageID+"-pending.json" {
		return true
	}
	if !strings.HasPrefix(name, stageID+"-") {
		return false
	}
	return isHelperOrReadyArtifact(name)
}

func isHelperOrReadyArtifact(name string) bool {
	return strings.HasSuffix(name, "-helper") ||
		strings.HasSuffix(name, "-helper.exe") ||
		strings.HasSuffix(name, "-ready") ||
		strings.HasSuffix(name, "-ready.tmp")
}
