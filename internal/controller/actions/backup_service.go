package actions

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
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

type uploadedBackupImportSource struct {
	validateArchive           func() error
	archiveSize               func() (int64, error)
	storeArchive              func(archivePath string) error
	ensureNamedLocalCollision bool
}

// ImportUploadedBackup validates and imports an uploaded zip archive into the managed backup catalog.
func (inst *Instance) ImportUploadedBackup(
	gameServer *models.GameServer,
	createdBy string,
	uploadedArchivePath string,
	originalFilename string,
) (*models.GameServerBackup, error) {
	return inst.importUploadedBackup(gameServer, createdBy, originalFilename, uploadedBackupImportSource{
		validateArchive: func() error {
			return validateUploadedBackupArchive(uploadedArchivePath)
		},
		archiveSize: func() (int64, error) {
			archiveInfo, errStat := os.Stat(uploadedArchivePath)
			if errStat != nil {
				return 0, fmt.Errorf("actions: stat uploaded backup archive: %w", errStat)
			}
			return archiveInfo.Size(), nil
		},
		storeArchive: func(archivePath string) error {
			return inst.storeUploadedBackupArchive(gameServer, uploadedArchivePath, archivePath)
		},
		ensureNamedLocalCollision: true,
	})
}

// ImportUploadedBackupBytes validates and imports an uploaded zip archive already held in memory.
func (inst *Instance) ImportUploadedBackupBytes(
	gameServer *models.GameServer,
	createdBy string,
	uploadedArchive []byte,
	originalFilename string,
) (*models.GameServerBackup, error) {
	return inst.importUploadedBackup(gameServer, createdBy, originalFilename, uploadedBackupImportSource{
		validateArchive: func() error {
			return validateUploadedBackupArchiveBytes(uploadedArchive)
		},
		archiveSize: func() (int64, error) {
			return int64(len(uploadedArchive)), nil
		},
		storeArchive: func(archivePath string) error {
			return inst.storeUploadedBackupArchiveBytes(gameServer, uploadedArchive, archivePath)
		},
	})
}

func (inst *Instance) importUploadedBackup(
	gameServer *models.GameServer,
	createdBy string,
	originalFilename string,
	source uploadedBackupImportSource,
) (*models.GameServerBackup, error) {
	backupDirectory, errValidate := inst.validateBackupCreateSettings(gameServer)
	if errValidate != nil {
		return nil, errValidate
	}

	fileExtension := filepath.Ext(strings.TrimSpace(originalFilename))
	if !strings.EqualFold(fileExtension, ".zip") {
		return nil, errUnsupportedBackupArchive
	}

	errValidateArchive := source.validateArchive()
	if errValidateArchive != nil {
		return nil, errValidateArchive
	}

	archiveBaseName := uploadedBackupArchiveBaseName(originalFilename, fileExtension)

	now := backupNowFunc().UTC()
	archivePath, errArchivePath := inst.resolveUniqueBackupArchivePath(
		gameServer,
		backupDirectory,
		now,
		"manual",
		archiveBaseName,
	)
	if errArchivePath != nil {
		return nil, fmt.Errorf("actions: resolve imported backup archive path: %w", errArchivePath)
	}
	if source.ensureNamedLocalCollision && archiveBaseName != "" && !inst.isRemoteGameServer(gameServer) {
		errEnsurePath := ensureBackupArchivePathAvailable(inst.db, gameServer.ID, archivePath)
		if errEnsurePath != nil {
			return nil, errEnsurePath
		}
	}

	archiveSize, errSize := source.archiveSize()
	if errSize != nil {
		return nil, errSize
	}

	errStore := source.storeArchive(archivePath)
	if errStore != nil {
		return nil, errStore
	}

	return inst.createImportedBackupRow(gameServer, createdBy, backupDirectory, archivePath, now, archiveSize)
}

func uploadedBackupArchiveBaseName(originalFilename string, fileExtension string) string {
	archiveBaseName := strings.TrimSpace(strings.TrimSuffix(filepath.Base(originalFilename), fileExtension))
	errValidateName := ValidateManualBackupName(archiveBaseName)
	if errValidateName != nil {
		return ""
	}
	return archiveBaseName
}

func (inst *Instance) createImportedBackupRow(
	gameServer *models.GameServer,
	createdBy string,
	backupDirectory string,
	archivePath string,
	now time.Time,
	sizeBytes int64,
) (*models.GameServerBackup, error) {
	completedAt := now
	backup, errCreateRow := createGameServerBackupRow(inst.db, db.CreateGameServerBackupParams{
		GameServerID:    gameServer.ID,
		NodeID:          gameServer.NodeID,
		CreatedBy:       createdBy,
		TriggerSource:   "manual",
		ArchivePath:     archivePath,
		ArchiveRoot:     backupDirectory,
		ArchiveFormat:   "zip",
		Status:          "completed",
		SizeBytes:       sizeBytes,
		RetentionExempt: true,
		CreatedAt:       now,
		CompletedAt:     &completedAt,
	})
	if errCreateRow != nil {
		errRemoveArchive := inst.removeBackupArchive(gameServer, archivePath)
		if errRemoveArchive != nil {
			return nil, errors.Join(
				fmt.Errorf("actions: create imported backup row: %w", errCreateRow),
				fmt.Errorf("actions: remove imported backup archive after row failure: %w", errRemoveArchive),
			)
		}
		return nil, fmt.Errorf("actions: create imported backup row: %w", errCreateRow)
	}

	return backup, nil
}

func (inst *Instance) storeUploadedBackupArchive(gameServer *models.GameServer, uploadedArchivePath string, archivePath string) error {
	if inst.isRemoteGameServer(gameServer) {
		return inst.storeUploadedBackupArchiveRemote(gameServer, uploadedArchivePath, archivePath)
	}

	parentDir := filepath.Dir(archivePath)
	errMkdir := os.MkdirAll(parentDir, 0o750)
	if errMkdir != nil {
		return fmt.Errorf("actions: create imported backup directory: %w", errMkdir)
	}

	errMove := moveUploadedBackupArchive(uploadedArchivePath, archivePath)
	if errMove != nil {
		return fmt.Errorf("actions: move uploaded backup archive: %w", errMove)
	}
	return nil
}

func (inst *Instance) storeUploadedBackupArchiveBytes(gameServer *models.GameServer, archiveBytes []byte, archivePath string) error {
	if inst.isRemoteGameServer(gameServer) {
		return inst.storeUploadedBackupArchiveRemoteBytes(gameServer, archiveBytes, archivePath)
	}
	return errors.New("actions: byte-based uploaded backup import is only supported for remote game servers")
}

func (inst *Instance) storeUploadedBackupArchiveRemote(gameServer *models.GameServer, uploadedArchivePath string, archivePath string) error {
	archiveFile, errOpen := os.Open(uploadedArchivePath)
	if errOpen != nil {
		return fmt.Errorf("actions: open uploaded backup archive for remote import: %w", errOpen)
	}

	errStore := inst.storeUploadedBackupArchiveRemoteReader(gameServer, archiveFile, archivePath)
	errClose := archiveFile.Close()
	if errStore != nil {
		if errClose != nil {
			return errors.Join(errStore, fmt.Errorf("actions: close uploaded backup archive after remote import: %w", errClose))
		}
		return errStore
	}
	if errClose != nil {
		return fmt.Errorf("actions: close uploaded backup archive after remote import: %w", errClose)
	}

	errRemove := os.Remove(uploadedArchivePath)
	if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
		return fmt.Errorf("actions: remove uploaded backup archive after remote import: %w", errRemove)
	}
	return nil
}

func (inst *Instance) storeUploadedBackupArchiveRemoteBytes(gameServer *models.GameServer, archiveBytes []byte, archivePath string) error {
	return inst.storeUploadedBackupArchiveRemoteReader(gameServer, bytes.NewReader(archiveBytes), archivePath)
}

func (inst *Instance) storeUploadedBackupArchiveRemoteReader(gameServer *models.GameServer, archiveReader io.Reader, archivePath string) error {
	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		return fmt.Errorf("actions: resolve node client for uploaded backup import: %w", errClient)
	}

	archiveDir := remotePathDir(archivePath)
	errCreateDir := client.CreateFileOrDirectory(inst.ctx, gameServer.BackupDirectory, gameServer.ID, "", true, node.ProtectionPolicy{})
	if errCreateDir != nil {
		return fmt.Errorf("actions: create remote imported backup directory: %w", errCreateDir)
	}

	archiveName := remotePathBase(archivePath)
	entries, errList := client.ListFiles(inst.ctx, archiveDir, "")
	if errList != nil {
		return fmt.Errorf("actions: list remote imported backup directory: %w", errList)
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name, archiveName) {
			return ErrManualBackupNameAlreadyExists
		}
	}

	_, errWrite := client.StreamWriteFile(inst.ctx, archiveDir, archiveName, archiveReader, node.ProtectionPolicy{})
	if errWrite != nil {
		errDelete := inst.removeBackupArchive(gameServer, archivePath)
		if errDelete != nil {
			return errors.Join(
				fmt.Errorf("actions: write remote imported backup archive: %w", errWrite),
				fmt.Errorf("actions: remove partial remote imported backup archive: %w", errDelete),
			)
		}
		return fmt.Errorf("actions: write remote imported backup archive: %w", errWrite)
	}

	return nil
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

// ValidateGameServerBackupDirectory checks whether the configured backup directory is usable for the server.
func ValidateGameServerBackupDirectory(gameServer *models.GameServer) error {
	_, errValidate := resolveValidGameServerBackupDirectory(gameServer)
	return errValidate
}

// ValidateRemoteGameServerBackupDirectory checks a node-native backup directory
// without applying controller-host filepath semantics.
func ValidateRemoteGameServerBackupDirectory(gameServer *models.GameServer) error {
	_, errValidate := resolveValidRemoteGameServerBackupDirectory(gameServer)
	return errValidate
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

func validateBackupRestoreSettings(inst *Instance, gameServer *models.GameServer) error {
	if !gameServer.BackupsEnabled {
		return errBackupsDisabled
	}
	if !isGameServerOffline(inst, gameServer) {
		return errBackupRestoreRequiresOffline
	}

	return nil
}

// ResolveManagedBackupArchivePath resolves and validates a backup archive path for callers outside this package.
func ResolveManagedBackupArchivePath(gameServer *models.GameServer, backup *models.GameServerBackup) (string, error) {
	return resolveValidatedBackupArchivePath(gameServer, backup)
}

// ResolveManagedRemoteBackupArchivePath resolves and validates a node-native backup archive path.
func ResolveManagedRemoteBackupArchivePath(gameServer *models.GameServer, backup *models.GameServerBackup) (string, error) {
	return resolveValidatedRemoteBackupArchivePath(gameServer, backup)
}

func (inst *Instance) resolveValidatedBackupArchivePath(gameServer *models.GameServer, backup *models.GameServerBackup) (string, error) {
	if inst.isRemoteGameServer(gameServer) {
		return resolveValidatedRemoteBackupArchivePath(gameServer, backup)
	}
	return resolveValidatedBackupArchivePath(gameServer, backup)
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

	return strings.TrimSpace(backup.ArchivePath), nil
}

func resolveValidatedRemoteBackupArchivePath(gameServer *models.GameServer, backup *models.GameServerBackup) (string, error) {
	if backup == nil {
		return "", errInvalidBackupArchivePath
	}
	if backup.NodeID != gameServer.NodeID {
		return "", errBackupNodeMismatch
	}

	archivePath := strings.TrimSpace(backup.ArchivePath)
	archiveRoot := strings.TrimSpace(backup.ArchiveRoot)
	if archivePath == "" || archiveRoot == "" {
		return "", errInvalidBackupArchivePath
	}
	if !strings.EqualFold(path.Ext(strings.ReplaceAll(archivePath, `\`, "/")), ".zip") {
		return "", errInvalidBackupArchivePath
	}

	archiveDirectory := buildBackupArchiveDirectory(archiveRoot, gameServer.ID)
	if !remotePathWithinOrEqual(archiveDirectory, archivePath) {
		return "", errInvalidBackupArchivePath
	}
	if remotePathWithinOrEqual(gameServer.Directory, archiveDirectory) {
		return "", errBackupDirectoryInsideServer
	}

	return archivePath, nil
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

func resolveValidRemoteGameServerBackupDirectory(gameServer *models.GameServer) (string, error) {
	backupDirectory := strings.TrimSpace(gameServer.BackupDirectory)
	if backupDirectory == "" {
		return "", errBackupDirectoryNotConfigured
	}
	archiveDirectory := buildBackupArchiveDirectory(backupDirectory, gameServer.ID)
	if remotePathWithinOrEqual(gameServer.Directory, archiveDirectory) {
		return "", errBackupDirectoryInsideServer
	}

	return backupDirectory, nil
}

func isGameServerOffline(inst *Instance, gameServer *models.GameServer) bool {
	if inst != nil {
		return inst.currentProcessStatus(gameServer) == xylona.Status_OFFLINE
	}

	return strings.EqualFold(gameServer.Status, xylona.Status_OFFLINE.String())
}

func buildBackupArchivePath(
	backupDirectory string,
	gameServerID string,
	now time.Time,
	triggerSource string,
	backupName string,
) (string, error) {
	fileName, errFileName := buildBackupArchiveFileName(now, triggerSource, backupName)
	if errFileName != nil {
		return "", errFileName
	}

	return joinRemotePath(buildBackupArchiveDirectory(backupDirectory, gameServerID), fileName), nil
}

func buildBackupArchiveFileName(now time.Time, triggerSource string, backupName string) (string, error) {
	timestamp := now.UTC().Format("20060102T150405.000000000Z")
	normalizedName, errNormalize := normalizeManualBackupName(backupName)
	if errNormalize != nil {
		return "", errNormalize
	}
	if normalizedName == "" {
		return timestamp + "-" + triggerSource + ".zip", nil
	}

	if strings.EqualFold(filepath.Ext(normalizedName), ".zip") {
		return normalizedName, nil
	}

	return normalizedName + ".zip", nil
}

func buildBackupArchiveDirectory(backupDirectory string, gameServerID string) string {
	return joinRemotePath(backupDirectory, gameServerID)
}

func resolveUniqueBackupArchivePath(
	backupDirectory string,
	gameServerID string,
	now time.Time,
	triggerSource string,
	backupName string,
) (string, error) {
	basePath, errBasePath := buildBackupArchivePath(backupDirectory, gameServerID, now, triggerSource, backupName)
	if errBasePath != nil {
		return "", errBasePath
	}
	if strings.TrimSpace(backupName) != "" {
		_, errStat := os.Stat(basePath)
		if errors.Is(errStat, os.ErrNotExist) {
			return basePath, nil
		}
		if errStat != nil {
			return "", fmt.Errorf("stat backup archive candidate: %w", errStat)
		}

		return "", ErrManualBackupNameAlreadyExists
	}

	baseDirectory := remotePathDir(basePath)
	baseName := strings.TrimSuffix(remotePathBase(basePath), ".zip")

	for suffix := range maxBackupArchiveResolveAttempts {
		candidatePath := basePath
		if suffix > 0 {
			fileName := fmt.Sprintf("%s-%d.zip", baseName, suffix)
			candidatePath = joinRemotePath(baseDirectory, fileName)
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

func (inst *Instance) resolveUniqueBackupArchivePath(
	gameServer *models.GameServer,
	backupDirectory string,
	now time.Time,
	triggerSource string,
	backupName string,
) (string, error) {
	if inst.isRemoteGameServer(gameServer) {
		return resolveUniqueRemoteBackupArchivePath(inst.db, backupDirectory, gameServer.ID, now, triggerSource, backupName)
	}
	return resolveUniqueBackupArchivePath(backupDirectory, gameServer.ID, now, triggerSource, backupName)
}

func resolveUniqueRemoteBackupArchivePath(
	conn *db.Connection,
	backupDirectory string,
	gameServerID string,
	now time.Time,
	triggerSource string,
	backupName string,
) (string, error) {
	basePath, errBasePath := buildBackupArchivePath(backupDirectory, gameServerID, now, triggerSource, backupName)
	if errBasePath != nil {
		return "", errBasePath
	}

	backups, errList := listGameServerBackupsByGameServerID(conn, gameServerID)
	if errList != nil {
		return "", fmt.Errorf("actions: list backups while checking remote archive path: %w", errList)
	}

	if strings.TrimSpace(backupName) != "" {
		if remoteBackupArchivePathClaimed(backups, basePath) {
			return "", ErrManualBackupNameAlreadyExists
		}
		return basePath, nil
	}

	baseDirectory := remotePathDir(basePath)
	baseName := strings.TrimSuffix(remotePathBase(basePath), ".zip")
	for suffix := range maxBackupArchiveResolveAttempts {
		candidatePath := basePath
		if suffix > 0 {
			fileName := fmt.Sprintf("%s-%d.zip", baseName, suffix)
			candidatePath = joinRemotePath(baseDirectory, fileName)
		}
		if !remoteBackupArchivePathClaimed(backups, candidatePath) {
			return candidatePath, nil
		}
	}

	return "", fmt.Errorf("exhausted backup archive path candidates after %d attempts", maxBackupArchiveResolveAttempts)
}

func ensureBackupArchivePathAvailable(conn *db.Connection, gameServerID string, archivePath string) error {
	backups, errList := listGameServerBackupsByGameServerID(conn, gameServerID)
	if errList != nil {
		return fmt.Errorf("actions: list backups while checking archive path: %w", errList)
	}
	if backupArchivePathClaimed(backups, archivePath) {
		return ErrManualBackupNameAlreadyExists
	}

	return nil
}

func backupArchivePathClaimed(backups []*models.GameServerBackup, archivePath string) bool {
	if len(backups) == 0 {
		return false
	}

	resolvedArchivePath, errArchivePath := resolvePathForComparison(archivePath)
	if errArchivePath != nil {
		resolvedArchivePath = filepath.Clean(strings.TrimSpace(archivePath))
	}

	for _, backup := range backups {
		if backup == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(backup.Status), "failed") {
			continue
		}

		resolvedExistingPath, errExistingPath := resolvePathForComparison(backup.ArchivePath)
		if errExistingPath != nil {
			resolvedExistingPath = filepath.Clean(strings.TrimSpace(backup.ArchivePath))
		}
		if strings.EqualFold(resolvedArchivePath, resolvedExistingPath) {
			return true
		}
	}

	return false
}

func remoteBackupArchivePathClaimed(backups []*models.GameServerBackup, archivePath string) bool {
	if len(backups) == 0 {
		return false
	}

	resolvedArchivePath := normalizeRemoteComparablePath(archivePath)
	for _, backup := range backups {
		if backup == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(backup.Status), "failed") {
			continue
		}

		resolvedExistingPath := normalizeRemoteComparablePath(backup.ArchivePath)
		if strings.EqualFold(resolvedArchivePath, resolvedExistingPath) {
			return true
		}
	}

	return false
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

func validateUploadedBackupArchive(uploadedArchivePath string) error {
	archiveReader, errOpen := zip.OpenReader(uploadedArchivePath)
	if errOpen != nil {
		return errInvalidUploadedBackupArchive
	}
	defer func() {
		_ = archiveReader.Close()
	}()

	for _, file := range archiveReader.File {
		_, errValidate := validateBackupZipEntryPath(file.Name)
		if errValidate != nil {
			return errInvalidUploadedBackupArchive
		}
		if file.UncompressedSize64 > uint64(maxBackupExtractEntrySizeBytes) {
			return errInvalidUploadedBackupArchive
		}

		fileInfo := file.FileInfo()
		if fileInfo.IsDir() {
			continue
		}

		archiveFile, errOpenFile := file.Open()
		if errOpenFile != nil {
			return errInvalidUploadedBackupArchive
		}

		limitedArchiveReader := io.LimitReader(archiveFile, maxBackupExtractEntrySizeBytes+1)
		bytesCopied, errCopy := io.Copy(io.Discard, limitedArchiveReader)
		errCloseArchiveFile := archiveFile.Close()
		if bytesCopied > maxBackupExtractEntrySizeBytes || errCopy != nil || errCloseArchiveFile != nil {
			return errInvalidUploadedBackupArchive
		}
	}

	return nil
}

func validateUploadedBackupArchiveBytes(uploadedArchive []byte) error {
	reader := bytes.NewReader(uploadedArchive)
	archiveReader, errOpen := zip.NewReader(reader, int64(len(uploadedArchive)))
	if errOpen != nil {
		return errInvalidUploadedBackupArchive
	}
	return validateUploadedBackupArchiveFiles(archiveReader.File)
}

func validateUploadedBackupArchiveFiles(files []*zip.File) error {
	for _, file := range files {
		_, errValidate := validateBackupZipEntryPath(file.Name)
		if errValidate != nil {
			return errInvalidUploadedBackupArchive
		}
		if file.UncompressedSize64 > uint64(maxBackupExtractEntrySizeBytes) {
			return errInvalidUploadedBackupArchive
		}

		fileInfo := file.FileInfo()
		if fileInfo.IsDir() {
			continue
		}

		archiveFile, errOpenFile := file.Open()
		if errOpenFile != nil {
			return errInvalidUploadedBackupArchive
		}

		limitedArchiveReader := io.LimitReader(archiveFile, maxBackupExtractEntrySizeBytes+1)
		bytesCopied, errCopy := io.Copy(io.Discard, limitedArchiveReader)
		errCloseArchiveFile := archiveFile.Close()
		if bytesCopied > maxBackupExtractEntrySizeBytes || errCopy != nil || errCloseArchiveFile != nil {
			return errInvalidUploadedBackupArchive
		}
	}

	return nil
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
	client nodeclient.NodeClient,
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

func joinRemotePath(directory string, name string) string {
	separator := remotePathSeparator(directory)
	trimmedDirectory := strings.TrimRight(directory, `/\`)
	if trimmedDirectory == "" {
		return separator + name
	}
	return trimmedDirectory + separator + name
}

func remotePathSeparator(directory string) string {
	hasBackslash := strings.Contains(directory, `\`)
	hasSlash := strings.Contains(directory, "/")
	if hasBackslash && !hasSlash {
		return `\`
	}
	return "/"
}

func remotePathBase(pathValue string) string {
	trimmedPath := strings.TrimRight(pathValue, `/\`)
	index := strings.LastIndexAny(trimmedPath, `/\`)
	if index < 0 {
		return trimmedPath
	}
	return trimmedPath[index+1:]
}

func remotePathDir(pathValue string) string {
	trimmedPath := strings.TrimRight(pathValue, `/\`)
	index := strings.LastIndexAny(trimmedPath, `/\`)
	if index < 0 {
		return "."
	}
	if index == 0 {
		return trimmedPath[:1]
	}
	return trimmedPath[:index]
}

func remotePathWithinOrEqual(parentPath string, candidatePath string) bool {
	parent := normalizeRemoteComparablePath(parentPath)
	candidate := normalizeRemoteComparablePath(candidatePath)
	if parent == "" || candidate == "" {
		return false
	}
	if strings.EqualFold(parent, candidate) {
		return true
	}
	prefix := parent
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return strings.HasPrefix(strings.ToLower(candidate), strings.ToLower(prefix))
}

func normalizeRemoteComparablePath(pathValue string) string {
	slashPath := strings.ReplaceAll(strings.TrimSpace(pathValue), `\`, "/")
	cleanPath := path.Clean(slashPath)
	if cleanPath == "." {
		return ""
	}
	if cleanPath != "/" {
		cleanPath = strings.TrimRight(cleanPath, "/")
	}
	return cleanPath
}

type backupProgressReporter struct {
	inst           *Instance
	gameServerID   string
	gameServerName string
	backupID       string
	mu             sync.Mutex
	latestBytes    int64
	flushedBytes   int64
	closed         bool
	flushOnStop    bool
	stopCh         chan struct{}
	doneCh         chan struct{}
}

func newBackupProgressReporter(
	inst *Instance,
	gameServerID string,
	gameServerName string,
	backupID string,
) *backupProgressReporter {
	reporter := &backupProgressReporter{
		inst:           inst,
		gameServerID:   gameServerID,
		gameServerName: gameServerName,
		backupID:       backupID,
		stopCh:         make(chan struct{}),
		doneCh:         make(chan struct{}),
	}

	go reporter.run()

	return reporter
}

func (r *backupProgressReporter) Observe(sizeBytes int64) {
	if sizeBytes <= 0 {
		return
	}

	r.mu.Lock()
	if sizeBytes > r.latestBytes {
		r.latestBytes = sizeBytes
	}
	r.mu.Unlock()
}

func (r *backupProgressReporter) Close(finalSizeBytes int64) {
	r.shutdown(true, finalSizeBytes)
}

func (r *backupProgressReporter) Abort() {
	r.shutdown(false, 0)
}

func (r *backupProgressReporter) shutdown(flush bool, finalSizeBytes int64) {
	if r == nil {
		return
	}

	if flush {
		r.Observe(finalSizeBytes)
	}

	r.mu.Lock()
	if r.closed {
		doneCh := r.doneCh
		r.mu.Unlock()
		<-doneCh
		return
	}
	r.closed = true
	r.flushOnStop = flush
	close(r.stopCh)
	doneCh := r.doneCh
	r.mu.Unlock()

	<-doneCh
}

func (r *backupProgressReporter) run() {
	ticker := time.NewTicker(backupProgressUpdateInterval)
	defer ticker.Stop()
	defer close(r.doneCh)

	for {
		select {
		case <-ticker.C:
			r.flush()
		case <-r.stopCh:
			if r.shouldFlushOnStop() {
				r.flush()
			}
			return
		}
	}
}

func (r *backupProgressReporter) shouldFlushOnStop() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.flushOnStop
}

func (r *backupProgressReporter) flush() {
	r.mu.Lock()
	latestBytes := r.latestBytes
	if latestBytes <= r.flushedBytes {
		r.mu.Unlock()
		return
	}
	r.flushedBytes = latestBytes
	r.mu.Unlock()

	_, errUpdate := updateGameServerBackupProgress(r.inst.db, r.backupID, latestBytes)
	if errUpdate != nil {
		log.Error().
			Err(errUpdate).
			Str("backup_id", r.backupID).
			Msg("Failed to persist in-flight backup progress")
	}

	r.inst.broadcastBackupProgress(
		r.gameServerID,
		r.gameServerName,
		r.backupID,
		xylona.BackupProgressOperation_BACKUP_PROGRESS_OPERATION_CREATE,
		xylona.BackupProgressPhase_BACKUP_PROGRESS_PHASE_ARCHIVING,
		50,
		latestBytes,
		"Archiving game server files",
	)
}

func backupCreateCancelErr(backupCtx context.Context) error {
	if backupCtx == nil {
		return nil
	}

	errCtx := backupCtx.Err()
	if errCtx != nil {
		return errors.Join(errBackupCreateCancelled, errCtx)
	}

	return nil
}

func (inst *Instance) registerBackupCreate(backupID string) (context.Context, func()) {
	if inst == nil {
		return context.Background(), nil
	}

	backupCtx, cancelBackup := context.WithCancel(inst.ctx)
	call := &backupCreateCall{
		cancel: cancelBackup,
		done:   make(chan struct{}),
	}

	inst.backupCreateMu.Lock()
	inst.backupCreateCalls[backupID] = call
	inst.backupCreateMu.Unlock()

	return backupCtx, func() {
		cancelBackup()
		inst.backupCreateMu.Lock()
		delete(inst.backupCreateCalls, backupID)
		close(call.done)
		inst.backupCreateMu.Unlock()
	}
}

func (inst *Instance) cancelBackupCreate(backupID string) <-chan struct{} {
	if inst == nil {
		return nil
	}

	inst.backupCreateMu.Lock()
	call := inst.backupCreateCalls[backupID]
	inst.backupCreateMu.Unlock()
	if call == nil {
		return nil
	}

	call.cancel()

	return call.done
}

func moveUploadedBackupArchive(sourcePath string, destinationPath string) error {
	errRename := os.Rename(sourcePath, destinationPath)
	if errRename == nil {
		return nil
	}

	sourceFile, errOpenSource := os.Open(sourcePath)
	if errOpenSource != nil {
		return fmt.Errorf("open uploaded archive: %w", errOpenSource)
	}
	defer func() {
		_ = sourceFile.Close()
	}()

	destinationFile, errCreateDestination := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errCreateDestination != nil {
		return fmt.Errorf("create destination archive: %w", errCreateDestination)
	}

	_, errCopy := io.Copy(destinationFile, sourceFile)
	errCloseDestination := destinationFile.Close()
	if errCopy != nil {
		errRemoveDestination := os.Remove(destinationPath)
		if errRemoveDestination != nil && !errors.Is(errRemoveDestination, os.ErrNotExist) {
			return errors.Join(
				fmt.Errorf("copy uploaded archive: %w", errCopy),
				fmt.Errorf("remove partial destination archive: %w", errRemoveDestination),
			)
		}
		return fmt.Errorf("copy uploaded archive: %w", errCopy)
	}
	if errCloseDestination != nil {
		errRemoveDestination := os.Remove(destinationPath)
		if errRemoveDestination != nil && !errors.Is(errRemoveDestination, os.ErrNotExist) {
			return errors.Join(
				fmt.Errorf("close destination archive: %w", errCloseDestination),
				fmt.Errorf("remove destination archive after close failure: %w", errRemoveDestination),
			)
		}
		return fmt.Errorf("close destination archive: %w", errCloseDestination)
	}

	errRemoveSource := os.Remove(sourcePath)
	if errRemoveSource != nil {
		return fmt.Errorf("remove uploaded temp archive: %w", errRemoveSource)
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

func (inst *Instance) broadcastBackupProgress(
	gameServerID string,
	gameServerName string,
	backupID string,
	operation xylona.BackupProgressOperation,
	phase xylona.BackupProgressPhase,
	percent int32,
	sizeBytes int64,
	message string,
) {
	if inst == nil || inst.backupBroadcaster == nil {
		return
	}

	inst.backupBroadcaster.BroadcastBackupProgress(gameServerID, &xylona.BackupProgress{
		GameServerId:   gameServerID,
		GameServerName: gameServerName,
		BackupId:       backupID,
		Operation:      operation,
		Phase:          phase,
		Percent:        percent,
		SizeBytes:      sizeBytes,
		Message:        message,
	})
}
