package actions

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/sql/models"
)

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
	return inst.runUploadedBackupImport(gameServer, createdBy, originalFilename, uploadedBackupImportSource{
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
	return inst.runUploadedBackupImport(gameServer, createdBy, originalFilename, uploadedBackupImportSource{
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

func (inst *Instance) runUploadedBackupImport(
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
