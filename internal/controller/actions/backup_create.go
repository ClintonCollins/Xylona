package actions

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type backupArchiveCreator interface {
	CreateBackupArchive(ctx context.Context, directory string, includePaths []string, destinationArchivePath string) (int64, string, error)
}

// DefaultBackupDirectory returns the default root directory for stored backups.
func DefaultBackupDirectory() (string, error) {
	installPath, errInstallPath := DefaultInstallPath()
	if errInstallPath != nil {
		return "", errInstallPath
	}
	return joinManagedPath(installPath, "backups"), nil
}

// ValidateManualBackupName checks whether a user-supplied manual backup name can
// be safely converted into an archive filename.
func ValidateManualBackupName(name string) error {
	_, errNormalize := normalizeManualBackupName(name)
	return errNormalize
}

// CreateManualBackup writes a zip archive and records a retention-exempt manual backup row.
func (inst *Instance) CreateManualBackup(gameServer *models.GameServer, createdBy string, name string) (*models.GameServerBackup, error) {
	backup, errCreate := inst.prepareBackup(gameServer, "manual", createdBy, true, name)
	if errCreate != nil {
		return nil, errCreate
	}

	backupCtx, backupDone := inst.registerBackupCreate(backup.ID)
	go inst.completeManualBackup(backupCtx, backupDone, gameServer, backup)

	return backup, nil
}

// CreateScheduledBackup writes a zip archive and prunes older scheduled artifacts beyond retention.
func (inst *Instance) CreateScheduledBackup(gameServer *models.GameServer) (*models.GameServerBackup, error) {
	backup, errCreate := inst.prepareBackup(gameServer, "scheduled", "", false, "")
	if errCreate != nil {
		return nil, errCreate
	}

	completedBackup, errComplete := inst.executeBackupCreate(inst.ctx, gameServer, backup)
	if errComplete != nil {
		return nil, errComplete
	}

	keepCount := int(gameServer.MaxBackups)
	if keepCount <= 0 {
		keepCount = DefaultScheduledBackupRetention
	}

	inst.broadcastBackupProgress(
		gameServer.ID,
		gameServer.Name,
		completedBackup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_PRUNING,
		90,
		completedBackup.SizeBytes,
		"Pruning scheduled backups",
	)
	errPrune := inst.pruneScheduledBackups(gameServer, keepCount)
	if errPrune != nil {
		inst.broadcastBackupProgress(
			gameServer.ID,
			gameServer.Name,
			completedBackup.ID,
			xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
			xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_FAILED,
			100,
			completedBackup.SizeBytes,
			"Failed to prune scheduled backups",
		)
		return completedBackup, errPrune
	}

	inst.broadcastBackupProgress(
		gameServer.ID,
		gameServer.Name,
		completedBackup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_COMPLETE,
		100,
		completedBackup.SizeBytes,
		"Backup complete",
	)

	return completedBackup, nil
}

func (inst *Instance) prepareBackup(
	gameServer *models.GameServer,
	triggerSource string,
	createdBy string,
	retentionExempt bool,
	backupName string,
) (*models.GameServerBackup, error) {
	backupDirectory, errValidate := inst.validateBackupCreateSettings(gameServer)
	if errValidate != nil {
		return nil, errValidate
	}

	now := backupNowFunc().UTC()
	archivePath, errPath := inst.resolveUniqueBackupArchivePath(gameServer, backupDirectory, now, triggerSource, backupName)
	if errPath != nil {
		return nil, fmt.Errorf("actions: resolve backup archive path: %w", errPath)
	}
	trimmedBackupName := strings.TrimSpace(backupName)
	if trimmedBackupName != "" && !inst.isRemoteGameServer(gameServer) {
		errEnsurePath := ensureBackupArchivePathAvailable(inst.db, gameServer.ID, archivePath)
		if errEnsurePath != nil {
			return nil, errEnsurePath
		}
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

	return backup, nil
}

func (inst *Instance) completeManualBackup(backupCtx context.Context, backupDone func(), gameServer *models.GameServer, backup *models.GameServerBackup) {
	if inst == nil || gameServer == nil || backup == nil {
		return
	}
	if backupDone != nil {
		defer backupDone()
	}

	completedBackup, errComplete := inst.executeBackupCreate(backupCtx, gameServer, backup)
	if errComplete != nil {
		if errors.Is(errComplete, errBackupCreateCancelled) {
			return
		}
		log.Error().
			Err(errComplete).
			Str("game_server_id", gameServer.ID).
			Str("backup_id", backup.ID).
			Msg("Manual backup failed in background")
		return
	}

	inst.broadcastBackupProgress(
		gameServer.ID,
		gameServer.Name,
		completedBackup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_COMPLETE,
		100,
		completedBackup.SizeBytes,
		"Backup complete",
	)
}

func (inst *Instance) executeBackupCreate(backupCtx context.Context, gameServer *models.GameServer, backup *models.GameServerBackup) (*models.GameServerBackup, error) {
	inst.broadcastBackupProgress(
		gameServer.ID,
		gameServer.Name,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_PREPARING,
		5,
		0,
		"Preparing backup archive",
	)

	inst.broadcastBackupProgress(
		gameServer.ID,
		gameServer.Name,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_ARCHIVING,
		50,
		0,
		"Archiving game server files",
	)

	progressReporter := newBackupProgressReporter(inst, gameServer.ID, gameServer.Name, backup.ID)

	sizeBytes, errArchive := inst.produceBackupArchive(backupCtx, gameServer, backup.ArchivePath, progressReporter.Observe)
	if errArchive != nil {
		progressReporter.Abort()
		if errors.Is(errArchive, errBackupCreateCancelled) {
			return nil, errBackupCreateCancelled
		}
		return nil, inst.failBackupCreate(gameServer, backup, gameServer.ID, gameServer.Name, errArchive)
	}
	progressReporter.Close(sizeBytes)

	completedAt := time.Now().UTC()
	updatedBackup, errUpdate := updateGameServerBackupResult(inst.db, backup.ID, db.UpdateGameServerBackupResultParams{
		Status:       "completed",
		SizeBytes:    sizeBytes,
		ErrorMessage: "",
		CompletedAt:  &completedAt,
	})
	if errUpdate != nil {
		return nil, inst.handleBackupFinalizationFailure(gameServer, backup, gameServer.ID, gameServer.Name, errUpdate)
	}

	return updatedBackup, nil
}

func (inst *Instance) validateBackupCreateSettings(gameServer *models.GameServer) (string, error) {
	if !gameServer.BackupsEnabled {
		return "", errBackupsDisabled
	}
	if inst.isRemoteGameServer(gameServer) {
		return resolveValidRemoteGameServerBackupDirectory(gameServer)
	}
	return resolveValidGameServerBackupDirectory(gameServer)
}

func isGameServerOffline(inst *Instance, gameServer *models.GameServer) bool {
	if inst != nil {
		return inst.currentProcessStatus(gameServer) == xylona.Status_OFFLINE
	}

	return strings.EqualFold(gameServer.Status, xylona.Status_OFFLINE.String())
}

func normalizeManualBackupName(name string) (string, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return "", nil
	}

	var builder strings.Builder
	builder.Grow(len(trimmedName))
	lastSeparator := false

	for _, currentRune := range trimmedName {
		if isAllowedManualBackupNameRune(currentRune) {
			builder.WriteRune(currentRune)
			lastSeparator = false
			continue
		}
		if !lastSeparator {
			builder.WriteByte('-')
			lastSeparator = true
		}
	}

	normalizedName := strings.Trim(builder.String(), " .-_")
	if len(normalizedName) > maxManualBackupNameLength {
		normalizedName = strings.Trim(normalizedName[:maxManualBackupNameLength], " .-_")
	}
	if normalizedName == "" {
		return "", ErrInvalidManualBackupName
	}
	if isWindowsReservedArchiveBaseName(normalizedName) {
		normalizedName += "-backup"
	}

	return normalizedName, nil
}

func isAllowedManualBackupNameRune(currentRune rune) bool {
	switch {
	case currentRune >= 'a' && currentRune <= 'z':
		return true
	case currentRune >= 'A' && currentRune <= 'Z':
		return true
	case currentRune >= '0' && currentRune <= '9':
		return true
	case currentRune == '-':
		return true
	case currentRune == '_':
		return true
	case currentRune == '.':
		return true
	default:
		return false
	}
}

func isWindowsReservedArchiveBaseName(name string) bool {
	upperName := strings.ToUpper(strings.TrimSpace(strings.TrimSuffix(name, filepath.Ext(name))))

	switch upperName {
	case "CON", "PRN", "AUX", "NUL":
		return true
	case "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9":
		return true
	case "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}

// produceBackupArchive routes archive creation through the owning node.
func (inst *Instance) produceBackupArchive(
	ctx context.Context,
	gameServer *models.GameServer,
	controllerArchivePath string,
	onProgress backupArchiveProgressFunc,
) (int64, error) {
	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		return 0, fmt.Errorf("actions: resolve node client for backup: %w", errClient)
	}
	return inst.produceBackupArchiveWithNodeClient(ctx, client, gameServer, controllerArchivePath, onProgress)
}

// produceBackupArchiveWithNodeClient creates and stores the archive on the owning node.
func (inst *Instance) produceBackupArchiveWithNodeClient(
	ctx context.Context,
	client backupArchiveCreator,
	gameServer *models.GameServer,
	archivePath string,
	onProgress backupArchiveProgressFunc,
) (int64, error) {
	archiveBytes, _, errArchive := client.CreateBackupArchive(ctx, gameServer.Directory, nil, archivePath)
	if errArchive != nil {
		errCancel := backupCreateCancelErr(ctx)
		if errCancel != nil {
			return 0, errCancel
		}
		return 0, fmt.Errorf("actions: create backup archive on node: %w", errArchive)
	}

	if onProgress != nil {
		onProgress(archiveBytes)
	}
	return archiveBytes, nil
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
	gameServer *models.GameServer,
	backup *models.GameServerBackup,
	gameServerID string,
	gameServerName string,
	finalizeErr error,
) error {
	terminalErr := fmt.Errorf("actions: finalize backup row after archive write: %w", finalizeErr)

	errRemoveArchive := inst.removeBackupArchive(gameServer, backup.ArchivePath)
	if errRemoveArchive != nil {
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
		gameServerName,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_FAILED,
		100,
		backup.SizeBytes,
		"Backup failed",
	)

	return terminalErr
}

func (inst *Instance) failBackupCreate(
	gameServer *models.GameServer,
	backup *models.GameServerBackup,
	gameServerID string,
	gameServerName string,
	cause error,
) error {
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
		errRemove := inst.removeBackupArchive(gameServer, backup.ArchivePath)
		if errRemove != nil {
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
		gameServerName,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_FAILED,
		100,
		backup.SizeBytes,
		"Backup failed",
	)

	return terminalErr
}
