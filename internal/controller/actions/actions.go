// Package actions contains the control-plane business logic for game server
// lifecycle, file operations, synchronization, and background jobs.
package actions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"

	"github.com/ClintonCollins/Xylona/internal/controller/readiness"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
	"github.com/ClintonCollins/Xylona/internal/modmanager"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/placeholder"
	"github.com/ClintonCollins/Xylona/internal/startargs"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/pkg/cfgschema"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
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
	// ErrGameServerOperationInProgress is returned when the same server already has an active lifecycle operation.
	ErrGameServerOperationInProgress = errors.New("game server lifecycle operation is already in progress")
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

// Instance coordinates game server lifecycle, files, remote nodes, and background jobs.
type Instance struct {
	ctx                   context.Context
	embeddedNodeClient    nodeclient.NodeClient
	nodeRegistry          *noderegistry.Registry
	serverQueriesInfoMap  map[string]*xylona.ServerQuery
	serverQueriesMutex    *sync.RWMutex
	gameServerStatuses    map[string]xylona.Status
	gameServerStatusMutex sync.RWMutex
	palworldMaps          map[string]PalworldMapState
	palworldMapsMutex     sync.RWMutex
	queryTelemetry        gameServerQueryTelemetryStore
	db                    *db.Connection
	modManager            *modmanager.ModManager
	versionState          *versiontracker.VersionStateMap
	resolverConfig        versiontracker.ResolverConfig
	dummyTracker          *versiontracker.DummyTracker
	versionBroadcaster    VersionBroadcaster
	backupBroadcaster     BackupProgressBroadcaster
	versionInstalledTTL   time.Duration
	versionLatestTTL      time.Duration
	versionRefreshMu      sync.Mutex
	versionRefreshCalls   map[string]*versionRefreshCall
	backupCreateMu        sync.Mutex
	backupCreateCalls     map[string]*backupCreateCall
	gameServerOperationMu sync.Mutex
	activeGameServerOps   map[string]struct{}
	hytaleClient          readiness.HytaleClient
	hytaleLaunchLocks     *readiness.HytaleLaunchLocks
	localNodeID           string
	restartState          *restartStateMap
	exitHooks             *exitHookRegistry
	intentionalStops      intentionalStopRegistry
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

// SetHytaleClient overrides the Hytale client used by readiness launch prep.
func (inst *Instance) SetHytaleClient(client readiness.HytaleClient) {
	if client == nil {
		client = readiness.NewHytaleHTTPClient(nil)
	}
	inst.hytaleClient = client
}

// StartAlertJobs launches the alert-related background goroutines. Call this
// after the node ID is known (i.e. after settings are loaded from the DB).
func (inst *Instance) StartAlertJobs(localNodeID string) {
	inst.localNodeID = localNodeID
	go inst.backgroundJobThresholdPoller()
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
func NewInstance(ctx context.Context, database *db.Connection, embeddedNodeClient nodeclient.NodeClient, nodeRegistry *noderegistry.Registry, modMgr *modmanager.ModManager, versionState *versiontracker.VersionStateMap, resolverConfig versiontracker.ResolverConfig) *Instance {
	inst := &Instance{
		ctx:                  ctx,
		embeddedNodeClient:   embeddedNodeClient,
		nodeRegistry:         nodeRegistry,
		serverQueriesInfoMap: make(map[string]*xylona.ServerQuery),
		serverQueriesMutex:   &sync.RWMutex{},
		gameServerStatuses:   make(map[string]xylona.Status),
		palworldMaps:         make(map[string]PalworldMapState),
		db:                   database,
		modManager:           modMgr,
		versionState:         versionState,
		resolverConfig:       resolverConfig,
		versionInstalledTTL:  readVersionDurationEnv("XYLONA_VERSION_INSTALLED_TTL", 15*time.Second),
		versionLatestTTL:     readVersionDurationEnv("XYLONA_VERSION_LATEST_TTL", 2*time.Minute),
		versionRefreshCalls:  make(map[string]*versionRefreshCall),
		backupCreateCalls:    make(map[string]*backupCreateCall),
		activeGameServerOps:  make(map[string]struct{}),
		hytaleClient:         readiness.NewHytaleHTTPClient(nil),
		hytaleLaunchLocks:    readiness.NewHytaleLaunchLocks(),
		restartState:         &restartStateMap{},
		exitHooks:            newExitHookRegistry(),
	}
	go inst.backgroundJobQueryAllGameServers()
	go inst.backgroundJobQueryPalworldMaps()
	go inst.backgroundJobCheckModUpdates()
	go inst.backgroundJobCheckVersionUpdates()

	// Start the eventbus-driven auto-restart subscriber. Both embedded and
	// remote nodes publish status-change events; this one subscriber drives
	// restart decisions uniformly.
	inst.startAutoRestartSubscriber(ctx)
	inst.startMetricsEventRecorder(ctx)

	return inst
}

// TryBeginGameServerLifecycleOperation reserves a server for an update or
// restart. The returned release function must be called when the operation
// finishes.
func (inst *Instance) TryBeginGameServerLifecycleOperation(gameServerID string) (func(), error) {
	gameServerID = strings.TrimSpace(gameServerID)
	if gameServerID == "" {
		return nil, errors.New("actions: game server ID is required for lifecycle operation")
	}

	inst.gameServerOperationMu.Lock()
	defer inst.gameServerOperationMu.Unlock()
	if inst.activeGameServerOps == nil {
		inst.activeGameServerOps = make(map[string]struct{})
	}
	_, active := inst.activeGameServerOps[gameServerID]
	if active {
		return nil, ErrGameServerOperationInProgress
	}
	inst.activeGameServerOps[gameServerID] = struct{}{}

	var releaseOnce sync.Once
	return func() {
		releaseOnce.Do(func() {
			inst.gameServerOperationMu.Lock()
			delete(inst.activeGameServerOps, gameServerID)
			inst.gameServerOperationMu.Unlock()
		})
	}, nil
}

// InstallGameServer creates the server record, schedules install, and starts post-install startup.
func (inst *Instance) InstallGameServer(game *models.Game, gameServer *models.GameServer, owner *models.User) (*models.GameServer, error) {
	normalInstallEnv, errNormalInstallEnv := mergeNormalLaunchEnvironment(game, gameServer.EnvVars)
	if errNormalInstallEnv != nil {
		return nil, fmt.Errorf("actions: prepare install environment: %w", errNormalInstallEnv)
	}
	installLaunchEnv, errInstallLaunchEnv := buildNormalLaunchEnvironment(normalInstallEnv)
	if errInstallLaunchEnv != nil {
		return nil, fmt.Errorf("actions: prepare install environment: %w", errInstallLaunchEnv)
	}
	starboundSteamUsername := ""
	if game.ID == gameintegrations.StarboundGameID {
		var errSteamUsername error
		starboundSteamUsername, errSteamUsername = gameintegrations.StarboundSteamUsername(installLaunchEnv)
		if errSteamUsername != nil {
			return nil, fmt.Errorf("actions: validate Starbound Steam account: %w", errSteamUsername)
		}
	}
	storedServerEnv := strings.TrimSpace(gameServer.EnvVars)
	if storedServerEnv == "" {
		storedServerEnv = "[]"
	}

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
		errRollback := tx.Rollback(inst.ctx)
		if errRollback != nil && !errors.Is(errRollback, sql.ErrTxDone) {
			log.Error().Err(errRollback).Msg("Failed to roll back game server install transaction")
		}
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
		ID:               omit.From(uuid.NewString()),
		UserID:           omit.From(owner.ID),
		Name:             omit.From(gameServer.Name),
		GameID:           omit.From(game.ID),
		StartArgsPatches: omit.From("[]"),
		Status:           omit.From(""),
		SetPlayers:       omit.From(gameServer.SetPlayers),
		MaxPlayers:       omit.From(gameServer.MaxPlayers),
		Map:              omit.From(gameServer.Map),
		IP:               omit.From(gameServer.IP),
		Port:             omit.From(gameServer.Port),
		QueryPort:        omit.From(gameServer.QueryPort),
		Directory:        omit.From(gameServer.Directory),
		MaxMemoryMB:      omit.From(gameServer.MaxMemoryMB),
		BackupsEnabled:   omit.From(gameServer.BackupsEnabled),
		EnvVars:          omit.From(storedServerEnv),
		BackupDirectory:  omit.From(gameServer.BackupDirectory),
		MaxBackups:       omit.From(gameServer.MaxBackups),
		NodeID:           omit.From(gameServer.NodeID),
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
	newGameServer.R.Game = game

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
	addNormalEnvironmentPlaceholders(normalInstallEnv, installVars)
	if game.ID == gameintegrations.StarboundGameID {
		installVars[gameintegrations.StarboundSteamUsernameEnv] = starboundSteamUsername
	}
	baseCommand, args, errCommandArgs := resolveCommandLineToProcessArgs(
		gameInstallCommand(game, nodeOS),
		installVars,
	)
	if errCommandArgs != nil {
		errInstall := fmt.Errorf("actions: parse install command: %w", errCommandArgs)
		errCleanup := inst.cleanupFailedInstall(newGameServer.ID, newGameServer.Directory, newGameServer.NodeID)
		if errCleanup != nil {
			return nil, errors.Join(errInstall, errCleanup)
		}
		return nil, errInstall
	}

	client, errClient := inst.resolveNodeClient(newGameServer.NodeID)
	if errClient != nil {
		errInstall := fmt.Errorf("actions: resolve node client for install: %w", errClient)
		errCleanup := inst.cleanupFailedInstall(newGameServer.ID, newGameServer.Directory, newGameServer.NodeID)
		if errCleanup != nil {
			return nil, errors.Join(errInstall, errCleanup)
		}
		return nil, errInstall
	}
	errLaunchEnvSupported := inst.ensureLaunchEnvSupported(client, len(installLaunchEnv) > 0)
	if errLaunchEnvSupported != nil {
		errCleanup := inst.cleanupFailedInstall(newGameServer.ID, newGameServer.Directory, newGameServer.NodeID)
		if errCleanup != nil {
			return nil, errors.Join(errLaunchEnvSupported, errCleanup)
		}
		return nil, errLaunchEnvSupported
	}

	installCfg := node.ProcessConfig{
		ID:               newGameServer.ID,
		ExecutionID:      uuid.NewString(),
		Name:             newGameServer.Name,
		BaseCommand:      baseCommand,
		Args:             args,
		WorkingDirectory: newGameServer.Directory,
		User:             newGameServer.UserID,
		NodeID:           newGameServer.NodeID,
		ServiceID:        newGameServer.GameID,
		LaunchEnv:        installLaunchEnv,
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
	inst.exitHooks.set(newGameServer.ID, installCfg.ExecutionID, func(event eventbus.StatusChangedEvent) {
		if event.ExitCode != 0 {
			log.Warn().Int("exit_code", event.ExitCode).Str("game_server_id", installedGS.ID).
				Msg("Install command exited with non-zero; skipping post-install")
			return
		}
		errPostInstall := inst.postInstallStep(installedGS)
		if errPostInstall != nil {
			log.Error().Err(errPostInstall).Msg("Failed to perform post install step")
			return
		}
		_, errStart := inst.StartGameServer(installedGS)
		if errStart != nil {
			log.Error().Err(errStart).Str("game_server_id", installedGS.ID).
				Msg("Failed to start server after install")
		}
	})

	errStart := client.StartProcess(inst.ctx, installCfg, xylona.Status_INSTALLING)
	if errStart != nil {
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
		errDeleteGameServerFiles := inst.deleteGameServerDirectory(inst.actionContext(), resolvedNodeID, gameServerDir)
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
	return inst.resolveStructuredStartCommandWithVars(gameServer, nil)
}

func (inst *Instance) resolveStructuredStartCommandWithVars(
	gameServer *models.GameServer,
	extraVars map[string]string,
) (string, []string, error) {
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

	startVars := placeholder.BuildVarsFromGameServer(gameServer)
	maps.Copy(startVars, extraVars)
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

	args, errResolve := startargs.ResolveServer(startargs.ServerConfig{
		TemplateJSON:  templateJSON,
		PatchesJSON:   gameServer.StartArgsPatches,
		BlocklistJSON: gameServer.R.Game.StartArgBlocklist,
		Variables:     startVars,
	})
	if errResolve != nil {
		return "", nil, fmt.Errorf("resolve start args: %w", errResolve)
	}

	return baseCommand, args, nil
}

func (inst *Instance) postInstallStep(gameServer *models.GameServer) error {
	switch gameServer.GameID {
	case "minecraft":
		// Minecraft EULA acceptance is a readiness action from the server view.
		return nil
	case "7_days_to_die":
		return inst.post7DaysToDieInstall(gameServer)
	case "sunkenland":
		return inst.postSunkenlandInstall(gameServer)
	default:
		log.Debug().Str("Game ID", gameServer.GameID).Msg("No post install step")
		return nil
	}
}

// StartGameServer resolves and starts the configured runtime command for a
// server. Routes through NodeClient so both embedded and remote nodes launch
// processes via the same surface; exit handling is driven by the status-
// change eventbus subscriber.
func (inst *Instance) StartGameServer(gameServer *models.GameServer) (*StartGameServerResult, error) {
	if gameServer == nil {
		return nil, startConfigurationError("game server is missing", errors.New("game server is nil"))
	}

	reloadedGameServer, errReload := inst.reloadGameServerForStart(gameServer)
	if errReload != nil {
		inst.reportStartFailure(gameServer, "Failed to load server start configuration: "+errReload.Error())
		return nil, startInternalError("failed to load server start configuration", errReload)
	}
	gameServer = reloadedGameServer

	normalLaunchEnv, secretLaunchEnvStates, errLaunchEnvMetadata := inst.loadStartLaunchEnvMetadata(gameServer)
	if errLaunchEnvMetadata != nil {
		inst.reportStartFailure(gameServer, "Launch environment is invalid: "+errLaunchEnvMetadata.Error())
		return nil, startConfigurationError("launch environment is invalid", errLaunchEnvMetadata)
	}

	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		log.Error().Err(errClient).Str("game_server_id", gameServer.ID).
			Str("node_id", gameServer.NodeID).Msg("Failed to resolve node client for start")
		inst.reportStartFailure(gameServer, "Failed to reach target node: "+errClient.Error())
		return nil, startUnavailableError("target node is unavailable", errClient)
	}
	adminInput := gameServerAdminInput{}
	adminInterfacePassword := ""
	configureAdminInput, disableManagedConsole, errConfigureAdminInput := inst.shouldConfigureAdminInput(gameServer)
	if errConfigureAdminInput != nil {
		inst.reportStartFailure(gameServer, "Admin console configuration failed: "+errConfigureAdminInput.Error())
		return nil, startConfigurationError("admin console configuration failed", errConfigureAdminInput)
	}
	if configureAdminInput {
		var errAdminInterfacePassword error
		adminInterfacePassword, errAdminInterfacePassword = inst.loadOrCreateAdminInterfacePassword(gameServer)
		if errAdminInterfacePassword != nil {
			inst.reportStartFailure(gameServer, "Admin interface password setup failed: "+errAdminInterfacePassword.Error())
			return nil, startConfigurationError("admin interface password setup failed", errAdminInterfacePassword)
		}
		previousAdminPasswords, errPasswordHistory := inst.loadAdminInterfacePasswordHistory(gameServer)
		if errPasswordHistory != nil {
			inst.reportStartFailure(gameServer, "Admin interface password history failed: "+errPasswordHistory.Error())
			return nil, startConfigurationError("admin interface password history failed", errPasswordHistory)
		}
		var errAdminInput error
		adminInput, errAdminInput = newGameServerAdminInput(
			gameServer,
			adminInterfacePassword,
			previousAdminPasswords,
		)
		if errAdminInput != nil {
			inst.reportStartFailure(gameServer, "Admin console configuration failed: "+errAdminInput.Error())
			return nil, startConfigurationError("admin console configuration failed", errAdminInput)
		}
	}
	errAdminInputSupported := inst.ensureAdminInputSupported(client, adminInput)
	if errAdminInputSupported != nil {
		inst.reportStartFailure(gameServer, errAdminInputSupported.Error())
		return nil, errAdminInputSupported
	}

	errPalworldQuery := inst.ensurePalworldQueryConfig(gameServer, client, adminInterfacePassword)
	if errPalworldQuery != nil {
		inst.reportStartFailure(gameServer, "Palworld query configuration failed: "+errPalworldQuery.Error())
		return nil, startConfigurationError("Palworld query configuration failed", errPalworldQuery)
	}
	// Generate and enforce managed config before readiness checks so games
	// with required first-run values can present an editable file immediately.
	switch {
	case adminInput.managedConfigRequired:
		errConfigPreStart := inst.runConfigPreStartWithConsolePassword(gameServer, adminInput.localConsolePassword, disableManagedConsole, true)
		if errConfigPreStart != nil {
			inst.reportStartFailure(gameServer, "Admin console configuration failed: "+errConfigPreStart.Error())
			return nil, startConfigurationError("admin console configuration failed", errConfigPreStart)
		}
	default:
		errConfigPreStart := inst.runConfigPreStartWithConsolePassword(gameServer, adminInput.localConsolePassword, disableManagedConsole, false)
		if errConfigPreStart != nil {
			log.Warn().Err(errConfigPreStart).Str("game_server_id", gameServer.ID).
				Msg("Pre-start: config processing failed; continuing startup")
		}
	}

	errReadiness := readiness.CheckStart(inst.ctx, inst.db, gameServer, client)
	if errReadiness != nil {
		inst.reportStartFailure(gameServer, errReadiness.Error())
		return nil, startConfigurationError("server setup is incomplete", errReadiness)
	}
	secretStartVars, errSecretStartVars := inst.secretStartPlaceholderVars(gameServer)
	if errSecretStartVars != nil {
		inst.reportStartFailure(gameServer, "Failed to load server launch secret: "+errSecretStartVars.Error())
		return nil, startConfigurationError("server launch secret is unavailable", errSecretStartVars)
	}
	adminInput.mergePlaceholderVars(secretStartVars)

	errGameLaunchSecrets := inst.writeGameLaunchSecrets(gameServer, client, secretStartVars)
	if errGameLaunchSecrets != nil {
		inst.reportStartFailure(gameServer, "Failed to prepare server launch secret: "+errGameLaunchSecrets.Error())
		return nil, startConfigurationError("server launch secret could not be prepared", errGameLaunchSecrets)
	}
	// Run mod auto-updates before launching the process.
	inst.runModAutoUpdates(gameServer)

	baseCommand, args, errResolve := inst.resolveStructuredStartCommandWithVars(gameServer, secretStartVars)
	if errResolve != nil {
		inst.reportStartFailure(gameServer, errResolve.Error())
		return nil, startConfigurationError("start configuration is incomplete", errResolve)
	}

	cfg := node.ProcessConfig{
		ID:               gameServer.ID,
		ExecutionID:      uuid.NewString(),
		Name:             gameServer.Name,
		BaseCommand:      baseCommand,
		Args:             args,
		WorkingDirectory: gameServer.Directory,
		User:             gameServer.UserID,
		NodeID:           gameServer.NodeID,
		ServiceID:        gameServer.GameID,
	}
	adminInput.apply(&cfg)

	launchEnvRequired := startLaunchEnvRequired(normalLaunchEnv, secretLaunchEnvStates) || readiness.RequiresLaunchEnv(gameServer)
	errLaunchEnvSupported := inst.ensureLaunchEnvSupported(client, launchEnvRequired)
	if errLaunchEnvSupported != nil {
		inst.reportStartFailure(gameServer, errLaunchEnvSupported.Error())
		return nil, errLaunchEnvSupported
	}

	launchEnv, errLaunchEnv := inst.decryptStartLaunchEnv(gameServer, normalLaunchEnv)
	if errLaunchEnv != nil {
		inst.reportStartFailure(gameServer, "Launch environment could not be loaded: "+errLaunchEnv.Error())
		return nil, startConfigurationError("launch environment could not be loaded", errLaunchEnv)
	}
	launchEnv, errLaunchEnv = inst.prepareLaunchSecrets(gameServer, client, launchEnv)
	if errLaunchEnv != nil {
		inst.reportStartFailure(gameServer, "Launch secrets could not be prepared: "+errLaunchEnv.Error())
		return nil, startConfigurationError("launch secrets could not be prepared", errLaunchEnv)
	}
	cfg.LaunchEnv = launchEnv

	errStart := client.StartProcess(inst.ctx, cfg, xylona.Status_ONLINE)
	if errStart != nil {
		log.Error().Err(errStart).Str("game_server_id", gameServer.ID).
			Msg("Failed to start game server")
		inst.reportStartFailure(gameServer, "Failed to start server: "+errStart.Error())
		return nil, startInternalError("failed to start server process", errStart)
	}
	inst.intentionalStops.clear(gameServer.ID)
	inst.restartState.recordStarted(gameServer.ID)
	return &StartGameServerResult{Started: true}, nil
}

func (inst *Instance) ensureLaunchEnvSupported(client nodeclient.NodeClient, required bool) error {
	if !required {
		return nil
	}
	caps, errCaps := client.GetRuntimeCapabilities(inst.ctx)
	if errCaps != nil {
		return startUnavailableError("target node runtime capabilities unavailable", errCaps)
	}
	if !caps.LaunchEnv {
		return startConfigurationError("target node does not support launch environment variables", nil)
	}
	return nil
}

func (inst *Instance) ensureTelnetInputSupported(client nodeclient.NodeClient) error {
	caps, errCaps := client.GetRuntimeCapabilities(inst.ctx)
	if errCaps != nil {
		return startUnavailableError("target node runtime capabilities unavailable", errCaps)
	}
	if !caps.TelnetInput {
		return startConfigurationError("target node does not support the required local management console; upgrade the node before starting this server", nil)
	}
	return nil
}

// resolveNodeClient looks up the NodeClient for a node ID. When the
// registry is not configured (tests, migrations), falls back to the embedded
// node client supplied at construction. With a configured registry, non-self
// node misses fail closed instead of running remote work against the
// controller filesystem.
// Returns a plain Go error (not a connect error) since actions callers don't
// want connect types.
func (inst *Instance) resolveNodeClient(nodeID string) (nodeclient.NodeClient, error) {
	if inst.nodeRegistry != nil {
		client, errGet := inst.nodeRegistry.Get(nodeID)
		if errGet == nil {
			return client, nil
		}

		targetID := strings.TrimSpace(nodeID)
		selfID := strings.TrimSpace(inst.nodeRegistry.SelfID())
		if targetID != "" && targetID != selfID {
			return nil, fmt.Errorf("actions: resolve node client for %q: %w", nodeID, errGet)
		}
	}
	if inst.embeddedNodeClient == nil {
		return nil, errors.New("actions: neither node registry nor embedded node client is configured")
	}
	return inst.embeddedNodeClient, nil
}

func (inst *Instance) runConfigPreStart(gameServer *models.GameServer) {
	errRun := inst.runConfigPreStartMode(gameServer, false)
	if errRun != nil {
		log.Warn().Err(errRun).Str("game_server_id", gameServer.ID).
			Msg("Pre-start: config processing failed; continuing startup")
	}
}

func (inst *Instance) runConfigPreStartStrict(gameServer *models.GameServer) error {
	return inst.runConfigPreStartMode(gameServer, true)
}

func (inst *Instance) runConfigPreStartMode(gameServer *models.GameServer, strict bool) error {
	return inst.runConfigPreStartWithConsolePassword(gameServer, "", false, strict)
}

func (inst *Instance) runConfigPreStartWithConsolePassword(
	gameServer *models.GameServer,
	localConsolePassword string,
	disableManagedConsole bool,
	strict bool,
) error {
	schemasJSON, errGet := inst.db.GetGameConfigSchemas(gameServer.GameID)
	if errGet != nil {
		return fmt.Errorf("actions: get pre-start config schemas: %w", errGet)
	}
	if schemasJSON == "" {
		if strict {
			return errors.New("actions: required pre-start config schema is unavailable")
		}
		return nil
	}
	entries, errParseSchemas := cfgschema.ParseConfigSchemas(schemasJSON)
	if errParseSchemas != nil {
		return fmt.Errorf("actions: parse pre-start config schemas: %w", errParseSchemas)
	}
	localConsoleEnabled := strings.TrimSpace(localConsolePassword) != ""
	if !localConsoleEnabled {
		managedSourcesToRemove := []string{
			"xylona.local_console_port",
			"xylona.local_console_password",
		}
		if !disableManagedConsole {
			managedSourcesToRemove = append(managedSourcesToRemove, "xylona.local_console_enabled")
		}
		var errWithoutConsole error
		schemasJSON, errWithoutConsole = cfgschema.WithoutManagedSources(
			schemasJSON,
			managedSourcesToRemove...,
		)
		if errWithoutConsole != nil {
			return fmt.Errorf("actions: preserve disabled local console config: %w", errWithoutConsole)
		}
		entries, errParseSchemas = cfgschema.ParseConfigSchemas(schemasJSON)
		if errParseSchemas != nil {
			return fmt.Errorf("actions: parse filtered pre-start config schemas: %w", errParseSchemas)
		}
	}
	nodeOS := OperatingSystem
	if cfgschema.HasPlatformPaths(entries) {
		var errNodeOS error
		nodeOS, errNodeOS = inst.resolveNodeOSRequired(inst.ctx, gameServer.NodeID)
		if errNodeOS != nil {
			return fmt.Errorf("actions: resolve pre-start config schema platform: %w", errNodeOS)
		}
	}
	resolvedSchemasJSON, errResolveSchemas := cfgschema.ResolvePlatformConfigSchemas(schemasJSON, string(nodeOS))
	if errResolveSchemas != nil {
		return fmt.Errorf("actions: resolve pre-start config schema paths: %w", errResolveSchemas)
	}
	schemasJSON = resolvedSchemasJSON

	resolver := cfgschema.GameServerSettingsResolver(cfgschema.GameServerSettings{
		Name:                   gameServer.Name,
		Directory:              gameServer.Directory,
		IP:                     gameServer.IP,
		Port:                   gameServer.Port,
		QueryPort:              gameServer.QueryPort,
		MaxPlayers:             gameServer.MaxPlayers,
		LocalConsoleConfigured: true,
		LocalConsoleEnabled:    localConsoleEnabled,
		LocalConsolePort:       gameServer.QueryPort + 1,
		LocalConsolePassword:   localConsolePassword,
	})
	if inst.isRemoteGameServer(gameServer) {
		client, errClient := inst.resolveNodeClient(gameServer.NodeID)
		if errClient != nil {
			return fmt.Errorf("actions: resolve pre-start node client: %w", errClient)
		}
		store := remotePreStartFileStore{
			ctx:       inst.ctx,
			client:    client,
			directory: gameServer.Directory,
		}
		if strict {
			errRun := cfgschema.RunPreStartWithStoreStrict(schemasJSON, resolver, store)
			if errRun != nil {
				return fmt.Errorf("actions: run strict remote pre-start config: %w", errRun)
			}
			return nil
		}
		cfgschema.RunPreStartWithStore(schemasJSON, resolver, store)
		return nil
	}
	if strict {
		errRun := cfgschema.RunPreStartStrict(gameServer.Directory, schemasJSON, resolver)
		if errRun != nil {
			return fmt.Errorf("actions: run strict local pre-start config: %w", errRun)
		}
		return nil
	}
	cfgschema.RunPreStart(gameServer.Directory, schemasJSON, resolver)
	return nil
}

type remotePreStartFileStore struct {
	ctx       context.Context
	client    nodeclient.NodeClient
	directory string
}

func (store remotePreStartFileStore) ReadFile(relativePath string) ([]byte, error) {
	data, errRead := store.client.ReadFile(store.context(), store.directory, relativePath)
	if errRead != nil {
		return nil, fmt.Errorf("actions: read remote pre-start config: %w", errRead)
	}
	return data, nil
}

func (store remotePreStartFileStore) EnsureDir(relativePath string) error {
	if relativePath == "" || relativePath == "." {
		return nil
	}

	errCreate := store.client.CreateFileOrDirectory(store.context(), store.directory, relativePath, "", true, node.ProtectionPolicy{})
	if errCreate != nil {
		return fmt.Errorf("actions: create remote pre-start config directory: %w", errCreate)
	}
	return nil
}

func (store remotePreStartFileStore) WriteFile(relativePath string, data []byte) error {
	errWrite := store.client.WriteFile(store.context(), store.directory, relativePath, data, node.ProtectionPolicy{})
	if errWrite != nil {
		return fmt.Errorf("actions: write remote pre-start config: %w", errWrite)
	}
	return nil
}

func (store remotePreStartFileStore) context() context.Context {
	if store.ctx == nil {
		return context.Background()
	}
	return store.ctx
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

	var errAutoUpdate error
	if inst.isRemoteGameServer(gameServer) {
		client, errClient := inst.resolveNodeClient(gameServer.NodeID)
		if errClient != nil {
			log.Warn().Err(errClient).Str("game_server_id", gameServer.ID).
				Msg("Pre-start mod auto-update: remote node unavailable, skipping")
			return
		}
		errAutoUpdate = inst.modManager.RunAutoUpdatesRemote(inst.ctx, client, gameServer.ID, "", gameServer.Directory, statusFn)
	} else {
		errAutoUpdate = inst.modManager.RunAutoUpdates(inst.ctx, gameServer.ID, "", gameServer.Directory, statusFn)
	}
	if errAutoUpdate != nil {
		log.Warn().Err(errAutoUpdate).Str("game_server_id", gameServer.ID).
			Msg("Pre-start mod auto-update failed; continuing server startup")
	}
}

// StopGameServer stops a running game server using its configured stop
// command. Routes through the NodeClient for the server's owning node so
// embedded and remote servers stop on the same code path. A missing process
// is treated as already stopped; node resolution and transport failures are
// returned so callers never continue under a false assumption of shutdown.
func (inst *Instance) StopGameServer(ctx context.Context, gameServer *models.GameServer) error {
	if gameServer == nil {
		return errors.New("actions: stop game server: game server is nil")
	}
	if ctx == nil {
		ctx = inst.actionContext()
	}

	nodeOS := inst.resolveNodeOS(ctx, gameServer.NodeID)
	inst.intentionalStops.mark(gameServer.ID)
	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		inst.intentionalStops.clear(gameServer.ID)
		return fmt.Errorf("actions: stop game server: resolve node client: %w", errClient)
	}

	errStop := client.StopProcess(ctx, gameServer.ID, gameStopCommand(gameServer.R.Game, nodeOS))
	if errStop != nil {
		if errors.Is(errStop, node.ErrProcessNotFound) || errors.Is(errStop, os.ErrNotExist) {
			inst.intentionalStops.clear(gameServer.ID)
			return nil
		}
		inst.intentionalStops.clear(gameServer.ID)
		return fmt.Errorf("actions: stop game server process on node %q: %w", gameServer.NodeID, errStop)
	}
	return nil
}

// UpdateGameServer starts the configured update flow for a game server.
// Routes through NodeClient for both embedded and remote nodes.
func (inst *Instance) UpdateGameServer(gameServer *models.GameServer) error {
	_, errUpdate := inst.startGameServerUpdate(gameServer)
	return errUpdate
}

// startGameServerUpdate starts an update with a fresh execution ID and returns
// that ID so callers can correlate the exact terminal lifecycle transition.
func (inst *Instance) startGameServerUpdate(gameServer *models.GameServer) (string, error) {
	executionID := uuid.NewString()
	errUpdate := inst.updateGameServerWithExecutionID(gameServer, executionID)
	return executionID, errUpdate
}

func (inst *Instance) updateGameServerWithExecutionID(gameServer *models.GameServer, executionID string) error {
	// Stop if currently running. Use snapshot status rather than direct
	// supervisor lookup so remote-node servers are handled uniformly.
	currentStatus := inst.currentProcessStatus(gameServer)
	if currentStatus == xylona.Status_UNKNOWN {
		return errors.New("actions: cannot update game server while runtime status is unavailable")
	}
	if currentStatus != xylona.Status_OFFLINE {
		log.Info().Str("Game Server ID", gameServer.ID).Msg("Stopping game server before update")
		errStop := inst.StopGameServer(inst.actionContext(), gameServer)
		if errStop != nil {
			return fmt.Errorf("actions: stop game server before update: %w", errStop)
		}
	}

	handled, errMinecraft := inst.tryUpdateMinecraftServerSoftware(gameServer)
	if handled || errMinecraft != nil {
		return errMinecraft
	}

	nodeOS := inst.resolveNodeOS(inst.ctx, gameServer.NodeID)
	updateCmd := gameUpdateCommand(gameServer.R.Game, nodeOS)
	internalUpdate := gameUpdateCommandType(gameServer.R.Game, nodeOS) == CommandTypeInternal
	if !internalUpdate && updateCmd == "" {
		_, internalUpdate = gameintegrations.GetGame(gameServer.GameID)
	}
	if updateCmd == "" && !internalUpdate {
		log.Warn().Str("Game Server ID", gameServer.ID).Msg("No update command configured for this game")
		return ErrGameUpdateNotConfigured
	}
	if internalUpdate {
		_, exists := gameintegrations.GetGame(gameServer.GameID)
		if !exists {
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
		ExecutionID:      executionID,
		Name:             gameServer.Name,
		WorkingDirectory: gameServer.Directory,
		User:             gameServer.UserID,
		NodeID:           gameServer.NodeID,
		ServiceID:        gameServer.GameID,
	}
	if internalUpdate && readiness.RequiresLaunchEnv(gameServer) {
		errReadiness := readiness.CheckStart(inst.ctx, inst.db, gameServer, client)
		if errReadiness != nil {
			return startConfigurationError("server setup is incomplete", errReadiness)
		}
	}
	normalLaunchEnv, secretLaunchEnvStates, errLaunchEnvMetadata := inst.loadStartLaunchEnvMetadata(gameServer)
	if errLaunchEnvMetadata != nil {
		return startConfigurationError("update launch environment is invalid", errLaunchEnvMetadata)
	}
	launchEnvRequired := startLaunchEnvRequired(normalLaunchEnv, secretLaunchEnvStates) || readiness.RequiresLaunchEnv(gameServer)
	errLaunchEnvSupported := inst.ensureLaunchEnvSupported(client, launchEnvRequired)
	if errLaunchEnvSupported != nil {
		return errLaunchEnvSupported
	}
	if launchEnvRequired {
		launchEnv, errLaunchEnv := inst.decryptStartLaunchEnv(gameServer, normalLaunchEnv)
		if errLaunchEnv != nil {
			return startConfigurationError("update launch environment could not be loaded", errLaunchEnv)
		}
		launchEnv, errLaunchEnv = inst.prepareLaunchSecrets(gameServer, client, launchEnv)
		if errLaunchEnv != nil {
			return startConfigurationError("update launch secrets could not be prepared", errLaunchEnv)
		}
		updateCfg.LaunchEnv = launchEnv
	}
	if !internalUpdate {
		updateVars := placeholder.BuildVarsFromGameServer(gameServer)
		addNormalEnvironmentPlaceholders(normalLaunchEnv, updateVars)
		if gameServer.GameID == gameintegrations.StarboundGameID {
			steamUsername, errSteamUsername := gameintegrations.StarboundSteamUsername(updateCfg.LaunchEnv)
			if errSteamUsername != nil {
				return fmt.Errorf("actions: validate Starbound Steam account: %w", errSteamUsername)
			}
			updateVars[gameintegrations.StarboundSteamUsernameEnv] = steamUsername
		}
		updateCommand := appendSteamBranchToUpdateCommand(updateCmd, gameServer.Branch)
		baseCommand, args, errCommandArgs := resolveCommandLineToProcessArgs(updateCommand, updateVars)
		if errCommandArgs != nil {
			return fmt.Errorf("actions: parse update command: %w", errCommandArgs)
		}
		updateCfg.BaseCommand = baseCommand
		updateCfg.Args = args
	}
	if internalUpdate {
		updateCfg.InternalCommand = true
		updateCfg.InternalGameServerID = gameServer.ID
		updateCfg.InternalGameID = gameServer.GameID
		updateCfg.InternalGameServer = gameServer
	}

	errStart := client.StartProcess(inst.ctx, updateCfg, xylona.Status_UPDATING)
	if errStart != nil {
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

// GetCachedGameServerStatus returns the latest status captured by the
// background server poll without contacting a node.
func (inst *Instance) GetCachedGameServerStatus(gameServerID string) xylona.Status {
	inst.gameServerStatusMutex.RLock()
	status, ok := inst.gameServerStatuses[gameServerID]
	inst.gameServerStatusMutex.RUnlock()
	if !ok {
		return xylona.Status_UNKNOWN
	}
	return status
}

func (inst *Instance) storeGameServerStatus(gameServerID string, status xylona.Status) {
	inst.gameServerStatusMutex.Lock()
	if inst.gameServerStatuses == nil {
		inst.gameServerStatuses = make(map[string]xylona.Status)
	}
	inst.gameServerStatuses[gameServerID] = status
	inst.gameServerStatusMutex.Unlock()
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
	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		return xylona.Status_UNKNOWN
	}
	snap, found, errSnap := client.GetProcessSnapshot(inst.ctx, gameServer.ID)
	if errSnap != nil {
		return xylona.Status_UNKNOWN
	}
	if !found || snap == nil {
		return xylona.Status_OFFLINE
	}
	status, ok := xylona.Status_value[snap.Status]
	if !ok {
		return xylona.Status_UNKNOWN
	}
	return xylona.Status(status)
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
func (inst *Instance) ReadGameServerBuffer(ctx context.Context, gameServer *models.GameServer) (string, error) {
	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		return "", fmt.Errorf("actions: read game server buffer: %w", errClient)
	}
	chunk, errRead := client.ReadConsoleBuffer(ctx, gameServer.ID)
	if errRead != nil {
		return "", fmt.Errorf("actions: read game server buffer: %w", errRead)
	}
	return chunk.Data, nil
}

// RemoveGameServer deletes a server's files before deleting its database row.
// File cleanup is mandatory so a node outage cannot orphan unmanaged server
// files while the controller reports successful removal.
func (inst *Instance) RemoveGameServer(ctx context.Context, gameServer *models.GameServer) error {
	if gameServer == nil {
		return errors.New("actions: remove game server: game server is nil")
	}
	if ctx == nil {
		ctx = inst.actionContext()
	}

	errPurge := inst.PurgeAllGameServerFiles(ctx, gameServer)
	if errPurge != nil {
		return fmt.Errorf("actions: remove game server files: %w", errPurge)
	}
	err := inst.db.DeleteGameServer(gameServer.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to remove game server from database")
		return fmt.Errorf("actions: delete game server: %w", err)
	}
	eb := eventbus.Get()
	eb.Publish("game_server_removed", gameServer)
	return nil
}
