package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/updateconfig"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	updateProcessTimeout      = 6 * time.Hour
	updateProcessPollInterval = time.Second
)

var (
	errUpdateExecutionReplaced       = errors.New("update process execution was replaced")
	errUpdateCompletionIndeterminate = errors.New("update process completion is indeterminate")
)

// UpdateProgressBroadcaster receives update progress events.
// Implemented by the WebSocket layer.
type UpdateProgressBroadcaster interface {
	BroadcastUpdateProgress(serverID string, serverName string, step xylona.UpdateStep, stepStatus xylona.StepStatus, message string)
}

// configExtensions is the set of file extensions treated as config files for
// backup purposes.
var configExtensions = map[string]bool{
	".json":       true,
	".ini":        true,
	".toml":       true,
	".yaml":       true,
	".yml":        true,
	".properties": true,
	".xml":        true,
	".cfg":        true,
	".conf":       true,
}

// backupServerFiles copies the server executable and config files to
// .update-backup/ in the server's directory.
func (inst *Instance) backupServerFiles(gs *models.GameServer) error {
	client, errClient := inst.resolveUpdateFileClient(gs, "update backup")
	if errClient != nil {
		return errClient
	}
	return inst.backupNodeServerFiles(gs, client)
}

func (inst *Instance) backupNodeServerFiles(gs *models.GameServer, client nodeclient.NodeClient) error {
	ctx := inst.actionContext()
	_, errExisting := client.StatFile(ctx, gs.Directory, ".update-backup")
	if errExisting == nil {
		return errors.New("actions: update recovery is pending; resolve .update-backup before starting another update")
	}
	if !errors.Is(errExisting, os.ErrNotExist) {
		return fmt.Errorf("actions: inspect node update recovery directory: %w", errExisting)
	}

	pendingDirectory := ".update-backup.pending-" + uuid.NewString()
	errCreate := client.CreateFileOrDirectory(ctx, gs.Directory, pendingDirectory, "", true, node.ProtectionPolicy{})
	if errCreate != nil {
		return fmt.Errorf("actions: create pending node backup directory: %w", errCreate)
	}
	cleanupPending := func() error {
		deleted, errDelete := client.DeleteFiles(ctx, gs.Directory, []string{pendingDirectory}, node.ProtectionPolicy{})
		if errDelete != nil {
			return fmt.Errorf("remove pending node backup directory: %w", errDelete)
		}
		if len(deleted) != 1 {
			return errors.New("remove pending node backup directory: node did not confirm deletion")
		}
		return nil
	}

	entries, errList := client.ListFiles(ctx, gs.Directory, "")
	if errList != nil {
		return errors.Join(
			fmt.Errorf("actions: list node server directory: %w", errList),
			cleanupPending(),
		)
	}

	namedExecutable := ""
	if !gs.ServerExecutable.IsNull() {
		namedExecutable = gs.ServerExecutable.GetOr("")
	}

	operations := make([]node.CopyFileOperation, 0)
	for _, entry := range entries {
		if entry.IsDirectory {
			continue
		}

		name := entry.Name
		if name == ".update-backup" {
			continue
		}

		shouldCopy := shouldBackupUpdateFile(name, namedExecutable, entry.IsExecutable)
		if !shouldCopy {
			continue
		}

		operations = append(operations, node.CopyFileOperation{
			SourceRelativePath:      name,
			DestinationRelativePath: path.Join(pendingDirectory, name),
		})
	}

	if len(operations) > 0 {
		copied, errCopy := client.CopyFiles(ctx, gs.Directory, operations, node.ProtectionPolicy{})
		if errCopy != nil {
			return errors.Join(
				fmt.Errorf("actions: copy node update backup files: %w", errCopy),
				cleanupPending(),
			)
		}
		if len(copied) != len(operations) {
			return errors.Join(
				fmt.Errorf("actions: copy node update backup files: node confirmed %d of %d copies", len(copied), len(operations)),
				cleanupPending(),
			)
		}
	}

	_, errPromote := client.RenameFile(
		ctx,
		gs.Directory,
		pendingDirectory,
		".update-backup",
		node.ProtectionPolicy{},
	)
	if errPromote != nil {
		return errors.Join(
			fmt.Errorf("actions: promote pending node update backup: %w", errPromote),
			cleanupPending(),
		)
	}
	return nil
}

func shouldBackupUpdateFile(name string, namedExecutable string, executable bool) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if configExtensions[ext] {
		return true
	}

	if namedExecutable != "" {
		return name == namedExecutable
	}

	return executable
}

// restoreServerFiles restores from .update-backup/ back to the server
// directory and removes the backup directory.
func (inst *Instance) restoreServerFiles(gs *models.GameServer) error {
	client, errClient := inst.resolveUpdateFileClient(gs, "update restore")
	if errClient != nil {
		return errClient
	}
	return inst.restoreNodeServerFiles(gs, client)
}

func (inst *Instance) restoreNodeServerFiles(gs *models.GameServer, client nodeclient.NodeClient) error {
	ctx := inst.actionContext()
	foundInternal, errInternal := restoreInternalUpdateTransaction(ctx, gs, client)
	if errInternal != nil {
		return errInternal
	}
	entries, errList := client.ListFiles(ctx, gs.Directory, ".update-backup")
	if errors.Is(errList, os.ErrNotExist) {
		if foundInternal {
			return errors.New("actions: internal update recovery disappeared before legacy restore")
		}
		return nil
	}
	if errList != nil {
		return fmt.Errorf("actions: list node backup directory: %w", errList)
	}

	operations := make([]node.CopyFileOperation, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDirectory {
			continue
		}

		backupPath := path.Join(".update-backup", entry.Name)
		operations = append(operations, node.CopyFileOperation{
			SourceRelativePath:      backupPath,
			DestinationRelativePath: entry.Name,
		})
	}

	if len(operations) > 0 {
		_, errCopy := client.CopyFiles(ctx, gs.Directory, operations, node.ProtectionPolicy{})
		if errCopy != nil {
			return fmt.Errorf("actions: copy node update restore files: %w", errCopy)
		}
	}

	_, errDelete := client.DeleteFiles(ctx, gs.Directory, []string{".update-backup"}, node.ProtectionPolicy{})
	if errDelete != nil {
		return fmt.Errorf("actions: remove node backup directory: %w", errDelete)
	}

	return nil
}

func restoreInternalUpdateTransaction(
	ctx context.Context,
	gs *models.GameServer,
	client nodeclient.NodeClient,
) (bool, error) {
	manifestBytes, errRead := client.ReadFile(ctx, gs.Directory, gameintegrations.InternalUpdateManifestPath)
	if errors.Is(errRead, os.ErrNotExist) {
		return false, nil
	}
	if errRead != nil {
		return false, fmt.Errorf("actions: read internal update manifest: %w", errRead)
	}
	var manifest gameintegrations.UpdateTransactionManifest
	errDecode := json.Unmarshal(manifestBytes, &manifest)
	if errDecode != nil {
		return true, fmt.Errorf("actions: decode internal update manifest: %w", errDecode)
	}
	errValidate := gameintegrations.ValidateUpdateTransactionManifest(manifest)
	if errValidate != nil {
		return true, fmt.Errorf("actions: validate internal update manifest: %w", errValidate)
	}

	for _, entry := range slices.Backward(manifest.Entries) {
		backupPath := path.Join(gameintegrations.InternalUpdateFilesDirectory, entry.Path)
		if entry.Existed {
			_, errBackup := client.StatFile(ctx, gs.Directory, backupPath)
			if errors.Is(errBackup, os.ErrNotExist) {
				_, errLive := client.StatFile(ctx, gs.Directory, entry.Path)
				if errLive == nil {
					continue
				}
				return true, fmt.Errorf("actions: rollback original %q and live path are both unavailable: %w", entry.Path, errLive)
			}
			if errBackup != nil {
				return true, fmt.Errorf("actions: inspect rollback original %q: %w", entry.Path, errBackup)
			}
		}

		deleted, errDelete := client.DeleteFiles(ctx, gs.Directory, []string{entry.Path}, node.ProtectionPolicy{})
		if errDelete != nil {
			return true, fmt.Errorf("actions: remove updated path %q: %w", entry.Path, errDelete)
		}
		if len(deleted) != 1 {
			return true, fmt.Errorf("actions: remove updated path %q: node did not confirm deletion", entry.Path)
		}
		if !entry.Existed {
			continue
		}
		copied, errCopy := client.CopyFiles(ctx, gs.Directory, []node.CopyFileOperation{{
			SourceRelativePath:      backupPath,
			DestinationRelativePath: entry.Path,
		}}, node.ProtectionPolicy{})
		if errCopy != nil {
			return true, fmt.Errorf("actions: restore internal update original %q: %w", entry.Path, errCopy)
		}
		if len(copied) != 1 {
			return true, fmt.Errorf("actions: restore internal update original %q: node did not confirm copy", entry.Path)
		}
	}
	return true, nil
}

// cleanupBackup removes the .update-backup/ directory. Errors are logged but
// not returned.
func (inst *Instance) cleanupBackup(gs *models.GameServer) {
	client, errClient := inst.resolveUpdateFileClient(gs, "backup cleanup")
	if errClient != nil {
		log.Error().Err(errClient).Str("game_server_id", gs.ID).Msg("Failed to resolve node client for backup cleanup")
		return
	}
	inst.cleanupNodeBackup(gs, client)
}

func (inst *Instance) cleanupNodeBackup(gs *models.GameServer, client nodeclient.NodeClient) {
	_, errDelete := client.DeleteFiles(inst.actionContext(), gs.Directory, []string{".update-backup"}, node.ProtectionPolicy{})
	if errDelete != nil {
		log.Error().Err(errDelete).Str("game_server_id", gs.ID).Msg("Failed to remove node backup directory")
	}
}

func (inst *Instance) resolveUpdateFileClient(gs *models.GameServer, operation string) (nodeclient.NodeClient, error) {
	client, errClient := inst.resolveNodeClient(gs.NodeID)
	if errClient == nil {
		return client, nil
	}
	return nil, fmt.Errorf("actions: resolve node client for %s: %w", operation, errClient)
}

// UpdateGameServerWithBackup stops the server (if running), backs up config
// and executable files, runs the update, then restarts. On failure the backup
// is restored and the server is restarted if it was running before. The whole
// sequence runs in a goroutine so it does not block the caller. A server can
// have only one active update operation.
func (inst *Instance) UpdateGameServerWithBackup(
	gameServer *models.GameServer,
	selectedTarget string,
	broadcaster UpdateProgressBroadcaster,
) error {
	if gameServer == nil || strings.TrimSpace(gameServer.ID) == "" {
		return errors.New("actions: game server is required for update")
	}
	releaseOperation, errOperation := inst.TryBeginGameServerLifecycleOperation(gameServer.ID)
	if errOperation != nil {
		return errOperation
	}

	target := strings.TrimSpace(selectedTarget)
	if target != "" {
		if gameServer.R.Game != nil && gameServer.R.Game.UsesSteamcmd {
			errPersist := inst.PersistSteamBranchSelection(gameServer, target)
			if errPersist != nil {
				releaseOperation()
				return errPersist
			}
		} else {
			gameServer.Branch = target
		}
	}

	go func() {
		defer releaseOperation()
		inst.runUpdateWithBackup(gameServer, broadcaster)
	}()
	return nil
}

func (inst *Instance) runUpdateWithBackup(gameServer *models.GameServer, broadcaster UpdateProgressBroadcaster) {
	serverID := gameServer.ID
	operationStartedAt := time.Now()
	operationPhase := db.GameServerOperationPhasePreparing
	operationOutcome := db.GameServerOperationOutcomeFailed
	defer func() {
		inst.recordGameServerOperation(
			serverID,
			db.GameServerOperationUpdate,
			operationPhase,
			operationOutcome,
			operationStartedAt,
			time.Now(),
			nil,
			db.GameServerOperationSourceController,
		)
	}()

	// Determine pre-update running state via NodeClient so both embedded
	// and remote game servers report a consistent status.
	preStatus := inst.currentProcessStatus(gameServer)
	if preStatus == xylona.Status_UNKNOWN {
		message := "Game server status is unavailable; update was not started"
		inst.sendConsoleLine(gameServer, message)
		if broadcaster != nil {
			broadcaster.BroadcastUpdateProgress(
				serverID,
				gameServer.Name,
				xylona.UpdateStep_UPDATE_STEP_STOPPING,
				xylona.StepStatus_STEP_STATUS_FAILED,
				message,
			)
		}
		return
	}
	wasRunning := preStatus != xylona.Status_OFFLINE

	updatePlan, errPlan := inst.resolveMinecraftUpdatePlan(gameServer)
	if errPlan != nil && gameServer.GameID == "minecraft" {
		log.Debug().
			Err(errPlan).
			Str("game_server_id", serverID).
			Msg("Could not resolve detailed Minecraft update info")
	}

	broadcast := func(step xylona.UpdateStep, status xylona.StepStatus, msg string) {
		if msg != "" {
			inst.sendConsoleLine(gameServer, msg)
		}
		if broadcaster != nil {
			broadcaster.BroadcastUpdateProgress(serverID, gameServer.Name, step, status, msg)
		}
	}

	// Step 1: Stop if running.
	if wasRunning {
		operationPhase = db.GameServerOperationPhaseStopping
		broadcast(xylona.UpdateStep_UPDATE_STEP_STOPPING, xylona.StepStatus_STEP_STATUS_IN_PROGRESS, "Stopping server")
		errStop := inst.StopGameServer(inst.actionContext(), gameServer)
		if errStop != nil {
			log.Error().Err(errStop).Str("game_server_id", serverID).Msg("Failed to stop server before update")
			broadcast(
				xylona.UpdateStep_UPDATE_STEP_STOPPING,
				xylona.StepStatus_STEP_STATUS_FAILED,
				"Failed to stop server; update was not started: "+errStop.Error(),
			)
			return
		}
		// Wait for server to fully stop (poll up to 30s). Uses NodeClient
		// snapshots so remote-node stops are observed identically.
		stopped := false
		for range 30 {
			s := inst.currentProcessStatus(gameServer)
			if s == xylona.Status_OFFLINE {
				stopped = true
				break
			}
			time.Sleep(time.Second)
		}
		if !stopped {
			log.Error().Str("game_server_id", serverID).Msg("Could not confirm server shutdown before update")
			broadcast(
				xylona.UpdateStep_UPDATE_STEP_STOPPING,
				xylona.StepStatus_STEP_STATUS_FAILED,
				"Could not confirm server shutdown; update was not started",
			)
			return
		}
		broadcast(xylona.UpdateStep_UPDATE_STEP_STOPPING, xylona.StepStatus_STEP_STATUS_COMPLETED, "Server stopped")
	}

	// Step 2: Backup.
	operationPhase = db.GameServerOperationPhaseBackingUp
	broadcast(xylona.UpdateStep_UPDATE_STEP_BACKING_UP, xylona.StepStatus_STEP_STATUS_IN_PROGRESS, "Backing up files")
	errBackup := inst.backupServerFiles(gameServer)
	if errBackup != nil {
		log.Error().Err(errBackup).Str("game_server_id", serverID).Msg("Backup failed")
		broadcast(
			xylona.UpdateStep_UPDATE_STEP_BACKING_UP,
			xylona.StepStatus_STEP_STATUS_FAILED,
			"Backup failed; update was not started",
		)
		if wasRunning {
			broadcast(
				xylona.UpdateStep_UPDATE_STEP_RESTARTING,
				xylona.StepStatus_STEP_STATUS_IN_PROGRESS,
				"Restarting server after backup failure",
			)
			_, errRestart := inst.StartGameServer(gameServer)
			if errRestart != nil {
				log.Error().Err(errRestart).Str("game_server_id", serverID).Msg("Failed to restart server after backup failure")
				broadcast(
					xylona.UpdateStep_UPDATE_STEP_RESTARTING,
					xylona.StepStatus_STEP_STATUS_FAILED,
					"Server failed to restart after backup failure",
				)
				return
			}
			restarted := waitForServerOnline(inst.ctx, func() (xylona.Status, bool) {
				status := inst.currentProcessStatus(gameServer)
				return status, status != xylona.Status_UNKNOWN
			}, 60, time.Second)
			if !restarted {
				broadcast(
					xylona.UpdateStep_UPDATE_STEP_RESTARTING,
					xylona.StepStatus_STEP_STATUS_FAILED,
					"Server failed to return online after backup failure",
				)
				return
			}
			broadcast(
				xylona.UpdateStep_UPDATE_STEP_RESTARTING,
				xylona.StepStatus_STEP_STATUS_COMPLETED,
				"Server restarted after backup failure",
			)
		}
		return
	}
	broadcast(xylona.UpdateStep_UPDATE_STEP_BACKING_UP, xylona.StepStatus_STEP_STATUS_COMPLETED, "Backup complete")

	// Step 3: Update (download + install).
	operationPhase = db.GameServerOperationPhaseUpdating
	broadcast(
		xylona.UpdateStep_UPDATE_STEP_DOWNLOADING,
		xylona.StepStatus_STEP_STATUS_IN_PROGRESS,
		downloadStartMessage(gameServer, updatePlan),
	)

	// If the dummy tracker has failure simulation enabled, treat this as a failed update.
	updateFailed := inst.dummyTracker != nil && inst.dummyTracker.SimulateFailure()
	asyncUpdate := gameServer.GameID != "minecraft"
	updateExecutionID := ""
	var updateEvents chan any
	if !updateFailed && asyncUpdate {
		eb := eventbus.Get()
		updateEvents = eb.SubscribeReliable(eventbus.TopicGameServerStatusChanged)
		defer eb.Unsubscribe(eventbus.TopicGameServerStatusChanged, updateEvents)
	}

	if !updateFailed {
		var errUpdate error
		updateExecutionID, errUpdate = inst.startGameServerUpdate(gameServer)
		if errUpdate != nil {
			log.Error().Err(errUpdate).Str("game_server_id", serverID).Msg("Failed to start game update")
			updateFailed = true
		}
	}
	if !updateFailed {
		broadcast(
			xylona.UpdateStep_UPDATE_STEP_DOWNLOADING,
			xylona.StepStatus_STEP_STATUS_COMPLETED,
			downloadCompleteMessage(gameServer, updatePlan),
		)
		broadcast(
			xylona.UpdateStep_UPDATE_STEP_INSTALLING,
			xylona.StepStatus_STEP_STATUS_IN_PROGRESS,
			installStartMessage(gameServer, updatePlan),
		)
	}
	if !updateFailed && asyncUpdate {
		client, errClient := inst.resolveNodeClient(gameServer.NodeID)
		if errClient != nil {
			log.Error().Err(errClient).Str("game_server_id", serverID).Msg("Failed resolving node client while waiting for game update")
			updateFailed = true
		}
		var exitEvent eventbus.StatusChangedEvent
		var errWait error
		if !updateFailed {
			exitEvent, errWait = waitForUpdateProcessExit(
				inst.ctx,
				updateEvents,
				serverID,
				updateExecutionID,
				client.GetProcessSnapshot,
				updateProcessTimeout,
				updateProcessPollInterval,
			)
		}
		if errWait != nil {
			log.Error().Err(errWait).Str("game_server_id", serverID).Msg("Failed waiting for game update")
			updateFailed = true
		} else if exitEvent.ExitCode != 0 {
			log.Error().
				Int("exit_code", exitEvent.ExitCode).
				Str("game_server_id", serverID).
				Msg("Game update exited with non-zero status")
			updateFailed = true
		}
	}

	if updateFailed {
		broadcast(xylona.UpdateStep_UPDATE_STEP_INSTALLING, xylona.StepStatus_STEP_STATUS_FAILED, "Update failed")
		inst.rollbackUpdate(gameServer, wasRunning, broadcaster)
		return
	}
	if wasRunning {
		broadcast(
			xylona.UpdateStep_UPDATE_STEP_INSTALLING,
			xylona.StepStatus_STEP_STATUS_COMPLETED,
			installCompleteMessage(gameServer, updatePlan, true),
		)
	} else {
		broadcast(
			xylona.UpdateStep_UPDATE_STEP_INSTALLING,
			xylona.StepStatus_STEP_STATUS_COMPLETED,
			installCompleteMessage(gameServer, updatePlan, false),
		)
	}

	// Step 4: Restart if the server was running before the update.
	if wasRunning {
		operationPhase = db.GameServerOperationPhaseRestarting
		broadcast(xylona.UpdateStep_UPDATE_STEP_RESTARTING, xylona.StepStatus_STEP_STATUS_IN_PROGRESS, "Restarting server")
		_, errStart := inst.StartGameServer(gameServer)
		if errStart != nil {
			log.Error().Err(errStart).Str("game_server_id", serverID).Msg("Failed to restart server after update")
			broadcast(xylona.UpdateStep_UPDATE_STEP_RESTARTING, xylona.StepStatus_STEP_STATUS_FAILED, "Server failed to restart")
			inst.rollbackUpdate(gameServer, wasRunning, broadcaster)
			return
		}
		// Poll up to 60s for the server to come online. Uses NodeClient
		// so embedded and remote restarts are observed uniformly.
		restarted := waitForServerOnline(inst.ctx, func() (xylona.Status, bool) {
			s := inst.currentProcessStatus(gameServer)
			if s == xylona.Status_UNKNOWN {
				return xylona.Status_UNKNOWN, false
			}
			return s, true
		}, 60, time.Second)
		if !restarted {
			broadcast(xylona.UpdateStep_UPDATE_STEP_RESTARTING, xylona.StepStatus_STEP_STATUS_FAILED, "Server failed to restart")
			inst.rollbackUpdate(gameServer, wasRunning, broadcaster)
			return
		}
		broadcast(xylona.UpdateStep_UPDATE_STEP_RESTARTING, xylona.StepStatus_STEP_STATUS_COMPLETED, "Server restarted")
	}

	inst.cleanupBackup(gameServer)

	// If the dummy tracker is active, mark it as updated so subsequent version
	// checks report the installed version as up-to-date.
	if inst.dummyTracker != nil {
		inst.dummyTracker.MarkUpdated()
	}

	log.Info().Str("game_server_id", serverID).Msg("Update completed successfully")

	// Trigger a version re-check so the UI reflects the new installed version.
	if inst.db != nil {
		inst.CheckServerVersionByID(inst.ctx, serverID)
	}
	operationPhase = db.GameServerOperationPhaseComplete
	operationOutcome = db.GameServerOperationOutcomeSucceeded
}

func waitForUpdateProcessExit(
	ctx context.Context,
	events <-chan any,
	serverID string,
	executionID string,
	getSnapshot func(context.Context, string) (*node.ProcessSnapshot, bool, error),
	timeout time.Duration,
	pollInterval time.Duration,
) (eventbus.StatusChangedEvent, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	poll := time.NewTicker(pollInterval)
	defer poll.Stop()

	for {
		select {
		case <-ctx.Done():
			return eventbus.StatusChangedEvent{}, fmt.Errorf("wait for update process: %w", ctx.Err())
		case <-timer.C:
			return eventbus.StatusChangedEvent{}, fmt.Errorf("wait for update process: timed out after %s", timeout)
		case rawEvent, open := <-events:
			if !open {
				events = nil
				continue
			}
			event, ok := rawEvent.(eventbus.StatusChangedEvent)
			if !ok || event.ServerID != serverID {
				continue
			}
			if event.ExecutionID != "" && event.ExecutionID != executionID {
				continue
			}
			oldStatus := strings.ToUpper(strings.TrimSpace(event.OldStatus))
			newStatus := strings.ToUpper(strings.TrimSpace(event.NewStatus))
			if oldStatus != xylona.Status_UPDATING.String() || newStatus != xylona.Status_OFFLINE.String() {
				continue
			}
			return event, nil
		case <-poll.C:
			snapshot, found, errSnapshot := getSnapshot(ctx, serverID)
			if errSnapshot != nil {
				continue
			}
			if !found || snapshot == nil {
				return eventbus.StatusChangedEvent{}, fmt.Errorf("wait for update process: %w: process snapshot unavailable", errUpdateCompletionIndeterminate)
			}

			if snapshot.ExecutionID == "" {
				status := strings.ToUpper(strings.TrimSpace(snapshot.Status))
				if status == xylona.Status_OFFLINE.String() {
					return eventbus.StatusChangedEvent{}, fmt.Errorf("wait for update process: %w: legacy node reported offline without a terminal event", errUpdateCompletionIndeterminate)
				}
				continue
			}
			if snapshot.ExecutionID != executionID {
				return eventbus.StatusChangedEvent{}, fmt.Errorf(
					"wait for update process: %w: expected %s, got %s",
					errUpdateExecutionReplaced,
					executionID,
					snapshot.ExecutionID,
				)
			}

			status := strings.ToUpper(strings.TrimSpace(snapshot.Status))
			if status != xylona.Status_OFFLINE.String() {
				continue
			}
			previousStatus := strings.ToUpper(strings.TrimSpace(snapshot.PreviousStatus))
			if previousStatus != xylona.Status_UPDATING.String() ||
				snapshot.TransitionSequence == 0 || !snapshot.ExitCodeKnown {
				return eventbus.StatusChangedEvent{}, fmt.Errorf("wait for update process: %w: terminal snapshot lacks correlated update metadata", errUpdateCompletionIndeterminate)
			}
			return eventbus.StatusChangedEvent{
				ServerID:           serverID,
				OldStatus:          previousStatus,
				NewStatus:          status,
				ExecutionID:        snapshot.ExecutionID,
				TransitionSequence: snapshot.TransitionSequence,
				IntentionalStop:    snapshot.IntentionalStop,
				ExitCode:           snapshot.ExitCode,
				ExitCodeKnown:      true,
			}, nil
		}
	}
}

func downloadStartMessage(gameServer *models.GameServer, plan *minecraftUpdatePlan) string {
	if plan == nil {
		if usesSteamCMDUpdate(gameServer) {
			return "Preparing SteamCMD update"
		}
		return "Downloading update"
	}

	message := "Downloading " + plan.softwareName + " for Minecraft " + plan.targetVersion
	if plan.downloadVersionName != "" && plan.downloadVersionName != plan.targetVersion {
		message += " (" + plan.downloadVersionName + ")"
	}
	if plan.plannedFileName != "" {
		message += " as " + plan.plannedFileName
	}
	return message
}

func downloadCompleteMessage(gameServer *models.GameServer, plan *minecraftUpdatePlan) string {
	if plan == nil {
		if usesSteamCMDUpdate(gameServer) {
			return "SteamCMD session ready"
		}
		return "Download complete"
	}

	targetJar := updatedJarName(gameServer, plan)
	if targetJar != "" {
		return "Downloaded " + targetJar + " for " + plan.softwareName + " " + plan.targetVersion
	}
	return "Downloaded " + plan.softwareName + " " + plan.targetVersion
}

func installStartMessage(gameServer *models.GameServer, plan *minecraftUpdatePlan) string {
	if plan == nil {
		if usesSteamCMDUpdate(gameServer) {
			branch := versiontracker.NormalizeSteamBranch(gameServer.Branch)
			if branch == "public" {
				return "Running SteamCMD update"
			}
			return "Running SteamCMD update for branch " + branch
		}
		return "Installing update"
	}

	targetJar := updatedJarName(gameServer, plan)
	if targetJar != "" {
		return "Applying " + targetJar
	}
	return "Applying " + plan.softwareName + " " + plan.targetVersion
}

func installCompleteMessage(gameServer *models.GameServer, plan *minecraftUpdatePlan, wasRunning bool) string {
	if plan == nil {
		if usesSteamCMDUpdate(gameServer) {
			return "SteamCMD update complete"
		}
		if wasRunning {
			return "Update installed"
		}
		return "Update complete"
	}

	targetJar := updatedJarName(gameServer, plan)
	if targetJar != "" {
		return "Installed " + plan.softwareName + " " + plan.targetVersion + " with " + targetJar
	}
	return "Installed " + plan.softwareName + " " + plan.targetVersion
}

func updatedJarName(gameServer *models.GameServer, plan *minecraftUpdatePlan) string {
	if gameServer != nil {
		executable := filepath.Base(gameServer.ServerExecutable.GetOr(""))
		if executable != "." && executable != "" {
			return executable
		}
	}
	if plan != nil {
		return plan.plannedFileName
	}
	return ""
}

func usesSteamCMDUpdate(gameServer *models.GameServer) bool {
	if gameServer == nil || gameServer.R.Game == nil {
		return false
	}

	resolved, errResolve := updateconfig.ResolveModelConfig(gameServer.R.Game, gameServer)
	if errResolve == nil {
		return resolved.Provider.Kind == updateproviders.ProviderKindSteamCMD
	}

	return gameServer.R.Game.UsesSteamcmd
}

func waitForServerOnline(
	ctx context.Context,
	statusLookup func() (xylona.Status, bool),
	attempts int,
	interval time.Duration,
) bool {
	for i := range attempts {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		status, ok := statusLookup()
		if ok && status == xylona.Status_ONLINE {
			return true
		}
		if i == attempts-1 {
			break
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return false
		case <-timer.C:
		}
	}
	return false
}

func (inst *Instance) rollbackUpdate(gameServer *models.GameServer, wasRunning bool, broadcaster UpdateProgressBroadcaster) {
	serverID := gameServer.ID
	broadcast := func(status xylona.StepStatus, msg string) {
		if msg != "" {
			inst.sendConsoleLine(gameServer, msg)
		}
		if broadcaster != nil {
			broadcaster.BroadcastUpdateProgress(
				serverID,
				gameServer.Name,
				xylona.UpdateStep_UPDATE_STEP_ROLLING_BACK,
				status,
				msg,
			)
		}
	}

	broadcast(xylona.StepStatus_STEP_STATUS_IN_PROGRESS, "Rolling back")
	errRestore := inst.restoreServerFiles(gameServer)
	if errRestore != nil {
		log.Error().Err(errRestore).Str("game_server_id", serverID).Msg("Rollback restore failed")
		broadcast(
			xylona.StepStatus_STEP_STATUS_FAILED,
			"Rollback failed; recovery data was retained",
		)
		return
	}
	if wasRunning {
		_, errStart := inst.StartGameServer(gameServer)
		if errStart != nil {
			log.Error().Err(errStart).Str("game_server_id", serverID).Msg("Failed to restart server after rollback")
			broadcast(
				xylona.StepStatus_STEP_STATUS_FAILED,
				"Files were restored, but the server failed to restart",
			)
			return
		}
		restarted := waitForServerOnline(inst.ctx, func() (xylona.Status, bool) {
			status := inst.currentProcessStatus(gameServer)
			return status, status != xylona.Status_UNKNOWN
		}, 60, time.Second)
		if !restarted {
			broadcast(
				xylona.StepStatus_STEP_STATUS_FAILED,
				"Files were restored, but the server did not return online",
			)
			return
		}
	}
	broadcast(xylona.StepStatus_STEP_STATUS_COMPLETED, "Rollback complete")
	log.Warn().Str("game_server_id", serverID).Msg("Update rolled back")
}
