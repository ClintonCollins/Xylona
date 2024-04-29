package actions

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

var (
	ErrInvalidPath = errors.New("invalid path")
)

type Instance struct {
	ctx                context.Context
	supervisorInstance *supervisor.Instance
	db                 *db.Connection
}

func NewInstance(ctx context.Context, db *db.Connection, supervisorInstance *supervisor.Instance) *Instance {
	return &Instance{
		ctx:                ctx,
		supervisorInstance: supervisorInstance,
		db:                 db,
	}
}

func (inst *Instance) ListGameServerFiles(gameServer *models.GameServer, path string) ([]*xylona.File, error) {
	// Check if path is empty or if it is a local path. If it is not a local path, return an error.
	if path != "" && !filepath.IsLocal(path) {
		log.Error().Err(errors.New("invalid path"))
		return nil, ErrInvalidPath
	}
	fullPath := filepath.Join(gameServer.Directory, path)
	files, errReadDir := os.ReadDir(fullPath)
	if errReadDir != nil {
		if errors.Is(errReadDir, os.ErrNotExist) {
			log.Error().Err(errReadDir).Msg("Path does not exist")
			return nil, errReadDir
		}
		log.Error().Err(errReadDir).Msg("Failed to read directory")
		return nil, errReadDir
	}

	xylonaFiles := make([]*xylona.File, 0, len(files))
	for _, file := range files {
		file := file
		fileInfo, errFileInfo := file.Info()
		if errFileInfo != nil {
			log.Error().Err(errFileInfo).Msg("Failed to get file info")
			return nil, errFileInfo
		}
		size := fileInfo.Size()
		if fileInfo.IsDir() {
			// TODO walk directory recursively to get size of directory

			var totalSize int64 = 0

			err := filepath.Walk(filepath.Join(fullPath, file.Name()), func(path string, fileInfo os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !fileInfo.IsDir() {
					totalSize += fileInfo.Size()
				}
				return nil
			})

			if err != nil {
				log.Error().Err(err).Msg("Failed to walk through directory")
				return nil, err
			}
			size = totalSize
		}
		xylonaFiles = append(xylonaFiles, &xylona.File{
			Name:         fileInfo.Name(),
			Size:         size,
			IsDirectory:  fileInfo.IsDir(),
			LastModified: timestamppb.New(fileInfo.ModTime()),
		})
	}

	return xylonaFiles, nil
}

func (inst *Instance) DownloadGameServerFiles(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Error parsing multipart form", http.StatusBadRequest)
		return
	}

	gameServerID := r.PostForm.Get("gameServerId")

	paths := r.MultipartForm.Value["path"]
	files := r.MultipartForm.File["file"]

	if len(paths) != len(files) {
		log.Warn().Str("game server ID", gameServerID).Msg("Number of paths and files do not match")
		http.Error(w, "Number of paths and files do not match", http.StatusBadRequest)
		return
	}

	gameServer, errGetGameServer := inst.db.GetGameServerByID(gameServerID)
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			log.Error().Err(errGetGameServer).Msg("Game server not found")
			http.Error(w, "Game server not found", http.StatusNotFound)
			return
		}
		log.Error().Err(errGetGameServer).Msg("Failed to get game server")
		http.Error(w, "Failed to get game server", http.StatusInternalServerError)
		return
	}

	for i, file := range files {
		path := paths[i]
		errDownload := inst.downloadGameServerFile(gameServer, path, file)
		if errDownload != nil {
			log.Error().Err(errDownload).Msg("Failed to download file")
			http.Error(w, "Failed to download file", http.StatusInternalServerError)
			return
		}
	}
}

func (inst *Instance) downloadGameServerFile(gameServer *models.GameServer, path string, fileHeader *multipart.FileHeader) error {
	// Check if path is empty or if it is a local path. If it is not a local path, return an error.
	if path != "" && !filepath.IsLocal(path) {
		log.Error().Err(errors.New("invalid path"))
		return ErrInvalidPath
	}

	fileSource, errOpenFileSource := fileHeader.Open()
	if errOpenFileSource != nil {
		log.Error().Err(errOpenFileSource).Msg("Failed to open file source")
		return errOpenFileSource
	}
	defer func() {
		_ = fileSource.Close()
	}()

	gameServerDirPlusPath := filepath.Join(gameServer.Directory, path)
	errMkdirAll := os.MkdirAll(gameServerDirPlusPath, os.ModePerm)
	if errMkdirAll != nil {
		log.Error().Err(errMkdirAll).Msg("Failed to create directory")
		return errMkdirAll
	}

	fullPath := filepath.Join(gameServer.Directory, path, fileHeader.Filename)
	file, errReadFile := os.Create(fullPath)
	if errReadFile != nil {
		if errors.Is(errReadFile, os.ErrNotExist) {
			log.Error().Err(errReadFile).Msg("File does not exist")
			return errReadFile
		}
		log.Error().Err(errReadFile).Msg("Failed to read file")
		return errReadFile
	}
	defer func() {
		_ = file.Close()
	}()

	written, errCopy := io.Copy(file, fileSource)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy file")
		return errCopy
	}

	log.Debug().Msgf("Wrote %d bytes to %s", written, fullPath)

	return nil
}

func (inst *Instance) GetGameServerFile(gameServer *models.GameServer, path string, writer io.Writer) error {
	// Check if path is empty or if it is a local path. If it is not a local path, return an error.
	if path != "" && !filepath.IsLocal(path) {
		log.Error().Err(errors.New("invalid path"))
		return ErrInvalidPath
	}
	log.Debug().Msgf("Path %s is local: %t", path, filepath.IsLocal(path))
	fullPath := filepath.Join(gameServer.Directory, path)
	file, errReadFile := os.Open(fullPath)
	if errReadFile != nil {
		if errors.Is(errReadFile, os.ErrNotExist) {
			log.Error().Err(errReadFile).Msg("File does not exist")
			return errReadFile
		}
		log.Error().Err(errReadFile).Msg("Failed to read file")
		return errReadFile
	}

	_, errCopy := io.Copy(writer, file)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy file")
		return errCopy
	}
	return nil
}
