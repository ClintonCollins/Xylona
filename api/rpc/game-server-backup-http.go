package rpc

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const maxBackupUploadFieldBytes = 10 << 10

var (
	errBackupUploadStagingFile = errors.New("failed to create backup upload staging file")
	errBackupUploadStore       = errors.New("failed to store backup upload")
)

// DownloadGameServerBackupArchive streams a managed backup archive to the browser.
func (xs *XylonaService) DownloadGameServerBackupArchive(w http.ResponseWriter, r *http.Request) {
	user, errUser := xs.getUserFromHeader(r.Header)
	if errUser != nil {
		writeBackupHTTPError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	gameServerIDEncoded := chi.URLParam(r, "gameServerId")
	backupIDEncoded := chi.URLParam(r, "backupId")
	gameServerID, errGameServerID := url.QueryUnescape(gameServerIDEncoded)
	backupID, errBackupID := url.QueryUnescape(backupIDEncoded)
	if errGameServerID != nil || errBackupID != nil {
		writeBackupHTTPError(w, http.StatusBadRequest, "game_server_id and backup_id are required")
		return
	}

	gameServer, errGetGameServer := xs.getGameServerForBackupRPC(gameServerID)
	if errGetGameServer != nil {
		writeBackupConnectError(w, errGetGameServer)
		return
	}

	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionBackup)
	if errPermission != nil {
		writeBackupConnectError(w, errPermission)
		return
	}

	if !isLocalBackupServer(gameServer) {
		writeBackupHTTPError(w, http.StatusPreconditionFailed, backupDisabledReasonLocalOnly)
		return
	}

	backup, errGetBackup := xs.getBackupByIDForGameServer(gameServerID, backupID)
	if errGetBackup != nil {
		writeBackupConnectError(w, errGetBackup)
		return
	}

	archivePath, errArchivePath := actions.ResolveManagedBackupArchivePath(gameServer, backup)
	if errArchivePath != nil {
		writeBackupHTTPError(w, http.StatusPreconditionFailed, errArchivePath.Error())
		return
	}

	//nolint:gosec // archivePath is resolved and confined to the managed backup directory above.
	archiveFile, errOpenArchive := os.Open(archivePath)
	if errOpenArchive != nil {
		if errors.Is(errOpenArchive, os.ErrNotExist) {
			writeBackupHTTPError(w, http.StatusNotFound, "backup archive not found")
			return
		}
		writeBackupHTTPError(w, http.StatusInternalServerError, "failed to open backup archive")
		return
	}
	defer func() {
		_ = archiveFile.Close()
	}()

	archiveInfo, errArchiveInfo := archiveFile.Stat()
	if errArchiveInfo != nil {
		writeBackupHTTPError(w, http.StatusInternalServerError, "failed to stat backup archive")
		return
	}

	downloadName := filepath.Base(archivePath)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
	http.ServeContent(w, r, downloadName, archiveInfo.ModTime(), archiveFile)
}

// UploadGameServerBackupArchive imports a managed backup archive from a multipart upload.
func (xs *XylonaService) UploadGameServerBackupArchive(w http.ResponseWriter, r *http.Request) {
	user, errUser := xs.getUserFromHeader(r.Header)
	if errUser != nil {
		writeBackupHTTPError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	multipartReader, errMultipartReader := r.MultipartReader()
	if errMultipartReader != nil {
		writeBackupHTTPError(w, http.StatusBadRequest, "invalid multipart upload")
		return
	}

	var gameServerLoaded bool
	var gameServerForUpload *models.GameServer
	var stagingDirectory string
	var importedBackup bool

	for {
		part, errNextPart := multipartReader.NextPart()
		if errNextPart == io.EOF {
			break
		}
		if errNextPart != nil {
			writeBackupHTTPError(w, http.StatusBadRequest, "failed to read multipart upload")
			return
		}

		switch part.FormName() {
		case "gameServerId":
			gameServerIDBytes, errReadGameServerID := io.ReadAll(io.LimitReader(part, maxBackupUploadFieldBytes))
			if errReadGameServerID != nil {
				writeBackupHTTPError(w, http.StatusBadRequest, "failed to read game server id")
				return
			}

			gameServerID := strings.TrimSpace(string(gameServerIDBytes))
			if gameServerID == "" {
				writeBackupHTTPError(w, http.StatusBadRequest, "game_server_id is required")
				return
			}

			gameServer, errPrepareUpload := xs.prepareBackupUpload(user, gameServerID)
			if errPrepareUpload != nil {
				writeBackupConnectError(w, errPrepareUpload)
				return
			}

			backupDirectory := strings.TrimSpace(gameServer.BackupDirectory)
			stagingDirectory = filepath.Join(backupDirectory, gameServer.ID)
			errMkdirStaging := os.MkdirAll(stagingDirectory, 0o750)
			if errMkdirStaging != nil {
				writeBackupHTTPError(w, http.StatusInternalServerError, "failed to create backup upload staging directory")
				return
			}

			gameServerForUpload = gameServer
			gameServerLoaded = true
		case "file":
			if !gameServerLoaded || gameServerForUpload == nil {
				writeBackupHTTPError(w, http.StatusBadRequest, "game_server_id must be provided before file upload")
				return
			}
			if importedBackup {
				writeBackupHTTPError(w, http.StatusBadRequest, "only one backup file may be uploaded at a time")
				return
			}
			if xs.actionsInst == nil {
				writeBackupHTTPError(w, http.StatusInternalServerError, "backup service unavailable")
				return
			}

			uploadedFilename := strings.TrimSpace(part.FileName())
			if uploadedFilename == "" {
				writeBackupHTTPError(w, http.StatusBadRequest, "backup file is required")
				return
			}

			errImportBackup := xs.importUploadedBackupPart(
				part,
				gameServerForUpload,
				user.ID,
				stagingDirectory,
				uploadedFilename,
			)
			if errImportBackup != nil {
				writeBackupImportError(w, errImportBackup)
				return
			}

			importedBackup = true
		}
	}

	if !gameServerLoaded {
		writeBackupHTTPError(w, http.StatusBadRequest, "game_server_id is required")
		return
	}
	if !importedBackup {
		writeBackupHTTPError(w, http.StatusBadRequest, "backup file is required")
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (xs *XylonaService) importUploadedBackupPart(
	part *multipart.Part,
	gameServer *models.GameServer,
	userID string,
	stagingDirectory string,
	uploadedFilename string,
) error {
	stagedFile, errCreateTemp := os.CreateTemp(stagingDirectory, "backup-upload-*.zip")
	if errCreateTemp != nil {
		return errBackupUploadStagingFile
	}
	stagedFilePath := stagedFile.Name()
	defer func() {
		_ = os.Remove(stagedFilePath)
	}()

	_, errCopyPart := io.Copy(stagedFile, part)
	errCloseStaged := stagedFile.Close()
	if errCopyPart != nil || errCloseStaged != nil {
		return errBackupUploadStore
	}

	_, errImportBackup := xs.actionsInst.ImportUploadedBackup(
		gameServer,
		userID,
		stagedFilePath,
		uploadedFilename,
	)
	if errImportBackup != nil {
		return fmt.Errorf("import uploaded backup: %w", errImportBackup)
	}

	return nil
}

func (xs *XylonaService) prepareBackupUpload(user *models.User, gameServerID string) (*models.GameServer, error) {
	gameServer, errGetGameServer := xs.getGameServerForBackupRPC(gameServerID)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}

	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionBackup)
	if errPermission != nil {
		return nil, errPermission
	}

	operationsAllowed, disabledReason := backupOperationsAllowed(gameServer)
	if !operationsAllowed {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New(disabledReason))
	}

	return gameServer, nil
}

func writeBackupImportError(w http.ResponseWriter, err error) {
	if errors.Is(err, errBackupUploadStagingFile) {
		writeBackupHTTPError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if errors.Is(err, errBackupUploadStore) {
		writeBackupHTTPError(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, actions.ErrInvalidManualBackupName) {
		writeBackupHTTPError(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, actions.ErrManualBackupNameAlreadyExists) {
		writeBackupHTTPError(w, http.StatusConflict, err.Error())
		return
	}
	if strings.Contains(err.Error(), "unsupported backup archive format") || strings.Contains(err.Error(), "backup archive is invalid") {
		writeBackupHTTPError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeBackupHTTPError(w, http.StatusInternalServerError, "failed to import backup archive")
}

func writeBackupConnectError(w http.ResponseWriter, err error) {
	statusCode := http.StatusInternalServerError
	switch connect.CodeOf(err) {
	case connect.CodeUnauthenticated:
		statusCode = http.StatusUnauthorized
	case connect.CodePermissionDenied:
		statusCode = http.StatusForbidden
	case connect.CodeNotFound:
		statusCode = http.StatusNotFound
	case connect.CodeFailedPrecondition:
		statusCode = http.StatusPreconditionFailed
	case connect.CodeInvalidArgument:
		statusCode = http.StatusBadRequest
	case connect.CodeAlreadyExists:
		statusCode = http.StatusConflict
	}

	writeBackupHTTPError(w, statusCode, err.Error())
}

func writeBackupHTTPError(w http.ResponseWriter, statusCode int, message string) {
	http.Error(w, message, statusCode)
}
