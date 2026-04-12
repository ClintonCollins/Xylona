// Package actions contains the control-plane business logic for game server
// lifecycle, file operations, synchronization, and background jobs.
package actions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"

	internal "github.com/ClintonCollins/Xylona/api/xylona-internal"
	"github.com/ClintonCollins/Xylona/cfgschema"
	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/pkg/modmanager"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/placeholder"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers/federation"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/startargs"
	"github.com/ClintonCollins/Xylona/supervisor"
)

var (
	// ErrInvalidPath is returned when a requested file path escapes the server root.
	ErrInvalidPath = errors.New("invalid path")
	// ErrGameUpdateNotConfigured is returned when a game has no configured updater.
	ErrGameUpdateNotConfigured = errors.New("game update is not configured")
	// ErrInternalGameUpdateMissing is returned when an internal updater is unavailable.
	ErrInternalGameUpdateMissing = errors.New("internal game updater is not registered")
	// ErrMinecraftVariantUpdateNotSupported is returned when a Minecraft variant cannot be updated automatically.
	ErrMinecraftVariantUpdateNotSupported = errors.New("updates are not supported for this Minecraft server software")
	// ErrNotMinecraftServer is returned when a non-Minecraft server is used with Minecraft-only flows.
	ErrNotMinecraftServer             = errors.New("game server is not a minecraft server")
	errGameRelationNotLoaded          = errors.New("game relation not loaded")
	errStartCommandTemplateMissing    = errors.New("no start command template configured for this game. a superuser must set up the start command template before servers can be started")
	errBaseCommandMissing             = errors.New("no base command configured for this game")
	errMinecraftGameRelationNotLoaded = errors.New("minecraft game relation not loaded")
	reSteamBranchableUpdate           = regexp.MustCompile(`(?i)(\+app_update\s+\d+)`)
)

// Supported shell command types for install and update actions.
const (
	CommandTypeDirect     = "direct"
	CommandTypeBash       = "bash"
	CommandTypePowershell = "powershell"
	CommandTypeCmd        = "cmd"
	CommandTypeInternal   = "internal"
)

// SyncEngine is implemented by FederationSyncEngine. Using an interface
// avoids a circular dependency between actions and sync-engine.
type SyncEngine interface {
	SyncPeer(peerNodeID string)
	RemovePeer(peerNodeID string)
	BroadcastPeerChange(changeType xylona.PeerChangeType, peer *xylona.PeerInfo, initiatedByNodeID string, initiatedByNodeName string)
}

// VersionBroadcaster pushes refreshed version information to connected clients.
type VersionBroadcaster interface {
	BroadcastGameServerVersion(serverID string, version string, versionInfo *xylona.VersionInfo)
}

// BackupProgressBroadcaster pushes backup lifecycle updates to connected clients.
type BackupProgressBroadcaster interface {
	BroadcastBackupProgress(serverID string, progress *xylona.BackupProgress)
}

type versionRefreshCall struct {
	done chan struct{}
}

type backupCreateCall struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// VersionResolveOptions controls synchronous versus asynchronous version refresh behavior.
type VersionResolveOptions struct {
	ForceRefresh bool
	AllowAsync   bool
}

// Instance coordinates game server lifecycle, files, federation, and background jobs.
type Instance struct {
	ctx                  context.Context
	supervisorInstance   *supervisor.Instance
	serverQueriesInfoMap map[string]*xylona.ServerQuery
	serverQueriesMutex   *sync.RWMutex
	db                   *db.Connection
	federationMTLS       *federation.MTLS
	syncEngine           SyncEngine
	modManager           *modmanager.ModManager
	versionState         *versiontracker.VersionStateMap
	resolverConfig       versiontracker.ResolverConfig
	dummyTracker         *versiontracker.DummyTracker
	versionBroadcaster   VersionBroadcaster
	backupBroadcaster    BackupProgressBroadcaster
	versionInstalledTTL  time.Duration
	versionLatestTTL     time.Duration
	versionRefreshMu     sync.Mutex
	versionRefreshCalls  map[string]*versionRefreshCall
	backupCreateMu       sync.Mutex
	backupCreateCalls    map[string]*backupCreateCall
	localNodeID          string
	restartState         *restartStateMap
}

// SetSyncEngine sets the sync engine on the actions instance. This is called
// after both the actions instance and the sync engine are created to avoid
// circular constructor dependencies.
func (inst *Instance) SetSyncEngine(se SyncEngine) {
	inst.syncEngine = se
}

// VersionState returns the version state map used to track game server versions.
func (inst *Instance) VersionState() *versiontracker.VersionStateMap {
	return inst.versionState
}

// SetDummyTracker sets the dummy tracker used for E2E update failure simulation.
func (inst *Instance) SetDummyTracker(dt *versiontracker.DummyTracker) {
	inst.dummyTracker = dt
}

// SetVersionBroadcaster sets the websocket-facing broadcaster for version changes.
func (inst *Instance) SetVersionBroadcaster(b VersionBroadcaster) {
	inst.versionBroadcaster = b
}

// SetBackupProgressBroadcaster sets the websocket-facing broadcaster for backup progress.
func (inst *Instance) SetBackupProgressBroadcaster(b BackupProgressBroadcaster) {
	inst.backupBroadcaster = b
}

// StartAlertJobs launches the alert-related background goroutines. Call this
// after the node ID is known (i.e. after settings are loaded from the DB).
func (inst *Instance) StartAlertJobs(localNodeID string) {
	inst.localNodeID = localNodeID
	go inst.backgroundJobThresholdPoller(localNodeID)
	go inst.backgroundJobAlertHistoryPruner()
}

// CheckServerVersionByID loads the game server from the DB and runs a version check.
// Called from the RPC layer to trigger on-demand checks.
func (inst *Instance) CheckServerVersionByID(_ context.Context, gameServerID string) {
	gs, errGet := inst.db.GetGameServerByID(gameServerID)
	if errGet != nil {
		log.Warn().Err(errGet).Str("game_server_id", gameServerID).
			Msg("Version check: failed to get game server")
		return
	}
	eb := eventbus.Get()
	inst.checkServerVersion(gs, eb)
}

// NewInstance creates an actions instance and starts background polling jobs.
func NewInstance(ctx context.Context, database *db.Connection, supervisorInstance *supervisor.Instance, federationMTLS *federation.MTLS, modMgr *modmanager.ModManager, versionState *versiontracker.VersionStateMap, resolverConfig versiontracker.ResolverConfig) *Instance {
	inst := &Instance{
		ctx:                  ctx,
		supervisorInstance:   supervisorInstance,
		serverQueriesInfoMap: make(map[string]*xylona.ServerQuery),
		serverQueriesMutex:   &sync.RWMutex{},
		db:                   database,
		federationMTLS:       federationMTLS,
		modManager:           modMgr,
		versionState:         versionState,
		resolverConfig:       resolverConfig,
		versionInstalledTTL:  readVersionDurationEnv("XYLONA_VERSION_INSTALLED_TTL", 15*time.Second),
		versionLatestTTL:     readVersionDurationEnv("XYLONA_VERSION_LATEST_TTL", 2*time.Minute),
		versionRefreshCalls:  make(map[string]*versionRefreshCall),
		backupCreateCalls:    make(map[string]*backupCreateCall),
		restartState:         &restartStateMap{},
	}
	go inst.backgroundJobQueryAllGameServers()
	go inst.backgroundJobCheckModUpdates()
	go inst.backgroundJobCheckVersionUpdates()
	return inst
}

// InstallGameServer creates the server record, schedules install, and starts post-install startup.
func (inst *Instance) InstallGameServer(game *models.Game, gameServer *models.GameServer, owner *models.User) (*models.GameServer, error) {
	gameServerDir, errCreateGameServerDir := inst.createGameServerDirectory(gameServer, owner)
	if errCreateGameServerDir != nil {
		log.Error().Err(errCreateGameServerDir).Msg("Failed to create game server directory")
		return nil, errCreateGameServerDir
	}
	gameServer.Directory = gameServerDir

	t, errTx := inst.db.SQLDb.BeginTx(inst.ctx, nil)
	if errTx != nil {
		log.Error().Err(errTx).Msg("Failed to start transaction")
		errInstall := fmt.Errorf("actions: begin install transaction: %w", errTx)
		errCleanup := inst.cleanupFailedInstall("", gameServerDir)
		if errCleanup != nil {
			return nil, errors.Join(errInstall, errCleanup)
		}
		return nil, errInstall
	}
	tx := bob.NewTx(t)

	defer func() {
		_ = tx.Rollback(inst.ctx)
	}()

	node, errGetNode := inst.db.GetNodeByID(gameServer.NodeID)
	if errGetNode != nil {
		log.Error().Err(errGetNode).Msg("Failed to get node")
		errInstall := fmt.Errorf("actions: get node for install: %w", errGetNode)
		errCleanup := inst.cleanupFailedInstall("", gameServerDir)
		if errCleanup != nil {
			return nil, errors.Join(errInstall, errCleanup)
		}
		return nil, errInstall
	}

	newGameServer, errInsert := inst.db.InsertGameServer(tx, &models.GameServerSetter{
		ID:                        omit.From(uuid.NewString()),
		UserID:                    omit.From(owner.ID),
		Name:                      omit.From(gameServer.Name),
		GameID:                    omit.From(game.ID),
		StartArgsPatches:          omit.From("[]"),
		Status:                    omit.From(""),
		SetPlayers:                omit.From(gameServer.SetPlayers),
		MaxPlayers:                omit.From(gameServer.MaxPlayers),
		Map:                       omit.From(gameServer.Map),
		IP:                        omit.From(gameServer.IP),
		Port:                      omit.From(gameServer.Port),
		QueryPort:                 omit.From(gameServer.QueryPort),
		Directory:                 omit.From(gameServer.Directory),
		MaxMemoryMB:               omit.From(gameServer.MaxMemoryMB),
		BackupsEnabled:            omit.From(gameServer.BackupsEnabled),
		SteamGameServerLoginToken: omit.From(gameServer.SteamGameServerLoginToken),
		BackupDirectory:           omit.From(gameServer.BackupDirectory),
		MaxBackups:                omit.From(gameServer.MaxBackups),
		NodeID:                    omit.From(gameServer.NodeID),
	})
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Failed to insert game server")
		errInstall := fmt.Errorf("actions: insert game server: %w", errInsert)
		errCleanup := inst.cleanupFailedInstall("", gameServerDir)
		if errCleanup != nil {
			return nil, errors.Join(errInstall, errCleanup)
		}
		return nil, errInstall
	}
	newGameServer.R.Node = node

	errCommit := tx.Commit(inst.ctx)
	if errCommit != nil {
		log.Error().Str("Game Server ID", gameServer.ID).Err(errCommit).Msg("Failed to commit transaction")
		errInstall := fmt.Errorf("actions: commit install transaction: %w", errCommit)
		errCleanup := inst.cleanupFailedInstall("", gameServerDir)
		if errCleanup != nil {
			return nil, errors.Join(errInstall, errCleanup)
		}
		return nil, errInstall
	}

	installVars := placeholder.BuildVarsFromGameServer(newGameServer)
	installCommand := placeholder.Resolve(gameInstallCommand(game), installVars)
	baseCommand, args := splitCommandString(installCommand)
	preparedCommand := supervisor.PreparedCommand{
		ID:               newGameServer.ID,
		GameServerName:   newGameServer.Name,
		BaseCommand:      baseCommand,
		Args:             args,
		WorkingDirectory: newGameServer.Directory,
		User:             newGameServer.UserID,
		NodeID:           newGameServer.NodeID,
		GameServerID:     &newGameServer.ID,
		Status:           xylona.Status_INSTALLING,
		ServiceID:        newGameServer.GameID,
		CallbackFunction: func(_ *supervisor.Command) {
			errPostInstall := postInstallStep(newGameServer)
			if errPostInstall != nil {
				log.Error().Err(errPostInstall).Msg("Failed to perform post install step")
				return
			}
			inst.StartGameServer(newGameServer)
		},
	}

	if gameInstallCommandType(game) == CommandTypeInternal {
		preparedCommand.InternalCommand = true
		preparedCommand.InternalGameServer = newGameServer
		preparedCommand.GameID = &game.ID
	}

	_, err := inst.supervisorInstance.StartCommand(preparedCommand)
	if err != nil {
		log.Error().Err(err).Msg("Failed to start install command")
		errInstall := fmt.Errorf("actions: start install command: %w", err)
		errCleanup := inst.cleanupFailedInstall(newGameServer.ID, newGameServer.Directory)
		if errCleanup != nil {
			return nil, errors.Join(errInstall, errCleanup)
		}
		return nil, errInstall
	}

	eb := eventbus.Get()
	eb.Publish("game_server_created", newGameServer)

	return newGameServer, nil
}

func (inst *Instance) cleanupFailedInstall(gameServerID string, gameServerDir string) error {
	var errCleanup error

	if gameServerID != "" {
		errDeleteGameServer := inst.db.DeleteGameServer(gameServerID)
		if errDeleteGameServer != nil {
			log.Error().Err(errDeleteGameServer).Str("game_server_id", gameServerID).Msg("Failed to delete game server after install failure")
			errCleanup = errors.Join(errCleanup, fmt.Errorf("actions: delete failed install game server: %w", errDeleteGameServer))
		}
	}

	if gameServerDir != "" {
		errDeleteGameServerFiles := os.RemoveAll(gameServerDir)
		if errDeleteGameServerFiles != nil {
			log.Error().Err(errDeleteGameServerFiles).Str("game_server_dir", gameServerDir).Msg("Failed to remove game server directory after install failure")
			errCleanup = errors.Join(errCleanup, fmt.Errorf("actions: delete failed install directory: %w", errDeleteGameServerFiles))
		}
	}

	return errCleanup
}

func (inst *Instance) reportStartFailure(gameServer *models.GameServer, message string) {
	log.Error().Str("game_server_id", gameServer.ID).Msg(message)

	cmd := inst.supervisorInstance.GetCommandByIDOrCreateShell(gameServer.ID)
	cmd.SendOutput(message)
}

func (inst *Instance) resolveStructuredStartCommand(gameServer *models.GameServer) (string, []string, error) {
	if gameServer.R.Game == nil {
		return "", nil, errGameRelationNotLoaded
	}

	templateJSON := strings.TrimSpace(gameStartArgsTemplate(gameServer.R.Game))
	if templateJSON == "" {
		return "", nil, errStartCommandTemplateMissing
	}

	template, errTemplate := startargs.ParseTemplate(templateJSON)
	if errTemplate != nil {
		return "", nil, fmt.Errorf("parse start args template: %w", errTemplate)
	}

	patches, errPatches := startargs.ParsePatches(gameServer.StartArgsPatches)
	if errPatches != nil {
		return "", nil, fmt.Errorf("parse start arg patches: %w", errPatches)
	}

	startVars := placeholder.BuildVarsFromGameServer(gameServer)
	baseCommand := placeholder.ResolveToken(gameBaseCommand(gameServer.R.Game), startVars)
	if strings.TrimSpace(baseCommand) == "" {
		return "", nil, errBaseCommandMissing
	}

	args, _, errResolve := startargs.ResolveArgs(template, patches, startVars)
	if errResolve != nil {
		return "", nil, fmt.Errorf("resolve start args: %w", errResolve)
	}

	blocklistEntries, errBlocklist := startargs.ParseBlocklist(gameServer.R.Game.StartArgBlocklist)
	if errBlocklist != nil {
		return "", nil, fmt.Errorf("parse start arg blocklist: %w", errBlocklist)
	}

	compiledBlocklist, errCompile := startargs.CompileBlocklist(blocklistEntries)
	if errCompile != nil {
		return "", nil, fmt.Errorf("compile start arg blocklist: %w", errCompile)
	}

	violation := compiledBlocklist.Validate(args)
	if violation != nil {
		return "", nil, fmt.Errorf("blocked start argument %q: %s", violation.Token, violation.Reason)
	}

	return baseCommand, args, nil
}

func postInstallStep(gameServer *models.GameServer) error {
	switch gameServer.GameID {
	case "minecraft":
		return postMinecraftInstall(gameServer)
	case "7_days_to_die":
		return post7DaysToDieInstall(gameServer)
	default:
		log.Debug().Str("Game ID", gameServer.GameID).Msg("No post install step")
		return nil
	}
}

// StartGameServer resolves and starts the configured runtime command for a server.
func (inst *Instance) StartGameServer(gameServer *models.GameServer) {
	// Run pre-start config enforcement before launching the process.
	inst.runConfigPreStart(gameServer)
	// Run mod auto-updates before launching the process.
	inst.runModAutoUpdates(gameServer)

	baseCommand, args, errResolve := inst.resolveStructuredStartCommand(gameServer)
	if errResolve != nil {
		inst.reportStartFailure(gameServer, errResolve.Error())
		return
	}

	preparedCommand := supervisor.PreparedCommand{
		ID:               gameServer.ID,
		GameServerName:   gameServer.Name,
		BaseCommand:      baseCommand,
		Args:             args,
		WorkingDirectory: gameServer.Directory,
		User:             gameServer.UserID,
		NodeID:           gameServer.NodeID,
		GameServerID:     &gameServer.ID,
		Status:           xylona.Status_ONLINE,
		ServiceID:        gameServer.GameID,
		CallbackFunction: func(cmd *supervisor.Command) {
			inst.handleServerExit(cmd, gameServer.ID)
		},
	}
	log.Debug().Msg("Checking input method during startup action")
	if gameServer.GameID == "7_days_to_die" {
		log.Debug().Msg("Found 7 Days to Die. Setting input method to telnet")
		preparedCommand.InputMethod = supervisor.InputMethod{
			Type: supervisor.InputTypeTelnet,
			TelnetCredentials: &supervisor.TelnetCredentials{
				Port:     int(gameServer.Port + 1),
				Password: "",
			},
		}
	}

	_, err := inst.supervisorInstance.StartCommand(preparedCommand)
	if err != nil {
		log.Error().Err(err).Str("game_server_id", gameServer.ID).
			Msg("Failed to start game server")
		inst.supervisorInstance.SendConsoleOutput(gameServer.ID,
			"Failed to start server: "+err.Error())
		return
	}
	inst.restartState.recordStarted(gameServer.ID)
}

func (inst *Instance) runConfigPreStart(gameServer *models.GameServer) {
	schemasJSON, errGet := inst.db.GetGameConfigSchemas(gameServer.GameID)
	if errGet != nil {
		log.Debug().Err(errGet).Str("game_id", gameServer.GameID).
			Msg("Pre-start: could not get config schemas, skipping")
		return
	}
	if schemasJSON == "" {
		return
	}

	resolver := cfgschema.ServerSettingsResolver(gameServer.IP, gameServer.Port, gameServer.QueryPort)
	cfgschema.RunPreStart(gameServer.Directory, schemasJSON, resolver)
}

func (inst *Instance) runModAutoUpdates(gameServer *models.GameServer) {
	if inst.modManager == nil {
		return
	}

	mods, errGet := inst.db.GetInstalledModsByGameServerID(gameServer.ID)
	if errGet != nil {
		log.Warn().Err(errGet).Str("game_server_id", gameServer.ID).
			Msg("Pre-start mod auto-update: failed to get installed mods, skipping")
		return
	}

	hasAutoUpdate := false
	for _, m := range mods {
		if m.AutoUpdate == 1 && m.Enabled == 1 && m.PinnedVersion.IsNull() {
			hasAutoUpdate = true
			break
		}
	}
	if !hasAutoUpdate {
		return
	}

	// Use the shell command slot so messages appear in the console before
	// the server process takes over the same slot.
	cmd := inst.supervisorInstance.GetCommandByIDOrCreateShell(gameServer.ID)

	statusFn := func(msg string) {
		formatted := fmt.Sprintf("[%s] [Xylona]: %s", time.Now().Format("2006-01-02 15:04:05"), msg)
		cmd.SendOutput(formatted)
		log.Info().Str("game_server_id", gameServer.ID).Msg(msg)
	}

	errAutoUpdate := inst.modManager.RunAutoUpdates(inst.ctx, gameServer.ID, "", gameServer.Directory, statusFn)
	if errAutoUpdate != nil {
		log.Warn().Err(errAutoUpdate).Str("game_server_id", gameServer.ID).
			Msg("Pre-start mod auto-update failed; continuing server startup")
	}
}

// StopGameServer stops a running game server using its configured stop command.
func (inst *Instance) StopGameServer(gameServer *models.GameServer) {
	gameServerCommand, err := inst.supervisorInstance.GetCommandByID(gameServer.ID)
	if err != nil {
		if errors.Is(err, supervisor.ErrCommandDoesNotExist) {
			log.Error().Err(err).Msg("Failed to get game server command")
		}
		return
	}
	gameServerCommand.Stop(gameStopCommand(gameServer.R.Game))
}

// UpdateGameServer starts the configured update flow for a game server.
func (inst *Instance) UpdateGameServer(gameServer *models.GameServer) error {
	gameServerCommand, errGetCommand := inst.supervisorInstance.GetCommandByID(gameServer.ID)
	if errGetCommand == nil {
		status := gameServerCommand.Status()
		if status != xylona.Status_OFFLINE && status != xylona.Status_UNKNOWN {
			log.Info().Str("Game Server ID", gameServer.ID).Msg("Stopping game server before update")
			gameServerCommand.Stop(gameStopCommand(gameServer.R.Game))
		}
	}

	handled, errMinecraft := inst.tryUpdateMinecraftServerSoftware(gameServer)
	if handled || errMinecraft != nil {
		return errMinecraft
	}

	updateCmd := gameUpdateCommand(gameServer.R.Game)
	internalUpdate := gameUpdateCommandType(gameServer.R.Game) == CommandTypeInternal
	if !internalUpdate && updateCmd == "" {
		_, internalUpdate = internal.GetGame(gameServer.GameID)
	}
	if updateCmd == "" && !internalUpdate {
		log.Warn().Str("Game Server ID", gameServer.ID).Msg("No update command configured for this game")
		return ErrGameUpdateNotConfigured
	}
	if internalUpdate {
		if _, exists := internal.GetGame(gameServer.GameID); !exists {
			log.Warn().Str("Game Server ID", gameServer.ID).Str("Game ID", gameServer.GameID).Msg("No internal updater registered for this game")
			return ErrInternalGameUpdateMissing
		}
	}

	preparedCommand := supervisor.PreparedCommand{
		ID:               gameServer.ID,
		GameServerName:   gameServer.Name,
		WorkingDirectory: gameServer.Directory,
		User:             gameServer.UserID,
		NodeID:           gameServer.NodeID,
		GameServerID:     &gameServer.ID,
		Status:           xylona.Status_UPDATING,
		ServiceID:        gameServer.GameID,
		CallbackFunction: func(_ *supervisor.Command) {
			log.Info().Str("Game Server ID", gameServer.ID).Msg("Game server update completed")
		},
	}
	if !internalUpdate {
		updateCommand := appendSteamBranchToUpdateCommand(
			placeholder.Resolve(updateCmd, placeholder.BuildVarsFromGameServer(gameServer)),
			gameServer.Branch,
		)
		preparedCommand.BaseCommand, preparedCommand.Args = splitCommandString(updateCommand)
	}

	if internalUpdate {
		preparedCommand.InternalCommand = true
		preparedCommand.InternalGameServer = gameServer
		gameID := gameServer.GameID
		preparedCommand.GameID = &gameID
	}

	_, errStart := inst.supervisorInstance.StartCommand(preparedCommand)
	if errStart != nil {
		log.Error().Err(errStart).Str("Game Server ID", gameServer.ID).Msg("Failed to start update command")
		return fmt.Errorf("actions: start update command: %w", errStart)
	}
	return nil
}

func normalizeSteamBranch(branch string) string {
	return versiontracker.NormalizeSteamBranch(branch)
}

func appendSteamBranchToUpdateCommand(command string, branch string) string {
	if !strings.Contains(strings.ToLower(command), "steamcmd") {
		return command
	}

	normalizedBranch := normalizeSteamBranch(branch)
	if normalizedBranch == "public" {
		return command
	}

	if strings.Contains(strings.ToLower(command), " -beta ") {
		return command
	}

	if reSteamBranchableUpdate.MatchString(command) {
		return reSteamBranchableUpdate.ReplaceAllString(command, fmt.Sprintf("${1} -beta %s", normalizedBranch))
	}

	return command + " -beta " + normalizedBranch
}

// PersistSteamBranchSelection stores the selected Steam branch on the server record.
func (inst *Instance) PersistSteamBranchSelection(gameServer *models.GameServer, branch string) error {
	normalizedBranch := normalizeSteamBranch(branch)
	gameServer.Branch = normalizedBranch

	if inst.db == nil {
		return nil
	}

	updated, errUpdate := inst.db.UpdateGameServer(inst.db.DB, &models.GameServerSetter{
		ID:     omit.From(gameServer.ID),
		Branch: omit.From(normalizedBranch),
	})
	if errUpdate != nil {
		return fmt.Errorf("actions: persist steam branch selection: %w", errUpdate)
	}

	gameServer.Branch = updated.Branch
	return nil
}

// ReadGameServerBuffer returns the buffered console output for a running server.
func (inst *Instance) ReadGameServerBuffer(gameServer *models.GameServer) string {
	gameServerCommand, err := inst.supervisorInstance.GetCommandByID(gameServer.ID)
	if err != nil {
		return ""
	}
	return gameServerCommand.GetOutputBuffer()
}

// RemoveGameServer deletes a server and optionally tolerates file cleanup failures.
func (inst *Instance) RemoveGameServer(gameServer *models.GameServer, force bool) error {
	err := inst.PurgeAllGameServerFiles(gameServer)
	if err != nil {
		if !force {
			log.Error().Err(err).Msg("Failed to remove game server files")
			return err
		}
		log.Warn().Err(err).Msg("Failed to remove game server files")
	}
	err = inst.db.DeleteGameServer(gameServer.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to remove game server from database")
		return fmt.Errorf("actions: delete game server: %w", err)
	}
	eb := eventbus.Get()
	eb.Publish("game_server_removed", gameServer)
	return nil
}
