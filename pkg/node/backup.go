package node

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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
	walkRoots := includes
	if len(walkRoots) == 0 {
		walkRoots = []string{root}
	}

	for _, walkRoot := range walkRoots {
		errOuterCtx := ctx.Err()
		if errOuterCtx != nil {
			return fmt.Errorf("node: backup canceled: %w", errOuterCtx)
		}

		errWalk := filepath.WalkDir(walkRoot, func(currentPath string, entry fs.DirEntry, walkErr error) error {
			errInnerCtx := ctx.Err()
			if errInnerCtx != nil {
				return fmt.Errorf("node: backup canceled: %w", errInnerCtx)
			}
			if walkErr != nil {
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
			return addBackupFileEntry(zipWriter, cleanCurrent, archiveEntryPath, info)
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
	// #nosec G304 -- source path was produced by filepath.WalkDir under a
	// controller-supplied root that pkg/node's caller has already validated.
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

// maxBackupExtractEntryBytes caps individual archive entries to 8 GiB so a
// decompression bomb cannot fill the node's disk via ExtractBackupArchive.
const maxBackupExtractEntryBytes int64 = 8 << 30

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

	// Collect the set of archive-relative paths (for exact-mode pruning
	// and for cycle-safe extraction). Using forward slashes because
	// zip.File.Name always uses forward slashes.
	archivePaths := make(map[string]struct{}, len(reader.File))
	for _, f := range reader.File {
		archivePaths[filepath.ToSlash(filepath.Clean(f.Name))] = struct{}{}
	}

	errMkdir := os.MkdirAll(cleanRoot, 0o750)
	if errMkdir != nil {
		return fmt.Errorf("node: prepare extract directory: %w", errMkdir)
	}

	if mode == ExtractModeExact {
		errPrune := pruneDirectoryForExactExtract(cleanRoot, archivePaths)
		if errPrune != nil {
			return errPrune
		}
	}

	for _, f := range reader.File {
		errCtx := ctx.Err()
		if errCtx != nil {
			return fmt.Errorf("node: extract canceled: %w", errCtx)
		}
		errExtract := extractBackupEntry(cleanRoot, f)
		if errExtract != nil {
			return errExtract
		}
	}
	return nil
}

func extractBackupEntry(root string, f *zip.File) error {
	relative := filepath.ToSlash(filepath.Clean(f.Name))
	if !filepath.IsLocal(relative) {
		return fmt.Errorf("node: archive entry escapes root: %s", f.Name)
	}
	full := filepath.Join(root, filepath.FromSlash(relative))

	// Reject symlinks for safety.
	if f.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("node: symlinks not allowed in backup archive: %s", f.Name)
	}

	if f.FileInfo().IsDir() {
		errMkdir := os.MkdirAll(full, 0o750)
		if errMkdir != nil {
			return fmt.Errorf("node: create archive directory %s: %w", f.Name, errMkdir)
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
	// Bound per-entry writes to guard against decompression bombs (gosec G110).
	_, errCopy := io.Copy(dst, io.LimitReader(src, maxBackupExtractEntryBytes))
	errClose := dst.Close()
	if errCopy != nil {
		return fmt.Errorf("node: write archive entry %s: %w", f.Name, errCopy)
	}
	if errClose != nil {
		return fmt.Errorf("node: close extracted file %s: %w", f.Name, errClose)
	}
	return nil
}

// pruneDirectoryForExactExtract removes any entry inside root that is not in
// archivePaths. Used by ExtractModeExact so restored directories match the
// archive exactly.
func pruneDirectoryForExactExtract(root string, archivePaths map[string]struct{}) error {
	errWalk := filepath.WalkDir(root, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("node: walk exact-extract prune: %w", walkErr)
		}
		if currentPath == root {
			return nil
		}
		rel, errRel := filepath.Rel(root, currentPath)
		if errRel != nil {
			return fmt.Errorf("node: resolve prune relative path: %w", errRel)
		}
		relSlash := filepath.ToSlash(rel)
		if _, inArchive := archivePaths[relSlash]; inArchive {
			return nil
		}
		if entry.IsDir() {
			// Only remove dirs if they contain no archive entries.
			if dirContainsNoArchiveEntries(relSlash, archivePaths) {
				errRemove := os.RemoveAll(currentPath) //nolint:gosec // G122: currentPath is filepath.WalkDir result rooted at validated directory
				if errRemove != nil {
					return fmt.Errorf("node: remove extra directory %s: %w", relSlash, errRemove)
				}
				return filepath.SkipDir
			}
			return nil
		}
		errRemove := os.Remove(currentPath) //nolint:gosec // G122: currentPath is filepath.WalkDir result rooted at validated directory
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
