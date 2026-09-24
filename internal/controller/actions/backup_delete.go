package actions

import (
	"errors"
	"fmt"
	"slices"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// ErrBackupNotDeletable reports a recorded backup whose archive cannot be
// deleted through the server's current node, such as one left on a node the
// server has since moved away from.
var ErrBackupNotDeletable = errors.New("backup cannot be deleted")

// CheckGameServerBackupsDeletable reports whether every backup recorded for the
// server can be deleted, so a removal can refuse before it stops the server.
func (inst *Instance) CheckGameServerBackupsDeletable(gameServer *models.GameServer) error {
	_, errBackups := inst.deletableGameServerBackups(gameServer)
	return errBackups
}

// deletableGameServerBackups lists the server's recorded backups and checks each
// archive path, so one bad row fails before any archive is deleted.
func (inst *Instance) deletableGameServerBackups(gameServer *models.GameServer) ([]*models.GameServerBackup, error) {
	backups, errList := listGameServerBackupsByGameServerID(inst.db, gameServer.ID)
	if errList != nil {
		return nil, fmt.Errorf("actions: list backups to delete: %w", errList)
	}
	for _, backup := range backups {
		_, errPath := inst.resolveValidatedBackupArchivePath(gameServer, backup)
		if errPath != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrBackupNotDeletable, remotePathBase(backup.ArchivePath), errPath)
		}
	}
	return backups, nil
}

// deleteAllGameServerBackups deletes every backup recorded for the server, only
// ever touching each backup's own archive file.
func (inst *Instance) deleteAllGameServerBackups(gameServer *models.GameServer) error {
	backups, errBackups := inst.deletableGameServerBackups(gameServer)
	if errBackups != nil {
		return errBackups
	}
	for deleted, backup := range backups {
		errDelete := inst.DeleteGameServerBackup(gameServer, backup)
		if errDelete != nil {
			return fmt.Errorf(
				"actions: delete backup %s (%d of %d already deleted): %w",
				remotePathBase(backup.ArchivePath), deleted, len(backups), errDelete,
			)
		}
	}
	return nil
}

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
