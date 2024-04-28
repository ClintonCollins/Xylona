package actions

import (
	"context"
	"errors"
	"io"
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
	log.Debug().Msgf("Path %s is local: %t", path, filepath.IsLocal(path))
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
		fileInfo, errFileInfo := file.Info()
		if errFileInfo != nil {
			log.Error().Err(errFileInfo).Msg("Failed to get file info")
			return nil, errFileInfo
		}
		xylonaFiles = append(xylonaFiles, &xylona.File{
			Name:         fileInfo.Name(),
			Size:         fileInfo.Size(),
			IsDirectory:  fileInfo.IsDir(),
			LastModified: timestamppb.New(fileInfo.ModTime()),
		})
	}

	return xylonaFiles, nil
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
