package rpc

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestListScheduledTasksRedactsConsoleCommandWithoutConsolePermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	assignScheduledTaskRole(t, fixture, "user-other", "role-scheduled-task-reader", permissionScheduledTasks)

	consoleUser := createUserForRPCUserTests(t, fixture, "scheduled-console-reader", false)
	assignScheduledTaskRole(
		t,
		fixture,
		consoleUser.GetId(),
		"role-scheduled-task-console-reader",
		permissionScheduledTasks,
		permissionConsole,
	)

	_, errInsert := fixture.conn.InsertScheduledTask(
		"server-local-1",
		"user-owner",
		"Nightly Console Command",
		"console_command",
		"0 * * * *",
		"UTC",
		"say secret",
		true,
	)
	if errInsert != nil {
		t.Fatalf("InsertScheduledTask() error = %v", errInsert)
	}

	limitedRequest := connect.NewRequest(&xylona.ListScheduledTasksRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, limitedRequest, "user-other")

	limitedResponse, errListLimited := fixture.service.ListScheduledTasks(context.Background(), limitedRequest)
	if errListLimited != nil {
		t.Fatalf("ListScheduledTasks(limited) error = %v", errListLimited)
	}
	if len(limitedResponse.Msg.GetTasks()) != 1 {
		t.Fatalf("ListScheduledTasks(limited) task count = %d, want %d", len(limitedResponse.Msg.GetTasks()), 1)
	}
	if limitedResponse.Msg.GetTasks()[0].ConsoleCommand != nil {
		t.Fatalf("ListScheduledTasks(limited) exposed console command = %q", limitedResponse.Msg.GetTasks()[0].GetConsoleCommand())
	}

	consoleRequest := connect.NewRequest(&xylona.ListScheduledTasksRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, consoleRequest, consoleUser.GetId())

	consoleResponse, errListConsole := fixture.service.ListScheduledTasks(context.Background(), consoleRequest)
	if errListConsole != nil {
		t.Fatalf("ListScheduledTasks(console) error = %v", errListConsole)
	}
	if len(consoleResponse.Msg.GetTasks()) != 1 {
		t.Fatalf("ListScheduledTasks(console) task count = %d, want %d", len(consoleResponse.Msg.GetTasks()), 1)
	}
	if consoleResponse.Msg.GetTasks()[0].ConsoleCommand == nil {
		t.Fatalf("ListScheduledTasks(console) redacted console command")
	}
	if consoleResponse.Msg.GetTasks()[0].GetConsoleCommand() != "say secret" {
		t.Fatalf("ListScheduledTasks(console) console command = %q, want %q", consoleResponse.Msg.GetTasks()[0].GetConsoleCommand(), "say secret")
	}
}

func TestGetScheduledTaskLogsRedactsConsoleMessagesWithoutConsolePermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	assignScheduledTaskRole(t, fixture, "user-other", "role-scheduled-log-reader", permissionScheduledTasks)

	consoleUser := createUserForRPCUserTests(t, fixture, "scheduled-log-console-reader", false)
	assignScheduledTaskRole(
		t,
		fixture,
		consoleUser.GetId(),
		"role-scheduled-log-console-reader",
		permissionScheduledTasks,
		permissionConsole,
	)

	task, errInsert := fixture.conn.InsertScheduledTask(
		"server-local-1",
		"user-owner",
		"Nightly Console Command",
		"console_command",
		"0 * * * *",
		"UTC",
		"say secret",
		true,
	)
	if errInsert != nil {
		t.Fatalf("InsertScheduledTask() error = %v", errInsert)
	}

	finishedAt := time.Now().UTC()
	_, errLog := fixture.conn.InsertScheduledTaskLog(
		task.ID,
		task.GameServerID,
		task.TaskType,
		"success",
		"sent command: say secret",
		finishedAt,
		&finishedAt,
	)
	if errLog != nil {
		t.Fatalf("InsertScheduledTaskLog() error = %v", errLog)
	}

	limitedRequest := connect.NewRequest(&xylona.GetScheduledTaskLogsRequest{
		ScheduledTaskId: task.ID,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, limitedRequest, "user-other")

	limitedResponse, errLogsLimited := fixture.service.GetScheduledTaskLogs(context.Background(), limitedRequest)
	if errLogsLimited != nil {
		t.Fatalf("GetScheduledTaskLogs(limited) error = %v", errLogsLimited)
	}
	if len(limitedResponse.Msg.GetLogs()) != 1 {
		t.Fatalf("GetScheduledTaskLogs(limited) log count = %d, want %d", len(limitedResponse.Msg.GetLogs()), 1)
	}
	if limitedResponse.Msg.GetLogs()[0].Message != nil {
		t.Fatalf("GetScheduledTaskLogs(limited) exposed log message = %q", limitedResponse.Msg.GetLogs()[0].GetMessage())
	}

	consoleRequest := connect.NewRequest(&xylona.GetScheduledTaskLogsRequest{
		ScheduledTaskId: task.ID,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, consoleRequest, consoleUser.GetId())

	consoleResponse, errLogsConsole := fixture.service.GetScheduledTaskLogs(context.Background(), consoleRequest)
	if errLogsConsole != nil {
		t.Fatalf("GetScheduledTaskLogs(console) error = %v", errLogsConsole)
	}
	if len(consoleResponse.Msg.GetLogs()) != 1 {
		t.Fatalf("GetScheduledTaskLogs(console) log count = %d, want %d", len(consoleResponse.Msg.GetLogs()), 1)
	}
	if consoleResponse.Msg.GetLogs()[0].Message == nil {
		t.Fatalf("GetScheduledTaskLogs(console) redacted log message")
	}
	if consoleResponse.Msg.GetLogs()[0].GetMessage() != "sent command: say secret" {
		t.Fatalf("GetScheduledTaskLogs(console) message = %q, want %q", consoleResponse.Msg.GetLogs()[0].GetMessage(), "sent command: say secret")
	}
}

func TestCreateScheduledTaskBackupRequiresBackupPermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	assignScheduledTaskRole(t, fixture, "user-other", "role-scheduled-task-backup-base", permissionScheduledTasks)

	request := connect.NewRequest(&xylona.CreateScheduledTaskRequest{
		GameServerId:   "server-local-1",
		Name:           "Nightly Backup",
		TaskType:       "backup",
		CronExpression: "0 3 * * *",
		Timezone:       "UTC",
		Enabled:        true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-other")

	_, errCreate := fixture.service.CreateScheduledTask(context.Background(), request)
	if errCreate == nil {
		t.Fatal("CreateScheduledTask(backup) error = nil, want permission denied")
	}
	if connect.CodeOf(errCreate) != connect.CodePermissionDenied {
		t.Fatalf("CreateScheduledTask(backup) code = %v, want %v", connect.CodeOf(errCreate), connect.CodePermissionDenied)
	}
	if !strings.Contains(errCreate.Error(), permissionBackup) {
		t.Fatalf("CreateScheduledTask(backup) error = %q, want mention of %q", errCreate.Error(), permissionBackup)
	}
}

func TestCreateScheduledTaskBackupRejectsDisabledBackups(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:             omit.From("server-local-1"),
		BackupsEnabled: omit.From(false),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	request := connect.NewRequest(&xylona.CreateScheduledTaskRequest{
		GameServerId:   "server-local-1",
		Name:           "Nightly Backup",
		TaskType:       "backup",
		CronExpression: "0 3 * * *",
		Timezone:       "UTC",
		Enabled:        true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	_, errCreate := fixture.service.CreateScheduledTask(context.Background(), request)
	if errCreate == nil {
		t.Fatal("CreateScheduledTask(backup disabled) error = nil, want failed precondition")
	}
	if connect.CodeOf(errCreate) != connect.CodeFailedPrecondition {
		t.Fatalf("CreateScheduledTask(backup disabled) code = %v, want %v", connect.CodeOf(errCreate), connect.CodeFailedPrecondition)
	}
	if errCreate.Error() != "failed_precondition: Backups are disabled for this server." {
		t.Fatalf("CreateScheduledTask(backup disabled) error = %q, want exact disabled message", errCreate.Error())
	}
}

func TestCreateScheduledTaskBackupRejectsInvalidBackupDirectory(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	gameServer, errGetServer := fixture.conn.GetGameServerByID("server-local-1")
	if errGetServer != nil {
		t.Fatalf("GetGameServerByID() error = %v", errGetServer)
	}

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From(filepath.Join(gameServer.Directory, "backups")),
		MaxBackups:      omit.From(int64(5)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	request := connect.NewRequest(&xylona.CreateScheduledTaskRequest{
		GameServerId:   "server-local-1",
		Name:           "Nightly Backup",
		TaskType:       "backup",
		CronExpression: "0 3 * * *",
		Timezone:       "UTC",
		Enabled:        true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	_, errCreate := fixture.service.CreateScheduledTask(context.Background(), request)
	if errCreate == nil {
		t.Fatal("CreateScheduledTask(invalid backup directory) error = nil, want failed precondition")
	}
	if connect.CodeOf(errCreate) != connect.CodeFailedPrecondition {
		t.Fatalf("CreateScheduledTask(invalid backup directory) code = %v, want %v", connect.CodeOf(errCreate), connect.CodeFailedPrecondition)
	}
	if errCreate.Error() != "failed_precondition: Backup directory is not valid for this server." {
		t.Fatalf("CreateScheduledTask(invalid backup directory) error = %q, want invalid directory message", errCreate.Error())
	}
}

func TestCreateScheduledTaskBackupSucceeds(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	_, errUpdateServer := fixture.conn.UpdateGameServer(fixture.conn.DB, &models.GameServerSetter{
		ID:              omit.From("server-local-1"),
		BackupsEnabled:  omit.From(true),
		BackupDirectory: omit.From("/srv/xylona-backups"),
		MaxBackups:      omit.From(int64(5)),
	})
	if errUpdateServer != nil {
		t.Fatalf("UpdateGameServer() error = %v", errUpdateServer)
	}

	request := connect.NewRequest(&xylona.CreateScheduledTaskRequest{
		GameServerId:   "server-local-1",
		Name:           "Nightly Backup",
		TaskType:       "backup",
		CronExpression: "0 3 * * *",
		Timezone:       "UTC",
		Enabled:        true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errCreate := fixture.service.CreateScheduledTask(context.Background(), request)
	if errCreate != nil {
		t.Fatalf("CreateScheduledTask(backup) error = %v", errCreate)
	}
	if response.Msg.GetTask() == nil {
		t.Fatal("CreateScheduledTask(backup) returned nil task")
	}
	if response.Msg.GetTask().GetTaskType() != "backup" {
		t.Fatalf("CreateScheduledTask(backup).Task.TaskType = %q, want %q", response.Msg.GetTask().GetTaskType(), "backup")
	}

	storedTask, errGetTask := fixture.conn.GetScheduledTaskByID(response.Msg.GetTask().GetId())
	if errGetTask != nil {
		t.Fatalf("GetScheduledTaskByID() error = %v", errGetTask)
	}
	if storedTask.TaskType != "backup" {
		t.Fatalf("GetScheduledTaskByID().TaskType = %q, want %q", storedTask.TaskType, "backup")
	}
}

func assignScheduledTaskRole(t *testing.T, fixture *rbacRPCFixture, userID string, roleID string, permissionIDs ...string) {
	t.Helper()

	_, errRole := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`INSERT INTO role (id, name, description, is_system) VALUES (?, ?, ?, ?)`,
		roleID, roleID, "scheduled task test role", false,
	)
	if errRole != nil {
		t.Fatalf("failed to insert role %q: %v", roleID, errRole)
	}

	for _, permissionID := range permissionIDs {
		_, errPermission := fixture.conn.SQLDb.ExecContext(
			context.Background(),
			`INSERT INTO role_permission (role_id, permission_id) VALUES (?, ?)`,
			roleID, permissionID,
		)
		if errPermission != nil {
			t.Fatalf("failed to insert role_permission %q for role %q: %v", permissionID, roleID, errPermission)
		}
	}

	errAssign := fixture.conn.CreateUserRoleAssignment(
		"assignment-"+roleID+"-"+userID,
		userID,
		roleID,
		"server-local-1",
		"user-admin",
	)
	if errAssign != nil {
		t.Fatalf("CreateUserRoleAssignment(%q, %q) error = %v", userID, roleID, errAssign)
	}
}
