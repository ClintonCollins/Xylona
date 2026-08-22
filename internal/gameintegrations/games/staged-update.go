package games

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
)

type stagedUpdateMode int

const (
	stagedUpdateInstall stagedUpdateMode = iota
	stagedUpdateRetainRollback
)

type stagedUpdate struct {
	directory       string
	gameID          string
	workspace       string
	payload         string
	mode            stagedUpdateMode
	retainWorkspace bool
	renamePath      func(oldPath string, newPath string) error
}

func newStagedUpdate(directory string, gameID string, mode stagedUpdateMode) (*stagedUpdate, error) {
	cleanDirectory := filepath.Clean(strings.TrimSpace(directory))
	if cleanDirectory == "." || cleanDirectory == "" {
		return nil, errors.New("staged update directory is required")
	}
	errMkdir := os.MkdirAll(cleanDirectory, 0o750)
	if errMkdir != nil {
		return nil, fmt.Errorf("create staged update root: %w", errMkdir)
	}
	manifestPath := filepath.Join(cleanDirectory, filepath.FromSlash(gameintegrations.InternalUpdateManifestPath))
	_, errManifest := os.Lstat(manifestPath)
	if errManifest == nil {
		return nil, errors.New("an unresolved internal update transaction already exists")
	}
	if !errors.Is(errManifest, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect internal update manifest: %w", errManifest)
	}

	prefixID := strings.Map(func(character rune) rune {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '-' || character == '_' {
			return character
		}
		return '-'
	}, strings.TrimSpace(gameID))
	if prefixID == "" {
		prefixID = "game"
	}
	workspace, errWorkspace := os.MkdirTemp(cleanDirectory, ".xylona-"+prefixID+"-update-")
	if errWorkspace != nil {
		return nil, fmt.Errorf("create staged update workspace: %w", errWorkspace)
	}
	payload := filepath.Join(workspace, "payload")
	errPayload := os.MkdirAll(payload, 0o750)
	if errPayload != nil {
		errCleanup := removeStagedUpdateTree(cleanDirectory, workspace)
		return nil, errors.Join(
			fmt.Errorf("create staged update payload: %w", errPayload),
			wrapError("remove staged update workspace", errCleanup),
		)
	}

	return &stagedUpdate{
		directory:  cleanDirectory,
		gameID:     strings.TrimSpace(gameID),
		workspace:  workspace,
		payload:    payload,
		mode:       mode,
		renamePath: os.Rename,
	}, nil
}

func (update *stagedUpdate) PayloadDirectory() string {
	if update == nil {
		return ""
	}
	return update.payload
}

func (update *stagedUpdate) WorkspacePath(name string) string {
	if update == nil {
		return ""
	}
	return filepath.Join(update.workspace, name)
}

func (update *stagedUpdate) CleanupTransient() error {
	if update == nil || update.retainWorkspace {
		return nil
	}
	return removeStagedUpdateTree(update.directory, update.workspace)
}

func (update *stagedUpdate) Apply(
	protected func(relativePath string) bool,
	directoryReplacements []string,
) error {
	if update == nil {
		return errors.New("staged update is nil")
	}

	replacementDirectories := make(map[string]struct{}, len(directoryReplacements))
	for _, replacement := range directoryReplacements {
		relative, errRelative := normalizeStagedRelativePath(replacement)
		if errRelative != nil {
			return errRelative
		}
		replacementDirectories[relative] = struct{}{}
	}

	entries, errEntries := update.collectEntries(protected, replacementDirectories)
	if errEntries != nil {
		return errEntries
	}
	if len(entries) == 0 {
		return errors.New("staged update payload has no applicable files")
	}

	manifestEntries := make([]gameintegrations.UpdateTransactionEntry, 0, len(entries))
	for _, entry := range entries {
		livePath := filepath.Join(update.directory, filepath.FromSlash(entry.path))
		errParents := validateStagedTargetParents(update.directory, entry.path)
		if errParents != nil {
			return errParents
		}
		info, errStat := os.Lstat(livePath)
		existed := errStat == nil
		if errStat != nil && !errors.Is(errStat, os.ErrNotExist) {
			return fmt.Errorf("inspect staged update target %q: %w", entry.path, errStat)
		}
		if existed {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("staged update target %q is a symlink", entry.path)
			}
			if entry.directory != info.IsDir() {
				return fmt.Errorf("staged update target %q has a file/directory collision", entry.path)
			}
			if !entry.directory && !info.Mode().IsRegular() {
				return fmt.Errorf("staged update target %q is not a regular file", entry.path)
			}
		}
		manifestEntries = append(manifestEntries, gameintegrations.UpdateTransactionEntry{
			Path:      entry.path,
			Existed:   existed,
			Directory: entry.directory,
		})
	}

	manifest := gameintegrations.NewUpdateTransactionManifest(update.gameID, manifestEntries)
	errManifest := gameintegrations.ValidateUpdateTransactionManifest(manifest)
	if errManifest != nil {
		return fmt.Errorf("validate internal update manifest: %w", errManifest)
	}
	errWriteManifest := update.writeManifest(manifest)
	if errWriteManifest != nil {
		return errWriteManifest
	}

	for index, entry := range entries {
		errApply := update.applyEntry(entry, manifest.Entries[index])
		if errApply == nil {
			continue
		}
		errRollback := update.rollback(manifest)
		if errRollback != nil {
			update.retainWorkspace = true
			return errors.Join(errApply, fmt.Errorf("rollback staged update: %w", errRollback))
		}
		return errApply
	}

	errCommitted := writeSyncedFile(
		filepath.Join(update.directory, filepath.FromSlash(gameintegrations.InternalUpdateCommittedPath)),
		[]byte("committed\n"),
		0o600,
	)
	if errCommitted != nil {
		errRollback := update.rollback(manifest)
		if errRollback != nil {
			update.retainWorkspace = true
			return errors.Join(
				fmt.Errorf("write staged update commit marker: %w", errCommitted),
				fmt.Errorf("rollback staged update: %w", errRollback),
			)
		}
		return fmt.Errorf("write staged update commit marker: %w", errCommitted)
	}

	if update.mode == stagedUpdateInstall {
		errFinalize := update.removeRollbackDirectory()
		if errFinalize != nil {
			return fmt.Errorf("finalize staged install: %w", errFinalize)
		}
	}
	return nil
}

type stagedUpdateEntry struct {
	path      string
	directory bool
}

func (update *stagedUpdate) collectEntries(
	protected func(relativePath string) bool,
	directoryReplacements map[string]struct{},
) ([]stagedUpdateEntry, error) {
	entries := make([]stagedUpdateEntry, 0)
	seen := make(map[string]struct{})
	errWalk := filepath.WalkDir(update.payload, func(currentPath string, dirEntry fs.DirEntry, errWalk error) error {
		if errWalk != nil {
			return fmt.Errorf("walk staged update payload: %w", errWalk)
		}
		if currentPath == update.payload {
			return nil
		}
		relativeOS, errRelative := filepath.Rel(update.payload, currentPath)
		if errRelative != nil {
			return fmt.Errorf("resolve staged update path: %w", errRelative)
		}
		relative, errNormalize := normalizeStagedRelativePath(filepath.ToSlash(relativeOS))
		if errNormalize != nil {
			return errNormalize
		}
		info, errInfo := dirEntry.Info()
		if errInfo != nil {
			return fmt.Errorf("inspect staged update payload %q: %w", relative, errInfo)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("staged update payload %q is a symlink", relative)
		}

		_, replaceDirectory := directoryReplacements[relative]
		if replaceDirectory {
			if !info.IsDir() {
				return fmt.Errorf("staged update directory replacement %q is not a directory", relative)
			}
			errTree := validateStagedTree(currentPath)
			if errTree != nil {
				return errTree
			}
			if protected == nil || !protected(relative) {
				entries = append(entries, stagedUpdateEntry{path: relative, directory: true})
			}
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("staged update payload %q is not a regular file", relative)
		}
		if protected != nil && protected(relative) {
			return nil
		}
		key := relative
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("staged update payload contains duplicate path %q", relative)
		}
		seen[key] = struct{}{}
		entries = append(entries, stagedUpdateEntry{path: relative})
		return nil
	})
	if errWalk != nil {
		return nil, fmt.Errorf("collect staged update entries: %w", errWalk)
	}
	slices.SortFunc(entries, func(left stagedUpdateEntry, right stagedUpdateEntry) int {
		return strings.Compare(left.path, right.path)
	})
	return entries, nil
}

func validateStagedTree(root string) error {
	errWalk := filepath.WalkDir(root, func(currentPath string, entry fs.DirEntry, errWalk error) error {
		if errWalk != nil {
			return fmt.Errorf("walk staged replacement directory: %w", errWalk)
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			return fmt.Errorf("inspect staged replacement directory: %w", errInfo)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("staged replacement contains symlink %q", currentPath)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("staged replacement contains non-regular path %q", currentPath)
		}
		return nil
	})
	if errWalk != nil {
		return fmt.Errorf("validate staged replacement tree: %w", errWalk)
	}
	return nil
}

func (update *stagedUpdate) writeManifest(manifest gameintegrations.UpdateTransactionManifest) error {
	manifestPath := filepath.Join(update.directory, filepath.FromSlash(gameintegrations.InternalUpdateManifestPath))
	_, errExisting := os.Lstat(manifestPath)
	if errExisting == nil {
		return errors.New("an unresolved internal update transaction already exists")
	}
	if !errors.Is(errExisting, os.ErrNotExist) {
		return fmt.Errorf("inspect internal update manifest: %w", errExisting)
	}
	manifestBytes, errMarshal := json.Marshal(manifest)
	if errMarshal != nil {
		return fmt.Errorf("marshal internal update manifest: %w", errMarshal)
	}
	errMkdir := os.MkdirAll(filepath.Dir(manifestPath), 0o750)
	if errMkdir != nil {
		return fmt.Errorf("create internal update rollback directory: %w", errMkdir)
	}
	errWrite := writeSyncedFile(manifestPath, manifestBytes, 0o600)
	if errWrite != nil {
		return fmt.Errorf("write internal update manifest: %w", errWrite)
	}
	return nil
}

func (update *stagedUpdate) applyEntry(
	entry stagedUpdateEntry,
	manifestEntry gameintegrations.UpdateTransactionEntry,
) error {
	if manifestEntry.Path != entry.path {
		return fmt.Errorf("internal update manifest is missing %q", entry.path)
	}
	livePath := filepath.Join(update.directory, filepath.FromSlash(entry.path))
	stagedPath := filepath.Join(update.payload, filepath.FromSlash(entry.path))
	backupPath := filepath.Join(
		update.directory,
		filepath.FromSlash(gameintegrations.InternalUpdateFilesDirectory),
		filepath.FromSlash(entry.path),
	)
	if manifestEntry.Existed {
		errMkdirBackup := os.MkdirAll(filepath.Dir(backupPath), 0o750)
		if errMkdirBackup != nil {
			return fmt.Errorf("create staged update rollback parent for %q: %w", entry.path, errMkdirBackup)
		}
		errBackup := update.renamePath(livePath, backupPath)
		if errBackup != nil {
			return fmt.Errorf("retain staged update original %q: %w", entry.path, errBackup)
		}
	}
	errMkdirLive := os.MkdirAll(filepath.Dir(livePath), 0o750)
	if errMkdirLive != nil {
		return fmt.Errorf("create staged update target parent for %q: %w", entry.path, errMkdirLive)
	}
	errPromote := update.renamePath(stagedPath, livePath)
	if errPromote != nil {
		return fmt.Errorf("promote staged update path %q: %w", entry.path, errPromote)
	}
	return nil
}

func (update *stagedUpdate) rollback(manifest gameintegrations.UpdateTransactionManifest) error {
	var rollbackErrors []error
	for _, entry := range slices.Backward(manifest.Entries) {
		livePath := filepath.Join(update.directory, filepath.FromSlash(entry.Path))
		backupPath := filepath.Join(
			update.directory,
			filepath.FromSlash(gameintegrations.InternalUpdateFilesDirectory),
			filepath.FromSlash(entry.Path),
		)
		_, errBackupStat := os.Lstat(backupPath)
		backupExists := errBackupStat == nil
		if errBackupStat != nil && !errors.Is(errBackupStat, os.ErrNotExist) {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("inspect rollback path %q: %w", entry.Path, errBackupStat))
			continue
		}
		if entry.Existed && !backupExists {
			continue
		}
		errRemoveLive := removeUpdateTarget(update.directory, livePath, entry.Directory)
		if errRemoveLive != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("remove updated path %q: %w", entry.Path, errRemoveLive))
			continue
		}
		if entry.Existed {
			errMkdir := os.MkdirAll(filepath.Dir(livePath), 0o750)
			if errMkdir != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("create rollback target parent %q: %w", entry.Path, errMkdir))
				continue
			}
			errRestore := update.renamePath(backupPath, livePath)
			if errRestore != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore original path %q: %w", entry.Path, errRestore))
			}
		}
	}
	if len(rollbackErrors) > 0 {
		return errors.Join(rollbackErrors...)
	}
	return update.removeRollbackDirectory()
}

func (update *stagedUpdate) removeRollbackDirectory() error {
	rollbackDirectory := filepath.Join(update.directory, filepath.FromSlash(gameintegrations.InternalUpdateBackupDirectory))
	errRemove := removeStagedUpdateTree(update.directory, rollbackDirectory)
	if errRemove != nil {
		return errRemove
	}
	updateBackupRoot := filepath.Join(update.directory, ".update-backup")
	errRemoveRoot := os.Remove(updateBackupRoot)
	if errRemoveRoot != nil && !errors.Is(errRemoveRoot, os.ErrNotExist) && !isDirectoryNotEmpty(errRemoveRoot) {
		return fmt.Errorf("remove empty update backup root: %w", errRemoveRoot)
	}
	return nil
}

func validateStagedTargetParents(root string, relativePath string) error {
	parent := filepath.Dir(filepath.FromSlash(relativePath))
	if parent == "." {
		return nil
	}
	current := root
	for part := range strings.SplitSeq(parent, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, errStat := os.Lstat(current)
		if errors.Is(errStat, os.ErrNotExist) {
			continue
		}
		if errStat != nil {
			return fmt.Errorf("inspect staged update parent %q: %w", current, errStat)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("staged update parent %q is a symlink", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("staged update parent %q is not a directory", current)
		}
	}
	return nil
}

func normalizeStagedRelativePath(relativePath string) (string, error) {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(relativePath)))
	if cleaned == "." || !fs.ValidPath(cleaned) {
		return "", fmt.Errorf("unsafe staged update path %q", relativePath)
	}
	if cleaned == ".update-backup" || strings.HasPrefix(cleaned, ".update-backup/") ||
		strings.HasPrefix(cleaned, ".xylona-") {
		return "", fmt.Errorf("reserved staged update path %q", relativePath)
	}
	return cleaned, nil
}

func removeUpdateTarget(root string, target string, directory bool) error {
	info, errStat := os.Lstat(target)
	if errors.Is(errStat, os.ErrNotExist) {
		return nil
	}
	if errStat != nil {
		return fmt.Errorf("inspect update rollback target: %w", errStat)
	}
	if directory {
		return removeStagedUpdateTree(root, target)
	}
	if info.IsDir() {
		return errors.New("target is unexpectedly a directory")
	}
	errRemove := os.Remove(target)
	if errRemove != nil {
		return fmt.Errorf("remove update rollback target: %w", errRemove)
	}
	return nil
}

func writeSyncedFile(target string, contents []byte, mode fs.FileMode) (errResult error) {
	temporary, errCreate := os.CreateTemp(filepath.Dir(target), ".xylona-update-write-*")
	if errCreate != nil {
		return fmt.Errorf("create temporary update file: %w", errCreate)
	}
	temporaryPath := temporary.Name()
	defer func() {
		errRemove := os.Remove(temporaryPath)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			errResult = errors.Join(errResult, fmt.Errorf("remove temporary update file: %w", errRemove))
		}
	}()
	errChmod := temporary.Chmod(mode)
	_, errWrite := temporary.Write(contents)
	errSync := temporary.Sync()
	errClose := temporary.Close()
	if errChmod != nil || errWrite != nil || errSync != nil || errClose != nil {
		return errors.Join(errChmod, errWrite, errSync, errClose)
	}
	errRename := os.Rename(temporaryPath, target)
	if errRename != nil {
		return fmt.Errorf("promote temporary update file: %w", errRename)
	}
	return nil
}

func removeStagedUpdateTree(root string, target string) error {
	cleanRoot := filepath.Clean(root)
	cleanTarget := filepath.Clean(target)
	relative, errRelative := filepath.Rel(cleanRoot, cleanTarget)
	if errRelative != nil {
		return fmt.Errorf("resolve staged update cleanup path: %w", errRelative)
	}
	if relative == "." || !filepath.IsLocal(relative) {
		return fmt.Errorf("refuse unsafe staged update cleanup path %q", target)
	}
	errRemove := os.RemoveAll(cleanTarget)
	if errRemove != nil {
		return fmt.Errorf("remove staged update tree: %w", errRemove)
	}
	return nil
}

func isDirectoryNotEmpty(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "directory not empty") || strings.Contains(message, "not empty")
}
