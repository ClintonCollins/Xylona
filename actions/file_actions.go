package actions

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/zip"
	"github.com/mholt/archiver/v4"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type progressWriter struct {
	io.Writer
	bytesWritten int64
	BytesChan    chan int64
	ctx          context.Context
}

func (pw *progressWriter) Write(p []byte) (n int, err error) {
	n, err = pw.Writer.Write(p)
	if err != nil {
		return
	}
	if n > 0 {
		pw.bytesWritten += int64(n)
		select {
		case pw.BytesChan <- pw.bytesWritten:
		default:
		}
	}
	return
}

type progressReader struct {
	io.Reader
	bytesRead int64
	BytesChan chan int64
	fileName  string
	ctx       context.Context
}

func (pr *progressReader) Close() error {
	if closer, ok := pr.Reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	n, err = pr.Reader.Read(p)
	if err != nil {
		return
	}
	if n > 0 {
		pr.bytesRead += int64(n)
		select {
		case pr.BytesChan <- int64(n):
		default:
		}
	}
	return
}

type archiveJobResult struct {
	file archiver.File
	err  error
}

func (inst *Instance) DeleteFiles(ctx context.Context, gameServer *models.GameServer, files []string) ([]string, error) {
	successfullyDeleted := make([]string, 0, len(files))
	for _, file := range files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			file = strings.TrimPrefix(file, "/")
			fileIsLocal := filepath.IsLocal(file)
			if !fileIsLocal {
				log.Error().Str("Game Server ID", gameServer.ID).Msg("Invalid path")
				return nil, ErrInvalidPath
			}
			fullPath := filepath.Join(gameServer.Directory, file)
			errRemove := os.RemoveAll(fullPath)
			if errRemove != nil {
				log.Error().Err(errRemove).Msg("Failed to remove file")
				continue
			}
			successfullyDeleted = append(successfullyDeleted, file)
		}
	}
	return successfullyDeleted, nil
}

func (inst *Instance) RenameFile(gameServer *models.GameServer, oldFilePath, newFilePath string) (string, error) {
	oldFilePath = strings.TrimPrefix(oldFilePath, "/")
	newFilePath = strings.TrimPrefix(newFilePath, "/")
	oldFilePathIsLocal := filepath.IsLocal(oldFilePath)
	newFilePathIsLocal := filepath.IsLocal(newFilePath)
	if !oldFilePathIsLocal || !newFilePathIsLocal {
		log.Error().Str("Game Server ID", gameServer.ID).Msg("Invalid path")
		return "", ErrInvalidPath
	}
	oldFullPath := filepath.Join(gameServer.Directory, oldFilePath)
	newFullPath := filepath.Join(gameServer.Directory, newFilePath)
	errRename := os.Rename(oldFullPath, newFullPath)
	if errRename != nil {
		log.Error().Err(errRename).Msg("Failed to rename file")
		return "", errRename
	}
	return newFilePath, nil
}

func (inst *Instance) MoveFiles(ctx context.Context, gameServer *models.GameServer, files []string, destination string) ([]string, error) {
	successfullyMoved := make([]string, 0, len(files))
	destination = strings.TrimPrefix(destination, "/")
	destinationIsLocal := filepath.IsLocal(destination)
	if !destinationIsLocal {
		log.Error().Str("Game Server ID", gameServer.ID).Msg("Invalid path")
		return nil, ErrInvalidPath
	}
	for _, file := range files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			// Strip everything except the filename.
			file = filepath.Base(file)
			file = strings.TrimPrefix(file, "/")
			fileIsLocal := filepath.IsLocal(file)
			if !fileIsLocal {
				log.Error().Str("Game Server ID", gameServer.ID).Msg("Invalid path")
				return nil, ErrInvalidPath
			}
			fullPath := filepath.Join(gameServer.Directory, file)
			destinationFullPath := filepath.Join(gameServer.Directory, destination, file)
			errRename := os.Rename(fullPath, destinationFullPath)
			if errRename != nil {
				log.Error().Err(errRename).Msg("Failed to move file")
				continue
			}
			successfullyMoved = append(successfullyMoved, file)
		}
	}
	return successfullyMoved, nil
}

func (inst *Instance) EditFile(gameServer *models.GameServer, filePath string, content string) error {
	filePath = strings.TrimPrefix(filePath, "/")
	filePathIsLocal := filepath.IsLocal(filePath)
	if !filePathIsLocal {
		log.Error().Str("Game Server ID", gameServer.ID).Msg("Invalid path")
		return ErrInvalidPath
	}
	fullPath := filepath.Join(gameServer.Directory, filePath)
	file, errOpen := os.Create(fullPath)
	if errOpen != nil {
		log.Error().Err(errOpen).Msg("Failed to open file")
		return errOpen
	}
	defer func() {
		_ = file.Close()
	}()
	_, errWrite := file.WriteString(content)
	if errWrite != nil {
		log.Error().Err(errWrite).Msg("Failed to write to file")
	}
	return errWrite
}

func (inst *Instance) DownloadFileFromURL(ctx context.Context, gameServer *models.GameServer, url, destinationDirectoryPath string) (string, error) {
	destinationDirectoryPath = strings.TrimPrefix(destinationDirectoryPath, "/")
	destinationDirectoryPathIsLocal := filepath.IsLocal(destinationDirectoryPath)
	if !destinationDirectoryPathIsLocal {
		log.Error().Str("Game Server ID", gameServer.ID).Msg("Invalid path")
		return "", ErrInvalidPath
	}

	httpClient := helpers.GetXylonaHTTPClient()
	req, errNewReq := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if errNewReq != nil {
		log.Error().Err(errNewReq).Msg("Failed to create request")
		return "", errNewReq
	}

	fileName := path.Base(req.URL.Path)
	fileName = strings.TrimPrefix(fileName, "/")
	filePathIsLocal := filepath.IsLocal(fileName)
	if !filePathIsLocal {
		log.Error().Str("Game Server ID", gameServer.ID).Msg("Invalid path")
		return "", ErrInvalidPath
	}

	resp, errGet := httpClient.Do(req)
	if errGet != nil {
		log.Error().Err(errGet).Msg("Failed to get URL")
		return "", errGet
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	destinationFullPath := filepath.Join(gameServer.Directory, destinationDirectoryPath, fileName)
	file, errCreate := os.Create(destinationFullPath)
	if errCreate != nil {
		log.Error().Err(errCreate).Msg("Failed to create file")
		return "", errCreate
	}
	defer func() {
		_ = file.Close()
	}()

	_, errCopy := io.Copy(file, resp.Body)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy file")
		return "", errCopy
	}
	return "", nil
}

func (inst *Instance) ArchiveFiles(ctx context.Context, gameServer *models.GameServer, fullArchivePath string, fullFilePaths []string,
	compression xylona.GameServerFilesCompressionType,
	xylonaFileArchiveProgressChan chan xylona.GameServerFilesArchiveProgress,
) (xylona.GameServerFilesArchiveProgress, error) {

	archiveFullPath := filepath.Join(gameServer.Directory, fullArchivePath)

	pathFilesMap := make(map[string]string)
	for _, f := range fullFilePaths {
		absPath, errAbsPath := filepath.Abs(filepath.Join(gameServer.Directory, f))
		if errAbsPath != nil {
			log.Error().Err(errAbsPath).Msg("Failed to get absolute path")
			return xylona.GameServerFilesArchiveProgress{}, errAbsPath
		}
		pathFilesMap[absPath] = filepath.Base(f)
	}

	archiverDiskOptions := &archiver.FromDiskOptions{
		FollowSymlinks:  false,
		ClearAttributes: false,
	}

	archiverFiles, errFilesFromPaths := archiver.FilesFromDisk(archiverDiskOptions, pathFilesMap)
	if errFilesFromPaths != nil {
		log.Error().Err(errFilesFromPaths).Msg("Failed to get files from paths")
		return xylona.GameServerFilesArchiveProgress{}, errFilesFromPaths
	}

	return inst.archiveFilesWithProgress(ctx, archiveFullPath, archiverFiles, compression, xylonaFileArchiveProgressChan)
}

func createXylonaArchiveResult(totalFiles, filesCompressedSoFar, totalBytes, bytesCompressedSoFar int64,
	currentFile string,
) xylona.GameServerFilesArchiveProgress {
	return xylona.GameServerFilesArchiveProgress{
		TotalFiles:      totalFiles,
		FilesCompressed: filesCompressedSoFar,
		TotalBytes:      totalBytes,
		BytesCompressed: bytesCompressedSoFar,
		CurrentFile:     currentFile,
	}
}

func (inst *Instance) attemptToSendOnXylonaProgressChan(ctx context.Context,
	xylonaProgress xylona.GameServerFilesArchiveProgress, progressChan chan xylona.GameServerFilesArchiveProgress,
) {
	timeout := time.After(time.Millisecond * 50)
	select {
	case <-ctx.Done():
		return
	case <-inst.ctx.Done():
		return
	case progressChan <- xylonaProgress:
	case <-timeout:
		log.Warn().Msg("Failed to send progress")
		return
	}
}

func (inst *Instance) archiveFilesWithProgress(ctx context.Context, archiveFullPath string,
	archiverFiles []archiver.File, compression xylona.GameServerFilesCompressionType,
	xylonaFileArchiveProgressChan chan xylona.GameServerFilesArchiveProgress,
) (xylona.GameServerFilesArchiveProgress, error) {

	format := archiver.CompressedArchive{}
	// Default to tar if no compression is specified.
	format.Archival = &archiver.Tar{}

	switch compression {
	case xylona.GameServerFilesCompressionType_ZIP:
		format.Archival = &archiver.Zip{
			SelectiveCompression: true,
			Compression:          zip.Deflate,
			ContinueOnError:      false,
			TextEncoding:         "",
		}
	case xylona.GameServerFilesCompressionType_GZIP:
		format.Compression = &archiver.Gz{
			CompressionLevel: gzip.DefaultCompression,
			Multithreaded:    true,
		}
	case xylona.GameServerFilesCompressionType_BZIP2:
		format.Compression = &archiver.Bz2{CompressionLevel: bzip2.DefaultCompression}
	case xylona.GameServerFilesCompressionType_ZST:
		format.Compression = &archiver.Zstd{}
	case xylona.GameServerFilesCompressionType_XZ:
		format.Compression = &archiver.Xz{}
	}

	archiveFullPath = archiveFullPath + format.Name()

	archiveOut, errCreate := os.Create(archiveFullPath)
	if errCreate != nil {
		log.Error().Err(errCreate).Msg("Failed to create archive")
		return xylona.GameServerFilesArchiveProgress{}, errCreate
	}
	defer func() {
		_ = archiveOut.Close()
	}()

	totalBytes := int64(0)
	totalFiles := int64(len(archiverFiles))
	filesCompressedSoFar := int64(0)
	bytesReadSoFar := int64(0)
	currentFile := ""

	for _, f := range archiverFiles {
		totalBytes += f.Size()
	}

	archiveJobs := make(chan archiver.ArchiveAsyncJob, len(archiverFiles))
	errChan := make(chan error, 1)
	fileResultChan := make(chan archiveJobResult)
	readBytesChan := make(chan int64)

	go func() {
		ticker := time.NewTicker(time.Millisecond * 250)
		debugTicker := time.NewTicker(time.Second * 3)
		defer ticker.Stop()
		defer debugTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-inst.ctx.Done():
				return
			case bytesRead := <-readBytesChan:
				bytesReadSoFar += bytesRead
			case <-debugTicker.C:
				log.Debug().Fields(map[string]interface{}{
					"Total Files":      totalFiles,
					"Files Compressed": filesCompressedSoFar,
					"Total Bytes":      totalBytes,
					"Bytes Read":       bytesReadSoFar,
					"Current File":     currentFile,
					"Archive Path":     archiveFullPath,
				}).Msg("Archive in progress")
			case <-ticker.C:
				xylonaProgress := createXylonaArchiveResult(totalFiles, filesCompressedSoFar, totalBytes,
					bytesReadSoFar, currentFile)
				inst.attemptToSendOnXylonaProgressChan(ctx, xylonaProgress, xylonaFileArchiveProgressChan)
			case <-fileResultChan:
				filesCompressedSoFar += 1
				xylonaProgress := createXylonaArchiveResult(totalFiles, filesCompressedSoFar, totalBytes,
					bytesReadSoFar, currentFile)
				inst.attemptToSendOnXylonaProgressChan(ctx, xylonaProgress, xylonaFileArchiveProgressChan)
				if filesCompressedSoFar == totalFiles {
					return
				}
			}
		}

	}()

	go func() {
		for _, f := range archiverFiles {
			select {
			case <-ctx.Done():
				close(archiveJobs)
				return
			case <-inst.ctx.Done():
				close(archiveJobs)
				return
			default:
				currentFile = f.NameInArchive
				originalOpen := f.Open
				f = attachReaderToArchiveFile(f, originalOpen, readBytesChan, ctx)

				archiveJobs <- archiver.ArchiveAsyncJob{
					File:   f,
					Result: errChan,
				}
				result := <-errChan
				if result != nil {
					log.Error().Err(result).Str("file", f.NameInArchive).Msg("Failed to archive files")
					return
				}
				select {
				case <-ctx.Done():
					close(archiveJobs)
					return
				case <-inst.ctx.Done():
					close(archiveJobs)
					return
				case fileResultChan <- archiveJobResult{file: f, err: result}:
				}
			}
		}
		close(archiveJobs)
	}()

	err := format.ArchiveAsync(ctx, archiveOut, archiveJobs)
	if err != nil {
		log.Error().Err(err).Msg("Failed to archive files")
		return xylona.GameServerFilesArchiveProgress{}, err
	}
	// log.Debug().Msg("Finished archiving files")
	return createXylonaArchiveResult(totalFiles, filesCompressedSoFar, totalBytes,
		bytesReadSoFar, currentFile), nil
}

func attachReaderToArchiveFile(f archiver.File, originalOpen func() (io.ReadCloser, error), readBytesChan chan int64, ctx context.Context) archiver.File {
	f.Open = func() (io.ReadCloser, error) {
		archivedFile, errOpenArchivedFile := originalOpen()
		if errOpenArchivedFile != nil {
			log.Error().Err(errOpenArchivedFile).Msg("Failed to open archived file")
			return nil, errOpenArchivedFile
		}
		return &progressReader{
			Reader:    archivedFile,
			BytesChan: readBytesChan,
			fileName:  f.NameInArchive,
			ctx:       ctx,
		}, nil
	}
	return f
}

func (inst *Instance) ArchiveAndCompressFiles(ctx context.Context, gameServer *models.GameServer, destinationArchivePath string, fullFilePaths []string, compression xylona.GameServerFilesCompressionType) (string, error) {
	archivePath := filepath.Join(gameServer.Directory, destinationArchivePath)

	pathFilesMap := make(map[string]string)
	for _, f := range fullFilePaths {
		absPath, errAbsPath := filepath.Abs(filepath.Join(gameServer.Directory, f))
		if errAbsPath != nil {
			log.Error().Err(errAbsPath).Msg("Failed to get absolute path")
			return "", errAbsPath
		}
		pathFilesMap[absPath] = filepath.Base(f)
	}

	archiverDiskOptions := &archiver.FromDiskOptions{
		FollowSymlinks:  false,
		ClearAttributes: false,
	}

	archiverFiles, errFilesFromPaths := archiver.FilesFromDisk(archiverDiskOptions, pathFilesMap)
	if errFilesFromPaths != nil {
		log.Error().Err(errFilesFromPaths).Msg("Failed to get files from paths")
		return "", errFilesFromPaths
	}

	return inst.handleArchiveAndCompression(ctx, archivePath, archiverFiles, compression)
}

func (inst *Instance) handleArchiveAndCompression(ctx context.Context, archivePathAndFileName string, archiverFiles []archiver.File, compression xylona.GameServerFilesCompressionType) (string, error) {
	ctx = context.WithoutCancel(inst.ctx)
	archiveFullPath := archivePathAndFileName

	format := archiver.CompressedArchive{}
	// Default to tar if no compression is specified.
	format.Archival = &archiver.Tar{}

	switch compression {
	case xylona.GameServerFilesCompressionType_ZIP:
		format.Archival = &archiver.Zip{
			SelectiveCompression: true,
			Compression:          zip.Deflate,
			ContinueOnError:      false,
			TextEncoding:         "",
		}
	case xylona.GameServerFilesCompressionType_GZIP:
		format.Compression = &archiver.Gz{
			CompressionLevel: gzip.DefaultCompression,
			Multithreaded:    true,
		}
	case xylona.GameServerFilesCompressionType_BZIP2:
		format.Compression = &archiver.Bz2{CompressionLevel: bzip2.DefaultCompression}
	case xylona.GameServerFilesCompressionType_ZST:
		format.Compression = &archiver.Zstd{}
	case xylona.GameServerFilesCompressionType_XZ:
		format.Compression = &archiver.Xz{}
	}

	archiveFullPath = archiveFullPath + format.Name()

	archiveOut, errCreate := os.Create(archiveFullPath)
	if errCreate != nil {
		log.Error().Err(errCreate).Msg("Failed to create archive")
		return "", errCreate
	}
	defer func() {
		_ = archiveOut.Close()
	}()

	errArchive := format.Archive(ctx, archiveOut, archiverFiles)
	if errArchive != nil {
		log.Error().Err(errArchive).Msg("Failed to archive files")
		return "", errArchive
	}
	return archiveFullPath, nil
}

func (inst *Instance) ExtractArchive(ctx context.Context, gameServer *models.GameServer, archivePath string, destinationDirectoryPath string) ([]string, error) {
	archivePath = strings.TrimPrefix(archivePath, "/")
	destinationDirectoryPath = strings.TrimPrefix(destinationDirectoryPath, "/")
	archivePathIsLocal := filepath.IsLocal(archivePath)
	destinationDirectoryPathIsLocal := filepath.IsLocal(destinationDirectoryPath)
	if !archivePathIsLocal || (!destinationDirectoryPathIsLocal && destinationDirectoryPath != "") {
		log.Error().Str("archivePath", archivePath).Str("destinationPath", destinationDirectoryPath).Str("Game Server ID", gameServer.ID).Msg("Invalid path")
		return nil, ErrInvalidPath
	}

	archiveFullPath := filepath.Join(gameServer.Directory, archivePath)
	destinationFullPath := filepath.Join(gameServer.Directory, destinationDirectoryPath)
	archiveName := filepath.Base(archiveFullPath)

	archiveFile, errOpen := os.Open(archiveFullPath)
	if errOpen != nil {
		log.Error().Err(errOpen).Msg("Failed to open archive")
		return nil, errOpen
	}
	defer func() { _ = archiveFile.Close() }()

	format, archiveStream, errIdentify := archiver.Identify(archiveName, archiveFile)
	if errIdentify != nil {
		log.Error().Err(errIdentify).Msg("Failed to identify archive")
		return nil, errIdentify
	}

	var uncompressedArchiveExtractor archiver.Extractor
	compressedArchive, compressedArchiveOk := format.(archiver.CompressedArchive)
	if !compressedArchiveOk {
		a, ok := format.(archiver.Extractor)
		if !ok {
			log.Error().Msg("Failed to get archive from identify")
			return nil, errors.New("unable to identify archive type")
		}
		uncompressedArchiveExtractor = a
	}

	files := make([]os.FileInfo, 0)
	extractHandler := func(ctx context.Context, f archiver.File) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			isLocal := filepath.IsLocal(f.NameInArchive)
			if !isLocal {
				log.Error().Str("Game Server ID", gameServer.ID).Msg("Invalid path")
				return ErrInvalidPath
			}

			archivedFile, errOpenArchivedFile := f.Open()
			if errOpenArchivedFile != nil {
				log.Error().Str("Game Server ID", gameServer.ID).Err(errOpenArchivedFile).Msg("Failed to open archived file")
				return errOpenArchivedFile
			}
			defer func() { _ = archivedFile.Close() }()

			filePath := filepath.Join(destinationFullPath, f.NameInArchive)
			// If the file is a directory, we just need to create it. If it's a file, we need to create it and copy the contents.
			if f.IsDir() {
				// Create the directory.
				errMkdirAll := os.MkdirAll(filePath, 0700)
				if errMkdirAll != nil {
					log.Error().Str("Game Server ID", gameServer.ID).Err(errMkdirAll).Msg("Failed to create directory")
					return errMkdirAll
				}
				return nil
			}
			// Make sure the directory for the file exists, before creating the file.
			basePath := filepath.Dir(filePath)
			errMkdirAll := os.MkdirAll(basePath, 0700)
			if errMkdirAll != nil {
				log.Error().Str("Game Server ID", gameServer.ID).Err(errMkdirAll).Msg("Failed to create directory")
				return errMkdirAll
			}

			newFile, errCreate := os.Create(filePath)
			if errCreate != nil {
				log.Error().Str("Game Server ID", gameServer.ID).Err(errCreate).Msg("Failed to create file")
				return errCreate
			}
			defer func() { _ = newFile.Close() }()

			_, errCopy := io.Copy(newFile, archivedFile)
			if errCopy != nil {
				log.Error().Str("Game Server ID", gameServer.ID).Err(errCopy).Msg("Failed to copy file")
				return errCopy
			}
			files = append(files, f)
			return nil
		}
	}

	if compressedArchiveOk {
		errExtract := compressedArchive.Extract(ctx, archiveStream, nil, extractHandler)
		if errExtract != nil {
			log.Error().Err(errExtract).Msg("Failed to extract archive")
			return nil, errExtract
		}
	} else {
		errExtract := uncompressedArchiveExtractor.Extract(ctx, archiveStream, nil, extractHandler)
		if errExtract != nil {
			log.Error().Err(errExtract).Msg("Failed to extract archive")
			return nil, errExtract
		}
	}

	extractedFiles := make([]string, 0, len(files))
	for _, f := range files {
		extractedFiles = append(extractedFiles, f.Name())
	}
	return extractedFiles, nil
}
