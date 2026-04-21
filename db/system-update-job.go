package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrSystemUpdateJobActive is returned when a target already has a non-terminal update job.
var ErrSystemUpdateJobActive = errors.New("system update job already active")

// SystemUpdateJob is a persisted controller/node update job.
type SystemUpdateJob struct {
	ID                string
	Component         string
	NodeID            sql.NullString
	CurrentVersion    string
	TargetVersion     string
	Status            string
	Phase             string
	ProgressPercent   int32
	Error             sql.NullString
	ArtifactName      sql.NullString
	ArtifactSHA256    sql.NullString
	RequestedByUserID sql.NullString
	CreatedAt         time.Time
	UpdatedAt         time.Time
	StartedAt         sql.NullTime
	CompletedAt       sql.NullTime
}

// ActiveSystemUpdateJobError identifies the active job that blocked a new one.
type ActiveSystemUpdateJobError struct {
	JobID     string
	Component string
	NodeID    string
}

func (e *ActiveSystemUpdateJobError) Error() string {
	target := e.Component
	if e.NodeID != "" {
		target += "/" + e.NodeID
	}
	return fmt.Sprintf("%s: %s has active job %s", ErrSystemUpdateJobActive, target, e.JobID)
}

// Is reports whether target is ErrSystemUpdateJobActive.
func (e *ActiveSystemUpdateJobError) Is(target error) bool {
	return target == ErrSystemUpdateJobActive
}

// SystemUpdateJobEvent is one progress event for a persisted update job.
type SystemUpdateJobEvent struct {
	ID              string
	JobID           string
	Status          string
	Phase           string
	ProgressPercent int32
	Message         sql.NullString
	Error           sql.NullString
	CreatedAt       time.Time
}

// CreateSystemUpdateJobParams contains required job creation fields.
type CreateSystemUpdateJobParams struct {
	Component         string
	NodeID            string
	CurrentVersion    string
	TargetVersion     string
	Status            string
	Phase             string
	ArtifactName      string
	ArtifactSHA256    string
	RequestedByUserID string
}

// UpdateSystemUpdateJobParams contains mutable job state.
type UpdateSystemUpdateJobParams struct {
	Status          string
	Phase           string
	ProgressPercent int32
	Error           string
	Completed       bool
}

// CreateSystemUpdateJob inserts a new job row.
func (c *Connection) CreateSystemUpdateJob(params CreateSystemUpdateJobParams) (*SystemUpdateJob, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`INSERT INTO system_update_job (
			id, component, node_id, current_version, target_version, status, phase,
			progress_percent, artifact_name, artifact_sha256, requested_by_user_id,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?)`,
		id,
		params.Component,
		nullString(params.NodeID),
		params.CurrentVersion,
		params.TargetVersion,
		params.Status,
		params.Phase,
		nullString(params.ArtifactName),
		nullString(params.ArtifactSHA256),
		nullString(params.RequestedByUserID),
		now,
		now,
	)
	if errExec != nil {
		if isUniqueConstraintError(errExec) {
			activeJob, errActive := c.GetActiveSystemUpdateJob(params.Component, params.NodeID)
			if errActive == nil {
				return nil, &ActiveSystemUpdateJobError{
					JobID:     activeJob.ID,
					Component: params.Component,
					NodeID:    params.NodeID,
				}
			}
			if errors.Is(errActive, sql.ErrNoRows) {
				return nil, fmt.Errorf("create system update job: %w", ErrSystemUpdateJobActive)
			}
			return nil, fmt.Errorf("find active system update job after conflict: %w", errActive)
		}
		return nil, fmt.Errorf("create system update job: %w", errExec)
	}
	return c.GetSystemUpdateJob(id)
}

// UpdateSystemUpdateJobState updates job status, phase, progress, and error.
func (c *Connection) UpdateSystemUpdateJobState(jobID string, params UpdateSystemUpdateJobParams) (*SystemUpdateJob, error) {
	now := time.Now().UTC()
	completedFlag := 0
	completedAt := sql.NullTime{}
	if params.Completed {
		completedFlag = 1
		completedAt = sql.NullTime{Time: now, Valid: true}
	}
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`UPDATE system_update_job
		 SET status = ?,
		     phase = ?,
		     progress_percent = ?,
		     error = ?,
		     updated_at = ?,
		     started_at = CASE WHEN started_at IS NULL THEN ? ELSE started_at END,
		     completed_at = CASE WHEN ? = 1 THEN ? ELSE completed_at END
		 WHERE id = ?`,
		params.Status,
		params.Phase,
		params.ProgressPercent,
		nullString(params.Error),
		now,
		now,
		completedFlag,
		completedAt,
		jobID,
	)
	if errExec != nil {
		return nil, fmt.Errorf("update system update job: %w", errExec)
	}
	return c.GetSystemUpdateJob(jobID)
}

// AddSystemUpdateJobEvent appends a job event.
func (c *Connection) AddSystemUpdateJobEvent(jobID string, status string, phase string, progress int32, message string, errorMessage string) (*SystemUpdateJobEvent, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`INSERT INTO system_update_job_event (
			id, job_id, status, phase, progress_percent, message, error, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id,
		jobID,
		status,
		phase,
		progress,
		nullString(message),
		nullString(errorMessage),
		now,
	)
	if errExec != nil {
		return nil, fmt.Errorf("add system update job event: %w", errExec)
	}
	return c.GetSystemUpdateJobEvent(id)
}

// GetSystemUpdateJob fetches one update job.
func (c *Connection) GetSystemUpdateJob(jobID string) (*SystemUpdateJob, error) {
	row := c.SQLDb.QueryRowContext(
		c.ctx,
		`SELECT id, component, node_id, current_version, target_version, status, phase,
		        progress_percent, error, artifact_name, artifact_sha256, requested_by_user_id,
		        created_at, updated_at, started_at, completed_at
		   FROM system_update_job
		  WHERE id = ?`,
		jobID,
	)
	return scanSystemUpdateJob(row)
}

// ListSystemUpdateJobs returns recent jobs.
func (c *Connection) ListSystemUpdateJobs(limit int, offset int) ([]*SystemUpdateJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`SELECT id, component, node_id, current_version, target_version, status, phase,
		        progress_percent, error, artifact_name, artifact_sha256, requested_by_user_id,
		        created_at, updated_at, started_at, completed_at
		   FROM system_update_job
		  ORDER BY created_at DESC
		  LIMIT ? OFFSET ?`,
		limit,
		offset,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("list system update jobs: %w", errQuery)
	}
	defer func() {
		_ = rows.Close()
	}()

	var jobs []*SystemUpdateJob
	for rows.Next() {
		job, errScan := scanSystemUpdateJob(rows)
		if errScan != nil {
			return nil, errScan
		}
		jobs = append(jobs, job)
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate system update jobs: %w", errRows)
	}
	return jobs, nil
}

// ListActiveSystemUpdateJobs returns all non-terminal update jobs.
func (c *Connection) ListActiveSystemUpdateJobs() ([]*SystemUpdateJob, error) {
	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`SELECT id, component, node_id, current_version, target_version, status, phase,
		        progress_percent, error, artifact_name, artifact_sha256, requested_by_user_id,
		        created_at, updated_at, started_at, completed_at
		   FROM system_update_job
		  WHERE status NOT IN ('succeeded', 'failed')
		  ORDER BY created_at ASC`,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("list active system update jobs: %w", errQuery)
	}
	defer func() {
		_ = rows.Close()
	}()

	var jobs []*SystemUpdateJob
	for rows.Next() {
		job, errScan := scanSystemUpdateJob(rows)
		if errScan != nil {
			return nil, errScan
		}
		jobs = append(jobs, job)
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate active system update jobs: %w", errRows)
	}
	return jobs, nil
}

// GetActiveSystemUpdateJob fetches the non-terminal update job for a target.
func (c *Connection) GetActiveSystemUpdateJob(component string, nodeID string) (*SystemUpdateJob, error) {
	row := c.SQLDb.QueryRowContext(
		c.ctx,
		`SELECT id, component, node_id, current_version, target_version, status, phase,
		        progress_percent, error, artifact_name, artifact_sha256, requested_by_user_id,
		        created_at, updated_at, started_at, completed_at
		   FROM system_update_job
		  WHERE component = ?
		    AND COALESCE(node_id, '') = ?
		    AND status NOT IN ('succeeded', 'failed')
		  ORDER BY created_at DESC
		  LIMIT 1`,
		component,
		nodeID,
	)
	return scanSystemUpdateJob(row)
}

// GetSystemUpdateJobEvents returns all events for a job.
func (c *Connection) GetSystemUpdateJobEvents(jobID string) ([]*SystemUpdateJobEvent, error) {
	rows, errQuery := c.SQLDb.QueryContext(
		c.ctx,
		`SELECT id, job_id, status, phase, progress_percent, message, error, created_at
		   FROM system_update_job_event
		  WHERE job_id = ?
		  ORDER BY created_at ASC`,
		jobID,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("list system update job events: %w", errQuery)
	}
	defer func() {
		_ = rows.Close()
	}()

	var events []*SystemUpdateJobEvent
	for rows.Next() {
		event, errScan := scanSystemUpdateJobEvent(rows)
		if errScan != nil {
			return nil, errScan
		}
		events = append(events, event)
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate system update job events: %w", errRows)
	}
	return events, nil
}

// GetSystemUpdateJobEvent fetches one update job event.
func (c *Connection) GetSystemUpdateJobEvent(eventID string) (*SystemUpdateJobEvent, error) {
	row := c.SQLDb.QueryRowContext(
		c.ctx,
		`SELECT id, job_id, status, phase, progress_percent, message, error, created_at
		   FROM system_update_job_event
		  WHERE id = ?`,
		eventID,
	)
	return scanSystemUpdateJobEvent(row)
}

type systemUpdateJobScanner interface {
	Scan(dest ...any) error
}

func scanSystemUpdateJob(scanner systemUpdateJobScanner) (*SystemUpdateJob, error) {
	job := &SystemUpdateJob{}
	errScan := scanner.Scan(
		&job.ID,
		&job.Component,
		&job.NodeID,
		&job.CurrentVersion,
		&job.TargetVersion,
		&job.Status,
		&job.Phase,
		&job.ProgressPercent,
		&job.Error,
		&job.ArtifactName,
		&job.ArtifactSHA256,
		&job.RequestedByUserID,
		&job.CreatedAt,
		&job.UpdatedAt,
		&job.StartedAt,
		&job.CompletedAt,
	)
	if errScan != nil {
		return nil, fmt.Errorf("scan system update job: %w", errScan)
	}
	return job, nil
}

func scanSystemUpdateJobEvent(scanner systemUpdateJobScanner) (*SystemUpdateJobEvent, error) {
	event := &SystemUpdateJobEvent{}
	errScan := scanner.Scan(
		&event.ID,
		&event.JobID,
		&event.Status,
		&event.Phase,
		&event.ProgressPercent,
		&event.Message,
		&event.Error,
		&event.CreatedAt,
	)
	if errScan != nil {
		return nil, fmt.Errorf("scan system update job event: %w", errScan)
	}
	return event, nil
}

func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}
