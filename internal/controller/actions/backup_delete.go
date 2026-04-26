package actions

import (
	"fmt"
	"slices"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// DeleteGameServerBackup deletes a backup row and its archive when it belongs to the current server node.
func (inst *Instance) DeleteGameServerBackup(gameServer *models.GameServer, backup *models.GameServerBackup) error {
	if backup == nil {
		return errInvalidBackupArchivePath
	}
	if backup.GameServerID != gameServer.ID {
		return fmt.Errorf("actions: backup does not belong to game server %s", gameServer.ID)
	}

	backupDone := inst.cancelBackupCreate(backup.ID)
	if backupDone != nil {
		<-backupDone
	}

	archivePath, errArchivePath := inst.resolveValidatedBackupArchivePath(gameServer, backup)
	if errArchivePath != nil {
		return errArchivePath
	}

	errRemoveArchive := inst.removeBackupArchive(gameServer, archivePath)
	if errRemoveArchive != nil {
		return fmt.Errorf("actions: remove backup archive before deleting backup row: %w", errRemoveArchive)
	}

	errDeleteRow := deleteGameServerBackupRow(inst.db, backup.ID)
	if errDeleteRow != nil {
		return fmt.Errorf("actions: delete backup row after archive cleanup: %w", errDeleteRow)
	}

	return nil
}

func (inst *Instance) removeBackupArchive(gameServer *models.GameServer, archivePath string) error {
	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		return fmt.Errorf("actions: resolve node client for backup archive removal: %w", errClient)
	}

	archiveDir := remotePathDir(archivePath)
	archiveName := remotePathBase(archivePath)
	deleted, errDelete := client.DeleteFiles(inst.ctx, archiveDir, []string{archiveName}, node.ProtectionPolicy{})
	if errDelete != nil {
		return fmt.Errorf("actions: delete backup archive on node: %w", errDelete)
	}
	if !slices.Contains(deleted, archiveName) {
		return fmt.Errorf("actions: delete backup archive on node: node did not confirm deletion of %q", archiveName)
	}
	return nil
}
