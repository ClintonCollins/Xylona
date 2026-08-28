package node

import (
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/mholt/archives"
)

var (
	maxArchiveExtractEntries    int64 = 100_000
	maxArchiveExtractEntryBytes int64 = 8 << 30
	maxArchiveExtractTotalBytes int64 = 64 << 30
)

// CreateFileArchive creates an archive inside directory using only node-local
// filesystem access. includePaths and destinationArchivePath are relative to
// directory; the returned path is slash-style and relative to directory.
func (n *Node) CreateFileArchive(
	ctx context.Context,
	directory string,
	destinationArchivePath string,
	includePaths []string,
	compression ArchiveCompression,
	policy ProtectionPolicy,
) (string, ArchiveProgress, error) {
	return n.CreateFileArchiveWithProgress(ctx, directory, destinationArchivePath, includePaths, compression, policy, nil)
}

// CreateFileArchiveWithProgress is CreateFileArchive plus optional progress
// callbacks suitable for forwarding over the controller's streaming API.
func (n *Node) CreateFileArchiveWithProgress(
	ctx context.Context,
	directory string,
	destinationArchivePath string,
	includePaths []string,
	compression ArchiveCompression,
	policy ProtectionPolicy,
	onProgress func(ArchiveProgress) error,
) (string, ArchiveProgress, error) {
	validatedDestination, errDestination := validateLocalPath(destinationArchivePath)
	if errDestination != nil {
		return "", ArchiveProgress{}, errDestination
	}

	outputRelative := validatedDestination + archiveCompressionExtension(compression)
	errProtected := enforceProtection(outputRelative, policy)
	if errProtected != nil {
		return "", ArchiveProgress{}, errProtected
	}

	outputPath, errResolve := resolveWithinRoot(directory, outputRelative)
	if errResolve != nil {
		return "", ArchiveProgress{}, errResolve
	}

	pathFilesMap := make(map[string]string, len(includePaths))
	for _, includePath := range includePaths {
		validatedSource, errSource := validateLocalPath(includePath)
		if errSource != nil {
			return "", ArchiveProgress{}, errSource
		}
		sourcePath, errSourceResolve := resolveWithinRoot(directory, validatedSource)
		if errSourceResolve != nil {
			return "", ArchiveProgress{}, errSourceResolve
		}
		pathFilesMap[sourcePath] = filepath.Base(validatedSource)
	}

	diskOptions := &archives.FromDiskOptions{
		FollowSymlinks:  false,
		ClearAttributes: false,
	}
	archiveFiles, errFiles := archives.FilesFromDisk(ctx, diskOptions, pathFilesMap)
	if errFiles != nil {
		return "", ArchiveProgress{}, fmt.Errorf("node: collect archive files: %w", errFiles)
	}

	progress := archiveProgressForFiles(archiveFiles)
	errProgress := sendArchiveProgress(onProgress, progress)
	if errProgress != nil {
		return "", ArchiveProgress{}, errProgress
	}
	errArchive := writeNodeArchive(ctx, directory, outputRelative, outputPath, archiveFiles, compression, progress, onProgress)
	if errArchive != nil {
		return "", ArchiveProgress{}, errArchive
	}
	return filepath.ToSlash(outputRelative), completedArchiveProgress(progress), nil
}

func archiveProgressForFiles(files []archives.FileInfo) ArchiveProgress {
	progress := ArchiveProgress{
		TotalFiles: int64(len(files)),
	}
	for _, file := range files {
		progress.TotalBytes += file.Size()
		progress.CurrentFile = file.NameInArchive
	}
	return progress
}

func completedArchiveProgress(progress ArchiveProgress) ArchiveProgress {
	progress.FilesCompressed = progress.TotalFiles
	progress.BytesCompressed = progress.TotalBytes
	return progress
}

func writeNodeArchive(ctx context.Context, rootDirectory string, outputRelative string, outputPath string, archiveFiles []archives.FileInfo, compression ArchiveCompression, progress ArchiveProgress, onProgress func(ArchiveProgress) error) error {
	format := archiveFormat(compression)
	archiveOut, errCreate := os.Create(outputPath)
	if errCreate != nil {
		return fmt.Errorf("node: create file archive: %w", errCreate)
	}

	errArchive := archiveWithProgress(ctx, format, archiveOut, archiveFiles, progress, onProgress)
	errClose := archiveOut.Close()
	if errArchive != nil {
		errCleanup := cleanupPartialNodeArchive(rootDirectory, outputRelative)
		if errCleanup != nil {
			return errors.Join(fmt.Errorf("node: archive files: %w", errArchive), errCleanup)
		}
		return fmt.Errorf("node: archive files: %w", errArchive)
	}
	if errClose != nil {
		errCleanup := cleanupPartialNodeArchive(rootDirectory, outputRelative)
		if errCleanup != nil {
			return errors.Join(fmt.Errorf("node: close file archive: %w", errClose), errCleanup)
		}
		return fmt.Errorf("node: close file archive: %w", errClose)
	}
	return nil
}

func archiveWithProgress(ctx context.Context, format archives.CompressedArchive, archiveOut *os.File, archiveFiles []archives.FileInfo, progress ArchiveProgress, onProgress func(ArchiveProgress) error) error {
	archiveJobs := make(chan archives.ArchiveAsyncJob, len(archiveFiles))
	resultChans := make([]chan error, 0, len(archiveFiles))
	for _, archiveFile := range archiveFiles {
		resultChan := make(chan error, 1)
		resultChans = append(resultChans, resultChan)
		archiveJobs <- archives.ArchiveAsyncJob{
			File:   archiveFile,
			Result: resultChan,
		}
	}
	close(archiveJobs)

	progressErrChan := make(chan error, 1)
	go func() {
		workingProgress := progress
		for index, resultChan := range resultChans {
			errResult := <-resultChan
			if errResult != nil {
				progressErrChan <- errResult
				return
			}
			workingProgress.FilesCompressed++
			workingProgress.BytesCompressed += archiveFiles[index].Size()
			workingProgress.CurrentFile = archiveFiles[index].NameInArchive
			errProgress := sendArchiveProgress(onProgress, workingProgress)
			if errProgress != nil {
				progressErrChan <- errProgress
				return
			}
		}
		progressErrChan <- nil
	}()

	errArchive := format.ArchiveAsync(ctx, archiveOut, archiveJobs)
	errProgress := <-progressErrChan
	if errArchive != nil {
		return fmt.Errorf("node: archive files: %w", errArchive)
	}
	if errProgress != nil {
		return fmt.Errorf("node: archive progress: %w", errProgress)
	}
	return nil
}

func sendArchiveProgress(onProgress func(ArchiveProgress) error, progress ArchiveProgress) error {
	if onProgress == nil {
		return nil
	}
	errProgress := onProgress(progress)
	if errProgress != nil {
		return fmt.Errorf("node: send archive progress: %w", errProgress)
	}
	return nil
}

func cleanupPartialNodeArchive(rootDirectory string, relativePath string) error {
	validated, errPath := validateLocalPath(relativePath)
	if errPath != nil {
		return errPath
	}

	archiveRoot, errRoot := os.OpenRoot(rootDirectory)
	if errRoot != nil {
		return fmt.Errorf("node: open archive root: %w", errRoot)
	}

	errRemove := archiveRoot.Remove(validated)
	errClose := archiveRoot.Close()
	if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
		if errClose != nil {
			return errors.Join(fmt.Errorf("node: remove partial file archive: %w", errRemove), fmt.Errorf("node: close archive root: %w", errClose))
		}
		return fmt.Errorf("node: remove partial file archive: %w", errRemove)
	}
	if errClose != nil {
		return fmt.Errorf("node: close archive root: %w", errClose)
	}
	return nil
}

func archiveFormat(compression ArchiveCompression) archives.CompressedArchive {
	format := archives.CompressedArchive{
		Archival: &archives.Tar{},
	}

	switch compression {
	case ArchiveCompressionZIP:
		format.Archival = &archives.Zip{
			SelectiveCompression: true,
			Compression:          zip.Deflate,
			ContinueOnError:      false,
		}
	case ArchiveCompressionGZIP:
		format.Compression = &archives.Gz{
			CompressionLevel: gzip.DefaultCompression,
			Multithreaded:    true,
		}
	case ArchiveCompressionBZIP2:
		format.Compression = &archives.Bz2{}
	case ArchiveCompressionZST:
		format.Compression = &archives.Zstd{}
	case ArchiveCompressionXZ:
		format.Compression = &archives.Xz{}
	}
	return format
}

func archiveCompressionExtension(compression ArchiveCompression) string {
	switch compression {
	case ArchiveCompressionGZIP:
		return ".tar.gz"
	case ArchiveCompressionBZIP2:
		return ".tar.bz2"
	case ArchiveCompressionZST:
		return ".tar.zst"
	case ArchiveCompressionXZ:
		return ".tar.xz"
	default:
		return ".zip"
	}
}

// ExtractFileArchive extracts archivePath into destinationDirectoryPath using
// only node-local filesystem access. Returned paths are slash-style archive
// entry names, matching the controller's local extraction response.
func (n *Node) ExtractFileArchive(
	ctx context.Context,
	directory string,
	archivePath string,
	destinationDirectoryPath string,
	policy ProtectionPolicy,
) ([]string, ExtractProgress, error) {
	return n.ExtractFileArchiveWithProgress(ctx, directory, archivePath, destinationDirectoryPath, policy, nil)
}

// ExtractFileArchiveWithProgress is ExtractFileArchive plus optional progress
// callbacks suitable for forwarding over the controller's streaming API.
func (n *Node) ExtractFileArchiveWithProgress(
	ctx context.Context,
	directory string,
	archivePath string,
	destinationDirectoryPath string,
	policy ProtectionPolicy,
	onProgress func(ExtractProgress) error,
) ([]string, ExtractProgress, error) {
	validatedArchivePath, errArchivePath := validateLocalPath(archivePath)
	if errArchivePath != nil {
		return nil, ExtractProgress{}, errArchivePath
	}
	validatedDestination, errDestination := validateLocalPath(destinationDirectoryPath)
	if errDestination != nil {
		return nil, ExtractProgress{}, errDestination
	}

	fullArchivePath, errResolveArchive := resolveWithinRoot(directory, validatedArchivePath)
	if errResolveArchive != nil {
		return nil, ExtractProgress{}, errResolveArchive
	}
	progress, errProgress := inspectNodeArchive(ctx, fullArchivePath)
	if errProgress != nil {
		return nil, ExtractProgress{}, errProgress
	}
	errSendInitial := sendExtractProgress(onProgress, progress)
	if errSendInitial != nil {
		return nil, ExtractProgress{}, errSendInitial
	}

	archiveFile, errOpen := os.Open(fullArchivePath)
	if errOpen != nil {
		return nil, ExtractProgress{}, fmt.Errorf("node: open file archive: %w", errOpen)
	}

	format, archiveStream, errIdentify := archives.Identify(ctx, filepath.Base(fullArchivePath), archiveFile)
	if errIdentify != nil {
		errClose := archiveFile.Close()
		if errClose != nil {
			return nil, ExtractProgress{}, errors.Join(fmt.Errorf("node: identify file archive: %w", errIdentify), fmt.Errorf("node: close file archive: %w", errClose))
		}
		return nil, ExtractProgress{}, fmt.Errorf("node: identify file archive: %w", errIdentify)
	}

	destinationRoot, errRoot := os.OpenRoot(directory)
	if errRoot != nil {
		errClose := archiveFile.Close()
		if errClose != nil {
			return nil, ExtractProgress{}, errors.Join(fmt.Errorf("node: open archive destination root: %w", errRoot), fmt.Errorf("node: close file archive: %w", errClose))
		}
		return nil, ExtractProgress{}, fmt.Errorf("node: open archive destination root: %w", errRoot)
	}

	extractor := &nodeArchiveExtractor{
		root:                 destinationRoot,
		destinationDirectory: validatedDestination,
		policy:               policy,
		progress:             progress,
		onProgress:           onProgress,
	}

	errExtract := extractNodeArchive(ctx, format, archiveStream, extractor)
	errCloseRoot := destinationRoot.Close()
	if errCloseRoot != nil {
		errCloseRoot = fmt.Errorf("node: close archive destination root: %w", errCloseRoot)
	}
	errCloseArchive := archiveFile.Close()
	if errCloseArchive != nil {
		errCloseArchive = fmt.Errorf("node: close file archive: %w", errCloseArchive)
	}
	errExtract = errors.Join(errExtract, errCloseRoot, errCloseArchive)
	if errExtract != nil {
		return nil, ExtractProgress{}, errExtract
	}

	return append([]string(nil), extractor.extractedPaths...), extractor.progress, nil
}

func inspectNodeArchive(ctx context.Context, archivePath string) (ExtractProgress, error) {
	archiveFS, errFS := archives.FileSystem(ctx, archivePath, nil)
	if errFS != nil {
		return ExtractProgress{}, fmt.Errorf("node: open file archive file system: %w", errFS)
	}

	progress := ExtractProgress{}
	var entryCount int64
	errWalk := fs.WalkDir(archiveFS, ".", func(pathValue string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if pathValue == "." {
			return nil
		}
		entryCount++
		if entryCount > maxArchiveExtractEntries {
			return fmt.Errorf("node: archive contains more than %d entries", maxArchiveExtractEntries)
		}
		if d.IsDir() {
			return nil
		}
		info, errInfo := d.Info()
		if errInfo != nil {
			return fmt.Errorf("node: stat archive entry: %w", errInfo)
		}
		progress.TotalFiles++
		progress.TotalBytes += info.Size()
		progress.CurrentFile = pathValue
		return nil
	})
	if errWalk != nil {
		return ExtractProgress{}, fmt.Errorf("node: walk file archive: %w", errWalk)
	}
	return progress, nil
}

type nodeArchiveExtractor struct {
	root                 *os.Root
	destinationDirectory string
	policy               ProtectionPolicy
	progress             ExtractProgress
	onProgress           func(ExtractProgress) error
	extractedPaths       []string
	entryCount           int64
}

func extractNodeArchive(ctx context.Context, format archives.Format, archiveStream io.Reader, extractor *nodeArchiveExtractor) error {
	compressedArchive, compressedArchiveOk := format.(archives.CompressedArchive)
	if compressedArchiveOk {
		errExtract := compressedArchive.Extract(ctx, archiveStream, extractor.extractFile)
		if errExtract != nil {
			return fmt.Errorf("node: extract compressed file archive: %w", errExtract)
		}
		return nil
	}

	uncompressedArchive, ok := format.(archives.Extractor)
	if !ok {
		return errors.New("node: unable to identify file archive type")
	}
	errExtract := uncompressedArchive.Extract(ctx, archiveStream, extractor.extractFile)
	if errExtract != nil {
		return fmt.Errorf("node: extract file archive: %w", errExtract)
	}
	return nil
}

func (e *nodeArchiveExtractor) extractFile(ctx context.Context, file archives.FileInfo) error {
	errContext := ctx.Err()
	if errContext != nil {
		return fmt.Errorf("node: extract file archive canceled: %w", errContext)
	}
	e.entryCount++
	if e.entryCount > maxArchiveExtractEntries {
		return fmt.Errorf("node: archive contains more than %d entries", maxArchiveExtractEntries)
	}

	localEntryPath, slashEntryPath, errEntry := cleanArchiveEntryPath(file.NameInArchive)
	if errEntry != nil {
		return errEntry
	}

	outputRelativePath := filepath.Join(e.destinationDirectory, localEntryPath)
	errProtected := enforceProtection(outputRelativePath, e.policy)
	if errProtected != nil {
		return errProtected
	}

	if file.IsDir() {
		errMkdir := e.root.MkdirAll(outputRelativePath, 0o700)
		if errMkdir != nil {
			return fmt.Errorf("node: create extracted directory: %w", errMkdir)
		}
		return nil
	}

	errParent := e.root.MkdirAll(filepath.Dir(outputRelativePath), 0o700)
	if errParent != nil {
		return fmt.Errorf("node: create extracted parent directory: %w", errParent)
	}

	archiveFile, errOpen := file.Open()
	if errOpen != nil {
		return fmt.Errorf("node: open archive entry: %w", errOpen)
	}

	outputFile, errCreate := e.root.Create(outputRelativePath)
	if errCreate != nil {
		errCloseEntry := archiveFile.Close()
		if errCloseEntry != nil {
			return errors.Join(fmt.Errorf("node: create extracted file: %w", errCreate), fmt.Errorf("node: close archive entry: %w", errCloseEntry))
		}
		return fmt.Errorf("node: create extracted file: %w", errCreate)
	}

	errCopy := copyArchiveEntry(outputFile, archiveFile, &e.progress.BytesExtracted)
	errCloseOutput := outputFile.Close()
	errCloseEntry := archiveFile.Close()
	if errCopy != nil {
		errRemove := e.root.Remove(outputRelativePath)
		if errors.Is(errRemove, fs.ErrNotExist) {
			errRemove = nil
		} else if errRemove != nil {
			errRemove = fmt.Errorf("node: remove partial extracted file: %w", errRemove)
		}
		return errors.Join(fmt.Errorf("node: write extracted file: %w", errCopy), closeErrors(errCloseOutput, errCloseEntry), errRemove)
	}
	if errCloseOutput != nil || errCloseEntry != nil {
		return closeErrors(errCloseOutput, errCloseEntry)
	}

	e.progress.FilesExtracted++
	e.progress.CurrentFile = slashEntryPath
	e.extractedPaths = append(e.extractedPaths, slashEntryPath)
	errProgress := sendExtractProgress(e.onProgress, e.progress)
	if errProgress != nil {
		return errProgress
	}
	return nil
}

func copyArchiveEntry(dst io.Writer, src io.Reader, total *int64) error {
	remainingTotal := max(0, maxArchiveExtractTotalBytes-*total)
	copyLimit := min(maxArchiveExtractEntryBytes, remainingTotal)
	written, errCopy := io.Copy(dst, io.LimitReader(src, copyLimit+1))
	*total += written
	if errCopy != nil {
		return fmt.Errorf("copy archive entry: %w", errCopy)
	}
	if written > maxArchiveExtractEntryBytes {
		return fmt.Errorf("archive entry exceeds %d bytes", maxArchiveExtractEntryBytes)
	}
	if *total > maxArchiveExtractTotalBytes {
		return fmt.Errorf("archive exceeds %d expanded bytes", maxArchiveExtractTotalBytes)
	}
	return nil
}

func sendExtractProgress(onProgress func(ExtractProgress) error, progress ExtractProgress) error {
	if onProgress == nil {
		return nil
	}
	errProgress := onProgress(progress)
	if errProgress != nil {
		return fmt.Errorf("node: send extract progress: %w", errProgress)
	}
	return nil
}

func cleanArchiveEntryPath(entryPath string) (string, string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(entryPath), `\`, "/")
	if normalized == "" || strings.HasPrefix(normalized, "/") || hasWindowsDrivePrefix(normalized) {
		return "", "", ErrInvalidPath
	}

	cleaned := path.Clean(normalized)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", "", ErrInvalidPath
	}
	if hasWindowsDrivePrefix(cleaned) {
		return "", "", ErrInvalidPath
	}

	localPath := filepath.FromSlash(cleaned)
	if !filepath.IsLocal(localPath) {
		return "", "", ErrInvalidPath
	}
	return localPath, cleaned, nil
}

func hasWindowsDrivePrefix(pathValue string) bool {
	if len(pathValue) < 2 {
		return false
	}
	first := pathValue[0]
	if (first < 'A' || first > 'Z') && (first < 'a' || first > 'z') {
		return false
	}
	return pathValue[1] == ':'
}

func closeErrors(errs ...error) error {
	var joined error
	for _, err := range errs {
		if err == nil {
			continue
		}
		joined = errors.Join(joined, fmt.Errorf("node: close file: %w", err))
	}
	return joined
}
