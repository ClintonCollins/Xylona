package rpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/controller/authz"
	"github.com/ClintonCollins/Xylona/internal/scheduler"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	permissionScheduledTasks = "game_server.scheduled_tasks"
	permissionConsole        = "game_server.console"
)

// scheduledTaskToProto converts a DB model scheduled task to its protobuf
// representation.
func scheduledTaskToProto(task *models.ScheduledTask, includeConsoleCommand bool) *xylona.ScheduledTask {
	proto := &xylona.ScheduledTask{
		Id:             task.ID,
		GameServerId:   task.GameServerID,
		CreatedBy:      task.CreatedBy,
		Name:           task.Name,
		TaskType:       task.TaskType,
		CronExpression: task.CronExpression,
		Timezone:       task.Timezone,
		Enabled:        task.Enabled != 0,
		CreatedAt:      timestamppb.New(task.CreatedAt),
		UpdatedAt:      timestamppb.New(task.UpdatedAt),
	}

	consoleCommand, consoleCommandSet := task.ConsoleCommand.Get()
	if consoleCommandSet && includeConsoleCommand {
		proto.ConsoleCommand = &consoleCommand
	}

	lastRunAt, lastRunAtSet := task.LastRunAt.Get()
	if lastRunAtSet {
		proto.LastRunAt = timestamppb.New(lastRunAt)
	}

	nextRunAt, nextRunAtSet := task.NextRunAt.Get()
	if nextRunAtSet {
		proto.NextRunAt = timestamppb.New(nextRunAt)
	}

	return proto
}

// scheduledTaskLogToProto converts a DB model task log to protobuf.
func scheduledTaskLogToProto(entry *models.ScheduledTaskLog, includeMessage bool) *xylona.ScheduledTaskLog {
	proto := &xylona.ScheduledTaskLog{
		Id:              entry.ID,
		ScheduledTaskId: entry.ScheduledTaskID,
		GameServerId:    entry.GameServerID,
		TaskType:        entry.TaskType,
		Status:          entry.Status,
		StartedAt:       timestamppb.New(entry.StartedAt),
		CreatedAt:       timestamppb.New(entry.CreatedAt),
	}

	message, messageSet := entry.Message.Get()
	if messageSet && includeMessage {
		proto.Message = &message
	}

	finishedAt, finishedAtSet := entry.FinishedAt.Get()
	if finishedAtSet {
		proto.FinishedAt = timestamppb.New(finishedAt)
	}

	return proto
}

func (xs *XylonaService) canReadConsoleTaskDetails(user *models.User, gameServer *models.GameServer) (bool, error) {
	allowed, errPermission := authz.HasPermission(xs.db, user, gameServer.ID, gameServer.UserID, permissionConsole)
	if errPermission != nil {
		log.Error().
			Err(errPermission).
			Str("server_id", gameServer.ID).
			Str("permission_id", permissionConsole).
			Str("user_id", user.ID).
			Msg("failed to check console permission for scheduled task read")
		return false, connect.NewError(connect.CodeInternal, errors.New("failed to check permissions"))
	}

	return allowed, nil
}

// requiredPermissionsForTaskType returns the additional permission required for
// a given task type. This prevents privilege escalation: users need both
// game_server.scheduled_tasks AND the action-specific permission.
func requiredPermissionsForTaskType(taskType string) []string {
	switch taskType {
	case "restart":
		return []string{"game_server.restart"}
	case "console_command":
		return []string{"game_server.console"}
	case "backup":
		return []string{permissionBackup}
	default:
		return nil
	}
}

// validateScheduledTaskInput checks common input fields. It returns a
// user-facing error if anything is invalid.
func validateScheduledTaskInput(name, taskType, cronExpression, timezone, consoleCommand string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if taskType != "restart" && taskType != "console_command" && taskType != "backup" {
		return errors.New("task_type must be 'restart', 'console_command', or 'backup'")
	}
	if cronExpression == "" {
		return errors.New("cron_expression is required")
	}
	if taskType == "console_command" && consoleCommand == "" {
		return errors.New("console_command is required when task_type is 'console_command'")
	}

	// Validate timezone. Callers normalise empty timezone to "UTC" before
	// calling this function.
	_, errTZ := time.LoadLocation(timezone)
	if errTZ != nil {
		return fmt.Errorf("invalid timezone: %s", timezone)
	}

	// Validate cron expression by attempting a trial parse via the scheduler
	// helper. We use a simple heuristic: 5 space-separated fields.
	fields := 0
	inSpace := true
	for _, c := range cronExpression {
		if c == ' ' || c == '\t' {
			inSpace = true
		} else if inSpace {
			fields++
			inSpace = false
		}
	}
	if fields != 5 {
		return errors.New("cron_expression must have exactly 5 fields (minute hour day month weekday)")
	}

	return nil
}

// ListScheduledTasks returns all scheduled tasks for a game server.
func (xs *XylonaService) ListScheduledTasks(
	_ context.Context,
	request *connect.Request[xylona.ListScheduledTasksRequest],
) (*connect.Response[xylona.ListScheduledTasksResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	serverID := request.Msg.GetGameServerId()
	if serverID == "" {
		return nil, invalidArg("game_server_id is required")
	}

	gameServer, errGS := xs.db.GetGameServerByID(serverID)
	if errGS != nil {
		return nil, notFoundErr()
	}

	errPerm := xs.ensureLocalServerPermission(user, gameServer, permissionScheduledTasks)
	if errPerm != nil {
		return nil, errPerm
	}

	canReadConsoleDetails, errConsolePerm := xs.canReadConsoleTaskDetails(user, gameServer)
	if errConsolePerm != nil {
		return nil, errConsolePerm
	}

	tasks, errGet := xs.db.GetScheduledTasksByGameServerID(serverID)
	if errGet != nil {
		log.Error().Err(errGet).Str("game_server_id", serverID).Msg("Failed to list scheduled tasks")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list scheduled tasks"))
	}

	resp := &xylona.ListScheduledTasksResponse{
		Tasks: make([]*xylona.ScheduledTask, 0, len(tasks)),
	}
	for _, t := range tasks {
		includeConsoleCommand := t.TaskType != "console_command" || canReadConsoleDetails
		resp.Tasks = append(resp.Tasks, scheduledTaskToProto(t, includeConsoleCommand))
	}

	return connect.NewResponse(resp), nil
}

// CreateScheduledTask creates a new scheduled task for a game server.
func (xs *XylonaService) CreateScheduledTask(
	ctx context.Context,
	request *connect.Request[xylona.CreateScheduledTaskRequest],
) (*connect.Response[xylona.CreateScheduledTaskResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	serverID := request.Msg.GetGameServerId()
	if serverID == "" {
		return nil, invalidArg("game_server_id is required")
	}

	gameServer, errGS := xs.db.GetGameServerByID(serverID)
	if errGS != nil {
		return nil, notFoundErr()
	}

	// Check base scheduled_tasks permission.
	errPerm := xs.ensureLocalServerPermission(user, gameServer, permissionScheduledTasks)
	if errPerm != nil {
		return nil, errPerm
	}

	taskType := request.Msg.GetTaskType()
	consoleCommand := request.Msg.GetConsoleCommand()
	timezone := request.Msg.GetTimezone()
	if timezone == "" {
		timezone = "UTC"
	}

	// Check action-specific permission to prevent privilege escalation.
	for _, perm := range requiredPermissionsForTaskType(taskType) {
		errActionPerm := xs.ensureLocalServerPermission(user, gameServer, perm)
		if errActionPerm != nil {
			return nil, connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("creating a '%s' task also requires '%s' permission", taskType, perm))
		}
	}

	if taskType == "backup" && request.Msg.GetEnabled() {
		operationsAllowed, disabledReason := xs.backupOperationsAllowed(ctx, gameServer)
		if !operationsAllowed {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New(disabledReason))
		}
	}

	// Validate input.
	errValidate := validateScheduledTaskInput(
		request.Msg.GetName(),
		taskType,
		request.Msg.GetCronExpression(),
		timezone,
		consoleCommand,
	)
	if errValidate != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errValidate)
	}

	task, errInsert := xs.db.InsertScheduledTask(
		serverID,
		user.ID,
		request.Msg.GetName(),
		taskType,
		request.Msg.GetCronExpression(),
		timezone,
		consoleCommand,
		request.Msg.GetEnabled(),
	)
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Failed to create scheduled task")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create scheduled task"))
	}

	// Register with the scheduler if enabled. If AddTask fails (e.g. invalid
	// cron expression that passed the heuristic field-count check), clean up
	// the DB row and return an error so the client is not left with a
	// non-executing task.
	if task.Enabled == 1 && xs.taskScheduler != nil {
		errAdd := xs.taskScheduler.AddTask(task)
		if errAdd != nil {
			log.Error().Err(errAdd).Str("task_id", task.ID).Msg("Failed to register scheduled task with scheduler")
			_ = xs.db.DeleteScheduledTask(task.ID)
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid schedule: %w", errAdd))
		}
	}

	return connect.NewResponse(&xylona.CreateScheduledTaskResponse{
		Task: scheduledTaskToProto(task, true),
	}), nil
}

// UpdateScheduledTask updates an existing scheduled task.
func (xs *XylonaService) UpdateScheduledTask(
	ctx context.Context,
	request *connect.Request[xylona.UpdateScheduledTaskRequest],
) (*connect.Response[xylona.UpdateScheduledTaskResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	taskID := request.Msg.GetId()
	if taskID == "" {
		return nil, invalidArg("id is required")
	}

	existing, errExisting := xs.db.GetScheduledTaskByID(taskID)
	if errExisting != nil {
		return nil, notFoundErr()
	}

	gameServer, errGS := xs.db.GetGameServerByID(existing.GameServerID)
	if errGS != nil {
		return nil, notFoundErr()
	}

	errPerm := xs.ensureLocalServerPermission(user, gameServer, permissionScheduledTasks)
	if errPerm != nil {
		return nil, errPerm
	}

	taskType := request.Msg.GetTaskType()
	consoleCommand := request.Msg.GetConsoleCommand()
	timezone := request.Msg.GetTimezone()
	if timezone == "" {
		timezone = "UTC"
	}

	// Check action-specific permission for the new task type.
	for _, perm := range requiredPermissionsForTaskType(taskType) {
		errActionPerm := xs.ensureLocalServerPermission(user, gameServer, perm)
		if errActionPerm != nil {
			return nil, connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("a '%s' task requires '%s' permission", taskType, perm))
		}
	}

	if taskType == "backup" && request.Msg.GetEnabled() {
		operationsAllowed, disabledReason := xs.backupOperationsAllowed(ctx, gameServer)
		if !operationsAllowed {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New(disabledReason))
		}
	}

	errValidate := validateScheduledTaskInput(
		request.Msg.GetName(),
		taskType,
		request.Msg.GetCronExpression(),
		timezone,
		consoleCommand,
	)
	if errValidate != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errValidate)
	}

	updated, errUpdate := xs.db.UpdateScheduledTask(
		taskID,
		request.Msg.GetName(),
		taskType,
		request.Msg.GetCronExpression(),
		timezone,
		consoleCommand,
		request.Msg.GetEnabled(),
	)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("task_id", taskID).Msg("Failed to update scheduled task")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update scheduled task"))
	}

	// Sync with the scheduler. If updating fails (e.g. invalid cron), return
	// the error to the client rather than silently leaving a broken schedule.
	if xs.taskScheduler != nil {
		errSync := xs.taskScheduler.UpdateTask(updated)
		if errSync != nil {
			log.Error().Err(errSync).Str("task_id", taskID).Msg("Failed to sync scheduled task with scheduler")
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid schedule: %w", errSync))
		}
	}

	return connect.NewResponse(&xylona.UpdateScheduledTaskResponse{
		Task: scheduledTaskToProto(updated, true),
	}), nil
}

// DeleteScheduledTask removes a scheduled task.
func (xs *XylonaService) DeleteScheduledTask(
	_ context.Context,
	request *connect.Request[xylona.DeleteScheduledTaskRequest],
) (*connect.Response[xylona.DeleteScheduledTaskResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	taskID := request.Msg.GetId()
	if taskID == "" {
		return nil, invalidArg("id is required")
	}

	existing, errExisting := xs.db.GetScheduledTaskByID(taskID)
	if errExisting != nil {
		return nil, notFoundErr()
	}

	gameServer, errGS := xs.db.GetGameServerByID(existing.GameServerID)
	if errGS != nil {
		return nil, notFoundErr()
	}

	errPerm := xs.ensureLocalServerPermission(user, gameServer, permissionScheduledTasks)
	if errPerm != nil {
		return nil, errPerm
	}

	// Delete from DB first — if this fails the scheduler still has a stale
	// entry (harmless, the executor will fail to find the row). The reverse
	// order would silently remove from the scheduler with the row surviving.
	errDelete := xs.db.DeleteScheduledTask(taskID)
	if errDelete != nil {
		log.Error().Err(errDelete).Str("task_id", taskID).Msg("Failed to delete scheduled task")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete scheduled task"))
	}

	if xs.taskScheduler != nil {
		xs.taskScheduler.RemoveTask(taskID)
	}

	return connect.NewResponse(&xylona.DeleteScheduledTaskResponse{}), nil
}

// GetScheduledTaskLogs returns execution log entries for a scheduled task.
func (xs *XylonaService) GetScheduledTaskLogs(
	_ context.Context,
	request *connect.Request[xylona.GetScheduledTaskLogsRequest],
) (*connect.Response[xylona.GetScheduledTaskLogsResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	scheduledTaskID := request.Msg.GetScheduledTaskId()
	if scheduledTaskID == "" {
		return nil, invalidArg("scheduled_task_id is required")
	}

	task, errTask := xs.db.GetScheduledTaskByID(scheduledTaskID)
	if errTask != nil {
		return nil, notFoundErr()
	}

	gameServer, errGS := xs.db.GetGameServerByID(task.GameServerID)
	if errGS != nil {
		return nil, notFoundErr()
	}

	errPerm := xs.ensureLocalServerPermission(user, gameServer, permissionScheduledTasks)
	if errPerm != nil {
		return nil, errPerm
	}

	canReadConsoleDetails, errConsolePerm := xs.canReadConsoleTaskDetails(user, gameServer)
	if errConsolePerm != nil {
		return nil, errConsolePerm
	}

	limit := int(request.Msg.GetLimit())
	offset := int(request.Msg.GetOffset())

	logs, errGet := xs.db.GetScheduledTaskLogs(scheduledTaskID, limit, offset)
	if errGet != nil {
		log.Error().Err(errGet).Str("scheduled_task_id", scheduledTaskID).Msg("Failed to get scheduled task logs")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get scheduled task logs"))
	}

	resp := &xylona.GetScheduledTaskLogsResponse{
		Logs: make([]*xylona.ScheduledTaskLog, 0, len(logs)),
	}
	for _, entry := range logs {
		includeMessage := entry.TaskType != "console_command" || canReadConsoleDetails
		resp.Logs = append(resp.Logs, scheduledTaskLogToProto(entry, includeMessage))
	}

	return connect.NewResponse(resp), nil
}

// SetScheduler sets the task scheduler on the RPC service. Called from main.go
// after the scheduler is created.
func (xs *XylonaService) SetScheduler(s *scheduler.Scheduler) {
	xs.taskScheduler = s
}
