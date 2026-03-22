package db

import (
	"database/sql"
	"errors"

	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/sqlite"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// InsertInstalledMod inserts a new installed mod record.
func (c *Connection) InsertInstalledMod(exec bob.Executor, setter *models.InstalledModSetter) (*models.InstalledMod, error) {
	mod, errInsert := models.InstalledMods.Insert(setter).One(c.ctx, c.DB)
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Error inserting installed mod")
		return nil, errInsert
	}
	return mod, nil
}

// GetInstalledModByID fetches a single installed mod by ID.
func (c *Connection) GetInstalledModByID(id string) (*models.InstalledMod, error) {
	mod, errGet := models.InstalledMods.Query(models.SelectWhere.InstalledMods.ID.EQ(id)).One(c.ctx, c.DB)
	if errGet != nil {
		if !errors.Is(errGet, sql.ErrNoRows) {
			log.Error().Err(errGet).Msg("Error querying installed mod")
		}
		return nil, errGet
	}
	return mod, nil
}

// GetInstalledModsByGameServerID fetches all installed mods for a game server.
func (c *Connection) GetInstalledModsByGameServerID(gameServerID string) ([]*models.InstalledMod, error) {
	mods, errGet := models.InstalledMods.Query(models.SelectWhere.InstalledMods.GameServerID.EQ(gameServerID)).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Msg("Error querying installed mods by game server ID")
		return nil, errGet
	}
	return mods, nil
}

// UpdateInstalledMod updates an installed mod record.
func (c *Connection) UpdateInstalledMod(exec bob.Executor, mod *models.InstalledMod, setter *models.InstalledModSetter) (*models.InstalledMod, error) {
	errUpdate := mod.Update(c.ctx, exec, setter)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Msg("Error updating installed mod")
		return nil, errUpdate
	}
	updated, errGet := c.GetInstalledModByID(mod.ID)
	if errGet != nil {
		log.Error().Err(errGet).Msg("Error getting updated installed mod")
		return nil, errGet
	}
	return updated, nil
}

// DeleteInstalledModByID deletes an installed mod by ID.
func (c *Connection) DeleteInstalledModByID(id string) error {
	mods := models.InstalledModSlice{&models.InstalledMod{ID: id}}
	return mods.DeleteAll(c.ctx, c.DB)
}

// InsertInstalledModFile inserts a file record for an installed mod.
func (c *Connection) InsertInstalledModFile(exec bob.Executor, setter *models.InstalledModFileSetter) (*models.InstalledModFile, error) {
	file, errInsert := models.InstalledModFiles.Insert(setter).One(c.ctx, c.DB)
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Error inserting installed mod file")
		return nil, errInsert
	}
	return file, nil
}

// GetInstalledModFilesByModID fetches all files for an installed mod.
func (c *Connection) GetInstalledModFilesByModID(modID string) ([]*models.InstalledModFile, error) {
	files, errGet := models.InstalledModFiles.Query(models.SelectWhere.InstalledModFiles.InstalledModID.EQ(modID)).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Msg("Error querying installed mod files")
		return nil, errGet
	}
	return files, nil
}

// DeleteInstalledModFilesByModID deletes all file records for an installed mod.
func (c *Connection) DeleteInstalledModFilesByModID(modID string) error {
	_, errExec := sqlite.RawQuery(
		`DELETE FROM installed_mod_file WHERE installed_mod_id = ?`,
		modID,
	).Exec(c.ctx, c.DB)
	if errExec != nil {
		log.Error().Err(errExec).Msg("Error deleting installed mod files")
		return errExec
	}
	return nil
}
