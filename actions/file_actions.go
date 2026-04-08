package actions

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dsnet/compress/bzip2"
	"github.com/gabriel-vasile/mimetype"
	"github.com/go-chi/chi/v5"
	"github.com/gosimple/slug"
	"github.com/klauspost/compress/zip"
	"github.com/mholt/archives"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// MaxRequestBodySize caps file action request bodies at 1 MiB.
const (
	MaxRequestBodySize = 1024 * 1024 * 1 // 1 MB
)

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
		errClose := closer.Close()
		if errClose != nil {
			return wrapFileActionError("close progress reader", errClose)
		}
		return nil
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
	progressChan            chan *xylona.GameServerFilesExtractProgress
}

func wrapFileActionError(operation string, err error) error {
	return fmt.Errorf("actions: %s: %w", operation, err)
}

// CreateFileOrDirectory creates or writes a file or directory inside the server root.
func (inst *Instance) CreateFileOrDirectory(gameServer *models.GameServer, relativePath string, content string, isDirectory bool) error {
	validatedPath, errPath := validateWritableServerPath(gameServer, relativePath)
	if errPath != nil {
		return errPath
	}
	fullPath := filepath.Join(gameServer.Directory, validatedPath)
	if isDirectory {
		errMkdir := os.MkdirAll(fullPath, 0o750)
		if errMkdir != nil {
			log.Error().Err(errMkdir).Msg("Failed to create directory")
			return wrapFileActionError("create directory", errMkdir)
		}
	} else {
		file, errCreate := os.Create(fullPath)
		if errCreate != nil {
			log.Error().Err(errCreate).Msg("Failed to create file")
			return wrapFileActionError("create file", errCreate)
		}

		if content != "" {
			_, errWrite := file.WriteString(content)
			if errWrite != nil {
				log.Error().Err(errWrite).Msg("Failed to write to file")
				errClose := file.Close()
				if errClose != nil {
					log.Error().Err(errClose).Msg("Failed to close file after write error")
				}
				return wrapFileActionError("write file", errWrite)
			}
		}

		errClose := file.Close()
		if errClose != nil {
			log.Error().Err(errClose).Msg("Failed to close file")
			return wrapFileActionError("close file", errClose)
		}
	}
	return nil
}

// DeleteFiles deletes multiple files or directories from a server.
func (inst *Instance) DeleteFiles(ctx context.Context, gameServer *models.GameServer, files []string) ([]string, error) {
	successfullyDeleted := make([]string, 0, len(files))
	for _, file := range files {
		select {
		case <-ctx.Done():
			return nil, wrapFileActionError("delete files context canceled", ctx.Err())
		default:
			validatedPath, errPath := validateWritableServerPath(gameServer, file)
			if errPath != nil {
				return nil, errPath
			}
			fullPath := filepath.Join(gameServer.Directory, validatedPath)
			errRemove := os.RemoveAll(fullPath)
			if errRemove != nil {
				log.Error().Err(errRemove).Msg("Failed to remove file")
				continue
			}
			successfullyDeleted = append(successfullyDeleted, validatedPath)
		}
	}
	return successfullyDeleted, nil
}

// RenameFile renames a file or directory within a server.
func (inst *Instance) RenameFile(gameServer *models.GameServer, oldFilePath, newFilePath string) (string, error) {
	validatedOldPath, errOldPath := validateWritableServerPath(gameServer, oldFilePath)
	if errOldPath != nil {
		return "", errOldPath
	}
	validatedNewPath, errNewPath := validateWritableServerPath(gameServer, newFilePath)
	if errNewPath != nil {
		return "", errNewPath
	}
	oldFullPath := filepath.Join(gameServer.Directory, validatedOldPath)
	newFullPath := filepath.Join(gameServer.Directory, validatedNewPath)
	errRename := os.Rename(oldFullPath, newFullPath)
	if errRename != nil {
		log.Error().Err(errRename).Msg("Failed to rename file")
		return "", wrapFileActionError("rename file", errRename)
	}
	return validatedNewPath, nil
}

// MoveFiles moves files into another directory within the same server root.
func (inst *Instance) MoveFiles(ctx context.Context, gameServer *models.GameServer, files []string, destination string) ([]string, error) {
	successfullyMoved := make([]string, 0, len(files))
	validatedDestination, errDestination := validateLocalServerPath(gameServer, destination)
	if errDestination != nil {
		return nil, errDestination
	}
	if validatedDestination == ".." {
		log.Error().Str("Game Server ID", gameServer.ID).Msg("Invalid path")
		return nil, ErrInvalidPath
	}
	destinationFullPath := filepath.Join(gameServer.Directory, validatedDestination)
	errMkdir := os.MkdirAll(destinationFullPath, 0o750)
	if errMkdir != nil {
		log.Error().Err(errMkdir).Msg("Failed to create destination directory")
		return nil, wrapFileActionError("create destination directory", errMkdir)
	}
	for _, file := range files {
		select {
		case <-ctx.Done():
			return nil, wrapFileActionError("move files context canceled", ctx.Err())
		default:
			validatedFilePath, errFilePath := validateWritableServerPath(gameServer, file)
			if errFilePath != nil {
				return nil, errFilePath
			}
			destinationFilePath := filepath.Join(validatedDestination, filepath.Base(validatedFilePath))
			_, errProtected := validateWritableServerPath(gameServer, destinationFilePath)
			if errProtected != nil {
				return nil, errProtected
			}
			fullPath := filepath.Join(gameServer.Directory, validatedFilePath)
			fileDestinationPath := filepath.Join(destinationFullPath, filepath.Base(file))
			errRename := os.Rename(fullPath, fileDestinationPath)
			if errRename != nil {
				log.Error().Err(errRename).Msg("Failed to move file")
				continue
			}
			successfullyMoved = append(successfullyMoved, validatedFilePath)
		}
	}
	return successfullyMoved, nil
}

// EditFile overwrites a file with the provided content.
func (inst *Instance) EditFile(gameServer *models.GameServer, filePath string, content string) error {
	validatedPath, errPath := validateWritableServerPath(gameServer, filePath)
	if errPath != nil {
		return errPath
	}
	fullPath := filepath.Join(gameServer.Directory, validatedPath)
	file, errOpen := os.Create(fullPath)
	if errOpen != nil {
		log.Error().Err(errOpen).Msg("Failed to open file")
		return wrapFileActionError("open file for edit", errOpen)
	}
	defer func() {
		errClose := file.Close()
		if errClose != nil {
			log.Error().Err(errClose).Msg("Failed to close file")
		}
	}()
	_, errWrite := file.WriteString(content)
	if errWrite != nil {
		log.Error().Err(errWrite).Msg("Failed to write to file")
		return wrapFileActionError("write edited file", errWrite)
	}
	return nil
}

// DownloadFileFromURL downloads a remote file into a server directory.
func (inst *Instance) DownloadFileFromURL(ctx context.Context, gameServer *models.GameServer, rawURL, destinationDirectoryPath string) (string, error) {
	validatedDestinationDirectory, errPath := validateLocalServerPath(gameServer, destinationDirectoryPath)
	if errPath != nil {
		return "", errPath
	}

	// Validate URL scheme to prevent SSRF via file://, gopher://, etc.
	parsedURL, errParseURL := url.Parse(rawURL)
	if errParseURL != nil {
		log.Error().Err(errParseURL).Msg("Failed to parse URL")
		return "", errors.New("invalid URL")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		log.Error().Str("scheme", parsedURL.Scheme).Msg("URL scheme not allowed")
		return "", errors.New("only http and https URLs are allowed")
	}

	httpClient := helpers.GetXylonaHTTPClient()
	req, errNewReq := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if errNewReq != nil {
		log.Error().Err(errNewReq).Msg("Failed to create request")
		return "", wrapFileActionError("create download request", errNewReq)
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
		return "", wrapFileActionError("download file from URL", errGet)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	protectedRelativePath := filepath.Join(validatedDestinationDirectory, fileName)
	_, errProtected := validateWritableServerPath(gameServer, protectedRelativePath)
	if errProtected != nil {
		return "", errProtected
	}

	destinationFullPath := filepath.Join(gameServer.Directory, validatedDestinationDirectory, fileName)
	file, errCreate := os.Create(destinationFullPath)
	if errCreate != nil {
		log.Error().Err(errCreate).Msg("Failed to create file")
		return "", wrapFileActionError("create downloaded file", errCreate)
	}
	defer func() {
		_ = file.Close()
	}()

	_, errCopy := io.Copy(file, resp.Body)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy file")
		return "", wrapFileActionError("write downloaded file", errCopy)
	}
	return "", nil
}

// ArchiveFiles archives the provided files and emits progress updates.
func (inst *Instance) ArchiveFiles(ctx context.Context, gameServer *models.GameServer, fullArchivePath string, fullFilePaths []string,
	compression xylona.GameServerFilesCompressionType,
	xylonaFileArchiveProgressChan chan *xylona.GameServerFilesArchiveProgress,
) (*xylona.GameServerFilesArchiveProgress, error) {
	validatedArchivePath, errArchivePath := validateWritableServerPath(gameServer, fullArchivePath)
	if errArchivePath != nil {
		return nil, errArchivePath
	}

	archiveFullPath := filepath.Join(gameServer.Directory, validatedArchivePath)

	pathFilesMap := make(map[string]string)
	for _, f := range fullFilePaths {
		absPath, errAbsPath := filepath.Abs(filepath.Join(gameServer.Directory, f))
		if errAbsPath != nil {
			log.Error().Err(errAbsPath).Msg("Failed to get absolute path")
			return nil, wrapFileActionError("resolve absolute archive path", errAbsPath)
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
		return nil, wrapFileActionError("collect files from disk", errFilesFromPaths)
	}

	return inst.archiveFilesWithProgress(ctx, archiveFullPath, archivesFiles, compression, xylonaFileArchiveProgressChan)
}

func createXylonaarchivesesult(totalFiles, filesCompressedSoFar, totalBytes, bytesCompressedSoFar int64,
	currentFile string,
) *xylona.GameServerFilesArchiveProgress {
	return &xylona.GameServerFilesArchiveProgress{
		TotalFiles:      totalFiles,
		FilesCompressed: filesCompressedSoFar,
		TotalBytes:      totalBytes,
		BytesCompressed: bytesCompressedSoFar,
		CurrentFile:     currentFile,
	}
}

func (inst *Instance) attemptToSendOnXylonaProgressChan(ctx context.Context,
	xylonaProgress *xylona.GameServerFilesArchiveProgress, progressChan chan *xylona.GameServerFilesArchiveProgress,
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
	xylonaFileArchiveProgressChan chan *xylona.GameServerFilesArchiveProgress,
) (*xylona.GameServerFilesArchiveProgress, error) {

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

	archiveFullPath += format.Extension()

	archiveOut, errCreate := os.Create(archiveFullPath)
	if errCreate != nil {
		log.Error().Err(errCreate).Msg("Failed to create archive")
		return nil, wrapFileActionError("create archive output", errCreate)
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
				log.Debug().Fields(map[string]any{
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
				filesCompressedSoFar++
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
				f = attachReaderToArchiveFile(ctx, f, originalOpen, readBytesChan)

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
		return nil, wrapFileActionError("archive files asynchronously", err)
	}
	// log.Debug().Msg("Finished archiving files")
	return createXylonaarchivesesult(totalFiles, filesCompressedSoFar, totalBytes,
		bytesReadSoFar, currentFile), nil
}

func attachReaderToArchiveFile(ctx context.Context, f archives.FileInfo, originalOpen func() (fs.File, error), readBytesChan chan int64) archives.FileInfo {
	f.Open = func() (fs.File, error) {
		archivedFile, errOpenArchivedFile := originalOpen()
		if errOpenArchivedFile != nil {
			log.Error().Err(errOpenArchivedFile).Msg("Failed to open archived file")
			return nil, wrapFileActionError("open archived file", errOpenArchivedFile)
		}
		stat, statErr := f.Stat()
		if statErr != nil {
			log.Error().Err(statErr).Msg("Failed to stat file")
			return nil, wrapFileActionError("stat archived file", statErr)
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

// ArchiveAndCompressFiles archives files to a compressed output file.
func (inst *Instance) ArchiveAndCompressFiles(ctx context.Context, gameServer *models.GameServer, destinationArchivePath string, fullFilePaths []string, compression xylona.GameServerFilesCompressionType) (string, error) {
	validatedArchivePath, errArchivePath := validateWritableServerPath(gameServer, destinationArchivePath)
	if errArchivePath != nil {
		return "", errArchivePath
	}
	archivePath := filepath.Join(gameServer.Directory, validatedArchivePath)

	pathFilesMap := make(map[string]string)
	for _, f := range fullFilePaths {
		absPath, errAbsPath := filepath.Abs(filepath.Join(gameServer.Directory, f))
		if errAbsPath != nil {
			log.Error().Err(errAbsPath).Msg("Failed to get absolute path")
			return "", wrapFileActionError("resolve absolute archive path", errAbsPath)
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
		return "", wrapFileActionError("collect files from disk", errFilesFromPaths)
	}

	return inst.handleArchiveAndCompression(ctx, archivePath, archivesFiles, compression)
}

func (inst *Instance) handleArchiveAndCompression(_ context.Context, archivePathAndFileName string, archivesFiles []archives.FileInfo, compression xylona.GameServerFilesCompressionType) (string, error) {
	ctx := context.WithoutCancel(inst.ctx)
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

	archiveFullPath += format.Extension()

	archiveOut, errCreate := os.Create(archiveFullPath)
	if errCreate != nil {
		log.Error().Err(errCreate).Msg("Failed to create archive")
		return "", wrapFileActionError("create archive output", errCreate)
	}
	defer func() {
		_ = archiveOut.Close()
	}()

	errArchive := format.Archive(ctx, archiveOut, archivesFiles)
	if errArchive != nil {
		log.Error().Err(errArchive).Msg("Failed to archive files")
		return "", wrapFileActionError("archive files", errArchive)
	}
	return archiveFullPath, nil
}

// ExtractFiles extracts an archive and reports progress updates.
func (inst *Instance) ExtractFiles(ctx context.Context, gameServer *models.GameServer, fullArchivePath string, destinationPath string,
	xylonaFileExtractProgressChan chan *xylona.GameServerFilesExtractProgress,
) (*xylona.GameServerFilesExtractProgress, error) {
	archiveFullPath := filepath.Join(gameServer.Directory, fullArchivePath)
	validatedDestinationPath, errDestinationPath := validateLocalServerPath(gameServer, destinationPath)
	if errDestinationPath != nil {
		return nil, errDestinationPath
	}
	fullDestinationPath := filepath.Join(gameServer.Directory, validatedDestinationPath)

	archiveFS, errFS := archives.FileSystem(ctx, archiveFullPath, nil)
	if errFS != nil {
		log.Error().Err(errFS).Msg("Failed to get file system")
		return nil, wrapFileActionError("open archive file system", errFS)
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
			return wrapFileActionError("stat archive entry", errStat)
		}
		totalFiles++
		totalBytes += info.Size()
		filesInArchive = append(filesInArchive, path)
		return nil
	})
	if errWalk != nil {
		log.Error().Err(errWalk).Msg("Failed to walk directory")
		return nil, wrapFileActionError("walk archive file system", errWalk)
	}

	archiveFile, errOpen := os.Open(archiveFullPath)
	if errOpen != nil {
		log.Error().Err(errOpen).Msg("Failed to open archive")
		return nil, wrapFileActionError("open archive", errOpen)
	}
	defer func() { _ = archiveFile.Close() }()

	format, archiveStream, errIdentify := archives.Identify(ctx, archiveFullPath, archiveFile)
	if errIdentify != nil {
		log.Error().Err(errIdentify).Msg("Failed to identify archive")
		return nil, wrapFileActionError("identify archive", errIdentify)
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
	initialProgress := &xylona.GameServerFilesExtractProgress{
		TotalFiles:     totalFiles,
		FilesExtracted: 0,
		TotalBytes:     totalBytes,
		BytesExtracted: 0,
		CurrentFile:    firstFile,
	}
	timeout := time.After(time.Millisecond * 50)
	select {
	case <-ctx.Done():
		return nil, wrapFileActionError("extract files context canceled", ctx.Err())
	case <-inst.ctx.Done():
		return nil, wrapFileActionError("extract files instance context canceled", ctx.Err())
	case <-timeout:
		return nil, errors.New("timeout")
	case xylonaFileExtractProgressChan <- initialProgress:
	}
	// end initial status

	if compressedArchiveOk {
		errExtract := compressedArchive.Extract(ctx, archiveStream, fx.extractFileHandler)
		if errExtract != nil {
			log.Error().Err(errExtract).Msg("Failed to extract archive")
			return nil, wrapFileActionError("extract compressed archive", errExtract)
		}
	} else {
		errExtract := uncompressedArchiveExtractor.Extract(ctx, archiveStream, fx.extractFileHandler)
		if errExtract != nil {
			log.Error().Err(errExtract).Msg("Failed to extract archive")
			return nil, wrapFileActionError("extract archive", errExtract)
		}
	}
	return &xylona.GameServerFilesExtractProgress{
		TotalFiles:     fx.totalFiles,
		FilesExtracted: fx.filesExtractedSoFar,
		TotalBytes:     fx.totalBytes,
		BytesExtracted: fx.bytesExtractedSoFar,
		CurrentFile:    fx.currentFilePath,
	}, nil
}

// StreamFileToUser streams a server file to an HTTP response.
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

	target, errResolve := inst.resolveFileRequestTarget(fileRequest.GetGameServerId())
	if errResolve != nil {
		log.Error().Err(errResolve).Msg("Failed to get game server")
		writeGameServerLookupError(w, errResolve)
		return
	}

	if target.isLocal() {
		errGetFile := inst.GetGameServerFile(target.gameServer, fileRequest.GetPath(), w, true, false)
		if errGetFile != nil {
			log.Error().Err(errGetFile).Msg("Failed to get file")
			http.Error(w, "Failed to get file", http.StatusInternalServerError)
			return
		}
		return
	}

	errProxy := inst.proxyRemoteFileGet(r.Context(), target, fileRequest.GetPath(), w)
	if errProxy != nil {
		log.Error().Err(errProxy).Msg("Failed to proxy remote file request")
		http.Error(w, "Failed to get file", http.StatusInternalServerError)
		return
	}
}

// UploadFileToUserGET streams a file download requested via query parameters.
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

// UploadFileToUserPOST streams a file download requested via form fields.
func (inst *Instance) UploadFileToUserPOST(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
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

// ExtractArchive extracts an archive into a server directory without progress streaming.
func (inst *Instance) ExtractArchive(ctx context.Context, gameServer *models.GameServer, archivePath string, destinationDirectoryPath string) ([]string, error) {
	validatedArchivePath, errArchivePath := validateLocalServerPath(gameServer, archivePath)
	if errArchivePath != nil {
		return nil, errArchivePath
	}
	validatedDestinationPath, errDestinationPath := validateLocalServerPath(gameServer, destinationDirectoryPath)
	if errDestinationPath != nil {
		return nil, errDestinationPath
	}

	archiveFullPath := filepath.Join(gameServer.Directory, validatedArchivePath)
	destinationFullPath := filepath.Join(gameServer.Directory, validatedDestinationPath)
	archiveName := filepath.Base(archiveFullPath)

	archiveFile, errOpen := os.Open(archiveFullPath)
	if errOpen != nil {
		log.Error().Err(errOpen).Msg("Failed to open archive")
		return nil, wrapFileActionError("open archive", errOpen)
	}
	defer func() { _ = archiveFile.Close() }()

	format, archiveStream, errIdentify := archives.Identify(ctx, archiveName, archiveFile)
	if errIdentify != nil {
		log.Error().Err(errIdentify).Msg("Failed to identify archive")
		return nil, wrapFileActionError("identify archive", errIdentify)
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
			return nil, wrapFileActionError("extract compressed archive", errExtract)
		}
	} else {
		errExtract := uncompressedArchiveExtractor.Extract(ctx, archiveStream, fx.extractFileHandler)
		if errExtract != nil {
			log.Error().Err(errExtract).Msg("Failed to extract archive")
			return nil, wrapFileActionError("extract archive", errExtract)
		}
	}

	extractedFiles := append([]string(nil), fx.extractedFilePathsSoFar...)
	return extractedFiles, nil
}

func (fx *fileExtractor) extractFileHandler(ctx context.Context, f archives.FileInfo) error {
	select {
	case <-ctx.Done():
		return wrapFileActionError("extract file context canceled", ctx.Err())
	default:
		isLocal := filepath.IsLocal(f.NameInArchive)
		if !isLocal {
			log.Error().Str("Game Server ID", fx.gameServer.ID).Msg("Invalid path")
			return ErrInvalidPath
		}

		archivedFile, errOpenArchivedFile := f.Open()
		if errOpenArchivedFile != nil {
			log.Error().Str("Game Server ID", fx.gameServer.ID).Err(errOpenArchivedFile).Msg("Failed to open archived file")
			return wrapFileActionError("open extracted archive file", errOpenArchivedFile)
		}
		defer func() { _ = archivedFile.Close() }()

		filePath := filepath.Join(fx.destinationPath, f.NameInArchive)
		relativeOutputPath, errRelative := filepath.Rel(fx.gameServer.Directory, filePath)
		if errRelative != nil {
			log.Error().Str("Game Server ID", fx.gameServer.ID).Err(errRelative).Msg("Failed to resolve extracted file path")
			return wrapFileActionError("resolve extracted file path", errRelative)
		}
		_, errProtected := validateWritableServerPath(fx.gameServer, relativeOutputPath)
		if errProtected != nil {
			return errProtected
		}
		// If the file is a directory, we just need to create it. If it's a file, we need to create it and copy the contents.
		if f.IsDir() {
			// Create the directory.
			errMkdirAll := os.MkdirAll(filePath, 0700)
			if errMkdirAll != nil {
				log.Error().Str("Game Server ID", fx.gameServer.ID).Err(errMkdirAll).Msg("Failed to create directory")
				return wrapFileActionError("create extracted directory", errMkdirAll)
			}
			return nil
		}
		// Make sure the directory for the file exists, before creating the file.
		basePath := filepath.Dir(filePath)
		errMkdirAll := os.MkdirAll(basePath, 0700)
		if errMkdirAll != nil {
			log.Error().Str("Game Server ID", fx.gameServer.ID).Err(errMkdirAll).Msg("Failed to create directory")
			return wrapFileActionError("create extracted parent directory", errMkdirAll)
		}

		newFile, errCreate := os.Create(filePath)
		if errCreate != nil {
			log.Error().Str("Game Server ID", fx.gameServer.ID).Err(errCreate).Msg("Failed to create file")
			return wrapFileActionError("create extracted file", errCreate)
		}
		defer func() { _ = newFile.Close() }()

		_, errCopy := io.Copy(newFile, archivedFile)
		if errCopy != nil {
			log.Error().Str("Game Server ID", fx.gameServer.ID).Err(errCopy).Msg("Failed to copy file")
			return wrapFileActionError("write extracted file", errCopy)
		}

		// Update file extractor data.
		fx.currentFilePath = f.NameInArchive
		fx.filesExtractedSoFar++
		fx.bytesExtractedSoFar += f.Size()
		fx.extractedFilePathsSoFar = append(fx.extractedFilePathsSoFar, f.NameInArchive)

		if fx.reportProgress {
			progress := &xylona.GameServerFilesExtractProgress{
				TotalFiles:     fx.totalFiles,
				FilesExtracted: fx.filesExtractedSoFar,
				TotalBytes:     fx.totalBytes,
				BytesExtracted: fx.bytesExtractedSoFar,
				CurrentFile:    f.NameInArchive,
			}
			timeout := time.After(time.Millisecond * 50)
			select {
			case <-ctx.Done():
				return wrapFileActionError("extract file progress context canceled", ctx.Err())
			case <-timeout:
				log.Warn().Msg("Failed to send progress")
				return nil
			case fx.progressChan <- progress:
			}
		}
		return nil
	}
}

// ListGameServerFiles lists files and directories for a relative server path.
func (inst *Instance) ListGameServerFiles(gameServer *models.GameServer, relativePath string) ([]*xylona.File, error) {
	// Check if path is empty or if it is a local path. If it is not a local path, return an error.
	if relativePath != "" && !filepath.IsLocal(relativePath) {
		log.Error().Err(errors.New("invalid path")).Msg("Path is not local")
		return nil, ErrInvalidPath
	}
	fullPath := filepath.Join(gameServer.Directory, relativePath)
	files, errReadDir := os.ReadDir(fullPath)
	if errReadDir != nil {
		if errors.Is(errReadDir, os.ErrNotExist) {
			log.Error().Err(errReadDir).Msg("Path does not exist")
			return nil, fmt.Errorf("actions: read server directory: %w", errReadDir)
		}
		log.Error().Err(errReadDir).Msg("Failed to read directory")
		return nil, fmt.Errorf("actions: read server directory: %w", errReadDir)
	}

	xylonaFiles := make([]*xylona.File, 0, len(files))
	for _, file := range files {
		fileInfo, errFileInfo := file.Info()
		if errFileInfo != nil {
			log.Error().Err(errFileInfo).Msg("Failed to get file info")
			return nil, fmt.Errorf("actions: stat directory entry: %w", errFileInfo)
		}
		size := fileInfo.Size()
		if fileInfo.IsDir() {
			var totalSize int64

			err := filepath.WalkDir(filepath.Join(fullPath, file.Name()), func(_ string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !d.IsDir() {
					entryInfo, errInfo := d.Info()
					if errInfo != nil {
						return fmt.Errorf("actions: stat nested directory entry: %w", errInfo)
					}
					totalSize += entryInfo.Size()
				}
				return nil
			})

			if err != nil {
				log.Error().Err(err).Msg("Failed to walk through directory")
				return nil, fmt.Errorf("actions: walk directory: %w", err)
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

// DownloadGameServerFile handles multipart uploads into a game server directory.
func (inst *Instance) DownloadGameServerFile(w http.ResponseWriter, r *http.Request) {
	multiReader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "Error creating multipart reader", http.StatusBadRequest)
		return
	}
	foundGameServerID := false
	foundPath := false
	gameServerID := ""
	relativePath := ""
	for {
		part, errNext := multiReader.NextPart()
		if errNext == io.EOF {
			break
		} else if errNext != nil {
			http.Error(w, "Error reading next part", http.StatusBadRequest)
			return
		}
		switch part.FormName() {
		case "gameServerId":
			gameServerIDBytes, errRead := io.ReadAll(io.LimitReader(part, 10<<10))
			if errRead != nil {
				log.Error().Err(errRead).Msg("Failed to read game server ID")
				http.Error(w, "Error reading game server ID", http.StatusBadRequest)
				return
			}
			gameServerID = string(gameServerIDBytes)
			foundGameServerID = true
		case "path":
			pathBytes, errRead := io.ReadAll(io.LimitReader(part, 1<<20))
			if errRead != nil {
				log.Error().Err(errRead).Msg("Failed to read path")
				http.Error(w, "Error reading path", http.StatusBadRequest)
				return
			}
			relativePath = string(pathBytes)
			foundPath = true
		case "file":
			if !foundGameServerID || !foundPath {
				log.Error().Msg("Game server ID and path must be specified")
				http.Error(w, "Game server ID and path must be specified", http.StatusBadRequest)
				return
			}
			filename := part.FileName()
			target, errResolve := inst.resolveFileRequestTarget(gameServerID)
			if errResolve != nil {
				log.Error().Err(errResolve).Msg("Failed to get game server")
				writeGameServerLookupError(w, errResolve)
				return
			}

			if target.isLocal() {
				errDownload := inst.saveUploadedGameServerFile(target.gameServer, relativePath, filename, part)
				if errDownload != nil {
					log.Error().Err(errDownload).Msg("Failed to download file")
					http.Error(w, "Failed to download file", http.StatusInternalServerError)
					return
				}
				continue
			}

			errProxy := inst.proxyRemoteFileUpload(r.Context(), target, relativePath, filename, part, w)
			if errProxy != nil {
				log.Error().Err(errProxy).Msg("Failed to proxy remote file upload")
				http.Error(w, "Failed to upload file", http.StatusInternalServerError)
				return
			}
			continue
		}
	}
}

func (inst *Instance) saveUploadedGameServerFile(gameServer *models.GameServer, relativePath, fileName string, fileSource io.Reader) error {
	validatedPath, errPath := validateLocalServerPath(gameServer, relativePath)
	if errPath != nil {
		return errPath
	}

	// Sanitize uploaded filename to prevent path traversal (e.g., "../../etc/passwd").
	sanitizedFileName := filepath.Base(fileName)
	if sanitizedFileName == "." || sanitizedFileName == string(filepath.Separator) {
		log.Error().Str("fileName", fileName).Msg("Invalid file name")
		return ErrInvalidPath
	}

	protectedRelativePath := filepath.Join(validatedPath, sanitizedFileName)
	_, errProtected := validateWritableServerPath(gameServer, protectedRelativePath)
	if errProtected != nil {
		return errProtected
	}

	cleanGameServerDir := filepath.Clean(gameServer.Directory)
	gameServerDirPlusPath := filepath.Clean(filepath.Join(cleanGameServerDir, validatedPath))
	gameServerDirPrefix := cleanGameServerDir + string(filepath.Separator)
	if gameServerDirPlusPath != cleanGameServerDir && !strings.HasPrefix(gameServerDirPlusPath, gameServerDirPrefix) {
		log.Error().Str("path", gameServerDirPlusPath).Msg("Upload directory escaped game server root")
		return ErrInvalidPath
	}

	errMkdirAll := os.MkdirAll(gameServerDirPlusPath, 0o750)
	if errMkdirAll != nil {
		log.Error().Err(errMkdirAll).Msg("Failed to create directory")
		return fmt.Errorf("actions: create upload directory: %w", errMkdirAll)
	}

	fullPath := filepath.Clean(filepath.Join(cleanGameServerDir, validatedPath, sanitizedFileName))
	if fullPath != cleanGameServerDir && !strings.HasPrefix(fullPath, gameServerDirPrefix) {
		log.Error().Str("path", fullPath).Msg("Upload file path escaped game server root")
		return ErrInvalidPath
	}

	file, errCreateFile := os.Create(fullPath)
	if errCreateFile != nil {
		log.Error().Err(errCreateFile).Msg("Failed to create file")
		return fmt.Errorf("actions: create uploaded file: %w", errCreateFile)
	}
	defer func() {
		_ = file.Close()
	}()

	_, errCopy := io.Copy(file, fileSource)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy file")
		return fmt.Errorf("actions: write uploaded file: %w", errCopy)
	}

	return nil
}

// GetGameServerFile streams a local game server file to the provided writer.
func (inst *Instance) GetGameServerFile(gameServer *models.GameServer, relativePath string, writer io.Writer, setHeaders, setAsAttachment bool) error {
	validatedPath, errPath := validateLocalServerPath(gameServer, relativePath)
	if errPath != nil {
		return errPath
	}

	cleanGameServerDir := filepath.Clean(gameServer.Directory)
	fullPath := filepath.Clean(filepath.Join(cleanGameServerDir, validatedPath))
	gameServerDirPrefix := cleanGameServerDir + string(filepath.Separator)
	if fullPath != cleanGameServerDir && !strings.HasPrefix(fullPath, gameServerDirPrefix) {
		log.Error().Str("path", fullPath).Msg("Read path escaped game server root")
		return ErrInvalidPath
	}

	file, errReadFile := os.Open(fullPath)
	if errReadFile != nil {
		if errors.Is(errReadFile, os.ErrNotExist) {
			log.Error().Err(errReadFile).Msg("File does not exist")
			return fmt.Errorf("actions: open game server file: %w", errReadFile)
		}
		log.Error().Err(errReadFile).Msg("Failed to read file")
		return fmt.Errorf("actions: open game server file: %w", errReadFile)
	}
	defer func() { _ = file.Close() }()

	fileInfo, errFileInfo := file.Stat()
	if errFileInfo != nil {
		log.Error().Err(errFileInfo).Msg("Failed to get file info")
		return fmt.Errorf("actions: stat game server file: %w", errFileInfo)
	}

	if setHeaders {
		w, ok := writer.(http.ResponseWriter)
		if !ok {
			log.Error().Msg("Writer is not an http.ResponseWriter")
			return errors.New("writer is not an http.ResponseWriter")
		}

		mimeType, errDetect := mimetype.DetectFile(fullPath)
		if errDetect != nil {
			w.Header().Set("Content-Type", "application/octet-stream")
		} else {
			w.Header().Set("Content-Type", mimeType.String())
		}

		w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
		if setAsAttachment {
			// Sanitize filename to prevent header injection via quotes or newlines.
			safeName := strings.Map(func(r rune) rune {
				if r == '"' || r == '\\' || r == '\n' || r == '\r' {
					return '_'
				}
				return r
			}, fileInfo.Name())
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
		}
	}

	_, errCopy := io.Copy(writer, file)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy file")
		return fmt.Errorf("actions: stream game server file: %w", errCopy)
	}
	return nil
}

func (inst *Instance) createGameServerDirectory(gameServer *models.GameServer, owner *models.User) (string, error) {
	gsNameSlug := slug.Make(gameServer.Name)
	gameServerDir := filepath.Join(DefaultInstallPath(), owner.UserName, gsNameSlug)
	errMakePath := os.MkdirAll(gameServerDir, 0o750)
	if errMakePath != nil {
		log.Error().Err(errMakePath).Msg("Failed to create game server directory")
		return "", fmt.Errorf("actions: create game server directory: %w", errMakePath)
	}
	return gameServerDir, nil
}

// PurgeAllGameServerFiles deletes the server's working directory.
func (inst *Instance) PurgeAllGameServerFiles(gameServer *models.GameServer) error {
	err := os.RemoveAll(gameServer.Directory)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete game server files")
		return fmt.Errorf("actions: delete game server files: %w", err)
	}
	return nil
}
