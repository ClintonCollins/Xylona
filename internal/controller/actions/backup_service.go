package actions

import (
	"errors"
	"time"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type backupArchiveProgressFunc func(sizeBytes int64)

const (
	// DefaultScheduledBackupRetention is the scheduled-backup keep count used
	// when a server has no explicit retention configured.
	DefaultScheduledBackupRetention = 5
	maxBackupArchiveResolveAttempts = 1000
	maxBackupExtractEntrySizeBytes  = 8 << 30
	maxManualBackupNameLength       = 80
	backupProgressUpdateInterval    = 750 * time.Millisecond
)

var (
	errBackupsDisabled              = errors.New("backups are disabled for this game server")
	errBackupDirectoryNotConfigured = errors.New("backup directory is not configured for this game server")
	errBackupDirectoryInsideServer  = errors.New("backup directory cannot be inside the game server directory")
	errBackupCreateCancelled        = errors.New("backup creation cancelled")
	errBackupRestoreRequiresOffline = errors.New("game server must be offline to restore a backup")
	errBackupNodeMismatch           = errors.New("backup belongs to a different node")
	errBackupNotCompleted           = errors.New("backup is not completed")
	errUnsupportedBackupArchive     = errors.New("unsupported backup archive format")
	errInvalidUploadedBackupArchive = errors.New("backup archive is invalid")
	errInvalidBackupArchivePath     = errors.New("invalid backup archive path")
	// ErrInvalidManualBackupName reports a manual backup name that cannot be
	// safely turned into an archive filename.
	ErrInvalidManualBackupName = errors.New("backup name is invalid")
	// ErrManualBackupNameAlreadyExists reports that a manual backup name maps to
	// an archive filename that already exists for the server.
	ErrManualBackupNameAlreadyExists = errors.New("backup name already exists")
	backupRestoreUserFacingErrors    = []error{
		errBackupsDisabled,
		errBackupRestoreRequiresOffline,
		errBackupNodeMismatch,
		errBackupNotCompleted,
		errUnsupportedBackupArchive,
		errInvalidBackupArchivePath,
		errBackupDirectoryInsideServer,
		node.ErrRestoreDestinationSymlink,
	}
)

var (
	createGameServerBackupRow = func(conn *db.Connection, params db.CreateGameServerBackupParams) (*models.GameServerBackup, error) {
		return conn.CreateGameServerBackup(params)
	}
	backupNowFunc                = func() time.Time { return time.Now().UTC() }
	updateGameServerBackupResult = func(
		conn *db.Connection,
		backupID string,
		params db.UpdateGameServerBackupResultParams,
	) (*models.GameServerBackup, error) {
		return conn.UpdateGameServerBackupResult(backupID, params)
	}
	updateGameServerBackupProgress = func(conn *db.Connection, backupID string, sizeBytes int64) (*models.GameServerBackup, error) {
		return conn.UpdateGameServerBackupProgress(backupID, sizeBytes)
	}
	listGameServerBackupsByGameServerID = func(conn *db.Connection, gameServerID string) ([]*models.GameServerBackup, error) {
		return conn.ListGameServerBackupsByGameServerID(gameServerID)
	}
	deleteGameServerBackupRow = func(conn *db.Connection, backupID string) error {
		return conn.DeleteGameServerBackup(backupID)
	}
)

func (inst *Instance) isRemoteGameServer(gameServer *models.GameServer) bool {
	if inst == nil || gameServer == nil || inst.nodeRegistry == nil {
		return false
	}
	selfID := inst.nodeRegistry.SelfID()
	if selfID == "" {
		return false
	}
	return gameServer.NodeID != selfID
}
