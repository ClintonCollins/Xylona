package rpc

import (
	"context"
	"errors"
	"net/http"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type fileMutationSetup struct {
	gameServer *models.GameServer
	client     nodeclient.NodeClient
	policy     node.ProtectionPolicy
}

func (xs *XylonaService) prepareFileMutation(header http.Header, gameServerID string) (*fileMutationSetup, error) {
	user, errUser := xs.getUserFromHeader(header)
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errLookup := xs.db.GetGameServerByID(gameServerID)
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

	return &fileMutationSetup{
		gameServer: gameServer,
		client:     client,
		policy:     xs.buildProtectionPolicy(gameServer),
	}, nil
}

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
	setup, errSetup := xs.prepareFileMutation(request.Header(), request.Msg.GetGameServerId())
	if errSetup != nil {
		return nil, errSetup
	}
	errCreate := setup.client.CreateFileOrDirectory(ctx, setup.gameServer.Directory, request.Msg.GetFullFilePath(),
		request.Msg.GetContent(), request.Msg.GetIsDirectory(), setup.policy)
	if errCreate != nil {
		return nil, fileMutationError(errCreate)
	}
	return connect.NewResponse(&xylona.GameServerFileOrDirectoryCreateResponse{}), nil
}

// GameServerFilesDelete deletes files or directories for a game server.
func (xs *XylonaService) GameServerFilesDelete(ctx context.Context, request *connect.Request[xylona.GameServerFilesDeleteRequest]) (*connect.Response[xylona.GameServerFilesDeleteResponse], error) {
	setup, errSetup := xs.prepareFileMutation(request.Header(), request.Msg.GetGameServerId())
	if errSetup != nil {
		return nil, errSetup
	}
	results, errDelete := setup.client.DeleteFiles(ctx, setup.gameServer.Directory, request.Msg.GetFullFilePaths(), setup.policy)
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
	setup, errSetup := xs.prepareFileMutation(request.Header(), request.Msg.GetGameServerId())
	if errSetup != nil {
		return nil, errSetup
	}
	result, errDownload := setup.client.DownloadFileFromURL(ctx, setup.gameServer.Directory, request.Msg.GetUrl(), request.Msg.GetDestinationBasePath(), node.DownloadIntegrity{}, setup.policy)
	if errDownload != nil {
		return nil, fileMutationError(errDownload)
	}
	return connect.NewResponse(&xylona.GameServersFileDownloadFromURLResponse{FilePath: result.RelativePath}), nil
}

// GameServerFileRename renames a file for a game server.
func (xs *XylonaService) GameServerFileRename(ctx context.Context, request *connect.Request[xylona.GameServerFileRenameRequest]) (*connect.Response[xylona.GameServerFileRenameResponse], error) {
	setup, errSetup := xs.prepareFileMutation(request.Header(), request.Msg.GetGameServerId())
	if errSetup != nil {
		return nil, errSetup
	}
	newFilePath, errRename := setup.client.RenameFile(ctx, setup.gameServer.Directory, request.Msg.GetOldPath(), request.Msg.GetNewPath(), setup.policy)
	if errRename != nil {
		return nil, fileMutationError(errRename)
	}
	return connect.NewResponse(&xylona.GameServerFileRenameResponse{NewPath: newFilePath}), nil
}

// GameServerFilesMove moves files for a game server.
func (xs *XylonaService) GameServerFilesMove(ctx context.Context, request *connect.Request[xylona.GameServerFilesMoveRequest]) (*connect.Response[xylona.GameServerFilesMoveResponse], error) {
	setup, errSetup := xs.prepareFileMutation(request.Header(), request.Msg.GetGameServerId())
	if errSetup != nil {
		return nil, errSetup
	}
	results, errMove := setup.client.MoveFiles(ctx, setup.gameServer.Directory, request.Msg.GetFullFilePaths(), request.Msg.GetDestinationBasePath(), setup.policy)
	if errMove != nil {
		return nil, fileMutationError(errMove)
	}
	return connect.NewResponse(&xylona.GameServerFilesMoveResponse{FullFilePaths: results}), nil
}

// GameServersFileEdit edits a file for a game server.
func (xs *XylonaService) GameServersFileEdit(ctx context.Context, request *connect.Request[xylona.GameServersFileEditRequest]) (*connect.Response[xylona.GameServersFileEditResponse], error) {
	setup, errSetup := xs.prepareFileMutation(request.Header(), request.Msg.GetGameServerId())
	if errSetup != nil {
		return nil, errSetup
	}
	errEdit := setup.client.WriteFile(ctx, setup.gameServer.Directory, request.Msg.GetFullFilePath(), []byte(request.Msg.GetContent()), setup.policy)
	if errEdit != nil {
		return nil, fileMutationError(errEdit)
	}
	return connect.NewResponse(&xylona.GameServersFileEditResponse{}), nil
}
