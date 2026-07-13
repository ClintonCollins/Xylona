package scheduler

import (
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	controlleractions "github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	statusSuccess  = "success"
	statusFailed   = "failed"
	statusSkipped  = "skipped"
	statusTimedOut = "timed_out"

	// schedulerStopTimeout gives long-running local backup jobs time to finish
	// cleanly before Shutdown returns a timeout error.
	schedulerStopTimeout = 30 * time.Minute

	// restartStopTimeout is how long to wait for a server to reach OFFLINE
	// after a stop command before giving up.
	restartStopTimeout = 30 * time.Second
	// restartPollInterval is how often to check status during a restart.
	restartPollInterval = 500 * time.Millisecond
)

// executeTask is the callback invoked by gocron for each scheduled task.
// It loads the task definition from the database at runtime to use fresh state.
func (s *Scheduler) executeTask(taskID string) {
	task, errGet := s.db.GetScheduledTaskByID(taskID)
	if errGet != nil {
		log.Error().Err(errGet).Str("task_id", taskID).Msg("Scheduled task not found in DB")
		return
	}

	if task.Enabled == 0 {
		return
	}

	startedAt := time.Now().UTC()

	logger := log.With().
		Str("task_id", task.ID).
		Str("task_name", task.Name).
		Str("task_type", task.TaskType).
		Str("game_server_id", task.GameServerID).
		Logger()

	logger.Info().Msg("Executing scheduled task")

	var status string
	var message string

	switch task.TaskType {
	case "restart":
		status, message = s.executeRestart(task)
	case "console_command":
		status, message = s.executeConsoleCommand(task)
	case "backup":
		status, message = s.executeBackup(task)
	default:
		status = statusFailed
		message = fmt.Sprintf("unknown task type: %s", task.TaskType)
	}

	finishedAt := time.Now().UTC()

	logger.Info().Str("status", status).Str("message", message).Msg("Scheduled task completed")

	// Log the execution.
	_, errLog := s.db.InsertScheduledTaskLog(
		task.ID,
		task.GameServerID,
		task.TaskType,
		status,
		message,
		startedAt,
		&finishedAt,
	)
	if errLog != nil {
		logger.Error().Err(errLog).Msg("Failed to write scheduled task log")
	}

	// Update last_run_at on the task.
	errUpdate := s.db.UpdateScheduledTaskLastRun(task.ID, finishedAt, nil)
	if errUpdate != nil {
		logger.Error().Err(errUpdate).Msg("Failed to update scheduled task last_run_at")
	}

	// Publish event for future alert integration.
	eventbus.Get().Publish(eventbus.TopicScheduledTaskExecuted, eventbus.ScheduledTaskExecutedEvent{
		TaskID:       task.ID,
		GameServerID: task.GameServerID,
		TaskType:     task.TaskType,
		Status:       status,
		Message:      message,
		Timestamp:    finishedAt,
	})
}

func (s *Scheduler) executeRestart(task *models.ScheduledTask) (string, string) {
	gameServer, errGS := s.db.GetGameServerByID(task.GameServerID)
	if errGS != nil {
		return statusFailed, fmt.Sprintf("failed to load game server: %s", errGS)
	}

	currentStatus := s.actions.CurrentStatus(gameServer)

	switch currentStatus {
	case xylona.Status_OFFLINE:
		return statusSkipped, "server is offline; scheduled restart does not start stopped servers"
	case xylona.Status_INSTALLING, xylona.Status_UPDATING:
		return statusSkipped, fmt.Sprintf("server is busy (%s); skipping restart", currentStatus)
	case xylona.Status_PRE_START:
		return statusSkipped, fmt.Sprintf("server is in transition (%s); skipping restart", currentStatus)
	case xylona.Status_ONLINE:
		// proceed with restart
	case xylona.Status_UNKNOWN:
		return statusSkipped, "server status unknown; skipping restart"
	default:
		return statusSkipped, fmt.Sprintf("unexpected server status: %s", currentStatus)
	}

	// Phase 1: Stop the server.
	errStop := s.actions.StopGameServer(s.ctx, gameServer)
	if errStop != nil {
		return statusFailed, fmt.Sprintf("failed to stop server: %s", errStop)
	}

	// Phase 2: Wait for OFFLINE status.
	deadline := time.After(restartStopTimeout)
	pollTick := time.NewTicker(restartPollInterval)
	defer pollTick.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return statusFailed, "scheduler shutting down during restart"
		case <-deadline:
			return statusTimedOut, "server did not reach OFFLINE within timeout after stop command"
		case <-pollTick.C:
			if s.actions.CurrentStatus(gameServer) == xylona.Status_OFFLINE {
				// Phase 3: Start the server.
				_, errStart := s.actions.StartGameServer(gameServer)
				if errStart != nil {
					return statusFailed, fmt.Sprintf("server stop confirmed, but start failed: %s", errStart)
				}
				return statusSuccess, "server stop confirmed, start issued"
			}
		}
	}
}

func (s *Scheduler) executeConsoleCommand(task *models.ScheduledTask) (string, string) {
	consoleCmd, consoleCmdSet := task.ConsoleCommand.Get()
	consoleCommand := ""
	if consoleCmdSet {
		consoleCommand = consoleCmd
	}
	if consoleCommand == "" {
		return statusFailed, "no console command configured"
	}

	gameServer, errGS := s.db.GetGameServerByID(task.GameServerID)
	if errGS != nil {
		return statusFailed, fmt.Sprintf("failed to load game server: %s", errGS)
	}

	currentStatus := s.actions.CurrentStatus(gameServer)
	if currentStatus != xylona.Status_ONLINE {
		return statusSkipped, fmt.Sprintf("server is not online (%s); cannot send console command", currentStatus)
	}

	errSend := s.actions.SendConsoleInput(gameServer, consoleCommand)
	if errSend != nil {
		return statusFailed, fmt.Sprintf("failed to send console command: %s", errSend)
	}

	return statusSuccess, "console command sent"
}

func (s *Scheduler) executeBackup(task *models.ScheduledTask) (string, string) {
	if s.backup == nil {
		return statusFailed, "backup executor is not configured"
	}

	gameServer, errGetGS := s.db.GetGameServerByID(task.GameServerID)
	if errGetGS != nil {
		return statusFailed, fmt.Sprintf("failed to load game server: %s", errGetGS)
	}

	_, errGetNode := s.db.GetNodeByID(gameServer.NodeID)
	if errGetNode != nil {
		return statusFailed, fmt.Sprintf("failed to load game server node: %s", errGetNode)
	}
	// backup_service.produceBackupArchive handles local vs. remote node
	// routing internally (writing directly on the controller for embedded
	// servers, round-tripping via NodeClient for remote nodes).

	backup, errCreateBackup := s.backup.CreateScheduledBackup(gameServer)
	if errCreateBackup != nil {
		if errors.Is(errCreateBackup, controlleractions.ErrBackupsUnsupported) {
			return statusSkipped, "scheduled backup skipped because this game does not support backups on the node platform"
		}
		if errors.Is(errCreateBackup, controlleractions.ErrBackupCapabilityUnavailable) {
			return statusFailed, "failed to determine backup support for the game server node"
		}
		if backup != nil {
			backupID := backup.ID
			if backupID == "" {
				backupID = "unknown"
			}
			return statusFailed, fmt.Sprintf(
				"scheduled backup %s created, but post-backup work failed: %s",
				backupID,
				errCreateBackup,
			)
		}
		return statusFailed, fmt.Sprintf("failed to create scheduled backup: %s", errCreateBackup)
	}

	return statusSuccess, "scheduled backup created"
}
