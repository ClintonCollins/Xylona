// Package actions contains the control-plane business logic for game server
// lifecycle, file operations, synchronization, and background jobs.
package actions

import (
	"context"
	"database/sql"
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

	"github.com/ClintonCollins/Xylona/internal/controller/readiness"
	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
	"github.com/ClintonCollins/Xylona/internal/modmanager"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/placeholder"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/pkg/cfgschema"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/startargs"
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
	ctx                  context.Context
	embeddedNodeClient   nodeclient.NodeClient
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
	hytaleClient         readiness.HytaleClient
	hytaleLaunchLocks    *readiness.HytaleLaunchLocks
	localNodeID          string
	restartState         *restartStateMap
	exitHooks            *exitHookRegistry
	intentionalStops     intentionalStopRegistry
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
		db:                   database,
		modManager:           modMgr,
		versionState:         versionState,
		resolverConfig:       resolverConfig,
		versionInstalledTTL:  readVersionDurationEnv("XYLONA_VERSION_INSTALLED_TTL", 15*time.Second),
		versionLatestTTL:     readVersionDurationEnv("XYLONA_VERSION_LATEST_TTL", 2*time.Minute),
		versionRefreshCalls:  make(map[string]*versionRefreshCall),
		backupCreateCalls:    make(map[string]*backupCreateCall),
		hytaleClient:         readiness.NewHytaleHTTPClient(nil),
		hytaleLaunchLocks:    readiness.NewHytaleLaunchLocks(),
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

// InstallGameServerOptions controls install-time setup that must be persisted
// before the install process starts.
type InstallGameServerOptions struct {
	AcceptMinecraftEULA bool
	AcceptedByUserID    string
}

// InstallGameServer creates the server record, schedules install, and starts post-install startup.
func (inst *Instance) InstallGameServer(game *models.Game, gameServer *models.GameServer, owner *models.User) (*models.GameServer, error) {
	return inst.InstallGameServerWithOptions(game, gameServer, owner, InstallGameServerOptions{})
}

// InstallGameServerWithOptions creates the server record, schedules install,
// and starts post-install startup with install-time setup state.
func (inst *Instance) InstallGameServerWithOptions(game *models.Game, gameServer *models.GameServer, owner *models.User, options InstallGameServerOptions) (*models.GameServer, error) {
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
		EnvVars:          omit.From("[]"),
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

	if game.ID == "minecraft" && options.AcceptMinecraftEULA {
		errEULA := readiness.PersistMinecraftEULAAccepted(inst.db, newGameServer.ID, options.AcceptedByUserID)
		if errEULA != nil {
			errInstall := fmt.Errorf("actions: persist minecraft EULA acceptance: %w", errEULA)
			errCleanup := inst.cleanupFailedInstall(newGameServer.ID, newGameServer.Directory, newGameServer.NodeID)
			if errCleanup != nil {
				return nil, errors.Join(errInstall, errCleanup)
			}
			return nil, errInstall
		}
	}

	nodeOS := inst.resolveNodeOS(inst.ctx, newGameServer.NodeID)
	installVars := placeholder.BuildVarsFromGameServer(newGameServer)
	installCommand := placeholder.Resolve(gameInstallCommand(game, nodeOS), installVars)
	baseCommand, args, errCommandArgs := commandLineToProcessArgs(installCommand)
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

func (inst *Instance) postInstallStep(gameServer *models.GameServer) error {
	switch gameServer.GameID {
	case "minecraft":
		return inst.postMinecraftInstall(gameServer)
	case "7_days_to_die":
		return inst.post7DaysToDieInstall(gameServer)
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

	errReadiness := readiness.CheckStart(inst.ctx, inst.db, gameServer, client)
	if errReadiness != nil {
		inst.reportStartFailure(gameServer, errReadiness.Error())
		return nil, startConfigurationError("server setup is incomplete", errReadiness)
	}

	// Run pre-start config enforcement before launching the process.
	inst.runConfigPreStart(gameServer)
	// Run mod auto-updates before launching the process.
	inst.runModAutoUpdates(gameServer)

	baseCommand, args, errResolve := inst.resolveStructuredStartCommand(gameServer)
	if errResolve != nil {
		inst.reportStartFailure(gameServer, errResolve.Error())
		return nil, startConfigurationError("start configuration is incomplete", errResolve)
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
	if inst.isRemoteGameServer(gameServer) {
		client, errClient := inst.resolveNodeClient(gameServer.NodeID)
		if errClient != nil {
			log.Warn().Err(errClient).Str("game_server_id", gameServer.ID).
				Msg("Pre-start: failed to resolve node client, skipping")
			return
		}
		store := remotePreStartFileStore{
			ctx:       inst.ctx,
			client:    client,
			directory: gameServer.Directory,
		}
		cfgschema.RunPreStartWithStore(schemasJSON, resolver, store)
		return
	}
	cfgschema.RunPreStart(gameServer.Directory, schemasJSON, resolver)
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
// embedded and remote servers stop on the same code path.
func (inst *Instance) StopGameServer(gameServer *models.GameServer) {
	nodeOS := inst.resolveNodeOS(inst.ctx, gameServer.NodeID)
	inst.intentionalStops.mark(gameServer.ID)
	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		inst.intentionalStops.clear(gameServer.ID)
		log.Warn().Err(errClient).Str("node_id", gameServer.NodeID).
			Msg("StopGameServer: node client unavailable")
		return
	}

	errStop := client.StopProcess(inst.ctx, gameServer.ID, gameStopCommand(gameServer.R.Game, nodeOS))
	if errStop != nil {
		if errors.Is(errStop, node.ErrProcessNotFound) || errors.Is(errStop, os.ErrNotExist) {
			return
		}
		inst.intentionalStops.clear(gameServer.ID)
		log.Error().Err(errStop).Str("node_id", gameServer.NodeID).
			Msg("Failed to stop game server command")
		return
	}
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
		baseCommand, args, errCommandArgs := commandLineToProcessArgs(updateCommand)
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
	if errClient == nil {
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
	status, ok := xylona.Status_value[gameServer.Status]
	if ok {
		return xylona.Status(status)
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
	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		return ""
	}
	chunk, errRead := client.ReadConsoleBuffer(inst.ctx, gameServer.ID)
	if errRead != nil {
		return ""
	}
	return chunk.Data
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
