package games

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
	"github.com/ClintonCollins/Xylona/internal/processenv"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	hytaleMavenBaseURL     = "https://maven.hytale.com/release/com/hypixel/hytale/Server"
	hytaleSeedJARName      = "HytaleServer.jar"
	hytaleLauncherFileName = "XylonaHytaleLauncher.java"
	hytaleServerJARPath    = "Server/HytaleServer.jar"
	hytaleAssetsPath       = "Assets.zip"
	hytaleMetadataMaxBytes = 1 << 20
	hytaleJARMaxBytes      = 512 << 20
	hytaleDownloadTimeout  = 30 * time.Minute
	hytaleBootstrapTimeout = 2 * time.Hour

	// #nosec G101 -- These are environment variable names, not secret values.
	hytaleSessionTokenEnv = "HYTALE_SERVER_SESSION_TOKEN"
	// #nosec G101 -- These are environment variable names, not secret values.
	hytaleIdentityTokenEnv = "HYTALE_SERVER_IDENTITY_TOKEN"
)

// Hytale installs the public bootstrap JAR and uses Xylona's ephemeral Hytale
// session credentials to obtain the full dedicated-server payload.
type Hytale struct {
	mavenBaseURL string
	httpClient   *http.Client
	javaCommand  string
}

type hytaleMavenMetadata struct {
	Versioning struct {
		Latest  string `xml:"latest"`
		Release string `xml:"release"`
	} `xml:"versioning"`
}

// Install downloads the current public bootstrap JAR and writes the portable
// Java launcher used for bootstrap, normal runtime, console I/O, and staged
// updates. Full game assets are downloaded on first authenticated start.
func (h *Hytale) Install(gameServer *models.GameServer, stdOutWriter, _ io.Writer) error {
	directory, errDirectory := h.prepareDirectory(gameServer)
	if errDirectory != nil {
		return errDirectory
	}

	errLauncher := writeHytaleLauncher(directory)
	if errLauncher != nil {
		return errLauncher
	}

	fullServerPath := filepath.Join(directory, filepath.FromSlash(hytaleServerJARPath))
	fullServerInfo, errFullServer := os.Stat(fullServerPath)
	if errFullServer == nil && fullServerInfo.Mode().IsRegular() {
		_, errMessage := fmt.Fprintln(stdOutWriter, "Hytale server files are already installed; launcher refreshed.")
		if errMessage != nil {
			return fmt.Errorf("install Hytale: write completion status: %w", errMessage)
		}
		return nil
	}
	if errFullServer != nil && !errors.Is(errFullServer, os.ErrNotExist) {
		return fmt.Errorf("install Hytale: inspect full server JAR: %w", errFullServer)
	}

	_, errMessage := fmt.Fprintln(stdOutWriter, "Downloading the current official Hytale bootstrap server...")
	if errMessage != nil {
		return fmt.Errorf("install Hytale: write download status: %w", errMessage)
	}
	version, errDownload := h.downloadSeedJAR(directory)
	if errDownload != nil {
		return errDownload
	}
	_, errCompleteMessage := fmt.Fprintf(
		stdOutWriter,
		"Hytale bootstrap %s is ready. Link a Hytale account, then start the server to download the full payload.\n",
		version,
	)
	if errCompleteMessage != nil {
		return fmt.Errorf("install Hytale: write completion status: %w", errCompleteMessage)
	}
	return nil
}

// Update requires the ephemeral account credentials supplied by Xylona's
// encrypted Hytale readiness flow.
func (h *Hytale) Update(_ *models.GameServer, _, _ io.Writer) error {
	return errors.New("update Hytale: authenticated launch environment is required")
}

// UpdateWithEnvironment downloads a fresh public bootstrap JAR and lets the
// official bootstrap updater replace the full server and assets atomically.
func (h *Hytale) UpdateWithEnvironment(
	gameServer *models.GameServer,
	stdOutWriter io.Writer,
	stdErrWriter io.Writer,
	environment map[string]string,
) (errResult error) {
	errCredentials := validateHytaleUpdateEnvironment(environment)
	if errCredentials != nil {
		return errCredentials
	}
	directory, errDirectory := h.prepareDirectory(gameServer)
	if errDirectory != nil {
		return errDirectory
	}
	bootstrapDirectory, errBootstrapDirectory := os.MkdirTemp(directory, ".xylona-hytale-bootstrap-")
	if errBootstrapDirectory != nil {
		return fmt.Errorf("update Hytale: create bootstrap workspace: %w", errBootstrapDirectory)
	}
	defer func() {
		errResult = errors.Join(
			errResult,
			wrapError("update Hytale: clean bootstrap workspace", removeStagedUpdateTree(directory, bootstrapDirectory)),
		)
	}()

	_, errMessage := fmt.Fprintln(stdOutWriter, "Downloading the latest Hytale bootstrap before updating...")
	if errMessage != nil {
		return fmt.Errorf("update Hytale: write download status: %w", errMessage)
	}
	version, errDownload := h.downloadSeedJAR(bootstrapDirectory)
	if errDownload != nil {
		return errDownload
	}

	_, errBootstrapMessage := fmt.Fprintf(stdOutWriter, "Running authenticated Hytale bootstrap %s...\n", version)
	if errBootstrapMessage != nil {
		return fmt.Errorf("update Hytale: write bootstrap status: %w", errBootstrapMessage)
	}
	bootstrapJARPath := filepath.Join(bootstrapDirectory, hytaleSeedJARName)
	errBootstrap := h.runBootstrap(directory, bootstrapJARPath, environment, stdOutWriter, stdErrWriter)
	if errBootstrap != nil {
		return errBootstrap
	}

	errValidate := validateHytalePayload(directory)
	if errValidate != nil {
		return errValidate
	}
	_, errCompleteMessage := fmt.Fprintln(stdOutWriter, "Hytale server payload is up to date.")
	if errCompleteMessage != nil {
		return fmt.Errorf("update Hytale: write completion status: %w", errCompleteMessage)
	}
	return nil
}

func (h *Hytale) prepareDirectory(gameServer *models.GameServer) (string, error) {
	if gameServer == nil {
		return "", errors.New("install Hytale: game server is nil")
	}
	directory := strings.TrimSpace(gameServer.Directory)
	if directory == "" {
		return "", errors.New("install Hytale: server directory is empty")
	}
	errDirectory := os.MkdirAll(filepath.Join(directory, "Server"), 0o750)
	if errDirectory != nil {
		return "", fmt.Errorf("install Hytale: create server directory: %w", errDirectory)
	}
	return directory, nil
}

func (h *Hytale) downloadSeedJAR(directory string) (string, error) {
	version, errVersion := h.latestVersion()
	if errVersion != nil {
		return "", errVersion
	}
	jarURL := h.baseURL() + "/" + version + "/Server-" + version + ".jar"
	request, errRequest := http.NewRequestWithContext(context.Background(), http.MethodGet, jarURL, nil)
	if errRequest != nil {
		return "", fmt.Errorf("install Hytale: create JAR request: %w", errRequest)
	}
	request.Header.Set("User-Agent", "Xylona/0.1 (https://github.com/ClintonCollins/Xylona)")
	response, errResponse := h.client().Do(request)
	if errResponse != nil {
		return "", fmt.Errorf("install Hytale: download server JAR: %w", errResponse)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		errClose := response.Body.Close()
		errStatus := fmt.Errorf("install Hytale: download server JAR: unexpected HTTP status %s", response.Status)
		return "", errors.Join(errStatus, wrapError("close Hytale JAR response", errClose))
	}

	temporary, errCreate := os.CreateTemp(directory, ".hytale-server-*.jar")
	if errCreate != nil {
		errClose := response.Body.Close()
		return "", errors.Join(
			fmt.Errorf("install Hytale: create temporary JAR: %w", errCreate),
			wrapError("close Hytale JAR response", errClose),
		)
	}
	temporaryPath := temporary.Name()
	limitedReader := &io.LimitedReader{R: response.Body, N: hytaleJARMaxBytes + 1}
	written, errCopy := io.Copy(temporary, limitedReader)
	errSync := temporary.Sync()
	errTemporaryClose := temporary.Close()
	errResponseClose := response.Body.Close()
	if written > hytaleJARMaxBytes {
		errCopy = fmt.Errorf("server JAR exceeds %d bytes", hytaleJARMaxBytes)
	}
	if errCopy != nil || errSync != nil || errTemporaryClose != nil || errResponseClose != nil {
		errRemove := os.Remove(temporaryPath)
		return "", errors.Join(
			wrapError("install Hytale: write temporary JAR", errCopy),
			wrapError("install Hytale: sync temporary JAR", errSync),
			wrapError("install Hytale: close temporary JAR", errTemporaryClose),
			wrapError("install Hytale: close JAR response", errResponseClose),
			wrapError("install Hytale: remove incomplete JAR", errRemove),
		)
	}
	errArchive := validateHytaleJAR(temporaryPath)
	if errArchive != nil {
		errRemove := os.Remove(temporaryPath)
		return "", errors.Join(errArchive, wrapError("install Hytale: remove invalid JAR", errRemove))
	}

	targetPath := filepath.Join(directory, hytaleSeedJARName)
	errRename := replaceHytaleFile(temporaryPath, targetPath)
	if errRename != nil {
		errRemove := os.Remove(temporaryPath)
		return "", errors.Join(
			fmt.Errorf("install Hytale: replace bootstrap JAR: %w", errRename),
			wrapError("install Hytale: remove temporary JAR", errRemove),
		)
	}
	return version, nil
}

func (h *Hytale) latestVersion() (string, error) {
	metadataURL := h.baseURL() + "/maven-metadata.xml"
	request, errRequest := http.NewRequestWithContext(context.Background(), http.MethodGet, metadataURL, nil)
	if errRequest != nil {
		return "", fmt.Errorf("install Hytale: create metadata request: %w", errRequest)
	}
	request.Header.Set("User-Agent", "Xylona/0.1 (https://github.com/ClintonCollins/Xylona)")
	response, errResponse := h.client().Do(request)
	if errResponse != nil {
		return "", fmt.Errorf("install Hytale: download Maven metadata: %w", errResponse)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		errClose := response.Body.Close()
		errStatus := fmt.Errorf("install Hytale: download Maven metadata: unexpected HTTP status %s", response.Status)
		return "", errors.Join(errStatus, wrapError("close Hytale metadata response", errClose))
	}

	metadataBytes, errRead := io.ReadAll(io.LimitReader(response.Body, hytaleMetadataMaxBytes+1))
	errClose := response.Body.Close()
	if errRead != nil || errClose != nil {
		return "", errors.Join(
			wrapError("install Hytale: read Maven metadata", errRead),
			wrapError("install Hytale: close Maven metadata response", errClose),
		)
	}
	if len(metadataBytes) > hytaleMetadataMaxBytes {
		return "", fmt.Errorf("install Hytale: Maven metadata exceeds %d bytes", hytaleMetadataMaxBytes)
	}

	var metadata hytaleMavenMetadata
	errXML := xml.Unmarshal(metadataBytes, &metadata)
	if errXML != nil {
		return "", fmt.Errorf("install Hytale: parse Maven metadata: %w", errXML)
	}
	version := strings.TrimSpace(metadata.Versioning.Latest)
	if version == "" {
		version = strings.TrimSpace(metadata.Versioning.Release)
	}
	if !validHytaleMavenVersion(version) {
		return "", fmt.Errorf("install Hytale: Maven metadata returned unsafe version %q", version)
	}
	return version, nil
}

func (h *Hytale) runBootstrap(
	directory string,
	bootstrapJARPath string,
	environment map[string]string,
	stdOutWriter io.Writer,
	stdErrWriter io.Writer,
) error {
	errBootstrapConfig := prepareHytaleBootstrapConfig(directory)
	if errBootstrapConfig != nil {
		return errBootstrapConfig
	}

	ctx, cancel := context.WithTimeout(context.Background(), hytaleBootstrapTimeout)
	defer cancel()
	command := exec.CommandContext( //nolint:gosec // Java and fixed Hytale arguments are controlled by the built-in integration.
		ctx,
		h.javaExecutable(),
		"-Xms512M",
		"-Xmx1G",
		"-jar",
		bootstrapJARPath,
		"--bootstrap",
		"--boot-command",
		"update download --force",
	)
	command.Dir = directory
	command.Env = hytaleBootstrapEnvironment(os.Environ(), environment)
	command.Stdout = stdOutWriter
	command.Stderr = stdErrWriter
	errRun := command.Run()
	if errRun != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("update Hytale: bootstrap timed out: %w", ctx.Err())
		}
		exitError := &exec.ExitError{}
		if errors.As(errRun, &exitError) && exitError.ExitCode() == 8 {
			errRun = nil
		}
	}
	if errRun != nil {
		return fmt.Errorf("update Hytale: run authenticated bootstrap: %w", errRun)
	}

	errApply := applyHytaleStagedUpdate(directory, bootstrapJARPath)
	if errApply != nil {
		return errApply
	}
	return nil
}

func prepareHytaleBootstrapConfig(directory string) (errResult error) {
	root, errRoot := os.OpenRoot(directory)
	if errRoot != nil {
		return fmt.Errorf("update Hytale: open server directory: %w", errRoot)
	}
	defer func() {
		errClose := root.Close()
		errResult = errors.Join(errResult, wrapError("update Hytale: close server directory", errClose))
	}()

	managedConfig, errManagedConfig := root.ReadFile("Server/config.json")
	if errors.Is(errManagedConfig, os.ErrNotExist) {
		return nil
	}
	if errManagedConfig != nil {
		return fmt.Errorf("update Hytale: read managed config: %w", errManagedConfig)
	}
	errWriteConfig := root.WriteFile("config.json", managedConfig, 0o600)
	if errWriteConfig != nil {
		return fmt.Errorf("update Hytale: prepare bootstrap config: %w", errWriteConfig)
	}
	return nil
}

func (h *Hytale) baseURL() string {
	baseURL := strings.TrimRight(strings.TrimSpace(h.mavenBaseURL), "/")
	if baseURL == "" {
		return hytaleMavenBaseURL
	}
	return baseURL
}

func (h *Hytale) client() *http.Client {
	if h.httpClient != nil {
		return h.httpClient
	}
	return &http.Client{Timeout: hytaleDownloadTimeout}
}

func (h *Hytale) javaExecutable() string {
	javaCommand := strings.TrimSpace(h.javaCommand)
	if javaCommand != "" {
		return javaCommand
	}
	return "java"
}

func validHytaleMavenVersion(version string) bool {
	if version == "" || len(version) > 128 {
		return false
	}
	for _, character := range version {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func validateHytaleJAR(path string) error {
	file, errOpen := os.Open(path)
	if errOpen != nil {
		return fmt.Errorf("install Hytale: open downloaded JAR: %w", errOpen)
	}
	header := make([]byte, 4)
	_, errRead := io.ReadFull(file, header)
	errClose := file.Close()
	if errRead != nil || errClose != nil {
		return errors.Join(
			wrapError("install Hytale: read downloaded JAR header", errRead),
			wrapError("install Hytale: close downloaded JAR", errClose),
		)
	}
	if string(header) != "PK\x03\x04" {
		return errors.New("install Hytale: downloaded server JAR is not a ZIP archive")
	}
	return nil
}

func validateHytaleUpdateEnvironment(environment map[string]string) error {
	if strings.TrimSpace(environment[hytaleSessionTokenEnv]) == "" {
		return errors.New("update Hytale: session token is missing")
	}
	if strings.TrimSpace(environment[hytaleIdentityTokenEnv]) == "" {
		return errors.New("update Hytale: identity token is missing")
	}
	return nil
}

func hytaleBootstrapEnvironment(hostEnvironment []string, environment map[string]string) []string {
	credentials := map[string]string{
		hytaleSessionTokenEnv:  environment[hytaleSessionTokenEnv],
		hytaleIdentityTokenEnv: environment[hytaleIdentityTokenEnv],
	}
	return processenv.Append(processenv.Build(runtime.GOOS, hostEnvironment), credentials)
}

func validateHytalePayload(directory string) error {
	for _, relativePath := range []string{hytaleServerJARPath, hytaleAssetsPath} {
		fullPath := filepath.Join(directory, filepath.FromSlash(relativePath))
		info, errStat := os.Stat(fullPath)
		if errStat != nil {
			return fmt.Errorf("update Hytale: required payload %q is unavailable: %w", relativePath, errStat)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("update Hytale: required payload %q is not a non-empty regular file", relativePath)
		}
	}
	return nil
}

func applyHytaleStagedUpdate(directory string, bootstrapJARPaths ...string) (errResult error) {
	stagingDirectory := filepath.Join(directory, "updater", "staging")
	stagedServerJAR := filepath.Join(stagingDirectory, "Server", hytaleSeedJARName)
	stagedJARInfo, errStagedJAR := os.Lstat(stagedServerJAR)
	if errors.Is(errStagedJAR, os.ErrNotExist) {
		return nil
	}
	if errStagedJAR != nil {
		return fmt.Errorf("update Hytale: inspect staged server JAR: %w", errStagedJAR)
	}
	if !stagedJARInfo.Mode().IsRegular() || stagedJARInfo.Size() == 0 {
		return errors.New("update Hytale: staged server JAR is not a non-empty regular file")
	}
	stagedAssets := filepath.Join(stagingDirectory, hytaleAssetsPath)
	assetsInfo, errAssets := os.Lstat(stagedAssets)
	if errAssets != nil {
		return fmt.Errorf("update Hytale: inspect staged assets: %w", errAssets)
	}
	if !assetsInfo.Mode().IsRegular() || assetsInfo.Size() == 0 {
		return errors.New("update Hytale: staged assets are not a non-empty regular file")
	}

	update, errUpdate := newStagedUpdate(directory, "hytale", stagedUpdateRetainRollback)
	if errUpdate != nil {
		return fmt.Errorf("update Hytale: prepare staged update: %w", errUpdate)
	}
	defer func() {
		errResult = errors.Join(errResult, wrapError("update Hytale: clean staged update", update.CleanupTransient()))
	}()
	errLauncher := writeHytaleLauncher(update.PayloadDirectory())
	if errLauncher != nil {
		return errLauncher
	}
	if len(bootstrapJARPaths) > 1 {
		return errors.New("update Hytale: multiple bootstrap JAR candidates were provided")
	}
	if len(bootstrapJARPaths) == 1 {
		bootstrapJARPath := bootstrapJARPaths[0]
		errBootstrapJAR := validateHytaleJAR(bootstrapJARPath)
		if errBootstrapJAR != nil {
			return errBootstrapJAR
		}
		errCopyBootstrap := copyHytaleFile(
			bootstrapJARPath,
			filepath.Join(update.PayloadDirectory(), hytaleSeedJARName),
			0o640,
		)
		if errCopyBootstrap != nil {
			return errCopyBootstrap
		}
	}

	for _, relativePath := range []string{hytaleServerJARPath, hytaleAssetsPath, "start.sh", "start.bat"} {
		sourceRelativePath := relativePath
		if relativePath == hytaleServerJARPath {
			sourceRelativePath = filepath.ToSlash(filepath.Join("Server", hytaleSeedJARName))
		}
		source := filepath.Join(stagingDirectory, filepath.FromSlash(sourceRelativePath))
		info, errSource := os.Lstat(source)
		if errors.Is(errSource, os.ErrNotExist) && relativePath != hytaleServerJARPath && relativePath != hytaleAssetsPath {
			continue
		}
		if errSource != nil {
			return fmt.Errorf("update Hytale: inspect staged %s: %w", relativePath, errSource)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("update Hytale: staged %s is not a non-empty regular file", relativePath)
		}
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0o640
		}
		if relativePath == "start.sh" {
			mode |= 0o110
		}
		target := filepath.Join(update.PayloadDirectory(), filepath.FromSlash(relativePath))
		errCopy := copyHytaleFile(source, target, mode)
		if errCopy != nil {
			return errCopy
		}
	}

	stagedLicenses := filepath.Join(stagingDirectory, "Server", "Licenses")
	licenseInfo, errLicenses := os.Lstat(stagedLicenses)
	if errLicenses == nil {
		if !licenseInfo.IsDir() || licenseInfo.Mode()&os.ModeSymlink != 0 {
			return errors.New("update Hytale: staged licenses are not a directory")
		}
		errCopyLicenses := copyHytaleTree(stagedLicenses, filepath.Join(update.PayloadDirectory(), "Server", "Licenses"))
		if errCopyLicenses != nil {
			return errCopyLicenses
		}
	} else if !errors.Is(errLicenses, os.ErrNotExist) {
		return fmt.Errorf("update Hytale: inspect staged licenses: %w", errLicenses)
	}

	errValidate := validateHytalePayload(update.PayloadDirectory())
	if errValidate != nil {
		return errValidate
	}
	errApply := update.Apply(nil, []string{"Server/Licenses"})
	if errApply != nil {
		return fmt.Errorf("update Hytale: commit staged update: %w", errApply)
	}
	errRemoveStaging := removeStagedUpdateTree(directory, stagingDirectory)
	if errRemoveStaging != nil {
		return fmt.Errorf("update Hytale: remove applied staging directory: %w", errRemoveStaging)
	}
	return nil
}

func copyHytaleTree(source string, target string) error {
	errWalkTree := filepath.WalkDir(source, func(path string, entry os.DirEntry, errWalk error) error {
		if errWalk != nil {
			return fmt.Errorf("update Hytale: walk staged licenses: %w", errWalk)
		}
		relativePath, errRelative := filepath.Rel(source, path)
		if errRelative != nil {
			return fmt.Errorf("update Hytale: resolve staged license path: %w", errRelative)
		}
		destination := filepath.Join(target, relativePath)
		if entry.IsDir() {
			errMkdir := os.MkdirAll(destination, 0o750)
			if errMkdir != nil {
				return fmt.Errorf("update Hytale: create license directory: %w", errMkdir)
			}
			return nil
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			return fmt.Errorf("update Hytale: inspect staged license: %w", errInfo)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("update Hytale: staged license %q is not a regular file", relativePath)
		}
		return copyHytaleFile(path, destination, info.Mode().Perm())
	})
	if errWalkTree != nil {
		return fmt.Errorf("update Hytale: copy staged license tree: %w", errWalkTree)
	}
	return nil
}

func copyHytaleFile(source string, target string, mode os.FileMode) error {
	input, errOpenInput := os.Open(source)
	if errOpenInput != nil {
		return fmt.Errorf("update Hytale: open %q: %w", source, errOpenInput)
	}
	errMkdir := os.MkdirAll(filepath.Dir(target), 0o750)
	if errMkdir != nil {
		errClose := input.Close()
		return errors.Join(
			fmt.Errorf("update Hytale: create target directory: %w", errMkdir),
			wrapError("update Hytale: close source file", errClose),
		)
	}
	output, errOpenOutput := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if errOpenOutput != nil {
		errClose := input.Close()
		return errors.Join(
			fmt.Errorf("update Hytale: create %q: %w", target, errOpenOutput),
			wrapError("update Hytale: close source file", errClose),
		)
	}
	_, errCopy := io.Copy(output, input)
	errSync := output.Sync()
	errOutputClose := output.Close()
	errInputClose := input.Close()
	if errCopy != nil || errSync != nil || errOutputClose != nil || errInputClose != nil {
		return errors.Join(
			wrapError("update Hytale: copy file", errCopy),
			wrapError("update Hytale: sync copied file", errSync),
			wrapError("update Hytale: close copied file", errOutputClose),
			wrapError("update Hytale: close source file", errInputClose),
		)
	}
	return nil
}

func writeHytaleLauncher(directory string) error {
	temporary, errCreate := os.CreateTemp(directory, ".xylona-hytale-launcher-*.java")
	if errCreate != nil {
		return fmt.Errorf("install Hytale: create temporary launcher: %w", errCreate)
	}
	temporaryPath := temporary.Name()
	_, errWrite := io.WriteString(temporary, hytaleLauncherSource)
	errSync := temporary.Sync()
	errClose := temporary.Close()
	if errWrite != nil || errSync != nil || errClose != nil {
		errRemove := os.Remove(temporaryPath)
		return errors.Join(
			wrapError("install Hytale: write launcher", errWrite),
			wrapError("install Hytale: sync launcher", errSync),
			wrapError("install Hytale: close launcher", errClose),
			wrapError("install Hytale: remove incomplete launcher", errRemove),
		)
	}
	targetPath := filepath.Join(directory, hytaleLauncherFileName)
	errRename := replaceHytaleFile(temporaryPath, targetPath)
	if errRename != nil {
		errRemove := os.Remove(temporaryPath)
		return errors.Join(
			fmt.Errorf("install Hytale: replace launcher: %w", errRename),
			wrapError("install Hytale: remove temporary launcher", errRemove),
		)
	}
	return nil
}

func replaceHytaleFile(temporaryPath string, targetPath string) error {
	errRename := os.Rename(temporaryPath, targetPath)
	if errRename == nil {
		return nil
	}
	if runtime.GOOS != "windows" {
		return fmt.Errorf("rename Hytale file: %w", errRename)
	}

	_, errTarget := os.Stat(targetPath)
	if errTarget != nil {
		return fmt.Errorf("rename Hytale file: %w", errRename)
	}
	errRemove := os.Remove(targetPath)
	if errRemove != nil {
		return errors.Join(errRename, fmt.Errorf("remove existing Hytale file: %w", errRemove))
	}
	errRenameAgain := os.Rename(temporaryPath, targetPath)
	if errRenameAgain != nil {
		return errors.Join(errRename, fmt.Errorf("replace existing Hytale file: %w", errRenameAgain))
	}
	return nil
}

const hytaleLauncherSource = `import java.io.IOException;
import java.lang.management.ManagementFactory;
import java.nio.channels.FileChannel;
import java.nio.charset.StandardCharsets;
import java.nio.file.AtomicMoveNotSupportedException;
import java.nio.file.DirectoryNotEmptyException;
import java.nio.file.FileVisitResult;
import java.nio.file.Files;
import java.nio.file.LinkOption;
import java.nio.file.Path;
import java.nio.file.SimpleFileVisitor;
import java.nio.file.StandardOpenOption;
import java.nio.file.attribute.BasicFileAttributes;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

public final class XylonaHytaleLauncher {
    private static final int UPDATE_RESTART_EXIT_CODE = 8;
    private static final int UPDATE_MANIFEST_VERSION = 1;
    private static final String UPDATE_GAME_ID = "hytale";
    private static final String UPDATE_BACKUP_DIRECTORY = ".update-backup";
    private static final String INTERNAL_UPDATE_DIRECTORY = ".internal-update";
    private static final String UPDATE_MANIFEST_NAME = "manifest.json";
    private static final String UPDATE_COMMITTED_NAME = "committed";
    private static final String UPDATE_FILES_DIRECTORY = "files";

    private static final class UpdateEntry {
        private final String relativePath;
        private final Path staged;
        private final Path live;
        private final Path backup;
        private final boolean existed;
        private final boolean directory;
        private boolean backupMoved;
        private boolean stagedMoved;

        private UpdateEntry(
                String relativePath,
                Path staged,
                Path live,
                Path backup,
                boolean existed,
                boolean directory
        ) {
            this.relativePath = relativePath;
            this.staged = staged;
            this.live = live;
            this.backup = backup;
            this.existed = existed;
            this.directory = directory;
        }
    }

    public static void main(String[] serverArguments) throws Exception {
        Path root = Path.of("").toAbsolutePath().normalize();
        Path serverDirectory = root.resolve("Server");
        Files.createDirectories(serverDirectory);

        String javaExecutable = ProcessHandle.current().info().command().orElseGet(() -> {
            String executable = System.getProperty("os.name", "").toLowerCase().contains("win") ? "java.exe" : "java";
            return Path.of(System.getProperty("java.home"), "bin", executable).toString();
        });
        List<String> jvmArguments = new ArrayList<>(ManagementFactory.getRuntimeMXBean().getInputArguments());

        Path fullServerJar = serverDirectory.resolve("HytaleServer.jar");
        if (!Files.isRegularFile(fullServerJar)) {
            runBootstrap(root, serverDirectory, javaExecutable, jvmArguments);
        }
        applyStagedUpdate(root, serverDirectory);
        if (!Files.isRegularFile(fullServerJar) || !Files.isRegularFile(root.resolve("Assets.zip"))) {
            throw new IOException("Hytale bootstrap completed without a full Server/HytaleServer.jar and Assets.zip payload");
        }
        Files.deleteIfExists(root.resolve("HytaleServer.jar"));

        while (true) {
            applyStagedUpdate(root, serverDirectory);
            List<String> command = new ArrayList<>();
            command.add(javaExecutable);
            command.addAll(jvmArguments);
            if (Files.isRegularFile(serverDirectory.resolve("HytaleServer.aot"))) {
                command.add("-XX:AOTCache=HytaleServer.aot");
            }
            command.add("-jar");
            command.add("HytaleServer.jar");
            command.add("--assets");
            command.add("../Assets.zip");
            command.addAll(Arrays.asList(serverArguments));

            Process server = new ProcessBuilder(command)
                    .directory(serverDirectory.toFile())
                    .inheritIO()
                    .start();
            Thread.sleep(10_000L);
            if (server.isAlive()) {
                try {
                    cleanupCommittedUpdate(root);
                } catch (IOException cleanupError) {
                    System.err.println("[Xylona] Unable to clean committed Hytale update recovery data: "
                            + cleanupError.getMessage());
                }
            }
            int exitCode = server.waitFor();
            if (exitCode != UPDATE_RESTART_EXIT_CODE) {
                System.exit(exitCode);
            }
            cleanupCommittedUpdate(root);
            System.out.println("[Xylona] Applying staged Hytale update before restart...");
        }
    }

    private static void runBootstrap(
            Path root,
            Path serverDirectory,
            String javaExecutable,
            List<String> jvmArguments
    ) throws Exception {
        Path seedJar = root.resolve("HytaleServer.jar");
        if (!Files.isRegularFile(seedJar)) {
            throw new IOException("Hytale bootstrap JAR is missing; run the Xylona update action to repair it");
        }

        Path managedConfig = serverDirectory.resolve("config.json");
        if (Files.isRegularFile(managedConfig)) {
            Files.copy(managedConfig, root.resolve("config.json"), java.nio.file.StandardCopyOption.REPLACE_EXISTING);
        }

        List<String> command = new ArrayList<>();
        command.add(javaExecutable);
        command.addAll(jvmArguments);
        command.add("-jar");
        command.add("HytaleServer.jar");
        command.add("--bootstrap");
        command.add("--boot-command");
        command.add("update download --force");

        Process bootstrap = new ProcessBuilder(command)
                .directory(root.toFile())
                .inheritIO()
                .start();
        int exitCode = bootstrap.waitFor();
        if (exitCode != 0 && exitCode != UPDATE_RESTART_EXIT_CODE) {
            throw new IOException("Hytale bootstrap exited with code " + exitCode);
        }
        applyStagedUpdate(root, serverDirectory);
    }

    private static void applyStagedUpdate(Path root, Path serverDirectory) throws IOException {
        Path updateBackup = root.resolve(UPDATE_BACKUP_DIRECTORY);
        Path transaction = updateBackup.resolve(INTERNAL_UPDATE_DIRECTORY);
        if (existsNoFollow(transaction)) {
            Path committed = transaction.resolve(UPDATE_COMMITTED_NAME);
            if (Files.isRegularFile(committed, LinkOption.NOFOLLOW_LINKS)) {
                return;
            }
            throw new IOException("An unresolved Hytale update transaction requires recovery before another update can start");
        }

        Path staging = root.resolve("updater").resolve("staging");
        Path stagedServerJar = staging.resolve("Server").resolve("HytaleServer.jar");
        if (!existsNoFollow(stagedServerJar)) {
            return;
        }

        validateStagingTree(staging);
        Path stagedAssets = staging.resolve("Assets.zip");
        validateRequiredFile(stagedServerJar, "Server/HytaleServer.jar");
        validateRequiredFile(stagedAssets, "Assets.zip");

        List<UpdateEntry> entries = new ArrayList<>();
        addFileEntry(entries, root, transaction, staging, "Assets.zip", true);
        addFileEntry(entries, root, transaction, staging, "Server/HytaleServer.jar", true);
        addFileEntry(entries, root, transaction, staging, "start.bat", false);
        addFileEntry(entries, root, transaction, staging, "start.sh", false);
        Path stagedLicenses = staging.resolve("Server").resolve("Licenses");
        if (existsNoFollow(stagedLicenses)) {
            validateDirectoryTree(stagedLicenses, "Server/Licenses");
            addDirectoryEntry(entries, root, transaction, staging, "Server/Licenses");
        }

        entries.sort((left, right) -> left.relativePath.compareTo(right.relativePath));
        preflightLiveEntries(root, entries);
        createTransaction(transaction, entries);

        try {
            applyEntries(entries);
            writeDurableFile(transaction.resolve(UPDATE_COMMITTED_NAME), "committed\n");
        } catch (IOException | RuntimeException applyError) {
            IOException rollbackError = rollbackEntries(transaction, entries);
            if (rollbackError != null) {
                applyError.addSuppressed(rollbackError);
            }
            throw applyError;
        }

        deleteTree(staging);
    }

    private static void validateStagingTree(Path staging) throws IOException {
        Files.walkFileTree(staging, new SimpleFileVisitor<>() {
            @Override
            public FileVisitResult preVisitDirectory(Path directory, BasicFileAttributes attributes) throws IOException {
                if (!attributes.isDirectory() || attributes.isSymbolicLink()) {
                    throw new IOException("Unsupported non-directory entry in staged Hytale update");
                }
                return FileVisitResult.CONTINUE;
            }

            @Override
            public FileVisitResult visitFile(Path file, BasicFileAttributes attributes) throws IOException {
                if (!attributes.isRegularFile() || attributes.isSymbolicLink()) {
                    throw new IOException("Unsupported non-regular entry in staged Hytale update");
                }
                return FileVisitResult.CONTINUE;
            }
        });
    }

    private static void validateRequiredFile(Path candidate, String relativePath) throws IOException {
        BasicFileAttributes attributes = Files.readAttributes(
                candidate,
                BasicFileAttributes.class,
                LinkOption.NOFOLLOW_LINKS
        );
        if (!attributes.isRegularFile() || attributes.isSymbolicLink() || attributes.size() <= 0L) {
            throw new IOException("Staged Hytale update requires a non-empty regular " + relativePath);
        }
    }

    private static void validateDirectoryTree(Path directory, String relativePath) throws IOException {
        BasicFileAttributes rootAttributes = Files.readAttributes(
                directory,
                BasicFileAttributes.class,
                LinkOption.NOFOLLOW_LINKS
        );
        if (!rootAttributes.isDirectory() || rootAttributes.isSymbolicLink()) {
            throw new IOException("Staged Hytale update requires a regular directory at " + relativePath);
        }
        validateStagingTree(directory);
    }

    private static void addFileEntry(
            List<UpdateEntry> entries,
            Path root,
            Path transaction,
            Path staging,
            String relativePath,
            boolean required
    ) throws IOException {
        Path staged = staging.resolve(relativePath).normalize();
        if (!existsNoFollow(staged)) {
            if (required) {
                throw new IOException("Staged Hytale update is missing " + relativePath);
            }
            return;
        }
        BasicFileAttributes attributes = Files.readAttributes(
                staged,
                BasicFileAttributes.class,
                LinkOption.NOFOLLOW_LINKS
        );
        if (!attributes.isRegularFile() || attributes.isSymbolicLink()) {
            throw new IOException("Staged Hytale update contains a non-regular " + relativePath);
        }
        entries.add(newEntry(root, transaction, staged, relativePath, false));
    }

    private static void addDirectoryEntry(
            List<UpdateEntry> entries,
            Path root,
            Path transaction,
            Path staging,
            String relativePath
    ) throws IOException {
        Path staged = staging.resolve(relativePath).normalize();
        entries.add(newEntry(root, transaction, staged, relativePath, true));
    }

    private static UpdateEntry newEntry(
            Path root,
            Path transaction,
            Path staged,
            String relativePath,
            boolean directory
    ) throws IOException {
        Path live = root.resolve(relativePath).normalize();
        Path backup = transaction.resolve(UPDATE_FILES_DIRECTORY).resolve(relativePath).normalize();
        requireWithin(root, live, "live update path");
        requireWithin(transaction, backup, "rollback update path");
        boolean existed = existsNoFollow(live);
        return new UpdateEntry(relativePath, staged, live, backup, existed, directory);
    }

    private static void preflightLiveEntries(Path root, List<UpdateEntry> entries) throws IOException {
        for (UpdateEntry entry : entries) {
            validateSafeParents(root, entry.live.getParent());
            if (!entry.existed) {
                continue;
            }
            BasicFileAttributes attributes = Files.readAttributes(
                    entry.live,
                    BasicFileAttributes.class,
                    LinkOption.NOFOLLOW_LINKS
            );
            if (attributes.isSymbolicLink()) {
                throw new IOException("Live Hytale update target is a symbolic link: " + entry.relativePath);
            }
            if (entry.directory) {
                if (!attributes.isDirectory()) {
                    throw new IOException("Live Hytale update target is not a directory: " + entry.relativePath);
                }
                validateDirectoryTree(entry.live, entry.relativePath);
            } else if (!attributes.isRegularFile()) {
                throw new IOException("Live Hytale update target is not a regular file: " + entry.relativePath);
            }
        }
    }

    private static void validateSafeParents(Path root, Path parent) throws IOException {
        if (parent == null) {
            throw new IOException("Hytale update target has no parent directory");
        }
        requireWithin(root, parent, "update target parent");
        Path relativeParent = root.relativize(parent);
        Path current = root;
        for (Path part : relativeParent) {
            current = current.resolve(part);
            if (!existsNoFollow(current)) {
                continue;
            }
            BasicFileAttributes attributes = Files.readAttributes(
                    current,
                    BasicFileAttributes.class,
                    LinkOption.NOFOLLOW_LINKS
            );
            if (!attributes.isDirectory() || attributes.isSymbolicLink()) {
                throw new IOException("Unsafe Hytale update target parent");
            }
        }
    }

    private static void requireWithin(Path root, Path candidate, String description) throws IOException {
        Path normalizedRoot = root.toAbsolutePath().normalize();
        Path normalizedCandidate = candidate.toAbsolutePath().normalize();
        if (normalizedCandidate.equals(normalizedRoot) || !normalizedCandidate.startsWith(normalizedRoot)) {
            throw new IOException("Unsafe " + description);
        }
    }

    private static void createTransaction(Path transaction, List<UpdateEntry> entries) throws IOException {
        if (existsNoFollow(transaction)) {
            throw new IOException("An unresolved Hytale update transaction requires recovery before another update can start");
        }
        try {
            Files.createDirectories(transaction.resolve(UPDATE_FILES_DIRECTORY));
            writeDurableFile(transaction.resolve(UPDATE_MANIFEST_NAME), serializeManifest(entries));
        } catch (IOException createError) {
            try {
                deleteTree(transaction);
            } catch (IOException cleanupError) {
                createError.addSuppressed(cleanupError);
            }
            throw createError;
        }
    }

    private static String serializeManifest(List<UpdateEntry> entries) {
        StringBuilder manifest = new StringBuilder();
        manifest.append("{\"version\":").append(UPDATE_MANIFEST_VERSION)
                .append(",\"game_id\":\"").append(UPDATE_GAME_ID)
                .append("\",\"entries\":[");
        for (int index = 0; index < entries.size(); index++) {
            if (index > 0) {
                manifest.append(',');
            }
            UpdateEntry entry = entries.get(index);
            manifest.append("{\"path\":\"")
                    .append(escapeJson(entry.relativePath))
                    .append("\",\"existed\":")
                    .append(entry.existed);
            if (entry.directory) {
                manifest.append(",\"directory\":true");
            }
            manifest.append('}');
        }
        return manifest.append("]}\n").toString();
    }

    private static String escapeJson(String value) {
        StringBuilder escaped = new StringBuilder(value.length());
        for (int index = 0; index < value.length(); index++) {
            char character = value.charAt(index);
            switch (character) {
                case '\"' -> escaped.append("\\\"");
                case '\\' -> escaped.append("\\\\");
                case '\b' -> escaped.append("\\b");
                case '\f' -> escaped.append("\\f");
                case '\n' -> escaped.append("\\n");
                case '\r' -> escaped.append("\\r");
                case '\t' -> escaped.append("\\t");
                default -> {
                    if (character < 0x20) {
                        escaped.append(String.format("\\u%04x", (int) character));
                    } else {
                        escaped.append(character);
                    }
                }
            }
        }
        return escaped.toString();
    }

    private static void applyEntries(List<UpdateEntry> entries) throws IOException {
        for (UpdateEntry entry : entries) {
            Files.createDirectories(entry.live.getParent());
            Files.createDirectories(entry.backup.getParent());
            if (entry.existed) {
                movePath(entry.live, entry.backup);
                entry.backupMoved = true;
            }
            movePath(entry.staged, entry.live);
            entry.stagedMoved = true;
        }
    }

    private static IOException rollbackEntries(Path transaction, List<UpdateEntry> entries) {
        IOException rollbackFailure = null;
        for (int index = entries.size() - 1; index >= 0; index--) {
            UpdateEntry entry = entries.get(index);
            try {
                if (entry.stagedMoved) {
                    Files.createDirectories(entry.staged.getParent());
                    movePath(entry.live, entry.staged);
                    entry.stagedMoved = false;
                }
                if (entry.backupMoved) {
                    Files.createDirectories(entry.live.getParent());
                    movePath(entry.backup, entry.live);
                    entry.backupMoved = false;
                }
            } catch (IOException entryError) {
                rollbackFailure = appendFailure(rollbackFailure, entryError);
            }
        }
        if (rollbackFailure == null) {
            try {
                deleteTree(transaction);
            } catch (IOException cleanupError) {
                rollbackFailure = cleanupError;
            }
        }
        if (rollbackFailure != null) {
            return new IOException(
                    "Hytale update rollback was incomplete; recovery data was retained",
                    rollbackFailure
            );
        }
        return null;
    }

    private static IOException appendFailure(IOException current, IOException next) {
        if (current == null) {
            return next;
        }
        current.addSuppressed(next);
        return current;
    }

    private static void movePath(Path source, Path target) throws IOException {
        try {
            Files.move(source, target, java.nio.file.StandardCopyOption.ATOMIC_MOVE);
        } catch (AtomicMoveNotSupportedException unsupported) {
            Files.move(source, target);
        }
    }

    private static void writeDurableFile(Path target, String contents) throws IOException {
        Files.createDirectories(target.getParent());
        Path temporary = target.resolveSibling(target.getFileName() + ".tmp");
        try {
            try (FileChannel channel = FileChannel.open(
                    temporary,
                    StandardOpenOption.CREATE_NEW,
                StandardOpenOption.WRITE
            )) {
                byte[] encoded = contents.getBytes(StandardCharsets.UTF_8);
                java.nio.ByteBuffer buffer = java.nio.ByteBuffer.wrap(encoded);
                while (buffer.hasRemaining()) {
                    channel.write(buffer);
                }
                channel.force(true);
            }
            try {
                Files.move(temporary, target, java.nio.file.StandardCopyOption.ATOMIC_MOVE);
            } catch (AtomicMoveNotSupportedException unsupported) {
                Files.move(temporary, target);
            }
        } catch (IOException writeError) {
            try {
                Files.deleteIfExists(temporary);
            } catch (IOException cleanupError) {
                writeError.addSuppressed(cleanupError);
            }
            throw writeError;
        }
    }

    private static boolean existsNoFollow(Path path) {
        return Files.exists(path, LinkOption.NOFOLLOW_LINKS);
    }

    private static void cleanupCommittedUpdate(Path root) throws IOException {
        Path updateBackup = root.resolve(UPDATE_BACKUP_DIRECTORY);
        Path transaction = updateBackup.resolve(INTERNAL_UPDATE_DIRECTORY);
        Path committed = transaction.resolve(UPDATE_COMMITTED_NAME);
        if (!Files.isRegularFile(committed, LinkOption.NOFOLLOW_LINKS)) {
            return;
        }
        deleteTree(transaction);
        try {
            Files.delete(updateBackup);
        } catch (DirectoryNotEmptyException ignored) {
            // The controller may retain other update recovery data here.
        }
    }

    private static void deleteTree(Path root) throws IOException {
        if (!Files.exists(root)) {
            return;
        }
        Files.walkFileTree(root, new SimpleFileVisitor<>() {
            @Override
            public FileVisitResult visitFile(Path file, BasicFileAttributes attributes) throws IOException {
                Files.delete(file);
                return FileVisitResult.CONTINUE;
            }

            @Override
            public FileVisitResult postVisitDirectory(Path directory, IOException error) throws IOException {
                if (error != null) {
                    throw error;
                }
                Files.delete(directory);
                return FileVisitResult.CONTINUE;
            }
        });
    }
}
`

var _ gameintegrations.Game = (*Hytale)(nil)
var _ gameintegrations.EnvironmentUpdater = (*Hytale)(nil)
