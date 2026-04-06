package rpc

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
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
