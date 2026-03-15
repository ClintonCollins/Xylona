package rpc

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
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

func fallbackNodeID(requestNodeID string, defaultNodeID string) string {
	trimmedNodeID := strings.TrimSpace(requestNodeID)
	if trimmedNodeID == "" {
		return defaultNodeID
	}
	return trimmedNodeID
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
	if strings.TrimSpace(newGameServerModel.NodeID) == "" {
		localSettings, errSettings := xs.db.GetLocalSettings()
		if errSettings != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to resolve local node"))
		}
		newGameServerModel.NodeID = fallbackNodeID(newGameServerModel.NodeID, localSettings.NodeID)
	}

	_, errNode := xs.db.GetNodeByID(newGameServerModel.NodeID)
	if errNode != nil {
		if errors.Is(errNode, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid node"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to validate node"))
	}

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
	serverID := request.Msg.GetServerId()
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(existingGameServer *models.GameServer) (*connect.Response[xylona.EditGameServerResponse], error) {
			gameServerModel := helpers.GameServerProtoToModel(request.Msg.GetGameServer())
			gameServerModel.NodeID = fallbackNodeID(gameServerModel.NodeID, existingGameServer.NodeID)

			_, errNode := xs.db.GetNodeByID(gameServerModel.NodeID)
			if errNode != nil {
				if errors.Is(errNode, sql.ErrNoRows) {
					return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid node"))
				}
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to validate node"))
			}

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
		},
		func() (*connect.Response[xylona.EditGameServerResponse], error) {
			return xs.editRemoteGameServer(ctx, serverID, request.Msg.GetGameServer())
		},
	)
}

func (xs XylonaService) RemoveGameServer(ctx context.Context, request *connect.Request[xylona.RemoveGameServerRequest]) (*connect.Response[xylona.RemoveGameServerResponse], error) {
	serverID := request.Msg.GetServerId()
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.RemoveGameServerResponse], error) {
			xs.actionsInst.StopGameServer(gameServer)
			errRemove := xs.actionsInst.RemoveGameServer(gameServer, true)
			if errRemove != nil {
				return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
			}
			return connect.NewResponse(&xylona.RemoveGameServerResponse{}), nil
		},
		func() (*connect.Response[xylona.RemoveGameServerResponse], error) {
			return xs.removeRemoteGameServer(ctx, serverID)
		},
	)
}

func (xs XylonaService) StartGameServer(ctx context.Context, request *connect.Request[xylona.StartGameServerRequest]) (*connect.Response[xylona.StartGameServerResponse], error) {
	serverID := request.Msg.GetServerId()
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.StartGameServerResponse], error) {
			xs.actionsInst.StartGameServer(gameServer)
			return connect.NewResponse(&xylona.StartGameServerResponse{}), nil
		},
		func() (*connect.Response[xylona.StartGameServerResponse], error) {
			return xs.startRemoteGameServer(ctx, serverID)
		},
	)
}

func (xs XylonaService) startRemoteGameServer(ctx context.Context, serverID string) (*connect.Response[xylona.StartGameServerResponse], error) {
	peerNode, _, errGetPeer := xs.getRemoteNodeForServer(serverID)
	if errGetPeer != nil {
		return nil, errGetPeer
	}

	client, secretKey := newRemoteFederationClient(peerNode)

	req := connect.NewRequest(&xylona.FederationRemoteActionRequest{
		ServerId: serverID,
	})
	req.Header().Set("X-Federation-Key", secretKey)

	resp, errStart := client.StartRemoteServer(ctx, req)
	if errStart != nil {
		log.Error().Err(errStart).Str("server_id", serverID).Str("peer", peerNode.Name).Msg("Failed to start remote game server")
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to start remote server"))
	}

	if !resp.Msg.Success {
		return nil, connect.NewError(connect.CodeInternal, errors.New(resp.Msg.Error))
	}

	return connect.NewResponse(&xylona.StartGameServerResponse{}), nil
}

func (xs XylonaService) StopGameServer(ctx context.Context, request *connect.Request[xylona.StopGameServerRequest]) (*connect.Response[xylona.StopGameServerResponse], error) {
	serverID := request.Msg.GetServerId()
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.StopGameServerResponse], error) {
			xs.actionsInst.StopGameServer(gameServer)
			return connect.NewResponse(&xylona.StopGameServerResponse{}), nil
		},
		func() (*connect.Response[xylona.StopGameServerResponse], error) {
			return xs.stopRemoteGameServer(ctx, serverID)
		},
	)
}

func (xs XylonaService) stopRemoteGameServer(ctx context.Context, serverID string) (*connect.Response[xylona.StopGameServerResponse], error) {
	peerNode, _, errGetPeer := xs.getRemoteNodeForServer(serverID)
	if errGetPeer != nil {
		return nil, errGetPeer
	}

	client, secretKey := newRemoteFederationClient(peerNode)

	req := connect.NewRequest(&xylona.FederationRemoteActionRequest{
		ServerId: serverID,
	})
	req.Header().Set("X-Federation-Key", secretKey)

	resp, errStop := client.StopRemoteServer(ctx, req)
	if errStop != nil {
		log.Error().Err(errStop).Str("server_id", serverID).Str("peer", peerNode.Name).Msg("Failed to stop remote game server")
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to stop remote server"))
	}

	if !resp.Msg.Success {
		return nil, connect.NewError(connect.CodeInternal, errors.New(resp.Msg.Error))
	}

	return connect.NewResponse(&xylona.StopGameServerResponse{}), nil
}

func (xs XylonaService) ReadGameServerOutput(ctx context.Context, request *connect.Request[xylona.ReadGameServerOutputRequest]) (*connect.Response[xylona.ReadGameServerOutputResponse], error) {
	serverID := request.Msg.GetServerId()
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.ReadGameServerOutputResponse], error) {
			output := xs.actionsInst.ReadGameServerBuffer(gameServer)
			response := &xylona.ReadGameServerOutputResponse{
				Output: output,
			}
			return connect.NewResponse(response), nil
		},
		func() (*connect.Response[xylona.ReadGameServerOutputResponse], error) {
			return xs.readRemoteGameServerOutput(ctx, serverID)
		},
	)
}

func (xs XylonaService) readRemoteGameServerOutput(ctx context.Context, serverID string) (*connect.Response[xylona.ReadGameServerOutputResponse], error) {
	peerNode, _, errGetPeer := xs.getRemoteNodeForServer(serverID)
	if errGetPeer != nil {
		return nil, errGetPeer
	}

	client, secretKey := newRemoteFederationClient(peerNode)

	req := connect.NewRequest(&xylona.FederationReadConsoleBufferRequest{
		ServerId: serverID,
	})
	req.Header().Set("X-Federation-Key", secretKey)

	resp, errRead := client.ReadConsoleBuffer(ctx, req)
	if errRead != nil {
		log.Error().Err(errRead).Str("server_id", serverID).Str("peer", peerNode.Name).Msg("Failed to read remote console buffer")
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to read remote console"))
	}

	return connect.NewResponse(&xylona.ReadGameServerOutputResponse{
		Output: resp.Msg.Output,
	}), nil
}

func (xs XylonaService) SendGameServerInput(ctx context.Context, request *connect.Request[xylona.SendGameServerInputRequest]) (*connect.Response[xylona.SendGameServerInputResponse], error) {
	serverID := request.Msg.GetServerId()
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.SendGameServerInputResponse], error) {
			gameServerCmd, errGetCommand := xs.supervisorInst.GetCommandByID(gameServer.ID)
			if errGetCommand != nil {
				log.Error().Err(errGetCommand).Msg("Failed to get game server command")
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
			return connect.NewResponse(&xylona.SendGameServerInputResponse{}), nil
		},
		func() (*connect.Response[xylona.SendGameServerInputResponse], error) {
			return xs.sendRemoteGameServerInput(ctx, serverID, request.Msg.GetInput())
		},
	)
}

func (xs XylonaService) sendRemoteGameServerInput(ctx context.Context, serverID string, input string) (*connect.Response[xylona.SendGameServerInputResponse], error) {
	peerNode, _, errGetPeer := xs.getRemoteNodeForServer(serverID)
	if errGetPeer != nil {
		return nil, errGetPeer
	}

	client, secretKey := newRemoteFederationClient(peerNode)

	req := connect.NewRequest(&xylona.FederationSendConsoleInputRequest{
		ServerId: serverID,
		Input:    input,
	})
	req.Header().Set("X-Federation-Key", secretKey)

	resp, errSend := client.SendConsoleInput(ctx, req)
	if errSend != nil {
		log.Error().Err(errSend).Str("server_id", serverID).Str("peer", peerNode.Name).Msg("Failed to send remote console input")
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to send remote input"))
	}

	if !resp.Msg.Success {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New(resp.Msg.Error))
	}

	return connect.NewResponse(&xylona.SendGameServerInputResponse{}), nil
}

func (xs XylonaService) ListDirectoryFiles(ctx context.Context, request *connect.Request[xylona.ListDirectoryFilesRequest]) (*connect.Response[xylona.ListDirectoryFilesResponse], error) {
	serverID := request.Msg.GetGameServerId()
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.ListDirectoryFilesResponse], error) {
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
		},
		func() (*connect.Response[xylona.ListDirectoryFilesResponse], error) {
			return xs.listRemoteDirectoryFiles(ctx, serverID, request.Msg.GetPath())
		},
	)
}

func (xs XylonaService) GetGameServer(ctx context.Context, request *connect.Request[xylona.GetGameServerRequest]) (*connect.Response[xylona.GetGameServerResponse], error) {
	serverID := request.Msg.GetId()
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.GetGameServerResponse], error) {
			gameServerCmd, errGetCommand := xs.supervisorInst.GetCommandByID(gameServer.ID)
			if errGetCommand != nil {
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
		},
		func() (*connect.Response[xylona.GetGameServerResponse], error) {
			return xs.getRemoteGameServer(ctx, serverID)
		},
	)
}

func (xs XylonaService) getRemoteGameServer(ctx context.Context, serverID string) (*connect.Response[xylona.GetGameServerResponse], error) {
	peerNode, remoteCache, errGetPeer := xs.getRemoteNodeForServer(serverID)
	if errGetPeer != nil {
		return nil, errGetPeer
	}

	// Try to fetch live detail from the peer.
	client, secretKey := newRemoteFederationClient(peerNode)

	req := connect.NewRequest(&xylona.FederationGetServerDetailRequest{
		ServerId: serverID,
	})
	req.Header().Set("X-Federation-Key", secretKey)

	resp, errDetail := client.GetServerDetail(ctx, req)
	if errDetail != nil {
		// Fall back to cached data.
		log.Debug().Err(errDetail).Str("server_id", serverID).Msg("Failed to get remote server detail, using cache")
		return connect.NewResponse(&xylona.GetGameServerResponse{
			GameServer: helpers.RemoteServerCacheToProto(remoteCache, peerNode),
		}), nil
	}

	server := resp.Msg.Server
	gs := &xylona.GameServer{
		Id:                 server.ServerId,
		Name:               server.DisplayName,
		GameId:             server.GameId,
		Status:             server.Status,
		Ip:                 &xylona.IP{Address: server.IpAddress},
		Port:               int64(server.Port),
		QueryPort:          int64(server.QueryPort),
		SetMaxPlayers:      int64(server.MaxPlayers),
		MaxPlayers:         int64(server.MaxPlayers),
		CurrentPlayerCount: int64(server.CurrentPlayers),
		Map:                server.MapName,
		Version:            server.Version,
		GameName:           server.GameName,
		NodeId:             peerNode.ID,
		NodeName:           peerNode.Name,
		NodeHost:           peerNode.BaseURL,
	}

	return connect.NewResponse(&xylona.GetGameServerResponse{
		GameServer: gs,
	}), nil
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
	serverID := request.Msg.GetServerId()
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.UpdateGameServerResponse], error) {
			xs.actionsInst.UpdateGameServer(gameServer)
			return connect.NewResponse(&xylona.UpdateGameServerResponse{}), nil
		},
		func() (*connect.Response[xylona.UpdateGameServerResponse], error) {
			return xs.updateRemoteGameServer(ctx, serverID)
		},
	)
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
	serverID := request.Msg.GetServerId()
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.QueryGameServerResponse], error) {
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
					Source: &xylona.SourceQueryInfo{
						Players:    0,
						MaxPlayers: uint32(gameServer.MaxPlayers),
					},
				}}
				return connect.NewResponse(resp), nil
			}
			return connect.NewResponse(&xylona.QueryGameServerResponse{QueryInfo: queryInfo}), nil
		},
		func() (*connect.Response[xylona.QueryGameServerResponse], error) {
			return xs.queryRemoteGameServer(ctx, serverID)
		},
	)
}
