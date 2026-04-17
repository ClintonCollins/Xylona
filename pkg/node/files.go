package node

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/webhooks"
	"github.com/ClintonCollins/Xylona/startargs"
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
// game server directory before invoking pkg/node.
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

		results = append(results, NewFileEntry(info.Name(), size, info.IsDir(), info.ModTime()))
	}
	return results, nil
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

// WriteFile writes content to directory/relativePath. The parent directory is
// not created. When policy is configured, writes to the game server's
// protected paths (server executable, launch script) are rejected with
// ErrProtectedPath.
func (n *Node) WriteFile(directory, relativePath string, content []byte, policy ProtectionPolicy) error {
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

	errWrite := os.WriteFile(fullPath, content, 0o600)
	if errWrite != nil {
		return fmt.Errorf("node: write file: %w", errWrite)
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

// DownloadFileFromURL fetches rawURL over HTTP/HTTPS and stores the result
// inside directory under destinationDirectoryPath, using the URL's basename as
// the file name. The function rejects non-http(s) schemes, applies the
// webhook SSRF guard, and validates redirects against the same guard. The
// final destination path (destinationDirectoryPath + basename) is subject to
// the protected-path check.
func (n *Node) DownloadFileFromURL(ctx context.Context, directory, rawURL, destinationDirectoryPath string, policy ProtectionPolicy) (string, error) {
	validatedDestination, errPath := validateLocalPath(destinationDirectoryPath)
	if errPath != nil {
		return "", errPath
	}

	parsedURL, errParseURL := url.Parse(rawURL)
	if errParseURL != nil {
		log.Error().Err(errParseURL).Msg("node: failed to parse download URL")
		return "", errors.New("node: invalid URL")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		log.Error().Str("scheme", parsedURL.Scheme).Msg("node: download URL scheme not allowed")
		return "", errors.New("node: only http and https URLs are allowed")
	}

	errSSRF := webhooks.ValidateWebhookTarget(rawURL)
	if errSSRF != nil {
		return "", fmt.Errorf("node: validate download URL: %w", errSSRF)
	}

	httpClientBase := helpers.GetXylonaHTTPClient()
	httpClientCopy := *httpClientBase
	httpClient := &httpClientCopy
	httpClient.CheckRedirect = validateDownloadRedirectTarget

	req, errNewReq := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if errNewReq != nil {
		return "", fmt.Errorf("node: create download request: %w", errNewReq)
	}

	fileName := strings.TrimPrefix(path.Base(req.URL.Path), "/")
	if !filepath.IsLocal(fileName) {
		return "", ErrInvalidPath
	}

	resp, errGet := httpClient.Do(req)
	if errGet != nil {
		return "", fmt.Errorf("node: download file from URL: %w", errGet)
	}
	defer func() {
		errClose := resp.Body.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("node: close download response body")
		}
	}()

	destinationRelative := filepath.Join(validatedDestination, fileName)
	errProtected := enforceProtection(destinationRelative, policy)
	if errProtected != nil {
		return "", errProtected
	}
	destinationFullPath, errResolve := resolveWithinRoot(directory, destinationRelative)
	if errResolve != nil {
		return "", errResolve
	}

	file, errCreate := os.Create(destinationFullPath)
	if errCreate != nil {
		return "", fmt.Errorf("node: create downloaded file: %w", errCreate)
	}
	defer func() {
		errClose := file.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("node: close downloaded file")
		}
	}()

	_, errCopy := io.Copy(file, resp.Body)
	if errCopy != nil {
		return "", fmt.Errorf("node: write downloaded file: %w", errCopy)
	}
	return destinationRelative, nil
}

func validateDownloadRedirectTarget(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}

	errValidateRedirect := webhooks.ValidateWebhookTarget(req.URL.String())
	if errValidateRedirect != nil {
		return fmt.Errorf("node: download redirect blocked: %w", errValidateRedirect)
	}
	return nil
}
