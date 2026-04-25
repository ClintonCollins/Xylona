package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob/dialect/sqlite/sm"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// CreateGameServerBackupParams contains the fields needed to insert a backup row.
type CreateGameServerBackupParams struct {
	GameServerID    string
	NodeID          string
	CreatedBy       string
	TriggerSource   string
	ArchivePath     string
	ArchiveRoot     string
	ArchiveFormat   string
	Status          string
	SizeBytes       int64
	RetentionExempt bool
	ErrorMessage    string
	CreatedAt       time.Time
	CompletedAt     *time.Time
}

// UpdateGameServerBackupResultParams contains the mutable result fields for a backup row.
type UpdateGameServerBackupResultParams struct {
	Status       string
	SizeBytes    int64
	ErrorMessage string
	CompletedAt  *time.Time
}

// CreateGameServerBackup inserts a game server backup catalog row.
func (c *Connection) CreateGameServerBackup(params CreateGameServerBackupParams) (*models.GameServerBackup, error) {
	createdAt := params.CreatedAt.UTC()
	if params.CreatedAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	id := uuid.New().String()
	setter := &models.GameServerBackupSetter{
		ID:              omit.From(id),
		GameServerID:    omit.From(params.GameServerID),
		NodeID:          omit.From(params.NodeID),
		CreatedBy:       setNullableString(params.CreatedBy),
		TriggerSource:   omit.From(params.TriggerSource),
		ArchivePath:     omit.From(params.ArchivePath),
		ArchiveRoot:     omit.From(params.ArchiveRoot),
		ArchiveFormat:   omit.From(params.ArchiveFormat),
		Status:          omit.From(params.Status),
		SizeBytes:       omit.From(params.SizeBytes),
		RetentionExempt: omit.From(params.RetentionExempt),
		ErrorMessage:    setNullableString(params.ErrorMessage),
		CreatedAt:       omit.From(createdAt),
	}

	if params.CompletedAt != nil {
		setter.CompletedAt = omitnull.From(params.CompletedAt.UTC())
	}

	backup, errInsert := models.GameServerBackups.Insert(setter).One(c.ctx, c.DB)
	if errInsert != nil {
		log.Error().Err(errInsert).Str("game_server_id", params.GameServerID).Msg("Error inserting game server backup")
		return nil, fmt.Errorf("create game server backup: %w", errInsert)
	}

	return backup, nil
}

// GetGameServerBackupByID returns a single game server backup by ID.
func (c *Connection) GetGameServerBackupByID(id string) (*models.GameServerBackup, error) {
	backup, errGet := models.FindGameServerBackup(c.ctx, c.DB, id)
	if errGet != nil {
		return nil, fmt.Errorf("get game server backup by ID: %w", errGet)
	}

	return backup, nil
}

// ListGameServerBackupsByGameServerID returns backups for a game server ordered by newest first.
func (c *Connection) ListGameServerBackupsByGameServerID(gameServerID string) ([]*models.GameServerBackup, error) {
	backups, errGet := models.GameServerBackups.Query(
		models.SelectWhere.GameServerBackups.GameServerID.EQ(gameServerID),
		sm.OrderBy(models.GameServerBackups.Columns.CreatedAt).Desc(),
	).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Str("game_server_id", gameServerID).Msg("Error querying game server backups")
		return nil, fmt.Errorf("list game server backups by game server ID: %w", errGet)
	}

	return backups, nil
}

// UpdateGameServerBackupResult updates the completion fields for a backup row.
func (c *Connection) UpdateGameServerBackupResult(id string, params UpdateGameServerBackupResultParams) (*models.GameServerBackup, error) {
	setter := &models.GameServerBackupSetter{
		Status:       omit.From(params.Status),
		SizeBytes:    omit.From(params.SizeBytes),
		ErrorMessage: setNullableString(params.ErrorMessage),
	}

	if params.CompletedAt != nil {
		setter.CompletedAt = omitnull.From(params.CompletedAt.UTC())
	} else {
		setter.CompletedAt = omitnull.FromNull(null.Val[time.Time]{})
	}

	_, errUpdate := models.GameServerBackups.Update(
		models.UpdateWhere.GameServerBackups.ID.EQ(id),
		setter.UpdateMod(),
	).Exec(c.ctx, c.DB)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("id", id).Msg("Error updating game server backup result")
		return nil, fmt.Errorf("update game server backup result: %w", errUpdate)
	}

	return c.GetGameServerBackupByID(id)
}

// UpdateGameServerBackupProgress updates the in-flight size for a pending backup row.
func (c *Connection) UpdateGameServerBackupProgress(id string, sizeBytes int64) (*models.GameServerBackup, error) {
	setter := &models.GameServerBackupSetter{
		Status:       omit.From("pending"),
		SizeBytes:    omit.From(sizeBytes),
		CompletedAt:  omitnull.FromNull(null.Val[time.Time]{}),
		ErrorMessage: omitnull.FromNull(null.Val[string]{}),
	}

	_, errUpdate := models.GameServerBackups.Update(
		models.UpdateWhere.GameServerBackups.ID.EQ(id),
		setter.UpdateMod(),
	).Exec(c.ctx, c.DB)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("id", id).Msg("Error updating game server backup progress")
		return nil, fmt.Errorf("update game server backup progress: %w", errUpdate)
	}

	return c.GetGameServerBackupByID(id)
}

// DeleteGameServerBackup deletes a game server backup row by ID.
func (c *Connection) DeleteGameServerBackup(id string) error {
	_, errDelete := models.GameServerBackups.Delete(
		models.DeleteWhere.GameServerBackups.ID.EQ(id),
	).Exec(c.ctx, c.DB)
	if errDelete != nil {
		log.Error().Err(errDelete).Str("id", id).Msg("Error deleting game server backup")
		return fmt.Errorf("delete game server backup: %w", errDelete)
	}

	return nil
}

// PruneScheduledGameServerBackups returns scheduled completed backups beyond the retention keep count without deleting them.
func (c *Connection) PruneScheduledGameServerBackups(gameServerID string, nodeID string, keepCount int) ([]*models.GameServerBackup, error) {
	// The DB layer enforces a minimum retention floor; higher-level defaults are handled elsewhere.
	if keepCount < 1 {
		keepCount = 1
	}

	backups, errGet := models.GameServerBackups.Query(
		models.SelectWhere.GameServerBackups.GameServerID.EQ(gameServerID),
		models.SelectWhere.GameServerBackups.NodeID.EQ(nodeID),
		models.SelectWhere.GameServerBackups.TriggerSource.EQ("scheduled"),
		models.SelectWhere.GameServerBackups.RetentionExempt.EQ(false),
		models.SelectWhere.GameServerBackups.Status.EQ("completed"),
		sm.OrderBy(models.GameServerBackups.Columns.CreatedAt).Desc(),
	).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Str("game_server_id", gameServerID).Msg("Error selecting scheduled game server backups for pruning")
		return nil, fmt.Errorf("prune scheduled game server backups: %w", errGet)
	}

	if len(backups) <= keepCount {
		return nil, nil
	}

	return backups[keepCount:], nil
}
