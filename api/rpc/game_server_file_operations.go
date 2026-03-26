package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func fileMutationError(err error) error {
	if errors.Is(err, actions.ErrInvalidPath) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("invalid path"))
	}
	if errors.Is(err, actions.ErrProtectedPath) {
		return connect.NewError(connect.CodePermissionDenied, errors.New("path is protected"))
	}

	log.Error().Err(err).Msg("file mutation failed")
	return connect.NewError(connect.CodeInternal, errors.New("file operation failed"))
}

func (xs *XylonaService) GameServersFileOrDirectoryCreate(ctx context.Context, request *connect.Request[xylona.GameServerFileOrDirectoryCreateRequest]) (*connect.Response[xylona.GameServerFileOrDirectoryCreateResponse], error) {
	serverID := request.Msg.GameServerId
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.GameServerFileOrDirectoryCreateResponse], error) {
			errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
			if errPermission != nil {
				return nil, errPermission
			}

			errCreate := xs.actionsInst.CreateFileOrDirectory(gameServer, request.Msg.GetFullFilePath(),
				request.Msg.GetContent(), request.Msg.GetIsDirectory())
			if errCreate != nil {
				return nil, fileMutationError(errCreate)
			}
			return &connect.Response[xylona.GameServerFileOrDirectoryCreateResponse]{Msg: &xylona.GameServerFileOrDirectoryCreateResponse{}}, nil
		},
		func() (*connect.Response[xylona.GameServerFileOrDirectoryCreateResponse], error) {
			return xs.createRemoteFileOrDirectory(ctx, serverID, request.Msg.GetFullFilePath(), request.Msg.GetContent(), request.Msg.GetIsDirectory(), user)
		},
	)
}

func (xs *XylonaService) GameServerFilesDelete(ctx context.Context, request *connect.Request[xylona.GameServerFilesDeleteRequest]) (*connect.Response[xylona.GameServerFilesDeleteResponse], error) {
	serverID := request.Msg.GameServerId
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.GameServerFilesDeleteResponse], error) {
			errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
			if errPermission != nil {
				return nil, errPermission
			}

			results, errDelete := xs.actionsInst.DeleteFiles(ctx, gameServer, request.Msg.GetFullFilePaths())
			if errDelete != nil {
				return nil, fileMutationError(errDelete)
			}
			response := &xylona.GameServerFilesDeleteResponse{FullFilePaths: results}
			return &connect.Response[xylona.GameServerFilesDeleteResponse]{Msg: response}, nil
		},
		func() (*connect.Response[xylona.GameServerFilesDeleteResponse], error) {
			return xs.deleteRemoteFiles(ctx, serverID, request.Msg.GetFullFilePaths(), user)
		},
	)
}

func (xs *XylonaService) GameServerFilesArchive(ctx context.Context, request *connect.Request[xylona.GameServerFilesCompressionRequest], c *connect.ServerStream[xylona.GameServerFilesArchiveProgress]) error {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		return errGetGameServer
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
	if errPermission != nil {
		return errPermission
	}

	resultsChan := make(chan *xylona.GameServerFilesArchiveProgress)
	go func() {
		for {
			select {
			case <-xs.ctx.Done():
				return
			case <-ctx.Done():
				return
			case result := <-resultsChan:
				if c != nil {
					errSend := c.Send(result)
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
		return fileMutationError(errCompress)
	}
	errSend := c.Send(lastResult)
	if errSend != nil {
		log.Err(errSend).Msg("failed to send last result")
		return connect.NewError(connect.CodeInternal, errSend)
	}
	return nil
}

func (xs *XylonaService) GameServerFilesExtract(ctx context.Context, request *connect.Request[xylona.GameServerFilesDecompressionRequest], c *connect.ServerStream[xylona.GameServerFilesExtractProgress]) error {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		return errGetGameServer
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
	if errPermission != nil {
		return errPermission
	}

	resultsChan := make(chan *xylona.GameServerFilesExtractProgress)
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
					errSend := c.Send(result)
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
		return fileMutationError(errCompress)
	}
	return nil
}

func (xs *XylonaService) GameServerFilesCompress(ctx context.Context, request *connect.Request[xylona.GameServerFilesCompressionRequest]) (*connect.Response[xylona.GameServerFilesCompressionResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	results, errCompress := xs.actionsInst.ArchiveAndCompressFiles(ctx, gameServer, request.Msg.GetFullDestinationFilePath(),
		request.Msg.GetFullFilePaths(), request.Msg.GetCompressionType())
	if errCompress != nil {
		return nil, fileMutationError(errCompress)
	}
	response := &xylona.GameServerFilesCompressionResponse{FullFilePath: results}
	return &connect.Response[xylona.GameServerFilesCompressionResponse]{Msg: response}, nil
}

func (xs *XylonaService) GameServerFilesDecompress(ctx context.Context, request *connect.Request[xylona.GameServerFilesDecompressionRequest]) (*connect.Response[xylona.GameServerFilesDecompressionResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GameServerId)
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	results, errDecompress := xs.actionsInst.ExtractArchive(ctx, gameServer, request.Msg.GetFullFilePath(), request.Msg.GetDestinationBasePath())
	if errDecompress != nil {
		return nil, fileMutationError(errDecompress)
	}
	response := &xylona.GameServerFilesDecompressionResponse{FullFilePaths: results}
	return &connect.Response[xylona.GameServerFilesDecompressionResponse]{Msg: response}, nil
}

func (xs *XylonaService) GameServerFilesDownloadFromURL(ctx context.Context, request *connect.Request[xylona.GameServersFileDownloadFromURLRequest]) (*connect.Response[xylona.GameServersFileDownloadFromURLResponse], error) {
	serverID := request.Msg.GameServerId
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.GameServersFileDownloadFromURLResponse], error) {
			errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
			if errPermission != nil {
				return nil, errPermission
			}

			results, errDownload := xs.actionsInst.DownloadFileFromURL(ctx, gameServer, request.Msg.GetUrl(), request.Msg.GetDestinationBasePath())
			if errDownload != nil {
				return nil, fileMutationError(errDownload)
			}
			response := &xylona.GameServersFileDownloadFromURLResponse{FilePath: results}
			return &connect.Response[xylona.GameServersFileDownloadFromURLResponse]{Msg: response}, nil
		},
		func() (*connect.Response[xylona.GameServersFileDownloadFromURLResponse], error) {
			return xs.downloadRemoteFileFromURL(ctx, serverID, request.Msg.GetUrl(), request.Msg.GetDestinationBasePath(), user)
		},
	)
}

func (xs *XylonaService) GameServerFileRename(ctx context.Context, request *connect.Request[xylona.GameServerFileRenameRequest]) (*connect.Response[xylona.GameServerFileRenameResponse], error) {
	serverID := request.Msg.GameServerId
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.GameServerFileRenameResponse], error) {
			errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
			if errPermission != nil {
				return nil, errPermission
			}

			newFilePath, errRename := xs.actionsInst.RenameFile(gameServer, request.Msg.GetOldPath(), request.Msg.GetNewPath())
			if errRename != nil {
				return nil, fileMutationError(errRename)
			}
			response := &xylona.GameServerFileRenameResponse{NewPath: newFilePath}
			return &connect.Response[xylona.GameServerFileRenameResponse]{Msg: response}, nil
		},
		func() (*connect.Response[xylona.GameServerFileRenameResponse], error) {
			return xs.renameRemoteFile(ctx, serverID, request.Msg.GetOldPath(), request.Msg.GetNewPath(), user)
		},
	)
}

func (xs *XylonaService) GameServerFilesMove(ctx context.Context, request *connect.Request[xylona.GameServerFilesMoveRequest]) (*connect.Response[xylona.GameServerFilesMoveResponse], error) {
	serverID := request.Msg.GameServerId
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.GameServerFilesMoveResponse], error) {
			errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
			if errPermission != nil {
				return nil, errPermission
			}

			results, errMove := xs.actionsInst.MoveFiles(ctx, gameServer, request.Msg.GetFullFilePaths(), request.Msg.GetDestinationBasePath())
			if errMove != nil {
				return nil, fileMutationError(errMove)
			}
			response := &xylona.GameServerFilesMoveResponse{FullFilePaths: results}
			return &connect.Response[xylona.GameServerFilesMoveResponse]{Msg: response}, nil
		},
		func() (*connect.Response[xylona.GameServerFilesMoveResponse], error) {
			return xs.moveRemoteFiles(ctx, serverID, request.Msg.GetFullFilePaths(), request.Msg.GetDestinationBasePath(), user)
		},
	)
}

func (xs *XylonaService) GameServersFileEdit(ctx context.Context, request *connect.Request[xylona.GameServersFileEditRequest]) (*connect.Response[xylona.GameServersFileEditResponse], error) {
	serverID := request.Msg.GameServerId
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	return dispatchGameServerRequest(
		xs,
		serverID,
		func(gameServer *models.GameServer) (*connect.Response[xylona.GameServersFileEditResponse], error) {
			errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
			if errPermission != nil {
				return nil, errPermission
			}

			errEdit := xs.actionsInst.EditFile(gameServer, request.Msg.GetFullFilePath(), request.Msg.GetContent())
			if errEdit != nil {
				return nil, fileMutationError(errEdit)
			}
			return &connect.Response[xylona.GameServersFileEditResponse]{Msg: &xylona.GameServersFileEditResponse{}}, nil
		},
		func() (*connect.Response[xylona.GameServersFileEditResponse], error) {
			return xs.editRemoteFile(ctx, serverID, request.Msg.GetFullFilePath(), request.Msg.GetContent(), user)
		},
	)
}
