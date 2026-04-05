// Package actions contains the control-plane business logic for game server
// lifecycle, file operations, synchronization, and background jobs.
package actions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"
	"google.golang.org/protobuf/types/known/timestamppb"

	internal "github.com/ClintonCollins/Xylona/api/xylona-internal"
	"github.com/ClintonCollins/Xylona/cfgschema"
	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/pkg/modmanager"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/placeholder"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
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
	minecraftUpdateProviderLookup     = func(kind updateproviders.ProviderKind) (modproviders.ModProvider, bool) {
		switch kind {
		case updateproviders.ProviderKindPaperMC:
			return modproviders.GetProvider("papermc")
		case updateproviders.ProviderKindMojang:
			return modproviders.GetProvider("mojang")
		default:
			return nil, false
		}
	}
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

type versionRefreshCall struct {
	done chan struct{}
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
	federationMTLS       *helpers.FederationMTLS
	syncEngine           SyncEngine
	modManager           *modmanager.ModManager
	versionState         *versiontracker.VersionStateMap
	resolverConfig       versiontracker.ResolverConfig
	dummyTracker         *versiontracker.DummyTracker
	versionBroadcaster   VersionBroadcaster
	versionInstalledTTL  time.Duration
	versionLatestTTL     time.Duration
	versionRefreshMu     sync.Mutex
	versionRefreshCalls  map[string]*versionRefreshCall
	localNodeID          string
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
func NewInstance(ctx context.Context, database *db.Connection, supervisorInstance *supervisor.Instance, federationMTLS *helpers.FederationMTLS, modMgr *modmanager.ModManager, versionState *versiontracker.VersionStateMap, resolverConfig versiontracker.ResolverConfig) *Instance {
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
	}
	go inst.backgroundJobQueryAllGameServers()
	go inst.backgroundJobCheckModUpdates()
	go inst.backgroundJobCheckVersionUpdates()
	return inst
}

// GetServerQueries returns the latest query snapshot for all tracked servers.
func (inst *Instance) GetServerQueries() *xylona.AllServersQueryInfo {
	inst.serverQueriesMutex.RLock()
	defer inst.serverQueriesMutex.RUnlock()
	allServerQueryInfo := &xylona.AllServersQueryInfo{Servers: make(map[string]*xylona.ServerQuery)}
	for _, serverQuery := range inst.serverQueriesInfoMap {
		allServerQueryInfo.Servers[serverQuery.GetServerId()] = serverQuery
	}
	return allServerQueryInfo
}

// GetPlayerCount returns the current player count for a game server from the
// most recent query result. Returns 0 if no query data is available.
func (inst *Instance) GetPlayerCount(gameServerID string) int {
	inst.serverQueriesMutex.RLock()
	defer inst.serverQueriesMutex.RUnlock()
	sq, ok := inst.serverQueriesInfoMap[gameServerID]
	if !ok {
		return 0
	}
	if sq.GetMinecraft() != nil {
		return int(sq.GetMinecraft().GetNumberOfPlayers())
	}
	if sq.GetSource() != nil {
		return int(sq.GetSource().GetPlayers())
	}
	return 0
}

// ListGameServerFiles lists files and directories for a relative server path.
func (inst *Instance) ListGameServerFiles(gameServer *models.GameServer, relativePath string) ([]*xylona.File, error) {
	// Check if path is empty or if it is a local path. If it is not a local path, return an error.
	if relativePath != "" && !filepath.IsLocal(relativePath) {
		log.Error().Err(errors.New("invalid path")).Msg("Path is not local")
		return nil, ErrInvalidPath
	}
	fullPath := filepath.Join(gameServer.Directory, relativePath)
	files, errReadDir := os.ReadDir(fullPath)
	if errReadDir != nil {
		if errors.Is(errReadDir, os.ErrNotExist) {
			log.Error().Err(errReadDir).Msg("Path does not exist")
			return nil, fmt.Errorf("actions: read server directory: %w", errReadDir)
		}
		log.Error().Err(errReadDir).Msg("Failed to read directory")
		return nil, fmt.Errorf("actions: read server directory: %w", errReadDir)
	}

	xylonaFiles := make([]*xylona.File, 0, len(files))
	for _, file := range files {
		fileInfo, errFileInfo := file.Info()
		if errFileInfo != nil {
			log.Error().Err(errFileInfo).Msg("Failed to get file info")
			return nil, fmt.Errorf("actions: stat directory entry: %w", errFileInfo)
		}
		size := fileInfo.Size()
		if fileInfo.IsDir() {
			var totalSize int64

			err := filepath.WalkDir(filepath.Join(fullPath, file.Name()), func(_ string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !d.IsDir() {
					entryInfo, errInfo := d.Info()
					if errInfo != nil {
						return fmt.Errorf("actions: stat nested directory entry: %w", errInfo)
					}
					totalSize += entryInfo.Size()
				}
				return nil
			})

			if err != nil {
				log.Error().Err(err).Msg("Failed to walk through directory")
				return nil, fmt.Errorf("actions: walk directory: %w", err)
			}
			size = totalSize
		}
		xylonaFiles = append(xylonaFiles, &xylona.File{
			Name:         fileInfo.Name(),
			Size:         size,
			IsDirectory:  fileInfo.IsDir(),
			LastModified: timestamppb.New(fileInfo.ModTime()),
		})
	}

	return xylonaFiles, nil
}

// DownloadGameServerFile handles multipart uploads into a game server directory.
func (inst *Instance) DownloadGameServerFile(w http.ResponseWriter, r *http.Request) {
	multiReader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "Error creating multipart reader", http.StatusBadRequest)
		return
	}
	foundGameServerID := false
	foundPath := false
	gameServerID := ""
	relativePath := ""
	for {
		part, errNext := multiReader.NextPart()
		if errNext == io.EOF {
			break
		} else if errNext != nil {
			http.Error(w, "Error reading next part", http.StatusBadRequest)
			return
		}
		switch part.FormName() {
		case "gameServerId":
			gameServerIDBytes, errRead := io.ReadAll(io.LimitReader(part, 10<<10))
			if errRead != nil {
				log.Error().Err(errRead).Msg("Failed to read game server ID")
				http.Error(w, "Error reading game server ID", http.StatusBadRequest)
				return
			}
			gameServerID = string(gameServerIDBytes)
			foundGameServerID = true
		case "path":
			pathBytes, errRead := io.ReadAll(io.LimitReader(part, 1<<20))
			if errRead != nil {
				log.Error().Err(errRead).Msg("Failed to read path")
				http.Error(w, "Error reading path", http.StatusBadRequest)
				return
			}
			relativePath = string(pathBytes)
			foundPath = true
		case "file":
			if !foundGameServerID || !foundPath {
				log.Error().Msg("Game server ID and path must be specified")
				http.Error(w, "Game server ID and path must be specified", http.StatusBadRequest)
				return
			}
			filename := part.FileName()
			target, errResolve := inst.resolveFileRequestTarget(gameServerID)
			if errResolve != nil {
				log.Error().Err(errResolve).Msg("Failed to get game server")
				writeGameServerLookupError(w, errResolve)
				return
			}

			if target.isLocal() {
				errDownload := inst.saveUploadedGameServerFile(target.gameServer, relativePath, filename, part)
				if errDownload != nil {
					log.Error().Err(errDownload).Msg("Failed to download file")
					http.Error(w, "Failed to download file", http.StatusInternalServerError)
					return
				}
				continue
			}

			errProxy := inst.proxyRemoteFileUpload(r.Context(), target, relativePath, filename, part, w)
			if errProxy != nil {
				log.Error().Err(errProxy).Msg("Failed to proxy remote file upload")
				http.Error(w, "Failed to upload file", http.StatusInternalServerError)
				return
			}
			continue
		}
	}
}

func (inst *Instance) saveUploadedGameServerFile(gameServer *models.GameServer, relativePath, fileName string, fileSource io.Reader) error {
	validatedPath, errPath := validateLocalServerPath(gameServer, relativePath)
	if errPath != nil {
		return errPath
	}

	// Sanitize uploaded filename to prevent path traversal (e.g., "../../etc/passwd").
	sanitizedFileName := filepath.Base(fileName)
	if sanitizedFileName == "." || sanitizedFileName == string(filepath.Separator) {
		log.Error().Str("fileName", fileName).Msg("Invalid file name")
		return ErrInvalidPath
	}

	protectedRelativePath := filepath.Join(validatedPath, sanitizedFileName)
	_, errProtected := validateWritableServerPath(gameServer, protectedRelativePath)
	if errProtected != nil {
		return errProtected
	}

	cleanGameServerDir := filepath.Clean(gameServer.Directory)
	gameServerDirPlusPath := filepath.Clean(filepath.Join(cleanGameServerDir, validatedPath))
	gameServerDirPrefix := cleanGameServerDir + string(filepath.Separator)
	if gameServerDirPlusPath != cleanGameServerDir && !strings.HasPrefix(gameServerDirPlusPath, gameServerDirPrefix) {
		log.Error().Str("path", gameServerDirPlusPath).Msg("Upload directory escaped game server root")
		return ErrInvalidPath
	}

	errMkdirAll := os.MkdirAll(gameServerDirPlusPath, 0o750)
	if errMkdirAll != nil {
		log.Error().Err(errMkdirAll).Msg("Failed to create directory")
		return fmt.Errorf("actions: create upload directory: %w", errMkdirAll)
	}

	fullPath := filepath.Clean(filepath.Join(cleanGameServerDir, validatedPath, sanitizedFileName))
	if fullPath != cleanGameServerDir && !strings.HasPrefix(fullPath, gameServerDirPrefix) {
		log.Error().Str("path", fullPath).Msg("Upload file path escaped game server root")
		return ErrInvalidPath
	}

	file, errCreateFile := os.Create(fullPath)
	if errCreateFile != nil {
		log.Error().Err(errCreateFile).Msg("Failed to create file")
		return fmt.Errorf("actions: create uploaded file: %w", errCreateFile)
	}
	defer func() {
		_ = file.Close()
	}()

	_, errCopy := io.Copy(file, fileSource)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy file")
		return fmt.Errorf("actions: write uploaded file: %w", errCopy)
	}

	return nil
}

// GetGameServerFile streams a local game server file to the provided writer.
func (inst *Instance) GetGameServerFile(gameServer *models.GameServer, relativePath string, writer io.Writer, setHeaders, setAsAttachment bool) error {
	validatedPath, errPath := validateLocalServerPath(gameServer, relativePath)
	if errPath != nil {
		return errPath
	}

	cleanGameServerDir := filepath.Clean(gameServer.Directory)
	fullPath := filepath.Clean(filepath.Join(cleanGameServerDir, validatedPath))
	gameServerDirPrefix := cleanGameServerDir + string(filepath.Separator)
	if fullPath != cleanGameServerDir && !strings.HasPrefix(fullPath, gameServerDirPrefix) {
		log.Error().Str("path", fullPath).Msg("Read path escaped game server root")
		return ErrInvalidPath
	}

	file, errReadFile := os.Open(fullPath)
	if errReadFile != nil {
		if errors.Is(errReadFile, os.ErrNotExist) {
			log.Error().Err(errReadFile).Msg("File does not exist")
			return fmt.Errorf("actions: open game server file: %w", errReadFile)
		}
		log.Error().Err(errReadFile).Msg("Failed to read file")
		return fmt.Errorf("actions: open game server file: %w", errReadFile)
	}
	defer func() { _ = file.Close() }()

	fileInfo, errFileInfo := file.Stat()
	if errFileInfo != nil {
		log.Error().Err(errFileInfo).Msg("Failed to get file info")
		return fmt.Errorf("actions: stat game server file: %w", errFileInfo)
	}

	if setHeaders {
		w, ok := writer.(http.ResponseWriter)
		if !ok {
			log.Error().Msg("Writer is not an http.ResponseWriter")
			return errors.New("writer is not an http.ResponseWriter")
		}

		mimeType, errDetect := mimetype.DetectFile(fullPath)
		if errDetect != nil {
			w.Header().Set("Content-Type", "application/octet-stream")
		} else {
			w.Header().Set("Content-Type", mimeType.String())
		}

		w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
		if setAsAttachment {
			// Sanitize filename to prevent header injection via quotes or newlines.
			safeName := strings.Map(func(r rune) rune {
				if r == '"' || r == '\\' || r == '\n' || r == '\r' {
					return '_'
				}
				return r
			}, fileInfo.Name())
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
		}
	}

	_, errCopy := io.Copy(writer, file)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy file")
		return fmt.Errorf("actions: stream game server file: %w", errCopy)
	}
	return nil
}

func (inst *Instance) createGameServerDirectory(gameServer *models.GameServer, owner *models.User) (string, error) {
	gsNameSlug := slug.Make(gameServer.Name)
	gameServerDir := filepath.Join(DefaultInstallPath(), owner.UserName, gsNameSlug)
	errMakePath := os.MkdirAll(gameServerDir, 0o750)
	if errMakePath != nil {
		log.Error().Err(errMakePath).Msg("Failed to create game server directory")
		return "", fmt.Errorf("actions: create game server directory: %w", errMakePath)
	}
	return gameServerDir, nil
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
		return nil, fmt.Errorf("actions: begin install transaction: %w", errTx)
	}
	tx := bob.NewTx(t)

	defer func() {
		_ = tx.Rollback(inst.ctx)
	}()

	node, errGetNode := inst.db.GetNodeByID(gameServer.NodeID)
	if errGetNode != nil {
		log.Error().Err(errGetNode).Msg("Failed to get node")
		return nil, fmt.Errorf("actions: get node for install: %w", errGetNode)
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
	newGameServer.R.Node = node
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Failed to insert game server")
		return nil, fmt.Errorf("actions: insert game server: %w", errInsert)
	}

	installVars := placeholder.BuildVarsFromGameServer(newGameServer)
	installCommand := placeholder.Resolve(gameInstallCommand(game), installVars)
	baseCommand, args := splitCommandString(installCommand)
	preparedCommand := supervisor.PreparedCommand{
		ID:               newGameServer.ID,
		BaseCommand:      baseCommand,
		Args:             args,
		WorkingDirectory: gameServer.Directory,
		User:             gameServer.UserID,
		GameServerID:     &gameServer.ID,
		Status:           xylona.Status_INSTALLING,
		ServiceID:        gameServer.GameID,
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
		return nil, fmt.Errorf("actions: start install command: %w", err)
	}

	errCommit := tx.Commit(inst.ctx)
	if errCommit != nil {
		log.Error().Str("Game Server ID", gameServer.ID).Err(errCommit).Msg("Failed to commit transaction")
		return nil, fmt.Errorf("actions: commit install transaction: %w", errCommit)
	}

	eb := eventbus.Get()
	eb.Publish("game_server_created", newGameServer)

	return newGameServer, nil
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
		BaseCommand:      baseCommand,
		Args:             args,
		WorkingDirectory: gameServer.Directory,
		User:             gameServer.UserID,
		GameServerID:     &gameServer.ID,
		Status:           xylona.Status_ONLINE,
		ServiceID:        gameServer.GameID,
		CallbackFunction: func(_ *supervisor.Command) {
			// log.Info().Msg("Game server stopped")
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
		log.Error().Err(err).Msg("Failed to start game server")
	}
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
		WorkingDirectory: gameServer.Directory,
		User:             gameServer.UserID,
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

type minecraftUpdatePlan struct {
	softwareID          string
	softwareName        string
	provider            modproviders.ModProvider
	targetVersion       string
	downloadVersionID   string
	downloadVersionName string
	plannedFileName     string
}

func (inst *Instance) resolveMinecraftUpdatePlan(
	gameServer *models.GameServer,
) (*minecraftUpdatePlan, error) {
	if gameServer.GameID != "minecraft" {
		return nil, ErrNotMinecraftServer
	}
	if gameServer.R.Game == nil {
		return nil, errMinecraftGameRelationNotLoaded
	}

	resolved, errResolve := updateproviders.ResolveModelConfig(gameServer.R.Game, gameServer)
	if errResolve != nil {
		return nil, fmt.Errorf("resolve minecraft update config: %w", errResolve)
	}

	softwareID := strings.TrimSpace(resolved.VariantID)
	if softwareID == "" {
		softwareID = "vanilla"
	}

	if resolved.Provider.Kind != updateproviders.ProviderKindPaperMC &&
		resolved.Provider.Kind != updateproviders.ProviderKindMojang {
		return nil, ErrMinecraftVariantUpdateNotSupported
	}

	provider, ok := minecraftUpdateProviderLookup(resolved.Provider.Kind)
	if !ok {
		return nil, fmt.Errorf("jar source provider not found for %s", resolved.Provider.Kind)
	}

	updateCtx, cancel := context.WithTimeout(inst.ctx, 10*time.Minute)
	defer cancel()

	sourceID := strings.TrimSpace(resolved.Provider.SourceID)
	if sourceID == "" {
		switch resolved.Provider.Kind {
		case updateproviders.ProviderKindPaperMC:
			sourceID = softwareID
		case updateproviders.ProviderKindMojang:
			sourceID = "vanilla"
		}
	}

	details, errDetails := provider.GetModDetails(updateCtx, sourceID, nil)
	if errDetails != nil {
		return nil, fmt.Errorf("get minecraft variant versions: %w", errDetails)
	}
	if details == nil || len(details.Versions) == 0 {
		return nil, fmt.Errorf("no versions available for minecraft variant %s", sourceID)
	}

	targetVersion := resolvePreferredMinecraftTarget(details.Versions, resolved.Target)
	if targetVersion == "" {
		return nil, fmt.Errorf("no usable target version for minecraft variant %s", sourceID)
	}

	downloadVersionID := targetVersion
	downloadVersionName := targetVersion
	plannedFileName := ""

	builds, errBuilds := provider.GetVersions(updateCtx, sourceID, targetVersion, nil)
	if errBuilds == nil && len(builds) > 0 {
		selectedBuild := builds[len(builds)-1]
		if selectedBuild.VersionID != "" {
			downloadVersionID = selectedBuild.VersionID
		}
		if selectedBuild.VersionString != "" {
			downloadVersionName = selectedBuild.VersionString
		}
		if fileName := plannedDownloadFileName(selectedBuild.DownloadURL); fileName != "" {
			plannedFileName = fileName
		}
	}
	if downloadVersionName == "" {
		downloadVersionName = downloadVersionID
	}

	softwareName := strings.TrimSpace(resolved.VariantName)
	if softwareName == "" {
		softwareName = strings.TrimSpace(sourceID)
	}

	return &minecraftUpdatePlan{
		softwareID:          softwareID,
		softwareName:        softwareName,
		provider:            provider,
		targetVersion:       targetVersion,
		downloadVersionID:   downloadVersionID,
		downloadVersionName: downloadVersionName,
		plannedFileName:     plannedFileName,
	}, nil
}

func resolvePreferredMinecraftTarget(versions []modproviders.ModVersion, preferred string) string {
	normalizedPreferred := strings.TrimSpace(preferred)
	if normalizedPreferred != "" {
		for _, version := range versions {
			if strings.TrimSpace(version.VersionID) == normalizedPreferred {
				return normalizedPreferred
			}
			if strings.TrimSpace(version.VersionString) == normalizedPreferred {
				return normalizedPreferred
			}
		}
	}

	if len(versions) == 0 {
		return ""
	}

	latestVersion := versions[0]
	targetVersion := strings.TrimSpace(latestVersion.VersionString)
	if targetVersion == "" {
		targetVersion = strings.TrimSpace(latestVersion.VersionID)
	}
	return targetVersion
}

func plannedDownloadFileName(downloadURL string) string {
	trimmed := strings.TrimSpace(downloadURL)
	if trimmed == "" {
		return ""
	}

	parsedURL, errParse := url.Parse(trimmed)
	if errParse != nil {
		return path.Base(trimmed)
	}
	if parsedURL.Path == "" {
		return ""
	}
	return path.Base(parsedURL.Path)
}

func (inst *Instance) tryUpdateMinecraftServerSoftware(gameServer *models.GameServer) (bool, error) {
	plan, errPlan := inst.resolveMinecraftUpdatePlan(gameServer)
	if errors.Is(errPlan, ErrNotMinecraftServer) {
		return false, nil
	}
	if errPlan != nil {
		return true, errPlan
	}
	if plan == nil {
		return false, nil
	}

	updateCtx, cancel := context.WithTimeout(inst.ctx, 10*time.Minute)
	defer cancel()

	files, errDownload := plan.provider.Download(
		updateCtx,
		plan.softwareID,
		plan.downloadVersionID,
		gameServer.Directory,
	)
	if errDownload != nil {
		return true, fmt.Errorf("download minecraft update: %w", errDownload)
	}

	newExecutable := primaryDownloadedFile(files)
	if oldExecutable := gameServer.ServerExecutable.GetOr(""); oldExecutable != "" && oldExecutable != newExecutable {
		oldPath := filepath.Join(gameServer.Directory, oldExecutable)
		if errRemove := os.Remove(oldPath); errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			log.Warn().Err(errRemove).Str("game_server_id", gameServer.ID).Str("path", oldPath).
				Msg("Failed to remove superseded game server executable")
		}
	}

	if inst.db == nil {
		if newExecutable == "" {
			gameServer.ServerExecutable = null.FromPtr[string](nil)
		} else {
			gameServer.ServerExecutable = null.From(newExecutable)
		}
		gameServer.ServerSoftware = null.From(plan.softwareID)
		return true, nil
	}

	setter := &models.GameServerSetter{
		ID:             omit.From(gameServer.ID),
		ServerSoftware: omitnull.From(plan.softwareID),
	}
	if newExecutable == "" {
		setter.ServerExecutable = omitnull.FromNull(null.Val[string]{})
	} else {
		setter.ServerExecutable = omitnull.From(newExecutable)
	}

	updated, errUpdate := inst.db.UpdateGameServer(inst.db.DB, setter)
	if errUpdate != nil {
		return true, fmt.Errorf("persist minecraft update: %w", errUpdate)
	}
	gameServer.ServerExecutable = updated.ServerExecutable
	gameServer.ServerSoftware = updated.ServerSoftware
	return true, nil
}

func primaryDownloadedFile(files []modproviders.DownloadedFile) string {
	for _, f := range files {
		if f.IsPrimary {
			return f.Path
		}
	}
	return ""
}

// ReadGameServerBuffer returns the buffered console output for a running server.
func (inst *Instance) ReadGameServerBuffer(gameServer *models.GameServer) string {
	gameServerCommand, err := inst.supervisorInstance.GetCommandByID(gameServer.ID)
	if err != nil {
		// log.Error().Err(err).Msg("Failed to get game server command")
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

// PurgeAllGameServerFiles deletes the server's working directory.
func (inst *Instance) PurgeAllGameServerFiles(gameServer *models.GameServer) error {
	err := os.RemoveAll(gameServer.Directory)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete game server files")
		return fmt.Errorf("actions: delete game server files: %w", err)
	}
	return nil
}
