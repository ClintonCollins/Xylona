// Package actions contains the control-plane business logic for game server
// lifecycle, file operations, synchronization, and background jobs.
package actions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/noderegistry"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/placeholder"

	"github.com/ClintonCollins/Xylona/db"
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
	nodeRegistry         *noderegistry.Registry
	serverQueriesInfoMap map[string]*xylona.ServerQuery
	serverQueriesMutex   *sync.RWMutex
	db                   *db.Connection
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
	exitHooks            *exitHookRegistry
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
func NewInstance(ctx context.Context, database *db.Connection, supervisorInstance *supervisor.Instance, nodeRegistry *noderegistry.Registry, modMgr *modmanager.ModManager, versionState *versiontracker.VersionStateMap, resolverConfig versiontracker.ResolverConfig) *Instance {
	inst := &Instance{
		ctx:                  ctx,
		supervisorInstance:   supervisorInstance,
		nodeRegistry:         nodeRegistry,
		serverQueriesInfoMap: make(map[string]*xylona.ServerQuery),
		serverQueriesMutex:   &sync.RWMutex{},
		db:                   database,
		modManager:           modMgr,
		versionState:         versionState,
		resolverConfig:       resolverConfig,
		versionInstalledTTL:  readVersionDurationEnv("XYLONA_VERSION_INSTALLED_TTL", 15*time.Second),
		versionLatestTTL:     readVersionDurationEnv("XYLONA_VERSION_LATEST_TTL", 2*time.Minute),
		versionRefreshCalls:  make(map[string]*versionRefreshCall),
		backupCreateCalls:    make(map[string]*backupCreateCall),
		restartState:         &restartStateMap{},
		exitHooks:            newExitHookRegistry(),
	}
	go inst.backgroundJobQueryAllGameServers()
	go inst.backgroundJobCheckModUpdates()
	go inst.backgroundJobCheckVersionUpdates()

	// Start the eventbus-driven auto-restart subscriber. Both embedded and
	// remote nodes publish status-change events; this one subscriber drives
	// restart decisions uniformly.
	inst.startAutoRestartSubscriber(ctx)

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
		errCleanup := inst.cleanupFailedInstall("", gameServerDir, gameServer.NodeID)
		if errCleanup != nil {
			return nil, errors.Join(errInstall, errCleanup)
		}
		return nil, errInstall
	}
	tx := bob.NewTx(t)

	defer func() {
		_ = tx.Rollback(inst.ctx)
	}()

	nodeRow, errGetNode := inst.db.GetNodeByID(gameServer.NodeID)
	if errGetNode != nil {
		log.Error().Err(errGetNode).Msg("Failed to get node")
		errInstall := fmt.Errorf("actions: get node for install: %w", errGetNode)
		errCleanup := inst.cleanupFailedInstall("", gameServerDir, gameServer.NodeID)
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
		errCleanup := inst.cleanupFailedInstall("", gameServerDir, gameServer.NodeID)
		if errCleanup != nil {
			return nil, errors.Join(errInstall, errCleanup)
		}
		return nil, errInstall
	}
	newGameServer.R.Node = nodeRow

	errCommit := tx.Commit(inst.ctx)
	if errCommit != nil {
		log.Error().Str("Game Server ID", gameServer.ID).Err(errCommit).Msg("Failed to commit transaction")
		errInstall := fmt.Errorf("actions: commit install transaction: %w", errCommit)
		errCleanup := inst.cleanupFailedInstall("", gameServerDir, gameServer.NodeID)
		if errCleanup != nil {
			return nil, errors.Join(errInstall, errCleanup)
		}
		return nil, errInstall
	}

	nodeOS := inst.resolveNodeOS(inst.ctx, newGameServer.NodeID)
	installVars := placeholder.BuildVarsFromGameServer(newGameServer)
	installCommand := placeholder.Resolve(gameInstallCommand(game, nodeOS), installVars)
	baseCommand, args := splitCommandString(installCommand)

	client, errClient := inst.resolveNodeClient(newGameServer.NodeID)
	if errClient != nil {
		errInstall := fmt.Errorf("actions: resolve node client for install: %w", errClient)
		errCleanup := inst.cleanupFailedInstall(newGameServer.ID, newGameServer.Directory, newGameServer.NodeID)
		if errCleanup != nil {
			return nil, errors.Join(errInstall, errCleanup)
		}
		return nil, errInstall
	}

	installCfg := node.ProcessConfig{
		ID:               newGameServer.ID,
		Name:             newGameServer.Name,
		BaseCommand:      baseCommand,
		Args:             args,
		WorkingDirectory: newGameServer.Directory,
		User:             newGameServer.UserID,
		NodeID:           newGameServer.NodeID,
		ServiceID:        newGameServer.GameID,
	}
	if gameInstallCommandType(game, nodeOS) == CommandTypeInternal {
		installCfg.InternalCommand = true
		installCfg.InternalGameServerID = newGameServer.ID
		installCfg.InternalGameID = game.ID
		installCfg.InternalGameServer = newGameServer
	}

	// Register a one-shot exit hook so post-install runs + the server starts
	// once the install process exits. For embedded nodes this fires via the
	// supervisor's own status event; for remote nodes it fires via the
	// event bridge. Either way, the post-install code path is identical.
	installedGS := newGameServer
	inst.exitHooks.set(newGameServer.ID, func(event eventbus.StatusChangedEvent) {
		if event.ExitCode != 0 {
			log.Warn().Int("exit_code", event.ExitCode).Str("game_server_id", installedGS.ID).
				Msg("Install command exited with non-zero; skipping post-install")
			return
		}
		errPostInstall := postInstallStep(installedGS)
		if errPostInstall != nil {
			log.Error().Err(errPostInstall).Msg("Failed to perform post install step")
			return
		}
		inst.StartGameServer(installedGS)
	})

	_, errStart := client.StartProcess(inst.ctx, installCfg, xylona.Status_INSTALLING)
	if errStart != nil && !errors.Is(errStart, nodeclient.ErrRemoteStartProcessHandle) {
		inst.exitHooks.clear(newGameServer.ID)
		log.Error().Err(errStart).Msg("Failed to start install command")
		errInstall := fmt.Errorf("actions: start install command: %w", errStart)
		errCleanup := inst.cleanupFailedInstall(newGameServer.ID, newGameServer.Directory, newGameServer.NodeID)
		if errCleanup != nil {
			return nil, errors.Join(errInstall, errCleanup)
		}
		return nil, errInstall
	}

	eb := eventbus.Get()
	eb.Publish("game_server_created", newGameServer)

	return newGameServer, nil
}

func (inst *Instance) cleanupFailedInstall(gameServerID string, gameServerDir string, nodeID string) error {
	var errCleanup error

	resolvedNodeID := nodeID
	if resolvedNodeID == "" && gameServerID != "" {
		gameServer, errGetGameServer := inst.db.GetGameServerByID(gameServerID)
		if errGetGameServer == nil {
			resolvedNodeID = gameServer.NodeID
		}
		if errGetGameServer != nil && !errors.Is(errGetGameServer, sql.ErrNoRows) {
			log.Error().Err(errGetGameServer).Str("game_server_id", gameServerID).Msg("Failed to load game server for install cleanup")
			errCleanup = errors.Join(errCleanup, fmt.Errorf("actions: load failed install game server: %w", errGetGameServer))
		}
	}

	if gameServerID != "" {
		errDeleteGameServer := inst.db.DeleteGameServer(gameServerID)
		if errDeleteGameServer != nil {
			log.Error().Err(errDeleteGameServer).Str("game_server_id", gameServerID).Msg("Failed to delete game server after install failure")
			errCleanup = errors.Join(errCleanup, fmt.Errorf("actions: delete failed install game server: %w", errDeleteGameServer))
		}
	}

	if gameServerDir != "" {
		errDeleteGameServerFiles := inst.deleteGameServerDirectory(resolvedNodeID, gameServerDir)
		if errDeleteGameServerFiles != nil {
			log.Error().Err(errDeleteGameServerFiles).Str("game_server_dir", gameServerDir).Msg("Failed to remove game server directory after install failure")
			errCleanup = errors.Join(errCleanup, fmt.Errorf("actions: delete failed install directory: %w", errDeleteGameServerFiles))
		}
	}

	return errCleanup
}

func (inst *Instance) reportStartFailure(gameServer *models.GameServer, message string) {
	log.Error().Str("game_server_id", gameServer.ID).Msg(message)
	inst.sendConsoleLine(gameServer, message)
}

func (inst *Instance) resolveStructuredStartCommand(gameServer *models.GameServer) (string, []string, error) {
	if gameServer.R.Game == nil {
		return "", nil, errGameRelationNotLoaded
	}

	errEnsureExecutable := inst.ensureMinecraftServerExecutable(gameServer)
	if errEnsureExecutable != nil {
		return "", nil, errEnsureExecutable
	}

	nodeOS := inst.resolveNodeOS(inst.ctx, gameServer.NodeID)
	templateJSON := strings.TrimSpace(gameStartArgsTemplate(gameServer.R.Game, nodeOS))
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
	if gameServer.GameID == "minecraft" {
		serverExecutable := strings.TrimSpace(gameServer.ServerExecutable.GetOr(""))
		if serverExecutable == "" {
			return "", nil, errors.New("minecraft server executable is not configured. set it in server settings or place the server jar in the server root")
		}
	}
	baseCommand := placeholder.ResolveToken(gameBaseCommand(gameServer.R.Game, nodeOS), startVars)
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

// StartGameServer resolves and starts the configured runtime command for a
// server. Routes through NodeClient so both embedded and remote nodes launch
// processes via the same surface; exit handling is driven by the status-
// change eventbus subscriber.
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

	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		log.Error().Err(errClient).Str("game_server_id", gameServer.ID).
			Str("node_id", gameServer.NodeID).Msg("Failed to resolve node client for start")
		inst.reportStartFailure(gameServer, "Failed to reach target node: "+errClient.Error())
		return
	}

	cfg := node.ProcessConfig{
		ID:               gameServer.ID,
		Name:             gameServer.Name,
		BaseCommand:      baseCommand,
		Args:             args,
		WorkingDirectory: gameServer.Directory,
		User:             gameServer.UserID,
		NodeID:           gameServer.NodeID,
		ServiceID:        gameServer.GameID,
	}

	if gameServer.GameID == "7_days_to_die" {
		log.Debug().Msg("Found 7 Days to Die. Setting input method to telnet")
		cfg.InputTelnet = &node.TelnetInput{
			Port:     int(gameServer.Port + 1),
			Password: "",
		}
	}

	_, errStart := client.StartProcess(inst.ctx, cfg, xylona.Status_ONLINE)
	if errStart != nil && !errors.Is(errStart, nodeclient.ErrRemoteStartProcessHandle) {
		log.Error().Err(errStart).Str("game_server_id", gameServer.ID).
			Msg("Failed to start game server")
		inst.reportStartFailure(gameServer, "Failed to start server: "+errStart.Error())
		return
	}
	inst.restartState.recordStarted(gameServer.ID)
}

// resolveNodeClient looks up the NodeClient for a node ID. When the
// registry is not configured (tests, migrations), falls back to a fresh
// in-process client around the supervisor so callers still get working
// semantics. Returns a plain Go error (not a connect error) since actions
// callers don't want connect types.
func (inst *Instance) resolveNodeClient(nodeID string) (nodeclient.NodeClient, error) {
	if inst.nodeRegistry != nil {
		client, errGet := inst.nodeRegistry.Get(nodeID)
		if errGet == nil {
			return client, nil
		}
		// Registry configured but no client for that ID — fall through to the
		// supervisor fallback if we have one, otherwise bubble up the error.
		if inst.supervisorInstance == nil {
			return nil, fmt.Errorf("actions: resolve node client for %q: %w", nodeID, errGet)
		}
	}
	if inst.supervisorInstance == nil {
		return nil, errors.New("actions: neither node registry nor supervisor is configured")
	}
	embedded := node.New(inst.ctx, inst.supervisorInstance, inst.db)
	return nodeclient.NewInProcessClient(nodeID, embedded), nil
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

	// Route status messages through the NodeClient so they reach the right
	// console buffer for both embedded and remote game servers.
	statusFn := func(msg string) {
		formatted := fmt.Sprintf("[%s] [Xylona]: %s", time.Now().Format("2006-01-02 15:04:05"), msg)
		inst.sendConsoleLine(gameServer, formatted)
		log.Info().Str("game_server_id", gameServer.ID).Msg(msg)
	}

	errAutoUpdate := inst.modManager.RunAutoUpdates(inst.ctx, gameServer.ID, "", gameServer.Directory, statusFn)
	if errAutoUpdate != nil {
		log.Warn().Err(errAutoUpdate).Str("game_server_id", gameServer.ID).
			Msg("Pre-start mod auto-update failed; continuing server startup")
	}
}

// StopGameServer stops a running game server using its configured stop
// command. Routes through the NodeClient for the server's owning node so
// embedded and remote servers stop on the same code path.
func (inst *Instance) StopGameServer(gameServer *models.GameServer) {
	nodeOS := inst.resolveNodeOS(inst.ctx, gameServer.NodeID)
	if inst.nodeRegistry != nil {
		client, errGet := inst.nodeRegistry.Get(gameServer.NodeID)
		if errGet == nil {
			errStop := client.StopProcess(inst.ctx, gameServer.ID, gameStopCommand(gameServer.R.Game, nodeOS))
			if errStop != nil && !errors.Is(errStop, supervisor.ErrCommandDoesNotExist) {
				log.Error().Err(errStop).Str("node_id", gameServer.NodeID).
					Msg("Failed to stop game server command")
			}
			return
		}
		log.Warn().Err(errGet).Str("node_id", gameServer.NodeID).
			Msg("StopGameServer: node client unavailable, falling back to supervisor")
	}

	// Fallback: direct supervisor access if the registry is not wired up
	// (should only occur in tests that construct Instance directly). The
	// supervisor always runs the controller's own OS, so OperatingSystem is
	// the correct flavor here.
	if inst.supervisorInstance == nil {
		return
	}
	gameServerCommand, err := inst.supervisorInstance.GetCommandByID(gameServer.ID)
	if err != nil {
		if errors.Is(err, supervisor.ErrCommandDoesNotExist) {
			log.Error().Err(err).Msg("Failed to get game server command")
		}
		return
	}
	gameServerCommand.Stop(gameStopCommand(gameServer.R.Game, OperatingSystem))
}

// UpdateGameServer starts the configured update flow for a game server.
// Routes through NodeClient for both embedded and remote nodes.
func (inst *Instance) UpdateGameServer(gameServer *models.GameServer) error {
	// Stop if currently running. Use snapshot status rather than direct
	// supervisor lookup so remote-node servers are handled uniformly.
	currentStatus := inst.currentProcessStatus(gameServer)
	if currentStatus != xylona.Status_OFFLINE && currentStatus != xylona.Status_UNKNOWN {
		log.Info().Str("Game Server ID", gameServer.ID).Msg("Stopping game server before update")
		inst.StopGameServer(gameServer)
	}

	handled, errMinecraft := inst.tryUpdateMinecraftServerSoftware(gameServer)
	if handled || errMinecraft != nil {
		return errMinecraft
	}

	nodeOS := inst.resolveNodeOS(inst.ctx, gameServer.NodeID)
	updateCmd := gameUpdateCommand(gameServer.R.Game, nodeOS)
	internalUpdate := gameUpdateCommandType(gameServer.R.Game, nodeOS) == CommandTypeInternal
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

	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		return fmt.Errorf("actions: resolve node client for update: %w", errClient)
	}

	updateCfg := node.ProcessConfig{
		ID:               gameServer.ID,
		Name:             gameServer.Name,
		WorkingDirectory: gameServer.Directory,
		User:             gameServer.UserID,
		NodeID:           gameServer.NodeID,
		ServiceID:        gameServer.GameID,
	}
	if !internalUpdate {
		updateCommand := appendSteamBranchToUpdateCommand(
			placeholder.Resolve(updateCmd, placeholder.BuildVarsFromGameServer(gameServer)),
			gameServer.Branch,
		)
		updateCfg.BaseCommand, updateCfg.Args = splitCommandString(updateCommand)
	}
	if internalUpdate {
		updateCfg.InternalCommand = true
		updateCfg.InternalGameServerID = gameServer.ID
		updateCfg.InternalGameID = gameServer.GameID
		updateCfg.InternalGameServer = gameServer
	}

	_, errStart := client.StartProcess(inst.ctx, updateCfg, xylona.Status_UPDATING)
	if errStart != nil && !errors.Is(errStart, nodeclient.ErrRemoteStartProcessHandle) {
		log.Error().Err(errStart).Str("Game Server ID", gameServer.ID).Msg("Failed to start update command")
		return fmt.Errorf("actions: start update command: %w", errStart)
	}
	return nil
}

// CurrentStatus returns the current xylona.Status for the given game
// server, routing through the NodeClient so embedded and remote nodes
// resolve identically. Exposed for callers outside the actions package
// (e.g. the scheduler) that need status without depending on the
// supervisor directly.
func (inst *Instance) CurrentStatus(gameServer *models.GameServer) xylona.Status {
	return inst.currentProcessStatus(gameServer)
}

// SendConsoleInput writes a single line of input to the running game
// server's console via the appropriate NodeClient. Returns an error when
// the node is unreachable or the input write fails. Callers that want
// controller-generated chatter to appear in the console buffer should use
// sendConsoleLine instead.
func (inst *Instance) SendConsoleInput(gameServer *models.GameServer, input string) error {
	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		return fmt.Errorf("actions: send console input: %w", errClient)
	}
	errSend := client.SendConsoleInput(inst.ctx, gameServer.ID, input)
	if errSend != nil {
		return fmt.Errorf("actions: send console input: %w", errSend)
	}
	return nil
}

// currentProcessStatus returns the current xylona.Status for the game
// server's process, preferring the NodeClient path (works for both embedded
// and remote) and falling back to UNKNOWN when the status cannot be
// determined.
func (inst *Instance) currentProcessStatus(gameServer *models.GameServer) xylona.Status {
	if inst.nodeRegistry != nil {
		client, errGet := inst.nodeRegistry.Get(gameServer.NodeID)
		if errGet == nil {
			snap, found, errSnap := client.GetProcessSnapshot(inst.ctx, gameServer.ID)
			if errSnap == nil && found && snap != nil {
				status, ok := xylona.Status_value[snap.Status]
				if ok {
					return xylona.Status(status)
				}
				return xylona.Status_UNKNOWN
			}
			if errSnap == nil && !found {
				return xylona.Status_OFFLINE
			}
		}
	}
	if inst.supervisorInstance != nil {
		cmd, errCmd := inst.supervisorInstance.GetCommandByID(gameServer.ID)
		if errCmd == nil {
			return cmd.Status()
		}
	}
	return xylona.Status_UNKNOWN
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

// ReadGameServerBuffer returns the buffered console output for a running
// server, routing through NodeClient for the server's owning node so remote
// and embedded paths are identical.
func (inst *Instance) ReadGameServerBuffer(gameServer *models.GameServer) string {
	if inst.nodeRegistry != nil {
		client, errGet := inst.nodeRegistry.Get(gameServer.NodeID)
		if errGet == nil {
			chunk, errRead := client.ReadConsoleBuffer(inst.ctx, gameServer.ID)
			if errRead != nil {
				return ""
			}
			return chunk.Data
		}
	}

	// Fallback: direct supervisor access for tests that construct Instance
	// without a registry.
	if inst.supervisorInstance == nil {
		return ""
	}
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
