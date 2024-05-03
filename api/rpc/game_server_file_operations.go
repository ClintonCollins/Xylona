package rpc

import (
	"context"

	connect_go "github.com/bufbuild/connect-go"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (xs XylonaService) GameServerFilesDelete(ctx context.Context, request *connect_go.Request[xylona.GameServerFilesDeleteRequest]) (*connect_go.Response[xylona.GameServerFilesDeleteResponse], error) {
	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}
	results, errDelete := xs.actionsInst.DeleteFiles(ctx, gameServer, request.Msg.FilePaths)
	if errDelete != nil {
		return nil, errDelete
	}
	response := &xylona.GameServerFilesDeleteResponse{FilePaths: results}
	return &connect_go.Response[xylona.GameServerFilesDeleteResponse]{Msg: response}, nil
}

func (xs XylonaService) GameServerFilesCompress(ctx context.Context, request *connect_go.Request[xylona.GameServerFilesCompressionRequest]) (*connect_go.Response[xylona.GameServerFilesCompressionResponse], error) {
	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}
	results, errCompress := xs.actionsInst.ArchiveAndCompressFiles(ctx, gameServer, request.Msg.GetFileName(),
		request.Msg.GetFilePaths(), request.Msg.GetCompressionType(), request.Msg.GetSourcePath())
	if errCompress != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errCompress)
	}
	response := &xylona.GameServerFilesCompressionResponse{FilePath: results}
	return &connect_go.Response[xylona.GameServerFilesCompressionResponse]{Msg: response}, nil
}

func (xs XylonaService) GameServerFilesDecompress(ctx context.Context, request *connect_go.Request[xylona.GameServerFilesDecompressionRequest]) (*connect_go.Response[xylona.GameServerFilesDecompressionResponse], error) {
	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}
	results, errDecompress := xs.actionsInst.ExtractArchive(ctx, gameServer, request.Msg.GetFilePath(), request.Msg.GetDestinationPath())
	if errDecompress != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errDecompress)
	}
	response := &xylona.GameServerFilesDecompressionResponse{FilePaths: results}
	return &connect_go.Response[xylona.GameServerFilesDecompressionResponse]{Msg: response}, nil
}

func (xs XylonaService) GameServerFilesDownloadFromURL(ctx context.Context, request *connect_go.Request[xylona.GameServersFileDownloadFromURLRequest]) (*connect_go.Response[xylona.GameServersFileDownloadFromURLResponse], error) {
	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}
	results, errDownload := xs.actionsInst.DownloadFileFromURL(ctx, gameServer, request.Msg.GetUrl(), request.Msg.GetDestinationPath())
	if errDownload != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errDownload)
	}
	response := &xylona.GameServersFileDownloadFromURLResponse{FilePath: results}
	return &connect_go.Response[xylona.GameServersFileDownloadFromURLResponse]{Msg: response}, nil
}

func (xs XylonaService) GameServerFileRename(_ context.Context, request *connect_go.Request[xylona.GameServerFileRenameRequest]) (*connect_go.Response[xylona.GameServerFileRenameResponse], error) {
	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}
	newFilePath, errRename := xs.actionsInst.RenameFile(gameServer, request.Msg.GetOldPath(), request.Msg.GetNewPath())
	if errRename != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errRename)
	}
	response := &xylona.GameServerFileRenameResponse{NewPath: newFilePath}
	return &connect_go.Response[xylona.GameServerFileRenameResponse]{Msg: response}, nil
}

func (xs XylonaService) GameServerFilesMove(ctx context.Context, request *connect_go.Request[xylona.GameServerFilesMoveRequest]) (*connect_go.Response[xylona.GameServerFilesMoveResponse], error) {
	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}
	results, errMove := xs.actionsInst.MoveFiles(ctx, gameServer, request.Msg.FilePaths, request.Msg.Destination)
	if errMove != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errMove)
	}
	response := &xylona.GameServerFilesMoveResponse{FilePaths: results}
	return &connect_go.Response[xylona.GameServerFilesMoveResponse]{Msg: response}, nil
}

func (xs XylonaService) GameServersFileEdit(_ context.Context, request *connect_go.Request[xylona.GameServersFileEditRequest]) (*connect_go.Response[xylona.GameServersFileEditResponse], error) {
	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}
	errEdit := xs.actionsInst.EditFile(gameServer, request.Msg.GetFilePath(), request.Msg.GetContent())
	if errEdit != nil {
		return nil, connect_go.NewError(connect_go.CodeInternal, errEdit)
	}
	return &connect_go.Response[xylona.GameServersFileEditResponse]{Msg: &xylona.GameServersFileEditResponse{}}, nil
}
