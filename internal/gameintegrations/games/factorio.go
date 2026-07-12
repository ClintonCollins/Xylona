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
	factorioHeadlessURL  = "https://factorio.com/get-download/stable/headless/linux64"
	factorioDownloadTime = 30 * time.Minute
	factorioCreateTime   = 10 * time.Minute
)

// Factorio installs and updates the official Linux headless server build.
type Factorio struct {
}

// Install downloads Factorio and creates the initial world save required by the runtime definition.
func (f *Factorio) Install(gameServer *models.GameServer, stdOutWriter, stdErrWriter io.Writer) error {
	errInstall := installFactorio(gameServer, stdOutWriter)
	if errInstall != nil {
		return errInstall
	}
	return createFactorioSave(gameServer, stdOutWriter, stdErrWriter)
}

// Update replaces Factorio's shipped files while preserving generated configuration and saves.
func (f *Factorio) Update(gameServer *models.GameServer, stdOutWriter, _ io.Writer) error {
	return installFactorio(gameServer, stdOutWriter)
}

func installFactorio(gameServer *models.GameServer, stdOutWriter io.Writer) error {
	if gameServer == nil {
		return errors.New("install Factorio: game server is nil")
	}
	directory := strings.TrimSpace(gameServer.Directory)
	if directory == "" {
		return errors.New("install Factorio: server directory is empty")
	}
	errDirectory := os.MkdirAll(directory, 0o750)
	if errDirectory != nil {
		return fmt.Errorf("install Factorio: create server directory: %w", errDirectory)
	}

	_, errMessage := fmt.Fprintln(stdOutWriter, "Downloading the latest stable Factorio headless server...")
	if errMessage != nil {
		return fmt.Errorf("install Factorio: write download status: %w", errMessage)
	}
	archivePath, errDownload := downloadFactorioArchive(directory)
	if errDownload != nil {
		return errDownload
	}

	_, errExtractMessage := fmt.Fprintln(stdOutWriter, "Extracting the Factorio server...")
	if errExtractMessage != nil {
		errRemove := os.Remove(archivePath)
		return errors.Join(
			fmt.Errorf("install Factorio: write extraction status: %w", errExtractMessage),
			wrapError("install Factorio: remove temporary archive", errRemove),
		)
	}
	errExtract := extractFactorioArchive(archivePath, directory)
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
	_, errCompleteMessage := fmt.Fprintln(stdOutWriter, "Factorio server files are ready.")
	if errCompleteMessage != nil {
		return fmt.Errorf("install Factorio: write completion status: %w", errCompleteMessage)
	}
	return nil
}

func downloadFactorioArchive(directory string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), factorioDownloadTime)
	defer cancel()

	request, errRequest := http.NewRequestWithContext(ctx, http.MethodGet, factorioHeadlessURL, nil)
	if errRequest != nil {
		return "", fmt.Errorf("install Factorio: create download request: %w", errRequest)
	}
	request.Header.Set("User-Agent", "Xylona/0.1 (https://github.com/ClintonCollins/Xylona)")
	client := &http.Client{Timeout: factorioDownloadTime}
	response, errResponse := client.Do(request)
	if errResponse != nil {
		return "", fmt.Errorf("install Factorio: download archive: %w", errResponse)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		errClose := response.Body.Close()
		errStatus := fmt.Errorf("install Factorio: download archive: unexpected HTTP status %s", response.Status)
		if errClose != nil {
			return "", errors.Join(errStatus, fmt.Errorf("close download response: %w", errClose))
		}
		return "", errStatus
	}

	archive, errCreate := os.CreateTemp(directory, ".factorio-headless-*.tar.xz")
	if errCreate != nil {
		errClose := response.Body.Close()
		errArchive := fmt.Errorf("install Factorio: create temporary archive: %w", errCreate)
		if errClose != nil {
			return "", errors.Join(errArchive, fmt.Errorf("close download response: %w", errClose))
		}
		return "", errArchive
	}
	archivePath := archive.Name()
	_, errCopy := io.Copy(archive, response.Body)
	errArchiveClose := archive.Close()
	errResponseClose := response.Body.Close()
	if errCopy != nil || errArchiveClose != nil || errResponseClose != nil {
		errRemove := os.Remove(archivePath)
		errDownload := errors.Join(
			wrapError("install Factorio: write temporary archive", errCopy),
			wrapError("install Factorio: close temporary archive", errArchiveClose),
			wrapError("install Factorio: close download response", errResponseClose),
			wrapError("install Factorio: remove incomplete archive", errRemove),
		)
		return "", errDownload
	}
	return archivePath, nil
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
	errExtract := extractFactorioTar(root, tarReader)
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

func extractFactorioTar(root *os.Root, reader *tar.Reader) error {
	for {
		header, errNext := reader.Next()
		if errors.Is(errNext, io.EOF) {
			return nil
		}
		if errNext != nil {
			return fmt.Errorf("read Factorio archive: %w", errNext)
		}

		relativePath, errPath := factorioArchiveRelativePath(header.Name)
		if errPath != nil {
			return errPath
		}
		if relativePath == "" {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			errMkdir := root.MkdirAll(relativePath, fs.FileMode(header.Mode&0o777))
			if errMkdir != nil {
				return fmt.Errorf("create Factorio directory %q: %w", relativePath, errMkdir)
			}
		case tar.TypeReg:
			errWrite := extractFactorioFile(root, reader, header, relativePath)
			if errWrite != nil {
				return errWrite
			}
		case tar.TypeXGlobalHeader, tar.TypeXHeader:
			continue
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
	_, errCopy := io.Copy(file, reader)
	errClose := file.Close()
	if errCopy != nil || errClose != nil {
		return errors.Join(
			wrapError(fmt.Sprintf("write Factorio file %q", relativePath), errCopy),
			wrapError(fmt.Sprintf("close Factorio file %q", relativePath), errClose),
		)
	}
	return nil
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
