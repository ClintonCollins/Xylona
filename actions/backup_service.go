package actions

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	// DefaultScheduledBackupRetention is the scheduled-backup keep count used
	// when a server has no explicit retention configured.
	DefaultScheduledBackupRetention = 5
	maxBackupArchiveResolveAttempts = 1000
	maxBackupExtractEntrySizeBytes  = 8 << 30
)

var (
	errBackupsDisabled              = errors.New("backups are disabled for this game server")
	errBackupDirectoryNotConfigured = errors.New("backup directory is not configured for this game server")
	errBackupDirectoryInsideServer  = errors.New("backup directory cannot be inside the game server directory")
	errBackupRestoreRequiresOffline = errors.New("game server must be offline to restore a backup")
	errBackupNodeMismatch           = errors.New("backup belongs to a different node")
	errBackupNotCompleted           = errors.New("backup is not completed")
	errUnsupportedBackupArchive     = errors.New("unsupported backup archive format")
	errInvalidBackupArchivePath     = errors.New("invalid backup archive path")
	errRestoreDestinationSymlink    = errors.New("restore destination contains a symlink")
	backupRestoreUserFacingErrors   = []error{
		errBackupsDisabled,
		errBackupRestoreRequiresOffline,
		errBackupNodeMismatch,
		errBackupNotCompleted,
		errUnsupportedBackupArchive,
		errInvalidBackupArchivePath,
		errBackupDirectoryInsideServer,
		errRestoreDestinationSymlink,
	}
)

var (
	createGameServerBackupRow = func(conn *db.Connection, params db.CreateGameServerBackupParams) (*models.GameServerBackup, error) {
		return conn.CreateGameServerBackup(params)
	}
	writeBackupArchiveFunc       = writeBackupArchive
	copyRestoreFileFunc          = copyRestoreFile
	createRestoreStagingDirFunc  = os.MkdirTemp
	backupNowFunc                = func() time.Time { return time.Now().UTC() }
	updateGameServerBackupResult = func(
		conn *db.Connection,
		backupID string,
		params db.UpdateGameServerBackupResultParams,
	) (*models.GameServerBackup, error) {
		return conn.UpdateGameServerBackupResult(backupID, params)
	}
	deleteGameServerBackupRow = func(conn *db.Connection, backupID string) error {
		return conn.DeleteGameServerBackup(backupID)
	}
)

// DefaultBackupDirectory returns the default root directory for stored backups.
func DefaultBackupDirectory() string {
	return filepath.Join(DefaultInstallPath(), "backups")
}

// CreateManualBackup writes a zip archive and records a retention-exempt manual backup row.
func (inst *Instance) CreateManualBackup(gameServer *models.GameServer, createdBy string) (*models.GameServerBackup, error) {
	backup, errCreate := inst.createBackup(gameServer, "manual", createdBy, true)
	if errCreate != nil {
		return nil, errCreate
	}

	inst.broadcastBackupProgress(
		gameServer.ID,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_COMPLETE,
		100,
		"Backup complete",
	)

	return backup, nil
}

// CreateScheduledBackup writes a zip archive and prunes older scheduled artifacts beyond retention.
func (inst *Instance) CreateScheduledBackup(gameServer *models.GameServer) (*models.GameServerBackup, error) {
	backup, errCreate := inst.createBackup(gameServer, "scheduled", "", false)
	if errCreate != nil {
		return nil, errCreate
	}

	keepCount := int(gameServer.MaxBackups)
	if keepCount <= 0 {
		keepCount = DefaultScheduledBackupRetention
	}

	inst.broadcastBackupProgress(
		gameServer.ID,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_PRUNING,
		90,
		"Pruning scheduled backups",
	)
	errPrune := inst.pruneScheduledBackups(gameServer, keepCount)
	if errPrune != nil {
		inst.broadcastBackupProgress(
			gameServer.ID,
			backup.ID,
			xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
			xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_FAILED,
			100,
			"Failed to prune scheduled backups",
		)
		return backup, errPrune
	}

	inst.broadcastBackupProgress(
		gameServer.ID,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_COMPLETE,
		100,
		"Backup complete",
	)

	return backup, nil
}

// DeleteGameServerBackup deletes a backup row and its archive when it belongs to the current server node.
func (inst *Instance) DeleteGameServerBackup(gameServer *models.GameServer, backup *models.GameServerBackup) error {
	if backup == nil {
		return errInvalidBackupArchivePath
	}
	if backup.GameServerID != gameServer.ID {
		return fmt.Errorf("actions: backup does not belong to game server %s", gameServer.ID)
	}

	archivePath, errArchivePath := resolveValidatedBackupArchivePath(gameServer, backup)
	if errArchivePath != nil {
		return errArchivePath
	}

	errRemoveArchive := os.Remove(archivePath)
	if errRemoveArchive != nil && !errors.Is(errRemoveArchive, os.ErrNotExist) {
		return fmt.Errorf("actions: remove backup archive before deleting backup row: %w", errRemoveArchive)
	}

	errDeleteRow := deleteGameServerBackupRow(inst.db, backup.ID)
	if errDeleteRow != nil {
		return fmt.Errorf("actions: delete backup row after archive cleanup: %w", errDeleteRow)
	}

	return nil
}

func (inst *Instance) createBackup(
	gameServer *models.GameServer,
	triggerSource string,
	createdBy string,
	retentionExempt bool,
) (*models.GameServerBackup, error) {
	backupDirectory, errValidate := validateBackupCreateSettings(gameServer)
	if errValidate != nil {
		return nil, errValidate
	}

	now := backupNowFunc().UTC()
	archivePath, errPath := resolveUniqueBackupArchivePath(backupDirectory, gameServer.ID, now, triggerSource)
	if errPath != nil {
		return nil, fmt.Errorf("actions: resolve backup archive path: %w", errPath)
	}
	backup, errCreateRow := createGameServerBackupRow(inst.db, db.CreateGameServerBackupParams{
		GameServerID:    gameServer.ID,
		NodeID:          gameServer.NodeID,
		CreatedBy:       createdBy,
		TriggerSource:   triggerSource,
		ArchivePath:     archivePath,
		ArchiveRoot:     backupDirectory,
		ArchiveFormat:   "zip",
		Status:          "pending",
		SizeBytes:       0,
		RetentionExempt: retentionExempt,
		CreatedAt:       now,
		CompletedAt:     nil,
	})
	if errCreateRow != nil {
		return nil, fmt.Errorf("actions: create backup row: %w", errCreateRow)
	}

	inst.broadcastBackupProgress(
		gameServer.ID,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_PREPARING,
		5,
		"Preparing backup archive",
	)

	parentDir := filepath.Dir(archivePath)
	errMkdir := os.MkdirAll(parentDir, 0o750)
	if errMkdir != nil {
		return nil, inst.failBackupCreate(backup, gameServer.ID, fmt.Errorf("actions: create backup directory: %w", errMkdir))
	}

	inst.broadcastBackupProgress(
		gameServer.ID,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_ARCHIVING,
		50,
		"Archiving game server files",
	)
	sizeBytes, errArchive := writeBackupArchiveFunc(gameServer.Directory, archivePath)
	if errArchive != nil {
		return nil, inst.failBackupCreate(backup, gameServer.ID, errArchive)
	}

	completedAt := time.Now().UTC()
	updatedBackup, errUpdate := updateGameServerBackupResult(inst.db, backup.ID, db.UpdateGameServerBackupResultParams{
		Status:       "completed",
		SizeBytes:    sizeBytes,
		ErrorMessage: "",
		CompletedAt:  &completedAt,
	})
	if errUpdate != nil {
		return nil, inst.handleBackupFinalizationFailure(backup, gameServer.ID, errUpdate)
	}

	return updatedBackup, nil
}

// RestoreGameServerBackup extracts a backup into staging and applies it with overlay or exact semantics.
func (inst *Instance) RestoreGameServerBackup(
	gameServer *models.GameServer,
	backupID string,
	restoreMode xylona.BackupRestoreMode,
) error {
	errValidate := validateBackupRestoreSettings(inst, gameServer)
	if errValidate != nil {
		return errValidate
	}

	backup, errGetBackup := inst.db.GetGameServerBackupByID(backupID)
	if errGetBackup != nil {
		return fmt.Errorf("actions: get backup by ID: %w", errGetBackup)
	}
	if backup.GameServerID != gameServer.ID {
		return fmt.Errorf("actions: backup does not belong to game server %s", gameServer.ID)
	}

	archivePath, errArchivePath := resolveValidatedBackupArchivePath(gameServer, backup)
	if errArchivePath != nil {
		return errArchivePath
	}
	if backup.Status != "completed" {
		return errBackupNotCompleted
	}
	if backup.ArchiveFormat != "zip" {
		return errUnsupportedBackupArchive
	}

	inst.broadcastBackupProgress(
		gameServer.ID,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_RESTORE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_PREPARING,
		5,
		"Preparing backup restore",
	)

	stagingParent := filepath.Dir(gameServer.Directory)
	stagingDir, errTemp := createRestoreStagingDirFunc(stagingParent, gameServer.ID+"-restore-")
	if errTemp != nil {
		inst.broadcastBackupProgress(
			gameServer.ID,
			backup.ID,
			xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_RESTORE,
			xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_FAILED,
			100,
			"Restore failed",
		)
		return fmt.Errorf("actions: create restore staging directory: %w", errTemp)
	}
	defer func() {
		_ = os.RemoveAll(stagingDir)
	}()

	inst.broadcastBackupProgress(
		gameServer.ID,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_RESTORE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_STAGING,
		45,
		"Extracting backup archive",
	)
	errExtract := extractBackupArchive(archivePath, stagingDir)
	if errExtract != nil {
		inst.broadcastBackupProgress(
			gameServer.ID,
			backup.ID,
			xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_RESTORE,
			xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_FAILED,
			100,
			"Restore failed",
		)
		return errExtract
	}

	inst.broadcastBackupProgress(
		gameServer.ID,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_RESTORE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_APPLYING,
		80,
		"Applying backup contents",
	)
	errApply := applyBackupRestore(stagingDir, gameServer.Directory, restoreMode)
	if errApply != nil {
		inst.broadcastBackupProgress(
			gameServer.ID,
			backup.ID,
			xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_RESTORE,
			xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_FAILED,
			100,
			"Restore failed",
		)
		return errApply
	}

	inst.broadcastBackupProgress(
		gameServer.ID,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_RESTORE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_COMPLETE,
		100,
		"Restore complete",
	)

	return nil
}

// BackupRestoreUserFacingMessage returns a safe client-facing message for expected restore failures.
func BackupRestoreUserFacingMessage(err error) (string, bool) {
	if err == nil {
		return "", false
	}

	for _, userFacingErr := range backupRestoreUserFacingErrors {
		if errors.Is(err, userFacingErr) {
			return userFacingErr.Error(), true
		}
	}

	return "", false
}

// ValidateGameServerBackupDirectory checks whether the configured backup directory is usable for the server.
func ValidateGameServerBackupDirectory(gameServer *models.GameServer) error {
	_, errValidate := resolveValidGameServerBackupDirectory(gameServer)
	return errValidate
}

func validateBackupCreateSettings(gameServer *models.GameServer) (string, error) {
	if !gameServer.BackupsEnabled {
		return "", errBackupsDisabled
	}

	return resolveValidGameServerBackupDirectory(gameServer)
}

func validateBackupRestoreSettings(inst *Instance, gameServer *models.GameServer) error {
	if !gameServer.BackupsEnabled {
		return errBackupsDisabled
	}
	if !isGameServerOffline(inst, gameServer) {
		return errBackupRestoreRequiresOffline
	}

	return nil
}

func resolveValidatedBackupArchivePath(gameServer *models.GameServer, backup *models.GameServerBackup) (string, error) {
	if backup == nil {
		return "", errInvalidBackupArchivePath
	}
	if backup.NodeID != gameServer.NodeID {
		return "", errBackupNodeMismatch
	}

	resolvedArchivePath, errArchivePath := resolvePathForComparison(backup.ArchivePath)
	if errArchivePath != nil {
		return "", fmt.Errorf("actions: resolve backup archive path: %w", errArchivePath)
	}
	resolvedArchiveRoot, errArchiveRoot := resolvePathForComparison(strings.TrimSpace(backup.ArchiveRoot))
	if errArchiveRoot != nil {
		return "", fmt.Errorf("actions: resolve backup archive root: %w", errArchiveRoot)
	}
	if !strings.EqualFold(filepath.Ext(resolvedArchivePath), ".zip") {
		return "", errInvalidBackupArchivePath
	}
	resolvedArchiveDirectory, errArchiveDirectory := resolvePathForComparison(buildBackupArchiveDirectory(resolvedArchiveRoot, gameServer.ID))
	if errArchiveDirectory != nil {
		return "", fmt.Errorf("actions: resolve backup archive directory: %w", errArchiveDirectory)
	}
	if !pathWithinOrEqual(resolvedArchiveDirectory, resolvedArchivePath) {
		return "", errInvalidBackupArchivePath
	}
	if backupDirectoryWithinGameServer(gameServer.Directory, resolvedArchiveDirectory) {
		return "", errBackupDirectoryInsideServer
	}

	return resolvedArchivePath, nil
}

func resolveValidGameServerBackupDirectory(gameServer *models.GameServer) (string, error) {
	backupDirectory := strings.TrimSpace(gameServer.BackupDirectory)
	if backupDirectory == "" {
		return "", errBackupDirectoryNotConfigured
	}
	archiveDirectory := buildBackupArchiveDirectory(backupDirectory, gameServer.ID)
	if backupDirectoryWithinGameServer(gameServer.Directory, archiveDirectory) {
		return "", errBackupDirectoryInsideServer
	}

	return backupDirectory, nil
}

func isGameServerOffline(inst *Instance, gameServer *models.GameServer) bool {
	if inst != nil && inst.supervisorInstance != nil {
		command, errGetCommand := inst.supervisorInstance.GetCommandByID(gameServer.ID)
		if errGetCommand == nil {
			return command.Status() == xylona.Status_OFFLINE
		}
	}

	return strings.EqualFold(gameServer.Status, xylona.Status_OFFLINE.String())
}

func buildBackupArchivePath(backupDirectory string, gameServerID string, now time.Time, triggerSource string) string {
	timestamp := now.UTC().Format("20060102T150405.000000000Z")
	fileName := timestamp + "-" + triggerSource + ".zip"
	return filepath.Join(buildBackupArchiveDirectory(backupDirectory, gameServerID), fileName)
}

func buildBackupArchiveDirectory(backupDirectory string, gameServerID string) string {
	return filepath.Join(backupDirectory, gameServerID)
}

func resolveUniqueBackupArchivePath(
	backupDirectory string,
	gameServerID string,
	now time.Time,
	triggerSource string,
) (string, error) {
	basePath := buildBackupArchivePath(backupDirectory, gameServerID, now, triggerSource)
	baseDirectory := filepath.Dir(basePath)
	baseName := strings.TrimSuffix(filepath.Base(basePath), ".zip")

	for suffix := range maxBackupArchiveResolveAttempts {
		candidatePath := basePath
		if suffix > 0 {
			fileName := fmt.Sprintf("%s-%d.zip", baseName, suffix)
			candidatePath = filepath.Join(baseDirectory, fileName)
		}

		_, errStat := os.Stat(candidatePath)
		if errors.Is(errStat, os.ErrNotExist) {
			return candidatePath, nil
		}
		if errStat != nil {
			return "", fmt.Errorf("stat backup archive candidate: %w", errStat)
		}
	}

	return "", fmt.Errorf("exhausted backup archive path candidates after %d attempts", maxBackupArchiveResolveAttempts)
}

func writeBackupArchive(serverDirectory string, archivePath string) (int64, error) {
	outputFile, errCreate := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errCreate != nil {
		return 0, fmt.Errorf("actions: create backup archive: %w", errCreate)
	}

	zipWriter := zip.NewWriter(outputFile)
	cleanServerDirectory := filepath.Clean(serverDirectory)
	cleanArchivePath := filepath.Clean(archivePath)
	errWalk := filepath.WalkDir(cleanServerDirectory, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("actions: walk backup source: %w", walkErr)
		}
		if currentPath == cleanServerDirectory {
			return nil
		}

		cleanCurrentPath := filepath.Clean(currentPath)
		if cleanCurrentPath == cleanArchivePath {
			return nil
		}

		relativePath, errRel := filepath.Rel(cleanServerDirectory, cleanCurrentPath)
		if errRel != nil {
			return fmt.Errorf("actions: resolve backup relative path: %w", errRel)
		}
		archiveEntryPath, errValidate := validateBackupRelativePath(relativePath)
		if errValidate != nil {
			return errValidate
		}

		info, errInfo := entry.Info()
		if errInfo != nil {
			return fmt.Errorf("actions: stat backup source: %w", errInfo)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("actions: symlinks are not supported in backups: %s", archiveEntryPath)
		}
		if info.IsDir() {
			return addDirectoryToZipArchive(zipWriter, archiveEntryPath, info)
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		return addFileToZipArchive(zipWriter, cleanCurrentPath, archiveEntryPath, info)
	})

	errCloseZip := zipWriter.Close()
	errCloseFile := outputFile.Close()
	if errWalk != nil {
		return 0, cleanupPartialBackupArchive(archivePath, fmt.Errorf("actions: walk backup source: %w", errWalk))
	}
	if errCloseZip != nil {
		return 0, cleanupPartialBackupArchive(archivePath, fmt.Errorf("actions: finalize backup archive: %w", errCloseZip))
	}
	if errCloseFile != nil {
		return 0, cleanupPartialBackupArchive(archivePath, fmt.Errorf("actions: close backup archive: %w", errCloseFile))
	}

	archiveInfo, errStat := os.Stat(archivePath)
	if errStat != nil {
		return 0, fmt.Errorf("actions: stat backup archive: %w", errStat)
	}

	return archiveInfo.Size(), nil
}

func cleanupPartialBackupArchive(archivePath string, cause error) error {
	errRemoveArchive := os.Remove(archivePath)
	if errRemoveArchive != nil && !errors.Is(errRemoveArchive, os.ErrNotExist) {
		return errors.Join(cause, fmt.Errorf("actions: remove partial backup archive: %w", errRemoveArchive))
	}

	return cause
}

func addDirectoryToZipArchive(zipWriter *zip.Writer, archiveEntryPath string, info fs.FileInfo) error {
	header, errHeader := zip.FileInfoHeader(info)
	if errHeader != nil {
		return fmt.Errorf("actions: create zip directory header: %w", errHeader)
	}
	header.Name = archiveEntryPath + "/"
	header.Method = zip.Store

	_, errEntry := zipWriter.CreateHeader(header)
	if errEntry != nil {
		return fmt.Errorf("actions: create zip directory entry: %w", errEntry)
	}

	return nil
}

func addFileToZipArchive(zipWriter *zip.Writer, sourcePath string, archiveEntryPath string, info fs.FileInfo) error {
	header, errHeader := zip.FileInfoHeader(info)
	if errHeader != nil {
		return fmt.Errorf("actions: create zip header: %w", errHeader)
	}
	header.Name = archiveEntryPath
	header.Method = zip.Deflate

	archiveWriter, errEntry := zipWriter.CreateHeader(header)
	if errEntry != nil {
		return fmt.Errorf("actions: create zip entry: %w", errEntry)
	}

	sourceFile, errOpen := os.Open(sourcePath)
	if errOpen != nil {
		return fmt.Errorf("actions: open backup source file: %w", errOpen)
	}
	defer func() {
		_ = sourceFile.Close()
	}()

	_, errCopy := io.Copy(archiveWriter, sourceFile)
	if errCopy != nil {
		return fmt.Errorf("actions: write zip entry: %w", errCopy)
	}

	return nil
}

func extractBackupArchive(archivePath string, stagingDirectory string) error {
	archiveReader, errOpen := zip.OpenReader(archivePath)
	if errOpen != nil {
		return fmt.Errorf("actions: open backup archive: %w", errOpen)
	}
	defer func() {
		_ = archiveReader.Close()
	}()

	cleanStagingDirectory := filepath.Clean(stagingDirectory)
	stagingPrefix := cleanStagingDirectory + string(filepath.Separator)
	directoryModes := make(map[string]fs.FileMode)
	for _, file := range archiveReader.File {
		relativePath, errValidate := validateBackupZipEntryPath(file.Name)
		if errValidate != nil {
			return errValidate
		}

		targetPath := filepath.Join(cleanStagingDirectory, filepath.FromSlash(relativePath))
		cleanTargetPath := filepath.Clean(targetPath)
		if cleanTargetPath != cleanStagingDirectory && !strings.HasPrefix(cleanTargetPath, stagingPrefix) {
			return errInvalidBackupArchivePath
		}

		mode := file.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("actions: symlinks are not supported in backups: %s", relativePath)
		}

		if file.FileInfo().IsDir() {
			errMkdir := os.MkdirAll(cleanTargetPath, 0o750)
			if errMkdir != nil {
				return fmt.Errorf("actions: create staged directory: %w", errMkdir)
			}
			directoryModes[cleanTargetPath] = mode
			continue
		}

		parentDirectory := filepath.Dir(cleanTargetPath)
		errMkdir := os.MkdirAll(parentDirectory, 0o750)
		if errMkdir != nil {
			return fmt.Errorf("actions: create staged file parent: %w", errMkdir)
		}

		errWrite := writeZipEntryToFile(file, cleanTargetPath, mode)
		if errWrite != nil {
			return errWrite
		}
	}

	errApplyModes := applyDirectoryModes(directoryModes)
	if errApplyModes != nil {
		return fmt.Errorf("actions: apply staged directory modes: %w", errApplyModes)
	}

	return nil
}

func writeZipEntryToFile(file *zip.File, targetPath string, mode fs.FileMode) error {
	entryReader, errOpen := file.Open()
	if errOpen != nil {
		return fmt.Errorf("actions: open zip entry: %w", errOpen)
	}

	perm := mode.Perm()
	if perm == 0 {
		perm = 0o600
	}
	if file.UncompressedSize64 > maxBackupExtractEntrySizeBytes {
		errCloseReader := entryReader.Close()
		if errCloseReader != nil {
			return fmt.Errorf("actions: zip entry exceeds restore size limit: %w", errors.Join(
				fmt.Errorf("%s", file.Name),
				errCloseReader,
			))
		}
		return fmt.Errorf("actions: zip entry exceeds restore size limit: %s", file.Name)
	}
	targetFile, errCreate := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if errCreate != nil {
		errCloseReader := entryReader.Close()
		if errCloseReader != nil {
			return fmt.Errorf("actions: create staged file: %w", errors.Join(errCreate, errCloseReader))
		}
		return fmt.Errorf("actions: create staged file: %w", errCreate)
	}

	expectedSizeBytes := int64(file.UncompressedSize64)
	limitedEntryReader := io.LimitReader(entryReader, expectedSizeBytes+1)
	bytesWritten, errCopy := io.Copy(targetFile, limitedEntryReader)
	var errSizeMismatch error
	if bytesWritten != expectedSizeBytes {
		errSizeMismatch = fmt.Errorf(
			"actions: extracted zip entry size mismatch for %s: wrote %d bytes, expected %d",
			file.Name,
			bytesWritten,
			expectedSizeBytes,
		)
	}
	errSync := targetFile.Sync()
	errCloseFile := targetFile.Close()
	errCloseReader := entryReader.Close()

	if errCopy != nil || errSizeMismatch != nil || errSync != nil || errCloseFile != nil || errCloseReader != nil {
		errExtract := errors.Join(errCopy, errSizeMismatch, errSync, errCloseFile, errCloseReader)
		return fmt.Errorf("actions: extract zip entry: %w", errExtract)
	}

	return nil
}

func applyBackupRestore(stagingDirectory string, gameServerDirectory string, restoreMode xylona.BackupRestoreMode) error {
	switch restoreMode {
	case xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_UNSPECIFIED, xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_OVERLAY:
		return syncBackupOverlay(stagingDirectory, gameServerDirectory)
	case xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_EXACT:
		return syncBackupExact(stagingDirectory, gameServerDirectory)
	default:
		return fmt.Errorf("actions: unsupported restore mode: %s", restoreMode.String())
	}
}

func syncBackupOverlay(stagingDirectory string, gameServerDirectory string) error {
	directoryModes, errSync := syncBackupOverlayContents(stagingDirectory, gameServerDirectory)
	if errSync != nil {
		return errSync
	}

	errApplyModes := applyDirectoryModes(directoryModes)
	if errApplyModes != nil {
		return fmt.Errorf("actions: apply restore directory modes: %w", errApplyModes)
	}

	return nil
}

func syncBackupOverlayContents(stagingDirectory string, gameServerDirectory string) (map[string]fs.FileMode, error) {
	type stagedRestoreEntry struct {
		sourcePath   string
		relativePath string
		isDirectory  bool
		mode         fs.FileMode
	}

	var stagedEntries []stagedRestoreEntry
	directoryModes := make(map[string]fs.FileMode)
	errWalk := filepath.WalkDir(stagingDirectory, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("actions: walk staged restore files: %w", walkErr)
		}
		if currentPath == stagingDirectory {
			return nil
		}

		relativePath, errRel := filepath.Rel(stagingDirectory, currentPath)
		if errRel != nil {
			return fmt.Errorf("actions: resolve staged restore path: %w", errRel)
		}
		validatedPath, errValidate := validateBackupRelativePath(relativePath)
		if errValidate != nil {
			return errValidate
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			return fmt.Errorf("actions: stat staged restore entry: %w", errInfo)
		}

		stagedEntries = append(stagedEntries, stagedRestoreEntry{
			sourcePath:   currentPath,
			relativePath: validatedPath,
			isDirectory:  entry.IsDir(),
			mode:         info.Mode(),
		})
		return nil
	})
	if errWalk != nil {
		return nil, fmt.Errorf("actions: sync restore overlay: %w", errWalk)
	}

	for _, stagedEntry := range stagedEntries {
		targetPath, errPath := validateRestoreDestinationPath(gameServerDirectory, stagedEntry.relativePath)
		if errPath != nil {
			return nil, errPath
		}
		if stagedEntry.isDirectory {
			errEnsure := ensureRestoreDirectoryTarget(gameServerDirectory, targetPath)
			if errEnsure != nil {
				return nil, errEnsure
			}
			directoryModes[targetPath] = stagedEntry.mode
			continue
		}

		errPrepare := ensureRestoreFileTarget(gameServerDirectory, targetPath)
		if errPrepare != nil {
			return nil, errPrepare
		}
		errCopy := copyRestoreFileFunc(stagedEntry.sourcePath, targetPath)
		if errCopy != nil {
			return nil, errCopy
		}
	}

	return directoryModes, nil
}

func copyRestoreFile(src string, dst string) error {
	srcFile, errOpen := os.Open(src)
	if errOpen != nil {
		return fmt.Errorf("actions: open restore source file: %w", errOpen)
	}
	defer func() {
		_ = srcFile.Close()
	}()

	srcInfo, errStat := srcFile.Stat()
	if errStat != nil {
		return fmt.Errorf("actions: stat restore source file: %w", errStat)
	}

	parentDirectory := filepath.Dir(dst)
	tempFile, errCreate := os.CreateTemp(parentDirectory, `.xylona-restore-*`)
	if errCreate != nil {
		return fmt.Errorf("actions: create restore temp file: %w", errCreate)
	}
	tempPath := tempFile.Name()
	removeTempFile := true
	defer func() {
		if removeTempFile {
			_ = os.Remove(tempPath)
		}
	}()

	perm := srcInfo.Mode().Perm()
	if perm == 0 {
		perm = 0o600
	}

	_, errCopy := io.Copy(tempFile, srcFile)
	errSync := tempFile.Sync()
	errClose := tempFile.Close()
	if errCopy != nil || errSync != nil || errClose != nil {
		return fmt.Errorf("actions: copy restore file: %w", errors.Join(errCopy, errSync, errClose))
	}

	errChmod := os.Chmod(tempPath, perm)
	if errChmod != nil {
		return fmt.Errorf("actions: set restore destination mode: %w", errChmod)
	}

	errRemoveDestination := os.Remove(dst)
	if errRemoveDestination != nil && !errors.Is(errRemoveDestination, os.ErrNotExist) {
		return fmt.Errorf("actions: replace restore destination file: %w", errRemoveDestination)
	}

	errRename := os.Rename(tempPath, dst)
	if errRename != nil {
		return fmt.Errorf("actions: move restore temp file into place: %w", errRename)
	}
	removeTempFile = false

	return nil
}

func ensureRestoreDirectoryTarget(gameServerDirectory string, targetPath string) error {
	errParents := ensureRestoreDirectoryChain(gameServerDirectory, filepath.Dir(targetPath))
	if errParents != nil {
		return errParents
	}

	info, errLstat := os.Lstat(targetPath)
	if errLstat == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return errRestoreDestinationSymlink
		}
		if !info.IsDir() {
			errRemove := os.RemoveAll(targetPath)
			if errRemove != nil {
				return fmt.Errorf("actions: replace restore path with directory: %w", errRemove)
			}
		}
	} else if !errors.Is(errLstat, os.ErrNotExist) {
		return fmt.Errorf("actions: inspect restore directory target: %w", errLstat)
	}

	errMkdir := os.MkdirAll(targetPath, 0o750)
	if errMkdir != nil {
		return fmt.Errorf("actions: create restore directory: %w", errMkdir)
	}

	return nil
}

func ensureRestoreFileTarget(gameServerDirectory string, targetPath string) error {
	errParents := ensureRestoreDirectoryChain(gameServerDirectory, filepath.Dir(targetPath))
	if errParents != nil {
		return errParents
	}

	info, errLstat := os.Lstat(targetPath)
	if errLstat == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return errRestoreDestinationSymlink
		}
		if info.IsDir() {
			errRemove := os.RemoveAll(targetPath)
			if errRemove != nil {
				return fmt.Errorf("actions: replace restore path with file: %w", errRemove)
			}
		}
	} else if !errors.Is(errLstat, os.ErrNotExist) {
		return fmt.Errorf("actions: inspect restore file target: %w", errLstat)
	}

	return nil
}

func ensureRestoreDirectoryChain(gameServerDirectory string, targetDirectory string) error {
	cleanRoot := filepath.Clean(gameServerDirectory)
	cleanTargetDirectory := filepath.Clean(targetDirectory)
	if cleanTargetDirectory == cleanRoot {
		return nil
	}

	relativePath, errRel := filepath.Rel(cleanRoot, cleanTargetDirectory)
	if errRel != nil {
		return fmt.Errorf("actions: resolve restore parent path: %w", errRel)
	}
	if relativePath == "." {
		return nil
	}

	currentPath := cleanRoot
	for part := range strings.SplitSeq(relativePath, string(filepath.Separator)) {
		currentPath = filepath.Join(currentPath, part)
		info, errLstat := os.Lstat(currentPath)
		if errLstat == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return errRestoreDestinationSymlink
			}
			if !info.IsDir() {
				errRemove := os.RemoveAll(currentPath)
				if errRemove != nil {
					return fmt.Errorf("actions: replace restore parent with directory: %w", errRemove)
				}
				errMkdir := os.Mkdir(currentPath, 0o750)
				if errMkdir != nil {
					return fmt.Errorf("actions: create restore parent directory: %w", errMkdir)
				}
			}
			continue
		}
		if !errors.Is(errLstat, os.ErrNotExist) {
			return fmt.Errorf("actions: inspect restore parent directory: %w", errLstat)
		}

		errMkdir := os.Mkdir(currentPath, 0o750)
		if errMkdir != nil {
			return fmt.Errorf("actions: create restore parent directory: %w", errMkdir)
		}
	}

	return nil
}

func applyDirectoryMode(targetPath string, mode fs.FileMode) error {
	perm := mode.Perm()
	if perm == 0 {
		return nil
	}

	errChmod := os.Chmod(targetPath, perm)
	if errChmod != nil {
		return fmt.Errorf("chmod directory: %w", errChmod)
	}

	return nil
}

func applyDirectoryModes(directoryModes map[string]fs.FileMode) error {
	paths := make([]string, 0, len(directoryModes))
	for pathValue := range directoryModes {
		paths = append(paths, pathValue)
	}

	slices.SortFunc(paths, func(left string, right string) int {
		if len(left) == len(right) {
			return strings.Compare(right, left)
		}
		return len(right) - len(left)
	})

	for _, pathValue := range paths {
		errApply := applyDirectoryMode(pathValue, directoryModes[pathValue])
		if errApply != nil {
			return errApply
		}
	}

	return nil
}

func syncBackupExact(stagingDirectory string, gameServerDirectory string) error {
	directoryModes, errOverlay := syncBackupOverlayContents(stagingDirectory, gameServerDirectory)
	if errOverlay != nil {
		return errOverlay
	}

	var extraPaths []string
	errWalk := filepath.WalkDir(gameServerDirectory, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("actions: walk exact restore destination: %w", walkErr)
		}
		if currentPath == gameServerDirectory {
			return nil
		}

		relativePath, errRel := filepath.Rel(gameServerDirectory, currentPath)
		if errRel != nil {
			return fmt.Errorf("actions: resolve exact restore path: %w", errRel)
		}
		validatedPath, errValidate := validateBackupRelativePath(relativePath)
		if errValidate != nil {
			return errValidate
		}

		stagedPath := filepath.Join(stagingDirectory, filepath.FromSlash(validatedPath))
		_, errStat := os.Stat(stagedPath)
		if errors.Is(errStat, os.ErrNotExist) {
			extraPaths = append(extraPaths, currentPath)
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if errStat != nil {
			return fmt.Errorf("actions: stat exact restore staged entry: %w", errStat)
		}

		return nil
	})
	if errWalk != nil {
		return fmt.Errorf("actions: sync restore exact: %w", errWalk)
	}

	slices.SortFunc(extraPaths, func(left string, right string) int {
		if len(left) == len(right) {
			return strings.Compare(right, left)
		}
		return len(right) - len(left)
	})

	for _, extraPath := range extraPaths {
		errRemove := os.RemoveAll(extraPath)
		if errRemove != nil {
			return fmt.Errorf("actions: remove extra restore path: %w", errRemove)
		}
	}

	errApplyModes := applyDirectoryModes(directoryModes)
	if errApplyModes != nil {
		return fmt.Errorf("actions: apply restore directory modes: %w", errApplyModes)
	}

	return nil
}

func backupDirectoryWithinGameServer(gameServerDirectory string, backupDirectory string) bool {
	resolvedGameServerDirectory, errGameServer := resolvePathForComparison(gameServerDirectory)
	if errGameServer != nil {
		return false
	}
	resolvedBackupDirectory, errBackupDirectory := resolvePathForComparison(backupDirectory)
	if errBackupDirectory != nil {
		return false
	}

	return pathWithinOrEqual(resolvedGameServerDirectory, resolvedBackupDirectory)
}

func validateBackupRelativePath(relativePath string) (string, error) {
	slashNormalized := strings.ReplaceAll(relativePath, "\\", "/")
	return validateBackupSlashPath(slashNormalized)
}

func validateBackupZipEntryPath(entryName string) (string, error) {
	if entryName == "" {
		return "", errInvalidBackupArchivePath
	}

	slashNormalized := strings.ReplaceAll(entryName, "\\", "/")
	return validateBackupSlashPath(slashNormalized)
}

func validateBackupSlashPath(slashPath string) (string, error) {
	cleaned := path.Clean(slashPath)
	if cleaned == "." || cleaned == "" {
		return "", errInvalidBackupArchivePath
	}
	if strings.HasPrefix(cleaned, "/") {
		return "", errInvalidBackupArchivePath
	}
	if cleaned == ".." {
		return "", errInvalidBackupArchivePath
	}
	if strings.HasPrefix(cleaned, "../") {
		return "", errInvalidBackupArchivePath
	}
	if strings.Contains(cleaned, "/../") {
		return "", errInvalidBackupArchivePath
	}
	if hasWindowsDrivePrefix(cleaned) {
		return "", errInvalidBackupArchivePath
	}

	return cleaned, nil
}

func hasWindowsDrivePrefix(pathValue string) bool {
	if len(pathValue) < 2 {
		return false
	}
	if (pathValue[0] < 'A' || pathValue[0] > 'Z') && (pathValue[0] < 'a' || pathValue[0] > 'z') {
		return false
	}
	if pathValue[1] != ':' {
		return false
	}
	if len(pathValue) == 2 {
		return true
	}
	return pathValue[2] == '/'
}

func resolvePathForComparison(pathValue string) (string, error) {
	cleanAbsolutePath, errAbs := filepath.Abs(strings.TrimSpace(pathValue))
	if errAbs != nil {
		return "", fmt.Errorf("actions: resolve absolute path: %w", errAbs)
	}

	currentPath := filepath.Clean(cleanAbsolutePath)
	var missingPathSuffix []string
	for {
		resolvedPath, errEval := filepath.EvalSymlinks(currentPath)
		if errEval == nil {
			for i := len(missingPathSuffix) - 1; i >= 0; i-- {
				resolvedPath = filepath.Join(resolvedPath, missingPathSuffix[i])
			}
			return filepath.Clean(resolvedPath), nil
		}
		if !errors.Is(errEval, os.ErrNotExist) {
			return "", fmt.Errorf("actions: resolve symlinks: %w", errEval)
		}

		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			return currentPath, nil
		}

		missingPathSuffix = append(missingPathSuffix, filepath.Base(currentPath))
		currentPath = parentPath
	}
}

func pathWithinOrEqual(parentPath string, candidatePath string) bool {
	relativePath, errRel := filepath.Rel(parentPath, candidatePath)
	if errRel != nil {
		return false
	}
	if relativePath == "." {
		return true
	}
	if relativePath == ".." {
		return false
	}
	if strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func validateRestoreDestinationPath(gameServerDirectory string, relativePath string) (string, error) {
	targetPath := filepath.Join(gameServerDirectory, filepath.FromSlash(relativePath))
	cleanGameServerDirectory := filepath.Clean(gameServerDirectory)
	cleanTargetPath := filepath.Clean(targetPath)
	gameServerPrefix := cleanGameServerDirectory + string(filepath.Separator)
	if cleanTargetPath != cleanGameServerDirectory && !strings.HasPrefix(cleanTargetPath, gameServerPrefix) {
		return "", errInvalidBackupArchivePath
	}

	relativeParts := strings.Split(filepath.FromSlash(relativePath), string(filepath.Separator))
	currentPath := cleanGameServerDirectory
	for _, part := range relativeParts {
		currentPath = filepath.Join(currentPath, part)
		info, errLstat := os.Lstat(currentPath)
		if errLstat != nil {
			if errors.Is(errLstat, os.ErrNotExist) {
				return cleanTargetPath, nil
			}
			return "", fmt.Errorf("actions: inspect restore destination: %w", errLstat)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errRestoreDestinationSymlink
		}
	}

	return cleanTargetPath, nil
}

func (inst *Instance) pruneScheduledBackups(gameServer *models.GameServer, keepCount int) error {
	pruneCandidates, errPrune := inst.db.PruneScheduledGameServerBackups(gameServer.ID, gameServer.NodeID, keepCount)
	if errPrune != nil {
		return fmt.Errorf("actions: select scheduled backups to prune: %w", errPrune)
	}

	for _, pruneCandidate := range pruneCandidates {
		errDelete := inst.DeleteGameServerBackup(gameServer, pruneCandidate)
		if errDelete != nil {
			return fmt.Errorf("actions: delete pruned backup: %w", errDelete)
		}
	}

	return nil
}

func (inst *Instance) handleBackupFinalizationFailure(
	backup *models.GameServerBackup,
	gameServerID string,
	finalizeErr error,
) error {
	terminalErr := fmt.Errorf("actions: finalize backup row after archive write: %w", finalizeErr)

	errRemoveArchive := os.Remove(backup.ArchivePath)
	if errRemoveArchive != nil && !errors.Is(errRemoveArchive, os.ErrNotExist) {
		log.Error().
			Err(errRemoveArchive).
			Str("backup_id", backup.ID).
			Str("archive_path", backup.ArchivePath).
			Msg("Failed to remove archive after backup finalization failure")
		terminalErr = errors.Join(
			terminalErr,
			fmt.Errorf("actions: remove archive after backup finalization failure: %w", errRemoveArchive),
		)
	}

	errDeleteRow := deleteGameServerBackupRow(inst.db, backup.ID)
	if errDeleteRow != nil {
		log.Error().
			Err(errDeleteRow).
			Str("backup_id", backup.ID).
			Msg("Failed to delete backup row after backup finalization failure")
		terminalErr = errors.Join(
			terminalErr,
			fmt.Errorf("actions: delete backup row after finalization failure: %w", errDeleteRow),
		)

		completedAt := time.Now().UTC()
		_, errMarkFailed := updateGameServerBackupResult(inst.db, backup.ID, db.UpdateGameServerBackupResultParams{
			Status:       "failed",
			SizeBytes:    0,
			ErrorMessage: terminalErr.Error(),
			CompletedAt:  &completedAt,
		})
		if errMarkFailed != nil {
			log.Error().
				Err(errMarkFailed).
				Str("backup_id", backup.ID).
				Msg("Failed to reconcile backup row after backup finalization failure")
			terminalErr = errors.Join(
				terminalErr,
				fmt.Errorf("actions: reconcile backup row after finalization failure: %w", errMarkFailed),
			)
		}
	}

	inst.broadcastBackupProgress(
		gameServerID,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_FAILED,
		100,
		"Backup failed",
	)

	return terminalErr
}

func (inst *Instance) failBackupCreate(backup *models.GameServerBackup, gameServerID string, cause error) error {
	terminalErr := cause

	completedAt := time.Now().UTC()
	_, errUpdate := updateGameServerBackupResult(inst.db, backup.ID, db.UpdateGameServerBackupResultParams{
		Status:       "failed",
		SizeBytes:    0,
		ErrorMessage: cause.Error(),
		CompletedAt:  &completedAt,
	})
	if errUpdate != nil {
		log.Error().
			Err(errUpdate).
			Str("backup_id", backup.ID).
			Msg("Failed to mark backup row as failed after create failure")
		terminalErr = errors.Join(terminalErr, fmt.Errorf("actions: mark backup failed: %w", errUpdate))
	}

	if !errors.Is(cause, fs.ErrExist) {
		errRemove := os.Remove(backup.ArchivePath)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			log.Error().
				Err(errRemove).
				Str("backup_id", backup.ID).
				Str("archive_path", backup.ArchivePath).
				Msg("Failed to remove archive after backup create failure")
			terminalErr = errors.Join(
				terminalErr,
				fmt.Errorf("actions: remove archive after backup create failure: %w", errRemove),
			)
		}
	}

	if errUpdate != nil {
		errDelete := deleteGameServerBackupRow(inst.db, backup.ID)
		if errDelete != nil {
			log.Error().
				Err(errDelete).
				Str("backup_id", backup.ID).
				Msg("Failed to delete backup row after create failure reconciliation error")
			terminalErr = errors.Join(
				terminalErr,
				fmt.Errorf("actions: delete backup row after create failure reconciliation error: %w", errDelete),
			)
		}
	}

	inst.broadcastBackupProgress(
		gameServerID,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_FAILED,
		100,
		"Backup failed",
	)

	return terminalErr
}

func (inst *Instance) broadcastBackupProgress(
	gameServerID string,
	backupID string,
	operation xylona.BackupProgressOperation,
	phase xylona.BackupProgressPhase,
	percent int32,
	message string,
) {
	if inst == nil || inst.backupBroadcaster == nil {
		return
	}

	inst.backupBroadcaster.BroadcastBackupProgress(gameServerID, &xylona.BackupProgress{
		GameServerId: gameServerID,
		BackupId:     backupID,
		Operation:    operation,
		Phase:        phase,
		Percent:      percent,
		Message:      message,
	})
}
