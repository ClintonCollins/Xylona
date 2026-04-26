package actions

import (
	"errors"
	"fmt"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// RestoreGameServerBackup asks the owning node to restore a completed backup.
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

	archivePath, errArchivePath := inst.resolveValidatedBackupArchivePath(gameServer, backup)
	if errArchivePath != nil {
		return errArchivePath
	}
	if backup.Status != "completed" {
		return errBackupNotCompleted
	}
	if backup.ArchiveFormat != "zip" {
		return errUnsupportedBackupArchive
	}

	return inst.restoreBackupArchiveWithNodeClient(gameServer, backup, archivePath, restoreMode)
}

// restoreBackupArchiveWithNodeClient asks the owning node to extract its
// node-local archive in place via NodeClient.ExtractBackupArchive.
func (inst *Instance) restoreBackupArchiveWithNodeClient(
	gameServer *models.GameServer,
	backup *models.GameServerBackup,
	archivePath string,
	restoreMode xylona.BackupRestoreMode,
) error {
	inst.broadcastBackupProgress(
		gameServer.ID,
		gameServer.Name,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_RESTORE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_PREPARING,
		5,
		backup.SizeBytes,
		"Preparing backup restore",
	)

	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		inst.broadcastBackupProgress(
			gameServer.ID,
			gameServer.Name,
			backup.ID,
			xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_RESTORE,
			xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_FAILED,
			100,
			backup.SizeBytes,
			"Restore failed",
		)
		return fmt.Errorf("actions: resolve node client for restore: %w", errClient)
	}

	inst.broadcastBackupProgress(
		gameServer.ID,
		gameServer.Name,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_RESTORE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_STAGING,
		45,
		backup.SizeBytes,
		"Loading backup archive",
	)

	inst.broadcastBackupProgress(
		gameServer.ID,
		gameServer.Name,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_RESTORE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_APPLYING,
		80,
		backup.SizeBytes,
		"Applying backup contents",
	)
	errExtract := client.ExtractBackupArchive(inst.ctx, gameServer.Directory, archivePath, backupRestoreModeToExtractMode(restoreMode))
	if errExtract != nil {
		inst.broadcastBackupProgress(
			gameServer.ID,
			gameServer.Name,
			backup.ID,
			xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_RESTORE,
			xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_FAILED,
			100,
			backup.SizeBytes,
			"Restore failed",
		)
		return fmt.Errorf("actions: extract backup archive on node: %w", errExtract)
	}

	inst.broadcastBackupProgress(
		gameServer.ID,
		gameServer.Name,
		backup.ID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_RESTORE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_COMPLETE,
		100,
		backup.SizeBytes,
		"Restore complete",
	)
	return nil
}

// backupRestoreModeToExtractMode maps the proto-level restore mode onto the
// transport-agnostic node.ExtractMode understood by NodeClient.
func backupRestoreModeToExtractMode(restoreMode xylona.BackupRestoreMode) node.ExtractMode {
	switch restoreMode {
	case xylona.BackupRestoreMode_BACKUP_RESTORE_MODE_EXACT:
		return node.ExtractModeExact
	default:
		return node.ExtractModeOverlay
	}
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

func validateBackupRestoreSettings(inst *Instance, gameServer *models.GameServer) error {
	if !gameServer.BackupsEnabled {
		return errBackupsDisabled
	}
	if !isGameServerOffline(inst, gameServer) {
		return errBackupRestoreRequiresOffline
	}

	return nil
}
