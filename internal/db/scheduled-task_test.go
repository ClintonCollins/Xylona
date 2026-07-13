package db

import (
	"testing"
	"time"
)

func TestInsertScheduledTask(t *testing.T) {
	conn := newRBACMigratedConnection(t, "st-insert.sqlite")
	seedRBACFixture(t, conn)

	task, errInsert := conn.InsertScheduledTask(
		"server-local-1", "user-owner", "Daily Restart",
		"restart", "0 3 * * *", "UTC", "", true,
	)
	if errInsert != nil {
		t.Fatalf("InsertScheduledTask error = %v", errInsert)
	}
	if task.ID == "" {
		t.Fatal("expected task to have an ID")
	}
	if task.Name != "Daily Restart" {
		t.Errorf("expected Name = %q, got %q", "Daily Restart", task.Name)
	}
	if task.TaskType != "restart" {
		t.Errorf("expected TaskType = %q, got %q", "restart", task.TaskType)
	}
	if task.Enabled != 1 {
		t.Errorf("expected Enabled = 1, got %d", task.Enabled)
	}
}

func TestInsertScheduledTask_ConsoleCommand(t *testing.T) {
	conn := newRBACMigratedConnection(t, "st-console.sqlite")
	seedRBACFixture(t, conn)

	task, errInsert := conn.InsertScheduledTask(
		"server-local-1", "user-owner", "Say Hello",
		"console_command", "*/30 * * * *", "America/New_York", "say Hello everyone!", true,
	)
	if errInsert != nil {
		t.Fatalf("InsertScheduledTask error = %v", errInsert)
	}

	cmd, cmdSet := task.ConsoleCommand.Get()
	if !cmdSet || cmd != "say Hello everyone!" {
		t.Errorf("expected ConsoleCommand = %q, got %q (set=%v)", "say Hello everyone!", cmd, cmdSet)
	}
	if task.Timezone != "America/New_York" {
		t.Errorf("expected Timezone = %q, got %q", "America/New_York", task.Timezone)
	}
}

func TestInsertScheduledTask_DuplicateNamePerServer(t *testing.T) {
	conn := newRBACMigratedConnection(t, "st-dup.sqlite")
	seedRBACFixture(t, conn)

	_, errFirst := conn.InsertScheduledTask(
		"server-local-1", "user-owner", "Restart",
		"restart", "0 3 * * *", "UTC", "", true,
	)
	if errFirst != nil {
		t.Fatalf("first insert error = %v", errFirst)
	}

	_, errSecond := conn.InsertScheduledTask(
		"server-local-1", "user-owner", "Restart",
		"restart", "0 4 * * *", "UTC", "", true,
	)
	if errSecond == nil {
		t.Fatal("expected duplicate (game_server_id, name) insert to fail")
	}
}

func TestGetScheduledTasksByGameServerID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "st-list.sqlite")
	seedRBACFixture(t, conn)

	_, _ = conn.InsertScheduledTask("server-local-1", "user-owner", "Task 1", "restart", "0 3 * * *", "UTC", "", true)
	_, _ = conn.InsertScheduledTask("server-local-1", "user-owner", "Task 2", "console_command", "0 * * * *", "UTC", "say hi", false)

	tasks, errGet := conn.GetScheduledTasksByGameServerID("server-local-1")
	if errGet != nil {
		t.Fatalf("GetScheduledTasksByGameServerID error = %v", errGet)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestGetAllEnabledScheduledTasks(t *testing.T) {
	conn := newRBACMigratedConnection(t, "st-enabled.sqlite")
	seedRBACFixture(t, conn)

	_, _ = conn.InsertScheduledTask("server-local-1", "user-owner", "Enabled", "restart", "0 3 * * *", "UTC", "", true)
	_, _ = conn.InsertScheduledTask("server-local-1", "user-owner", "Disabled", "restart", "0 4 * * *", "UTC", "", false)

	tasks, errGet := conn.GetAllEnabledScheduledTasks()
	if errGet != nil {
		t.Fatalf("GetAllEnabledScheduledTasks error = %v", errGet)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 enabled task, got %d", len(tasks))
	}
	if tasks[0].Name != "Enabled" {
		t.Errorf("expected enabled task name = %q, got %q", "Enabled", tasks[0].Name)
	}
}

func TestUpdateScheduledTask(t *testing.T) {
	conn := newRBACMigratedConnection(t, "st-update.sqlite")
	seedRBACFixture(t, conn)

	task, _ := conn.InsertScheduledTask("server-local-1", "user-owner", "Original", "restart", "0 3 * * *", "UTC", "", true)

	updated, errUpdate := conn.UpdateScheduledTask(
		task.ID, "Renamed", "console_command", "0 */6 * * *", "Europe/Berlin", "save-all", false,
	)
	if errUpdate != nil {
		t.Fatalf("UpdateScheduledTask error = %v", errUpdate)
	}
	if updated.Name != "Renamed" {
		t.Errorf("expected Name = %q, got %q", "Renamed", updated.Name)
	}
	if updated.TaskType != "console_command" {
		t.Errorf("expected TaskType = %q, got %q", "console_command", updated.TaskType)
	}
	if updated.Enabled != 0 {
		t.Errorf("expected Enabled = 0, got %d", updated.Enabled)
	}
}

func TestDeleteScheduledTask(t *testing.T) {
	conn := newRBACMigratedConnection(t, "st-delete.sqlite")
	seedRBACFixture(t, conn)

	task, _ := conn.InsertScheduledTask("server-local-1", "user-owner", "ToDelete", "restart", "0 3 * * *", "UTC", "", true)

	errDelete := conn.DeleteScheduledTask(task.ID)
	if errDelete != nil {
		t.Fatalf("DeleteScheduledTask error = %v", errDelete)
	}

	_, errGet := conn.GetScheduledTaskByID(task.ID)
	if errGet == nil {
		t.Fatal("expected task to be deleted")
	}
}

func TestScheduledTaskCascadeDelete(t *testing.T) {
	conn := newRBACMigratedConnection(t, "st-cascade.sqlite")
	seedRBACFixture(t, conn)

	task, _ := conn.InsertScheduledTask("server-local-1", "user-owner", "WithLogs", "restart", "0 3 * * *", "UTC", "", true)

	now := time.Now().UTC()
	_, _ = conn.InsertScheduledTaskLog(task.ID, "server-local-1", "restart", "success", "ok", now, &now)
	_, _ = conn.InsertScheduledTaskLog(task.ID, "server-local-1", "restart", "skipped", "offline", now, &now)

	// Verify logs exist.
	logs, _ := conn.GetScheduledTaskLogs(task.ID, 10, 0)
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs before delete, got %d", len(logs))
	}

	// Delete the task — logs should cascade.
	errDelete := conn.DeleteScheduledTask(task.ID)
	if errDelete != nil {
		t.Fatalf("DeleteScheduledTask error = %v", errDelete)
	}

	logsAfter, _ := conn.GetScheduledTaskLogs(task.ID, 10, 0)
	if len(logsAfter) != 0 {
		t.Errorf("expected 0 logs after cascade delete, got %d", len(logsAfter))
	}
}

func TestInsertScheduledTaskLog(t *testing.T) {
	conn := newRBACMigratedConnection(t, "st-log.sqlite")
	seedRBACFixture(t, conn)

	task, _ := conn.InsertScheduledTask("server-local-1", "user-owner", "LogTest", "restart", "0 3 * * *", "UTC", "", true)

	started := time.Now().UTC()
	finished := started.Add(5 * time.Second)

	entry, errInsert := conn.InsertScheduledTaskLog(
		task.ID, "server-local-1", "restart", "success", "restarted OK", started, &finished,
	)
	if errInsert != nil {
		t.Fatalf("InsertScheduledTaskLog error = %v", errInsert)
	}
	if entry.Status != "success" {
		t.Errorf("expected Status = %q, got %q", "success", entry.Status)
	}
}

func TestGetLatestScheduledTaskLogsByGameServerID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "st-latest-logs.sqlite")
	seedRBACFixture(t, conn)

	firstTask, errFirstTask := conn.InsertScheduledTask(
		"server-local-1", "user-owner", "Restart", "restart", "0 3 * * *", "UTC", "", true,
	)
	if errFirstTask != nil {
		t.Fatalf("InsertScheduledTask(first) error = %v", errFirstTask)
	}
	secondTask, errSecondTask := conn.InsertScheduledTask(
		"server-local-1", "user-owner", "Backup", "backup", "0 4 * * *", "UTC", "", true,
	)
	if errSecondTask != nil {
		t.Fatalf("InsertScheduledTask(second) error = %v", errSecondTask)
	}

	oldTime := time.Date(2026, time.July, 13, 1, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(time.Hour)
	oldLog, errOldLog := conn.InsertScheduledTaskLog(
		firstTask.ID, firstTask.GameServerID, firstTask.TaskType, "failed", "old", oldTime, &oldTime,
	)
	if errOldLog != nil {
		t.Fatalf("InsertScheduledTaskLog(old) error = %v", errOldLog)
	}
	newLog, errNewLog := conn.InsertScheduledTaskLog(
		firstTask.ID, firstTask.GameServerID, firstTask.TaskType, "success", "new", newTime, &newTime,
	)
	if errNewLog != nil {
		t.Fatalf("InsertScheduledTaskLog(new) error = %v", errNewLog)
	}
	secondLog, errSecondLog := conn.InsertScheduledTaskLog(
		secondTask.ID, secondTask.GameServerID, secondTask.TaskType, "skipped", "offline", newTime, &newTime,
	)
	if errSecondLog != nil {
		t.Fatalf("InsertScheduledTaskLog(second task) error = %v", errSecondLog)
	}

	createdAtByID := map[string]time.Time{
		oldLog.ID:    oldTime,
		newLog.ID:    newTime,
		secondLog.ID: newTime,
	}
	for logID, createdAt := range createdAtByID {
		_, errUpdate := conn.SQLDb.ExecContext(
			conn.ctx,
			"update scheduled_task_log set created_at = ? where id = ?",
			createdAt,
			logID,
		)
		if errUpdate != nil {
			t.Fatalf("update log %q created_at: %v", logID, errUpdate)
		}
	}

	logs, errGet := conn.GetLatestScheduledTaskLogsByGameServerID("server-local-1")
	if errGet != nil {
		t.Fatalf("GetLatestScheduledTaskLogsByGameServerID() error = %v", errGet)
	}
	if len(logs) != 2 {
		t.Fatalf("GetLatestScheduledTaskLogsByGameServerID() count = %d, want %d", len(logs), 2)
	}

	logsByTaskID := make(map[string]string, len(logs))
	for _, entry := range logs {
		logsByTaskID[entry.ScheduledTaskID] = entry.Status
	}
	if logsByTaskID[firstTask.ID] != "success" {
		t.Fatalf("latest first task status = %v, want %q", logsByTaskID[firstTask.ID], "success")
	}
	if logsByTaskID[secondTask.ID] != "skipped" {
		t.Fatalf("latest second task status = %v, want %q", logsByTaskID[secondTask.ID], "skipped")
	}
}

func TestUpdateScheduledTaskLastRun(t *testing.T) {
	conn := newRBACMigratedConnection(t, "st-lastrun.sqlite")
	seedRBACFixture(t, conn)

	task, _ := conn.InsertScheduledTask("server-local-1", "user-owner", "LastRun", "restart", "0 3 * * *", "UTC", "", true)

	now := time.Now().UTC()
	errUpdate := conn.UpdateScheduledTaskLastRun(task.ID, now, nil)
	if errUpdate != nil {
		t.Fatalf("UpdateScheduledTaskLastRun error = %v", errUpdate)
	}

	updated, _ := conn.GetScheduledTaskByID(task.ID)
	lastRun, lastRunSet := updated.LastRunAt.Get()
	if !lastRunSet {
		t.Fatal("expected LastRunAt to be set")
	}
	if lastRun.Sub(now).Abs() > time.Second {
		t.Errorf("expected LastRunAt ~ %v, got %v", now, lastRun)
	}
}

func TestPruneScheduledTaskLogs(t *testing.T) {
	conn := newRBACMigratedConnection(t, "st-prune.sqlite")
	seedRBACFixture(t, conn)

	task, _ := conn.InsertScheduledTask("server-local-1", "user-owner", "PruneTest", "restart", "0 3 * * *", "UTC", "", true)

	old := time.Now().UTC().Add(-100 * 24 * time.Hour)
	recent := time.Now().UTC()

	// Insert old log with backdated created_at using raw SQL (InsertScheduledTaskLog always sets created_at to now).
	_, errOld := conn.SQLDb.ExecContext(conn.ctx,
		`INSERT INTO scheduled_task_log (id, scheduled_task_id, game_server_id, task_type, status, message, started_at, finished_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"log-old", task.ID, "server-local-1", "restart", "success", "old", old, old, old,
	)
	if errOld != nil {
		t.Fatalf("insert old log: %v", errOld)
	}

	// Insert recent log normally.
	_, _ = conn.InsertScheduledTaskLog(task.ID, "server-local-1", "restart", "success", "recent", recent, &recent)

	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
	deleted, errPrune := conn.PruneScheduledTaskLogs(cutoff, 0)
	if errPrune != nil {
		t.Fatalf("PruneScheduledTaskLogs error = %v", errPrune)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	remaining, _ := conn.GetScheduledTaskLogs(task.ID, 10, 0)
	if len(remaining) != 1 {
		t.Errorf("expected 1 remaining log, got %d", len(remaining))
	}
}
