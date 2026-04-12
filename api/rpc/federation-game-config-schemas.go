package rpc

import (
	"context"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// ListRemoteGameServerConfigFiles returns schema-backed config file summaries for a local server over federation.
func (fs FederationService) ListRemoteGameServerConfigFiles(
	ctx context.Context,
	request *connect.Request[xylona.FederationGetGameServerConfigFilesRequest],
) (*connect.Response[xylona.FederationGetGameServerConfigFilesResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	requestMsg := request.Msg.GetRequest()
	if requestMsg == nil {
		requestMsg = &xylona.GetGameServerConfigFilesRequest{}
	}

	serverID := requestMsg.GetGameServerId()
	errPermission := fs.authorizeFederatedPermission(
		ctx,
		request.Header(),
		request.Msg.GetActingUserId(),
		request.Msg.GetOriginNodeId(),
		serverID,
		"game_server.config",
	)
	if errPermission != nil {
		return nil, errPermission
	}

	gameServer, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, dbLookupMsg(errGet, "failed to get game server")
	}

	response, errList := getGameServerConfigFilesLocal(fs.db, gameServer)
	if errList != nil {
		return nil, errList
	}

	return connect.NewResponse(&xylona.FederationGetGameServerConfigFilesResponse{
		Response: response.Msg,
	}), nil
}

// GetRemoteGameServerConfigFile reads a schema-backed config file for a local server over federation.
func (fs FederationService) GetRemoteGameServerConfigFile(
	ctx context.Context,
	request *connect.Request[xylona.FederationGetGameServerConfigFileRequest],
) (*connect.Response[xylona.FederationGetGameServerConfigFileResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	requestMsg := request.Msg.GetRequest()
	if requestMsg == nil {
		requestMsg = &xylona.GetGameServerConfigFileRequest{}
	}

	serverID := requestMsg.GetGameServerId()
	errPermission := fs.authorizeFederatedPermission(
		ctx,
		request.Header(),
		request.Msg.GetActingUserId(),
		request.Msg.GetOriginNodeId(),
		serverID,
		"game_server.config",
	)
	if errPermission != nil {
		return nil, errPermission
	}

	gameServer, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, dbLookupMsg(errGet, "failed to get game server")
	}

	response, errGetFile := getGameServerConfigFileLocal(fs.db, gameServer, requestMsg.GetFilePath())
	if errGetFile != nil {
		return nil, errGetFile
	}

	return connect.NewResponse(&xylona.FederationGetGameServerConfigFileResponse{
		Response: response.Msg,
	}), nil
}

// UpdateRemoteGameServerConfigFile validates and writes a schema-backed config file over federation.
func (fs FederationService) UpdateRemoteGameServerConfigFile(
	ctx context.Context,
	request *connect.Request[xylona.FederationUpdateGameServerConfigFileRequest],
) (*connect.Response[xylona.FederationUpdateGameServerConfigFileResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	requestMsg := request.Msg.GetRequest()
	if requestMsg == nil {
		requestMsg = &xylona.UpdateGameServerConfigFileRequest{}
	}

	serverID := requestMsg.GetGameServerId()
	errPermission := fs.authorizeFederatedPermission(
		ctx,
		request.Header(),
		request.Msg.GetActingUserId(),
		request.Msg.GetOriginNodeId(),
		serverID,
		"game_server.config",
	)
	if errPermission != nil {
		return nil, errPermission
	}

	gameServer, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, dbLookupMsg(errGet, "failed to get game server")
	}

	response, errUpdate := updateGameServerConfigFileLocal(fs.db, gameServer, requestMsg)
	if errUpdate != nil {
		return nil, errUpdate
	}

	return connect.NewResponse(&xylona.FederationUpdateGameServerConfigFileResponse{
		Response: response.Msg,
	}), nil
}

// GenerateRemoteGameServerConfigFile generates a schema-backed config file over federation.
func (fs FederationService) GenerateRemoteGameServerConfigFile(
	ctx context.Context,
	request *connect.Request[xylona.FederationGenerateGameServerConfigFileRequest],
) (*connect.Response[xylona.FederationGenerateGameServerConfigFileResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	requestMsg := request.Msg.GetRequest()
	if requestMsg == nil {
		requestMsg = &xylona.GenerateGameServerConfigFileRequest{}
	}

	serverID := requestMsg.GetGameServerId()
	errPermission := fs.authorizeFederatedPermission(
		ctx,
		request.Header(),
		request.Msg.GetActingUserId(),
		request.Msg.GetOriginNodeId(),
		serverID,
		"game_server.config",
	)
	if errPermission != nil {
		return nil, errPermission
	}

	gameServer, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, dbLookupMsg(errGet, "failed to get game server")
	}

	response, errGenerate := generateGameServerConfigFileLocal(fs.db, gameServer, requestMsg.GetFilePath())
	if errGenerate != nil {
		return nil, errGenerate
	}

	return connect.NewResponse(&xylona.FederationGenerateGameServerConfigFileResponse{
		Response: response.Msg,
	}), nil
}
