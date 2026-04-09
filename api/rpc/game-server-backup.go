package rpc

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const permissionBackup = "game_server.backup"

const (
	backupDisabledReasonBackupsDisabled  = "Backups are disabled for this server."
	backupDisabledReasonInvalidDirectory = "Backup directory is not valid for this server."
	backupDisabledReasonLocalOnly        = "Backups are only available for local servers."
	backupDisabledReasonNotConfigured    = "Backups are not configured for this server."
)

type backupUserFacingError string

func (e backupUserFacingError) Error() string {
	return string(e)
}

var (
	errBackupLocalOnly = backupUserFacingError(backupDisabledReasonLocalOnly)
)

func isLocalBackupServer(gameServer *models.GameServer) bool {
	if gameServer == nil || gameServer.R.Node == nil {
		return false
	}

	return gameServer.R.Node.IsLocal
}

func backupDirectoryConfigured(gameServer *models.GameServer) bool {
	if gameServer == nil {
		return false
	}

	return strings.TrimSpace(gameServer.BackupDirectory) != ""
}

func backupOperationsAllowed(gameServer *models.GameServer) (bool, string) {
	if !isLocalBackupServer(gameServer) {
		return false, backupDisabledReasonLocalOnly
	}
	if !gameServer.BackupsEnabled {
		return false, backupDisabledReasonBackupsDisabled
	}
	if !backupDirectoryConfigured(gameServer) {
		return false, backupDisabledReasonNotConfigured
	}
	errValidateDirectory := actions.ValidateGameServerBackupDirectory(gameServer)
	if errValidateDirectory != nil {
		return false, backupDisabledReasonInvalidDirectory
	}

	return true, ""
}

func backupRestoreAllowed(gameServer *models.GameServer) (bool, string) {
	if !isLocalBackupServer(gameServer) {
		return false, backupDisabledReasonLocalOnly
	}
	if !gameServer.BackupsEnabled {
		return false, backupDisabledReasonBackupsDisabled
	}

	return true, ""
}

func gameServerBackupStatusToProto(status string) xylona.GameServerBackupStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending":
		return xylona.GameServerBackupStatus_GAME_SERVER_BACKUP_STATUS_PENDING
	case "completed":
		return xylona.GameServerBackupStatus_GAME_SERVER_BACKUP_STATUS_COMPLETED
	case "failed":
		return xylona.GameServerBackupStatus_GAME_SERVER_BACKUP_STATUS_FAILED
	default:
		return xylona.GameServerBackupStatus_GAME_SERVER_BACKUP_STATUS_UNSPECIFIED
	}
}

func gameServerBackupTriggerSourceToProto(triggerSource string) xylona.GameServerBackupTriggerSource {
	switch strings.ToLower(strings.TrimSpace(triggerSource)) {
	case "manual":
		return xylona.GameServerBackupTriggerSource_GAME_SERVER_BACKUP_TRIGGER_SOURCE_MANUAL
	case "scheduled":
		return xylona.GameServerBackupTriggerSource_GAME_SERVER_BACKUP_TRIGGER_SOURCE_SCHEDULED
	default:
		return xylona.GameServerBackupTriggerSource_GAME_SERVER_BACKUP_TRIGGER_SOURCE_UNSPECIFIED
	}
}

func gameServerBackupToProto(backup *models.GameServerBackup) *xylona.GameServerBackup {
	if backup == nil {
		return nil
	}

	protoBackup := &xylona.GameServerBackup{
		Id:              backup.ID,
		GameServerId:    backup.GameServerID,
		NodeId:          backup.NodeID,
		TriggerSource:   gameServerBackupTriggerSourceToProto(backup.TriggerSource),
		ArchivePath:     backup.ArchivePath,
		ArchiveFormat:   backup.ArchiveFormat,
		Status:          gameServerBackupStatusToProto(backup.Status),
		SizeBytes:       backup.SizeBytes,
		RetentionExempt: backup.RetentionExempt,
		CreatedAt:       timestamppb.New(backup.CreatedAt),
	}

	createdBy, createdBySet := backup.CreatedBy.Get()
	if createdBySet {
		protoBackup.CreatedBy = &createdBy
	}

	errorMessage, errorMessageSet := backup.ErrorMessage.Get()
	if errorMessageSet {
		protoBackup.ErrorMessage = &errorMessage
	}

	completedAt, completedAtSet := backup.CompletedAt.Get()
	if completedAtSet {
		protoBackup.CompletedAt = timestamppb.New(completedAt)
	}

	return protoBackup
}

func backupSettingsToProto(gameServer *models.GameServer, includeDirectory bool) *xylona.BackupSettings {
	settings := &xylona.BackupSettings{
		BackupsEnabled: gameServer.BackupsEnabled,
		MaxBackups:     normalizeBackupRetention(gameServer.MaxBackups),
	}
	if includeDirectory {
		backupDirectory := strings.TrimSpace(gameServer.BackupDirectory)
		if backupDirectory == "" {
			backupDirectory = defaultBackupDirectoryForServer(gameServer.Directory)
		}
		settings.BackupDirectory = backupDirectory
		settings.DefaultBackupDirectory = defaultBackupDirectoryForServer(gameServer.Directory)
	}

	return settings
}

func countScheduledBackups(tasks []*models.ScheduledTask) int32 {
	var scheduledBackupCount int32
	for _, task := range tasks {
		if task.TaskType == "backup" {
			scheduledBackupCount++
		}
	}

	return scheduledBackupCount
}

func (xs *XylonaService) getGameServerForBackupRPC(gameServerID string) (*models.GameServer, error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(gameServerID)
	if errGetGameServer != nil {
		return nil, dbLookupMsg(errGetGameServer, "failed to load game server")
	}

	return gameServer, nil
}

func (xs *XylonaService) getBackupByIDForGameServer(gameServerID string, backupID string) (*models.GameServerBackup, error) {
	backup, errGetBackup := xs.db.GetGameServerBackupByID(backupID)
	if errGetBackup != nil {
		return nil, dbLookupMsg(errGetBackup, "failed to load backup")
	}
	if backup.GameServerID != gameServerID {
		return nil, invalidArg("backup does not belong to this server")
	}

	return backup, nil
}

// GetGameServerBackupOverview returns backup capability state for a game server.
func (xs *XylonaService) GetGameServerBackupOverview(
	_ context.Context,
	request *connect.Request[xylona.GetGameServerBackupOverviewRequest],
) (*connect.Response[xylona.GetGameServerBackupOverviewResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServerID := request.Msg.GetGameServerId()
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}

	gameServer, errGetGameServer := xs.getGameServerForBackupRPC(gameServerID)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}

	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionBackup)
	if errPermission != nil {
		return nil, errPermission
	}

	scheduledTasks, errGetTasks := xs.db.GetScheduledTasksByGameServerID(gameServerID)
	if errGetTasks != nil {
		return nil, internalErrf("failed to load scheduled tasks")
	}

	operationsAllowed, disabledReason := backupOperationsAllowed(gameServer)
	overview := &xylona.GameServerBackupOverview{
		Enabled:                   gameServer.BackupsEnabled,
		CanManageSettings:         user.SuperUser,
		LocalServer:               isLocalBackupServer(gameServer),
		BackupDirectoryConfigured: backupDirectoryConfigured(gameServer),
		ScheduledBackupCount:      countScheduledBackups(scheduledTasks),
		OperationsAllowed:         operationsAllowed,
		DisabledReason:            disabledReason,
	}

	return connect.NewResponse(&xylona.GetGameServerBackupOverviewResponse{
		Overview: overview,
	}), nil
}

// GetBackupSettings returns the current backup settings for a game server.
func (xs *XylonaService) GetBackupSettings(
	_ context.Context,
	request *connect.Request[xylona.GetBackupSettingsRequest],
) (*connect.Response[xylona.GetBackupSettingsResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServerID := request.Msg.GetGameServerId()
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}

	gameServer, errGetGameServer := xs.getGameServerForBackupRPC(gameServerID)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}

	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionBackup)
	if errPermission != nil {
		return nil, errPermission
	}

	return connect.NewResponse(&xylona.GetBackupSettingsResponse{
		Settings: backupSettingsToProto(gameServer, user.SuperUser),
	}), nil
}

// UpdateBackupSettings updates the dedicated backup settings for a game server.
func (xs *XylonaService) UpdateBackupSettings(
	_ context.Context,
	request *connect.Request[xylona.UpdateBackupSettingsRequest],
) (*connect.Response[xylona.UpdateBackupSettingsResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser required")
	}

	gameServerID := request.Msg.GetGameServerId()
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}
	gameServer, errGetGameServer := xs.getGameServerForBackupRPC(gameServerID)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}

	updatedGameServer := *gameServer
	updatedGameServer.BackupsEnabled = request.Msg.GetBackupsEnabled()
	updatedGameServer.BackupDirectory = strings.TrimSpace(request.Msg.GetBackupDirectory())
	if updatedGameServer.BackupsEnabled && updatedGameServer.BackupDirectory == "" {
		updatedGameServer.BackupDirectory = defaultBackupDirectoryForServer(gameServer.Directory)
	}
	updatedGameServer.MaxBackups = normalizeBackupRetention(request.Msg.GetMaxBackups())
	if updatedGameServer.BackupsEnabled || backupDirectoryConfigured(&updatedGameServer) {
		errValidateDirectory := actions.ValidateGameServerBackupDirectory(&updatedGameServer)
		if errValidateDirectory != nil {
			reason := backupDisabledReasonInvalidDirectory
			if !backupDirectoryConfigured(&updatedGameServer) {
				reason = backupDisabledReasonNotConfigured
			}
			return nil, invalidArg(reason)
		}
	}

	setter := &models.GameServerSetter{
		ID:             omit.From(gameServer.ID),
		BackupsEnabled: omit.From(updatedGameServer.BackupsEnabled),
		BackupDirectory: omit.From(
			updatedGameServer.BackupDirectory,
		),
		MaxBackups: omit.From(updatedGameServer.MaxBackups),
	}

	updated, errUpdate := xs.db.UpdateGameServer(xs.db.DB, setter)
	if errUpdate != nil {
		return nil, internalErrf("failed to update backup settings")
	}

	return connect.NewResponse(&xylona.UpdateBackupSettingsResponse{
		Settings: backupSettingsToProto(updated, true),
	}), nil
}

// ListGameServerBackups lists recorded backups for a game server.
func (xs *XylonaService) ListGameServerBackups(
	_ context.Context,
	request *connect.Request[xylona.ListGameServerBackupsRequest],
) (*connect.Response[xylona.ListGameServerBackupsResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServerID := request.Msg.GetGameServerId()
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}

	gameServer, errGetGameServer := xs.getGameServerForBackupRPC(gameServerID)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}

	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionBackup)
	if errPermission != nil {
		return nil, errPermission
	}

	backups, errListBackups := xs.db.ListGameServerBackupsByGameServerID(gameServerID)
	if errListBackups != nil {
		return nil, internalErrf("failed to list backups")
	}

	protoBackups := make([]*xylona.GameServerBackup, 0, len(backups))
	for _, backup := range backups {
		protoBackups = append(protoBackups, gameServerBackupToProto(backup))
	}

	return connect.NewResponse(&xylona.ListGameServerBackupsResponse{
		Backups: protoBackups,
	}), nil
}

// CreateGameServerBackup creates a manual backup for a game server.
func (xs *XylonaService) CreateGameServerBackup(
	_ context.Context,
	request *connect.Request[xylona.CreateGameServerBackupRequest],
) (*connect.Response[xylona.CreateGameServerBackupResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServerID := request.Msg.GetGameServerId()
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}
	backupName := strings.TrimSpace(request.Msg.GetBackupName())
	errValidateName := actions.ValidateManualBackupName(backupName)
	if errValidateName != nil {
		return nil, invalidArg(errValidateName.Error())
	}

	gameServer, errGetGameServer := xs.getGameServerForBackupRPC(gameServerID)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}

	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionBackup)
	if errPermission != nil {
		return nil, errPermission
	}

	operationsAllowed, disabledReason := backupOperationsAllowed(gameServer)
	if !operationsAllowed {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New(disabledReason))
	}
	if xs.actionsInst == nil {
		return nil, internalErrf("backup service unavailable")
	}

	backup, errCreateBackup := xs.actionsInst.CreateManualBackup(gameServer, user.ID, backupName)
	if errCreateBackup != nil {
		if errors.Is(errCreateBackup, actions.ErrInvalidManualBackupName) {
			return nil, invalidArg(errCreateBackup.Error())
		}
		if errors.Is(errCreateBackup, actions.ErrManualBackupNameAlreadyExists) {
			return nil, invalidArg(errCreateBackup.Error())
		}
		return nil, internalErrf("failed to create backup")
	}

	return connect.NewResponse(&xylona.CreateGameServerBackupResponse{
		Backup: gameServerBackupToProto(backup),
	}), nil
}

// DeleteGameServerBackup removes a recorded backup and its archive file.
func (xs *XylonaService) DeleteGameServerBackup(
	_ context.Context,
	request *connect.Request[xylona.DeleteGameServerBackupRequest],
) (*connect.Response[xylona.DeleteGameServerBackupResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServerID := request.Msg.GetGameServerId()
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}
	backupID := request.Msg.GetBackupId()
	if backupID == "" {
		return nil, invalidArg("backup_id is required")
	}

	gameServer, errGetGameServer := xs.getGameServerForBackupRPC(gameServerID)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}

	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionBackup)
	if errPermission != nil {
		return nil, errPermission
	}

	if !isLocalBackupServer(gameServer) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errBackupLocalOnly)
	}

	backup, errGetBackup := xs.getBackupByIDForGameServer(gameServerID, backupID)
	if errGetBackup != nil {
		return nil, errGetBackup
	}
	if backup.NodeID != gameServer.NodeID {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("backup belongs to a different node"))
	}
	if xs.actionsInst == nil {
		return nil, internalErrf("backup service unavailable")
	}

	errDeleteBackup := xs.actionsInst.DeleteGameServerBackup(gameServer, backup)
	if errDeleteBackup != nil {
		return nil, internalErrf("failed to delete backup")
	}

	return connect.NewResponse(&xylona.DeleteGameServerBackupResponse{}), nil
}

// RestoreGameServerBackup restores a backup archive onto a game server.
func (xs *XylonaService) RestoreGameServerBackup(
	_ context.Context,
	request *connect.Request[xylona.RestoreGameServerBackupRequest],
) (*connect.Response[xylona.RestoreGameServerBackupResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServerID := request.Msg.GetGameServerId()
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}
	backupID := request.Msg.GetBackupId()
	if backupID == "" {
		return nil, invalidArg("backup_id is required")
	}

	gameServer, errGetGameServer := xs.getGameServerForBackupRPC(gameServerID)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}

	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionBackup)
	if errPermission != nil {
		return nil, errPermission
	}

	operationsAllowed, disabledReason := backupRestoreAllowed(gameServer)
	if !operationsAllowed {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New(disabledReason))
	}
	if xs.actionsInst == nil {
		return nil, internalErrf("backup service unavailable")
	}

	backup, errGetBackup := xs.getBackupByIDForGameServer(gameServerID, backupID)
	if errGetBackup != nil {
		return nil, errGetBackup
	}

	errRestore := xs.actionsInst.RestoreGameServerBackup(gameServer, backup.ID, request.Msg.GetRestoreMode())
	if errRestore != nil {
		userMessage, isUserFacing := actions.BackupRestoreUserFacingMessage(errRestore)
		if isUserFacing {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New(userMessage))
		}

		log.Error().
			Err(errRestore).
			Str("game_server_id", gameServer.ID).
			Str("backup_id", backup.ID).
			Msg("Restore game server backup failed")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to restore backup"))
	}

	return connect.NewResponse(&xylona.RestoreGameServerBackupResponse{}), nil
}
