package node

import (
	"bytes"
	"context"
	"crypto/sha1" // #nosec G505 -- used only for Mojang-published checksum verification.
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/startargs"
	"github.com/ClintonCollins/Xylona/internal/webhooks"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
)

var (
	downloadHTTPClient     = helpers.GetXylonaHTTPClient
	validateDownloadTarget = webhooks.ValidateWebhookTarget
)

// enforceProtection runs the shared IsProtectedServerPath check on a write-
// path target. Returns ErrProtectedPath when the path matches a protected
// game-server executable or launch command. Callers pass a zero-value
// ProtectionPolicy for requests unrelated to a game server (the check is
// then a no-op).
func enforceProtection(relativePath string, policy ProtectionPolicy) error {
	if !policy.IsConfigured() {
		return nil
	}
	if startargs.IsProtectedServerPath(relativePath, policy.BaseCommand, policy.ServerExecutable) {
		log.Warn().
			Str("path", relativePath).
			Str("server_executable", policy.ServerExecutable).
			Str("base_command", policy.BaseCommand).
			Msg("node: rejected write to protected path")
		return ErrProtectedPath
	}
	return nil
}

// validateLocalPath returns the trimmed relative path if it is a local
// (non-escaping) path within a server root, or ErrInvalidPath otherwise. An
// empty path is treated as the root and returned as-is.
//
// This mirrors the semantics of actions.validateLocalServerPath but does not
// reach into the game server model: the caller is expected to resolve the
// game server directory before invoking internal/node.
func validateLocalPath(relativePath string) (string, error) {
	trimmedPath := strings.TrimPrefix(relativePath, "/")
	if trimmedPath != "" && !filepath.IsLocal(trimmedPath) {
		log.Error().Str("path", relativePath).Msg("node: invalid path")
		return "", ErrInvalidPath
	}
	return trimmedPath, nil
}

// resolveWithinRoot joins root and relative, then verifies the cleaned result
// stays inside root. Used as a defense in depth on top of validateLocalPath
// for write operations and downloads.
func resolveWithinRoot(root, relative string) (string, error) {
	cleanRoot := filepath.Clean(root)
	full := filepath.Clean(filepath.Join(cleanRoot, relative))
	prefix := cleanRoot + string(filepath.Separator)
	if full != cleanRoot && !strings.HasPrefix(full, prefix) {
		log.Error().Str("path", full).Msg("node: path escaped game server root")
		return "", ErrInvalidPath
	}
	return full, nil
}

// ListFiles enumerates the entries directly under directory/relativePath. For
// directory entries, the returned size is the recursive total of contained
// file sizes (mirrors the controller's existing ListGameServerFiles behavior).
func (n *Node) ListFiles(directory, relativePath string) ([]FileEntry, error) {
	if relativePath != "" && !filepath.IsLocal(relativePath) {
		log.Error().Str("path", relativePath).Msg("node: list files invalid path")
		return nil, ErrInvalidPath
	}

	fullPath := filepath.Join(directory, relativePath)
	entries, errReadDir := os.ReadDir(fullPath)
	if errReadDir != nil {
		return nil, fmt.Errorf("node: read directory: %w", errReadDir)
	}

	results := make([]FileEntry, 0, len(entries))
	for _, entry := range entries {
		info, errInfo := entry.Info()
		if errInfo != nil {
			return nil, fmt.Errorf("node: stat directory entry: %w", errInfo)
		}

		size := info.Size()
		if info.IsDir() {
			recursiveSize, errWalk := walkDirectorySize(filepath.Join(fullPath, entry.Name()))
			if errWalk != nil {
				return nil, errWalk
			}
			size = recursiveSize
		}

		results = append(results, NewFileEntry(info.Name(), size, info.IsDir(), info.ModTime(), isExecutableFile(info)))
	}
	return results, nil
}

func isExecutableFile(info os.FileInfo) bool {
	if info == nil || info.IsDir() {
		return false
	}
	if info.Mode().Perm()&0o111 != 0 {
		return true
	}
	if runtime.GOOS != "windows" {
		return false
	}
	switch strings.ToLower(filepath.Ext(info.Name())) {
	case ".bat", ".cmd", ".com", ".exe", ".ps1", ".sh":
		return true
	default:
		return false
	}
}

func walkDirectorySize(root string) (int64, error) {
	var total int64
	errWalk := filepath.WalkDir(root, func(_ string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, errInfo := d.Info()
		if errInfo != nil {
			return fmt.Errorf("node: stat nested directory entry: %w", errInfo)
		}
		total += info.Size()
		return nil
	})
	if errWalk != nil {
		return 0, fmt.Errorf("node: walk directory: %w", errWalk)
	}
	return total, nil
}

// ReadFile returns the bytes of directory/relativePath, after verifying the
// resolved path stays inside directory.
func (n *Node) ReadFile(directory, relativePath string) ([]byte, error) {
	validated, errPath := validateLocalPath(relativePath)
	if errPath != nil {
		return nil, errPath
	}

	fullPath, errResolve := resolveWithinRoot(directory, validated)
	if errResolve != nil {
		return nil, errResolve
	}

	data, errRead := os.ReadFile(fullPath)
	if errRead != nil {
		return nil, fmt.Errorf("node: read file: %w", errRead)
	}
	return data, nil
}

// StatFile returns metadata for directory/relativePath after verifying the
// resolved path stays inside directory.
func (n *Node) StatFile(directory, relativePath string) (FileEntry, error) {
	validated, errPath := validateLocalPath(relativePath)
	if errPath != nil {
		return FileEntry{}, errPath
	}

	fullPath, errResolve := resolveWithinRoot(directory, validated)
	if errResolve != nil {
		return FileEntry{}, errResolve
	}

	info, errStat := os.Stat(fullPath)
	if errStat != nil {
		return FileEntry{}, fmt.Errorf("node: stat file: %w", errStat)
	}
	return NewFileEntry(info.Name(), info.Size(), info.IsDir(), info.ModTime(), isExecutableFile(info)), nil
}

// OpenFile opens directory/relativePath for streaming without loading the
// entire file into controller memory. The caller owns the returned closer.
func (n *Node) OpenFile(directory, relativePath string) (io.ReadCloser, error) {
	validated, errPath := validateLocalPath(relativePath)
	if errPath != nil {
		return nil, errPath
	}

	fullPath, errResolve := resolveWithinRoot(directory, validated)
	if errResolve != nil {
		return nil, errResolve
	}

	file, errOpen := os.Open(fullPath)
	if errOpen != nil {
		return nil, fmt.Errorf("node: open file: %w", errOpen)
	}

	info, errStat := file.Stat()
	if errStat != nil {
		errClose := file.Close()
		if errClose != nil {
			return nil, errors.Join(fmt.Errorf("node: stat opened file: %w", errStat), fmt.Errorf("node: close opened file after stat error: %w", errClose))
		}
		return nil, fmt.Errorf("node: stat opened file: %w", errStat)
	}
	if info.IsDir() {
		errClose := file.Close()
		if errClose != nil {
			return nil, errors.Join(ErrInvalidPath, fmt.Errorf("node: close opened directory: %w", errClose))
		}
		return nil, ErrInvalidPath
	}
	return file, nil
}

// WriteFile writes content to directory/relativePath. The parent directory is
// not created. When policy is configured, writes to the game server's
// protected paths (server executable, launch script) are rejected with
// ErrProtectedPath.
func (n *Node) WriteFile(directory, relativePath string, content []byte, policy ProtectionPolicy) error {
	_, errWrite := n.WriteFileFromReader(directory, relativePath, bytes.NewReader(content), policy)
	if errWrite != nil {
		return errWrite
	}
	return nil
}

// WriteFileFromReader writes reader content to directory/relativePath via a
// same-directory temp file, then replaces the target. It returns the byte count
// and SHA-256 digest of the written content.
func (n *Node) WriteFileFromReader(directory, relativePath string, reader io.Reader, policy ProtectionPolicy) (WriteFileResult, error) {
	validated, errPath := validateLocalPath(relativePath)
	if errPath != nil {
		return WriteFileResult{}, errPath
	}

	errProtected := enforceProtection(validated, policy)
	if errProtected != nil {
		return WriteFileResult{}, errProtected
	}

	fullPath, errResolve := resolveWithinRoot(directory, validated)
	if errResolve != nil {
		return WriteFileResult{}, errResolve
	}

	return writeFileFromReaderAtPath(fullPath, reader, 0o600)
}

func writeFileFromReaderAtPath(fullPath string, reader io.Reader, perm os.FileMode) (result WriteFileResult, err error) {
	parent := filepath.Dir(fullPath)
	tempFile, errCreate := os.CreateTemp(parent, ".xylona-write-*")
	if errCreate != nil {
		return WriteFileResult{}, fmt.Errorf("node: create temp file: %w", errCreate)
	}

	tempPath := tempFile.Name()
	removeTemp := true
	defer func() {
		if !removeTemp {
			return
		}
		errRemove := os.Remove(tempPath)
		if err == nil && errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			err = fmt.Errorf("node: remove temp file: %w", errRemove)
		}
	}()

	errChmod := tempFile.Chmod(perm)
	if errChmod != nil {
		errClose := tempFile.Close()
		if errClose != nil {
			return WriteFileResult{}, errors.Join(fmt.Errorf("node: chmod temp file: %w", errChmod), fmt.Errorf("node: close temp file after chmod error: %w", errClose))
		}
		return WriteFileResult{}, fmt.Errorf("node: chmod temp file: %w", errChmod)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(tempFile, hasher)
	bytesWritten, errCopy := io.Copy(writer, reader)
	errClose := tempFile.Close()
	if errCopy != nil {
		if errClose != nil {
			return WriteFileResult{}, errors.Join(fmt.Errorf("node: write temp file: %w", errCopy), fmt.Errorf("node: close temp file after write error: %w", errClose))
		}
		return WriteFileResult{}, fmt.Errorf("node: write temp file: %w", errCopy)
	}
	if errClose != nil {
		return WriteFileResult{}, fmt.Errorf("node: close temp file: %w", errClose)
	}

	errRename := replaceFile(tempPath, fullPath)
	if errRename != nil {
		return WriteFileResult{}, fmt.Errorf("node: replace file: %w", errRename)
	}
	removeTemp = false

	return WriteFileResult{
		BytesWritten: bytesWritten,
		SHA256:       hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

func replaceFile(tempPath, targetPath string) error {
	errRename := os.Rename(tempPath, targetPath)
	if errRename == nil {
		return nil
	}
	if runtime.GOOS != "windows" {
		return fmt.Errorf("rename temp file: %w", errRename)
	}

	_, errStat := os.Stat(targetPath)
	if errStat != nil {
		return fmt.Errorf("rename temp file: %w", errRename)
	}
	errRemove := os.Remove(targetPath)
	if errRemove != nil {
		return errors.Join(errRename, fmt.Errorf("node: remove existing file before replace: %w", errRemove))
	}
	errRenameAgain := os.Rename(tempPath, targetPath)
	if errRenameAgain != nil {
		return errors.Join(errRename, fmt.Errorf("node: rename after removing existing file: %w", errRenameAgain))
	}
	return nil
}

// CreateFileOrDirectory creates a file or directory inside directory. When
// isDirectory is true, the path is created with MkdirAll. When false, the file
// is created and content (if non-empty) is written into it.
func (n *Node) CreateFileOrDirectory(directory, relativePath, content string, isDirectory bool, policy ProtectionPolicy) error {
	validated, errPath := validateLocalPath(relativePath)
	if errPath != nil {
		return errPath
	}

	errProtected := enforceProtection(validated, policy)
	if errProtected != nil {
		return errProtected
	}

	fullPath, errResolve := resolveWithinRoot(directory, validated)
	if errResolve != nil {
		return errResolve
	}

	if isDirectory {
		errMkdir := os.MkdirAll(fullPath, 0o750)
		if errMkdir != nil {
			log.Error().Err(errMkdir).Msg("node: create directory failed")
			return fmt.Errorf("node: create directory: %w", errMkdir)
		}
		return nil
	}

	file, errCreate := os.Create(fullPath)
	if errCreate != nil {
		log.Error().Err(errCreate).Msg("node: create file failed")
		return fmt.Errorf("node: create file: %w", errCreate)
	}

	if content != "" {
		_, errWrite := file.WriteString(content)
		if errWrite != nil {
			log.Error().Err(errWrite).Msg("node: write file failed")
			errClose := file.Close()
			if errClose != nil {
				log.Error().Err(errClose).Msg("node: close file after write error")
			}
			return fmt.Errorf("node: write file: %w", errWrite)
		}
	}

	errClose := file.Close()
	if errClose != nil {
		log.Error().Err(errClose).Msg("node: close file failed")
		return fmt.Errorf("node: close file: %w", errClose)
	}
	return nil
}

// DeleteFiles removes each provided relative path from directory and returns
// the validated paths that were successfully removed. When policy is
// configured, any protected path in the set aborts the operation with
// ErrProtectedPath.
func (n *Node) DeleteFiles(ctx context.Context, directory string, files []string, policy ProtectionPolicy) ([]string, error) {
	deleted := make([]string, 0, len(files))
	for _, file := range files {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("node: delete files canceled: %w", ctx.Err())
		default:
		}

		validated, errPath := validateLocalPath(file)
		if errPath != nil {
			return nil, errPath
		}
		errProtected := enforceProtection(validated, policy)
		if errProtected != nil {
			return nil, errProtected
		}
		fullPath, errResolve := resolveWithinRoot(directory, validated)
		if errResolve != nil {
			return nil, errResolve
		}
		errRemove := os.RemoveAll(fullPath)
		if errRemove != nil {
			log.Error().Err(errRemove).Str("path", validated).Msg("node: remove failed")
			continue
		}
		deleted = append(deleted, validated)
	}
	return deleted, nil
}

// RenameFile renames oldRelativePath to newRelativePath inside directory. The
// validated new path is returned on success. Both paths are subject to the
// protected-path check.
func (n *Node) RenameFile(directory, oldRelativePath, newRelativePath string, policy ProtectionPolicy) (string, error) {
	validatedOld, errOld := validateLocalPath(oldRelativePath)
	if errOld != nil {
		return "", errOld
	}
	validatedNew, errNew := validateLocalPath(newRelativePath)
	if errNew != nil {
		return "", errNew
	}

	errProtectedOld := enforceProtection(validatedOld, policy)
	if errProtectedOld != nil {
		return "", errProtectedOld
	}
	errProtectedNew := enforceProtection(validatedNew, policy)
	if errProtectedNew != nil {
		return "", errProtectedNew
	}

	oldFullPath, errResolveOld := resolveWithinRoot(directory, validatedOld)
	if errResolveOld != nil {
		return "", errResolveOld
	}
	newFullPath, errResolveNew := resolveWithinRoot(directory, validatedNew)
	if errResolveNew != nil {
		return "", errResolveNew
	}

	errRename := os.Rename(oldFullPath, newFullPath)
	if errRename != nil {
		log.Error().Err(errRename).Msg("node: rename failed")
		return "", fmt.Errorf("node: rename file: %w", errRename)
	}
	return validatedNew, nil
}

// MoveFiles moves files into destination, which is a relative path inside
// directory. The destination directory is created if needed. The returned
// slice contains the validated source paths that were successfully moved.
// Each source path and the destination itself are subject to the protected-
// path check.
func (n *Node) MoveFiles(ctx context.Context, directory string, files []string, destination string, policy ProtectionPolicy) ([]string, error) {
	validatedDestination, errDestination := validateLocalPath(destination)
	if errDestination != nil {
		return nil, errDestination
	}
	if validatedDestination == ".." {
		return nil, ErrInvalidPath
	}

	destinationFullPath, errResolveDest := resolveWithinRoot(directory, validatedDestination)
	if errResolveDest != nil {
		return nil, errResolveDest
	}
	errMkdir := os.MkdirAll(destinationFullPath, 0o750)
	if errMkdir != nil {
		log.Error().Err(errMkdir).Msg("node: create destination directory failed")
		return nil, fmt.Errorf("node: create destination directory: %w", errMkdir)
	}

	moved := make([]string, 0, len(files))
	for _, file := range files {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("node: move files canceled: %w", ctx.Err())
		default:
		}

		validatedFile, errFilePath := validateLocalPath(file)
		if errFilePath != nil {
			return nil, errFilePath
		}
		errProtectedSrc := enforceProtection(validatedFile, policy)
		if errProtectedSrc != nil {
			return nil, errProtectedSrc
		}
		fullPath, errResolveSrc := resolveWithinRoot(directory, validatedFile)
		if errResolveSrc != nil {
			return nil, errResolveSrc
		}

		destinationRel := filepath.Join(validatedDestination, filepath.Base(validatedFile))
		errProtectedDest := enforceProtection(destinationRel, policy)
		if errProtectedDest != nil {
			return nil, errProtectedDest
		}
		destinationFilePath := filepath.Join(destinationFullPath, filepath.Base(validatedFile))
		_, errResolveTarget := resolveWithinRoot(directory, destinationRel)
		if errResolveTarget != nil {
			return nil, errResolveTarget
		}

		errRename := os.Rename(fullPath, destinationFilePath)
		if errRename != nil {
			log.Error().Err(errRename).Msg("node: move failed")
			continue
		}
		moved = append(moved, validatedFile)
	}
	return moved, nil
}

// CopyFiles copies each source path to its paired destination path inside
// directory. Destination paths are subject to the protected-path check.
func (n *Node) CopyFiles(ctx context.Context, directory string, operations []CopyFileOperation, policy ProtectionPolicy) ([]string, error) {
	copied := make([]string, 0, len(operations))
	for _, operation := range operations {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("node: copy files canceled: %w", ctx.Err())
		default:
		}

		validatedSource, errSourcePath := validateLocalPath(operation.SourceRelativePath)
		if errSourcePath != nil {
			return nil, errSourcePath
		}
		validatedDestination, errDestinationPath := validateLocalPath(operation.DestinationRelativePath)
		if errDestinationPath != nil {
			return nil, errDestinationPath
		}
		errProtected := enforceProtection(validatedDestination, policy)
		if errProtected != nil {
			return nil, errProtected
		}

		sourceFullPath, errSourceResolve := resolveWithinRoot(directory, validatedSource)
		if errSourceResolve != nil {
			return nil, errSourceResolve
		}
		destinationFullPath, errDestinationResolve := resolveWithinRoot(directory, validatedDestination)
		if errDestinationResolve != nil {
			return nil, errDestinationResolve
		}

		errCopy := copyPath(ctx, sourceFullPath, destinationFullPath, validatedDestination, policy)
		if errCopy != nil {
			return nil, errCopy
		}
		copied = append(copied, filepath.ToSlash(validatedDestination))
	}
	return copied, nil
}

func copyPath(ctx context.Context, sourceFullPath, destinationFullPath, destinationRelativePath string, policy ProtectionPolicy) error {
	sourceInfo, errStat := os.Lstat(sourceFullPath)
	if errStat != nil {
		return fmt.Errorf("node: stat copy source: %w", errStat)
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 {
		return ErrInvalidPath
	}
	if !sourceInfo.IsDir() {
		return copyRegularFile(sourceFullPath, destinationFullPath, sourceInfo.Mode().Perm())
	}

	cleanSource := filepath.Clean(sourceFullPath)
	cleanDestination := filepath.Clean(destinationFullPath)
	sourcePrefix := cleanSource + string(filepath.Separator)
	if cleanDestination == cleanSource || strings.HasPrefix(cleanDestination, sourcePrefix) {
		return ErrInvalidPath
	}

	errWalk := filepath.WalkDir(sourceFullPath, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		errCtx := ctx.Err()
		if errCtx != nil {
			return fmt.Errorf("node: copy files canceled: %w", errCtx)
		}

		relativeToSource, errRel := filepath.Rel(sourceFullPath, current)
		if errRel != nil {
			return fmt.Errorf("node: calculate copy relative path: %w", errRel)
		}
		targetPath := filepath.Join(destinationFullPath, relativeToSource)
		targetRelativePath := filepath.ToSlash(filepath.Join(destinationRelativePath, relativeToSource))
		if relativeToSource == "." {
			targetPath = destinationFullPath
			targetRelativePath = filepath.ToSlash(destinationRelativePath)
		}

		errProtected := enforceProtection(targetRelativePath, policy)
		if errProtected != nil {
			return errProtected
		}

		info, errInfo := entry.Info()
		if errInfo != nil {
			return fmt.Errorf("node: stat copy entry: %w", errInfo)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrInvalidPath
		}
		if info.IsDir() {
			errMkdir := os.MkdirAll(targetPath, info.Mode().Perm())
			if errMkdir != nil {
				return fmt.Errorf("node: create copy directory: %w", errMkdir)
			}
			return nil
		}
		return copyRegularFile(current, targetPath, info.Mode().Perm())
	})
	if errWalk != nil {
		return fmt.Errorf("node: copy directory: %w", errWalk)
	}
	return nil
}

func copyRegularFile(sourcePath, destinationPath string, perm os.FileMode) error {
	errMkdir := os.MkdirAll(filepath.Dir(destinationPath), 0o750)
	if errMkdir != nil {
		return fmt.Errorf("node: create copy parent directory: %w", errMkdir)
	}

	sourceFile, errOpen := os.Open(sourcePath)
	if errOpen != nil {
		return fmt.Errorf("node: open copy source: %w", errOpen)
	}
	defer func() {
		errClose := sourceFile.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("node: close copy source")
		}
	}()

	_, errWrite := writeFileFromReaderAtPath(destinationPath, sourceFile, perm)
	if errWrite != nil {
		return errWrite
	}
	return nil
}

// DownloadFileFromURL fetches rawURL over HTTP/HTTPS and stores the result
// inside directory under destinationDirectoryPath, using the URL's basename as
// the file name. The function rejects non-http(s) schemes, applies the
// webhook SSRF guard, and validates redirects against the same guard. The
// final destination path (destinationDirectoryPath + basename) is subject to
// the protected-path check.
func (n *Node) DownloadFileFromURL(ctx context.Context, directory, rawURL, destinationDirectoryPath string, integrity DownloadIntegrity, policy ProtectionPolicy) (DownloadFileResult, error) {
	validatedDestination, errPath := validateLocalPath(destinationDirectoryPath)
	if errPath != nil {
		return DownloadFileResult{}, errPath
	}

	parsedURL, errParseURL := url.Parse(rawURL)
	if errParseURL != nil {
		log.Error().Err(errParseURL).Msg("node: failed to parse download URL")
		return DownloadFileResult{}, errors.New("node: invalid URL")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		log.Error().Str("scheme", parsedURL.Scheme).Msg("node: download URL scheme not allowed")
		return DownloadFileResult{}, errors.New("node: only http and https URLs are allowed")
	}

	errSSRF := validateDownloadTarget(rawURL)
	if errSSRF != nil {
		return DownloadFileResult{}, fmt.Errorf("node: validate download URL: %w", errSSRF)
	}

	httpClientBase := downloadHTTPClient()
	httpClientCopy := *httpClientBase
	httpClient := &httpClientCopy
	httpClient.CheckRedirect = validateDownloadRedirectTarget

	req, errNewReq := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if errNewReq != nil {
		return DownloadFileResult{}, fmt.Errorf("node: create download request: %w", errNewReq)
	}

	fileName := strings.TrimPrefix(path.Base(req.URL.Path), "/")
	if !filepath.IsLocal(fileName) {
		return DownloadFileResult{}, ErrInvalidPath
	}

	resp, errGet := httpClient.Do(req)
	if errGet != nil {
		return DownloadFileResult{}, fmt.Errorf("node: download file from URL: %w", errGet)
	}
	defer func() {
		errClose := resp.Body.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("node: close download response body")
		}
	}()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return DownloadFileResult{}, fmt.Errorf("%w: %s", ErrUnexpectedHTTPStatus, resp.Status)
	}

	destinationRelative := filepath.Join(validatedDestination, fileName)
	errProtected := enforceProtection(destinationRelative, policy)
	if errProtected != nil {
		return DownloadFileResult{}, errProtected
	}
	destinationFullPath, errResolve := resolveWithinRoot(directory, destinationRelative)
	if errResolve != nil {
		return DownloadFileResult{}, errResolve
	}

	tempFile, errCreate := os.CreateTemp(filepath.Dir(destinationFullPath), "."+filepath.Base(destinationFullPath)+".download-*")
	if errCreate != nil {
		return DownloadFileResult{}, fmt.Errorf("node: create downloaded file: %w", errCreate)
	}
	tempPath := tempFile.Name()
	keepTemp := false
	defer func() {
		if keepTemp {
			return
		}
		errRemove := os.Remove(tempPath)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			log.Warn().Err(errRemove).Str("path", tempPath).Msg("node: remove incomplete download")
		}
	}()

	sha256Hasher := sha256.New()
	sha1Hasher := sha1.New() // #nosec G401 -- used only for Mojang-published checksum verification.
	writer := io.MultiWriter(tempFile, sha256Hasher, sha1Hasher)
	reader := io.Reader(resp.Body)
	if integrity.ExpectedSize > 0 {
		reader = io.LimitReader(resp.Body, integrity.ExpectedSize+1)
	}
	bytesWritten, errCopy := io.Copy(writer, reader)
	if errCopy != nil {
		errClose := tempFile.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("node: close downloaded file after copy error")
		}
		return DownloadFileResult{}, fmt.Errorf("node: write downloaded file: %w", errCopy)
	}

	sha256Hex := hex.EncodeToString(sha256Hasher.Sum(nil))
	sha1Hex := hex.EncodeToString(sha1Hasher.Sum(nil))
	errIntegrity := validateDownloadIntegrity(integrity, bytesWritten, sha256Hex, sha1Hex)
	if errIntegrity != nil {
		errClose := tempFile.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("node: close downloaded file after integrity failure")
		}
		return DownloadFileResult{}, errIntegrity
	}

	errClose := tempFile.Close()
	if errClose != nil {
		return DownloadFileResult{}, fmt.Errorf("node: close downloaded file: %w", errClose)
	}

	errRename := os.Rename(tempPath, destinationFullPath)
	if errRename != nil {
		errRemoveDestination := os.Remove(destinationFullPath)
		if errRemoveDestination != nil && !errors.Is(errRemoveDestination, os.ErrNotExist) {
			return DownloadFileResult{}, fmt.Errorf("node: replace downloaded file: %w", errRemoveDestination)
		}
		errRename = os.Rename(tempPath, destinationFullPath)
		if errRename != nil {
			return DownloadFileResult{}, fmt.Errorf("node: promote downloaded file: %w", errRename)
		}
	}
	keepTemp = true

	return DownloadFileResult{
		RelativePath:  destinationRelative,
		BytesWritten:  bytesWritten,
		SHA256:        sha256Hex,
		SHA1:          sha1Hex,
		ExpectedMatch: integrity.HasExpectedMetadata(),
	}, nil
}

func validateDownloadRedirectTarget(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}

	errValidateRedirect := validateDownloadTarget(req.URL.String())
	if errValidateRedirect != nil {
		return fmt.Errorf("node: download redirect blocked: %w", errValidateRedirect)
	}
	return nil
}

func validateDownloadIntegrity(integrity DownloadIntegrity, bytesWritten int64, sha256Hex string, sha1Hex string) error {
	if integrity.ExpectedSize > 0 && bytesWritten != integrity.ExpectedSize {
		return fmt.Errorf("%w: size got %d, want %d", ErrDownloadIntegrityMismatch, bytesWritten, integrity.ExpectedSize)
	}
	expectedSHA256 := strings.TrimSpace(integrity.ExpectedSHA256)
	if expectedSHA256 != "" && !strings.EqualFold(sha256Hex, expectedSHA256) {
		return fmt.Errorf("%w: sha256 got %s, want %s", ErrDownloadIntegrityMismatch, sha256Hex, expectedSHA256)
	}
	expectedSHA1 := strings.TrimSpace(integrity.ExpectedSHA1)
	if expectedSHA1 != "" && !strings.EqualFold(sha1Hex, expectedSHA1) {
		return fmt.Errorf("%w: sha1 got %s, want %s", ErrDownloadIntegrityMismatch, sha1Hex, expectedSHA1)
	}
	return nil
}
