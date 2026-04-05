// Package modmanager coordinates mod provider downloads, installed-mod state,
// and filesystem operations for game servers.
package modmanager

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"
	"golang.org/x/sync/errgroup"

	"github.com/ClintonCollins/Xylona/db"
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
)

// ModManager coordinates between mod providers, the database, and the filesystem.
type ModManager struct {
	db *db.Connection
}

// New creates a new ModManager.
func New(database *db.Connection) *ModManager {
	return &ModManager{db: database}
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
	errMkdir := os.MkdirAll(targetDir, 0o750)
	if errMkdir != nil {
		return nil, fmt.Errorf("modmanager: create install directory: %w", errMkdir)
	}
	downloaded, errDownload := provider.Download(ctx, sourceID, versionID, targetDir)
	if errDownload != nil {
		return nil, fmt.Errorf("modmanager: download failed: %w", errDownload)
	}
	if len(downloaded) == 0 {
		return nil, ErrNoFilesDownloaded
	}

	details, errDetails := provider.GetModDetails(ctx, sourceID, nil)
	if errDetails != nil {
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

	// Find the primary file hash.
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

	now := time.Now().UTC()
	modID := uuid.NewString()

	t, errTx := m.db.SQLDb.BeginTx(ctx, nil)
	if errTx != nil {
		return nil, fmt.Errorf("modmanager: begin transaction: %w", errTx)
	}
	tx := bob.NewTx(t)

	defer func() {
		_ = tx.Rollback(ctx)
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
		return nil, fmt.Errorf("modmanager: commit transaction: %w", errCommit)
	}

	return mod, nil
}

// Uninstall removes mod files from disk and deletes DB records.
func (m *ModManager) Uninstall(_ context.Context, modID, serverDir string) error {
	mod, errGet := m.db.GetInstalledModByID(modID)
	if errGet != nil {
		return fmt.Errorf("%w: %s: %w", ErrModNotFound, modID, errGet)
	}

	files, errFiles := m.db.GetInstalledModFilesByModID(mod.ID)
	if errFiles != nil {
		return fmt.Errorf("modmanager: get mod files: %w", errFiles)
	}

	for _, f := range files {
		fullPath := filepath.Join(serverDir, f.FilePath)
		errRemove := os.Remove(fullPath)
		if errRemove != nil && !os.IsNotExist(errRemove) {
			log.Warn().Err(errRemove).Str("path", fullPath).Msg("Failed to remove mod file")
		}
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

	// Remove old files.
	oldFiles, errOldFiles := m.db.GetInstalledModFilesByModID(mod.ID)
	if errOldFiles != nil {
		return nil, fmt.Errorf("modmanager: get old files: %w", errOldFiles)
	}
	for _, f := range oldFiles {
		fullPath := filepath.Join(serverDir, f.FilePath)
		errRemove := os.Remove(fullPath)
		if errRemove != nil && !os.IsNotExist(errRemove) {
			log.Warn().Err(errRemove).Str("path", fullPath).Msg("Failed to remove old mod file during update")
		}
	}

	// Determine install path from old file paths.
	installSubdir := ""
	if len(oldFiles) > 0 {
		installSubdir = filepath.Dir(oldFiles[0].FilePath)
	}
	installDir := filepath.Join(serverDir, installSubdir)

	downloaded, errDownload := provider.Download(ctx, mod.SourceID, versionID, installDir)
	if errDownload != nil {
		return nil, fmt.Errorf("modmanager: download update: %w", errDownload)
	}
	if len(downloaded) == 0 {
		return nil, ErrNoFilesDownloaded
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

	// Wrap all DB mutations in a transaction so a partial failure does not leave
	// the database in an inconsistent state.
	t, errTx := m.db.SQLDb.BeginTx(ctx, nil)
	if errTx != nil {
		return nil, fmt.Errorf("modmanager: begin update transaction: %w", errTx)
	}
	tx := bob.NewTx(t)

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// Delete old file records and insert new ones.
	errDeleteFiles := m.db.DeleteInstalledModFilesByModID(tx, mod.ID)
	if errDeleteFiles != nil {
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
		return nil, fmt.Errorf("modmanager: update mod record: %w", errUpdate)
	}

	errCommit := tx.Commit(ctx)
	if errCommit != nil {
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

		_, errUpdate := m.Update(ctx, mod.ID, version.VersionID, serverDir)
		if errUpdate != nil {
			log.Error().Err(errUpdate).Str("mod", mod.ModName).Msg("Failed to auto-update mod")
			statusFn(fmt.Sprintf("Failed to update %s: %s", mod.ModName, errUpdate.Error()))
			continue
		}

		statusFn(fmt.Sprintf("Updated %s to %s", mod.ModName, version.VersionString))
	}

	return nil
}

// Enable moves mod files back from the disabled directory.
func (m *ModManager) Enable(_ context.Context, modID, serverDir, installPath string) error {
	mod, errGet := m.db.GetInstalledModByID(modID)
	if errGet != nil {
		return fmt.Errorf("%w: %s: %w", ErrModNotFound, modID, errGet)
	}

	files, errFiles := m.db.GetInstalledModFilesByModID(mod.ID)
	if errFiles != nil {
		return fmt.Errorf("modmanager: get mod files: %w", errFiles)
	}

	disabledDir := filepath.Join(serverDir, installPath, "disabled")

	for _, f := range files {
		filename := filepath.Base(f.FilePath)
		src := filepath.Join(disabledDir, filename)
		dst := filepath.Join(serverDir, installPath, filename)

		errMove := os.Rename(src, dst)
		if errMove != nil {
			return fmt.Errorf("modmanager: move file from disabled: %w", errMove)
		}
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

// Disable moves mod files to a disabled subdirectory.
func (m *ModManager) Disable(_ context.Context, modID, serverDir, installPath string) error {
	mod, errGet := m.db.GetInstalledModByID(modID)
	if errGet != nil {
		return fmt.Errorf("%w: %s: %w", ErrModNotFound, modID, errGet)
	}

	files, errFiles := m.db.GetInstalledModFilesByModID(mod.ID)
	if errFiles != nil {
		return fmt.Errorf("modmanager: get mod files: %w", errFiles)
	}

	disabledDir := filepath.Join(serverDir, installPath, "disabled")
	errMkdir := os.MkdirAll(disabledDir, 0o750)
	if errMkdir != nil {
		return fmt.Errorf("modmanager: create disabled directory: %w", errMkdir)
	}

	for _, f := range files {
		filename := filepath.Base(f.FilePath)
		src := filepath.Join(serverDir, installPath, filename)
		dst := filepath.Join(disabledDir, filename)

		errMove := os.Rename(src, dst)
		if errMove != nil {
			return fmt.Errorf("modmanager: move file to disabled: %w", errMove)
		}
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
