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

// InsertScheduledTask creates a new scheduled task.
func (c *Connection) InsertScheduledTask(gameServerID, createdBy, name, taskType, cronExpression, timezone, consoleCommand string, enabled bool) (*models.ScheduledTask, error) {
	enabledInt := int64(0)
	if enabled {
		enabledInt = 1
	}

	now := time.Now().UTC()
	id := uuid.New().String()

	setter := &models.ScheduledTaskSetter{
		ID:             omit.From(id),
		GameServerID:   omit.From(gameServerID),
		CreatedBy:      omit.From(createdBy),
		Name:           omit.From(name),
		TaskType:       omit.From(taskType),
		CronExpression: omit.From(cronExpression),
		Timezone:       omit.From(timezone),
		ConsoleCommand: setNullableString(consoleCommand),
		Enabled:        omit.From(enabledInt),
		CreatedAt:      omit.From(now),
		UpdatedAt:      omit.From(now),
	}

	task, errInsert := models.ScheduledTasks.Insert(setter).One(c.ctx, c.DB)
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Error inserting scheduled task")
		return nil, fmt.Errorf("insert scheduled task: %w", errInsert)
	}

	return task, nil
}

// GetScheduledTaskByID returns a single scheduled task by ID.
func (c *Connection) GetScheduledTaskByID(id string) (*models.ScheduledTask, error) {
	task, errGet := models.FindScheduledTask(c.ctx, c.DB, id)
	if errGet != nil {
		return nil, fmt.Errorf("get scheduled task by ID: %w", errGet)
	}
	return task, nil
}

// GetScheduledTasksByGameServerID returns all scheduled tasks for a given game server.
func (c *Connection) GetScheduledTasksByGameServerID(gameServerID string) ([]*models.ScheduledTask, error) {
	tasks, errGet := models.ScheduledTasks.Query(
		models.SelectWhere.ScheduledTasks.GameServerID.EQ(gameServerID),
	).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Str("game_server_id", gameServerID).Msg("Error querying scheduled tasks")
		return nil, fmt.Errorf("get scheduled tasks by game server ID: %w", errGet)
	}
	return tasks, nil
}

// GetAllEnabledScheduledTasks returns all scheduled tasks that are enabled.
func (c *Connection) GetAllEnabledScheduledTasks() ([]*models.ScheduledTask, error) {
	tasks, errGet := models.ScheduledTasks.Query(
		models.SelectWhere.ScheduledTasks.Enabled.EQ(1),
	).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Msg("Error querying enabled scheduled tasks")
		return nil, fmt.Errorf("get all enabled scheduled tasks: %w", errGet)
	}
	return tasks, nil
}

// UpdateScheduledTask updates an existing scheduled task by ID. The ID must be
// set on the setter. Returns the updated task.
func (c *Connection) UpdateScheduledTask(id string, name, taskType, cronExpression, timezone, consoleCommand string, enabled bool) (*models.ScheduledTask, error) {
	enabledInt := int64(0)
	if enabled {
		enabledInt = 1
	}

	now := time.Now().UTC()

	setter := &models.ScheduledTaskSetter{
		Name:           omit.From(name),
		TaskType:       omit.From(taskType),
		CronExpression: omit.From(cronExpression),
		Timezone:       omit.From(timezone),
		ConsoleCommand: setNullableString(consoleCommand),
		Enabled:        omit.From(enabledInt),
		UpdatedAt:      omit.From(now),
	}

	_, errUpdate := models.ScheduledTasks.Update(
		models.UpdateWhere.ScheduledTasks.ID.EQ(id),
		setter.UpdateMod(),
	).Exec(c.ctx, c.DB)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("id", id).Msg("Error updating scheduled task")
		return nil, fmt.Errorf("update scheduled task: %w", errUpdate)
	}

	return c.GetScheduledTaskByID(id)
}

// UpdateScheduledTaskLastRun updates the last_run_at and next_run_at timestamps.
func (c *Connection) UpdateScheduledTaskLastRun(id string, lastRunAt time.Time, nextRunAt *time.Time) error {
	setter := &models.ScheduledTaskSetter{
		LastRunAt: omitnull.From(lastRunAt),
		UpdatedAt: omit.From(time.Now().UTC()),
	}
	if nextRunAt != nil {
		setter.NextRunAt = omitnull.From(*nextRunAt)
	} else {
		setter.NextRunAt = omitnull.FromNull(null.Val[time.Time]{})
	}

	_, errUpdate := models.ScheduledTasks.Update(
		models.UpdateWhere.ScheduledTasks.ID.EQ(id),
		setter.UpdateMod(),
	).Exec(c.ctx, c.DB)
	if errUpdate != nil {
		return fmt.Errorf("update scheduled task last run: %w", errUpdate)
	}

	return nil
}

// DeleteScheduledTask deletes a scheduled task by ID.
func (c *Connection) DeleteScheduledTask(id string) error {
	_, errDelete := models.ScheduledTasks.Delete(
		models.DeleteWhere.ScheduledTasks.ID.EQ(id),
	).Exec(c.ctx, c.DB)
	if errDelete != nil {
		log.Error().Err(errDelete).Str("id", id).Msg("Error deleting scheduled task")
		return fmt.Errorf("delete scheduled task: %w", errDelete)
	}
	return nil
}

// InsertScheduledTaskLog creates a new execution log entry.
func (c *Connection) InsertScheduledTaskLog(scheduledTaskID, gameServerID, taskType, status, message string, startedAt time.Time, finishedAt *time.Time) (*models.ScheduledTaskLog, error) {
	id := uuid.New().String()

	setter := &models.ScheduledTaskLogSetter{
		ID:              omit.From(id),
		ScheduledTaskID: omit.From(scheduledTaskID),
		GameServerID:    omit.From(gameServerID),
		TaskType:        omit.From(taskType),
		Status:          omit.From(status),
		Message:         setNullableString(message),
		StartedAt:       omit.From(startedAt),
		CreatedAt:       omit.From(time.Now().UTC()),
	}

	if finishedAt != nil {
		setter.FinishedAt = omitnull.From(*finishedAt)
	}

	entry, errInsert := models.ScheduledTaskLogs.Insert(setter).One(c.ctx, c.DB)
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Error inserting scheduled task log")
		return nil, fmt.Errorf("insert scheduled task log: %w", errInsert)
	}

	return entry, nil
}

// GetScheduledTaskLogs returns execution logs for a scheduled task, ordered by
// most recent first.
func (c *Connection) GetScheduledTaskLogs(scheduledTaskID string, limit, offset int) ([]*models.ScheduledTaskLog, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	logs, errGet := models.ScheduledTaskLogs.Query(
		models.SelectWhere.ScheduledTaskLogs.ScheduledTaskID.EQ(scheduledTaskID),
		sm.OrderBy(models.ScheduledTaskLogs.Columns.CreatedAt).Desc(),
		sm.Limit(int64(limit)),
		sm.Offset(int64(offset)),
	).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Str("scheduled_task_id", scheduledTaskID).Msg("Error querying scheduled task logs")
		return nil, fmt.Errorf("get scheduled task logs: %w", errGet)
	}

	return logs, nil
}

// GetLatestScheduledTaskLogsByGameServerID returns the most recent execution
// log for each scheduled task belonging to a game server.
func (c *Connection) GetLatestScheduledTaskLogsByGameServerID(gameServerID string) ([]*models.ScheduledTaskLog, error) {
	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`select logs.id,
			logs.scheduled_task_id,
			logs.game_server_id,
			logs.task_type,
			logs.status,
			logs.message,
			logs.started_at,
			logs.finished_at,
			logs.created_at
		 from scheduled_task_log as logs
		 where logs.game_server_id = ?
			and logs.id = (
				select latest.id
				from scheduled_task_log as latest
				where latest.scheduled_task_id = logs.scheduled_task_id
				order by latest.created_at desc, latest.id desc
				limit 1
			)
		 order by logs.created_at desc, logs.id desc`,
		gameServerID,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("get latest scheduled task logs by game server ID: %w", errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("Failed to close rows in GetLatestScheduledTaskLogsByGameServerID")
		}
	}()

	logs := make([]*models.ScheduledTaskLog, 0)
	for rows.Next() {
		entry := &models.ScheduledTaskLog{}
		var message sql.NullString
		var finishedAt sql.NullTime
		errScan := rows.Scan(
			&entry.ID,
			&entry.ScheduledTaskID,
			&entry.GameServerID,
			&entry.TaskType,
			&entry.Status,
			&message,
			&entry.StartedAt,
			&finishedAt,
			&entry.CreatedAt,
		)
		if errScan != nil {
			return nil, fmt.Errorf("scan latest scheduled task log: %w", errScan)
		}
		if message.Valid {
			entry.Message = null.From(message.String)
		}
		if finishedAt.Valid {
			entry.FinishedAt = null.From(finishedAt.Time)
		}
		logs = append(logs, entry)
	}

	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate latest scheduled task logs: %w", errRows)
	}

	return logs, nil
}

// PruneScheduledTaskLogs deletes log entries older than the given threshold and
// trims per-task logs to the given maximum count.
func (c *Connection) PruneScheduledTaskLogs(olderThan time.Time, maxPerTask int) (int64, error) {
	// Phase 1: Delete by age.
	ageDeleted, errDelete := models.ScheduledTaskLogs.Delete(
		models.DeleteWhere.ScheduledTaskLogs.CreatedAt.LT(olderThan),
	).Exec(c.ctx, c.DB)
	if errDelete != nil {
		return 0, fmt.Errorf("prune scheduled task logs by age: %w", errDelete)
	}

	// Phase 2: Trim per-task to maxPerTask rows. Use raw SQL because the
	// correlated subquery is not trivially expressed through the ORM.
	if maxPerTask > 0 {
		trimQuery := `
			DELETE FROM scheduled_task_log
			WHERE id IN (
				SELECT l.id FROM scheduled_task_log l
				WHERE (
					SELECT COUNT(*) FROM scheduled_task_log l2
					WHERE l2.scheduled_task_id = l.scheduled_task_id
					  AND l2.created_at >= l.created_at
				) > ?
			)
		`
		trimResult, errTrim := c.SQLDb.ExecContext(c.ctx, trimQuery, maxPerTask)
		if errTrim != nil {
			return ageDeleted, fmt.Errorf("prune scheduled task logs per-task cap: %w", errTrim)
		}
		trimDeleted, _ := trimResult.RowsAffected()
		ageDeleted += trimDeleted
	}

	return ageDeleted, nil
}
