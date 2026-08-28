package node

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"
)

// CreateBackupArchive writes a zip archive of directory (optionally restricted
// to includePaths, relative to directory) to destinationArchivePath. Returns
// the archive size in bytes and its SHA-256 digest so the controller can
// integrity-check the transfer.
//
// When includePaths is empty the whole directory is archived (matches the
// historical controller-side behavior). Each entry in includePaths is treated
// as a subdirectory or file relative to directory.
//
// The archive path must already be inside an existing parent directory; the
// node does not create arbitrary parent directories on behalf of the
// controller.
func (n *Node) CreateBackupArchive(ctx context.Context, directory string, includePaths []string, destinationArchivePath string) (int64, string, error) {
	if strings.TrimSpace(directory) == "" {
		return 0, "", errors.New("node: backup directory is required")
	}
	if strings.TrimSpace(destinationArchivePath) == "" {
		return 0, "", errors.New("node: destination archive path is required")
	}

	cleanRoot := filepath.Clean(directory)
	cleanArchivePath := filepath.Clean(destinationArchivePath)

	// Reject relative or path-escaping include entries up front so the walk
	// loop below stays simple.
	normalizedIncludes, errIncludes := normalizeIncludePaths(cleanRoot, includePaths)
	if errIncludes != nil {
		return 0, "", errIncludes
	}

	parentDir := filepath.Dir(cleanArchivePath)
	errMkdir := os.MkdirAll(parentDir, 0o750)
	if errMkdir != nil {
		return 0, "", fmt.Errorf("node: create backup archive parent dir: %w", errMkdir)
	}

	outputFile, errCreate := os.OpenFile(cleanArchivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errCreate != nil {
		return 0, "", fmt.Errorf("node: create backup archive: %w", errCreate)
	}

	hasher := sha256.New()
	counting := &countingWriter{writer: io.MultiWriter(outputFile, hasher)}
	zipWriter := zip.NewWriter(counting)

	errWalk := walkBackupSources(ctx, cleanRoot, cleanArchivePath, normalizedIncludes, zipWriter)

	errCloseZip := zipWriter.Close()
	errCloseFile := outputFile.Close()
	if errWalk != nil {
		cleanupBackupArchive(cleanArchivePath)
		return 0, "", errWalk
	}
	if errCloseZip != nil {
		cleanupBackupArchive(cleanArchivePath)
		return 0, "", fmt.Errorf("node: finalize backup archive: %w", errCloseZip)
	}
	if errCloseFile != nil {
		cleanupBackupArchive(cleanArchivePath)
		return 0, "", fmt.Errorf("node: close backup archive: %w", errCloseFile)
	}

	archiveInfo, errStat := os.Stat(cleanArchivePath)
	if errStat != nil {
		return 0, "", fmt.Errorf("node: stat backup archive: %w", errStat)
	}
	return archiveInfo.Size(), hex.EncodeToString(hasher.Sum(nil)), nil
}

func walkBackupSources(ctx context.Context, root, archivePath string, includes []string, zipWriter *zip.Writer) error {
	return walkBackupSourcesWith(ctx, root, archivePath, includes, zipWriter, filepath.WalkDir)
}

type backupWalkDirFunc func(root string, walkFn fs.WalkDirFunc) error

func walkBackupSourcesWith(
	ctx context.Context,
	root string,
	archivePath string,
	includes []string,
	zipWriter *zip.Writer,
	walkDir backupWalkDirFunc,
) error {
	walkRoots := includes
	if len(walkRoots) == 0 {
		walkRoots = []string{root}
	}

	for _, walkRoot := range walkRoots {
		errOuterCtx := ctx.Err()
		if errOuterCtx != nil {
			return fmt.Errorf("node: backup canceled: %w", errOuterCtx)
		}

		errWalk := walkDir(walkRoot, func(currentPath string, entry fs.DirEntry, walkErr error) error {
			errInnerCtx := ctx.Err()
			if errInnerCtx != nil {
				return fmt.Errorf("node: backup canceled: %w", errInnerCtx)
			}
			if walkErr != nil {
				if currentPath != walkRoot && errors.Is(walkErr, fs.ErrNotExist) {
					return nil
				}
				return fmt.Errorf("node: walk backup source: %w", walkErr)
			}
			if currentPath == root {
				return nil
			}

			cleanCurrent := filepath.Clean(currentPath)
			if cleanCurrent == archivePath {
				// Don't archive ourselves.
				return nil
			}

			archiveEntryPath, errEntry := backupArchiveEntryPath(root, cleanCurrent)
			if errEntry != nil {
				return errEntry
			}

			info, errInfo := entry.Info()
			if errInfo != nil {
				if currentPath != walkRoot && errors.Is(errInfo, fs.ErrNotExist) {
					if entry.IsDir() {
						return fs.SkipDir
					}
					return nil
				}
				return fmt.Errorf("node: stat backup source: %w", errInfo)
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("node: symlinks are not supported in backups: %s", archiveEntryPath)
			}
			if info.IsDir() {
				return addBackupDirectoryEntry(zipWriter, archiveEntryPath, info)
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			errAddFile := addBackupFileEntry(zipWriter, cleanCurrent, archiveEntryPath, info)
			if currentPath != walkRoot && errors.Is(errAddFile, fs.ErrNotExist) {
				return nil
			}
			return errAddFile
		})
		if errWalk != nil {
			return fmt.Errorf("node: walk backup source: %w", errWalk)
		}
	}
	return nil
}

func backupArchiveEntryPath(root, currentPath string) (string, error) {
	relative, errRel := filepath.Rel(root, currentPath)
	if errRel != nil {
		return "", fmt.Errorf("node: resolve backup relative path: %w", errRel)
	}
	if !filepath.IsLocal(relative) {
		return "", fmt.Errorf("node: backup source escapes root: %s", relative)
	}
	return filepath.ToSlash(relative), nil
}

func addBackupDirectoryEntry(zipWriter *zip.Writer, archiveEntryPath string, info fs.FileInfo) error {
	header, errHeader := zip.FileInfoHeader(info)
	if errHeader != nil {
		return fmt.Errorf("node: zip directory header: %w", errHeader)
	}
	header.Name = archiveEntryPath + "/"
	header.Method = zip.Store
	_, errCreate := zipWriter.CreateHeader(header)
	if errCreate != nil {
		return fmt.Errorf("node: zip create directory: %w", errCreate)
	}
	return nil
}

func addBackupFileEntry(zipWriter *zip.Writer, sourcePath, archiveEntryPath string, info fs.FileInfo) error {
	// #nosec G304 -- source path was produced by filepath.WalkDir under a
	// controller-supplied root that internal/node's caller has already validated.
	srcFile, errOpen := os.Open(sourcePath)
	if errOpen != nil {
		return fmt.Errorf("node: open backup source: %w", errOpen)
	}
	defer func() {
		errClose := srcFile.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Str("path", sourcePath).Msg("node: close backup source")
		}
	}()

	header, errHeader := zip.FileInfoHeader(info)
	if errHeader != nil {
		return fmt.Errorf("node: zip file header: %w", errHeader)
	}
	header.Name = archiveEntryPath
	header.Method = zip.Deflate

	writer, errCreate := zipWriter.CreateHeader(header)
	if errCreate != nil {
		return fmt.Errorf("node: zip create file: %w", errCreate)
	}

	_, errCopy := io.Copy(writer, srcFile)
	if errCopy != nil {
		return fmt.Errorf("node: copy backup source into archive: %w", errCopy)
	}
	return nil
}

// normalizeIncludePaths validates each caller-supplied include entry and
// returns absolute walk roots under root. Empty input returns nil (caller
// treats that as "walk the whole root").
func normalizeIncludePaths(root string, includePaths []string) ([]string, error) {
	if len(includePaths) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(includePaths))
	for _, include := range includePaths {
		trimmed := strings.TrimSpace(include)
		if trimmed == "" {
			continue
		}
		if !filepath.IsLocal(trimmed) {
			return nil, fmt.Errorf("node: backup include path is not local: %s", include)
		}
		out = append(out, filepath.Join(root, trimmed))
	}
	return out, nil
}

// countingWriter tracks how many bytes have been written. Used to hash the
// full archive stream alongside the file write.
type countingWriter struct {
	writer io.Writer
	total  int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.writer.Write(p)
	c.total += int64(n)
	if err != nil {
		return n, fmt.Errorf("node: countingWriter write: %w", err)
	}
	return n, nil
}

func cleanupBackupArchive(archivePath string) {
	errRemove := os.Remove(archivePath)
	if errRemove != nil && !errors.Is(errRemove, fs.ErrNotExist) {
		log.Warn().Err(errRemove).Str("path", archivePath).Msg("node: remove partial backup archive")
	}
}

// ExtractMode mirrors the proto ExtractMode enum so the node package can
// reason about extraction semantics without importing proto types.
type ExtractMode int

const (
	// ExtractModeOverlay preserves files that exist in the directory but not
	// in the archive (additive extraction).
	ExtractModeOverlay ExtractMode = iota
	// ExtractModeExact removes any file in the directory that is not also
	// in the archive before extracting.
	ExtractModeExact
)

// ErrRestoreDestinationSymlink reports a live restore target that would cause
// extraction to follow a symlink outside the intended tree.
var ErrRestoreDestinationSymlink = errors.New("restore destination contains a symlink")

// ExtractBackupArchive unpacks archivePath into directory using the given
// mode. Paths inside the archive that escape the directory or target a
// symlink are rejected. Intended for backup-restore flows; callers should
// have already stopped the game server process.
func (n *Node) ExtractBackupArchive(ctx context.Context, directory, archivePath string, mode ExtractMode) error {
	if strings.TrimSpace(directory) == "" {
		return errors.New("node: extract directory is required")
	}
	if strings.TrimSpace(archivePath) == "" {
		return errors.New("node: archive_path is required")
	}

	cleanRoot := filepath.Clean(directory)
	cleanArchive := filepath.Clean(archivePath)

	reader, errOpen := zip.OpenReader(cleanArchive)
	if errOpen != nil {
		return fmt.Errorf("node: open backup archive: %w", errOpen)
	}
	defer func() {
		errClose := reader.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("node: close backup archive")
		}
	}()

	stageRoot, errStage := os.MkdirTemp("", "xylona-backup-restore-*")
	if errStage != nil {
		return fmt.Errorf("node: create backup restore staging dir: %w", errStage)
	}
	defer cleanupBackupRestoreStaging(stageRoot)

	archivePaths, directoryModes, errStageExtract := extractBackupArchiveToStaging(ctx, stageRoot, reader.File)
	if errStageExtract != nil {
		return errStageExtract
	}

	errMkdir := os.MkdirAll(cleanRoot, 0o750)
	if errMkdir != nil {
		return fmt.Errorf("node: prepare extract directory: %w", errMkdir)
	}
	liveRoot, errRoot := os.OpenRoot(cleanRoot)
	if errRoot != nil {
		return fmt.Errorf("node: open backup restore root: %w", errRoot)
	}

	var errRestore error
	if mode == ExtractModeExact {
		errPrune := pruneDirectoryForExactExtract(liveRoot, archivePaths)
		if errPrune != nil {
			errRestore = errPrune
		}
	}

	if errRestore == nil {
		errRestore = applyStagedBackupExtract(ctx, stageRoot, liveRoot, directoryModes)
	}
	errCloseRoot := liveRoot.Close()
	if errCloseRoot != nil {
		errCloseRoot = fmt.Errorf("node: close backup restore root: %w", errCloseRoot)
	}
	return errors.Join(errRestore, errCloseRoot)
}

func extractBackupArchiveToStaging(ctx context.Context, stageRoot string, files []*zip.File) (map[string]struct{}, map[string]fs.FileMode, error) {
	if int64(len(files)) > maxArchiveExtractEntries {
		return nil, nil, fmt.Errorf("node: backup archive contains more than %d entries", maxArchiveExtractEntries)
	}
	archivePaths := make(map[string]struct{}, len(files))
	directoryModes := make(map[string]fs.FileMode, len(files))
	var totalBytes int64
	for _, f := range files {
		errCtx := ctx.Err()
		if errCtx != nil {
			return nil, nil, fmt.Errorf("node: extract canceled: %w", errCtx)
		}

		relative, errRelative := backupExtractRelativePath(f.Name)
		if errRelative != nil {
			return nil, nil, errRelative
		}
		archivePaths[relative] = struct{}{}

		errExtract := extractBackupEntry(stageRoot, f, directoryModes, &totalBytes)
		if errExtract != nil {
			return nil, nil, errExtract
		}
	}
	return archivePaths, directoryModes, nil
}

func extractBackupEntry(root string, f *zip.File, directoryModes map[string]fs.FileMode, totalBytes *int64) error {
	relative, errRelative := backupExtractRelativePath(f.Name)
	if errRelative != nil {
		return errRelative
	}
	full := filepath.Join(root, filepath.FromSlash(relative))

	// Reject symlinks for safety.
	if f.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("node: symlinks not allowed in backup archive: %s", f.Name)
	}

	if f.FileInfo().IsDir() {
		perm := backupDirectoryPerm(f.Mode().Perm())
		directoryModes[relative] = perm
		errMkdir := os.MkdirAll(full, writableBackupDirectoryPerm(perm))
		if errMkdir != nil {
			return fmt.Errorf("node: create archive directory %s: %w", f.Name, errMkdir)
		}
		errChmod := os.Chmod(full, writableBackupDirectoryPerm(perm))
		if errChmod != nil {
			return fmt.Errorf("node: set archive directory mode %s: %w", f.Name, errChmod)
		}
		return nil
	}

	errMkParent := os.MkdirAll(filepath.Dir(full), 0o750)
	if errMkParent != nil {
		return fmt.Errorf("node: create parent for %s: %w", f.Name, errMkParent)
	}

	src, errOpen := f.Open()
	if errOpen != nil {
		return fmt.Errorf("node: open archive entry %s: %w", f.Name, errOpen)
	}
	defer func() {
		errClose := src.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Str("entry", f.Name).Msg("node: close archive entry")
		}
	}()

	// #nosec G115 -- file mode from zip header is intentionally preserved.
	perm := f.Mode().Perm()
	if perm == 0 {
		perm = 0o600
	}
	// #nosec G304 -- full is filepath.Join(root, local-rel); filepath.IsLocal
	// rejected escapes above.
	dst, errCreate := os.OpenFile(full, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if errCreate != nil {
		return fmt.Errorf("node: create archive entry %s: %w", f.Name, errCreate)
	}
	errCopy := copyArchiveEntry(dst, src, totalBytes)
	errClose := dst.Close()
	if errCopy != nil {
		errRemove := os.Remove(full)
		if errors.Is(errRemove, fs.ErrNotExist) {
			errRemove = nil
		} else if errRemove != nil {
			errRemove = fmt.Errorf("node: remove partial staged backup file: %w", errRemove)
		}
		return errors.Join(fmt.Errorf("node: write archive entry %s: %w", f.Name, errCopy), errRemove)
	}
	if errClose != nil {
		return fmt.Errorf("node: close extracted file %s: %w", f.Name, errClose)
	}
	return nil
}

func backupExtractRelativePath(entryName string) (string, error) {
	relative := filepath.ToSlash(filepath.Clean(entryName))
	if !filepath.IsLocal(relative) {
		return "", fmt.Errorf("node: archive entry escapes root: %s", entryName)
	}
	return relative, nil
}

func applyStagedBackupExtract(ctx context.Context, stageRoot string, liveRoot *os.Root, archivedDirectoryModes map[string]fs.FileMode) error {
	finalDirectoryModes := make(map[string]fs.FileMode, len(archivedDirectoryModes))
	errWalk := filepath.WalkDir(stageRoot, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		errCtx := ctx.Err()
		if errCtx != nil {
			return fmt.Errorf("node: extract canceled: %w", errCtx)
		}
		if walkErr != nil {
			return fmt.Errorf("node: walk staged backup extract: %w", walkErr)
		}
		if currentPath == stageRoot {
			return nil
		}

		relative, errRel := filepath.Rel(stageRoot, currentPath)
		if errRel != nil {
			return fmt.Errorf("node: resolve staged backup path: %w", errRel)
		}
		if !filepath.IsLocal(relative) {
			return fmt.Errorf("node: staged backup path escapes root: %s", relative)
		}

		info, errInfo := entry.Info()
		if errInfo != nil {
			return fmt.Errorf("node: stat staged backup entry: %w", errInfo)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("node: symlinks not allowed in staged backup extract: %s", relative)
		}

		if info.IsDir() {
			relativeSlash := filepath.ToSlash(relative)
			perm := info.Mode().Perm()
			archivedPerm, hasArchivedPerm := archivedDirectoryModes[relativeSlash]
			if hasArchivedPerm {
				perm = archivedPerm
			}
			perm = backupDirectoryPerm(perm)
			finalDirectoryModes[relativeSlash] = perm

			errPrepare := prepareBackupRestoreDirectory(liveRoot, relative, writableBackupDirectoryPerm(perm))
			if errPrepare != nil {
				return errPrepare
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		errCopy := copyStagedBackupFile(liveRoot, currentPath, relative, info.Mode().Perm())
		if errCopy != nil {
			return errCopy
		}
		return nil
	})
	if errWalk != nil {
		return fmt.Errorf("node: apply staged backup extract: %w", errWalk)
	}
	errRestoreModes := restoreBackupDirectoryModes(liveRoot, finalDirectoryModes)
	if errRestoreModes != nil {
		return errRestoreModes
	}
	return nil
}

func backupDirectoryPerm(perm fs.FileMode) fs.FileMode {
	if perm == 0 {
		return 0o750
	}
	return perm
}

func writableBackupDirectoryPerm(perm fs.FileMode) fs.FileMode {
	return backupDirectoryPerm(perm) | 0o700
}

func restoreBackupDirectoryModes(liveRoot *os.Root, directoryModes map[string]fs.FileMode) error {
	directories := make([]string, 0, len(directoryModes))
	for relative := range directoryModes {
		directories = append(directories, relative)
	}
	sort.Slice(directories, func(i, j int) bool {
		return backupDirectoryDepth(directories[i]) > backupDirectoryDepth(directories[j])
	})

	for _, relative := range directories {
		errChmod := liveRoot.Chmod(filepath.FromSlash(relative), backupDirectoryPerm(directoryModes[relative]))
		if errChmod != nil {
			return fmt.Errorf("node: set live backup directory mode: %w", errChmod)
		}
	}
	return nil
}

func backupDirectoryDepth(relative string) int {
	if relative == "" || relative == "." {
		return 0
	}
	return strings.Count(relative, "/")
}

func copyStagedBackupFile(liveRoot *os.Root, sourcePath, destinationRelative string, perm fs.FileMode) error {
	srcFile, errOpen := os.Open(sourcePath)
	if errOpen != nil {
		return fmt.Errorf("node: open staged backup file: %w", errOpen)
	}
	defer func() {
		errClose := srcFile.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Str("path", sourcePath).Msg("node: close staged backup file")
		}
	}()

	errPrepare := prepareBackupRestoreFile(liveRoot, destinationRelative)
	if errPrepare != nil {
		return errPrepare
	}

	if perm == 0 {
		perm = 0o600
	}

	tempRelative := filepath.Join(filepath.Dir(destinationRelative), ".xylona-restore-"+rand.Text())
	tempFile, errCreate := liveRoot.OpenFile(tempRelative, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errCreate != nil {
		return fmt.Errorf("node: create live backup temp file: %w", errCreate)
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			errRemove := liveRoot.Remove(tempRelative)
			if errRemove != nil && !errors.Is(errRemove, fs.ErrNotExist) {
				log.Warn().Err(errRemove).Str("path", tempRelative).Msg("node: remove backup restore temp file")
			}
		}
	}()

	_, errCopy := io.Copy(tempFile, srcFile)
	errSync := tempFile.Sync()
	errClose := tempFile.Close()
	if errCopy != nil || errSync != nil || errClose != nil {
		return fmt.Errorf("node: write live backup temp file: %w", errors.Join(errCopy, errSync, errClose))
	}
	errChmod := liveRoot.Chmod(tempRelative, perm)
	if errChmod != nil {
		return fmt.Errorf("node: set live backup temp file mode: %w", errChmod)
	}
	errRename := liveRoot.Rename(tempRelative, destinationRelative)
	if errRename != nil {
		return fmt.Errorf("node: move live backup temp file into place: %w", errRename)
	}
	removeTemp = false

	errChmod = liveRoot.Chmod(destinationRelative, perm)
	if errChmod != nil {
		return fmt.Errorf("node: set live backup file mode: %w", errChmod)
	}
	return nil
}

func prepareBackupRestoreDirectory(liveRoot *os.Root, targetRelative string, perm fs.FileMode) error {
	errParents := ensureBackupRestoreDirectoryChain(liveRoot, filepath.Dir(targetRelative))
	if errParents != nil {
		return errParents
	}

	info, errLstat := liveRoot.Lstat(targetRelative)
	if errLstat == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrRestoreDestinationSymlink
		}
		if !info.IsDir() {
			errRemove := liveRoot.RemoveAll(targetRelative)
			if errRemove != nil {
				return fmt.Errorf("node: replace restore file with directory: %w", errRemove)
			}
		}
	} else if !errors.Is(errLstat, fs.ErrNotExist) {
		return fmt.Errorf("node: inspect restore directory target: %w", errLstat)
	}

	if perm == 0 {
		perm = 0o750
	}
	errMkdir := liveRoot.MkdirAll(targetRelative, perm)
	if errMkdir != nil {
		return fmt.Errorf("node: create live backup directory: %w", errMkdir)
	}
	errChmod := liveRoot.Chmod(targetRelative, perm)
	if errChmod != nil {
		return fmt.Errorf("node: set live backup directory mode: %w", errChmod)
	}
	return nil
}

func prepareBackupRestoreFile(liveRoot *os.Root, targetRelative string) error {
	errParents := ensureBackupRestoreDirectoryChain(liveRoot, filepath.Dir(targetRelative))
	if errParents != nil {
		return errParents
	}

	info, errLstat := liveRoot.Lstat(targetRelative)
	if errLstat != nil {
		if errors.Is(errLstat, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("node: inspect restore file target: %w", errLstat)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrRestoreDestinationSymlink
	}

	errRemove := liveRoot.RemoveAll(targetRelative)
	if errRemove != nil {
		return fmt.Errorf("node: replace restore destination: %w", errRemove)
	}
	return nil
}

func ensureBackupRestoreDirectoryChain(liveRoot *os.Root, targetDirectory string) error {
	relative := filepath.Clean(targetDirectory)
	if relative == "." {
		return nil
	}
	if !filepath.IsLocal(relative) {
		return fmt.Errorf("node: restore parent escapes root: %s", relative)
	}

	currentPath := ""
	for part := range strings.SplitSeq(relative, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		currentPath = filepath.Join(currentPath, part)
		info, errLstat := liveRoot.Lstat(currentPath)
		if errLstat == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return ErrRestoreDestinationSymlink
			}
			if info.IsDir() {
				continue
			}
			errRemove := liveRoot.RemoveAll(currentPath)
			if errRemove != nil {
				return fmt.Errorf("node: replace restore parent with directory: %w", errRemove)
			}
		} else if !errors.Is(errLstat, fs.ErrNotExist) {
			return fmt.Errorf("node: inspect restore parent directory: %w", errLstat)
		}

		errMkdir := liveRoot.Mkdir(currentPath, 0o750)
		if errMkdir != nil && !errors.Is(errMkdir, fs.ErrExist) {
			return fmt.Errorf("node: create restore parent directory: %w", errMkdir)
		}
	}
	return nil
}

func cleanupBackupRestoreStaging(stageRoot string) {
	errRemove := os.RemoveAll(stageRoot)
	if errRemove != nil {
		log.Warn().Err(errRemove).Str("path", stageRoot).Msg("node: remove backup restore staging dir")
	}
}

// pruneDirectoryForExactExtract removes any entry inside root that is not in
// archivePaths. Used by ExtractModeExact so restored directories match the
// archive exactly.
func pruneDirectoryForExactExtract(liveRoot *os.Root, archivePaths map[string]struct{}) error {
	errWalk := fs.WalkDir(liveRoot.FS(), ".", func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("node: walk exact-extract prune: %w", walkErr)
		}
		if currentPath == "." {
			return nil
		}
		relSlash := filepath.ToSlash(currentPath)
		if _, inArchive := archivePaths[relSlash]; inArchive {
			return nil
		}
		if entry.IsDir() {
			// Only remove dirs if they contain no archive entries.
			if dirContainsNoArchiveEntries(relSlash, archivePaths) {
				errRemove := liveRoot.RemoveAll(filepath.FromSlash(currentPath))
				if errRemove != nil {
					return fmt.Errorf("node: remove extra directory %s: %w", relSlash, errRemove)
				}
				return filepath.SkipDir
			}
			return nil
		}
		errRemove := liveRoot.Remove(filepath.FromSlash(currentPath))
		if errRemove != nil && !errors.Is(errRemove, fs.ErrNotExist) {
			return fmt.Errorf("node: remove extra file %s: %w", relSlash, errRemove)
		}
		return nil
	})
	if errWalk != nil {
		return fmt.Errorf("node: walk for exact-mode prune: %w", errWalk)
	}
	return nil
}

func dirContainsNoArchiveEntries(dirRelSlash string, archivePaths map[string]struct{}) bool {
	prefix := dirRelSlash + "/"
	for p := range archivePaths {
		if p == dirRelSlash || strings.HasPrefix(p, prefix) {
			return false
		}
	}
	return true
}
