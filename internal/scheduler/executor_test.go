package scheduler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	controlleractions "github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
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

func (db *executeTaskDBFake) DeleteExpiredJoinTokens(time.Time) (int64, error) {
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

type executeTaskActionsFake struct {
	stopErr       error
	currentStatus xylona.Status
}

func (a *executeTaskActionsFake) StartGameServer(*models.GameServer) (*controlleractions.StartGameServerResult, error) {
	return &controlleractions.StartGameServerResult{Started: true}, nil
}

func (a *executeTaskActionsFake) StopGameServer(context.Context, *models.GameServer) error {
	return a.stopErr
}

func (a *executeTaskActionsFake) CurrentStatus(*models.GameServer) xylona.Status {
	return a.currentStatus
}

func (a *executeTaskActionsFake) SendConsoleInput(*models.GameServer, string) error {
	return errors.New("unexpected SendConsoleInput in actions fake")
}

type executeTaskBackupFake struct {
	calls  []*models.GameServer
	backup *models.GameServerBackup
	err    error
}

func (b *executeTaskBackupFake) CreateScheduledBackup(gameServer *models.GameServer) (*models.GameServerBackup, error) {
	b.calls = append(b.calls, gameServer)
	return b.backup, b.err
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
			Enabled: true,
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

func TestExecuteBackupReportsCapabilityFailures(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus string
	}{
		{
			name:       "unsupported platform is skipped",
			err:        controlleractions.ErrBackupsUnsupported,
			wantStatus: statusSkipped,
		},
		{
			name:       "unavailable node is failed",
			err:        controlleractions.ErrBackupCapabilityUnavailable,
			wantStatus: statusFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			task := &models.ScheduledTask{GameServerID: "server-1"}
			database := &executeTaskDBFake{
				gameServer: &models.GameServer{ID: "server-1", NodeID: "node-1"},
				node:       &models.Node{ID: "node-1", Enabled: true},
			}
			scheduler := &Scheduler{
				db:     database,
				backup: &executeTaskBackupFake{err: test.err},
			}

			status, _ := scheduler.executeBackup(task)
			if status != test.wantStatus {
				t.Fatalf("executeBackup() status = %q, want %q", status, test.wantStatus)
			}
		})
	}
}

func TestExecuteRestartFailsWhenStopFails(t *testing.T) {
	gameServer := &models.GameServer{ID: "server-1"}
	scheduler := &Scheduler{
		ctx: context.Background(),
		db: &executeTaskDBFake{
			gameServer: gameServer,
		},
		actions: &executeTaskActionsFake{
			currentStatus: xylona.Status_ONLINE,
			stopErr:       errors.New("remote node unavailable"),
		},
	}

	status, message := scheduler.executeRestart(&models.ScheduledTask{GameServerID: gameServer.ID})
	if status != statusFailed {
		t.Fatalf("executeRestart() status = %q, want %q", status, statusFailed)
	}
	if message != "failed to stop server: remote node unavailable" {
		t.Fatalf("executeRestart() message = %q, want stop failure", message)
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
		node:       &models.Node{ID: "node-2", Enabled: true},
	}

	s := &Scheduler{
		ctx:     context.Background(),
		db:      db,
		actions: &executeTaskActionsFake{},
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
		node:       &models.Node{ID: "node-3", Enabled: true},
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

// Remote-node backup support is deferred; the executor currently assumes the
// game server lives on the controller's embedded node. Add coverage here when
// that invariant is revisited.
