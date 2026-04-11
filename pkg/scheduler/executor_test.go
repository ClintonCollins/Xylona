package scheduler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ClintonCollins/Xylona/sql/models"
)

type executeTaskDBFake struct {
	task       *models.ScheduledTask
	gameServer *models.GameServer
	node       *models.Node

	logCalls []scheduledTaskLogCall

	lastRunID   string
	lastRunAt   time.Time
	nextRunAt   *time.Time
	lastRunSeen bool
}

type scheduledTaskLogCall struct {
	scheduledTaskID string
	gameServerID    string
	taskType        string
	status          string
	message         string
	startedAt       time.Time
	finishedAt      *time.Time
}

func (db *executeTaskDBFake) GetAllEnabledScheduledTasks() ([]*models.ScheduledTask, error) {
	return nil, nil
}

func (db *executeTaskDBFake) GetScheduledTaskByID(id string) (*models.ScheduledTask, error) {
	if db.task == nil {
		return nil, fmt.Errorf("task %s not found", id)
	}
	return db.task, nil
}

func (db *executeTaskDBFake) UpdateScheduledTaskLastRun(id string, lastRunAt time.Time, nextRunAt *time.Time) error {
	db.lastRunID = id
	db.lastRunAt = lastRunAt
	db.nextRunAt = nextRunAt
	db.lastRunSeen = true
	return nil
}

func (db *executeTaskDBFake) InsertScheduledTaskLog(
	scheduledTaskID,
	gameServerID,
	taskType,
	status,
	message string,
	startedAt time.Time,
	finishedAt *time.Time,
) (*models.ScheduledTaskLog, error) {
	db.logCalls = append(db.logCalls, scheduledTaskLogCall{
		scheduledTaskID: scheduledTaskID,
		gameServerID:    gameServerID,
		taskType:        taskType,
		status:          status,
		message:         message,
		startedAt:       startedAt,
		finishedAt:      finishedAt,
	})

	return &models.ScheduledTaskLog{ID: "log-1"}, nil
}

func (db *executeTaskDBFake) PruneScheduledTaskLogs(time.Time, int) (int64, error) {
	return 0, nil
}

func (db *executeTaskDBFake) PruneExpiredUserSessions(time.Time) (int64, error) {
	return 0, nil
}

func (db *executeTaskDBFake) GetGameServerByID(_ string) (*models.GameServer, error) {
	if db.gameServer == nil {
		return nil, errors.New("game server not configured")
	}
	return db.gameServer, nil
}

func (db *executeTaskDBFake) GetNodeByID(_ string) (*models.Node, error) {
	if db.node == nil {
		return nil, errors.New("node not configured")
	}
	return db.node, nil
}

type executeTaskActionsFake struct{}

func (a *executeTaskActionsFake) StartGameServer(*models.GameServer) {}

func (a *executeTaskActionsFake) StopGameServer(*models.GameServer) {}

type executeTaskBackupFake struct {
	calls  []*models.GameServer
	backup *models.GameServerBackup
	err    error
}

func (b *executeTaskBackupFake) CreateScheduledBackup(gameServer *models.GameServer) (*models.GameServerBackup, error) {
	b.calls = append(b.calls, gameServer)
	return b.backup, b.err
}

type executeTaskSupervisorFake struct{}

func (s *executeTaskSupervisorFake) GetCommandByID(string) (SupervisorCommand, error) {
	return nil, errors.New("unexpected supervisor access")
}

func TestExecuteTaskRunsBackupExecutor(t *testing.T) {
	now := time.Now().UTC()
	task := &models.ScheduledTask{
		ID:             "task-1",
		GameServerID:   "server-1",
		Name:           "Nightly Backup",
		TaskType:       "backup",
		CronExpression: "0 2 * * *",
		Timezone:       "UTC",
		Enabled:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	gameServer := &models.GameServer{
		ID:              "server-1",
		Name:            "Server One",
		Directory:       "C:/servers/server-1",
		BackupsEnabled:  true,
		BackupDirectory: "C:/servers/backups",
		MaxBackups:      5,
		NodeID:          "node-1",
		Status:          "online",
	}
	db := &executeTaskDBFake{
		task:       task,
		gameServer: gameServer,
		node: &models.Node{
			ID:      "node-1",
			IsLocal: true,
		},
	}
	backupExecutor := &executeTaskBackupFake{
		backup: &models.GameServerBackup{ID: "backup-1"},
	}

	s := &Scheduler{
		ctx:     context.Background(),
		db:      db,
		actions: &executeTaskActionsFake{},
		backup:  backupExecutor,
		super:   &executeTaskSupervisorFake{},
		jobs:    map[string]uuid.UUID{},
	}

	s.executeTask(task.ID)

	if len(backupExecutor.calls) != 1 {
		t.Fatalf("backup executor call count = %d, want 1", len(backupExecutor.calls))
	}
	if backupExecutor.calls[0] != gameServer {
		t.Fatalf("backup executor gameServer = %#v, want %#v", backupExecutor.calls[0], gameServer)
	}
	if len(db.logCalls) != 1 {
		t.Fatalf("scheduled task log call count = %d, want 1", len(db.logCalls))
	}
	logCall := db.logCalls[0]
	if logCall.scheduledTaskID != task.ID {
		t.Fatalf("log scheduledTaskID = %q, want %q", logCall.scheduledTaskID, task.ID)
	}
	if logCall.gameServerID != task.GameServerID {
		t.Fatalf("log gameServerID = %q, want %q", logCall.gameServerID, task.GameServerID)
	}
	if logCall.taskType != "backup" {
		t.Fatalf("log taskType = %q, want %q", logCall.taskType, "backup")
	}
	if logCall.status != statusSuccess {
		t.Fatalf("log status = %q, want %q", logCall.status, statusSuccess)
	}
	if logCall.message != "scheduled backup created" {
		t.Fatalf("log message = %q, want %q", logCall.message, "scheduled backup created")
	}
	if logCall.startedAt.IsZero() {
		t.Fatal("log startedAt was not set")
	}
	if logCall.finishedAt == nil {
		t.Fatal("log finishedAt was not set")
	}
	if !db.lastRunSeen {
		t.Fatal("expected last_run_at update to be written")
	}
	if db.lastRunID != task.ID {
		t.Fatalf("lastRunID = %q, want %q", db.lastRunID, task.ID)
	}
	if db.lastRunAt.IsZero() {
		t.Fatal("lastRunAt was not set")
	}
	if db.nextRunAt != nil {
		t.Fatalf("nextRunAt = %#v, want nil", db.nextRunAt)
	}
}

func TestExecuteTaskBackupWithoutExecutorFailsClearly(t *testing.T) {
	now := time.Now().UTC()
	task := &models.ScheduledTask{
		ID:             "task-2",
		GameServerID:   "server-2",
		Name:           "Nightly Backup",
		TaskType:       "backup",
		CronExpression: "0 2 * * *",
		Timezone:       "UTC",
		Enabled:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	db := &executeTaskDBFake{
		task:       task,
		gameServer: &models.GameServer{ID: "server-2", Name: "Server Two"},
		node:       &models.Node{ID: "node-2", IsLocal: true},
	}

	s := &Scheduler{
		ctx:     context.Background(),
		db:      db,
		actions: &executeTaskActionsFake{},
		super:   &executeTaskSupervisorFake{},
		jobs:    map[string]uuid.UUID{},
	}

	s.executeTask(task.ID)

	if len(db.logCalls) != 1 {
		t.Fatalf("scheduled task log call count = %d, want 1", len(db.logCalls))
	}
	if db.logCalls[0].status != statusFailed {
		t.Fatalf("log status = %q, want %q", db.logCalls[0].status, statusFailed)
	}
	if db.logCalls[0].message != "backup executor is not configured" {
		t.Fatalf("log message = %q, want %q", db.logCalls[0].message, "backup executor is not configured")
	}
	if !db.lastRunSeen {
		t.Fatal("expected last_run_at update to be written")
	}
}

func TestExecuteTaskBackupPartialSuccessReportsPostBackupFailure(t *testing.T) {
	now := time.Now().UTC()
	task := &models.ScheduledTask{
		ID:             "task-3",
		GameServerID:   "server-3",
		Name:           "Nightly Backup",
		TaskType:       "backup",
		CronExpression: "0 2 * * *",
		Timezone:       "UTC",
		Enabled:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	db := &executeTaskDBFake{
		task:       task,
		gameServer: &models.GameServer{ID: "server-3", Name: "Server Three", NodeID: "node-3"},
		node:       &models.Node{ID: "node-3", IsLocal: true},
	}
	backupExecutor := &executeTaskBackupFake{
		backup: &models.GameServerBackup{ID: "backup-3"},
		err:    errors.New("prune failed"),
	}

	s := &Scheduler{
		ctx:     context.Background(),
		db:      db,
		actions: &executeTaskActionsFake{},
		backup:  backupExecutor,
		super:   &executeTaskSupervisorFake{},
		jobs:    map[string]uuid.UUID{},
	}

	s.executeTask(task.ID)

	if len(backupExecutor.calls) != 1 {
		t.Fatalf("backup executor call count = %d, want 1", len(backupExecutor.calls))
	}
	if len(db.logCalls) != 1 {
		t.Fatalf("scheduled task log call count = %d, want 1", len(db.logCalls))
	}
	if db.logCalls[0].status != statusFailed {
		t.Fatalf("log status = %q, want %q", db.logCalls[0].status, statusFailed)
	}
	wantMessage := "scheduled backup backup-3 created, but post-backup work failed: prune failed"
	if db.logCalls[0].message != wantMessage {
		t.Fatalf("log message = %q, want %q", db.logCalls[0].message, wantMessage)
	}
	if !db.lastRunSeen {
		t.Fatal("expected last_run_at update to be written")
	}
}

func TestExecuteTaskBackupFailsForNonLocalServer(t *testing.T) {
	now := time.Now().UTC()
	task := &models.ScheduledTask{
		ID:             "task-4",
		GameServerID:   "server-4",
		Name:           "Nightly Backup",
		TaskType:       "backup",
		CronExpression: "0 2 * * *",
		Timezone:       "UTC",
		Enabled:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	db := &executeTaskDBFake{
		task: task,
		gameServer: &models.GameServer{
			ID:             "server-4",
			Name:           "Server Four",
			BackupsEnabled: true,
			NodeID:         "node-remote",
		},
		node: &models.Node{
			ID:      "node-remote",
			IsLocal: false,
		},
	}
	backupExecutor := &executeTaskBackupFake{
		backup: &models.GameServerBackup{ID: "backup-4"},
	}

	s := &Scheduler{
		ctx:     context.Background(),
		db:      db,
		actions: &executeTaskActionsFake{},
		backup:  backupExecutor,
		super:   &executeTaskSupervisorFake{},
		jobs:    map[string]uuid.UUID{},
	}

	s.executeTask(task.ID)

	if len(backupExecutor.calls) != 0 {
		t.Fatalf("backup executor call count = %d, want 0", len(backupExecutor.calls))
	}
	if len(db.logCalls) != 1 {
		t.Fatalf("scheduled task log call count = %d, want 1", len(db.logCalls))
	}
	if db.logCalls[0].status != statusFailed {
		t.Fatalf("log status = %q, want %q", db.logCalls[0].status, statusFailed)
	}
	if db.logCalls[0].message != "scheduled backups only run on local servers" {
		t.Fatalf("log message = %q, want %q", db.logCalls[0].message, "scheduled backups only run on local servers")
	}
	if !db.lastRunSeen {
		t.Fatal("expected last_run_at update to be written")
	}
}
