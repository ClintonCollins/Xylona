package rpc

import (
	"context"
	"errors"
	"os"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// ListRemoteDirectoryFiles lists files for a local server directory over federation.
func (fs FederationService) ListRemoteDirectoryFiles(ctx context.Context, request *connect.Request[xylona.FederationListDirectoryFilesRequest]) (*connect.Response[xylona.FederationListDirectoryFilesResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.files.view")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, notFoundErr()
	}

	files, errList := fs.actionsInst.ListGameServerFiles(gs, request.Msg.GetPath())
	if errList != nil {
		if errors.Is(errList, actions.ErrInvalidPath) {
			return nil, invalidArg("invalid path")
		}
		if errors.Is(errList, os.ErrNotExist) {
			return nil, notFoundErr()
		}
		return nil, internalErrf("failed to list files")
	}

	return connect.NewResponse(&xylona.FederationListDirectoryFilesResponse{
		Files: files,
	}), nil
}

// EditRemoteFile edits a local file over federation.
func (fs FederationService) EditRemoteFile(ctx context.Context, request *connect.Request[xylona.FederationEditFileRequest]) (*connect.Response[xylona.FederationEditFileResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, notFoundErr()
	}

	errEdit := fs.actionsInst.EditFile(gs, request.Msg.GetFullFilePath(), request.Msg.GetContent())
	if errEdit != nil {
		return nil, internalErrf("failed to edit file")
	}

	return connect.NewResponse(&xylona.FederationEditFileResponse{
		Success: true,
	}), nil
}

// DeleteRemoteFiles deletes local files over federation.
func (fs FederationService) DeleteRemoteFiles(ctx context.Context, request *connect.Request[xylona.FederationDeleteFilesRequest]) (*connect.Response[xylona.FederationDeleteFilesResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, notFoundErr()
	}

	results, errDelete := fs.actionsInst.DeleteFiles(ctx, gs, request.Msg.GetFullFilePaths())
	if errDelete != nil {
		return nil, internalErrf("failed to delete files")
	}

	return connect.NewResponse(&xylona.FederationDeleteFilesResponse{
		Success:       true,
		FullFilePaths: results,
	}), nil
}

// RenameRemoteFile renames a local file over federation.
func (fs FederationService) RenameRemoteFile(ctx context.Context, request *connect.Request[xylona.FederationRenameFileRequest]) (*connect.Response[xylona.FederationRenameFileResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, notFoundErr()
	}

	newPath, errRename := fs.actionsInst.RenameFile(gs, request.Msg.GetOldPath(), request.Msg.GetNewPath())
	if errRename != nil {
		return nil, internalErrf("failed to rename file")
	}

	return connect.NewResponse(&xylona.FederationRenameFileResponse{
		Success: true,
		NewPath: newPath,
	}), nil
}

// MoveRemoteFiles moves local files over federation.
func (fs FederationService) MoveRemoteFiles(ctx context.Context, request *connect.Request[xylona.FederationMoveFilesRequest]) (*connect.Response[xylona.FederationMoveFilesResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, notFoundErr()
	}

	results, errMove := fs.actionsInst.MoveFiles(ctx, gs, request.Msg.GetFullFilePaths(), request.Msg.GetDestinationBasePath())
	if errMove != nil {
		return nil, internalErrf("failed to move files")
	}

	return connect.NewResponse(&xylona.FederationMoveFilesResponse{
		Success:       true,
		FullFilePaths: results,
	}), nil
}

// CreateRemoteFileOrDirectory creates a local file or directory over federation.
func (fs FederationService) CreateRemoteFileOrDirectory(ctx context.Context, request *connect.Request[xylona.FederationCreateFileOrDirectoryRequest]) (*connect.Response[xylona.FederationCreateFileOrDirectoryResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, notFoundErr()
	}

	errCreate := fs.actionsInst.CreateFileOrDirectory(gs, request.Msg.GetFullFilePath(), request.Msg.GetContent(), request.Msg.GetIsDirectory())
	if errCreate != nil {
		return nil, internalErrf("failed to create file or directory")
	}

	return connect.NewResponse(&xylona.FederationCreateFileOrDirectoryResponse{
		Success: true,
	}), nil
}

// DownloadRemoteFileFromURL downloads a file into a local server over federation.
func (fs FederationService) DownloadRemoteFileFromURL(ctx context.Context, request *connect.Request[xylona.FederationDownloadFileFromURLRequest]) (*connect.Response[xylona.FederationDownloadFileFromURLResponse], error) {
	_, errAuth := fs.authenticateRequest(ctx)
	if errAuth != nil {
		return nil, permissionDenied("authentication failed")
	}

	serverID := request.Msg.GetServerId()
	errPermission := fs.authorizeFederatedPermission(ctx, request.Header(), "", "", serverID, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	gs, errGet := fs.db.GetGameServerByID(serverID)
	if errGet != nil {
		return nil, notFoundErr()
	}

	filePath, errDownload := fs.actionsInst.DownloadFileFromURL(ctx, gs, request.Msg.GetUrl(), request.Msg.GetDestinationBasePath())
	if errDownload != nil {
		return nil, internalErrf("failed to download file")
	}

	return connect.NewResponse(&xylona.FederationDownloadFileFromURLResponse{
		Success:  true,
		FilePath: filePath,
	}), nil
}
