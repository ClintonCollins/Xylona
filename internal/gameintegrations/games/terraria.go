package games

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	terrariaServerNamesURL    = "https://terraria.org/api/get/dedicated-servers-names"
	terrariaServerDownloadURL = "https://terraria.org/api/download/pc-dedicated-server"
	terrariaMetadataMaxBytes  = 64 << 10
	terrariaArchiveMaxBytes   = 512 << 20
	terrariaArchiveEntryBytes = 512 << 20
	terrariaDownloadTimeout   = 30 * time.Minute
)

// Terraria installs the free dedicated-server package published by Re-Logic.
// The Steam client app requires ownership and is not a turnkey server source.
type Terraria struct {
	serverNamesURL string
	downloadURL    string
	httpClient     *http.Client
	runtimeOS      string
}

// Install downloads and extracts the current official package for this node's
// operating system.
func (t *Terraria) Install(gameServer *models.GameServer, stdOutWriter, _ io.Writer) error {
	return t.installOrUpdate(gameServer, stdOutWriter, "Installing", stagedUpdateInstall)
}

// Update refreshes shipped binaries while preserving server configuration and
// worlds created inside the managed server directory.
func (t *Terraria) Update(gameServer *models.GameServer, stdOutWriter, _ io.Writer) error {
	return t.installOrUpdate(gameServer, stdOutWriter, "Updating", stagedUpdateRetainRollback)
}

func (t *Terraria) installOrUpdate(
	gameServer *models.GameServer,
	stdOutWriter io.Writer,
	action string,
	mode stagedUpdateMode,
) (errResult error) {
	directory, errDirectory := prepareTerrariaDirectory(gameServer)
	if errDirectory != nil {
		return errDirectory
	}
	update, errUpdate := newStagedUpdate(directory, "terraria", mode)
	if errUpdate != nil {
		return fmt.Errorf("install Terraria: prepare staged update: %w", errUpdate)
	}
	defer func() {
		errResult = errors.Join(errResult, wrapError("install Terraria: clean staged update", update.CleanupTransient()))
	}()

	archiveName, errArchiveName := t.latestArchiveName()
	if errArchiveName != nil {
		return errArchiveName
	}
	_, errMessage := fmt.Fprintf(stdOutWriter, "%s Terraria from official package %s...\n", action, archiveName)
	if errMessage != nil {
		return fmt.Errorf("install Terraria: write download status: %w", errMessage)
	}

	archivePath, errDownload := t.downloadArchive(update.workspace, archiveName)
	if errDownload != nil {
		return errDownload
	}
	errExtract := extractTerrariaArchive(archivePath, update.PayloadDirectory(), t.operatingSystem())
	errRemove := os.Remove(archivePath)
	if errExtract != nil || (errRemove != nil && !errors.Is(errRemove, os.ErrNotExist)) {
		return errors.Join(
			wrapError("install Terraria: extract archive", errExtract),
			wrapError("install Terraria: remove temporary archive", errRemove),
		)
	}
	errValidate := validateTerrariaPayload(update.PayloadDirectory(), t.operatingSystem())
	if errValidate != nil {
		return errValidate
	}
	errApply := update.Apply(func(relativePath string) bool {
		if !terrariaPreservedPath(relativePath) {
			return false
		}
		_, errExisting := os.Lstat(filepath.Join(directory, filepath.FromSlash(relativePath)))
		return errExisting == nil
	}, nil)
	if errApply != nil {
		return fmt.Errorf("install Terraria: commit staged update: %w", errApply)
	}

	_, errCompleteMessage := fmt.Fprintln(stdOutWriter, "Terraria dedicated server files are ready.")
	if errCompleteMessage != nil {
		return fmt.Errorf("install Terraria: write completion status: %w", errCompleteMessage)
	}
	return nil
}

func prepareTerrariaDirectory(gameServer *models.GameServer) (string, error) {
	if gameServer == nil {
		return "", errors.New("install Terraria: game server is nil")
	}
	directory := strings.TrimSpace(gameServer.Directory)
	if directory == "" {
		return "", errors.New("install Terraria: server directory is empty")
	}
	errMkdir := os.MkdirAll(filepath.Join(directory, "worlds"), 0o750)
	if errMkdir != nil {
		return "", fmt.Errorf("install Terraria: create server directory: %w", errMkdir)
	}
	return directory, nil
}

func (t *Terraria) latestArchiveName() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), terrariaDownloadTimeout)
	defer cancel()
	request, errRequest := http.NewRequestWithContext(ctx, http.MethodGet, t.namesURL(), nil)
	if errRequest != nil {
		return "", fmt.Errorf("install Terraria: create package metadata request: %w", errRequest)
	}
	request.Header.Set("User-Agent", "Xylona/0.1 (https://github.com/ClintonCollins/Xylona)")
	response, errResponse := t.client().Do(request)
	if errResponse != nil {
		return "", fmt.Errorf("install Terraria: request package metadata: %w", errResponse)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		errClose := response.Body.Close()
		errStatus := fmt.Errorf("install Terraria: package metadata returned %s", response.Status)
		return "", errors.Join(errStatus, wrapError("close Terraria metadata response", errClose))
	}

	metadata, errRead := io.ReadAll(io.LimitReader(response.Body, terrariaMetadataMaxBytes+1))
	errClose := response.Body.Close()
	if errRead != nil || errClose != nil {
		return "", errors.Join(
			wrapError("install Terraria: read package metadata", errRead),
			wrapError("install Terraria: close package metadata response", errClose),
		)
	}
	if len(metadata) > terrariaMetadataMaxBytes {
		return "", errors.New("install Terraria: package metadata exceeds size limit")
	}

	var archiveNames []string
	errDecode := json.Unmarshal(metadata, &archiveNames)
	if errDecode != nil {
		return "", fmt.Errorf("install Terraria: decode package metadata: %w", errDecode)
	}
	if len(archiveNames) == 0 {
		return "", errors.New("install Terraria: official package metadata is empty")
	}
	archiveName := strings.TrimSpace(archiveNames[0])
	if archiveName == "" || path.Base(archiveName) != archiveName || !strings.HasSuffix(strings.ToLower(archiveName), ".zip") {
		return "", fmt.Errorf("install Terraria: unsafe package name %q", archiveName)
	}
	return archiveName, nil
}

func (t *Terraria) downloadArchive(directory string, archiveName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), terrariaDownloadTimeout)
	defer cancel()
	downloadURL := strings.TrimRight(t.archiveBaseURL(), "/") + "/" + url.PathEscape(archiveName)
	request, errRequest := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if errRequest != nil {
		return "", fmt.Errorf("install Terraria: create package request: %w", errRequest)
	}
	request.Header.Set("User-Agent", "Xylona/0.1 (https://github.com/ClintonCollins/Xylona)")
	response, errResponse := t.client().Do(request)
	if errResponse != nil {
		return "", fmt.Errorf("install Terraria: download package: %w", errResponse)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		errClose := response.Body.Close()
		errStatus := fmt.Errorf("install Terraria: package download returned %s", response.Status)
		return "", errors.Join(errStatus, wrapError("close Terraria package response", errClose))
	}
	if response.ContentLength > terrariaArchiveMaxBytes {
		errClose := response.Body.Close()
		errSize := fmt.Errorf("install Terraria: package is too large: %d bytes", response.ContentLength)
		return "", errors.Join(errSize, wrapError("close Terraria package response", errClose))
	}

	archive, errCreate := os.CreateTemp(directory, ".terraria-server-*.zip")
	if errCreate != nil {
		errClose := response.Body.Close()
		return "", errors.Join(
			fmt.Errorf("install Terraria: create temporary archive: %w", errCreate),
			wrapError("close Terraria package response", errClose),
		)
	}
	archivePath := archive.Name()
	written, errCopy := io.Copy(archive, io.LimitReader(response.Body, terrariaArchiveMaxBytes+1))
	errArchiveSync := archive.Sync()
	errArchiveClose := archive.Close()
	errResponseClose := response.Body.Close()
	if errCopy != nil || errArchiveSync != nil || errArchiveClose != nil || errResponseClose != nil || written > terrariaArchiveMaxBytes {
		errRemove := os.Remove(archivePath)
		var errSize error
		if written > terrariaArchiveMaxBytes {
			errSize = fmt.Errorf("install Terraria: package exceeds %d bytes", terrariaArchiveMaxBytes)
		}
		return "", errors.Join(
			wrapError("install Terraria: write temporary archive", errCopy),
			wrapError("install Terraria: sync temporary archive", errArchiveSync),
			wrapError("install Terraria: close temporary archive", errArchiveClose),
			wrapError("install Terraria: close package response", errResponseClose),
			errSize,
			wrapError("install Terraria: remove incomplete archive", errRemove),
		)
	}
	return archivePath, nil
}

func extractTerrariaArchive(archivePath string, directory string, operatingSystem string) error {
	platform, errPlatform := terrariaArchivePlatform(operatingSystem)
	if errPlatform != nil {
		return errPlatform
	}
	archive, errOpen := zip.OpenReader(archivePath)
	if errOpen != nil {
		return fmt.Errorf("open Terraria archive: %w", errOpen)
	}
	root, errRoot := os.OpenRoot(directory)
	if errRoot != nil {
		errClose := archive.Close()
		return errors.Join(
			fmt.Errorf("open Terraria extraction root: %w", errRoot),
			wrapError("close Terraria archive", errClose),
		)
	}

	matchedFiles := 0
	seenPaths := make(map[string]bool)
	var errExtract error
	for _, archiveFile := range archive.File {
		relativePath, selected, errPath := terrariaArchiveRelativePath(archiveFile.Name, platform)
		if errPath != nil {
			errExtract = errPath
			break
		}
		if !selected || relativePath == "" {
			continue
		}
		isDirectory := archiveFile.FileInfo().IsDir()
		errCollision := validateTerrariaArchivePath(seenPaths, relativePath, isDirectory)
		if errCollision != nil {
			errExtract = errCollision
			break
		}
		seenPaths[relativePath] = isDirectory
		if isDirectory {
			continue
		}
		if archiveFile.Mode()&os.ModeType != 0 {
			errExtract = fmt.Errorf("Terraria archive entry %q is not a regular file", archiveFile.Name)
			break
		}
		errWrite := extractTerrariaFile(root, archiveFile, relativePath)
		if errWrite != nil {
			errExtract = errWrite
			break
		}
		matchedFiles++
	}
	if errExtract == nil && matchedFiles == 0 {
		errExtract = fmt.Errorf("Terraria archive contains no %s server files", platform)
	}
	errRootClose := root.Close()
	errArchiveClose := archive.Close()
	return errors.Join(
		errExtract,
		wrapError("close Terraria extraction root", errRootClose),
		wrapError("close Terraria archive", errArchiveClose),
	)
}

func terrariaArchivePlatform(operatingSystem string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(operatingSystem)) {
	case "windows":
		return "Windows", nil
	case "linux":
		return "Linux", nil
	default:
		return "", fmt.Errorf("install Terraria: unsupported operating system %q", operatingSystem)
	}
}

func terrariaArchiveRelativePath(name string, platform string) (string, bool, error) {
	normalized := strings.TrimPrefix(filepath.ToSlash(name), "./")
	parts := strings.Split(normalized, "/")
	if len(parts) < 2 || parts[1] != platform {
		return "", false, nil
	}
	if len(parts) == 2 {
		return "", true, nil
	}
	relativePath := path.Clean(strings.Join(parts[2:], "/"))
	if relativePath == "." || relativePath == "" {
		return "", true, nil
	}
	if !fs.ValidPath(relativePath) {
		return "", false, fmt.Errorf("Terraria archive entry has unsafe path %q", name)
	}
	return relativePath, true, nil
}

func extractTerrariaFile(root *os.Root, archiveFile *zip.File, relativePath string) error {
	if archiveFile.UncompressedSize64 > terrariaArchiveEntryBytes {
		return fmt.Errorf(
			"Terraria archive file %q is %d bytes, limit is %d",
			archiveFile.Name,
			archiveFile.UncompressedSize64,
			terrariaArchiveEntryBytes,
		)
	}
	parent := path.Dir(relativePath)
	if parent != "." {
		errMkdir := root.MkdirAll(parent, 0o750)
		if errMkdir != nil {
			return fmt.Errorf("create Terraria directory %q: %w", parent, errMkdir)
		}
	}

	reader, errOpenArchiveFile := archiveFile.Open()
	if errOpenArchiveFile != nil {
		return fmt.Errorf("open Terraria archive file %q: %w", archiveFile.Name, errOpenArchiveFile)
	}
	mode := archiveFile.Mode().Perm()
	if mode == 0 {
		mode = 0o640
	}
	if filepath.Base(relativePath) == "TerrariaServer" || filepath.Base(relativePath) == "TerrariaServer.bin.x86_64" {
		mode |= 0o110
	}
	output, errOpenOutput := root.OpenFile(relativePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if errOpenOutput != nil {
		errClose := reader.Close()
		return errors.Join(
			fmt.Errorf("create Terraria file %q: %w", relativePath, errOpenOutput),
			wrapError("close Terraria archive file", errClose),
		)
	}
	written, errCopy := io.Copy(output, io.LimitReader(reader, terrariaArchiveEntryBytes+1))
	if written > terrariaArchiveEntryBytes {
		errCopy = fmt.Errorf("Terraria archive file %q exceeds %d bytes", archiveFile.Name, terrariaArchiveEntryBytes)
	}
	errOutputClose := output.Close()
	errReaderClose := reader.Close()
	if errCopy != nil || errOutputClose != nil || errReaderClose != nil {
		return errors.Join(
			wrapError(fmt.Sprintf("write Terraria file %q", relativePath), errCopy),
			wrapError(fmt.Sprintf("close Terraria file %q", relativePath), errOutputClose),
			wrapError(fmt.Sprintf("close Terraria archive file %q", archiveFile.Name), errReaderClose),
		)
	}
	return nil
}

func validateTerrariaArchivePath(seen map[string]bool, relativePath string, directory bool) error {
	if _, duplicate := seen[relativePath]; duplicate {
		return fmt.Errorf("Terraria archive contains duplicate normalized path %q", relativePath)
	}
	parent := path.Dir(relativePath)
	for parent != "." {
		parentDirectory, exists := seen[parent]
		if exists && !parentDirectory {
			return fmt.Errorf("Terraria archive path %q collides with file %q", relativePath, parent)
		}
		parent = path.Dir(parent)
	}
	if directory {
		return nil
	}
	prefix := relativePath + "/"
	for existing := range seen {
		if strings.HasPrefix(existing, prefix) {
			return fmt.Errorf("Terraria archive file %q collides with child path %q", relativePath, existing)
		}
	}
	return nil
}

func validateTerrariaPayload(directory string, operatingSystem string) error {
	platform, errPlatform := terrariaArchivePlatform(operatingSystem)
	if errPlatform != nil {
		return errPlatform
	}
	executable := "TerrariaServer.bin.x86_64"
	if platform == "Windows" {
		executable = "TerrariaServer.exe"
	}
	executablePath := filepath.Join(directory, executable)
	info, errStat := os.Lstat(executablePath)
	if errStat != nil {
		return fmt.Errorf("install Terraria: required %s executable is unavailable: %w", platform, errStat)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("install Terraria: required %s executable is not a non-empty regular file", platform)
	}
	if platform != "Windows" && runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return errors.New("install Terraria: required Linux executable is not executable")
	}
	return nil
}

func terrariaPreservedPath(relativePath string) bool {
	normalized := strings.ToLower(filepath.ToSlash(relativePath))
	if normalized == "serverconfig.txt" {
		return true
	}
	if normalized == "banlist.txt" || normalized == "whitelist.txt" || normalized == "allowlist.txt" {
		return true
	}
	return strings.HasPrefix(normalized, "worlds/")
}

func (t *Terraria) namesURL() string {
	if strings.TrimSpace(t.serverNamesURL) != "" {
		return t.serverNamesURL
	}
	return terrariaServerNamesURL
}

func (t *Terraria) archiveBaseURL() string {
	if strings.TrimSpace(t.downloadURL) != "" {
		return t.downloadURL
	}
	return terrariaServerDownloadURL
}

func (t *Terraria) client() *http.Client {
	if t.httpClient != nil {
		return t.httpClient
	}
	return &http.Client{Timeout: terrariaDownloadTimeout}
}

func (t *Terraria) operatingSystem() string {
	if strings.TrimSpace(t.runtimeOS) != "" {
		return t.runtimeOS
	}
	return runtime.GOOS
}
