package modmanager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/stephenafamo/bob"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/modproviders/thunderstore"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type valheimDownloader interface {
	DownloadValheim(context.Context, string, string, string, string) ([]modproviders.DownloadedFile, error)
}

// ErrValheimServerRunning requires a stopped server before modifying its runtime.
var ErrValheimServerRunning = errors.New("stop the Valheim server before changing mods")

func (m *ModManager) checkValheimStopped(ctx context.Context, client FileClient, serverID string) error {
	server, errServer := m.db.GetGameServerByID(serverID)
	if errServer != nil {
		return fmt.Errorf("valheim mod server: %w", errServer)
	}
	live, supported := client.(interface {
		GetProcessSnapshot(context.Context, string) (*node.ProcessSnapshot, bool, error)
	})
	if supported {
		snapshot, found, errSnapshot := live.GetProcessSnapshot(ctx, serverID)
		if errSnapshot != nil {
			return fmt.Errorf("valheim mod snapshot: %w", errSnapshot)
		}
		if found && (snapshot == nil || snapshot.Status != "OFFLINE") {
			return ErrValheimServerRunning
		}
		return nil
	}
	if server.Status != "OFFLINE" {
		return ErrValheimServerRunning
	}
	return nil
}

type valheimFileClient interface {
	FileClient
	ListFiles(context.Context, string, string) ([]node.FileEntry, error)
	StreamWriteFile(context.Context, string, string, io.Reader, node.ProtectionPolicy) (node.WriteFileResult, error)
	GetNodeSnapshot(context.Context) (*node.NodeSnapshot, error)
}

func (m *ModManager) isValheimMod(serverID, source string) (bool, error) {
	if source != "thunderstore" {
		return false, nil
	}
	server, errServer := m.db.GetGameServerByID(serverID)
	if errServer != nil {
		return false, fmt.Errorf("look up mod server: %w", errServer)
	}
	return server.GameID == "valheim", nil
}

func valheimModPath(mod *models.InstalledMod, file string) string {
	file = cleanRemotePath(file)
	if mod.Enabled == 0 && !valheimConfig(file) {
		return path.Join(".xylona-disabled", mod.ID, file)
	}
	return file
}

func valheimConfig(file string) bool {
	return strings.HasPrefix(strings.ToLower(cleanRemotePath(file)), "bepinex/config/")
}

func (m *ModManager) valheimClient(ctx context.Context, client FileClient) (valheimFileClient, string, error) {
	if client == nil {
		return nodeclient.NewInProcessClient("", node.New(ctx, nil, m.db)), runtime.GOOS, nil
	}
	files, ok := client.(valheimFileClient)
	if !ok {
		return nil, "", errors.New("target node does not support extracted mod installation")
	}
	snapshot, errSnapshot := files.GetNodeSnapshot(ctx)
	if errSnapshot != nil {
		return nil, "", fmt.Errorf("get target operating system: %w", errSnapshot)
	}
	if snapshot == nil || (snapshot.OS != "windows" && snapshot.OS != "linux") {
		return nil, "", errors.New("valheim mods require a Windows or Linux node")
	}
	return files, snapshot.OS, nil
}

func valheimFileExists(ctx context.Context, client valheimFileClient, directory, file string) (bool, error) {
	parent := path.Dir(file)
	errMkdir := client.CreateFileOrDirectory(ctx, directory, parent, "", true, node.ProtectionPolicy{})
	if errMkdir != nil {
		return false, fmt.Errorf("valheim mod mkdir: %w", errMkdir)
	}
	entries, errList := client.ListFiles(ctx, directory, parent)
	if errList != nil {
		return false, fmt.Errorf("valheim mod list: %w", errList)
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name, path.Base(file)) {
			return true, nil
		}
	}
	return false, nil
}

// changeValheimPackage stages the complete package before replacing its owned files.
// The caller holds the server mod lock.
func (m *ModManager) changeValheimPackage(ctx context.Context, supplied FileClient, provider modproviders.ModProvider, serverID, sourceID, versionID, directory string, old *models.InstalledMod) (_ *models.InstalledMod, resultErr error) {
	errStopped := m.checkValheimStopped(ctx, supplied, serverID)
	if errStopped != nil {
		return nil, fmt.Errorf("valheim mod stopped: %w", errStopped)
	}
	downloader, ok := provider.(valheimDownloader)
	if !ok {
		return nil, errors.New("thunderstore provider does not support Valheim archives")
	}
	mods, errMods := m.db.GetInstalledModsByGameServerID(serverID)
	if errMods != nil {
		return nil, fmt.Errorf("valheim mod mods: %w", errMods)
	}
	for _, installed := range mods {
		if old == nil && installed.Source == "thunderstore" && installed.SourceID == sourceID {
			if installed.InstalledVersionID == versionID {
				return installed, nil
			}
			return nil, errors.New("mod is already installed; update its version instead")
		}
	}
	client, targetOS, errClient := m.valheimClient(ctx, supplied)
	if errClient != nil {
		return nil, fmt.Errorf("valheim mod client: %w", errClient)
	}
	staging, errStaging := os.MkdirTemp("", "mod-download-")
	if errStaging != nil {
		return nil, fmt.Errorf("valheim mod staging: %w", errStaging)
	}
	defer logRemoveAllError(staging, "Failed to remove mod download staging")
	downloaded, errDownload := downloader.DownloadValheim(ctx, sourceID, versionID, staging, targetOS)
	if errDownload != nil {
		return nil, fmt.Errorf("valheim mod download: %w", errDownload)
	}
	details, errDetails := provider.GetModDetails(ctx, sourceID, nil)
	if errDetails != nil {
		return nil, fmt.Errorf("valheim mod details: %w", errDetails)
	}
	mod := old
	if mod == nil {
		mod = &models.InstalledMod{ID: uuid.NewString(), GameServerID: serverID, Source: "thunderstore", SourceID: sourceID, Enabled: 1}
	}
	owned := make(map[string]*models.InstalledModFile)
	if old != nil {
		files, errFiles := m.db.GetInstalledModFilesByModID(old.ID)
		if errFiles != nil {
			return nil, fmt.Errorf("valheim mod files: %w", errFiles)
		}
		for _, file := range files {
			owned[cleanRemotePath(file.FilePath)] = file
		}
	}
	stagePath := path.Join(".xylona-mod-staging", uuid.NewString())
	rollbackPath := path.Join(stagePath, "previous")
	var promoted, moved []fileMove
	committed := false
	defer func() {
		cleanupContext := context.WithoutCancel(ctx)
		if !committed {
			errCleanup := cleanupRemotePromotedFiles(cleanupContext, client, directory, promoted)
			errRestore := restoreRemoteMovedFiles(cleanupContext, client, directory, moved)
			resultErr = errors.Join(resultErr, errCleanup, errRestore)
			if errCleanup != nil || errRestore != nil {
				return // Retain recovery files when restoration fails.
			}
		}
		logRemoteDeleteError(cleanupContext, client, directory, []string{stagePath}, "Failed to remove mod transaction staging")
	}()
	retained := make(map[string]bool)
	files := make([]modproviders.DownloadedFile, 0, len(downloaded))
	seen := make(map[string]bool)
	for _, file := range downloaded {
		if !filepath.IsLocal(file.Path) || strings.Contains(file.Path, "\\") || path.Clean(file.Path) != file.Path || seen[strings.ToLower(file.Path)] {
			return nil, errors.New("invalid or duplicate extracted mod path")
		}
		seen[strings.ToLower(file.Path)] = true
		target := valheimModPath(mod, file.Path)
		exists, errExists := valheimFileExists(ctx, client, directory, target)
		if errExists != nil {
			return nil, fmt.Errorf("valheim mod exists: %w", errExists)
		}
		if exists && (valheimConfig(file.Path) || strings.EqualFold(file.Path, "doorstop_config.ini")) {
			retained[file.Path] = true
			previous, tracked := owned[file.Path]
			if tracked {
				file.Hash, file.Size = previous.FileHash, previous.FileSize
				files = append(files, file)
			}
			continue
		}
		if exists && owned[file.Path] == nil {
			return nil, fmt.Errorf("mod file already exists and is not owned by this package: %s", file.Path)
		}
		stream, errOpen := os.Open(filepath.Join(staging, file.Path))
		if errOpen != nil {
			return nil, fmt.Errorf("valheim mod open: %w", errOpen)
		}
		stagedFile := path.Join(stagePath, "next", file.Path)
		errMkdir := client.CreateFileOrDirectory(ctx, directory, path.Dir(stagedFile), "", true, node.ProtectionPolicy{})
		if errMkdir != nil {
			return nil, errors.Join(errMkdir, stream.Close())
		}
		receipt, errWrite := client.StreamWriteFile(ctx, directory, stagedFile, stream, node.ProtectionPolicy{})
		errClose := stream.Close()
		if errWrite != nil || errClose != nil {
			return nil, errors.Join(errWrite, errClose)
		}
		if receipt.BytesWritten != file.Size || receipt.SHA256 != file.Hash {
			return nil, errors.New("mod file changed during transfer")
		}
		files = append(files, file)
	}
	errStopped = m.checkValheimStopped(ctx, supplied, serverID)
	if errStopped != nil {
		return nil, errStopped
	}
	for name := range owned {
		if valheimConfig(name) || retained[name] {
			continue
		}
		original := valheimModPath(mod, name)
		exists, errExists := valheimFileExists(ctx, client, directory, original)
		if errExists != nil {
			return nil, fmt.Errorf("valheim mod exists: %w", errExists)
		}
		if !exists {
			continue
		}
		backup := path.Join(rollbackPath, name)
		errMkdir := client.CreateFileOrDirectory(ctx, directory, path.Dir(backup), "", true, node.ProtectionPolicy{})
		if errMkdir != nil {
			return nil, fmt.Errorf("valheim mod mkdir: %w", errMkdir)
		}
		_, errMove := client.RenameFile(ctx, directory, original, backup, node.ProtectionPolicy{})
		if errMove != nil {
			return nil, fmt.Errorf("valheim mod move: %w", errMove)
		}
		moved = append(moved, fileMove{source: backup, target: original})
	}
	for _, file := range files {
		if retained[file.Path] {
			continue
		}
		target := valheimModPath(mod, file.Path)
		staged := path.Join(stagePath, "next", file.Path)
		_, errPromote := client.RenameFile(ctx, directory, staged, target, node.ProtectionPolicy{})
		if errPromote != nil {
			return nil, fmt.Errorf("valheim mod promote: %w", errPromote)
		}
		promoted = append(promoted, fileMove{source: staged, target: target})
	}
	t, errTx := m.db.SQLDb.BeginTx(ctx, nil)
	if errTx != nil {
		return nil, fmt.Errorf("valheim mod tx: %w", errTx)
	}
	tx := bob.NewTx(t)
	defer func() {
		logRollbackError(tx.Rollback(context.WithoutCancel(ctx)), "Failed to rollback mod transaction")
	}()
	now := time.Now().UTC()
	setter := &models.InstalledModSetter{InstalledVersion: omit.From(versionID), InstalledVersionID: omit.From(versionID), FileHash: omit.From(primaryHashFromDownloaded(files)), UpdatedAt: omit.From(now)}
	if old == nil {
		setter.ID, setter.GameServerID = omit.From(mod.ID), omit.From(serverID)
		setter.Source, setter.SourceID = omit.From("thunderstore"), omit.From(sourceID)
		setter.ModName, setter.ModAuthor = omit.From(details.Name), omit.From(details.Author)
		setter.Enabled, setter.AutoUpdate, setter.CreatedAt = omit.From(int64(1)), omit.From(int64(0)), omit.From(now)
		var errInsert error
		mod, errInsert = m.db.InsertInstalledMod(tx, setter)
		if errInsert != nil {
			return nil, fmt.Errorf("valheim mod insert: %w", errInsert)
		}
	} else {
		errUpdate := m.db.UpdateInstalledModInTx(tx, mod, setter)
		if errUpdate != nil {
			return nil, fmt.Errorf("valheim mod update: %w", errUpdate)
		}
		errDelete := m.db.DeleteInstalledModFilesByModID(tx, mod.ID)
		if errDelete != nil {
			return nil, fmt.Errorf("valheim mod delete: %w", errDelete)
		}
	}
	for _, file := range files {
		primary := int64(0)
		if file.IsPrimary {
			primary = 1
		}
		_, errInsert := m.db.InsertInstalledModFile(tx, &models.InstalledModFileSetter{ID: omit.From(uuid.NewString()), InstalledModID: omit.From(mod.ID), FilePath: omit.From(file.Path), FileHash: omit.From(file.Hash), FileSize: omit.From(file.Size), IsPrimary: omit.From(primary)})
		if errInsert != nil {
			return nil, fmt.Errorf("valheim mod insert: %w", errInsert)
		}
	}
	errCommit := tx.Commit(ctx)
	if errCommit != nil {
		return nil, fmt.Errorf("valheim mod commit: %w", errCommit)
	}
	committed = true
	updated, errRead := m.db.GetInstalledModByID(mod.ID)
	if errRead != nil {
		return nil, fmt.Errorf("read installed mod: %w", errRead)
	}
	return updated, nil
}

func (m *ModManager) protectValheimLoader(mod *models.InstalledMod) error {
	if mod.SourceID != thunderstore.ValheimLoaderID {
		return nil
	}
	mods, errMods := m.db.GetInstalledModsByGameServerID(mod.GameServerID)
	if errMods != nil {
		return fmt.Errorf("valheim mod mods: %w", errMods)
	}
	for _, other := range mods {
		if other.ID != mod.ID && other.Source == "thunderstore" {
			return errors.New("remove other Valheim mods before disabling or removing BepInEx")
		}
	}
	return nil
}

func (m *ModManager) changeValheimMod(ctx context.Context, client FileClient, serverID, sourceID, versionID, directory string) (*models.InstalledMod, error) {
	provider, ok := modproviders.GetProvider("thunderstore")
	if !ok {
		return nil, ErrProviderNotFound
	}
	installed, errInstalled := m.db.GetInstalledModsByGameServerID(serverID)
	if errInstalled != nil {
		return nil, fmt.Errorf("valheim mod installed: %w", errInstalled)
	}
	plan, errPlan := resolveValheimPackages(ctx, provider, sourceID, versionID, installed)
	if errPlan != nil {
		return nil, fmt.Errorf("valheim mod plan: %w", errPlan)
	}
	var result *models.InstalledMod
	for _, pkg := range plan {
		var previous *models.InstalledMod
		for _, candidate := range installed {
			if candidate.Source == "thunderstore" && candidate.SourceID == pkg.ID {
				previous = candidate
				break
			}
		}
		if previous != nil && previous.InstalledVersionID == pkg.Version && pkg.ID != sourceID {
			files, errFiles := m.db.GetInstalledModFilesByModID(previous.ID)
			if errFiles != nil {
				return nil, fmt.Errorf("valheim mod files: %w", errFiles)
			}
			extracted := len(files) > 0
			for _, file := range files {
				extracted = extracted && !strings.HasSuffix(strings.ToLower(file.FilePath), ".zip")
			}
			if extracted {
				fileClient, _, errClient := m.valheimClient(ctx, client)
				if errClient != nil {
					return nil, errClient
				}
				for _, file := range files {
					if valheimConfig(file.FilePath) {
						continue
					}
					exists, errExists := valheimFileExists(ctx, fileClient, directory, valheimModPath(previous, file.FilePath))
					if errExists != nil {
						return nil, errExists
					}
					extracted = extracted && exists
				}
				if extracted {
					result = previous
					continue
				}
			}
		}
		var errChange error
		result, errChange = m.changeValheimPackage(ctx, client, provider, serverID, pkg.ID, pkg.Version, directory, previous)
		if errChange != nil {
			return nil, fmt.Errorf("install %s: %w", pkg.ID, errChange)
		}
	}
	return result, nil
}

// mutateValheimMod preserves relative paths when disabling and stages removals
// until their database transaction commits. Configuration remains available.
func (m *ModManager) mutateValheimMod(ctx context.Context, supplied FileClient, mod *models.InstalledMod, directory, operation string) (resultErr error) {
	errStopped := m.checkValheimStopped(ctx, supplied, mod.GameServerID)
	if errStopped != nil {
		return fmt.Errorf("valheim mod stopped: %w", errStopped)
	}
	if operation == "enable" && mod.Enabled == 1 || operation == "disable" && mod.Enabled == 0 {
		return nil
	}
	if operation != "enable" {
		errLoader := m.protectValheimLoader(mod)
		if errLoader != nil {
			return fmt.Errorf("valheim mod loader: %w", errLoader)
		}
	}
	client, _, errClient := m.valheimClient(ctx, supplied)
	if errClient != nil {
		return fmt.Errorf("valheim mod client: %w", errClient)
	}
	files, errFiles := m.db.GetInstalledModFilesByModID(mod.ID)
	if errFiles != nil {
		return fmt.Errorf("valheim mod files: %w", errFiles)
	}
	stage := path.Join(".xylona-mod-staging", uuid.NewString())
	var moved []fileMove
	committed := false
	defer func() {
		cleanupContext := context.WithoutCancel(ctx)
		if !committed {
			errRestore := restoreRemoteMovedFiles(cleanupContext, client, directory, moved)
			resultErr = errors.Join(resultErr, errRestore)
			if errRestore != nil {
				return
			}
		}
		if operation == "remove" && len(moved) > 0 {
			logRemoteDeleteError(cleanupContext, client, directory, []string{stage}, "Failed to remove uninstalled mod staging")
		}
	}()
	for _, file := range files {
		if valheimConfig(file.FilePath) {
			continue
		}
		original := valheimModPath(mod, file.FilePath)
		destination := path.Join(stage, cleanRemotePath(file.FilePath))
		switch operation {
		case "enable":
			destination = cleanRemotePath(file.FilePath)
		case "disable":
			destination = path.Join(".xylona-disabled", mod.ID, cleanRemotePath(file.FilePath))
		}
		exists, errExists := valheimFileExists(ctx, client, directory, original)
		if errExists != nil {
			return fmt.Errorf("valheim mod exists: %w", errExists)
		}
		if !exists && operation == "remove" {
			continue
		}
		if !exists {
			return fmt.Errorf("mod file is missing: %s; reinstall the mod", original)
		}
		conflict, errConflict := valheimFileExists(ctx, client, directory, destination)
		if errConflict != nil {
			return fmt.Errorf("valheim mod conflict: %w", errConflict)
		}
		if conflict {
			return fmt.Errorf("mod destination already exists: %s", destination)
		}
		_, errMove := client.RenameFile(ctx, directory, original, destination, node.ProtectionPolicy{})
		if errMove != nil {
			return fmt.Errorf("valheim mod move: %w", errMove)
		}
		moved = append(moved, fileMove{source: destination, target: original})
	}
	t, errTx := m.db.SQLDb.BeginTx(ctx, nil)
	if errTx != nil {
		return fmt.Errorf("valheim mod tx: %w", errTx)
	}
	tx := bob.NewTx(t)
	defer func() {
		logRollbackError(tx.Rollback(context.WithoutCancel(ctx)), "Failed to rollback mod state transaction")
	}()
	if operation == "remove" {
		_, errMod := t.ExecContext(ctx, "DELETE FROM installed_mod WHERE id = ?", mod.ID)
		if errMod != nil {
			return fmt.Errorf("valheim mod mod: %w", errMod)
		}
	} else {
		enabled := int64(0)
		if operation == "enable" {
			enabled = 1
		}
		errUpdate := m.db.UpdateInstalledModInTx(tx, mod, &models.InstalledModSetter{Enabled: omit.From(enabled), UpdatedAt: omit.From(time.Now().UTC())})
		if errUpdate != nil {
			return fmt.Errorf("valheim mod update: %w", errUpdate)
		}
	}
	errCommit := tx.Commit(ctx)
	if errCommit != nil {
		return fmt.Errorf("valheim mod commit: %w", errCommit)
	}
	committed = true
	return nil
}
