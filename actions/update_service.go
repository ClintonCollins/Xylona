package actions

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// UpdateProgressBroadcaster receives update progress events.
// Implemented by the WebSocket layer.
type UpdateProgressBroadcaster interface {
	BroadcastUpdateProgress(serverID string, step xylona.UpdateStep, stepStatus xylona.StepStatus, message string)
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
func backupServerFiles(gs *models.GameServer) error {
	backupDir := filepath.Join(gs.Directory, ".update-backup")
	errMkdir := os.MkdirAll(backupDir, 0755)
	if errMkdir != nil {
		return errMkdir
	}

	entries, errRead := os.ReadDir(gs.Directory)
	if errRead != nil {
		return errRead
	}

	// Determine whether we back up a named executable or all executables.
	namedExecutable := ""
	if !gs.ServerExecutable.IsNull() {
		namedExecutable = gs.ServerExecutable.GetOr("")
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip the backup directory itself if it somehow appears as an entry.
		if name == ".update-backup" {
			continue
		}

		shouldCopy := false
		ext := filepath.Ext(name)
		if configExtensions[ext] {
			shouldCopy = true
		}

		if !shouldCopy {
			if namedExecutable != "" {
				if name == namedExecutable {
					shouldCopy = true
				}
			} else {
				if isExecutableFile(entry) {
					shouldCopy = true
				}
			}
		}

		if !shouldCopy {
			continue
		}

		errCopy := copyFile(
			filepath.Join(gs.Directory, name),
			filepath.Join(backupDir, name),
		)
		if errCopy != nil {
			return errCopy
		}
	}

	return nil
}

// restoreServerFiles restores from .update-backup/ back to the server
// directory and removes the backup directory.
func restoreServerFiles(gs *models.GameServer) error {
	backupDir := filepath.Join(gs.Directory, ".update-backup")

	_, errStat := os.Stat(backupDir)
	if os.IsNotExist(errStat) {
		return nil
	}
	if errStat != nil {
		return errStat
	}

	entries, errRead := os.ReadDir(backupDir)
	if errRead != nil {
		return errRead
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		errCopy := copyFile(
			filepath.Join(backupDir, entry.Name()),
			filepath.Join(gs.Directory, entry.Name()),
		)
		if errCopy != nil {
			return errCopy
		}
	}

	errRemove := os.RemoveAll(backupDir)
	if errRemove != nil {
		return errRemove
	}

	return nil
}

// cleanupBackup removes the .update-backup/ directory. Errors are logged but
// not returned.
func cleanupBackup(gs *models.GameServer) {
	backupDir := filepath.Join(gs.Directory, ".update-backup")
	errRemove := os.RemoveAll(backupDir)
	if errRemove != nil {
		log.Error().Err(errRemove).Str("game_server_id", gs.ID).Msg("Failed to remove backup directory")
	}
}

// windowsExecutableExtensions is the set of extensions treated as executables
// on Windows, where POSIX execute bits are not supported.
var windowsExecutableExtensions = map[string]bool{
	".exe": true,
	".bat": true,
	".cmd": true,
	".ps1": true,
	".com": true,
}

// isExecutableFile reports whether a directory entry looks like an executable.
// On Linux/Darwin this checks POSIX execute bits. On Windows it checks file
// extension because the filesystem does not expose execute bits.
func isExecutableFile(entry os.DirEntry) bool {
	if OperatingSystem == Windows {
		ext := filepath.Ext(entry.Name())
		return windowsExecutableExtensions[ext]
	}
	info, errInfo := entry.Info()
	if errInfo != nil {
		return false
	}
	return info.Mode()&0111 != 0
}

// copyFile copies the file at src to dst, creating or truncating dst.
func copyFile(src, dst string) error {
	srcFile, errOpen := os.Open(src)
	if errOpen != nil {
		return errOpen
	}
	defer func() {
		_ = srcFile.Close()
	}()

	srcInfo, errStat := srcFile.Stat()
	if errStat != nil {
		return errStat
	}

	dstFile, errCreate := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if errCreate != nil {
		return errCreate
	}
	defer func() {
		_ = dstFile.Close()
	}()

	_, errCopy := io.Copy(dstFile, srcFile)
	return errCopy
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

	// Determine pre-update running state.
	wasRunning := false
	cmd, errGetCmd := inst.supervisorInstance.GetCommandByID(serverID)
	if errGetCmd == nil {
		status := cmd.Status()
		wasRunning = status != xylona.Status_OFFLINE && status != xylona.Status_UNKNOWN
	}

	broadcast := func(step xylona.UpdateStep, status xylona.StepStatus, msg string) {
		if broadcaster != nil {
			broadcaster.BroadcastUpdateProgress(serverID, step, status, msg)
		}
	}

	// Step 1: Stop if running.
	broadcast(xylona.UpdateStep_UPDATE_STEP_STOPPING, xylona.StepStatus_STEP_STATUS_IN_PROGRESS, "Stopping server")
	if wasRunning {
		inst.StopGameServer(gameServer)
		// Wait for server to fully stop (poll up to 30s).
		stopped := false
		for i := 0; i < 30; i++ {
			c, errGet := inst.supervisorInstance.GetCommandByID(serverID)
			if errGet != nil {
				stopped = true
				break
			}
			s := c.Status()
			if s == xylona.Status_OFFLINE || s == xylona.Status_UNKNOWN {
				stopped = true
				break
			}
			time.Sleep(time.Second)
		}
		if !stopped {
			log.Warn().Str("game_server_id", serverID).Msg("Server did not stop cleanly before update")
		}
	}
	broadcast(xylona.UpdateStep_UPDATE_STEP_STOPPING, xylona.StepStatus_STEP_STATUS_COMPLETED, "Server stopped")

	// Step 2: Backup.
	broadcast(xylona.UpdateStep_UPDATE_STEP_BACKING_UP, xylona.StepStatus_STEP_STATUS_IN_PROGRESS, "Backing up files")
	errBackup := backupServerFiles(gameServer)
	if errBackup != nil {
		log.Error().Err(errBackup).Str("game_server_id", serverID).Msg("Backup failed")
		broadcast(xylona.UpdateStep_UPDATE_STEP_BACKING_UP, xylona.StepStatus_STEP_STATUS_FAILED, "Backup failed")
		return
	}
	broadcast(xylona.UpdateStep_UPDATE_STEP_BACKING_UP, xylona.StepStatus_STEP_STATUS_COMPLETED, "Backup complete")

	// Step 3: Update (download + install).
	broadcast(xylona.UpdateStep_UPDATE_STEP_DOWNLOADING, xylona.StepStatus_STEP_STATUS_IN_PROGRESS, "Downloading update")
	broadcast(xylona.UpdateStep_UPDATE_STEP_INSTALLING, xylona.StepStatus_STEP_STATUS_IN_PROGRESS, "Installing update")
	inst.UpdateGameServer(gameServer)
	// UpdateGameServer is async (starts a supervised process). Wait for it to complete.
	updateFailed := false
	for i := 0; i < 120; i++ {
		c, errGet := inst.supervisorInstance.GetCommandByID(serverID)
		if errGet != nil {
			break // Command finished and was removed.
		}
		s := c.Status()
		if s == xylona.Status_OFFLINE || s == xylona.Status_UNKNOWN {
			break
		}
		if s == xylona.Status_ONLINE || s == xylona.Status_UPDATING {
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
	broadcast(xylona.UpdateStep_UPDATE_STEP_INSTALLING, xylona.StepStatus_STEP_STATUS_COMPLETED, "Update installed")

	// Step 4: Restart if was running.
	if wasRunning {
		broadcast(xylona.UpdateStep_UPDATE_STEP_RESTARTING, xylona.StepStatus_STEP_STATUS_IN_PROGRESS, "Restarting server")
		inst.StartGameServer(gameServer)
		// Wait 30s to verify it comes up.
		time.Sleep(30 * time.Second)
		c, errGet := inst.supervisorInstance.GetCommandByID(serverID)
		if errGet != nil || c.Status() == xylona.Status_OFFLINE || c.Status() == xylona.Status_UNKNOWN {
			broadcast(xylona.UpdateStep_UPDATE_STEP_RESTARTING, xylona.StepStatus_STEP_STATUS_FAILED, "Server failed to restart")
			inst.rollbackUpdate(gameServer, wasRunning, broadcaster)
			return
		}
		broadcast(xylona.UpdateStep_UPDATE_STEP_RESTARTING, xylona.StepStatus_STEP_STATUS_COMPLETED, "Server restarted")
	}

	cleanupBackup(gameServer)
	log.Info().Str("game_server_id", serverID).Msg("Update completed successfully")
}

func (inst *Instance) rollbackUpdate(gameServer *models.GameServer, wasRunning bool, broadcaster UpdateProgressBroadcaster) {
	serverID := gameServer.ID
	broadcast := func(step xylona.UpdateStep, status xylona.StepStatus, msg string) {
		if broadcaster != nil {
			broadcaster.BroadcastUpdateProgress(serverID, step, status, msg)
		}
	}

	broadcast(xylona.UpdateStep_UPDATE_STEP_ROLLING_BACK, xylona.StepStatus_STEP_STATUS_IN_PROGRESS, "Rolling back")
	errRestore := restoreServerFiles(gameServer)
	if errRestore != nil {
		log.Error().Err(errRestore).Str("game_server_id", serverID).Msg("Rollback restore failed")
	}
	if wasRunning {
		inst.StartGameServer(gameServer)
	}
	broadcast(xylona.UpdateStep_UPDATE_STEP_ROLLING_BACK, xylona.StepStatus_STEP_STATUS_COMPLETED, "Rollback complete")
	log.Warn().Str("game_server_id", serverID).Msg("Update rolled back")
}

