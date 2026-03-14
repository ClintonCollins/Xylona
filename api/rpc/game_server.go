package rpc

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type MinecraftVersionJSON struct {
	Id              string `json:"id"`
	Name            string `json:"name"`
	WorldVersion    int    `json:"world_version"`
	SeriesId        string `json:"series_id"`
	ProtocolVersion int    `json:"protocol_version"`
	PackVersion     struct {
		Resource int `json:"resource"`
		Data     int `json:"data"`
	} `json:"pack_version"`
	BuildTime     time.Time `json:"build_time"`
	JavaComponent string    `json:"java_component"`
	JavaVersion   int       `json:"java_version"`
	Stable        bool      `json:"stable"`
	UseEditor     bool      `json:"use_editor"`
}

// findAvailablePort checks for port conflicts on the given IP and returns the next available port.
// If the game requires a dedicated IP, no other game server should use the same IP at all.
// excludeServerID can be set to skip a specific server (useful when editing an existing server).
func (xs XylonaService) findAvailablePort(ip string, port int64, queryPort int64, game *models.Game, excludeServerID string) (int64, int64, error) {
	existingServers, errGetServers := xs.db.GetGameServersByIP(ip)
	if errGetServers != nil {
		return 0, 0, errGetServers
	}

	// If the game requires a dedicated IP, no other server should use this IP.
	if game.RequireDedicatedIP {
		for _, s := range existingServers {
			if s.ID != excludeServerID {
				return 0, 0, fmt.Errorf("game %s requires a dedicated IP, but IP %s is already in use by server %s", game.Name, ip, s.Name)
			}
		}
	}

	// Check if any existing server on this IP requires a dedicated IP (and thus blocks all ports).
	for _, s := range existingServers {
		if s.ID == excludeServerID {
			continue
		}
		if s.R.Game != nil && s.R.Game.RequireDedicatedIP {
			return 0, 0, fmt.Errorf("IP %s is dedicated to server %s and cannot be shared", ip, s.Name)
		}
	}

	// Collect all used ports on this IP (excluding the server being edited).
	usedPorts := make(map[int64]bool)
	for _, s := range existingServers {
		if s.ID == excludeServerID {
			continue
		}
		usedPorts[s.Port] = true
		usedPorts[s.QueryPort] = true
	}

	// Auto-increment port if it conflicts.
	for usedPorts[port] || usedPorts[queryPort] || (port == queryPort && port != 0) {
		port++
		queryPort++
		if port > 65535 || queryPort > 65535 {
			return 0, 0, fmt.Errorf("no available ports on IP %s", ip)
		}
	}

	return port, queryPort, nil
}

func (xs XylonaService) CreateGameServer(ctx context.Context, request *connect.Request[xylona.CreateGameServerRequest]) (*connect.Response[xylona.CreateGameServerResponse], error) {

	// log.Debug().Msgf("CreateGameServer request: %+v", request.Msg.GetGameServer())
	user, errGetUser := xs.db.GetUserByID(request.Msg.GetGameServer().UserId)
	if errGetUser != nil {
		if errors.Is(errGetUser, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	game, errGetGame := xs.db.GetGameByID(request.Msg.GetGameServer().GameId)
	if errGetGame != nil {
		if errors.Is(errGetGame, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	newGameServerModel := helpers.GameServerProtoToModel(request.Msg.GetGameServer())

	// Check for port conflicts and auto-increment if necessary.
	availablePort, availableQueryPort, errPortCheck := xs.findAvailablePort(
		newGameServerModel.IP, newGameServerModel.Port, newGameServerModel.QueryPort, game, "",
	)
	if errPortCheck != nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, errPortCheck)
	}
	newGameServerModel.Port = availablePort
	newGameServerModel.QueryPort = availableQueryPort

	newGameServer, errInstallGameServer := xs.actionsInst.InstallGameServer(game, newGameServerModel, user)
	if errInstallGameServer != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	response := &xylona.CreateGameServerResponse{
		GameServer: helpers.GameServerModelToProto(newGameServer),
	}
	return connect.NewResponse(response), nil
}

func (xs XylonaService) EditGameServer(ctx context.Context, request *connect.Request[xylona.EditGameServerRequest]) (*connect.Response[xylona.EditGameServerResponse], error) {
	existingGameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	gameServerModel := helpers.GameServerProtoToModel(request.Msg.GetGameServer())

	// Check for port conflicts when IP or port changed.
	game, errGetGame := xs.db.GetGameByID(gameServerModel.GameID)
	if errGetGame != nil {
		if errors.Is(errGetGame, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	if gameServerModel.IP != existingGameServer.IP || gameServerModel.Port != existingGameServer.Port || gameServerModel.QueryPort != existingGameServer.QueryPort {
		availablePort, availableQueryPort, errPortCheck := xs.findAvailablePort(
			gameServerModel.IP, gameServerModel.Port, gameServerModel.QueryPort, game, existingGameServer.ID,
		)
		if errPortCheck != nil {
			return nil, connect.NewError(connect.CodeAlreadyExists, errPortCheck)
		}
		gameServerModel.Port = availablePort
		gameServerModel.QueryPort = availableQueryPort
	}

	setter := helpers.GameServerModelToSetter(gameServerModel)
	_, errUpdate := xs.db.UpdateGameServer(xs.db.DB, setter)
	if errUpdate != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	response := &xylona.EditGameServerResponse{Game_Server: helpers.GameServerModelToProto(gameServerModel)}
	return connect.NewResponse(response), nil
}

func (xs XylonaService) RemoveGameServer(ctx context.Context, request *connect.Request[xylona.RemoveGameServerRequest]) (*connect.Response[xylona.RemoveGameServerResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	xs.actionsInst.StopGameServer(gameServer)
	errRemove := xs.actionsInst.RemoveGameServer(gameServer, true)
	if errRemove != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	response := &xylona.RemoveGameServerResponse{}
	return connect.NewResponse(response), nil
}

func (xs XylonaService) StartGameServer(ctx context.Context, request *connect.Request[xylona.StartGameServerRequest]) (*connect.Response[xylona.StartGameServerResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	xs.actionsInst.StartGameServer(gameServer)
	response := &xylona.StartGameServerResponse{}
	return connect.NewResponse(response), nil
}

func (xs XylonaService) StopGameServer(ctx context.Context, request *connect.Request[xylona.StopGameServerRequest]) (*connect.Response[xylona.StopGameServerResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	xs.actionsInst.StopGameServer(gameServer)
	response := &xylona.StopGameServerResponse{}
	return connect.NewResponse(response), nil
}

func (xs XylonaService) ReadGameServerOutput(ctx context.Context, request *connect.Request[xylona.ReadGameServerOutputRequest]) (*connect.Response[xylona.ReadGameServerOutputResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	output := xs.actionsInst.ReadGameServerBuffer(gameServer)
	response := &xylona.ReadGameServerOutputResponse{
		Output: output,
	}
	return connect.NewResponse(response), nil
}

func (xs XylonaService) SendGameServerInput(ctx context.Context, request *connect.Request[xylona.SendGameServerInputRequest]) (*connect.Response[xylona.SendGameServerInputResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	gameServerCmd, err := xs.supervisorInst.GetCommandByID(gameServer.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get game server command")
		return nil, connect.NewError(connect.CodeNotFound, errors.New("game server not running"))
	}
	status := gameServerCmd.Status()
	if status == xylona.Status_OFFLINE || status == xylona.Status_UNKNOWN {
		return connect.NewResponse(&xylona.SendGameServerInputResponse{}), nil
	}
	errSend := gameServerCmd.SendInput(request.Msg.GetInput())
	if errSend != nil {
		log.Error().Err(errSend).Msg("Failed to send input to game server")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	response := &xylona.SendGameServerInputResponse{}
	return connect.NewResponse(response), nil
}

func (xs XylonaService) ListDirectoryFiles(ctx context.Context, request *connect.Request[xylona.ListDirectoryFilesRequest]) (*connect.Response[xylona.ListDirectoryFilesResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetGameServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	files, errListGameServerFiles := xs.actionsInst.ListGameServerFiles(gameServer, request.Msg.GetPath())
	if errListGameServerFiles != nil {
		if errors.Is(errListGameServerFiles, actions.ErrInvalidPath) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid path"))
		}
		if errors.Is(errListGameServerFiles, os.ErrNotExist) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("invalid path"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	response := &xylona.ListDirectoryFilesResponse{
		Files: files,
	}

	return connect.NewResponse(response), nil
}

func (xs XylonaService) GetGameServer(ctx context.Context, request *connect.Request[xylona.GetGameServerRequest]) (*connect.Response[xylona.GetGameServerResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	gameServerCmd, err := xs.supervisorInst.GetCommandByID(gameServer.ID)
	if err != nil {
		gameServer.Status = xylona.Status_OFFLINE.String()
	} else {
		gameServer.Status = gameServerCmd.Status().String()
	}
	response := &xylona.GetGameServerResponse{
		GameServer: helpers.GameServerModelToProto(gameServer),
	}
	if gameServer.GameID == "minecraft" {
		version, errGetMinecraftVersion := xs.getMinecraftVersion(gameServer)
		if errGetMinecraftVersion == nil {
			response.GameServer.Version = version
		}
	}
	return connect.NewResponse(response), nil
}

func (xs XylonaService) getMinecraftVersion(gameServer *models.GameServer) (string, error) {
	dir := gameServer.Directory
	zipReader, errZipReader := zip.OpenReader(dir + "/minecraft_server.jar")
	if errZipReader != nil {
		log.Error().Err(errZipReader).Msg("Failed to open zip reader")
		return "", errZipReader
	}
	defer func() {
		_ = zipReader.Close()
	}()
	var versionJSONFile *zip.File
	for _, f := range zipReader.File {
		if f.Name == "version.json" {
			versionJSONFile = f
			break
		}
	}
	if versionJSONFile == nil {
		log.Error().Msg("Failed to find version.json")
		return "", errors.New("failed to find version.json")
	}
	versionJSONFileReader, errVersionJSONFileReader := versionJSONFile.Open()
	if errVersionJSONFileReader != nil {
		log.Error().Err(errVersionJSONFileReader).Msg("Failed to open version.json")
		return "", errVersionJSONFileReader
	}
	defer func() {
		_ = versionJSONFileReader.Close()
	}()

	minecraftVersionJSON := MinecraftVersionJSON{}
	errUnmarshal := json.NewDecoder(versionJSONFileReader).Decode(&minecraftVersionJSON)
	if errUnmarshal != nil {
		log.Error().Err(errUnmarshal).Msg("Failed to unmarshal version.json")
		return "", errUnmarshal
	}
	return minecraftVersionJSON.Name, nil
}

func (xs XylonaService) UpdateGameServer(ctx context.Context, request *connect.Request[xylona.UpdateGameServerRequest]) (*connect.Response[xylona.UpdateGameServerResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	xs.actionsInst.UpdateGameServer(gameServer)
	response := &xylona.UpdateGameServerResponse{}
	return connect.NewResponse(response), nil
}

func (xs XylonaService) ListGameServers(ctx context.Context, request *connect.Request[xylona.ListGameServersRequest]) (*connect.Response[xylona.ListGameServersResponse], error) {
	user, err := xs.getUserFromHeader(request.Header())
	if err != nil {
		return nil, err
	}
	if !user.SuperUser {
		gameServers, errGetGameServers := xs.db.GetGameServersByUser(user.ID)
		if errGetGameServers != nil {
			if errors.Is(errGetGameServers, sql.ErrNoRows) {
				return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
			}
			return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
		}
		gameServersProto := make([]*xylona.GameServer, len(gameServers))
		for i, gameServer := range gameServers {
			gameServerCmd, errGetCommand := xs.supervisorInst.GetCommandByID(gameServer.ID)
			if errGetCommand != nil {
				gameServer.Status = xylona.Status_OFFLINE.String()
			} else {
				gameServer.Status = gameServerCmd.Status().String()
			}
			gameServerProto := helpers.GameServerModelToProto(gameServer)
			gameServersProto[i] = gameServerProto
		}
		response := &xylona.ListGameServersResponse{
			GameServers: gameServersProto,
		}
		return connect.NewResponse(response), nil
	}
	gameServers, errGetGameServers := xs.db.GetAllGameServers()
	if errGetGameServers != nil {
		if errors.Is(errGetGameServers, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	gameServersProto := make([]*xylona.GameServer, len(gameServers))
	for i, gameServer := range gameServers {
		gameServerCmd, errGetCommand := xs.supervisorInst.GetCommandByID(gameServer.ID)
		if errGetCommand != nil {
			gameServer.Status = xylona.Status_OFFLINE.String()
		} else {
			gameServer.Status = gameServerCmd.Status().String()
		}
		gameServerProto := helpers.GameServerModelToProto(gameServer)
		gameServersProto[i] = gameServerProto
	}
	response := &xylona.ListGameServersResponse{
		GameServers: gameServersProto,
	}
	return connect.NewResponse(response), nil
}

func (xs XylonaService) QueryGameServer(ctx context.Context, request *connect.Request[xylona.QueryGameServerRequest]) (*connect.Response[xylona.QueryGameServerResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	allServerQueries := xs.actionsInst.GetServerQueries()
	queryInfo, exists := allServerQueries.Servers[gameServer.ID]
	if !exists {
		queryType := xylona.ServerQuery_Unknown
		if gameServer.GameID == "minecraft" {
			queryType = xylona.ServerQuery_Minecraft
		} else {
			queryType = xylona.ServerQuery_Source
		}
		resp := &xylona.QueryGameServerResponse{QueryInfo: &xylona.ServerQuery{
			ServerId:   gameServer.ID,
			ServerName: gameServer.Name,
			Type:       queryType,
			Minecraft:  &xylona.MinecraftQueryInfo{NumberOfPlayers: 0, MaxPlayers: uint32(gameServer.MaxPlayers)},
			Source: &xylona.SourceQueryInfo{Players: 0, MaxPlayers: uint32(gameServer.MaxPlayers),
			}},
		}
		return connect.NewResponse(resp), nil
	}
	return connect.NewResponse(&xylona.QueryGameServerResponse{QueryInfo: queryInfo}), nil
}
