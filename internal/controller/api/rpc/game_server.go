package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/controller/protomap"
	xylonadb "github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func fallbackNodeID(requestNodeID string, defaultNodeID string) string {
	trimmedNodeID := strings.TrimSpace(requestNodeID)
	if trimmedNodeID == "" {
		return defaultNodeID
	}
	return trimmedNodeID
}

func mergeEditableGameServerUpdate(
	existingGameServer *models.GameServer,
	incomingGameServer *models.GameServer,
	allowProvisioningChanges bool,
) *models.GameServer {
	var merged models.GameServer
	if allowProvisioningChanges {
		merged = *incomingGameServer
		merged.ID = existingGameServer.ID
	} else {
		merged = *existingGameServer
		merged.Name = incomingGameServer.Name
		merged.SetPlayers = incomingGameServer.SetPlayers
		merged.AutoRestartEnabled = incomingGameServer.AutoRestartEnabled
		merged.AutoRestartMaxRetries = incomingGameServer.AutoRestartMaxRetries
		merged.AutoRestartCooldownSeconds = incomingGameServer.AutoRestartCooldownSeconds
	}

	// Backup settings are managed exclusively through UpdateBackupSettings.
	merged.BackupsEnabled = existingGameServer.BackupsEnabled
	merged.BackupDirectory = existingGameServer.BackupDirectory
	merged.MaxBackups = existingGameServer.MaxBackups
	merged.EnvVars = existingGameServer.EnvVars
	return &merged
}

func normalizeBackupRetention(maxBackups int64) int64 {
	if maxBackups > 0 {
		return maxBackups
	}

	return actions.DefaultScheduledBackupRetention
}

func defaultBackupDirectoryForServer(serverDirectory string) (string, error) {
	trimmedDirectory := strings.TrimSpace(serverDirectory)
	if trimmedDirectory == "" {
		defaultBackupDirectory, errDefaultBackupDirectory := actions.DefaultBackupDirectory()
		if errDefaultBackupDirectory != nil {
			return "", fmt.Errorf("rpc: resolve default backup directory: %w", errDefaultBackupDirectory)
		}
		return defaultBackupDirectory, nil
	}

	parentDirectory := nodePathDir(trimmedDirectory)
	if parentDirectory == "." || parentDirectory == "" {
		defaultBackupDirectory, errDefaultBackupDirectory := actions.DefaultBackupDirectory()
		if errDefaultBackupDirectory != nil {
			return "", fmt.Errorf("rpc: resolve default backup directory: %w", errDefaultBackupDirectory)
		}
		return defaultBackupDirectory, nil
	}

	return joinNodePath(parentDirectory, "backups"), nil
}

func nodePathDir(pathValue string) string {
	trimmedPath := strings.TrimRight(strings.TrimSpace(pathValue), `/\`)
	index := strings.LastIndexAny(trimmedPath, `/\`)
	if index < 0 {
		return ""
	}
	if index == 0 {
		return trimmedPath[:1]
	}
	if index == 2 && len(trimmedPath) > 1 && trimmedPath[1] == ':' {
		return trimmedPath[:3]
	}
	return trimmedPath[:index]
}

func joinNodePath(directory string, name string) string {
	separator := nodePathSeparator(directory)
	trimmedDirectory := strings.TrimRight(directory, `/\`)
	if len(trimmedDirectory) == 2 && trimmedDirectory[1] == ':' {
		return trimmedDirectory + separator + name
	}
	if trimmedDirectory == "" {
		return name
	}
	if trimmedDirectory == "/" {
		return "/" + name
	}
	return trimmedDirectory + separator + name
}

func nodePathSeparator(pathValue string) string {
	hasBackslash := strings.Contains(pathValue, `\`)
	hasSlash := strings.Contains(pathValue, "/")
	if hasBackslash && !hasSlash {
		return `\`
	}
	return "/"
}

func (xs *XylonaService) ensureNodeScopedIP(ctx context.Context, nodeID string, address string) error {
	trimmedNodeID := strings.TrimSpace(nodeID)
	trimmedAddress := strings.TrimSpace(address)
	if trimmedAddress == "" {
		return invalidArg("invalid IP")
	}

	_, errGetIP := xs.db.GetIPByNodeIDAndAddress(trimmedNodeID, trimmedAddress)
	if errGetIP == nil {
		return nil
	}
	if !errors.Is(errGetIP, sql.ErrNoRows) {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("lookup node IP: %w", errGetIP))
	}

	for _, runtimeIP := range xs.listRuntimeIPs(ctx, trimmedNodeID) {
		if runtimeIP == nil {
			continue
		}
		if strings.TrimSpace(runtimeIP.GetAddress()) != trimmedAddress {
			continue
		}

		_, errUpsertIP := xs.db.UpsertIP(&models.IPSetter{
			Address:            omit.From(trimmedAddress),
			Usable:             omit.From(runtimeIP.GetUsable()),
			External:           omit.From(runtimeIP.GetExternal()),
			AutomaticallyAdded: omit.From(true),
			NodeID:             omit.From(trimmedNodeID),
		})
		if errUpsertIP != nil && !errors.Is(errUpsertIP, xylonadb.ErrIPConflict) {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("register node IP: %w", errUpsertIP))
		}
		return nil
	}

	return invalidArg("invalid IP")
}

// findAvailablePort checks each game's complete runtime port footprint and
// returns the next available configured game/query pair. Games that bind to
// all IPs occupy that footprint across every IP on the node.
// excludeServerID can be set to skip a specific server (useful when editing an existing server).
func (xs *XylonaService) findAvailablePort(nodeID string, ip string, port int64, queryPort int64, game *models.Game, excludeServerID string) (int64, int64, error) {
	existingServers, errGetServers := xs.db.GetGameServersByNodeID(nodeID)
	if errGetServers != nil {
		return 0, 0, fmt.Errorf("rpc: list game servers by node: %w", errGetServers)
	}

	selectedIP := strings.TrimSpace(ip)
	selectedBindsToAllIPs := game != nil && game.BindsToAllIps
	usedPorts := make(map[int64]bool)
	for _, s := range existingServers {
		if s.ID == excludeServerID {
			continue
		}
		existingBindsToAllIPs := s.R.Game != nil && s.R.Game.BindsToAllIps
		sameIP := strings.TrimSpace(s.IP) == selectedIP
		if !sameIP && !selectedBindsToAllIPs && !existingBindsToAllIPs {
			continue
		}
		serverPorts, errServerPorts := xs.gameServerPortFootprint(s)
		if errServerPorts != nil {
			return 0, 0, errServerPorts
		}
		for _, usedPort := range serverPorts {
			usedPorts[usedPort] = true
		}
	}

	gameID := ""
	if game != nil {
		gameID = game.ID
	}
	// Auto-increment both ports on any conflict so their configured offset is preserved.
	for {
		candidatePorts := gameServerPortFootprint(gameID, port, queryPort)
		if gameID == minecraftGameID && excludeServerID != "" {
			mapSettings, errMapSettings := xs.db.GetGameServerMinecraftMap(excludeServerID)
			if errMapSettings != nil {
				return 0, 0, fmt.Errorf("load Minecraft map port footprint: %w", errMapSettings)
			}
			if mapSettings.Enabled {
				candidatePorts = append(candidatePorts, queryPort+1)
			}
		}
		conflict := false
		for _, candidatePort := range candidatePorts {
			if candidatePort < 1 || candidatePort > 65535 {
				return 0, 0, fmt.Errorf("no available game/query port pair on node %s", nodeID)
			}
			if usedPorts[candidatePort] {
				conflict = true
				break
			}
		}
		if !conflict {
			return port, queryPort, nil
		}
		port++
		queryPort++
	}
}

func gameServerPortFootprint(gameID string, port int64, queryPort int64) []int64 {
	ports := []int64{port}
	if queryPort != port {
		ports = append(ports, queryPort)
	}
	switch gameID {
	case "7_days_to_die":
		for offset := int64(1); offset <= 3; offset++ {
			candidate := port + offset
			if candidate == queryPort {
				continue
			}
			ports = append(ports, candidate)
		}
		ports = append(ports, queryPort+1)
	case "project_zomboid":
		ports = append(ports, port+1)
	case "conan_exiles":
		ports = append(ports, port+1, port+2)
	case "sons_of_the_forest":
		ports = append(ports, queryPort+1)
	case "rust":
		ports = append(ports, port+67)
	case "garrys_mod", "team_fortress_2":
		ports = append(ports, port+1, port+2)
	}
	return ports
}

func (xs *XylonaService) gameServerPortFootprint(gameServer *models.GameServer) ([]int64, error) {
	if gameServer == nil {
		return nil, errors.New("game server is required")
	}
	ports := gameServerPortFootprint(gameServer.GameID, gameServer.Port, gameServer.QueryPort)
	if gameServer.GameID != minecraftGameID {
		return ports, nil
	}
	settings, errSettings := xs.db.GetGameServerMinecraftMap(gameServer.ID)
	if errSettings != nil {
		return nil, fmt.Errorf("get Minecraft map port footprint: %w", errSettings)
	}
	if settings.Enabled {
		ports = append(ports, gameServer.QueryPort+1)
	}
	return ports, nil
}

// CreateGameServer creates a new local game server.
func (xs *XylonaService) CreateGameServer(ctx context.Context, request *connect.Request[xylona.CreateGameServerRequest]) (*connect.Response[xylona.CreateGameServerResponse], error) {
	callingUser, errCallingUser := xs.getUserFromHeader(request.Header())
	if errCallingUser != nil {
		return nil, unauthenticated()
	}

	if !callingUser.SuperUser {
		return nil, permissionDenied("only superusers can create game servers")
	}

	targetUserID := request.Msg.GetGameServer().GetUserId()
	user, errGetUser := xs.db.GetUserByID(targetUserID)
	if errGetUser != nil {
		return nil, dbLookup(errGetUser)
	}
	game, errGetGame := xs.db.GetGameByID(request.Msg.GetGameServer().GetGameId())
	if errGetGame != nil {
		return nil, dbLookup(errGetGame)
	}

	newGameServerModel := protomap.GameServerProtoToModel(request.Msg.GetGameServer())
	encodedEnvironment, errEnvironment := prepareCreateGameServerEnvironment(game, request.Msg.GetEnvVars())
	if errEnvironment != nil {
		return nil, invalidArg(errEnvironment.Error())
	}
	newGameServerModel.EnvVars = encodedEnvironment
	if strings.TrimSpace(newGameServerModel.NodeID) == "" {
		localSettings, errSettings := xs.db.GetLocalSettings()
		if errSettings != nil {
			return nil, internalErrf("failed to resolve local node")
		}
		newGameServerModel.NodeID = fallbackNodeID(newGameServerModel.NodeID, localSettings.NodeID)
	}

	_, errNode := xs.db.GetNodeByID(newGameServerModel.NodeID)
	if errNode != nil {
		if errors.Is(errNode, sql.ErrNoRows) {
			return nil, invalidArg("invalid node")
		}
		return nil, internalErrf("failed to validate node")
	}
	errEnsureIP := xs.ensureNodeScopedIP(ctx, newGameServerModel.NodeID, newGameServerModel.IP)
	if errEnsureIP != nil {
		return nil, errEnsureIP
	}

	backupCapability, errBackupCapability := actions.ResolveBackupCapability(
		ctx,
		xs.db,
		xs.nodeRegistry,
		newGameServerModel,
		game,
	)
	if errBackupCapability != nil {
		return nil, connect.NewError(
			connect.CodeFailedPrecondition,
			errors.New(backupCapability.DisabledReason),
		)
	}
	newGameServerModel.BackupsEnabled = backupCapability.Supported
	newGameServerModel.MaxBackups = normalizeBackupRetention(newGameServerModel.MaxBackups)

	backupDirectory := strings.TrimSpace(newGameServerModel.BackupDirectory)
	if backupDirectory == "" {
		defaultBackupDirectory, errDefaultBackupDirectory := defaultBackupDirectoryForServer(newGameServerModel.Directory)
		if errDefaultBackupDirectory != nil {
			return nil, fmt.Errorf("rpc: resolve default backup directory: %w", errDefaultBackupDirectory)
		}
		backupDirectory = defaultBackupDirectory
	}
	newGameServerModel.BackupDirectory = backupDirectory

	errValidateSubmission := validateGameServerSubmission(game, newGameServerModel, true)
	if errValidateSubmission != nil {
		return nil, errValidateSubmission
	}

	// Check for port conflicts and auto-increment if necessary.
	availablePort, availableQueryPort, errPortCheck := xs.findAvailablePort(
		newGameServerModel.NodeID, newGameServerModel.IP, newGameServerModel.Port, newGameServerModel.QueryPort, game, "",
	)
	if errPortCheck != nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, errPortCheck)
	}
	newGameServerModel.Port = availablePort
	newGameServerModel.QueryPort = availableQueryPort

	installGameServer := xs.installGameServerFn
	if installGameServer == nil {
		newGameServer, errInstallGameServer := xs.actionsInst.InstallGameServer(game, newGameServerModel, user)
		if errInstallGameServer != nil {
			return nil, internalErr()
		}

		response := &xylona.CreateGameServerResponse{
			GameServer: protomap.GameServerModelToProto(newGameServer, xs.versionState),
		}
		return connect.NewResponse(response), nil
	}
	newGameServer, errInstallGameServer := installGameServer(game, newGameServerModel, user)
	if errInstallGameServer != nil {
		return nil, internalErr()
	}

	response := &xylona.CreateGameServerResponse{
		GameServer: protomap.GameServerModelToProto(newGameServer, xs.versionState),
	}
	return connect.NewResponse(response), nil
}

// EditGameServer updates an existing game server.
func (xs *XylonaService) EditGameServer(ctx context.Context, request *connect.Request[xylona.EditGameServerRequest]) (*connect.Response[xylona.EditGameServerResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	existingGameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, existingGameServer, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	incomingGameServer := protomap.GameServerProtoToModel(request.Msg.GetGameServer())
	gameServerModel := mergeEditableGameServerUpdate(existingGameServer, incomingGameServer, user.SuperUser)
	gameServerModel.NodeID = fallbackNodeID(gameServerModel.NodeID, existingGameServer.NodeID)

	game, errGetGame := xs.db.GetGameByID(gameServerModel.GameID)
	if errGetGame != nil {
		return nil, dbLookup(errGetGame)
	}

	errValidateSubmission := validateGameServerSubmission(game, gameServerModel, user.SuperUser)
	if errValidateSubmission != nil {
		return nil, errValidateSubmission
	}

	_, errNode := xs.db.GetNodeByID(gameServerModel.NodeID)
	if errNode != nil {
		if errors.Is(errNode, sql.ErrNoRows) {
			return nil, invalidArg("invalid node")
		}
		return nil, internalErrf("failed to validate node")
	}
	errEnsureIP := xs.ensureNodeScopedIP(ctx, gameServerModel.NodeID, gameServerModel.IP)
	if errEnsureIP != nil {
		return nil, errEnsureIP
	}

	portScopeChanged := gameServerModel.NodeID != existingGameServer.NodeID ||
		gameServerModel.GameID != existingGameServer.GameID ||
		gameServerModel.IP != existingGameServer.IP ||
		gameServerModel.Port != existingGameServer.Port ||
		gameServerModel.QueryPort != existingGameServer.QueryPort
	if portScopeChanged {
		availablePort, availableQueryPort, errPortCheck := xs.findAvailablePort(
			gameServerModel.NodeID, gameServerModel.IP, gameServerModel.Port, gameServerModel.QueryPort, game, existingGameServer.ID,
		)
		if errPortCheck != nil {
			return nil, connect.NewError(connect.CodeAlreadyExists, errPortCheck)
		}
		gameServerModel.Port = availablePort
		gameServerModel.QueryPort = availableQueryPort
	}

	setter := protomap.GameServerModelToSetter(gameServerModel)
	_, errUpdate := xs.db.UpdateGameServerForEdit(setter, existingGameServer.UserID)
	if errUpdate != nil {
		return nil, internalErr()
	}

	gameServerProto := protomap.GameServerModelToProto(gameServerModel, xs.versionState)
	if !user.SuperUser {
		redactGameServerForNonSuperuser(gameServerProto)
	}
	return connect.NewResponse(&xylona.EditGameServerResponse{Game_Server: gameServerProto}), nil
}

// RemoveGameServer stops a game server, confirms it is offline, removes its
// files, and only then deletes its database row.
func (xs *XylonaService) RemoveGameServer(ctx context.Context, request *connect.Request[xylona.RemoveGameServerRequest]) (*connect.Response[xylona.RemoveGameServerResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.delete")
	if errPermission != nil {
		return nil, errPermission
	}
	if gameServer.GameID == minecraftGameID {
		mapSettings, errMapSettings := xs.db.GetGameServerMinecraftMap(gameServer.ID)
		if errMapSettings != nil {
			return nil, internalErrf("failed to load Minecraft map settings")
		}
		errDisableMap := xs.db.UpdateGameServerMinecraftMapConfig(
			gameServer.ID,
			false,
			mapSettings.WorldName,
			false,
			user.ID,
		)
		if errDisableMap != nil {
			return nil, internalErrf("failed to disable Minecraft map before server removal")
		}
		client, errClient := xs.resolveNodeClient(gameServer)
		if errClient != nil {
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("minecraft map node is unavailable; the server was not removed"))
		}
		errStopMap := client.StopMinecraftMap(ctx, gameServer.ID)
		if errStopMap != nil {
			return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("stop Minecraft map before server removal: %w", errStopMap))
		}
	}

	errStop := xs.actionsInst.StopGameServer(ctx, gameServer)
	if errStop != nil {
		return nil, stopGameServerConnectError(errStop)
	}
	errConfirm := xs.waitForGameServerOffline(ctx, gameServer, 10*time.Second)
	if errConfirm != nil {
		return nil, errConfirm
	}
	errRemove := xs.actionsInst.RemoveGameServer(ctx, gameServer)
	if errRemove != nil {
		return nil, removeGameServerConnectError(errRemove)
	}
	return connect.NewResponse(&xylona.RemoveGameServerResponse{}), nil
}

func (xs *XylonaService) waitForGameServerOffline(ctx context.Context, gameServer *models.GameServer, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		errWait := waitCtx.Err()
		if errWait != nil {
			return gameServerOfflineWaitError(ctx)
		}
		status, _, errStatus := xs.resolveGameServerRuntimeState(waitCtx, gameServer)
		if errStatus != nil {
			if ctx.Err() != nil || waitCtx.Err() != nil {
				return gameServerOfflineWaitError(ctx)
			}
			return connect.NewError(
				connect.CodeUnavailable,
				fmt.Errorf("could not confirm game server shutdown: %w", errStatus),
			)
		}
		switch status {
		case xylona.Status_OFFLINE:
			return nil
		case xylona.Status_UNKNOWN:
			return connect.NewError(
				connect.CodeUnavailable,
				errors.New("could not confirm game server shutdown because its status is unavailable"),
			)
		default:
		}

		select {
		case <-waitCtx.Done():
			return gameServerOfflineWaitError(ctx)
		case <-ticker.C:
		}
	}
}

func gameServerOfflineWaitError(ctx context.Context) error {
	errContext := ctx.Err()
	if errContext != nil {
		return connect.NewError(contextConnectCode(errContext), fmt.Errorf("confirm game server shutdown: %w", errContext))
	}
	return connect.NewError(
		connect.CodeFailedPrecondition,
		errors.New("game server did not reach offline status before the operation timed out"),
	)
}

func removeGameServerConnectError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(contextConnectCode(err), fmt.Errorf("remove game server: %w", err))
	}
	if errors.Is(err, noderegistry.ErrNodeNotRegistered) {
		return connect.NewError(connect.CodeUnavailable, fmt.Errorf("remove game server: %w", err))
	}
	code := connect.CodeOf(err)
	switch code {
	case connect.CodeCanceled, connect.CodeDeadlineExceeded, connect.CodeUnavailable:
		return connect.NewError(code, fmt.Errorf("remove game server: %w", err))
	default:
		return connect.NewError(connect.CodeInternal, fmt.Errorf("remove game server: %w", err))
	}
}

// StartGameServer starts a game server managed by the controller's embedded node.
func (xs *XylonaService) StartGameServer(_ context.Context, request *connect.Request[xylona.StartGameServerRequest]) (*connect.Response[xylona.StartGameServerResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.start")
	if errPermission != nil {
		return nil, errPermission
	}

	_, errStart := xs.actionsInst.StartGameServer(gameServer)
	if errStart != nil {
		return nil, startGameServerConnectError(errStart)
	}
	return connect.NewResponse(&xylona.StartGameServerResponse{}), nil
}

func startGameServerConnectError(err error) error {
	var startErr *actions.StartGameServerError
	if errors.As(err, &startErr) && startErr != nil {
		switch startErr.Kind {
		case actions.StartFailureConfiguration:
			return connect.NewError(connect.CodeFailedPrecondition, errors.New(startErr.Error()))
		case actions.StartFailureUnavailable:
			return connect.NewError(connect.CodeUnavailable, errors.New(startErr.Error()))
		default:
			return connect.NewError(connect.CodeInternal, errors.New(startErr.Error()))
		}
	}
	return connect.NewError(connect.CodeInternal, errors.New(err.Error()))
}

// StopGameServer stops a game server managed by the controller's embedded node.
func (xs *XylonaService) StopGameServer(ctx context.Context, request *connect.Request[xylona.StopGameServerRequest]) (*connect.Response[xylona.StopGameServerResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.stop")
	if errPermission != nil {
		return nil, errPermission
	}

	errStop := xs.actionsInst.StopGameServer(ctx, gameServer)
	if errStop != nil {
		return nil, stopGameServerConnectError(errStop)
	}
	return connect.NewResponse(&xylona.StopGameServerResponse{}), nil
}

// RestartGameServer performs one permission-checked stop, waits for the
// authoritative offline state, and starts the server again.
func (xs *XylonaService) RestartGameServer(ctx context.Context, request *connect.Request[xylona.RestartGameServerRequest]) (*connect.Response[xylona.RestartGameServerResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.restart")
	if errPermission != nil {
		return nil, errPermission
	}
	releaseOperation, errOperation := xs.actionsInst.TryBeginGameServerLifecycleOperation(gameServer.ID)
	if errors.Is(errOperation, actions.ErrGameServerOperationInProgress) {
		return nil, connect.NewError(connect.CodeAlreadyExists, errOperation)
	}
	if errOperation != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("reserve game server restart: %w", errOperation))
	}
	defer releaseOperation()

	status, _, errStatus := xs.resolveGameServerRuntimeState(ctx, gameServer)
	if errStatus != nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("confirm game server status before restart: %w", errStatus))
	}
	if status != xylona.Status_ONLINE {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("game server must be online to restart"))
	}
	errStop := xs.actionsInst.StopGameServer(ctx, gameServer)
	if errStop != nil {
		return nil, stopGameServerConnectError(errStop)
	}
	errWait := xs.waitForGameServerOffline(ctx, gameServer, 30*time.Second)
	if errWait != nil {
		return nil, errWait
	}
	_, errStart := xs.actionsInst.StartGameServer(gameServer)
	if errStart != nil {
		return nil, startGameServerConnectError(errStart)
	}
	return connect.NewResponse(&xylona.RestartGameServerResponse{}), nil
}

func stopGameServerConnectError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(contextConnectCode(err), fmt.Errorf("stop game server: %w", err))
	}
	code := connect.CodeOf(err)
	switch code {
	case connect.CodeCanceled, connect.CodeDeadlineExceeded, connect.CodeUnavailable:
		return connect.NewError(code, fmt.Errorf("stop game server: %w", err))
	default:
		return connect.NewError(connect.CodeUnavailable, fmt.Errorf("stop game server: %w", err))
	}
}

// ReadGameServerOutput returns buffered console output.
func (xs *XylonaService) ReadGameServerOutput(ctx context.Context, request *connect.Request[xylona.ReadGameServerOutputRequest]) (*connect.Response[xylona.ReadGameServerOutputResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.console")
	if errPermission != nil {
		return nil, errPermission
	}

	output, errRead := xs.actionsInst.ReadGameServerBuffer(ctx, gameServer)
	if errRead != nil {
		if errors.Is(errRead, context.Canceled) || errors.Is(errRead, context.DeadlineExceeded) {
			return nil, connect.NewError(contextConnectCode(errRead), fmt.Errorf("read game server console: %w", errRead))
		}
		code := connect.CodeOf(errRead)
		if code == connect.CodeCanceled || code == connect.CodeDeadlineExceeded {
			return nil, connect.NewError(code, fmt.Errorf("read game server console: %w", errRead))
		}
		log.Warn().Err(errRead).Str("game_server_id", gameServer.ID).
			Msg("Failed to read game server console buffer")
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("game server console is unavailable"))
	}
	return connect.NewResponse(&xylona.ReadGameServerOutputResponse{Output: output}), nil
}

// SendGameServerInput sends console input to a running server.
func (xs *XylonaService) SendGameServerInput(ctx context.Context, request *connect.Request[xylona.SendGameServerInputRequest]) (*connect.Response[xylona.SendGameServerInputResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.console")
	if errPermission != nil {
		return nil, errPermission
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}
	errSend := client.SendConsoleInput(ctx, gameServer.ID, request.Msg.GetInput())
	if errSend != nil {
		if errors.Is(errSend, node.ErrProcessNotFound) || errors.Is(errSend, os.ErrNotExist) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("game server process is not running"))
		}
		rejectedError, isRejected := errors.AsType[*node.ConsoleInputRejectedError](errSend)
		if isRejected {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New(rejectedError.Detail()))
		}
		if errors.Is(errSend, node.ErrConsoleInputUnavailable) {
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("game server console input is reconnecting; retry shortly"))
		}
		if errors.Is(errSend, context.Canceled) || errors.Is(errSend, context.DeadlineExceeded) {
			return nil, connect.NewError(contextConnectCode(errSend), fmt.Errorf("send game server console input: %w", errSend))
		}
		code := connect.CodeOf(errSend)
		switch code {
		case connect.CodeCanceled, connect.CodeDeadlineExceeded, connect.CodeUnavailable:
			return nil, connect.NewError(code, fmt.Errorf("send game server console input: %w", errSend))
		}
		log.Error().Err(errSend).Msg("Failed to send input to game server")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	return connect.NewResponse(&xylona.SendGameServerInputResponse{}), nil
}

// ListDirectoryFiles lists files in a server directory via the controller's embedded node.
func (xs *XylonaService) ListDirectoryFiles(ctx context.Context, request *connect.Request[xylona.ListDirectoryFilesRequest]) (*connect.Response[xylona.ListDirectoryFilesResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetGameServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.view")
	if errPermission != nil {
		return nil, errPermission
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}
	entries, errListGameServerFiles := client.ListFiles(ctx, gameServer.Directory, request.Msg.GetPath())
	if errListGameServerFiles != nil {
		if errors.Is(errListGameServerFiles, actions.ErrInvalidPath) || errors.Is(errListGameServerFiles, node.ErrInvalidPath) {
			return nil, invalidArg("invalid path")
		}
		if errors.Is(errListGameServerFiles, os.ErrNotExist) {
			return nil, notFoundErr()
		}
		return nil, internalErr()
	}
	files := make([]*xylona.File, len(entries))
	for i, entry := range entries {
		files[i] = &xylona.File{
			Name:         entry.Name,
			Size:         entry.Size,
			IsDirectory:  entry.IsDirectory,
			LastModified: timestamppb.New(entry.LastModified),
		}
	}
	return connect.NewResponse(&xylona.ListDirectoryFilesResponse{Files: files}), nil
}

// GetGameServer returns details for a game server managed by the controller's embedded node.
func (xs *XylonaService) GetGameServer(ctx context.Context, request *connect.Request[xylona.GetGameServerRequest]) (*connect.Response[xylona.GetGameServerResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}

	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.view")
	if errPermission != nil {
		return nil, errPermission
	}

	runtimeStatus, snap, errSnap := xs.resolveGameServerRuntimeState(ctx, gameServer)
	if errSnap != nil {
		log.Debug().Err(errSnap).Str("game_server_id", gameServer.ID).
			Msg("GetGameServer: snapshot unavailable; using unknown status")
	}
	gameServer.Status = runtimeStatus.String()

	gsProto := protomap.GameServerModelToProto(gameServer, xs.versionState)
	if !user.SuperUser {
		redactGameServerForNonSuperuser(gsProto)
	}

	resolvedVersion, resolvedVersionInfo, errResolveVersion := xs.resolveVersionData(ctx, gameServer, actions.VersionResolveOptions{
		AllowAsync: true,
	})
	if errResolveVersion != nil {
		log.Debug().Err(errResolveVersion).Str("game_server_id", gameServer.ID).
			Msg("GetGameServer: version resolution unavailable")
	} else {
		gsProto.Version = resolvedVersion
		gsProto.VersionInfo = resolvedVersionInfo
	}

	gsProto.EffectivePermissions = xs.computeEffectivePermissions(user, gameServer)
	applyProcessMetricsToProto(gsProto, snap)
	return connect.NewResponse(&xylona.GetGameServerResponse{GameServer: gsProto}), nil
}

// UpdateGameServer triggers an update for a game server managed by the controller's embedded node.
func (xs *XylonaService) UpdateGameServer(_ context.Context, request *connect.Request[xylona.UpdateGameServerRequest]) (*connect.Response[xylona.UpdateGameServerResponse], error) {
	selectedTarget := strings.TrimSpace(request.Msg.GetTarget())
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.settings")
	if errPermission != nil {
		return nil, errPermission
	}

	errUpdate := xs.actionsInst.UpdateGameServerWithBackup(gameServer, selectedTarget, xs.updateBroadcast)
	if errors.Is(errUpdate, actions.ErrGameServerOperationInProgress) {
		return nil, connect.NewError(connect.CodeAlreadyExists, errUpdate)
	}
	if errUpdate != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("start game server update: %w", errUpdate))
	}
	return connect.NewResponse(&xylona.UpdateGameServerResponse{}), nil
}

// ListGameServers lists all game servers visible to the caller.
func (xs *XylonaService) ListGameServers(ctx context.Context, request *connect.Request[xylona.ListGameServersRequest]) (*connect.Response[xylona.ListGameServersResponse], error) {
	user, err := xs.getUserFromHeader(request.Header())
	if err != nil {
		return nil, err
	}
	var gameServers []*models.GameServer
	var errGetGameServers error
	if user.SuperUser {
		gameServers, errGetGameServers = xs.db.GetAllGameServers()
	} else {
		gameServers, errGetGameServers = xs.db.GetGameServersAccessibleByUser(user.ID)
	}
	if errGetGameServers != nil {
		if errors.Is(errGetGameServers, sql.ErrNoRows) {
			return connect.NewResponse(&xylona.ListGameServersResponse{
				GameServers: []*xylona.GameServer{},
			}), nil
		}
		return nil, internalErr()
	}

	bulkPerms := map[string][]string{}
	if !user.SuperUser {
		var grantedServerIDs []string
		for _, gs := range gameServers {
			if gs.UserID != user.ID {
				grantedServerIDs = append(grantedServerIDs, gs.ID)
			}
		}
		if len(grantedServerIDs) > 0 {
			var errBulkPerms error
			bulkPerms, errBulkPerms = xs.db.GetUserPermissionIDsForServers(user.ID, grantedServerIDs)
			if errBulkPerms != nil {
				log.Error().Err(errBulkPerms).Msg("Failed to get bulk permissions")
				bulkPerms = map[string][]string{}
			}
		}
	}

	nodeStates := xs.collectGameServerNodeSnapshots(ctx, gameServers)
	if ctx.Err() != nil {
		return nil, connect.NewError(contextConnectCode(ctx.Err()), fmt.Errorf("list game servers: %w", ctx.Err()))
	}

	gameServersProto := make([]*xylona.GameServer, len(gameServers))
	for i, gameServer := range gameServers {
		nodeState := xs.gameServerNodeSnapshotState(gameServer, nodeStates)
		status, snap, errSnapshot := processStateFromNodeSnapshot(gameServer.ID, nodeState)
		if errSnapshot != nil {
			log.Debug().Err(errSnapshot).Str("game_server_id", gameServer.ID).
				Msg("ListGameServers: node snapshot unavailable; using unknown status")
		}
		gameServerProto := protomap.GameServerModelToProto(gameServer, xs.versionState)
		gameServerProto.Status = status
		applyProcessMetricsToProto(gameServerProto, snap)
		if user.SuperUser || gameServer.UserID == user.ID {
			gameServerProto.EffectivePermissions = xs.allPermissionIDs
		} else {
			perms, ok := bulkPerms[gameServer.ID]
			if ok {
				gameServerProto.EffectivePermissions = perms
			}
		}
		if !user.SuperUser {
			redactGameServerForNonSuperuser(gameServerProto)
		}
		gameServersProto[i] = gameServerProto
	}
	response := &xylona.ListGameServersResponse{
		GameServers: gameServersProto,
	}
	return connect.NewResponse(response), nil
}

// QueryGameServer performs a live query against a game server managed by the controller's embedded node.
func (xs *XylonaService) QueryGameServer(_ context.Context, request *connect.Request[xylona.QueryGameServerRequest]) (*connect.Response[xylona.QueryGameServerResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.view")
	if errPermission != nil {
		return nil, errPermission
	}

	allServerQueries := xs.actionsInst.GetServerQueries()
	queryInfo, exists := allServerQueries.GetServers()[gameServer.ID]
	if !exists {
		var queryType xylona.ServerQuery_Type
		switch gameServer.GameID {
		case "minecraft":
			queryType = xylona.ServerQuery_Minecraft
		case "palworld":
			queryType = xylona.ServerQuery_Palworld
		default:
			queryType = xylona.ServerQuery_Source
		}
		resp := &xylona.QueryGameServerResponse{QueryInfo: &xylona.ServerQuery{
			ServerId:   gameServer.ID,
			ServerName: gameServer.Name,
			Type:       queryType,
			Minecraft:  &xylona.MinecraftQueryInfo{NumberOfPlayers: 0, MaxPlayers: helpers.ClampUint32FromInt64(gameServer.MaxPlayers)},
			Source: &xylona.SourceQueryInfo{
				Players:    0,
				MaxPlayers: helpers.ClampUint32FromInt64(gameServer.MaxPlayers),
			},
			Palworld: &xylona.PalworldQueryInfo{
				Players:    0,
				MaxPlayers: helpers.ClampUint32FromInt64(gameServer.MaxPlayers),
			},
		}}
		return connect.NewResponse(resp), nil
	}
	return connect.NewResponse(&xylona.QueryGameServerResponse{QueryInfo: queryInfo}), nil
}
