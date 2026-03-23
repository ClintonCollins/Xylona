package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (xs *XylonaService) remoteFederationClient(node *models.Node, serverID string) (xylonaconnect.FederationClient, error) {
	if xs.remoteFederationClientFactory != nil {
		return xs.remoteFederationClientFactory(node, serverID)
	}

	client, errClient := xs.newRemoteFederationClient(node)
	if errClient != nil {
		log.Error().
			Err(errClient).
			Str("server_id", serverID).
			Str("node", node.Name).
			Msg("Failed to create remote federation mTLS client")
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("remote federation trust is not configured"))
	}
	return client, nil
}

func (xs *XylonaService) editRemoteGameServer(ctx context.Context, serverID string, gameServer *xylona.GameServer, actingUser *models.User) (*connect.Response[xylona.EditGameServerResponse], error) {
	node, _, errGet := xs.getRemoteNodeForServer(serverID)
	if errGet != nil {
		return nil, errGet
	}

	client, errClient := xs.remoteFederationClient(node, serverID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationEditServerRequest{
		ServerId:   serverID,
		GameServer: gameServer,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to apply federation identity headers")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to edit remote server"))
	}

	resp, errEdit := client.EditRemoteServer(ctx, req)
	if errEdit != nil {
		log.Error().Err(errEdit).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to edit remote game server")
		return nil, wrapRemoteRPCError(errEdit, "failed to edit remote server")
	}

	if !resp.Msg.Success {
		return nil, connect.NewError(connect.CodeInternal, errors.New(resp.Msg.Error))
	}

	return connect.NewResponse(&xylona.EditGameServerResponse{
		Game_Server: resp.Msg.GameServer,
	}), nil
}

func (xs *XylonaService) removeRemoteGameServer(ctx context.Context, serverID string, actingUser *models.User) (*connect.Response[xylona.RemoveGameServerResponse], error) {
	node, remoteCache, errGet := xs.getRemoteNodeForServer(serverID)
	if errGet != nil {
		return nil, errGet
	}

	client, errClient := xs.remoteFederationClient(node, serverID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationRemoteActionRequest{
		ServerId: serverID,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to apply federation identity headers")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to remove remote server"))
	}

	resp, errRemove := client.RemoveRemoteServer(ctx, req)
	if errRemove != nil {
		log.Error().Err(errRemove).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to remove remote game server")
		return nil, wrapRemoteRPCError(errRemove, "failed to remove remote server")
	}

	if !resp.Msg.Success {
		return nil, connect.NewError(connect.CodeInternal, errors.New(resp.Msg.Error))
	}

	// Clean up only the removed server from cache.
	errDeleteCache := xs.db.DeleteRemoteServerCacheByCompositeKey(remoteCache.SourceNodeID, remoteCache.RemoteServerID)
	if errDeleteCache != nil {
		log.Warn().Err(errDeleteCache).Str("server_id", serverID).Msg("Failed to clean up remote server cache after removal")
	}

	return connect.NewResponse(&xylona.RemoveGameServerResponse{}), nil
}

func (xs *XylonaService) updateRemoteGameServer(ctx context.Context, serverID string, actingUser *models.User) (*connect.Response[xylona.UpdateGameServerResponse], error) {
	node, _, errGet := xs.getRemoteNodeForServer(serverID)
	if errGet != nil {
		return nil, errGet
	}

	client, errClient := xs.remoteFederationClient(node, serverID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationRemoteActionRequest{
		ServerId: serverID,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to apply federation identity headers")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update remote server"))
	}

	resp, errUpdate := client.UpdateRemoteServer(ctx, req)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to update remote game server")
		return nil, wrapRemoteRPCError(errUpdate, "failed to update remote server")
	}

	if !resp.Msg.Success {
		return nil, connect.NewError(connect.CodeInternal, errors.New(resp.Msg.Error))
	}

	return connect.NewResponse(&xylona.UpdateGameServerResponse{}), nil
}

func (xs *XylonaService) getRemoteVersionInfo(ctx context.Context, serverID string, actingUser *models.User) (*connect.Response[xylona.GetVersionInfoResponse], error) {
	node, _, errGet := xs.getRemoteNodeForServer(serverID)
	if errGet != nil {
		return nil, errGet
	}

	client, errClient := xs.remoteFederationClient(node, serverID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationRemoteActionRequest{
		ServerId: serverID,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to apply federation identity headers")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get remote version info"))
	}

	resp, errGetVersion := client.GetRemoteVersionInfo(ctx, req)
	if errGetVersion != nil {
		log.Error().Err(errGetVersion).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to get remote version info")
		return nil, wrapRemoteRPCError(errGetVersion, "failed to get remote version info")
	}

	return connect.NewResponse(&xylona.GetVersionInfoResponse{
		VersionInfo: resp.Msg.GetVersionInfo(),
	}), nil
}

func (xs *XylonaService) checkRemoteServerForUpdate(ctx context.Context, serverID string, actingUser *models.User) (*connect.Response[xylona.CheckForUpdateResponse], error) {
	node, _, errGet := xs.getRemoteNodeForServer(serverID)
	if errGet != nil {
		return nil, errGet
	}

	client, errClient := xs.remoteFederationClient(node, serverID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationRemoteActionRequest{
		ServerId: serverID,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to apply federation identity headers")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check remote server for updates"))
	}

	resp, errCheck := client.CheckRemoteServerForUpdate(ctx, req)
	if errCheck != nil {
		log.Error().Err(errCheck).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to check remote server for updates")
		return nil, wrapRemoteRPCError(errCheck, "failed to check remote server for updates")
	}

	return connect.NewResponse(&xylona.CheckForUpdateResponse{
		VersionInfo: resp.Msg.GetVersionInfo(),
	}), nil
}

func (xs *XylonaService) listRemoteDirectoryFiles(ctx context.Context, serverID string, path string, actingUser *models.User) (*connect.Response[xylona.ListDirectoryFilesResponse], error) {
	node, _, errGet := xs.getRemoteNodeForServer(serverID)
	if errGet != nil {
		return nil, errGet
	}

	client, errClient := xs.remoteFederationClient(node, serverID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationListDirectoryFilesRequest{
		ServerId: serverID,
		Path:     path,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to apply federation identity headers")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list remote files"))
	}

	resp, errList := client.ListRemoteDirectoryFiles(ctx, req)
	if errList != nil {
		log.Error().Err(errList).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to list remote directory files")
		return nil, wrapRemoteRPCError(errList, "failed to list remote files")
	}

	return connect.NewResponse(&xylona.ListDirectoryFilesResponse{
		Files: resp.Msg.Files,
	}), nil
}

func (xs *XylonaService) editRemoteFile(ctx context.Context, serverID string, fullFilePath string, content string, actingUser *models.User) (*connect.Response[xylona.GameServersFileEditResponse], error) {
	node, _, errGet := xs.getRemoteNodeForServer(serverID)
	if errGet != nil {
		return nil, errGet
	}

	client, errClient := xs.remoteFederationClient(node, serverID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationEditFileRequest{
		ServerId:     serverID,
		FullFilePath: fullFilePath,
		Content:      content,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to apply federation identity headers")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to edit remote file"))
	}

	_, errEdit := client.EditRemoteFile(ctx, req)
	if errEdit != nil {
		log.Error().Err(errEdit).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to edit remote file")
		return nil, wrapRemoteRPCError(errEdit, "failed to edit remote file")
	}

	return connect.NewResponse(&xylona.GameServersFileEditResponse{}), nil
}

func (xs *XylonaService) deleteRemoteFiles(ctx context.Context, serverID string, fullFilePaths []string, actingUser *models.User) (*connect.Response[xylona.GameServerFilesDeleteResponse], error) {
	node, _, errGet := xs.getRemoteNodeForServer(serverID)
	if errGet != nil {
		return nil, errGet
	}

	client, errClient := xs.remoteFederationClient(node, serverID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationDeleteFilesRequest{
		ServerId:      serverID,
		FullFilePaths: fullFilePaths,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to apply federation identity headers")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete remote files"))
	}

	resp, errDelete := client.DeleteRemoteFiles(ctx, req)
	if errDelete != nil {
		log.Error().Err(errDelete).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to delete remote files")
		return nil, wrapRemoteRPCError(errDelete, "failed to delete remote files")
	}

	return connect.NewResponse(&xylona.GameServerFilesDeleteResponse{
		FullFilePaths: resp.Msg.FullFilePaths,
	}), nil
}

func (xs *XylonaService) renameRemoteFile(ctx context.Context, serverID string, oldPath string, newPath string, actingUser *models.User) (*connect.Response[xylona.GameServerFileRenameResponse], error) {
	node, _, errGet := xs.getRemoteNodeForServer(serverID)
	if errGet != nil {
		return nil, errGet
	}

	client, errClient := xs.remoteFederationClient(node, serverID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationRenameFileRequest{
		ServerId: serverID,
		OldPath:  oldPath,
		NewPath:  newPath,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to apply federation identity headers")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to rename remote file"))
	}

	resp, errRename := client.RenameRemoteFile(ctx, req)
	if errRename != nil {
		log.Error().Err(errRename).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to rename remote file")
		return nil, wrapRemoteRPCError(errRename, "failed to rename remote file")
	}

	return connect.NewResponse(&xylona.GameServerFileRenameResponse{
		NewPath: resp.Msg.NewPath,
	}), nil
}

func (xs *XylonaService) moveRemoteFiles(ctx context.Context, serverID string, fullFilePaths []string, destinationBasePath string, actingUser *models.User) (*connect.Response[xylona.GameServerFilesMoveResponse], error) {
	node, _, errGet := xs.getRemoteNodeForServer(serverID)
	if errGet != nil {
		return nil, errGet
	}

	client, errClient := xs.remoteFederationClient(node, serverID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationMoveFilesRequest{
		ServerId:            serverID,
		FullFilePaths:       fullFilePaths,
		DestinationBasePath: destinationBasePath,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to apply federation identity headers")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to move remote files"))
	}

	resp, errMove := client.MoveRemoteFiles(ctx, req)
	if errMove != nil {
		log.Error().Err(errMove).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to move remote files")
		return nil, wrapRemoteRPCError(errMove, "failed to move remote files")
	}

	return connect.NewResponse(&xylona.GameServerFilesMoveResponse{
		FullFilePaths: resp.Msg.FullFilePaths,
	}), nil
}

func (xs *XylonaService) createRemoteFileOrDirectory(ctx context.Context, serverID string, fullFilePath string, content string, isDirectory bool, actingUser *models.User) (*connect.Response[xylona.GameServerFileOrDirectoryCreateResponse], error) {
	node, _, errGet := xs.getRemoteNodeForServer(serverID)
	if errGet != nil {
		return nil, errGet
	}

	client, errClient := xs.remoteFederationClient(node, serverID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationCreateFileOrDirectoryRequest{
		ServerId:     serverID,
		FullFilePath: fullFilePath,
		Content:      content,
		IsDirectory:  isDirectory,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to apply federation identity headers")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create remote file or directory"))
	}

	_, errCreate := client.CreateRemoteFileOrDirectory(ctx, req)
	if errCreate != nil {
		log.Error().Err(errCreate).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to create remote file/directory")
		return nil, wrapRemoteRPCError(errCreate, "failed to create remote file or directory")
	}

	return connect.NewResponse(&xylona.GameServerFileOrDirectoryCreateResponse{}), nil
}

func (xs *XylonaService) queryRemoteGameServer(ctx context.Context, serverID string, actingUser *models.User) (*connect.Response[xylona.QueryGameServerResponse], error) {
	node, _, errGet := xs.getRemoteNodeForServer(serverID)
	if errGet != nil {
		return nil, errGet
	}

	client, errClient := xs.remoteFederationClient(node, serverID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationQueryServerRequest{
		ServerId: serverID,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to apply federation identity headers")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to query remote server"))
	}

	resp, errQuery := client.QueryRemoteServer(ctx, req)
	if errQuery != nil {
		log.Error().Err(errQuery).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to query remote game server")
		return nil, wrapRemoteRPCError(errQuery, "failed to query remote server")
	}

	return connect.NewResponse(&xylona.QueryGameServerResponse{
		QueryInfo: resp.Msg.QueryInfo,
	}), nil
}

func (xs *XylonaService) downloadRemoteFileFromURL(ctx context.Context, serverID string, url string, destinationBasePath string, actingUser *models.User) (*connect.Response[xylona.GameServersFileDownloadFromURLResponse], error) {
	node, _, errGet := xs.getRemoteNodeForServer(serverID)
	if errGet != nil {
		return nil, errGet
	}

	client, errClient := xs.remoteFederationClient(node, serverID)
	if errClient != nil {
		return nil, errClient
	}

	req := connect.NewRequest(&xylona.FederationDownloadFileFromURLRequest{
		ServerId:            serverID,
		Url:                 url,
		DestinationBasePath: destinationBasePath,
	})
	errIdentity := xs.applyFederatedActingIdentity(req.Header(), actingUser)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to apply federation identity headers")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to download remote file from URL"))
	}

	resp, errDownload := client.DownloadRemoteFileFromURL(ctx, req)
	if errDownload != nil {
		log.Error().Err(errDownload).Str("server_id", serverID).Str("node", node.Name).Msg("Failed to download file from URL on remote")
		return nil, wrapRemoteRPCError(errDownload, "failed to download remote file from URL")
	}

	return connect.NewResponse(&xylona.GameServersFileDownloadFromURLResponse{
		FilePath: resp.Msg.FilePath,
	}), nil
}
