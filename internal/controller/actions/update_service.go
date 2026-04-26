package actions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/updateconfig"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
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
	errCreate := client.CreateFileOrDirectory(ctx, gs.Directory, ".update-backup", "", true, node.ProtectionPolicy{})
	if errCreate != nil {
		return fmt.Errorf("actions: create node backup directory: %w", errCreate)
	}

	entries, errList := client.ListFiles(ctx, gs.Directory, "")
	if errList != nil {
		return fmt.Errorf("actions: list node server directory: %w", errList)
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
			DestinationRelativePath: path.Join(".update-backup", name),
		})
	}

	if len(operations) == 0 {
		return nil
	}

	_, errCopy := client.CopyFiles(ctx, gs.Directory, operations, node.ProtectionPolicy{})
	if errCopy != nil {
		return fmt.Errorf("actions: copy node update backup files: %w", errCopy)
	}

	return nil
}

func shouldBackupUpdateFile(name string, namedExecutable string, executable bool) bool {
	ext := filepath.Ext(name)
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
	entries, errList := client.ListFiles(ctx, gs.Directory, ".update-backup")
	if errors.Is(errList, os.ErrNotExist) {
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
// sequence runs in a goroutine so it does not block the caller.
func (inst *Instance) UpdateGameServerWithBackup(
	gameServer *models.GameServer,
	broadcaster UpdateProgressBroadcaster,
) {
	go inst.runUpdateWithBackup(gameServer, broadcaster)
}

func (inst *Instance) runUpdateWithBackup(gameServer *models.GameServer, broadcaster UpdateProgressBroadcaster) {
	serverID := gameServer.ID

	// Determine pre-update running state via NodeClient so both embedded
	// and remote game servers report a consistent status.
	preStatus := inst.currentProcessStatus(gameServer)
	wasRunning := preStatus != xylona.Status_OFFLINE && preStatus != xylona.Status_UNKNOWN

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
		broadcast(xylona.UpdateStep_UPDATE_STEP_STOPPING, xylona.StepStatus_STEP_STATUS_IN_PROGRESS, "Stopping server")
		inst.StopGameServer(gameServer)
		// Wait for server to fully stop (poll up to 30s). Uses NodeClient
		// snapshots so remote-node stops are observed identically.
		stopped := false
		for range 30 {
			s := inst.currentProcessStatus(gameServer)
			if s == xylona.Status_OFFLINE || s == xylona.Status_UNKNOWN {
				stopped = true
				break
			}
			time.Sleep(time.Second)
		}
		if !stopped {
			log.Warn().Str("game_server_id", serverID).Msg("Server did not stop cleanly before update")
		}
		broadcast(xylona.UpdateStep_UPDATE_STEP_STOPPING, xylona.StepStatus_STEP_STATUS_COMPLETED, "Server stopped")
	}

	// Step 2: Backup.
	broadcast(xylona.UpdateStep_UPDATE_STEP_BACKING_UP, xylona.StepStatus_STEP_STATUS_IN_PROGRESS, "Backing up files")
	errBackup := inst.backupServerFiles(gameServer)
	if errBackup != nil {
		log.Error().Err(errBackup).Str("game_server_id", serverID).Msg("Backup failed")
		broadcast(xylona.UpdateStep_UPDATE_STEP_BACKING_UP, xylona.StepStatus_STEP_STATUS_FAILED, "Backup failed")
		return
	}
	broadcast(xylona.UpdateStep_UPDATE_STEP_BACKING_UP, xylona.StepStatus_STEP_STATUS_COMPLETED, "Backup complete")

	// Step 3: Update (download + install).
	broadcast(
		xylona.UpdateStep_UPDATE_STEP_DOWNLOADING,
		xylona.StepStatus_STEP_STATUS_IN_PROGRESS,
		downloadStartMessage(gameServer, updatePlan),
	)

	// If the dummy tracker has failure simulation enabled, treat this as a failed update.
	updateFailed := inst.dummyTracker != nil && inst.dummyTracker.SimulateFailure()

	if !updateFailed {
		errUpdate := inst.UpdateGameServer(gameServer)
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
	// UpdateGameServer is async (starts a supervised process). Wait for it
	// to complete. Uses NodeClient snapshot so remote-node updates are
	// observed identically.
	for range 120 {
		s := inst.currentProcessStatus(gameServer)
		if s == xylona.Status_OFFLINE || s == xylona.Status_UNKNOWN {
			break
		}
		if s == xylona.Status_ONLINE || s == xylona.Status_UPDATING || s == xylona.Status_INSTALLING {
			time.Sleep(time.Second)
			continue
		}
		// Unknown terminal state.
		updateFailed = true
		break
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
	broadcast := func(step xylona.UpdateStep, status xylona.StepStatus, msg string) {
		if msg != "" {
			inst.sendConsoleLine(gameServer, msg)
		}
		if broadcaster != nil {
			broadcaster.BroadcastUpdateProgress(serverID, gameServer.Name, step, status, msg)
		}
	}

	broadcast(xylona.UpdateStep_UPDATE_STEP_ROLLING_BACK, xylona.StepStatus_STEP_STATUS_IN_PROGRESS, "Rolling back")
	errRestore := inst.restoreServerFiles(gameServer)
	if errRestore != nil {
		log.Error().Err(errRestore).Str("game_server_id", serverID).Msg("Rollback restore failed")
	}
	if wasRunning {
		_, errStart := inst.StartGameServer(gameServer)
		if errStart != nil {
			log.Error().Err(errStart).Str("game_server_id", serverID).Msg("Failed to restart server after rollback")
		}
	}
	broadcast(xylona.UpdateStep_UPDATE_STEP_ROLLING_BACK, xylona.StepStatus_STEP_STATUS_COMPLETED, "Rollback complete")
	log.Warn().Str("game_server_id", serverID).Msg("Update rolled back")
}
