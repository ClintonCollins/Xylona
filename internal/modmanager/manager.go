// Package modmanager coordinates mod provider downloads, installed-mod state,
// and filesystem operations for game servers.
package modmanager

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"
	"golang.org/x/sync/errgroup"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/sql/models"
)

var (
	// ErrProviderNotFound is returned when the requested mod provider does not exist.
	ErrProviderNotFound = errors.New("modmanager: provider not found")
	// ErrModNotFound is returned when the requested installed mod does not exist.
	ErrModNotFound = errors.New("modmanager: installed mod not found")
	// ErrNoFilesDownloaded is returned when a provider download produces no files.
	ErrNoFilesDownloaded = errors.New("modmanager: provider returned no downloaded files")
	// ErrRemoteDownloadURLMissing is returned when a remote install/update cannot resolve a node-downloadable artifact URL.
	ErrRemoteDownloadURLMissing = errors.New("modmanager: provider did not expose a remote download URL")
)

// ModManager coordinates between mod providers, the database, and the filesystem.
type ModManager struct {
	db *db.Connection
}

type fileMove struct {
	source string
	target string
}

// FileClient is the subset of nodeclient.NodeClient needed for mod artifact
// work. Keeping this narrow lets tests use small fakes while the RPC/actions
// layers pass their existing node clients.
type FileClient interface {
	CreateFileOrDirectory(ctx context.Context, directory, relativePath, content string, isDirectory bool, policy node.ProtectionPolicy) error
	DeleteFiles(ctx context.Context, directory string, files []string, policy node.ProtectionPolicy) ([]string, error)
	DownloadFileFromURL(ctx context.Context, directory, rawURL, destinationDirectoryPath string, integrity node.DownloadIntegrity, policy node.ProtectionPolicy) (node.DownloadFileResult, error)
	MoveFiles(ctx context.Context, directory string, files []string, destination string, policy node.ProtectionPolicy) ([]string, error)
	RenameFile(ctx context.Context, directory, oldRelativePath, newRelativePath string, policy node.ProtectionPolicy) (string, error)
}

type remoteDownloadPlan struct {
	details       *modproviders.ModDetails
	version       modproviders.ModVersion
	versionString string
}

// New creates a new ModManager.
func New(database *db.Connection) *ModManager {
	return &ModManager{db: database}
}

func wrapProviderDownloadError(err error) error {
	if errors.Is(err, modproviders.ErrMissingIntegrityMetadata) {
		return fmt.Errorf("modmanager: provider integrity metadata missing: %w", err)
	}
	if errors.Is(err, modproviders.ErrIntegrityMismatch) {
		return fmt.Errorf("modmanager: provider integrity verification failed: %w", err)
	}
	return err
}

func createDownloadStagingDir(targetDir string) (string, error) {
	errMkdir := os.MkdirAll(targetDir, 0o750)
	if errMkdir != nil {
		return "", fmt.Errorf("modmanager: create install directory: %w", errMkdir)
	}

	stagingDir, errTempDir := os.MkdirTemp(targetDir, ".xylona-download-*")
	if errTempDir != nil {
		return "", fmt.Errorf("modmanager: create staging directory: %w", errTempDir)
	}

	return stagingDir, nil
}

func logRemoveAllError(removePath string, purpose string) {
	errRemoveAll := os.RemoveAll(removePath)
	if errRemoveAll != nil {
		log.Warn().Err(errRemoveAll).Str("path", removePath).Msg(purpose)
	}
}

func logRollbackError(err error, msg string) {
	if err == nil || errors.Is(err, sql.ErrTxDone) {
		return
	}

	log.Warn().Err(err).Msg(msg)
}

func cleanupPromotedFiles(moves []fileMove) error {
	var cleanupErr error
	for _, move := range slices.Backward(moves) {
		errRemove := os.Remove(move.target)
		if errRemove != nil && !os.IsNotExist(errRemove) && cleanupErr == nil {
			cleanupErr = fmt.Errorf("remove promoted file %s: %w", move.target, errRemove)
		}
	}

	return cleanupErr
}

func promoteDownloadedFiles(stagingDir string, targetDir string, downloaded []modproviders.DownloadedFile) ([]fileMove, error) {
	promotedMoves := make([]fileMove, 0, len(downloaded))
	for _, downloadedFile := range downloaded {
		sourcePath := filepath.Join(stagingDir, downloadedFile.Path)
		targetPath := filepath.Join(targetDir, downloadedFile.Path)

		errMkdir := os.MkdirAll(filepath.Dir(targetPath), 0o750)
		if errMkdir != nil {
			errCleanup := cleanupPromotedFiles(promotedMoves)
			if errCleanup != nil {
				return nil, fmt.Errorf("modmanager: create destination directory: %w; cleanup promoted files: %s", errMkdir, errCleanup.Error())
			}
			return nil, fmt.Errorf("modmanager: create destination directory: %w", errMkdir)
		}

		errRename := os.Rename(sourcePath, targetPath)
		if errRename != nil {
			errCleanup := cleanupPromotedFiles(promotedMoves)
			if errCleanup != nil {
				return nil, fmt.Errorf("modmanager: promote staged file %s: %w; cleanup promoted files: %s", targetPath, errRename, errCleanup.Error())
			}
			return nil, fmt.Errorf("modmanager: promote staged file %s: %w", targetPath, errRename)
		}

		promotedMoves = append(promotedMoves, fileMove{
			source: sourcePath,
			target: targetPath,
		})
	}

	return promotedMoves, nil
}

func relativeInstalledModPath(installSubdir string, filePath string) (string, error) {
	cleanInstallSubdir := filepath.Clean(installSubdir)
	if cleanInstallSubdir == "." || cleanInstallSubdir == "" {
		return filePath, nil
	}

	relativePath, errRelative := filepath.Rel(cleanInstallSubdir, filePath)
	if errRelative != nil {
		return "", fmt.Errorf("modmanager: determine relative installed mod path: %w", errRelative)
	}

	return relativePath, nil
}

func cleanRemotePath(value string) string {
	slashPath := strings.ReplaceAll(strings.TrimSpace(value), `\`, "/")
	slashPath = strings.TrimPrefix(slashPath, "/")
	cleaned := path.Clean(slashPath)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func joinRemotePath(parts ...string) string {
	cleanedParts := make([]string, 0, len(parts))
	for _, part := range parts {
		cleaned := cleanRemotePath(part)
		if cleaned != "" {
			cleanedParts = append(cleanedParts, cleaned)
		}
	}
	if len(cleanedParts) == 0 {
		return ""
	}
	return path.Join(cleanedParts...)
}

func remoteFileName(filePath string) string {
	return path.Base(cleanRemotePath(filePath))
}

func remoteInstalledModPath(installSubdir string, filePath string) string {
	cleanInstallSubdir := cleanRemotePath(installSubdir)
	cleanFilePath := cleanRemotePath(filePath)
	if cleanInstallSubdir == "" {
		return cleanFilePath
	}

	prefix := cleanInstallSubdir + "/"
	relativePath, hasPrefix := strings.CutPrefix(cleanFilePath, prefix)
	if hasPrefix {
		return relativePath
	}
	return remoteFileName(cleanFilePath)
}

func newRemoteScratchDir(installPath string, prefix string) string {
	return joinRemotePath(installPath, prefix+uuid.NewString())
}

func logRemoteDeleteError(ctx context.Context, client FileClient, directory string, files []string, purpose string) {
	_, errDelete := client.DeleteFiles(ctx, directory, files, node.ProtectionPolicy{})
	if errDelete != nil {
		log.Warn().Err(errDelete).Strs("paths", files).Msg(purpose)
	}
}

func cleanupRemotePromotedFiles(ctx context.Context, client FileClient, directory string, moves []fileMove) error {
	var cleanupErr error
	for _, move := range slices.Backward(moves) {
		_, errDelete := client.DeleteFiles(ctx, directory, []string{move.target}, node.ProtectionPolicy{})
		if errDelete != nil && cleanupErr == nil {
			cleanupErr = fmt.Errorf("remove promoted remote file %s: %w", move.target, errDelete)
		}
	}
	return cleanupErr
}

func restoreRemoteMovedFiles(ctx context.Context, client FileClient, directory string, moves []fileMove) error {
	var restoreErr error
	for _, move := range slices.Backward(moves) {
		parent := path.Dir(move.target)
		if parent != "." && parent != "" {
			errMkdir := client.CreateFileOrDirectory(ctx, directory, parent, "", true, node.ProtectionPolicy{})
			if errMkdir != nil {
				if restoreErr == nil {
					restoreErr = fmt.Errorf("create remote restore directory: %w", errMkdir)
				}
				continue
			}
		}

		_, errRename := client.RenameFile(ctx, directory, move.source, move.target, node.ProtectionPolicy{})
		if errRename != nil && restoreErr == nil {
			restoreErr = fmt.Errorf("restore remote file %s: %w", move.target, errRename)
		}
	}
	return restoreErr
}

func moveRemoteExistingFilesToRollback(
	ctx context.Context,
	client FileClient,
	serverDir string,
	installSubdir string,
	oldFiles []*models.InstalledModFile,
	rollbackDir string,
) ([]fileMove, error) {
	rollbackMoves := make([]fileMove, 0, len(oldFiles))
	for _, installedFile := range oldFiles {
		sourcePath := cleanRemotePath(installedFile.FilePath)
		relativePath := remoteInstalledModPath(installSubdir, sourcePath)
		targetPath := joinRemotePath(rollbackDir, relativePath)
		parent := path.Dir(targetPath)
		if parent != "." && parent != "" {
			errMkdir := client.CreateFileOrDirectory(ctx, serverDir, parent, "", true, node.ProtectionPolicy{})
			if errMkdir != nil {
				errRestore := restoreRemoteMovedFiles(ctx, client, serverDir, rollbackMoves)
				if errRestore != nil {
					return nil, fmt.Errorf("modmanager: create remote rollback directory: %w; restore rollback files: %s", errMkdir, errRestore.Error())
				}
				return nil, fmt.Errorf("modmanager: create remote rollback directory: %w", errMkdir)
			}
		}

		_, errRename := client.RenameFile(ctx, serverDir, sourcePath, targetPath, node.ProtectionPolicy{})
		if errRename != nil {
			errRestore := restoreRemoteMovedFiles(ctx, client, serverDir, rollbackMoves)
			if errRestore != nil {
				return nil, fmt.Errorf("modmanager: move remote existing file to rollback %s: %w; restore rollback files: %s", sourcePath, errRename, errRestore.Error())
			}
			return nil, fmt.Errorf("modmanager: move remote existing file to rollback %s: %w", sourcePath, errRename)
		}

		rollbackMoves = append(rollbackMoves, fileMove{
			source: targetPath,
			target: sourcePath,
		})
	}
	return rollbackMoves, nil
}

func buildRemoteDownloadPlan(ctx context.Context, provider modproviders.ModProvider, sourceID string, versionID string) (*remoteDownloadPlan, error) {
	details, errDetails := provider.GetModDetails(ctx, sourceID, nil)
	if errDetails != nil {
		return nil, fmt.Errorf("modmanager: get mod details failed: %w", errDetails)
	}
	if details == nil {
		return nil, errors.New("modmanager: provider returned nil mod details")
	}

	versionString := versionID
	targetVersion := modproviders.ModVersion{
		VersionID:     versionID,
		VersionString: versionID,
	}
	for _, version := range details.Versions {
		if version.VersionID == versionID {
			targetVersion = version
			if version.VersionString != "" {
				versionString = version.VersionString
			}
			break
		}
	}
	if strings.TrimSpace(targetVersion.DownloadURL) == "" {
		return nil, fmt.Errorf("%w: %s/%s", ErrRemoteDownloadURLMissing, provider.ID(), versionID)
	}
	if !remoteDownloadIntegrity(targetVersion).HasExpectedMetadata() {
		return nil, fmt.Errorf("%w: %s/%s", modproviders.ErrMissingIntegrityMetadata, provider.ID(), versionID)
	}

	return &remoteDownloadPlan{
		details:       details,
		version:       targetVersion,
		versionString: versionString,
	}, nil
}

func remoteDownloadIntegrity(version modproviders.ModVersion) node.DownloadIntegrity {
	return node.DownloadIntegrity{
		ExpectedSize:   version.FileSize,
		ExpectedSHA256: version.FileHashSHA256,
		ExpectedSHA1:   version.FileHashSHA1,
	}
}

func verifiedRemoteDownloadHash(version modproviders.ModVersion, result node.DownloadFileResult) string {
	if strings.TrimSpace(version.FileHashSHA256) != "" {
		return strings.TrimSpace(version.FileHashSHA256)
	}
	return strings.TrimSpace(result.SHA256)
}

func verifiedRemoteDownloadSize(version modproviders.ModVersion, result node.DownloadFileResult) int64 {
	if version.FileSize > 0 {
		return version.FileSize
	}
	return result.BytesWritten
}

func primaryHashFromDownloaded(downloaded []modproviders.DownloadedFile) string {
	primaryHash := ""
	for _, f := range downloaded {
		if f.IsPrimary {
			primaryHash = f.Hash
			break
		}
	}
	if primaryHash == "" && len(downloaded) > 0 {
		primaryHash = downloaded[0].Hash
	}
	return primaryHash
}

func moveExistingFilesToRollback(serverDir string, installSubdir string, oldFiles []*models.InstalledModFile, rollbackDir string) ([]fileMove, error) {
	rollbackMoves := make([]fileMove, 0, len(oldFiles))
	for _, installedFile := range oldFiles {
		relativePath, errRelative := relativeInstalledModPath(installSubdir, installedFile.FilePath)
		if errRelative != nil {
			return nil, errRelative
		}

		sourcePath := filepath.Join(serverDir, installedFile.FilePath)
		targetPath := filepath.Join(rollbackDir, relativePath)

		errMkdir := os.MkdirAll(filepath.Dir(targetPath), 0o750)
		if errMkdir != nil {
			errRestore := restoreMovedFiles(rollbackMoves)
			if errRestore != nil {
				return nil, fmt.Errorf("modmanager: create rollback directory: %w; restore rollback files: %s", errMkdir, errRestore.Error())
			}
			return nil, fmt.Errorf("modmanager: create rollback directory: %w", errMkdir)
		}

		errRename := os.Rename(sourcePath, targetPath)
		if errRename != nil {
			if os.IsNotExist(errRename) {
				continue
			}

			errRestore := restoreMovedFiles(rollbackMoves)
			if errRestore != nil {
				return nil, fmt.Errorf("modmanager: move existing file to rollback %s: %w; restore rollback files: %s", sourcePath, errRename, errRestore.Error())
			}
			return nil, fmt.Errorf("modmanager: move existing file to rollback %s: %w", sourcePath, errRename)
		}

		rollbackMoves = append(rollbackMoves, fileMove{
			source: targetPath,
			target: sourcePath,
		})
	}

	return rollbackMoves, nil
}

func restoreMovedFiles(moves []fileMove) error {
	var restoreErr error
	for _, move := range slices.Backward(moves) {
		errMkdir := os.MkdirAll(filepath.Dir(move.target), 0o750)
		if errMkdir != nil {
			if restoreErr == nil {
				restoreErr = fmt.Errorf("create restore directory: %w", errMkdir)
			}
			continue
		}

		errRename := os.Rename(move.source, move.target)
		if errRename != nil && !os.IsNotExist(errRename) && restoreErr == nil {
			restoreErr = fmt.Errorf("restore file %s: %w", move.target, errRename)
		}
	}

	return restoreErr
}

// Install downloads a mod via the provider and creates DB records.
func (m *ModManager) Install(
	ctx context.Context,
	serverID, source, sourceID, versionID, serverDir, installPath string,
) (*models.InstalledMod, error) {
	provider, ok := modproviders.GetProvider(source)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, source)
	}

	targetDir := filepath.Join(serverDir, installPath)
	stagingDir, errStagingDir := createDownloadStagingDir(targetDir)
	if errStagingDir != nil {
		return nil, errStagingDir
	}
	defer func() {
		logRemoveAllError(stagingDir, "Failed to remove mod install staging directory")
	}()

	downloaded, errDownload := provider.Download(ctx, sourceID, versionID, stagingDir)
	if errDownload != nil {
		return nil, fmt.Errorf("modmanager: download failed: %w", wrapProviderDownloadError(errDownload))
	}
	if len(downloaded) == 0 {
		return nil, ErrNoFilesDownloaded
	}

	promotedMoves, errPromote := promoteDownloadedFiles(stagingDir, targetDir, downloaded)
	if errPromote != nil {
		return nil, errPromote
	}

	details, errDetails := provider.GetModDetails(ctx, sourceID, nil)
	if errDetails != nil {
		errCleanup := cleanupPromotedFiles(promotedMoves)
		if errCleanup != nil {
			return nil, fmt.Errorf("modmanager: get mod details failed: %w; cleanup promoted files: %s", errDetails, errCleanup.Error())
		}
		return nil, fmt.Errorf("modmanager: get mod details failed: %w", errDetails)
	}

	// Find the version string from the downloaded version.
	versionString := versionID
	for _, v := range details.Versions {
		if v.VersionID == versionID {
			versionString = v.VersionString
			break
		}
	}

	primaryHash := primaryHashFromDownloaded(downloaded)

	now := time.Now().UTC()
	modID := uuid.NewString()

	t, errTx := m.db.SQLDb.BeginTx(ctx, nil)
	if errTx != nil {
		return nil, fmt.Errorf("modmanager: begin transaction: %w", errTx)
	}
	tx := bob.NewTx(t)

	defer func() {
		logRollbackError(tx.Rollback(ctx), "Failed to rollback mod install transaction")
	}()

	modSetter := &models.InstalledModSetter{
		ID:                 omit.From(modID),
		GameServerID:       omit.From(serverID),
		Source:             omit.From(source),
		SourceID:           omit.From(sourceID),
		ModName:            omit.From(details.Name),
		ModAuthor:          omit.From(details.Author),
		InstalledVersion:   omit.From(versionString),
		InstalledVersionID: omit.From(versionID),
		FileHash:           omit.From(primaryHash),
		AutoUpdate:         omit.From(int64(0)),
		Enabled:            omit.From(int64(1)),
		CreatedAt:          omit.From(now),
		UpdatedAt:          omit.From(now),
	}

	mod, errInsert := m.db.InsertInstalledMod(tx, modSetter)
	if errInsert != nil {
		return nil, fmt.Errorf("modmanager: insert mod: %w", errInsert)
	}

	for _, df := range downloaded {
		fileID := uuid.NewString()
		isPrimary := int64(0)
		if df.IsPrimary {
			isPrimary = 1
		}

		// Store the file path relative to serverDir (including the install subdirectory).
		relPath := filepath.Join(installPath, df.Path)

		fileSetter := &models.InstalledModFileSetter{
			ID:             omit.From(fileID),
			InstalledModID: omit.From(modID),
			FilePath:       omit.From(relPath),
			FileHash:       omit.From(df.Hash),
			FileSize:       omit.From(df.Size),
			IsPrimary:      omit.From(isPrimary),
		}

		_, errInsertFile := m.db.InsertInstalledModFile(tx, fileSetter)
		if errInsertFile != nil {
			return nil, fmt.Errorf("modmanager: insert mod file: %w", errInsertFile)
		}
	}

	errCommit := tx.Commit(ctx)
	if errCommit != nil {
		errCleanup := cleanupPromotedFiles(promotedMoves)
		if errCleanup != nil {
			return nil, fmt.Errorf("modmanager: commit transaction: %w; cleanup promoted files: %s", errCommit, errCleanup.Error())
		}
		return nil, fmt.Errorf("modmanager: commit transaction: %w", errCommit)
	}

	return mod, nil
}

// InstallRemote downloads a mod through a node client and creates DB records.
func (m *ModManager) InstallRemote(
	ctx context.Context,
	client FileClient,
	serverID, source, sourceID, versionID, serverDir, installPath string,
) (*models.InstalledMod, error) {
	provider, ok := modproviders.GetProvider(source)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, source)
	}

	plan, errPlan := buildRemoteDownloadPlan(ctx, provider, sourceID, versionID)
	if errPlan != nil {
		return nil, errPlan
	}

	cleanInstallPath := cleanRemotePath(installPath)
	errCreateInstall := client.CreateFileOrDirectory(ctx, serverDir, cleanInstallPath, "", true, node.ProtectionPolicy{})
	if errCreateInstall != nil {
		return nil, fmt.Errorf("modmanager: create remote install directory: %w", errCreateInstall)
	}

	stagingDir := newRemoteScratchDir(cleanInstallPath, ".xylona-download-")
	errCreateStaging := client.CreateFileOrDirectory(ctx, serverDir, stagingDir, "", true, node.ProtectionPolicy{})
	if errCreateStaging != nil {
		return nil, fmt.Errorf("modmanager: create remote staging directory: %w", errCreateStaging)
	}
	defer func() {
		logRemoteDeleteError(ctx, client, serverDir, []string{stagingDir}, "Failed to remove remote mod install staging directory")
	}()

	downloadResult, errDownload := client.DownloadFileFromURL(ctx, serverDir, plan.version.DownloadURL, stagingDir, remoteDownloadIntegrity(plan.version), node.ProtectionPolicy{})
	if errDownload != nil {
		return nil, fmt.Errorf("modmanager: remote download failed: %w", errDownload)
	}
	downloadedPath := downloadResult.RelativePath

	targetPath := joinRemotePath(cleanInstallPath, remoteFileName(downloadedPath))
	_, errPromote := client.RenameFile(ctx, serverDir, downloadedPath, targetPath, node.ProtectionPolicy{})
	if errPromote != nil {
		return nil, fmt.Errorf("modmanager: promote remote staged file %s: %w", targetPath, errPromote)
	}
	promotedMoves := []fileMove{{source: downloadedPath, target: targetPath}}

	now := time.Now().UTC()
	modID := uuid.NewString()
	downloadHash := verifiedRemoteDownloadHash(plan.version, downloadResult)
	downloadSize := verifiedRemoteDownloadSize(plan.version, downloadResult)

	t, errTx := m.db.SQLDb.BeginTx(ctx, nil)
	if errTx != nil {
		errCleanup := cleanupRemotePromotedFiles(ctx, client, serverDir, promotedMoves)
		if errCleanup != nil {
			return nil, fmt.Errorf("modmanager: begin transaction: %w; cleanup promoted files: %s", errTx, errCleanup.Error())
		}
		return nil, fmt.Errorf("modmanager: begin transaction: %w", errTx)
	}
	tx := bob.NewTx(t)

	defer func() {
		logRollbackError(tx.Rollback(ctx), "Failed to rollback remote mod install transaction")
	}()

	modSetter := &models.InstalledModSetter{
		ID:                 omit.From(modID),
		GameServerID:       omit.From(serverID),
		Source:             omit.From(source),
		SourceID:           omit.From(sourceID),
		ModName:            omit.From(plan.details.Name),
		ModAuthor:          omit.From(plan.details.Author),
		InstalledVersion:   omit.From(plan.versionString),
		InstalledVersionID: omit.From(versionID),
		FileHash:           omit.From(downloadHash),
		AutoUpdate:         omit.From(int64(0)),
		Enabled:            omit.From(int64(1)),
		CreatedAt:          omit.From(now),
		UpdatedAt:          omit.From(now),
	}

	mod, errInsert := m.db.InsertInstalledMod(tx, modSetter)
	if errInsert != nil {
		errCleanup := cleanupRemotePromotedFiles(ctx, client, serverDir, promotedMoves)
		if errCleanup != nil {
			return nil, fmt.Errorf("modmanager: insert mod: %w; cleanup promoted files: %s", errInsert, errCleanup.Error())
		}
		return nil, fmt.Errorf("modmanager: insert mod: %w", errInsert)
	}

	fileSetter := &models.InstalledModFileSetter{
		ID:             omit.From(uuid.NewString()),
		InstalledModID: omit.From(modID),
		FilePath:       omit.From(targetPath),
		FileHash:       omit.From(downloadHash),
		FileSize:       omit.From(downloadSize),
		IsPrimary:      omit.From(int64(1)),
	}

	_, errInsertFile := m.db.InsertInstalledModFile(tx, fileSetter)
	if errInsertFile != nil {
		errCleanup := cleanupRemotePromotedFiles(ctx, client, serverDir, promotedMoves)
		if errCleanup != nil {
			return nil, fmt.Errorf("modmanager: insert mod file: %w; cleanup promoted files: %s", errInsertFile, errCleanup.Error())
		}
		return nil, fmt.Errorf("modmanager: insert mod file: %w", errInsertFile)
	}

	errCommit := tx.Commit(ctx)
	if errCommit != nil {
		errCleanup := cleanupRemotePromotedFiles(ctx, client, serverDir, promotedMoves)
		if errCleanup != nil {
			return nil, fmt.Errorf("modmanager: commit transaction: %w; cleanup promoted files: %s", errCommit, errCleanup.Error())
		}
		return nil, fmt.Errorf("modmanager: commit transaction: %w", errCommit)
	}

	return mod, nil
}

// Uninstall removes mod files through a node client and deletes DB records.
func (m *ModManager) Uninstall(ctx context.Context, client FileClient, modID, serverDir string) error {
	mod, errGet := m.db.GetInstalledModByID(modID)
	if errGet != nil {
		return fmt.Errorf("%w: %s: %w", ErrModNotFound, modID, errGet)
	}

	files, errFiles := m.db.GetInstalledModFilesByModID(mod.ID)
	if errFiles != nil {
		return fmt.Errorf("modmanager: get mod files: %w", errFiles)
	}

	remoteFiles := make([]string, 0, len(files))
	for _, f := range files {
		remoteFiles = append(remoteFiles, cleanRemotePath(f.FilePath))
	}

	_, errDeleteRemote := client.DeleteFiles(ctx, serverDir, remoteFiles, node.ProtectionPolicy{})
	if errDeleteRemote != nil {
		return fmt.Errorf("modmanager: delete mod files on node: %w", errDeleteRemote)
	}

	errDeleteFiles := m.db.DeleteInstalledModFilesByModID(m.db.DB, mod.ID)
	if errDeleteFiles != nil {
		return fmt.Errorf("modmanager: delete mod files: %w", errDeleteFiles)
	}

	errDelete := m.db.DeleteInstalledModByID(mod.ID)
	if errDelete != nil {
		return fmt.Errorf("modmanager: delete mod: %w", errDelete)
	}

	return nil
}

// Update downloads a new version and replaces files.
func (m *ModManager) Update(ctx context.Context, modID, versionID, serverDir string) (*models.InstalledMod, error) {
	mod, errGet := m.db.GetInstalledModByID(modID)
	if errGet != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrModNotFound, modID, errGet)
	}

	provider, ok := modproviders.GetProvider(mod.Source)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, mod.Source)
	}

	oldFiles, errOldFiles := m.db.GetInstalledModFilesByModID(mod.ID)
	if errOldFiles != nil {
		return nil, fmt.Errorf("modmanager: get old files: %w", errOldFiles)
	}

	// Determine install path from old file paths.
	installSubdir := ""
	if len(oldFiles) > 0 {
		installSubdir = filepath.Dir(oldFiles[0].FilePath)
	}
	installDir := filepath.Join(serverDir, installSubdir)
	stagingDir, errStagingDir := createDownloadStagingDir(installDir)
	if errStagingDir != nil {
		return nil, errStagingDir
	}
	defer func() {
		logRemoveAllError(stagingDir, "Failed to remove mod update staging directory")
	}()

	downloaded, errDownload := provider.Download(ctx, mod.SourceID, versionID, stagingDir)
	if errDownload != nil {
		return nil, fmt.Errorf("modmanager: download update: %w", wrapProviderDownloadError(errDownload))
	}
	if len(downloaded) == 0 {
		return nil, ErrNoFilesDownloaded
	}

	rollbackDir, errRollbackDir := os.MkdirTemp(installDir, ".xylona-rollback-*")
	if errRollbackDir != nil {
		return nil, fmt.Errorf("modmanager: create rollback directory: %w", errRollbackDir)
	}
	defer func() {
		logRemoveAllError(rollbackDir, "Failed to remove mod update rollback directory")
	}()

	rollbackMoves, errRollbackMoves := moveExistingFilesToRollback(serverDir, installSubdir, oldFiles, rollbackDir)
	if errRollbackMoves != nil {
		return nil, errRollbackMoves
	}

	promotedMoves, errPromote := promoteDownloadedFiles(stagingDir, installDir, downloaded)
	if errPromote != nil {
		errRestore := restoreMovedFiles(rollbackMoves)
		if errRestore != nil {
			return nil, fmt.Errorf("%w; restore old files: %s", errPromote, errRestore.Error())
		}
		return nil, errPromote
	}

	// Find the version string.
	versionString := versionID
	details, errDetails := provider.GetModDetails(ctx, mod.SourceID, nil)
	if errDetails == nil {
		for _, v := range details.Versions {
			if v.VersionID == versionID {
				versionString = v.VersionString
				break
			}
		}
	}

	primaryHash := primaryHashFromDownloaded(downloaded)

	// Wrap all DB mutations in a transaction so a partial failure does not leave
	// the database in an inconsistent state.
	t, errTx := m.db.SQLDb.BeginTx(ctx, nil)
	if errTx != nil {
		return nil, fmt.Errorf("modmanager: begin update transaction: %w", errTx)
	}
	tx := bob.NewTx(t)

	defer func() {
		logRollbackError(tx.Rollback(ctx), "Failed to rollback mod update transaction")
	}()

	// Delete old file records and insert new ones.
	errDeleteFiles := m.db.DeleteInstalledModFilesByModID(tx, mod.ID)
	if errDeleteFiles != nil {
		errCleanup := cleanupPromotedFiles(promotedMoves)
		errRestore := restoreMovedFiles(rollbackMoves)
		if errCleanup != nil || errRestore != nil {
			return nil, errors.Join(
				fmt.Errorf("modmanager: delete old file records: %w", errDeleteFiles),
				errCleanup,
				errRestore,
			)
		}
		return nil, fmt.Errorf("modmanager: delete old file records: %w", errDeleteFiles)
	}

	for _, df := range downloaded {
		fileID := uuid.NewString()
		isPrimary := int64(0)
		if df.IsPrimary {
			isPrimary = 1
		}

		// Store the file path relative to serverDir (including the install subdirectory).
		relPath := filepath.Join(installSubdir, df.Path)

		fileSetter := &models.InstalledModFileSetter{
			ID:             omit.From(fileID),
			InstalledModID: omit.From(mod.ID),
			FilePath:       omit.From(relPath),
			FileHash:       omit.From(df.Hash),
			FileSize:       omit.From(df.Size),
			IsPrimary:      omit.From(isPrimary),
		}
		_, errInsertFile := m.db.InsertInstalledModFile(tx, fileSetter)
		if errInsertFile != nil {
			return nil, fmt.Errorf("modmanager: insert updated file record: %w", errInsertFile)
		}
	}

	updateSetter := &models.InstalledModSetter{
		InstalledVersion:   omit.From(versionString),
		InstalledVersionID: omit.From(versionID),
		FileHash:           omit.From(primaryHash),
		UpdatedAt:          omit.From(time.Now().UTC()),
	}

	errUpdate := m.db.UpdateInstalledModInTx(tx, mod, updateSetter)
	if errUpdate != nil {
		errCleanup := cleanupPromotedFiles(promotedMoves)
		errRestore := restoreMovedFiles(rollbackMoves)
		if errCleanup != nil || errRestore != nil {
			return nil, errors.Join(
				fmt.Errorf("modmanager: update mod record: %w", errUpdate),
				errCleanup,
				errRestore,
			)
		}
		return nil, fmt.Errorf("modmanager: update mod record: %w", errUpdate)
	}

	errCommit := tx.Commit(ctx)
	if errCommit != nil {
		errCleanup := cleanupPromotedFiles(promotedMoves)
		errRestore := restoreMovedFiles(rollbackMoves)
		if errCleanup != nil || errRestore != nil {
			return nil, errors.Join(
				fmt.Errorf("modmanager: commit update transaction: %w", errCommit),
				errCleanup,
				errRestore,
			)
		}
		return nil, fmt.Errorf("modmanager: commit update transaction: %w", errCommit)
	}

	// Re-fetch after commit so the returned record reflects the committed values.
	// Reading inside the transaction via UpdateInstalledMod would see pre-commit
	// state from the connection pool's read path.
	updated, errRefetch := m.db.GetInstalledModByID(mod.ID)
	if errRefetch != nil {
		return nil, fmt.Errorf("modmanager: re-fetch updated mod: %w", errRefetch)
	}

	return updated, nil
}

// UpdateRemote downloads a new version through a node client and replaces files.
func (m *ModManager) UpdateRemote(ctx context.Context, client FileClient, modID, versionID, serverDir string) (*models.InstalledMod, error) {
	mod, errGet := m.db.GetInstalledModByID(modID)
	if errGet != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrModNotFound, modID, errGet)
	}

	provider, ok := modproviders.GetProvider(mod.Source)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, mod.Source)
	}

	oldFiles, errOldFiles := m.db.GetInstalledModFilesByModID(mod.ID)
	if errOldFiles != nil {
		return nil, fmt.Errorf("modmanager: get old files: %w", errOldFiles)
	}

	installSubdir := ""
	if len(oldFiles) > 0 {
		installSubdir = path.Dir(cleanRemotePath(oldFiles[0].FilePath))
		if installSubdir == "." {
			installSubdir = ""
		}
	}

	plan, errPlan := buildRemoteDownloadPlan(ctx, provider, mod.SourceID, versionID)
	if errPlan != nil {
		return nil, errPlan
	}

	stagingDir := newRemoteScratchDir(installSubdir, ".xylona-download-")
	errCreateStaging := client.CreateFileOrDirectory(ctx, serverDir, stagingDir, "", true, node.ProtectionPolicy{})
	if errCreateStaging != nil {
		return nil, fmt.Errorf("modmanager: create remote staging directory: %w", errCreateStaging)
	}
	defer func() {
		logRemoteDeleteError(ctx, client, serverDir, []string{stagingDir}, "Failed to remove remote mod update staging directory")
	}()

	downloadResult, errDownload := client.DownloadFileFromURL(ctx, serverDir, plan.version.DownloadURL, stagingDir, remoteDownloadIntegrity(plan.version), node.ProtectionPolicy{})
	if errDownload != nil {
		return nil, fmt.Errorf("modmanager: remote download update: %w", errDownload)
	}
	downloadedPath := downloadResult.RelativePath
	downloadHash := verifiedRemoteDownloadHash(plan.version, downloadResult)
	downloadSize := verifiedRemoteDownloadSize(plan.version, downloadResult)

	rollbackDir := newRemoteScratchDir(installSubdir, ".xylona-rollback-")
	errCreateRollback := client.CreateFileOrDirectory(ctx, serverDir, rollbackDir, "", true, node.ProtectionPolicy{})
	if errCreateRollback != nil {
		return nil, fmt.Errorf("modmanager: create remote rollback directory: %w", errCreateRollback)
	}
	defer func() {
		logRemoteDeleteError(ctx, client, serverDir, []string{rollbackDir}, "Failed to remove remote mod update rollback directory")
	}()

	rollbackMoves, errRollbackMoves := moveRemoteExistingFilesToRollback(ctx, client, serverDir, installSubdir, oldFiles, rollbackDir)
	if errRollbackMoves != nil {
		return nil, errRollbackMoves
	}

	targetPath := joinRemotePath(installSubdir, remoteFileName(downloadedPath))
	_, errPromote := client.RenameFile(ctx, serverDir, downloadedPath, targetPath, node.ProtectionPolicy{})
	if errPromote != nil {
		errRestore := restoreRemoteMovedFiles(ctx, client, serverDir, rollbackMoves)
		if errRestore != nil {
			return nil, fmt.Errorf("modmanager: promote remote staged file %s: %w; restore old files: %s", targetPath, errPromote, errRestore.Error())
		}
		return nil, fmt.Errorf("modmanager: promote remote staged file %s: %w", targetPath, errPromote)
	}
	promotedMoves := []fileMove{{source: downloadedPath, target: targetPath}}

	t, errTx := m.db.SQLDb.BeginTx(ctx, nil)
	if errTx != nil {
		errCleanup := cleanupRemotePromotedFiles(ctx, client, serverDir, promotedMoves)
		errRestore := restoreRemoteMovedFiles(ctx, client, serverDir, rollbackMoves)
		if errCleanup != nil || errRestore != nil {
			return nil, errors.Join(
				fmt.Errorf("modmanager: begin update transaction: %w", errTx),
				errCleanup,
				errRestore,
			)
		}
		return nil, fmt.Errorf("modmanager: begin update transaction: %w", errTx)
	}
	tx := bob.NewTx(t)

	defer func() {
		logRollbackError(tx.Rollback(ctx), "Failed to rollback remote mod update transaction")
	}()

	errDeleteFiles := m.db.DeleteInstalledModFilesByModID(tx, mod.ID)
	if errDeleteFiles != nil {
		errCleanup := cleanupRemotePromotedFiles(ctx, client, serverDir, promotedMoves)
		errRestore := restoreRemoteMovedFiles(ctx, client, serverDir, rollbackMoves)
		if errCleanup != nil || errRestore != nil {
			return nil, errors.Join(
				fmt.Errorf("modmanager: delete old file records: %w", errDeleteFiles),
				errCleanup,
				errRestore,
			)
		}
		return nil, fmt.Errorf("modmanager: delete old file records: %w", errDeleteFiles)
	}

	fileSetter := &models.InstalledModFileSetter{
		ID:             omit.From(uuid.NewString()),
		InstalledModID: omit.From(mod.ID),
		FilePath:       omit.From(targetPath),
		FileHash:       omit.From(downloadHash),
		FileSize:       omit.From(downloadSize),
		IsPrimary:      omit.From(int64(1)),
	}
	_, errInsertFile := m.db.InsertInstalledModFile(tx, fileSetter)
	if errInsertFile != nil {
		errCleanup := cleanupRemotePromotedFiles(ctx, client, serverDir, promotedMoves)
		errRestore := restoreRemoteMovedFiles(ctx, client, serverDir, rollbackMoves)
		if errCleanup != nil || errRestore != nil {
			return nil, errors.Join(
				fmt.Errorf("modmanager: insert updated file record: %w", errInsertFile),
				errCleanup,
				errRestore,
			)
		}
		return nil, fmt.Errorf("modmanager: insert updated file record: %w", errInsertFile)
	}

	updateSetter := &models.InstalledModSetter{
		InstalledVersion:   omit.From(plan.versionString),
		InstalledVersionID: omit.From(versionID),
		FileHash:           omit.From(downloadHash),
		UpdatedAt:          omit.From(time.Now().UTC()),
	}

	errUpdate := m.db.UpdateInstalledModInTx(tx, mod, updateSetter)
	if errUpdate != nil {
		errCleanup := cleanupRemotePromotedFiles(ctx, client, serverDir, promotedMoves)
		errRestore := restoreRemoteMovedFiles(ctx, client, serverDir, rollbackMoves)
		if errCleanup != nil || errRestore != nil {
			return nil, errors.Join(
				fmt.Errorf("modmanager: update mod record: %w", errUpdate),
				errCleanup,
				errRestore,
			)
		}
		return nil, fmt.Errorf("modmanager: update mod record: %w", errUpdate)
	}

	errCommit := tx.Commit(ctx)
	if errCommit != nil {
		errCleanup := cleanupRemotePromotedFiles(ctx, client, serverDir, promotedMoves)
		errRestore := restoreRemoteMovedFiles(ctx, client, serverDir, rollbackMoves)
		if errCleanup != nil || errRestore != nil {
			return nil, errors.Join(
				fmt.Errorf("modmanager: commit update transaction: %w", errCommit),
				errCleanup,
				errRestore,
			)
		}
		return nil, fmt.Errorf("modmanager: commit update transaction: %w", errCommit)
	}

	updated, errRefetch := m.db.GetInstalledModByID(mod.ID)
	if errRefetch != nil {
		return nil, fmt.Errorf("modmanager: re-fetch updated mod: %w", errRefetch)
	}

	return updated, nil
}

// CheckUpdates queries providers for newer versions of all installed mods.
func (m *ModManager) CheckUpdates(
	ctx context.Context,
	serverID, gameVersion string,
) (map[string]*modproviders.ModVersion, error) {
	mods, errGet := m.db.GetInstalledModsByGameServerID(serverID)
	if errGet != nil {
		return nil, fmt.Errorf("modmanager: get installed mods: %w", errGet)
	}

	updates := make(map[string]*modproviders.ModVersion)
	var mu sync.Mutex

	g, gCtx := errgroup.WithContext(ctx)

	for _, mod := range mods {
		g.Go(func() error {
			provider, ok := modproviders.GetProvider(mod.Source)
			if !ok {
				return nil // Skip unknown providers.
			}

			version, errCheck := provider.CheckForUpdate(gCtx, mod.SourceID, gameVersion)
			if errCheck != nil {
				if errors.Is(errCheck, modproviders.ErrNoUpdateAvailable) {
					return nil
				}
				log.Warn().Err(errCheck).
					Str("mod", mod.ModName).
					Str("source", mod.Source).
					Msg("Failed to check for mod update")
				return nil // Don't fail the whole batch.
			}

			if version != nil && version.VersionID != mod.InstalledVersionID {
				mu.Lock()
				updates[mod.ID] = version
				mu.Unlock()
			}
			return nil
		})
	}

	errWait := g.Wait()
	if errWait != nil {
		return nil, fmt.Errorf("modmanager: wait for update checks: %w", errWait)
	}

	return updates, nil
}

// RunAutoUpdates updates mods with auto_update=true before server start.
// statusFn receives status messages for WebSocket streaming.
func (m *ModManager) RunAutoUpdates(
	ctx context.Context,
	serverID, gameVersion, serverDir string,
	statusFn func(string),
) error {
	return m.runEligibleAutoUpdates(
		ctx,
		serverID,
		gameVersion,
		serverDir,
		statusFn,
		func(ctx context.Context, modID, versionID, serverDir string) (*models.InstalledMod, error) {
			return m.Update(ctx, modID, versionID, serverDir)
		},
		"Failed to auto-update mod",
	)
}

// RunAutoUpdatesRemote updates eligible mods through a node client before server start.
func (m *ModManager) RunAutoUpdatesRemote(
	ctx context.Context,
	client FileClient,
	serverID, gameVersion, serverDir string,
	statusFn func(string),
) error {
	return m.runEligibleAutoUpdates(
		ctx,
		serverID,
		gameVersion,
		serverDir,
		statusFn,
		func(ctx context.Context, modID, versionID, serverDir string) (*models.InstalledMod, error) {
			return m.UpdateRemote(ctx, client, modID, versionID, serverDir)
		},
		"Failed to auto-update remote mod",
	)
}

func (m *ModManager) runEligibleAutoUpdates(
	ctx context.Context,
	serverID, gameVersion, serverDir string,
	statusFn func(string),
	updateMod func(context.Context, string, string, string) (*models.InstalledMod, error),
	updateFailureMessage string,
) error {
	mods, errGet := m.db.GetInstalledModsByGameServerID(serverID)
	if errGet != nil {
		return fmt.Errorf("modmanager: get installed mods: %w", errGet)
	}

	for _, mod := range mods {
		if mod.AutoUpdate != 1 {
			continue
		}
		if !mod.PinnedVersion.IsNull() {
			continue
		}
		if mod.Enabled != 1 {
			continue
		}

		provider, ok := modproviders.GetProvider(mod.Source)
		if !ok {
			continue
		}

		statusFn(fmt.Sprintf("Checking for updates: %s", mod.ModName))

		version, errCheck := provider.CheckForUpdate(ctx, mod.SourceID, gameVersion)
		if errCheck != nil {
			if errors.Is(errCheck, modproviders.ErrNoUpdateAvailable) {
				continue
			}
			log.Warn().Err(errCheck).Str("mod", mod.ModName).Msg("Failed to check for auto-update")
			statusFn(fmt.Sprintf("Failed to check update for %s: %s", mod.ModName, errCheck.Error()))
			continue
		}

		if version == nil || version.VersionID == mod.InstalledVersionID {
			continue
		}

		statusFn(fmt.Sprintf("Updating %s to %s", mod.ModName, version.VersionString))

		_, errUpdate := updateMod(ctx, mod.ID, version.VersionID, serverDir)
		if errUpdate != nil {
			log.Error().Err(errUpdate).Str("mod", mod.ModName).Msg(updateFailureMessage)
			statusFn(fmt.Sprintf("Failed to update %s: %s", mod.ModName, errUpdate.Error()))
			continue
		}

		statusFn(fmt.Sprintf("Updated %s to %s", mod.ModName, version.VersionString))
	}

	return nil
}

// Enable moves mod files back from the disabled directory through a node client.
func (m *ModManager) Enable(ctx context.Context, client FileClient, modID, serverDir, installPath string) error {
	mod, errGet := m.db.GetInstalledModByID(modID)
	if errGet != nil {
		return fmt.Errorf("%w: %s: %w", ErrModNotFound, modID, errGet)
	}

	files, errFiles := m.db.GetInstalledModFilesByModID(mod.ID)
	if errFiles != nil {
		return fmt.Errorf("modmanager: get mod files: %w", errFiles)
	}

	cleanInstallPath := cleanRemotePath(installPath)
	disabledDir := joinRemotePath(cleanInstallPath, "disabled")
	filesToMove := make([]string, 0, len(files))
	for _, f := range files {
		filesToMove = append(filesToMove, joinRemotePath(disabledDir, remoteFileName(f.FilePath)))
	}

	_, errMove := client.MoveFiles(ctx, serverDir, filesToMove, cleanInstallPath, node.ProtectionPolicy{})
	if errMove != nil {
		return fmt.Errorf("modmanager: move file from disabled on node: %w", errMove)
	}

	updateSetter := &models.InstalledModSetter{
		Enabled:   omit.From(int64(1)),
		UpdatedAt: omit.From(time.Now().UTC()),
	}
	_, errUpdate := m.db.UpdateInstalledMod(m.db.DB, mod, updateSetter)
	if errUpdate != nil {
		return fmt.Errorf("modmanager: update mod enabled state: %w", errUpdate)
	}

	return nil
}

// Disable moves mod files to a disabled subdirectory through a node client.
func (m *ModManager) Disable(ctx context.Context, client FileClient, modID, serverDir, installPath string) error {
	mod, errGet := m.db.GetInstalledModByID(modID)
	if errGet != nil {
		return fmt.Errorf("%w: %s: %w", ErrModNotFound, modID, errGet)
	}

	files, errFiles := m.db.GetInstalledModFilesByModID(mod.ID)
	if errFiles != nil {
		return fmt.Errorf("modmanager: get mod files: %w", errFiles)
	}

	cleanInstallPath := cleanRemotePath(installPath)
	disabledDir := joinRemotePath(cleanInstallPath, "disabled")
	errMkdir := client.CreateFileOrDirectory(ctx, serverDir, disabledDir, "", true, node.ProtectionPolicy{})
	if errMkdir != nil {
		return fmt.Errorf("modmanager: create disabled directory on node: %w", errMkdir)
	}

	filesToMove := make([]string, 0, len(files))
	for _, f := range files {
		filesToMove = append(filesToMove, cleanRemotePath(f.FilePath))
	}

	_, errMove := client.MoveFiles(ctx, serverDir, filesToMove, disabledDir, node.ProtectionPolicy{})
	if errMove != nil {
		return fmt.Errorf("modmanager: move file to disabled on node: %w", errMove)
	}

	updateSetter := &models.InstalledModSetter{
		Enabled:   omit.From(int64(0)),
		UpdatedAt: omit.From(time.Now().UTC()),
	}
	_, errUpdate := m.db.UpdateInstalledMod(m.db.DB, mod, updateSetter)
	if errUpdate != nil {
		return fmt.Errorf("modmanager: update mod disabled state: %w", errUpdate)
	}

	return nil
}

// SearchAll searches across all compatible providers for a server.
// Returns the combined results and the sum of TotalHits across all providers.
func (m *ModManager) SearchAll(
	ctx context.Context,
	query string,
	sources []SourceConfig,
	sortBy string,
	gameVersion string,
	categories []string,
	limit int,
	offset int,
) ([]modproviders.ModSearchResult, int, error) {
	var mu sync.Mutex
	var allResults []modproviders.ModSearchResult
	totalHits := 0
	hasUnknownTotal := false

	g, gCtx := errgroup.WithContext(ctx)

	for _, src := range sources {
		g.Go(func() error {
			provider, ok := modproviders.GetProvider(src.ID)
			if !ok {
				log.Warn().Str("provider", src.ID).Msg("Provider not found, skipping")
				return nil
			}

			// Merge well-known keys into a copy of the source's SearchParams.
			params := make(modproviders.SearchParams, len(src.SearchParams)+5)
			maps.Copy(params, src.SearchParams)
			if sortBy != "" {
				params[modproviders.ParamSortBy] = sortBy
			}
			if gameVersion != "" {
				params[modproviders.ParamGameVersion] = gameVersion
			}
			if len(categories) > 0 {
				params[modproviders.ParamCategories] = categories
			}
			if limit > 0 {
				params[modproviders.ParamLimit] = limit
			}
			if offset > 0 {
				params[modproviders.ParamOffset] = offset
			}

			searchResult, errSearch := provider.Search(gCtx, query, params)
			if errSearch != nil {
				log.Warn().Err(errSearch).Str("provider", src.ID).Msg("Search failed")
				return nil // Don't fail the whole search.
			}

			mu.Lock()
			allResults = append(allResults, searchResult.Results...)
			if searchResult.TotalHits == modproviders.UnknownTotalHits {
				hasUnknownTotal = true
			} else {
				totalHits += searchResult.TotalHits
			}
			mu.Unlock()
			return nil
		})
	}

	errWait := g.Wait()
	if errWait != nil {
		return nil, 0, fmt.Errorf("modmanager: wait for provider searches: %w", errWait)
	}
	if hasUnknownTotal {
		return allResults, modproviders.UnknownTotalHits, nil
	}

	return allResults, totalHits, nil
}
