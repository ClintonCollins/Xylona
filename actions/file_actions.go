package actions

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsnet/compress/bzip2"
	"github.com/go-chi/chi/v5"
	"github.com/klauspost/compress/zip"
	"github.com/mholt/archives"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	MaxRequestBodySize = 1024 * 1024 * 1 // 1 MB
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
	stat      fs.FileInfo
	ctx       context.Context
}

func (pr *progressReader) Stat() (fs.FileInfo, error) {
	return pr.stat, nil
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
	file archives.FileInfo
	err  error
}

type fileExtractor struct {
	gameServer              *models.GameServer
	currentFilePath         string
	destinationPath         string
	totalFiles              int64
	filesExtractedSoFar     int64
	totalBytes              int64
	bytesExtractedSoFar     int64
	extractedFilePathsSoFar []string
	reportProgress          bool
	progressChan            chan xylona.GameServerFilesExtractProgress
}

func (inst *Instance) CreateFileOrDirectory(gameServer *models.GameServer, path string, content string, isDirectory bool) error {
	path = strings.TrimPrefix(path, "/")
	pathIsLocal := filepath.IsLocal(path)
	if !pathIsLocal {
		log.Error().Str("Game Server ID", gameServer.ID).Msg("Invalid path")
		return ErrInvalidPath
	}
	fullPath := filepath.Join(gameServer.Directory, path)
	if isDirectory {
		errMkdir := os.MkdirAll(fullPath, os.ModePerm)
		if errMkdir != nil {
			log.Error().Err(errMkdir).Msg("Failed to create directory")
			return errMkdir
		}
	} else {
		file, errCreate := os.Create(fullPath)
		if errCreate != nil {
			log.Error().Err(errCreate).Msg("Failed to create file")
			return errCreate
		}

		if content != "" {
			_, errWrite := file.WriteString(content)
			if errWrite != nil {
				log.Error().Err(errWrite).Msg("Failed to write to file")
			}
		}

		_ = file.Close()
	}
	return nil
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
	if (!destinationIsLocal && destination != "") || destination == ".." {
		log.Error().Str("Game Server ID", gameServer.ID).Msg("Invalid path")
		return nil, ErrInvalidPath
	}
	destinationFullPath := filepath.Join(gameServer.Directory, destination)
	errMkdir := os.MkdirAll(destinationFullPath, os.ModePerm)
	if errMkdir != nil {
		log.Error().Err(errMkdir).Msg("Failed to create destination directory")
		return nil, errMkdir
	}
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
			fileDestinationPath := filepath.Join(destinationFullPath, filepath.Base(file))
			errRename := os.Rename(fullPath, fileDestinationPath)
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

	archivesDiskOptions := &archives.FromDiskOptions{
		FollowSymlinks:  false,
		ClearAttributes: false,
	}

	archivesFiles, errFilesFromPaths := archives.FilesFromDisk(ctx, archivesDiskOptions, pathFilesMap)
	if errFilesFromPaths != nil {
		log.Error().Err(errFilesFromPaths).Msg("Failed to get files from paths")
		return xylona.GameServerFilesArchiveProgress{}, errFilesFromPaths
	}

	return inst.archiveFilesWithProgress(ctx, archiveFullPath, archivesFiles, compression, xylonaFileArchiveProgressChan)
}

func createXylonaarchivesesult(totalFiles, filesCompressedSoFar, totalBytes, bytesCompressedSoFar int64,
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
	archivesFiles []archives.FileInfo, compression xylona.GameServerFilesCompressionType,
	xylonaFileArchiveProgressChan chan xylona.GameServerFilesArchiveProgress,
) (xylona.GameServerFilesArchiveProgress, error) {

	format := archives.CompressedArchive{}
	// Default to tar if no compression is specified.
	format.Archival = &archives.Tar{}

	switch compression {
	case xylona.GameServerFilesCompressionType_ZIP:
		format.Archival = &archives.Zip{
			SelectiveCompression: true,
			Compression:          zip.Deflate,
			ContinueOnError:      false,
		}
	case xylona.GameServerFilesCompressionType_GZIP:
		format.Compression = &archives.Gz{
			CompressionLevel: gzip.DefaultCompression,
			Multithreaded:    true,
		}
	case xylona.GameServerFilesCompressionType_BZIP2:
		format.Compression = &archives.Bz2{CompressionLevel: bzip2.DefaultCompression}
	case xylona.GameServerFilesCompressionType_ZST:
		format.Compression = &archives.Zstd{}
	case xylona.GameServerFilesCompressionType_XZ:
		format.Compression = &archives.Xz{}
	}

	archiveFullPath = archiveFullPath + format.Extension()

	archiveOut, errCreate := os.Create(archiveFullPath)
	if errCreate != nil {
		log.Error().Err(errCreate).Msg("Failed to create archive")
		return xylona.GameServerFilesArchiveProgress{}, errCreate
	}
	defer func() {
		_ = archiveOut.Close()
	}()

	totalBytes := int64(0)
	totalFiles := int64(len(archivesFiles))
	filesCompressedSoFar := int64(0)
	bytesReadSoFar := int64(0)
	currentFile := ""

	for _, f := range archivesFiles {
		totalBytes += f.Size()
	}

	archiveJobs := make(chan archives.ArchiveAsyncJob, len(archivesFiles))
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
				xylonaProgress := createXylonaarchivesesult(totalFiles, filesCompressedSoFar, totalBytes,
					bytesReadSoFar, currentFile)
				inst.attemptToSendOnXylonaProgressChan(ctx, xylonaProgress, xylonaFileArchiveProgressChan)
			case <-fileResultChan:
				filesCompressedSoFar += 1
				xylonaProgress := createXylonaarchivesesult(totalFiles, filesCompressedSoFar, totalBytes,
					bytesReadSoFar, currentFile)
				inst.attemptToSendOnXylonaProgressChan(ctx, xylonaProgress, xylonaFileArchiveProgressChan)
				if filesCompressedSoFar == totalFiles {
					return
				}
			}
		}

	}()

	go func() {
		for _, f := range archivesFiles {
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

				archiveJobs <- archives.ArchiveAsyncJob{
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
	return createXylonaarchivesesult(totalFiles, filesCompressedSoFar, totalBytes,
		bytesReadSoFar, currentFile), nil
}

func attachReaderToArchiveFile(f archives.FileInfo, originalOpen func() (fs.File, error), readBytesChan chan int64, ctx context.Context) archives.FileInfo {
	f.Open = func() (fs.File, error) {
		archivedFile, errOpenArchivedFile := originalOpen()
		if errOpenArchivedFile != nil {
			log.Error().Err(errOpenArchivedFile).Msg("Failed to open archived file")
			return nil, errOpenArchivedFile
		}
		stat, statErr := f.Stat()
		if statErr != nil {
			log.Error().Err(statErr).Msg("Failed to stat file")
			return nil, statErr
		}
		return &progressReader{
			Reader:    archivedFile,
			BytesChan: readBytesChan,
			fileName:  f.NameInArchive,
			stat:      stat,
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

	archivesDiskOptions := &archives.FromDiskOptions{
		FollowSymlinks:  false,
		ClearAttributes: false,
	}

	archivesFiles, errFilesFromPaths := archives.FilesFromDisk(ctx, archivesDiskOptions, pathFilesMap)
	if errFilesFromPaths != nil {
		log.Error().Err(errFilesFromPaths).Msg("Failed to get files from paths")
		return "", errFilesFromPaths
	}

	return inst.handleArchiveAndCompression(ctx, archivePath, archivesFiles, compression)
}

func (inst *Instance) handleArchiveAndCompression(ctx context.Context, archivePathAndFileName string, archivesFiles []archives.FileInfo, compression xylona.GameServerFilesCompressionType) (string, error) {
	ctx = context.WithoutCancel(inst.ctx)
	archiveFullPath := archivePathAndFileName

	format := archives.CompressedArchive{}
	// Default to tar if no compression is specified.
	format.Archival = &archives.Tar{}

	switch compression {
	case xylona.GameServerFilesCompressionType_ZIP:
		format.Archival = &archives.Zip{
			SelectiveCompression: true,
			Compression:          zip.Deflate,
			ContinueOnError:      false,
		}
	case xylona.GameServerFilesCompressionType_GZIP:
		format.Compression = &archives.Gz{
			CompressionLevel: gzip.DefaultCompression,
			Multithreaded:    true,
		}
	case xylona.GameServerFilesCompressionType_BZIP2:
		format.Compression = &archives.Bz2{CompressionLevel: bzip2.DefaultCompression}
	case xylona.GameServerFilesCompressionType_ZST:
		format.Compression = &archives.Zstd{}
	case xylona.GameServerFilesCompressionType_XZ:
		format.Compression = &archives.Xz{}
	}

	archiveFullPath = archiveFullPath + format.Extension()

	archiveOut, errCreate := os.Create(archiveFullPath)
	if errCreate != nil {
		log.Error().Err(errCreate).Msg("Failed to create archive")
		return "", errCreate
	}
	defer func() {
		_ = archiveOut.Close()
	}()

	errArchive := format.Archive(ctx, archiveOut, archivesFiles)
	if errArchive != nil {
		log.Error().Err(errArchive).Msg("Failed to archive files")
		return "", errArchive
	}
	return archiveFullPath, nil
}

func (inst *Instance) ExtractFiles(ctx context.Context, gameServer *models.GameServer, fullArchivePath string, destinationPath string,
	xylonaFileExtractProgressChan chan xylona.GameServerFilesExtractProgress,
) (xylona.GameServerFilesExtractProgress, error) {
	archiveFullPath := filepath.Join(gameServer.Directory, fullArchivePath)
	fullDestinationPath := filepath.Join(gameServer.Directory, destinationPath)

	archiveFS, errFS := archives.FileSystem(ctx, archiveFullPath, nil)
	if errFS != nil {
		log.Error().Err(errFS).Msg("Failed to get file system")
		return xylona.GameServerFilesExtractProgress{}, errFS
	}

	filesInArchive := make([]string, 0)
	totalFiles := int64(0)
	totalBytes := int64(0)

	errWalk := fs.WalkDir(archiveFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, errStat := d.Info()
		if errStat != nil {
			return errStat
		}
		totalFiles++
		totalBytes += info.Size()
		filesInArchive = append(filesInArchive, path)
		return nil
	})
	if errWalk != nil {
		log.Error().Err(errWalk).Msg("Failed to walk directory")
		return xylona.GameServerFilesExtractProgress{}, errWalk
	}

	archiveFile, errOpen := os.Open(archiveFullPath)
	if errOpen != nil {
		log.Error().Err(errOpen).Msg("Failed to open archive")
		return xylona.GameServerFilesExtractProgress{}, errOpen
	}
	defer func() { _ = archiveFile.Close() }()

	format, archiveStream, errIdentify := archives.Identify(ctx, archiveFullPath, archiveFile)
	if errIdentify != nil {
		log.Error().Err(errIdentify).Msg("Failed to identify archive")
		return xylona.GameServerFilesExtractProgress{}, errIdentify
	}

	var uncompressedArchiveExtractor archives.Extractor
	compressedArchive, compressedArchiveOk := format.(archives.CompressedArchive)
	if !compressedArchiveOk {
		a, ok := format.(archives.Extractor)
		if !ok {
			log.Error().Msg("Failed to get archive from identify")
			return xylona.GameServerFilesExtractProgress{}, errors.New("unable to identify archive type")
		}
		uncompressedArchiveExtractor = a
	}

	fx := &fileExtractor{
		gameServer:              gameServer,
		currentFilePath:         "",
		destinationPath:         fullDestinationPath,
		totalFiles:              totalFiles,
		filesExtractedSoFar:     0,
		totalBytes:              totalBytes,
		bytesExtractedSoFar:     0,
		extractedFilePathsSoFar: nil,
		reportProgress:          true,
		progressChan:            xylonaFileExtractProgressChan,
	}

	// Handle initial status.
	firstFile := ""
	if len(filesInArchive) > 0 {
		firstFile = filesInArchive[0]
	}
	initialProgress := xylona.GameServerFilesExtractProgress{
		TotalFiles:     totalFiles,
		FilesExtracted: 0,
		TotalBytes:     totalBytes,
		BytesExtracted: 0,
		CurrentFile:    firstFile,
	}
	timeout := time.After(time.Millisecond * 50)
	select {
	case <-ctx.Done():
		return xylona.GameServerFilesExtractProgress{}, ctx.Err()
	case <-inst.ctx.Done():
		return xylona.GameServerFilesExtractProgress{}, ctx.Err()
	case <-timeout:
		return xylona.GameServerFilesExtractProgress{}, errors.New("timeout")
	case xylonaFileExtractProgressChan <- initialProgress:
	}
	// end initial status

	if compressedArchiveOk {
		errExtract := compressedArchive.Extract(ctx, archiveStream, fx.extractFileHandler)
		if errExtract != nil {
			log.Error().Err(errExtract).Msg("Failed to extract archive")
			return xylona.GameServerFilesExtractProgress{}, errExtract
		}
	} else {
		errExtract := uncompressedArchiveExtractor.Extract(ctx, archiveStream, fx.extractFileHandler)
		if errExtract != nil {
			log.Error().Err(errExtract).Msg("Failed to extract archive")
			return xylona.GameServerFilesExtractProgress{}, errExtract
		}
	}
	return xylona.GameServerFilesExtractProgress{
		TotalFiles:     fx.totalFiles,
		FilesExtracted: fx.filesExtractedSoFar,
		TotalBytes:     fx.totalBytes,
		BytesExtracted: fx.bytesExtractedSoFar,
		CurrentFile:    fx.currentFilePath,
	}, nil
}

func (inst *Instance) StreamFileToUser(w http.ResponseWriter, r *http.Request) {
	fileRequest := xylona.DownloadFileRequest{}
	bodyBytes, errReadBody := io.ReadAll(io.LimitReader(r.Body, MaxRequestBodySize))
	if errReadBody != nil {
		log.Error().Err(errReadBody).Msg("Failed to read file request body")
		http.Error(w, "Failed to read file request body", http.StatusBadRequest)
		return
	}
	errDecode := protojson.Unmarshal(bodyBytes, &fileRequest)
	if errDecode != nil {
		log.Error().Err(errDecode).Msg("Failed to decode file request")
		http.Error(w, "Failed to decode file request", http.StatusBadRequest)
		return
	}

	target, errResolve := inst.resolveFileRequestTarget(fileRequest.GameServerId)
	if errResolve != nil {
		log.Error().Err(errResolve).Msg("Failed to get game server")
		writeGameServerLookupError(w, errResolve)
		return
	}

	if target.isLocal() {
		errGetFile := inst.GetGameServerFile(target.gameServer, fileRequest.Path, w, true, false)
		if errGetFile != nil {
			log.Error().Err(errGetFile).Msg("Failed to get file")
			http.Error(w, "Failed to get file", http.StatusInternalServerError)
			return
		}
		return
	}

	errProxy := inst.proxyRemoteFileGet(r.Context(), target, fileRequest.Path, w)
	if errProxy != nil {
		log.Error().Err(errProxy).Msg("Failed to proxy remote file request")
		http.Error(w, "Failed to get file", http.StatusInternalServerError)
		return
	}
}

func (inst *Instance) UploadFileToUserGET(w http.ResponseWriter, r *http.Request) {
	gameServerID, errGameServerID := url.QueryUnescape(chi.URLParam(r, "gameServerId"))
	filePath, errFilePath := url.QueryUnescape(chi.URLParam(r, "path"))
	if errGameServerID != nil || errFilePath != nil {
		log.Error().Err(errGameServerID).Err(errFilePath).Msg("Failed to get game server ID or file path")
		http.Error(w, "Failed to get game server ID or file path", http.StatusBadRequest)
		return
	}

	target, errResolve := inst.resolveFileRequestTarget(gameServerID)
	if errResolve != nil {
		log.Error().Err(errResolve).Msg("Failed to get game server")
		writeGameServerLookupError(w, errResolve)
		return
	}

	if target.isLocal() {
		errGetFile := inst.GetGameServerFile(target.gameServer, filePath, w, true, true)
		if errGetFile != nil {
			log.Error().Err(errGetFile).Msg("Failed to get file")
			http.Error(w, "Failed to get file", http.StatusInternalServerError)
			return
		}
		return
	}

	errProxy := inst.proxyRemoteFileDownload(r.Context(), target, filePath, w)
	if errProxy != nil {
		log.Error().Err(errProxy).Msg("Failed to proxy remote file download")
		http.Error(w, "Failed to get file", http.StatusInternalServerError)
		return
	}
}

func (inst *Instance) UploadFileToUserPOST(w http.ResponseWriter, r *http.Request) {
	errParseForm := r.ParseForm()
	if errParseForm != nil {
		log.Error().Err(errParseForm).Msg("Failed to parse form")
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}
	gameServerID := r.FormValue("gameServerId")
	filePath := r.FormValue("path")

	target, errResolve := inst.resolveFileRequestTarget(gameServerID)
	if errResolve != nil {
		log.Error().Err(errResolve).Msg("Failed to get game server")
		writeGameServerLookupError(w, errResolve)
		return
	}

	if target.isLocal() {
		errGetFile := inst.GetGameServerFile(target.gameServer, filePath, w, true, true)
		if errGetFile != nil {
			log.Error().Err(errGetFile).Msg("Failed to get file")
			http.Error(w, "Failed to get file", http.StatusInternalServerError)
			return
		}
		return
	}

	errProxy := inst.proxyRemoteFileDownload(r.Context(), target, filePath, w)
	if errProxy != nil {
		log.Error().Err(errProxy).Msg("Failed to proxy remote file download")
		http.Error(w, "Failed to get file", http.StatusInternalServerError)
		return
	}
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

	format, archiveStream, errIdentify := archives.Identify(ctx, archiveName, archiveFile)
	if errIdentify != nil {
		log.Error().Err(errIdentify).Msg("Failed to identify archive")
		return nil, errIdentify
	}

	var uncompressedArchiveExtractor archives.Extractor
	compressedArchive, compressedArchiveOk := format.(archives.CompressedArchive)
	if !compressedArchiveOk {
		a, ok := format.(archives.Extractor)
		if !ok {
			log.Error().Msg("Failed to get archive from identify")
			return nil, errors.New("unable to identify archive type")
		}
		uncompressedArchiveExtractor = a
	}

	fx := &fileExtractor{
		gameServer:              gameServer,
		currentFilePath:         "",
		destinationPath:         destinationFullPath,
		totalFiles:              0,
		filesExtractedSoFar:     0,
		totalBytes:              0,
		bytesExtractedSoFar:     0,
		extractedFilePathsSoFar: nil,
		reportProgress:          false,
	}

	if compressedArchiveOk {
		errExtract := compressedArchive.Extract(ctx, archiveStream, fx.extractFileHandler)
		if errExtract != nil {
			log.Error().Err(errExtract).Msg("Failed to extract archive")
			return nil, errExtract
		}
	} else {
		errExtract := uncompressedArchiveExtractor.Extract(ctx, archiveStream, fx.extractFileHandler)
		if errExtract != nil {
			log.Error().Err(errExtract).Msg("Failed to extract archive")
			return nil, errExtract
		}
	}

	extractedFiles := make([]string, 0, len(fx.extractedFilePathsSoFar))
	for _, f := range fx.extractedFilePathsSoFar {
		extractedFiles = append(extractedFiles, f)
	}
	return extractedFiles, nil
}

func (fx *fileExtractor) extractFileHandler(ctx context.Context, f archives.FileInfo) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		isLocal := filepath.IsLocal(f.NameInArchive)
		if !isLocal {
			log.Error().Str("Game Server ID", fx.gameServer.ID).Msg("Invalid path")
			return ErrInvalidPath
		}

		archivedFile, errOpenArchivedFile := f.Open()
		if errOpenArchivedFile != nil {
			log.Error().Str("Game Server ID", fx.gameServer.ID).Err(errOpenArchivedFile).Msg("Failed to open archived file")
			return errOpenArchivedFile
		}
		defer func() { _ = archivedFile.Close() }()

		filePath := filepath.Join(fx.destinationPath, f.NameInArchive)
		// If the file is a directory, we just need to create it. If it's a file, we need to create it and copy the contents.
		if f.IsDir() {
			// Create the directory.
			errMkdirAll := os.MkdirAll(filePath, 0700)
			if errMkdirAll != nil {
				log.Error().Str("Game Server ID", fx.gameServer.ID).Err(errMkdirAll).Msg("Failed to create directory")
				return errMkdirAll
			}
			return nil
		}
		// Make sure the directory for the file exists, before creating the file.
		basePath := filepath.Dir(filePath)
		errMkdirAll := os.MkdirAll(basePath, 0700)
		if errMkdirAll != nil {
			log.Error().Str("Game Server ID", fx.gameServer.ID).Err(errMkdirAll).Msg("Failed to create directory")
			return errMkdirAll
		}

		newFile, errCreate := os.Create(filePath)
		if errCreate != nil {
			log.Error().Str("Game Server ID", fx.gameServer.ID).Err(errCreate).Msg("Failed to create file")
			return errCreate
		}
		defer func() { _ = newFile.Close() }()

		_, errCopy := io.Copy(newFile, archivedFile)
		if errCopy != nil {
			log.Error().Str("Game Server ID", fx.gameServer.ID).Err(errCopy).Msg("Failed to copy file")
			return errCopy
		}

		// Update file extractor data.
		fx.currentFilePath = f.NameInArchive
		fx.filesExtractedSoFar++
		fx.bytesExtractedSoFar += f.Size()
		fx.extractedFilePathsSoFar = append(fx.extractedFilePathsSoFar, f.NameInArchive)

		if fx.reportProgress {
			progress := xylona.GameServerFilesExtractProgress{
				TotalFiles:     fx.totalFiles,
				FilesExtracted: fx.filesExtractedSoFar,
				TotalBytes:     fx.totalBytes,
				BytesExtracted: fx.bytesExtractedSoFar,
				CurrentFile:    f.NameInArchive,
			}
			timeout := time.After(time.Millisecond * 50)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timeout:
				log.Warn().Msg("Failed to send progress")
				return nil
			case fx.progressChan <- progress:
			}
		}
		return nil
	}
}
