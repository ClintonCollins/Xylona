package games

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ulikunitz/xz"

	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	factorioHeadlessURL          = "https://factorio.com/get-download/stable/headless/linux64"
	factorioDownloadTime         = 30 * time.Minute
	factorioCreateTime           = 10 * time.Minute
	factorioArchiveMaxBytes      = int64(512 << 20)
	factorioEntryMaxBytes        = int64(1 << 30)
	factorioExtractedMaxBytes    = int64(4 << 30)
	factorioExtractedMaxFiles    = 100_000
	factorioArchiveWorkspaceName = "factorio-headless.tar.xz"
)

// Factorio installs and updates the official Linux headless server build.
type Factorio struct {
	headlessURL     string
	httpClient      *http.Client
	archiveMaxBytes int64
}

type factorioArchiveLimits struct {
	entryBytes int64
	totalBytes int64
	files      int
}

// Install downloads Factorio and creates the initial world save required by the runtime definition.
func (f *Factorio) Install(gameServer *models.GameServer, stdOutWriter, stdErrWriter io.Writer) error {
	errInstall := f.installOrUpdate(gameServer, stdOutWriter, stagedUpdateInstall)
	if errInstall != nil {
		return errInstall
	}
	return createFactorioSave(gameServer, stdOutWriter, stdErrWriter)
}

// Update replaces Factorio's shipped files while preserving generated configuration and saves.
func (f *Factorio) Update(gameServer *models.GameServer, stdOutWriter, _ io.Writer) error {
	return f.installOrUpdate(gameServer, stdOutWriter, stagedUpdateRetainRollback)
}

func (f *Factorio) installOrUpdate(
	gameServer *models.GameServer,
	stdOutWriter io.Writer,
	mode stagedUpdateMode,
) (errResult error) {
	if gameServer == nil {
		return errors.New("install Factorio: game server is nil")
	}
	directory := strings.TrimSpace(gameServer.Directory)
	if directory == "" {
		return errors.New("install Factorio: server directory is empty")
	}
	update, errUpdate := newStagedUpdate(directory, "factorio", mode)
	if errUpdate != nil {
		return fmt.Errorf("install Factorio: prepare staged update: %w", errUpdate)
	}
	defer func() {
		errResult = errors.Join(errResult, wrapError("install Factorio: clean staged update", update.CleanupTransient()))
	}()

	_, errMessage := fmt.Fprintln(stdOutWriter, "Downloading the latest stable Factorio headless server...")
	if errMessage != nil {
		return fmt.Errorf("install Factorio: write download status: %w", errMessage)
	}
	archivePath := update.WorkspacePath(factorioArchiveWorkspaceName)
	errDownload := f.downloadArchive(archivePath)
	if errDownload != nil {
		return errDownload
	}

	_, errExtractMessage := fmt.Fprintln(stdOutWriter, "Extracting the Factorio server...")
	if errExtractMessage != nil {
		return fmt.Errorf("install Factorio: write extraction status: %w", errExtractMessage)
	}
	errExtract := extractFactorioArchive(archivePath, update.PayloadDirectory())
	errRemove := os.Remove(archivePath)
	if errExtract != nil {
		return errors.Join(
			fmt.Errorf("install Factorio: extract archive: %w", errExtract),
			wrapError("install Factorio: remove temporary archive", errRemove),
		)
	}
	if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
		return fmt.Errorf("install Factorio: remove temporary archive: %w", errRemove)
	}
	errValidate := validateFactorioPayload(update.PayloadDirectory())
	if errValidate != nil {
		return errValidate
	}
	errApply := update.Apply(func(relativePath string) bool {
		if !factorioPreservedPath(relativePath) {
			return false
		}
		_, errExisting := os.Lstat(filepath.Join(directory, filepath.FromSlash(relativePath)))
		return errExisting == nil
	}, nil)
	if errApply != nil {
		return fmt.Errorf("install Factorio: commit staged update: %w", errApply)
	}
	_, errCompleteMessage := fmt.Fprintln(stdOutWriter, "Factorio server files are ready.")
	if errCompleteMessage != nil {
		return fmt.Errorf("install Factorio: write completion status: %w", errCompleteMessage)
	}
	return nil
}

func (f *Factorio) downloadArchive(archivePath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), factorioDownloadTime)
	defer cancel()

	downloadURL := strings.TrimSpace(f.headlessURL)
	if downloadURL == "" {
		downloadURL = factorioHeadlessURL
	}
	request, errRequest := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if errRequest != nil {
		return fmt.Errorf("install Factorio: create download request: %w", errRequest)
	}
	request.Header.Set("User-Agent", "Xylona/0.1 (https://github.com/ClintonCollins/Xylona)")
	client := f.httpClient
	if client == nil {
		client = &http.Client{Timeout: factorioDownloadTime}
	}
	response, errResponse := client.Do(request)
	if errResponse != nil {
		return fmt.Errorf("install Factorio: download archive: %w", errResponse)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		errClose := response.Body.Close()
		errStatus := fmt.Errorf("install Factorio: download archive: unexpected HTTP status %s", response.Status)
		return errors.Join(errStatus, wrapError("close download response", errClose))
	}
	maxBytes := f.maxArchiveBytes()
	if response.ContentLength > maxBytes {
		errClose := response.Body.Close()
		return errors.Join(
			fmt.Errorf("install Factorio: archive Content-Length exceeds %d bytes", maxBytes),
			wrapError("close download response", errClose),
		)
	}

	archive, errCreate := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errCreate != nil {
		errClose := response.Body.Close()
		errArchive := fmt.Errorf("install Factorio: create temporary archive: %w", errCreate)
		return errors.Join(errArchive, wrapError("close download response", errClose))
	}
	limited := &io.LimitedReader{R: response.Body, N: maxBytes + 1}
	written, errCopy := io.Copy(archive, limited)
	errSync := archive.Sync()
	errArchiveClose := archive.Close()
	errResponseClose := response.Body.Close()
	if written > maxBytes {
		errCopy = fmt.Errorf("archive exceeds %d bytes", maxBytes)
	}
	if errCopy != nil || errSync != nil || errArchiveClose != nil || errResponseClose != nil {
		errRemove := os.Remove(archivePath)
		return errors.Join(
			wrapError("install Factorio: write temporary archive", errCopy),
			wrapError("install Factorio: sync temporary archive", errSync),
			wrapError("install Factorio: close temporary archive", errArchiveClose),
			wrapError("install Factorio: close download response", errResponseClose),
			wrapError("install Factorio: remove incomplete archive", errRemove),
		)
	}
	return nil
}

func (f *Factorio) maxArchiveBytes() int64 {
	if f.archiveMaxBytes > 0 {
		return f.archiveMaxBytes
	}
	return factorioArchiveMaxBytes
}

func extractFactorioArchive(archivePath string, directory string) error {
	archive, errOpen := os.Open(archivePath)
	if errOpen != nil {
		return fmt.Errorf("open Factorio archive: %w", errOpen)
	}
	xzReader, errXZ := xz.NewReader(archive)
	if errXZ != nil {
		errClose := archive.Close()
		if errClose != nil {
			return errors.Join(fmt.Errorf("open Factorio xz stream: %w", errXZ), fmt.Errorf("close Factorio archive: %w", errClose))
		}
		return fmt.Errorf("open Factorio xz stream: %w", errXZ)
	}

	root, errRoot := os.OpenRoot(directory)
	if errRoot != nil {
		errClose := archive.Close()
		if errClose != nil {
			return errors.Join(fmt.Errorf("open Factorio extraction root: %w", errRoot), fmt.Errorf("close Factorio archive: %w", errClose))
		}
		return fmt.Errorf("open Factorio extraction root: %w", errRoot)
	}

	tarReader := tar.NewReader(xzReader)
	errExtract := extractFactorioTar(root, tarReader, factorioArchiveLimits{
		entryBytes: factorioEntryMaxBytes,
		totalBytes: factorioExtractedMaxBytes,
		files:      factorioExtractedMaxFiles,
	})
	errRootClose := root.Close()
	errArchiveClose := archive.Close()
	if errExtract != nil || errRootClose != nil || errArchiveClose != nil {
		return errors.Join(
			errExtract,
			wrapError("close Factorio extraction root", errRootClose),
			wrapError("close Factorio archive", errArchiveClose),
		)
	}
	return nil
}

func extractFactorioTar(root *os.Root, reader *tar.Reader, limits factorioArchiveLimits) error {
	seen := make(map[string]bool)
	var totalBytes int64
	fileCount := 0
	for {
		header, errNext := reader.Next()
		if errors.Is(errNext, io.EOF) {
			return nil
		}
		if errNext != nil {
			return fmt.Errorf("read Factorio archive: %w", errNext)
		}
		if header.Typeflag == tar.TypeXGlobalHeader || header.Typeflag == tar.TypeXHeader {
			continue
		}

		relativePath, errPath := factorioArchiveRelativePath(header.Name)
		if errPath != nil {
			return errPath
		}
		if relativePath == "" {
			continue
		}
		isDirectory := header.Typeflag == tar.TypeDir
		errCollision := validateFactorioArchivePath(seen, relativePath, isDirectory)
		if errCollision != nil {
			return errCollision
		}
		seen[relativePath] = isDirectory

		switch header.Typeflag {
		case tar.TypeDir:
			errMkdir := root.MkdirAll(relativePath, fs.FileMode(header.Mode&0o777))
			if errMkdir != nil {
				return fmt.Errorf("create Factorio directory %q: %w", relativePath, errMkdir)
			}
		case tar.TypeReg:
			if header.Size < 0 || header.Size > limits.entryBytes {
				return fmt.Errorf("Factorio archive entry %q exceeds %d bytes", header.Name, limits.entryBytes)
			}
			fileCount++
			if fileCount > limits.files {
				return fmt.Errorf("Factorio archive exceeds %d regular files", limits.files)
			}
			if header.Size > limits.totalBytes-totalBytes {
				return fmt.Errorf("Factorio archive exceeds %d extracted bytes", limits.totalBytes)
			}
			totalBytes += header.Size
			errWrite := extractFactorioFile(root, reader, header, relativePath)
			if errWrite != nil {
				return errWrite
			}
		default:
			return fmt.Errorf("Factorio archive entry %q has unsupported type %d", header.Name, header.Typeflag)
		}
	}
}

func factorioArchiveRelativePath(name string) (string, error) {
	normalized := strings.TrimPrefix(filepath.ToSlash(name), "./")
	parts := strings.Split(normalized, "/")
	if len(parts) > 0 && parts[0] == "factorio" {
		parts = parts[1:]
	}
	relativePath := path.Clean(strings.Join(parts, "/"))
	if relativePath == "." || relativePath == "" {
		return "", nil
	}
	if !fs.ValidPath(relativePath) {
		return "", fmt.Errorf("Factorio archive entry has unsafe path %q", name)
	}
	return relativePath, nil
}

func extractFactorioFile(root *os.Root, reader io.Reader, header *tar.Header, relativePath string) error {
	parent := path.Dir(relativePath)
	if parent != "." {
		errMkdir := root.MkdirAll(parent, 0o750)
		if errMkdir != nil {
			return fmt.Errorf("create Factorio file directory %q: %w", parent, errMkdir)
		}
	}
	mode := fs.FileMode(header.Mode & 0o777)
	file, errOpen := root.OpenFile(relativePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if errOpen != nil {
		return fmt.Errorf("create Factorio file %q: %w", relativePath, errOpen)
	}
	_, errCopy := io.CopyN(file, reader, header.Size)
	errClose := file.Close()
	if errCopy != nil || errClose != nil {
		return errors.Join(
			wrapError(fmt.Sprintf("write Factorio file %q", relativePath), errCopy),
			wrapError(fmt.Sprintf("close Factorio file %q", relativePath), errClose),
		)
	}
	return nil
}

func validateFactorioArchivePath(seen map[string]bool, relativePath string, directory bool) error {
	if _, duplicate := seen[relativePath]; duplicate {
		return fmt.Errorf("Factorio archive contains duplicate normalized path %q", relativePath)
	}
	parent := path.Dir(relativePath)
	for parent != "." {
		parentDirectory, exists := seen[parent]
		if exists && !parentDirectory {
			return fmt.Errorf("Factorio archive path %q collides with file %q", relativePath, parent)
		}
		parent = path.Dir(parent)
	}
	if directory {
		return nil
	}
	prefix := relativePath + "/"
	for existing := range seen {
		if strings.HasPrefix(existing, prefix) {
			return fmt.Errorf("Factorio archive file %q collides with child path %q", relativePath, existing)
		}
	}
	return nil
}

func validateFactorioPayload(directory string) error {
	executablePath := filepath.Join(directory, "bin", "x64", "factorio")
	executableInfo, errExecutable := os.Lstat(executablePath)
	if errExecutable != nil {
		return fmt.Errorf("install Factorio: required executable is unavailable: %w", errExecutable)
	}
	if !executableInfo.Mode().IsRegular() || executableInfo.Size() == 0 || executableInfo.Mode().Perm()&0o111 == 0 {
		return errors.New("install Factorio: required executable is not a non-empty executable regular file")
	}
	dataDirectory := filepath.Join(directory, "data")
	dataInfo, errData := os.Lstat(dataDirectory)
	if errData != nil {
		return fmt.Errorf("install Factorio: required data payload is unavailable: %w", errData)
	}
	if !dataInfo.IsDir() || dataInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("install Factorio: required data payload is not a directory")
	}
	foundDataFile := false
	errWalk := filepath.WalkDir(dataDirectory, func(currentPath string, entry fs.DirEntry, errWalk error) error {
		if errWalk != nil {
			return errWalk
		}
		if currentPath == dataDirectory {
			return nil
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			return fmt.Errorf("inspect Factorio data payload path %q: %w", currentPath, errInfo)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported data payload path %q", currentPath)
		}
		if info.Mode().IsRegular() && info.Size() > 0 {
			foundDataFile = true
		}
		return nil
	})
	if errWalk != nil {
		return fmt.Errorf("install Factorio: validate data payload: %w", errWalk)
	}
	if !foundDataFile {
		return errors.New("install Factorio: required data payload contains no non-empty files")
	}
	return nil
}

func factorioPreservedPath(relativePath string) bool {
	normalized := strings.ToLower(filepath.ToSlash(relativePath))
	for _, prefix := range []string{"saves/", "mods/", "config/", "script-output/"} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	switch normalized {
	case "player-data.json", "achievements.dat", "data/server-settings.json":
		return true
	default:
		return false
	}
}

func createFactorioSave(gameServer *models.GameServer, stdOutWriter, stdErrWriter io.Writer) error {
	savePath := filepath.Join(gameServer.Directory, "saves", "default.zip")
	_, errStat := os.Stat(savePath)
	if errStat == nil {
		return nil
	}
	if !errors.Is(errStat, os.ErrNotExist) {
		return fmt.Errorf("install Factorio: inspect initial save: %w", errStat)
	}

	_, errMessage := fmt.Fprintln(stdOutWriter, "Creating the initial Factorio world...")
	if errMessage != nil {
		return fmt.Errorf("install Factorio: write world creation status: %w", errMessage)
	}
	ctx, cancel := context.WithTimeout(context.Background(), factorioCreateTime)
	defer cancel()
	executable := filepath.Join(gameServer.Directory, "bin", "x64", "factorio")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	command := exec.CommandContext(ctx, executable, "--create", filepath.ToSlash(filepath.Join("saves", "default.zip"))) // #nosec G204 -- executable and save path are fixed beneath the controller-managed install root.
	command.Dir = gameServer.Directory
	command.Stdout = stdOutWriter
	command.Stderr = stdErrWriter
	errRun := command.Run()
	if errRun != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("install Factorio: create initial world: %w", ctx.Err())
		}
		return fmt.Errorf("install Factorio: create initial world: %w", errRun)
	}
	return nil
}

func wrapError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}
