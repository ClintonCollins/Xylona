package actions

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// ValidateGameServerBackupDirectory checks whether the configured backup directory is usable for the server.
func ValidateGameServerBackupDirectory(gameServer *models.GameServer) error {
	_, errValidate := resolveValidGameServerBackupDirectory(gameServer)
	return errValidate
}

// ValidateRemoteGameServerBackupDirectory checks a node-native backup directory
// without applying controller-host filepath semantics.
func ValidateRemoteGameServerBackupDirectory(gameServer *models.GameServer) error {
	_, errValidate := resolveValidRemoteGameServerBackupDirectory(gameServer)
	return errValidate
}

// ResolveManagedBackupArchivePath resolves and validates a backup archive path for callers outside this package.
func ResolveManagedBackupArchivePath(gameServer *models.GameServer, backup *models.GameServerBackup) (string, error) {
	return resolveValidatedBackupArchivePath(gameServer, backup)
}

// ResolveManagedRemoteBackupArchivePath resolves and validates a node-native backup archive path.
func ResolveManagedRemoteBackupArchivePath(gameServer *models.GameServer, backup *models.GameServerBackup) (string, error) {
	return resolveValidatedRemoteBackupArchivePath(gameServer, backup)
}

func (inst *Instance) resolveValidatedBackupArchivePath(gameServer *models.GameServer, backup *models.GameServerBackup) (string, error) {
	if inst.isRemoteGameServer(gameServer) {
		return resolveValidatedRemoteBackupArchivePath(gameServer, backup)
	}
	return resolveValidatedBackupArchivePath(gameServer, backup)
}

func resolveValidatedBackupArchivePath(gameServer *models.GameServer, backup *models.GameServerBackup) (string, error) {
	if backup == nil {
		return "", errInvalidBackupArchivePath
	}
	if backup.NodeID != gameServer.NodeID {
		return "", errBackupNodeMismatch
	}

	resolvedArchivePath, errArchivePath := resolvePathForComparison(backup.ArchivePath)
	if errArchivePath != nil {
		return "", fmt.Errorf("actions: resolve backup archive path: %w", errArchivePath)
	}
	resolvedArchiveRoot, errArchiveRoot := resolvePathForComparison(strings.TrimSpace(backup.ArchiveRoot))
	if errArchiveRoot != nil {
		return "", fmt.Errorf("actions: resolve backup archive root: %w", errArchiveRoot)
	}
	if !strings.EqualFold(filepath.Ext(resolvedArchivePath), ".zip") {
		return "", errInvalidBackupArchivePath
	}
	resolvedArchiveDirectory, errArchiveDirectory := resolvePathForComparison(buildBackupArchiveDirectory(resolvedArchiveRoot, gameServer.ID))
	if errArchiveDirectory != nil {
		return "", fmt.Errorf("actions: resolve backup archive directory: %w", errArchiveDirectory)
	}
	if !pathWithinOrEqual(resolvedArchiveDirectory, resolvedArchivePath) {
		return "", errInvalidBackupArchivePath
	}
	if backupDirectoryWithinGameServer(gameServer.Directory, resolvedArchiveDirectory) {
		return "", errBackupDirectoryInsideServer
	}

	return strings.TrimSpace(backup.ArchivePath), nil
}

func resolveValidatedRemoteBackupArchivePath(gameServer *models.GameServer, backup *models.GameServerBackup) (string, error) {
	if backup == nil {
		return "", errInvalidBackupArchivePath
	}
	if backup.NodeID != gameServer.NodeID {
		return "", errBackupNodeMismatch
	}

	archivePath := strings.TrimSpace(backup.ArchivePath)
	archiveRoot := strings.TrimSpace(backup.ArchiveRoot)
	if archivePath == "" || archiveRoot == "" {
		return "", errInvalidBackupArchivePath
	}
	if !strings.EqualFold(path.Ext(strings.ReplaceAll(archivePath, `\`, "/")), ".zip") {
		return "", errInvalidBackupArchivePath
	}

	archiveDirectory := buildBackupArchiveDirectory(archiveRoot, gameServer.ID)
	if !remotePathWithinOrEqual(archiveDirectory, archivePath) {
		return "", errInvalidBackupArchivePath
	}
	if remotePathWithinOrEqual(gameServer.Directory, archiveDirectory) {
		return "", errBackupDirectoryInsideServer
	}

	return archivePath, nil
}

func resolveValidGameServerBackupDirectory(gameServer *models.GameServer) (string, error) {
	backupDirectory := strings.TrimSpace(gameServer.BackupDirectory)
	if backupDirectory == "" {
		return "", errBackupDirectoryNotConfigured
	}
	archiveDirectory := buildBackupArchiveDirectory(backupDirectory, gameServer.ID)
	if backupDirectoryWithinGameServer(gameServer.Directory, archiveDirectory) {
		return "", errBackupDirectoryInsideServer
	}

	return backupDirectory, nil
}

func resolveValidRemoteGameServerBackupDirectory(gameServer *models.GameServer) (string, error) {
	backupDirectory := strings.TrimSpace(gameServer.BackupDirectory)
	if backupDirectory == "" {
		return "", errBackupDirectoryNotConfigured
	}
	archiveDirectory := buildBackupArchiveDirectory(backupDirectory, gameServer.ID)
	if remotePathWithinOrEqual(gameServer.Directory, archiveDirectory) {
		return "", errBackupDirectoryInsideServer
	}

	return backupDirectory, nil
}

func buildBackupArchivePath(
	backupDirectory string,
	gameServerID string,
	now time.Time,
	triggerSource string,
	backupName string,
) (string, error) {
	fileName, errFileName := buildBackupArchiveFileName(now, triggerSource, backupName)
	if errFileName != nil {
		return "", errFileName
	}

	return joinRemotePath(buildBackupArchiveDirectory(backupDirectory, gameServerID), fileName), nil
}

func buildBackupArchiveFileName(now time.Time, triggerSource string, backupName string) (string, error) {
	timestamp := now.UTC().Format("20060102T150405.000000000Z")
	normalizedName, errNormalize := normalizeManualBackupName(backupName)
	if errNormalize != nil {
		return "", errNormalize
	}
	if normalizedName == "" {
		return timestamp + "-" + triggerSource + ".zip", nil
	}

	if strings.EqualFold(filepath.Ext(normalizedName), ".zip") {
		return normalizedName, nil
	}

	return normalizedName + ".zip", nil
}

func buildBackupArchiveDirectory(backupDirectory string, gameServerID string) string {
	return joinRemotePath(backupDirectory, gameServerID)
}

func resolveUniqueBackupArchivePath(
	backupDirectory string,
	gameServerID string,
	now time.Time,
	triggerSource string,
	backupName string,
) (string, error) {
	basePath, errBasePath := buildBackupArchivePath(backupDirectory, gameServerID, now, triggerSource, backupName)
	if errBasePath != nil {
		return "", errBasePath
	}
	if strings.TrimSpace(backupName) != "" {
		_, errStat := os.Stat(basePath)
		if errors.Is(errStat, os.ErrNotExist) {
			return basePath, nil
		}
		if errStat != nil {
			return "", fmt.Errorf("stat backup archive candidate: %w", errStat)
		}

		return "", ErrManualBackupNameAlreadyExists
	}

	baseDirectory := remotePathDir(basePath)
	baseName := strings.TrimSuffix(remotePathBase(basePath), ".zip")

	for suffix := range maxBackupArchiveResolveAttempts {
		candidatePath := basePath
		if suffix > 0 {
			fileName := fmt.Sprintf("%s-%d.zip", baseName, suffix)
			candidatePath = joinRemotePath(baseDirectory, fileName)
		}

		_, errStat := os.Stat(candidatePath)
		if errors.Is(errStat, os.ErrNotExist) {
			return candidatePath, nil
		}
		if errStat != nil {
			return "", fmt.Errorf("stat backup archive candidate: %w", errStat)
		}
	}

	return "", fmt.Errorf("exhausted backup archive path candidates after %d attempts", maxBackupArchiveResolveAttempts)
}

func (inst *Instance) resolveUniqueBackupArchivePath(
	gameServer *models.GameServer,
	backupDirectory string,
	now time.Time,
	triggerSource string,
	backupName string,
) (string, error) {
	if inst.isRemoteGameServer(gameServer) {
		return resolveUniqueRemoteBackupArchivePath(inst.db, backupDirectory, gameServer.ID, now, triggerSource, backupName)
	}
	return resolveUniqueBackupArchivePath(backupDirectory, gameServer.ID, now, triggerSource, backupName)
}

func resolveUniqueRemoteBackupArchivePath(
	conn *db.Connection,
	backupDirectory string,
	gameServerID string,
	now time.Time,
	triggerSource string,
	backupName string,
) (string, error) {
	basePath, errBasePath := buildBackupArchivePath(backupDirectory, gameServerID, now, triggerSource, backupName)
	if errBasePath != nil {
		return "", errBasePath
	}

	backups, errList := listGameServerBackupsByGameServerID(conn, gameServerID)
	if errList != nil {
		return "", fmt.Errorf("actions: list backups while checking remote archive path: %w", errList)
	}

	if strings.TrimSpace(backupName) != "" {
		if remoteBackupArchivePathClaimed(backups, basePath) {
			return "", ErrManualBackupNameAlreadyExists
		}
		return basePath, nil
	}

	baseDirectory := remotePathDir(basePath)
	baseName := strings.TrimSuffix(remotePathBase(basePath), ".zip")
	for suffix := range maxBackupArchiveResolveAttempts {
		candidatePath := basePath
		if suffix > 0 {
			fileName := fmt.Sprintf("%s-%d.zip", baseName, suffix)
			candidatePath = joinRemotePath(baseDirectory, fileName)
		}
		if !remoteBackupArchivePathClaimed(backups, candidatePath) {
			return candidatePath, nil
		}
	}

	return "", fmt.Errorf("exhausted backup archive path candidates after %d attempts", maxBackupArchiveResolveAttempts)
}

func ensureBackupArchivePathAvailable(conn *db.Connection, gameServerID string, archivePath string) error {
	backups, errList := listGameServerBackupsByGameServerID(conn, gameServerID)
	if errList != nil {
		return fmt.Errorf("actions: list backups while checking archive path: %w", errList)
	}
	if backupArchivePathClaimed(backups, archivePath) {
		return ErrManualBackupNameAlreadyExists
	}

	return nil
}

func backupArchivePathClaimed(backups []*models.GameServerBackup, archivePath string) bool {
	if len(backups) == 0 {
		return false
	}

	resolvedArchivePath, errArchivePath := resolvePathForComparison(archivePath)
	if errArchivePath != nil {
		resolvedArchivePath = filepath.Clean(strings.TrimSpace(archivePath))
	}

	for _, backup := range backups {
		if backup == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(backup.Status), "failed") {
			continue
		}

		resolvedExistingPath, errExistingPath := resolvePathForComparison(backup.ArchivePath)
		if errExistingPath != nil {
			resolvedExistingPath = filepath.Clean(strings.TrimSpace(backup.ArchivePath))
		}
		if strings.EqualFold(resolvedArchivePath, resolvedExistingPath) {
			return true
		}
	}

	return false
}

func remoteBackupArchivePathClaimed(backups []*models.GameServerBackup, archivePath string) bool {
	if len(backups) == 0 {
		return false
	}

	resolvedArchivePath := normalizeRemoteComparablePath(archivePath)
	for _, backup := range backups {
		if backup == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(backup.Status), "failed") {
			continue
		}

		resolvedExistingPath := normalizeRemoteComparablePath(backup.ArchivePath)
		if strings.EqualFold(resolvedArchivePath, resolvedExistingPath) {
			return true
		}
	}

	return false
}

func joinRemotePath(directory string, name string) string {
	separator := remotePathSeparator(directory)
	trimmedDirectory := strings.TrimRight(directory, `/\`)
	if trimmedDirectory == "" {
		return separator + name
	}
	return trimmedDirectory + separator + name
}

func remotePathSeparator(directory string) string {
	hasBackslash := strings.Contains(directory, `\`)
	hasSlash := strings.Contains(directory, "/")
	if hasBackslash && !hasSlash {
		return `\`
	}
	return "/"
}

func remotePathBase(pathValue string) string {
	trimmedPath := strings.TrimRight(pathValue, `/\`)
	index := strings.LastIndexAny(trimmedPath, `/\`)
	if index < 0 {
		return trimmedPath
	}
	return trimmedPath[index+1:]
}

func remotePathDir(pathValue string) string {
	trimmedPath := strings.TrimRight(pathValue, `/\`)
	index := strings.LastIndexAny(trimmedPath, `/\`)
	if index < 0 {
		return "."
	}
	if index == 0 {
		return trimmedPath[:1]
	}
	return trimmedPath[:index]
}

func remotePathWithinOrEqual(parentPath string, candidatePath string) bool {
	parent := normalizeRemoteComparablePath(parentPath)
	candidate := normalizeRemoteComparablePath(candidatePath)
	if parent == "" || candidate == "" {
		return false
	}
	if strings.EqualFold(parent, candidate) {
		return true
	}
	prefix := parent
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return strings.HasPrefix(strings.ToLower(candidate), strings.ToLower(prefix))
}

func normalizeRemoteComparablePath(pathValue string) string {
	slashPath := strings.ReplaceAll(strings.TrimSpace(pathValue), `\`, "/")
	cleanPath := path.Clean(slashPath)
	if cleanPath == "." {
		return ""
	}
	if cleanPath != "/" {
		cleanPath = strings.TrimRight(cleanPath, "/")
	}
	return cleanPath
}

func backupDirectoryWithinGameServer(gameServerDirectory string, backupDirectory string) bool {
	resolvedGameServerDirectory, errGameServer := resolvePathForComparison(gameServerDirectory)
	if errGameServer != nil {
		return false
	}
	resolvedBackupDirectory, errBackupDirectory := resolvePathForComparison(backupDirectory)
	if errBackupDirectory != nil {
		return false
	}

	return pathWithinOrEqual(resolvedGameServerDirectory, resolvedBackupDirectory)
}

func validateBackupRelativePath(relativePath string) (string, error) {
	slashNormalized := strings.ReplaceAll(relativePath, "\\", "/")
	return validateBackupSlashPath(slashNormalized)
}

func validateBackupZipEntryPath(entryName string) (string, error) {
	if entryName == "" {
		return "", errInvalidBackupArchivePath
	}

	slashNormalized := strings.ReplaceAll(entryName, "\\", "/")
	return validateBackupSlashPath(slashNormalized)
}

func validateBackupSlashPath(slashPath string) (string, error) {
	cleaned := path.Clean(slashPath)
	if cleaned == "." || cleaned == "" {
		return "", errInvalidBackupArchivePath
	}
	if strings.HasPrefix(cleaned, "/") {
		return "", errInvalidBackupArchivePath
	}
	if cleaned == ".." {
		return "", errInvalidBackupArchivePath
	}
	if strings.HasPrefix(cleaned, "../") {
		return "", errInvalidBackupArchivePath
	}
	if strings.Contains(cleaned, "/../") {
		return "", errInvalidBackupArchivePath
	}
	if hasWindowsDrivePrefix(cleaned) {
		return "", errInvalidBackupArchivePath
	}

	return cleaned, nil
}

func hasWindowsDrivePrefix(pathValue string) bool {
	if len(pathValue) < 2 {
		return false
	}
	if (pathValue[0] < 'A' || pathValue[0] > 'Z') && (pathValue[0] < 'a' || pathValue[0] > 'z') {
		return false
	}
	if pathValue[1] != ':' {
		return false
	}
	if len(pathValue) == 2 {
		return true
	}
	return pathValue[2] == '/'
}

func resolvePathForComparison(pathValue string) (string, error) {
	cleanAbsolutePath, errAbs := filepath.Abs(strings.TrimSpace(pathValue))
	if errAbs != nil {
		return "", fmt.Errorf("actions: resolve absolute path: %w", errAbs)
	}

	currentPath := filepath.Clean(cleanAbsolutePath)
	var missingPathSuffix []string
	for {
		resolvedPath, errEval := filepath.EvalSymlinks(currentPath)
		if errEval == nil {
			for _, suffix := range slices.Backward(missingPathSuffix) {
				resolvedPath = filepath.Join(resolvedPath, suffix)
			}
			return filepath.Clean(resolvedPath), nil
		}
		if !errors.Is(errEval, os.ErrNotExist) {
			return "", fmt.Errorf("actions: resolve symlinks: %w", errEval)
		}

		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			return currentPath, nil
		}

		missingPathSuffix = append(missingPathSuffix, filepath.Base(currentPath))
		currentPath = parentPath
	}
}

func pathWithinOrEqual(parentPath string, candidatePath string) bool {
	relativePath, errRel := filepath.Rel(parentPath, candidatePath)
	if errRel != nil {
		return false
	}
	if relativePath == "." {
		return true
	}
	if relativePath == ".." {
		return false
	}
	if strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}
