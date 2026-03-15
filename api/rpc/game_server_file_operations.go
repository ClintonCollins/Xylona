package rpc

import (
	"context"
	"database/sql"
	"errors"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (xs XylonaService) GameServersFileOrDirectoryCreate(ctx context.Context, request *connect.Request[xylona.GameServerFileOrDirectoryCreateRequest]) (*connect.Response[xylona.GameServerFileOrDirectoryCreateResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return xs.createRemoteFileOrDirectory(ctx, request.Msg.GameServerId, request.Msg.GetFullFilePath(), request.Msg.GetContent(), request.Msg.GetIsDirectory())
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	errCreate := xs.actionsInst.CreateFileOrDirectory(gameServer, request.Msg.GetFullFilePath(),
		request.Msg.GetContent(), request.Msg.GetIsDirectory())
	if errCreate != nil {
		return nil, connect.NewError(connect.CodeInternal, errCreate)
	}
	return &connect.Response[xylona.GameServerFileOrDirectoryCreateResponse]{Msg: &xylona.GameServerFileOrDirectoryCreateResponse{}}, nil
}

func (xs XylonaService) GameServerFilesDelete(ctx context.Context, request *connect.Request[xylona.GameServerFilesDeleteRequest]) (*connect.Response[xylona.GameServerFilesDeleteResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return xs.deleteRemoteFiles(ctx, request.Msg.GameServerId, request.Msg.GetFullFilePaths())
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	results, errDelete := xs.actionsInst.DeleteFiles(ctx, gameServer, request.Msg.GetFullFilePaths())
	if errDelete != nil {
		return nil, errDelete
	}
	response := &xylona.GameServerFilesDeleteResponse{FullFilePaths: results}
	return &connect.Response[xylona.GameServerFilesDeleteResponse]{Msg: response}, nil
}

func (xs XylonaService) GameServerFilesArchive(ctx context.Context, request *connect.Request[xylona.GameServerFilesCompressionRequest], c *connect.ServerStream[xylona.GameServerFilesArchiveProgress]) error {
	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		return nil
	}
	resultsChan := make(chan xylona.GameServerFilesArchiveProgress)
	go func() {
		for {
			select {
			case <-xs.ctx.Done():
				return
			case <-ctx.Done():
				return
			case result := <-resultsChan:
				if c != nil {
					errSend := c.Send(&result)
					if errSend != nil {
						return
					}
				}
			}
		}
	}()

	lastResult, errCompress := xs.actionsInst.ArchiveFiles(ctx, gameServer, request.Msg.GetFullDestinationFilePath(),
		request.Msg.GetFullFilePaths(), request.Msg.GetCompressionType(), resultsChan)
	if errCompress != nil {
		return connect.NewError(connect.CodeInternal, errCompress)
	}
	errSend := c.Send(&lastResult)
	if errSend != nil {
		log.Err(errSend).Msg("failed to send last result")
		return connect.NewError(connect.CodeInternal, errSend)
	}
	return nil
}

func (xs XylonaService) GameServerFilesExtract(ctx context.Context, request *connect.Request[xylona.GameServerFilesDecompressionRequest], c *connect.ServerStream[xylona.GameServerFilesExtractProgress]) error {
	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		return nil
	}
	resultsChan := make(chan xylona.GameServerFilesExtractProgress)
	go func() {
		if recover() != nil {
			log.Error().Msg("recovered from panic")
			return
		}
		for {
			select {
			case <-xs.ctx.Done():
				return
			case <-ctx.Done():
				return
			case result := <-resultsChan:
				if c != nil {
					errSend := c.Send(&result)
					if errSend != nil {
						return
					}
				}
			}
		}
	}()

	_, errCompress := xs.actionsInst.ExtractFiles(ctx, gameServer, request.Msg.GetFullFilePath(),
		request.Msg.GetDestinationBasePath(), resultsChan)
	if errCompress != nil {
		return connect.NewError(connect.CodeInternal, errCompress)
	}
	return nil
}

func (xs XylonaService) GameServerFilesCompress(ctx context.Context, request *connect.Request[xylona.GameServerFilesCompressionRequest]) (*connect.Response[xylona.GameServerFilesCompressionResponse], error) {
	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}
	results, errCompress := xs.actionsInst.ArchiveAndCompressFiles(ctx, gameServer, request.Msg.GetFullDestinationFilePath(),
		request.Msg.GetFullFilePaths(), request.Msg.GetCompressionType())
	if errCompress != nil {
		return nil, connect.NewError(connect.CodeInternal, errCompress)
	}
	response := &xylona.GameServerFilesCompressionResponse{FullFilePath: results}
	return &connect.Response[xylona.GameServerFilesCompressionResponse]{Msg: response}, nil
}

func (xs XylonaService) GameServerFilesDecompress(ctx context.Context, request *connect.Request[xylona.GameServerFilesDecompressionRequest]) (*connect.Response[xylona.GameServerFilesDecompressionResponse], error) {
	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}
	results, errDecompress := xs.actionsInst.ExtractArchive(ctx, gameServer, request.Msg.GetFullFilePath(), request.Msg.GetDestinationBasePath())
	if errDecompress != nil {
		return nil, connect.NewError(connect.CodeInternal, errDecompress)
	}
	response := &xylona.GameServerFilesDecompressionResponse{FullFilePaths: results}
	return &connect.Response[xylona.GameServerFilesDecompressionResponse]{Msg: response}, nil
}

func (xs XylonaService) GameServerFilesDownloadFromURL(ctx context.Context, request *connect.Request[xylona.GameServersFileDownloadFromURLRequest]) (*connect.Response[xylona.GameServersFileDownloadFromURLResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return xs.downloadRemoteFileFromURL(ctx, request.Msg.GameServerId, request.Msg.GetUrl(), request.Msg.GetDestinationBasePath())
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	results, errDownload := xs.actionsInst.DownloadFileFromURL(ctx, gameServer, request.Msg.GetUrl(), request.Msg.GetDestinationBasePath())
	if errDownload != nil {
		return nil, connect.NewError(connect.CodeInternal, errDownload)
	}
	response := &xylona.GameServersFileDownloadFromURLResponse{FilePath: results}
	return &connect.Response[xylona.GameServersFileDownloadFromURLResponse]{Msg: response}, nil
}

func (xs XylonaService) GameServerFileRename(ctx context.Context, request *connect.Request[xylona.GameServerFileRenameRequest]) (*connect.Response[xylona.GameServerFileRenameResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return xs.renameRemoteFile(ctx, request.Msg.GameServerId, request.Msg.GetOldPath(), request.Msg.GetNewPath())
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	newFilePath, errRename := xs.actionsInst.RenameFile(gameServer, request.Msg.GetOldPath(), request.Msg.GetNewPath())
	if errRename != nil {
		return nil, connect.NewError(connect.CodeInternal, errRename)
	}
	response := &xylona.GameServerFileRenameResponse{NewPath: newFilePath}
	return &connect.Response[xylona.GameServerFileRenameResponse]{Msg: response}, nil
}

func (xs XylonaService) GameServerFilesMove(ctx context.Context, request *connect.Request[xylona.GameServerFilesMoveRequest]) (*connect.Response[xylona.GameServerFilesMoveResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return xs.moveRemoteFiles(ctx, request.Msg.GameServerId, request.Msg.GetFullFilePaths(), request.Msg.GetDestinationBasePath())
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	results, errMove := xs.actionsInst.MoveFiles(ctx, gameServer, request.Msg.GetFullFilePaths(), request.Msg.GetDestinationBasePath())
	if errMove != nil {
		return nil, connect.NewError(connect.CodeInternal, errMove)
	}
	response := &xylona.GameServerFilesMoveResponse{FullFilePaths: results}
	return &connect.Response[xylona.GameServerFilesMoveResponse]{Msg: response}, nil
}

func (xs XylonaService) GameServersFileEdit(ctx context.Context, request *connect.Request[xylona.GameServersFileEditRequest]) (*connect.Response[xylona.GameServersFileEditResponse], error) {
	gameServer, errGetGameServer := xs.db.GetGameServerByID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		if errors.Is(errGetGameServer, sql.ErrNoRows) {
			return xs.editRemoteFile(ctx, request.Msg.GameServerId, request.Msg.GetFullFilePath(), request.Msg.GetContent())
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	errEdit := xs.actionsInst.EditFile(gameServer, request.Msg.GetFullFilePath(), request.Msg.GetContent())
	if errEdit != nil {
		return nil, connect.NewError(connect.CodeInternal, errEdit)
	}
	return &connect.Response[xylona.GameServersFileEditResponse]{Msg: &xylona.GameServersFileEditResponse{}}, nil
}
