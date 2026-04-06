// Package scheduler manages cron-based task scheduling for game servers.
// It wraps gocron v2 to provide runtime CRUD operations on scheduled tasks
// that are persisted in the database.
package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// DB is the subset of db.Connection needed by the scheduler.
type DB interface {
	GetAllEnabledScheduledTasks() ([]*models.ScheduledTask, error)
	GetScheduledTaskByID(id string) (*models.ScheduledTask, error)
	UpdateScheduledTaskLastRun(id string, lastRunAt time.Time, nextRunAt *time.Time) error
	InsertScheduledTaskLog(scheduledTaskID, gameServerID, taskType, status, message string, startedAt time.Time, finishedAt *time.Time) (*models.ScheduledTaskLog, error)
	PruneScheduledTaskLogs(olderThan time.Time, maxPerTask int) (int64, error)
	GetGameServerByID(gameServerID string) (*models.GameServer, error)
}

// ActionsExecutor abstracts the actions layer for game server lifecycle operations.
type ActionsExecutor interface {
	StartGameServer(gameServer *models.GameServer)
	StopGameServer(gameServer *models.GameServer)
}

// SupervisorCommand abstracts a supervisor.Command for reading status and
// sending console input.
type SupervisorCommand interface {
	Status() xylona.Status
	SendInput(input string) error
}

// SupervisorAccessor abstracts the supervisor.Instance for getting commands.
type SupervisorAccessor interface {
	GetCommandByID(commandID string) (SupervisorCommand, error)
}

// Scheduler manages cron-scheduled tasks for game servers.
type Scheduler struct {
	ctx       context.Context
	scheduler gocron.Scheduler
	db        DB
	actions   ActionsExecutor
	super     SupervisorAccessor
	mu        sync.RWMutex
	jobs      map[string]uuid.UUID // scheduled_task.id → gocron job UUID
}

// New creates a Scheduler. Call Start() to begin executing tasks.
func New(ctx context.Context, database DB, actions ActionsExecutor, super SupervisorAccessor) (*Scheduler, error) {
	gs, errNew := gocron.NewScheduler(
		gocron.WithStopTimeout(30 * time.Second),
	)
	if errNew != nil {
		return nil, fmt.Errorf("create gocron scheduler: %w", errNew)
	}

	return &Scheduler{
		ctx:       ctx,
		scheduler: gs,
		db:        database,
		actions:   actions,
		super:     super,
		jobs:      make(map[string]uuid.UUID),
	}, nil
}

// Start loads all enabled tasks from the database, registers them with gocron,
// and starts the scheduler. It also launches a background log pruner.
func (s *Scheduler) Start() error {
	tasks, errLoad := s.db.GetAllEnabledScheduledTasks()
	if errLoad != nil {
		return fmt.Errorf("load enabled scheduled tasks: %w", errLoad)
	}

	for _, task := range tasks {
		errAdd := s.addJob(task)
		if errAdd != nil {
			log.Warn().Err(errAdd).
				Str("task_id", task.ID).
				Str("task_name", task.Name).
				Msg("Skipping scheduled task due to error")
		}
	}

	s.scheduler.Start()

	go s.backgroundLogPruner()

	log.Info().Int("task_count", len(tasks)).Msg("Scheduled task scheduler started")
	return nil
}

// Stop gracefully shuts down the scheduler.
func (s *Scheduler) Stop() error {
	errShutdown := s.scheduler.Shutdown()
	if errShutdown != nil {
		return fmt.Errorf("scheduler shutdown: %w", errShutdown)
	}
	return nil
}

// AddTask registers a new task with the scheduler. The task must already be
// persisted in the database.
func (s *Scheduler) AddTask(task *models.ScheduledTask) error {
	return s.addJob(task)
}

// RemoveTask removes a task from the scheduler by its database ID.
func (s *Scheduler) RemoveTask(taskID string) {
	s.mu.Lock()
	jobUUID, ok := s.jobs[taskID]
	if ok {
		delete(s.jobs, taskID)
	}
	s.mu.Unlock()

	if ok {
		errRemove := s.scheduler.RemoveJob(jobUUID)
		if errRemove != nil {
			log.Warn().Err(errRemove).Str("task_id", taskID).Msg("Failed to remove scheduled job")
		}
	}
}

// UpdateTask removes and re-adds a task to pick up changed schedule or parameters.
func (s *Scheduler) UpdateTask(task *models.ScheduledTask) error {
	s.RemoveTask(task.ID)
	if task.Enabled == 1 {
		return s.addJob(task)
	}
	return nil
}

func (s *Scheduler) addJob(task *models.ScheduledTask) error {
	// Build cron expression with timezone prefix when not UTC.
	cronExpr := task.CronExpression
	if task.Timezone != "" && task.Timezone != "UTC" {
		cronExpr = fmt.Sprintf("CRON_TZ=%s %s", task.Timezone, task.CronExpression)
	}

	taskID := task.ID // capture for closure
	j, errJob := s.scheduler.NewJob(
		gocron.CronJob(cronExpr, false),
		gocron.NewTask(func() {
			s.executeTask(taskID)
		}),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
		gocron.WithTags(taskID),
	)
	if errJob != nil {
		return fmt.Errorf("register cron job for task %s: %w", task.ID, errJob)
	}

	s.mu.Lock()
	s.jobs[task.ID] = j.ID()
	s.mu.Unlock()

	log.Debug().
		Str("task_id", task.ID).
		Str("task_name", task.Name).
		Str("cron", cronExpr).
		Msg("Registered scheduled task")

	return nil
}

// backgroundLogPruner runs daily and removes old execution logs.
func (s *Scheduler) backgroundLogPruner() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
			deleted, errPrune := s.db.PruneScheduledTaskLogs(cutoff, 1000)
			if errPrune != nil {
				log.Error().Err(errPrune).Msg("Failed to prune scheduled task logs")
				continue
			}
			if deleted > 0 {
				log.Info().Int64("deleted", deleted).Msg("Pruned scheduled task logs")
			}
		}
	}
}
