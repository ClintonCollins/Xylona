package rpc

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"time"

	connect_go "github.com/bufbuild/connect-go"
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

func (xs XylonaService) CreateGameServer(ctx context.Context, request *connect_go.Request[xylona.CreateGameServerRequest]) (*connect_go.Response[xylona.CreateGameServerResponse], error) {

	// log.Debug().Msgf("CreateGameServer request: %+v", request.Msg.GetGameServer())
	user, errGetUser := xs.db.GetUserByID(request.Msg.GetGameServer().UserId)
	if errGetUser != nil {
		if errors.Is(errGetUser, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	game, errGetGame := xs.db.GetGameByID(request.Msg.GetGameServer().GameId)
	if errGetGame != nil {
		if errors.Is(errGetGame, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}

	newGameServerModel := helpers.GameServerProtoToModel(request.Msg.GetGameServer())

	newGameServer, errInstallGameServer := xs.actionsInst.InstallGameServer(game, newGameServerModel, user)
	if errInstallGameServer != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}

	response := &xylona.CreateGameServerResponse{
		GameServer: helpers.GameServerModelToProto(newGameServer),
	}
	return connect_go.NewResponse(response), nil
}

func (xs XylonaService) EditGameServer(ctx context.Context, request *connect_go.Request[xylona.EditGameServerRequest]) (*connect_go.Response[xylona.EditGameServerResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) RemoveGameServer(ctx context.Context, request *connect_go.Request[xylona.RemoveGameServerRequest]) (*connect_go.Response[xylona.RemoveGameServerResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	xs.actionsInst.StopGameServer(gameServer)
	errRemove := xs.actionsInst.RemoveGameServer(gameServer, true)
	if errRemove != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	response := &xylona.RemoveGameServerResponse{}
	return connect_go.NewResponse(response), nil
}

func (xs XylonaService) StartGameServer(ctx context.Context, request *connect_go.Request[xylona.StartGameServerRequest]) (*connect_go.Response[xylona.StartGameServerResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	xs.actionsInst.StartGameServer(gameServer)
	response := &xylona.StartGameServerResponse{}
	return connect_go.NewResponse(response), nil
}

func (xs XylonaService) StopGameServer(ctx context.Context, request *connect_go.Request[xylona.StopGameServerRequest]) (*connect_go.Response[xylona.StopGameServerResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	xs.actionsInst.StopGameServer(gameServer)
	response := &xylona.StopGameServerResponse{}
	return connect_go.NewResponse(response), nil
}

func (xs XylonaService) ReadGameServerOutput(ctx context.Context, request *connect_go.Request[xylona.ReadGameServerOutputRequest]) (*connect_go.Response[xylona.ReadGameServerOutputResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	output := xs.actionsInst.ReadGameServerBuffer(gameServer)
	response := &xylona.ReadGameServerOutputResponse{
		Output: output,
	}
	return connect_go.NewResponse(response), nil
}

func (xs XylonaService) SendGameServerInput(ctx context.Context, request *connect_go.Request[xylona.SendGameServerInputRequest]) (*connect_go.Response[xylona.SendGameServerInputResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	gameServerCmd, err := xs.supervisorInst.GetCommandByID(gameServer.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get game server command")
		return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("game server not running"))
	}
	status := gameServerCmd.Status()
	if status == xylona.Status_OFFLINE || status == xylona.Status_UNKNOWN {
		return connect_go.NewResponse(&xylona.SendGameServerInputResponse{}), nil
	}
	errSend := gameServerCmd.SendInput(request.Msg.GetInput())
	if errSend != nil {
		log.Error().Err(errSend).Msg("Failed to send input to game server")
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	response := &xylona.SendGameServerInputResponse{}
	return connect_go.NewResponse(response), nil
}

func (xs XylonaService) ListDirectoryFiles(ctx context.Context, request *connect_go.Request[xylona.ListDirectoryFilesRequest]) (*connect_go.Response[xylona.ListDirectoryFilesResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetGameServerId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	files, errListGameServerFiles := xs.actionsInst.ListGameServerFiles(gameServer, request.Msg.GetPath())
	if errListGameServerFiles != nil {
		if errors.Is(errListGameServerFiles, actions.ErrInvalidPath) {
			return nil, connect_go.NewError(connect_go.CodeInvalidArgument, errors.New("invalid path"))
		}
		if errors.Is(errListGameServerFiles, os.ErrNotExist) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("invalid path"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	response := &xylona.ListDirectoryFilesResponse{
		Files: files,
	}

	return connect_go.NewResponse(response), nil
}

func (xs XylonaService) GetGameServer(ctx context.Context, request *connect_go.Request[xylona.GetGameServerRequest]) (*connect_go.Response[xylona.GetGameServerResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GetId())
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}
	gameServerCmd, err := xs.supervisorInst.GetCommandByID(gameServer.ID)
	if err != nil {
		gameServer.Status = xylona.Status_OFFLINE.String()
	} else {
		gameServer.Status = gameServerCmd.Status().String()
	}
	log.Debug().Msgf("Game server status: %s", gameServer.Status)
	response := &xylona.GetGameServerResponse{
		GameServer: helpers.GameServerModelToProto(gameServer),
	}
	if gameServer.GameID == "minecraft" {
		version, errGetMinecraftVersion := xs.getMinecraftVersion(gameServer)
		if errGetMinecraftVersion == nil {
			response.GameServer.Version = version
		}
	}
	return connect_go.NewResponse(response), nil
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

func (xs XylonaService) UpdateGameServer(ctx context.Context, request *connect_go.Request[xylona.UpdateGameServerRequest]) (*connect_go.Response[xylona.UpdateGameServerResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) ListGameServers(ctx context.Context, request *connect_go.Request[xylona.ListGameServersRequest]) (*connect_go.Response[xylona.ListGameServersResponse], error) {
	user, err := xs.getUserFromHeader(request.Header())
	if err != nil {
		return nil, err
	}
	if !user.SuperUser {
		gameServers, errGetGameServers := xs.db.GetGameServersByUser(user.ID)
		if errGetGameServers != nil {
			if errors.Is(errGetGameServers, sql.ErrNoRows) {
				return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
			}
			return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
		}
		gameServersProto := make([]*xylona.GameServer, len(gameServers))
		for i, gameServer := range gameServers {
			gameServerProto := helpers.GameServerModelToProto(gameServer)
			gameServersProto[i] = gameServerProto
		}
		response := &xylona.ListGameServersResponse{
			GameServers: gameServersProto,
		}
		return connect_go.NewResponse(response), nil
	}
	gameServers, errGetGameServers := xs.db.GetAllGameServers()
	if errGetGameServers != nil {
		if errors.Is(errGetGameServers, sql.ErrNoRows) {
			return nil, connect_go.NewError(connect_go.CodeNotFound, errors.New("not found"))
		}
		return nil, connect_go.NewError(connect_go.CodeInternal, errors.New("internal error"))
	}

	gameServersProto := make([]*xylona.GameServer, len(gameServers))
	for i, gameServer := range gameServers {
		gameServerProto := helpers.GameServerModelToProto(gameServer)
		gameServersProto[i] = gameServerProto
	}
	response := &xylona.ListGameServersResponse{
		GameServers: gameServersProto,
	}
	return connect_go.NewResponse(response), nil
}
