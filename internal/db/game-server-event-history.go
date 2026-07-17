package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// GameServerOperation identifies a durable game-server operation.
type GameServerOperation string

// Supported game-server operations.
const (
	GameServerOperationBackup GameServerOperation = "backup"
	GameServerOperationUpdate GameServerOperation = "update"
)

// GameServerOperationPhase identifies the operation stage represented by an event.
type GameServerOperationPhase string

// Supported game-server operation phases.
const (
	GameServerOperationPhasePreparing   GameServerOperationPhase = "preparing"
	GameServerOperationPhaseArchiving   GameServerOperationPhase = "archiving"
	GameServerOperationPhaseStopping    GameServerOperationPhase = "stopping"
	GameServerOperationPhaseBackingUp   GameServerOperationPhase = "backing_up"
	GameServerOperationPhaseUpdating    GameServerOperationPhase = "updating"
	GameServerOperationPhaseRestarting  GameServerOperationPhase = "restarting"
	GameServerOperationPhaseRollingBack GameServerOperationPhase = "rolling_back"
	GameServerOperationPhaseComplete    GameServerOperationPhase = "complete"
)

// GameServerOperationOutcome identifies the terminal result of an operation.
type GameServerOperationOutcome string

// Supported game-server operation outcomes.
const (
	GameServerOperationOutcomeSucceeded GameServerOperationOutcome = "succeeded"
	GameServerOperationOutcomeFailed    GameServerOperationOutcome = "failed"
	GameServerOperationOutcomeCancelled GameServerOperationOutcome = "cancelled"
)

// GameServerOperationSource identifies the authoritative operation trigger.
type GameServerOperationSource string

// Supported game-server operation sources.
const (
	GameServerOperationSourceManual     GameServerOperationSource = "manual"
	GameServerOperationSourceScheduled  GameServerOperationSource = "scheduled"
	GameServerOperationSourceController GameServerOperationSource = "controller"
)

// GameServerLifecycleEvent is a durable process lifecycle transition.
type GameServerLifecycleEvent struct {
	ID                 string
	GameServerID       string
	NodeID             string
	ExecutionID        string
	TransitionSequence uint64
	PreviousStatus     string
	Status             string
	IntentionalStop    bool
	ExitCode           *int
	ObservedAt         time.Time
}

// InsertGameServerLifecycleEventParams contains a lifecycle transition to persist.
type InsertGameServerLifecycleEventParams struct {
	ID                 string
	GameServerID       string
	NodeID             string
	ExecutionID        string
	TransitionSequence uint64
	PreviousStatus     string
	Status             string
	IntentionalStop    bool
	ExitCode           *int
	ObservedAt         time.Time
}

// GameServerOperationEvent is a durable backup or update result.
type GameServerOperationEvent struct {
	ID             string
	GameServerID   string
	Operation      GameServerOperation
	Phase          GameServerOperationPhase
	Outcome        GameServerOperationOutcome
	StartedAt      time.Time
	CompletedAt    *time.Time
	DurationMS     *int64
	BytesProcessed *int64
	Source         GameServerOperationSource
}

// InsertGameServerOperationEventParams contains an operation result to persist.
type InsertGameServerOperationEventParams struct {
	ID             string
	GameServerID   string
	Operation      GameServerOperation
	Phase          GameServerOperationPhase
	Outcome        GameServerOperationOutcome
	StartedAt      time.Time
	CompletedAt    *time.Time
	DurationMS     *int64
	BytesProcessed *int64
	Source         GameServerOperationSource
}

// InsertGameServerLifecycleEvent persists a lifecycle transition. Duplicate
// correlated transitions are ignored so replayed node events remain idempotent.
func (c *Connection) InsertGameServerLifecycleEvent(ctx context.Context, params InsertGameServerLifecycleEventParams) (bool, error) {
	if c == nil || c.SQLDb == nil {
		return false, errors.New("insert game server lifecycle event: database is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	errValidate := validateLifecycleEvent(params)
	if errValidate != nil {
		return false, errValidate
	}

	id := strings.TrimSpace(params.ID)
	if id == "" {
		id = uuid.NewString()
	}

	result, errInsert := c.SQLDb.ExecContext(
		ctx,
		`insert into game_server_lifecycle_event
		 (id, game_server_id, node_id, execution_id, transition_sequence, previous_status, status, intentional_stop, exit_code, observed_at)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 on conflict(node_id, game_server_id, execution_id, transition_sequence) do nothing`,
		id,
		params.GameServerID,
		params.NodeID,
		params.ExecutionID,
		params.TransitionSequence,
		params.PreviousStatus,
		params.Status,
		params.IntentionalStop,
		nullableInt(params.ExitCode),
		params.ObservedAt.UTC(),
	)
	if errInsert != nil {
		return false, fmt.Errorf("insert game server lifecycle event: %w", errInsert)
	}

	rowsAffected, errRowsAffected := result.RowsAffected()
	if errRowsAffected != nil {
		return false, fmt.Errorf("insert game server lifecycle event rows affected: %w", errRowsAffected)
	}
	return rowsAffected == 1, nil
}

// GetGameServerLifecycleEvents returns transitions in chronological order.
func (c *Connection) GetGameServerLifecycleEvents(ctx context.Context, gameServerID string, start time.Time, end time.Time) ([]GameServerLifecycleEvent, error) {
	if c == nil || c.SQLDb == nil {
		return nil, errors.New("get game server lifecycle events: database is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	errRange := validateEventRange(gameServerID, start, end)
	if errRange != nil {
		return nil, fmt.Errorf("get game server lifecycle events: %w", errRange)
	}

	rows, errQuery := c.SQLDb.QueryContext(
		ctx,
		`select id, game_server_id, node_id, execution_id, transition_sequence,
		 previous_status, status, intentional_stop, exit_code, observed_at
		 from game_server_lifecycle_event
		 where game_server_id = ? and observed_at >= ? and observed_at <= ?
		 order by observed_at asc, transition_sequence asc, id asc`,
		gameServerID,
		start.UTC(),
		end.UTC(),
	)
	if errQuery != nil {
		return nil, fmt.Errorf("get game server lifecycle events: %w", errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			log.Error().Err(errClose).Msg("Failed to close game server lifecycle event rows")
		}
	}()

	events := make([]GameServerLifecycleEvent, 0)
	for rows.Next() {
		var event GameServerLifecycleEvent
		var exitCode sql.NullInt64
		errScan := rows.Scan(
			&event.ID,
			&event.GameServerID,
			&event.NodeID,
			&event.ExecutionID,
			&event.TransitionSequence,
			&event.PreviousStatus,
			&event.Status,
			&event.IntentionalStop,
			&exitCode,
			&event.ObservedAt,
		)
		if errScan != nil {
			return nil, fmt.Errorf("get game server lifecycle events: scan row: %w", errScan)
		}
		if exitCode.Valid {
			exitCodeValue := int(exitCode.Int64)
			event.ExitCode = &exitCodeValue
		}
		events = append(events, event)
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("get game server lifecycle events: iterate rows: %w", errRows)
	}
	return events, nil
}

// InsertGameServerOperationEvent persists a bounded operation result.
func (c *Connection) InsertGameServerOperationEvent(ctx context.Context, params InsertGameServerOperationEventParams) error {
	if c == nil || c.SQLDb == nil {
		return errors.New("insert game server operation event: database is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	errValidate := validateOperationEvent(params)
	if errValidate != nil {
		return errValidate
	}

	id := strings.TrimSpace(params.ID)
	if id == "" {
		id = uuid.NewString()
	}
	_, errInsert := c.SQLDb.ExecContext(
		ctx,
		`insert into game_server_operation_event
		 (id, game_server_id, operation, phase, outcome, started_at, completed_at, duration_ms, bytes_processed, source)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id,
		params.GameServerID,
		params.Operation,
		nullableString(string(params.Phase)),
		params.Outcome,
		params.StartedAt.UTC(),
		nullableTime(params.CompletedAt),
		nullableInt64(params.DurationMS),
		nullableInt64(params.BytesProcessed),
		params.Source,
	)
	if errInsert != nil {
		return fmt.Errorf("insert game server operation event: %w", errInsert)
	}
	return nil
}

// GetGameServerOperationEvents returns operation results in chronological order.
func (c *Connection) GetGameServerOperationEvents(ctx context.Context, gameServerID string, start time.Time, end time.Time) ([]GameServerOperationEvent, error) {
	if c == nil || c.SQLDb == nil {
		return nil, errors.New("get game server operation events: database is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	errRange := validateEventRange(gameServerID, start, end)
	if errRange != nil {
		return nil, fmt.Errorf("get game server operation events: %w", errRange)
	}

	rows, errQuery := c.SQLDb.QueryContext(
		ctx,
		`select id, game_server_id, operation, phase, outcome, started_at,
		 completed_at, duration_ms, bytes_processed, source
		 from game_server_operation_event
		 where game_server_id = ? and started_at <= ?
		   and (completed_at is null or completed_at >= ?)
		 order by started_at asc, id asc`,
		gameServerID,
		end.UTC(),
		start.UTC(),
	)
	if errQuery != nil {
		return nil, fmt.Errorf("get game server operation events: %w", errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			log.Error().Err(errClose).Msg("Failed to close game server operation event rows")
		}
	}()

	events := make([]GameServerOperationEvent, 0)
	for rows.Next() {
		var event GameServerOperationEvent
		var phase sql.NullString
		var completedAt sql.NullTime
		var durationMS sql.NullInt64
		var bytesProcessed sql.NullInt64
		errScan := rows.Scan(
			&event.ID,
			&event.GameServerID,
			&event.Operation,
			&phase,
			&event.Outcome,
			&event.StartedAt,
			&completedAt,
			&durationMS,
			&bytesProcessed,
			&event.Source,
		)
		if errScan != nil {
			return nil, fmt.Errorf("get game server operation events: scan row: %w", errScan)
		}
		if phase.Valid {
			event.Phase = GameServerOperationPhase(phase.String)
		}
		if completedAt.Valid {
			completedAtValue := completedAt.Time
			event.CompletedAt = &completedAtValue
		}
		if durationMS.Valid {
			durationValue := durationMS.Int64
			event.DurationMS = &durationValue
		}
		if bytesProcessed.Valid {
			bytesValue := bytesProcessed.Int64
			event.BytesProcessed = &bytesValue
		}
		events = append(events, event)
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("get game server operation events: iterate rows: %w", errRows)
	}
	return events, nil
}

// DeleteGameServerLifecycleEventsOlderThan removes lifecycle events outside
// the configured metrics-history retention window.
func (c *Connection) DeleteGameServerLifecycleEventsOlderThan(olderThan time.Time) (int64, error) {
	result, errExec := c.SQLDb.ExecContext(c.ctx, `delete from game_server_lifecycle_event where observed_at < ?`, olderThan.UTC())
	if errExec != nil {
		return 0, fmt.Errorf("delete old game server lifecycle events: %w", errExec)
	}
	rowsAffected, errRowsAffected := result.RowsAffected()
	if errRowsAffected != nil {
		return 0, fmt.Errorf("delete old game server lifecycle events rows affected: %w", errRowsAffected)
	}
	return rowsAffected, nil
}

// DeleteGameServerOperationEventsOlderThan removes terminal operation events
// outside the configured metrics-history retention window.
func (c *Connection) DeleteGameServerOperationEventsOlderThan(olderThan time.Time) (int64, error) {
	result, errExec := c.SQLDb.ExecContext(c.ctx, `delete from game_server_operation_event where coalesce(completed_at, started_at) < ?`, olderThan.UTC())
	if errExec != nil {
		return 0, fmt.Errorf("delete old game server operation events: %w", errExec)
	}
	rowsAffected, errRowsAffected := result.RowsAffected()
	if errRowsAffected != nil {
		return 0, fmt.Errorf("delete old game server operation events rows affected: %w", errRowsAffected)
	}
	return rowsAffected, nil
}

func validateLifecycleEvent(params InsertGameServerLifecycleEventParams) error {
	if strings.TrimSpace(params.GameServerID) == "" {
		return errors.New("insert game server lifecycle event: game server ID is required")
	}
	if strings.TrimSpace(params.NodeID) == "" {
		return errors.New("insert game server lifecycle event: node ID is required")
	}
	if strings.TrimSpace(params.ExecutionID) == "" {
		return errors.New("insert game server lifecycle event: execution ID is required")
	}
	if params.TransitionSequence > math.MaxInt64 {
		return errors.New("insert game server lifecycle event: transition sequence exceeds SQLite integer range")
	}
	if strings.TrimSpace(params.PreviousStatus) == "" || strings.TrimSpace(params.Status) == "" {
		return errors.New("insert game server lifecycle event: previous and current status are required")
	}
	if params.ObservedAt.IsZero() {
		return errors.New("insert game server lifecycle event: observed time is required")
	}
	return nil
}

func validateOperationEvent(params InsertGameServerOperationEventParams) error {
	if strings.TrimSpace(params.GameServerID) == "" {
		return errors.New("insert game server operation event: game server ID is required")
	}
	if !validOperation(params.Operation) {
		return fmt.Errorf("insert game server operation event: unsupported operation %q", params.Operation)
	}
	if params.Phase != "" && !validOperationPhase(params.Phase) {
		return fmt.Errorf("insert game server operation event: unsupported phase %q", params.Phase)
	}
	if !validOperationOutcome(params.Outcome) {
		return fmt.Errorf("insert game server operation event: unsupported outcome %q", params.Outcome)
	}
	if !validOperationSource(params.Source) {
		return fmt.Errorf("insert game server operation event: unsupported source %q", params.Source)
	}
	if params.StartedAt.IsZero() {
		return errors.New("insert game server operation event: start time is required")
	}
	if params.CompletedAt != nil && params.CompletedAt.Before(params.StartedAt) {
		return errors.New("insert game server operation event: completion precedes start")
	}
	if params.DurationMS != nil && *params.DurationMS < 0 {
		return errors.New("insert game server operation event: duration must not be negative")
	}
	if params.BytesProcessed != nil && *params.BytesProcessed < 0 {
		return errors.New("insert game server operation event: bytes processed must not be negative")
	}
	return nil
}

func validateEventRange(gameServerID string, start time.Time, end time.Time) error {
	if strings.TrimSpace(gameServerID) == "" {
		return errors.New("game server ID is required")
	}
	if start.IsZero() || end.IsZero() {
		return errors.New("start and end times are required")
	}
	if end.Before(start) {
		return errors.New("end time precedes start time")
	}
	return nil
}

func validOperation(operation GameServerOperation) bool {
	return operation == GameServerOperationBackup || operation == GameServerOperationUpdate
}

func validOperationPhase(phase GameServerOperationPhase) bool {
	switch phase {
	case GameServerOperationPhasePreparing,
		GameServerOperationPhaseArchiving,
		GameServerOperationPhaseStopping,
		GameServerOperationPhaseBackingUp,
		GameServerOperationPhaseUpdating,
		GameServerOperationPhaseRestarting,
		GameServerOperationPhaseRollingBack,
		GameServerOperationPhaseComplete:
		return true
	default:
		return false
	}
}

func validOperationOutcome(outcome GameServerOperationOutcome) bool {
	return outcome == GameServerOperationOutcomeSucceeded ||
		outcome == GameServerOperationOutcomeFailed ||
		outcome == GameServerOperationOutcomeCancelled
}

func validOperationSource(source GameServerOperationSource) bool {
	return source == GameServerOperationSourceManual ||
		source == GameServerOperationSourceScheduled ||
		source == GameServerOperationSourceController
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}
