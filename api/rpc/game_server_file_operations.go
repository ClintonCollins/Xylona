package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func fileMutationError(err error) error {
	if errors.Is(err, actions.ErrInvalidPath) || errors.Is(err, node.ErrInvalidPath) {
		return invalidArg("invalid path")
	}
	if errors.Is(err, actions.ErrProtectedPath) || errors.Is(err, node.ErrProtectedPath) {
		return permissionDenied("path is protected")
	}

	log.Error().Err(err).Msg("file mutation failed")
	return internalErrf("file operation failed")
}

// GameServersFileOrDirectoryCreate creates a file or directory for a game server.
func (xs *XylonaService) GameServersFileOrDirectoryCreate(ctx context.Context, request *connect.Request[xylona.GameServerFileOrDirectoryCreateRequest]) (*connect.Response[xylona.GameServerFileOrDirectoryCreateResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetGameServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}
	errCreate := client.CreateFileOrDirectory(ctx, gameServer.Directory, request.Msg.GetFullFilePath(),
		request.Msg.GetContent(), request.Msg.GetIsDirectory(), xs.buildProtectionPolicy(gameServer))
	if errCreate != nil {
		return nil, fileMutationError(errCreate)
	}
	return connect.NewResponse(&xylona.GameServerFileOrDirectoryCreateResponse{}), nil
}

// GameServerFilesDelete deletes files or directories for a game server.
func (xs *XylonaService) GameServerFilesDelete(ctx context.Context, request *connect.Request[xylona.GameServerFilesDeleteRequest]) (*connect.Response[xylona.GameServerFilesDeleteResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetGameServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}
	results, errDelete := client.DeleteFiles(ctx, gameServer.Directory, request.Msg.GetFullFilePaths(), xs.buildProtectionPolicy(gameServer))
	if errDelete != nil {
		return nil, fileMutationError(errDelete)
	}
	return connect.NewResponse(&xylona.GameServerFilesDeleteResponse{FullFilePaths: results}), nil
}

// GameServerFilesArchive streams archive progress for a game server file selection.
func (xs *XylonaService) GameServerFilesArchive(ctx context.Context, request *connect.Request[xylona.GameServerFilesCompressionRequest], c *connect.ServerStream[xylona.GameServerFilesArchiveProgress]) error {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return unauthenticated()
	}

	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GetGameServerId())
	if errGetGameServer != nil {
		return errGetGameServer
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
	if errPermission != nil {
		return errPermission
	}

	return xs.archiveGameServerFilesWithNodeClient(ctx, gameServer, request, c)
}

// GameServerFilesExtract streams archive extraction progress for a game server.
func (xs *XylonaService) GameServerFilesExtract(ctx context.Context, request *connect.Request[xylona.GameServerFilesDecompressionRequest], c *connect.ServerStream[xylona.GameServerFilesExtractProgress]) error {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return unauthenticated()
	}

	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GetGameServerId())
	if errGetGameServer != nil {
		return errGetGameServer
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
	if errPermission != nil {
		return errPermission
	}

	return xs.extractGameServerFilesWithNodeClient(ctx, gameServer, request, c)
}

// GameServerFilesCompress creates a compressed archive for game server files.
func (xs *XylonaService) GameServerFilesCompress(ctx context.Context, request *connect.Request[xylona.GameServerFilesCompressionRequest]) (*connect.Response[xylona.GameServerFilesCompressionResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GetGameServerId())
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	return xs.compressGameServerFilesWithNodeClient(ctx, gameServer, request)
}

// GameServerFilesDecompress extracts an archive for a game server.
func (xs *XylonaService) GameServerFilesDecompress(ctx context.Context, request *connect.Request[xylona.GameServerFilesDecompressionRequest]) (*connect.Response[xylona.GameServerFilesDecompressionResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	gameServer, errGetGameServer := xs.getGameServerFromID(request.Msg.GetGameServerId())
	if errGetGameServer != nil {
		return nil, errGetGameServer
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	return xs.decompressGameServerFilesWithNodeClient(ctx, gameServer, request)
}

// GameServerFilesDownloadFromURL downloads a file into a game server directory.
func (xs *XylonaService) GameServerFilesDownloadFromURL(ctx context.Context, request *connect.Request[xylona.GameServersFileDownloadFromURLRequest]) (*connect.Response[xylona.GameServersFileDownloadFromURLResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetGameServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}
	result, errDownload := client.DownloadFileFromURL(ctx, gameServer.Directory, request.Msg.GetUrl(), request.Msg.GetDestinationBasePath(), node.DownloadIntegrity{}, xs.buildProtectionPolicy(gameServer))
	if errDownload != nil {
		return nil, fileMutationError(errDownload)
	}
	return connect.NewResponse(&xylona.GameServersFileDownloadFromURLResponse{FilePath: result.RelativePath}), nil
}

// GameServerFileRename renames a file for a game server.
func (xs *XylonaService) GameServerFileRename(ctx context.Context, request *connect.Request[xylona.GameServerFileRenameRequest]) (*connect.Response[xylona.GameServerFileRenameResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetGameServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}
	newFilePath, errRename := client.RenameFile(ctx, gameServer.Directory, request.Msg.GetOldPath(), request.Msg.GetNewPath(), xs.buildProtectionPolicy(gameServer))
	if errRename != nil {
		return nil, fileMutationError(errRename)
	}
	return connect.NewResponse(&xylona.GameServerFileRenameResponse{NewPath: newFilePath}), nil
}

// GameServerFilesMove moves files for a game server.
func (xs *XylonaService) GameServerFilesMove(ctx context.Context, request *connect.Request[xylona.GameServerFilesMoveRequest]) (*connect.Response[xylona.GameServerFilesMoveResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetGameServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}
	results, errMove := client.MoveFiles(ctx, gameServer.Directory, request.Msg.GetFullFilePaths(), request.Msg.GetDestinationBasePath(), xs.buildProtectionPolicy(gameServer))
	if errMove != nil {
		return nil, fileMutationError(errMove)
	}
	return connect.NewResponse(&xylona.GameServerFilesMoveResponse{FullFilePaths: results}), nil
}

// GameServersFileEdit edits a file for a game server.
func (xs *XylonaService) GameServersFileEdit(ctx context.Context, request *connect.Request[xylona.GameServersFileEditRequest]) (*connect.Response[xylona.GameServersFileEditResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(request.Msg.GetGameServerId())
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.files.edit")
	if errPermission != nil {
		return nil, errPermission
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}
	errEdit := client.WriteFile(ctx, gameServer.Directory, request.Msg.GetFullFilePath(), []byte(request.Msg.GetContent()), xs.buildProtectionPolicy(gameServer))
	if errEdit != nil {
		return nil, fileMutationError(errEdit)
	}
	return connect.NewResponse(&xylona.GameServersFileEditResponse{}), nil
}
