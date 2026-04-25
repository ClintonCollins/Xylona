package rpc

import (
	"context"
	"fmt"
	"path"
	"strings"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (xs *XylonaService) gameServerUsesEmbeddedNode(gameServer *models.GameServer) bool {
	selfID := strings.TrimSpace(xs.selfNodeID())
	nodeID := strings.TrimSpace(gameServer.NodeID)
	return selfID == "" || nodeID == "" || nodeID == selfID
}

func sanitizeRemoteFileActionPath(relativePath string) (string, error) {
	normalizedPath := strings.ReplaceAll(strings.TrimSpace(relativePath), `\`, "/")
	if strings.HasPrefix(normalizedPath, "//") {
		return "", actions.ErrInvalidPath
	}

	normalizedPath = strings.TrimPrefix(normalizedPath, "/")
	cleanedPath := path.Clean(normalizedPath)
	if cleanedPath == "." {
		return "", nil
	}
	if cleanedPath == ".." || strings.HasPrefix(cleanedPath, "../") {
		return "", actions.ErrInvalidPath
	}
	if strings.HasPrefix(cleanedPath, "/") {
		return "", actions.ErrInvalidPath
	}
	if hasRemoteWindowsDrivePrefix(cleanedPath) {
		return "", actions.ErrInvalidPath
	}
	return cleanedPath, nil
}

func hasRemoteWindowsDrivePrefix(pathValue string) bool {
	if len(pathValue) < 2 {
		return false
	}
	if (pathValue[0] < 'A' || pathValue[0] > 'Z') && (pathValue[0] < 'a' || pathValue[0] > 'z') {
		return false
	}
	return pathValue[1] == ':'
}

func (xs *XylonaService) archiveGameServerFilesWithNodeClient(
	ctx context.Context,
	gameServer *models.GameServer,
	request *connect.Request[xylona.GameServerFilesCompressionRequest],
	stream *connect.ServerStream[xylona.GameServerFilesArchiveProgress],
) error {
	onProgress := func(progress node.ArchiveProgress) error {
		if stream == nil {
			return nil
		}
		errSend := stream.Send(remoteArchiveProgressToXylona(progress))
		if errSend != nil {
			return fmt.Errorf("send archive progress: %w", errSend)
		}
		return nil
	}
	_, _, errArchive := xs.createGameServerFileArchiveWithNodeClientProgress(ctx, gameServer, request.Msg, onProgress)
	if errArchive != nil {
		return fileMutationError(errArchive)
	}
	return nil
}

func (xs *XylonaService) extractGameServerFilesWithNodeClient(
	ctx context.Context,
	gameServer *models.GameServer,
	request *connect.Request[xylona.GameServerFilesDecompressionRequest],
	stream *connect.ServerStream[xylona.GameServerFilesExtractProgress],
) error {
	onProgress := func(progress node.ExtractProgress) error {
		if stream == nil {
			return nil
		}
		errSend := stream.Send(remoteExtractProgressToXylona(progress))
		if errSend != nil {
			return fmt.Errorf("send extract progress: %w", errSend)
		}
		return nil
	}
	_, _, errExtract := xs.extractGameServerFileArchiveWithNodeClientProgress(ctx, gameServer, request.Msg, onProgress)
	if errExtract != nil {
		return fileMutationError(errExtract)
	}
	return nil
}

func (xs *XylonaService) compressGameServerFilesWithNodeClient(
	ctx context.Context,
	gameServer *models.GameServer,
	request *connect.Request[xylona.GameServerFilesCompressionRequest],
) (*connect.Response[xylona.GameServerFilesCompressionResponse], error) {
	archivePath, _, errArchive := xs.createGameServerFileArchiveWithNodeClient(ctx, gameServer, request.Msg)
	if errArchive != nil {
		return nil, fileMutationError(errArchive)
	}
	return connect.NewResponse(&xylona.GameServerFilesCompressionResponse{FullFilePath: archivePath}), nil
}

func (xs *XylonaService) decompressGameServerFilesWithNodeClient(
	ctx context.Context,
	gameServer *models.GameServer,
	request *connect.Request[xylona.GameServerFilesDecompressionRequest],
) (*connect.Response[xylona.GameServerFilesDecompressionResponse], error) {
	extractedPaths, _, errExtract := xs.extractGameServerFileArchiveWithNodeClient(ctx, gameServer, request.Msg)
	if errExtract != nil {
		return nil, fileMutationError(errExtract)
	}
	return connect.NewResponse(&xylona.GameServerFilesDecompressionResponse{FullFilePaths: extractedPaths}), nil
}

func (xs *XylonaService) createGameServerFileArchiveWithNodeClient(
	ctx context.Context,
	gameServer *models.GameServer,
	request *xylona.GameServerFilesCompressionRequest,
) (string, node.ArchiveProgress, error) {
	return xs.createGameServerFileArchiveWithNodeClientProgress(ctx, gameServer, request, nil)
}

func (xs *XylonaService) createGameServerFileArchiveWithNodeClientProgress(
	ctx context.Context,
	gameServer *models.GameServer,
	request *xylona.GameServerFilesCompressionRequest,
	onProgress func(node.ArchiveProgress) error,
) (string, node.ArchiveProgress, error) {
	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return "", node.ArchiveProgress{}, errClient
	}
	includePaths, errIncludePaths := sanitizeRemoteFileActionPaths(request.GetFullFilePaths())
	if errIncludePaths != nil {
		return "", node.ArchiveProgress{}, errIncludePaths
	}
	destinationPath, errDestination := sanitizeRemoteFileActionPath(request.GetFullDestinationFilePath())
	if errDestination != nil {
		return "", node.ArchiveProgress{}, errDestination
	}
	compression := remoteArchiveCompressionFromXylona(request.GetCompressionType())
	archivePath, progress, errArchive := client.CreateFileArchiveWithProgress(ctx, gameServer.Directory, destinationPath, includePaths, compression, xs.buildProtectionPolicy(gameServer), onProgress)
	if errArchive != nil {
		return "", node.ArchiveProgress{}, fmt.Errorf("create game server file archive: %w", errArchive)
	}
	return archivePath, progress, nil
}

func (xs *XylonaService) extractGameServerFileArchiveWithNodeClient(
	ctx context.Context,
	gameServer *models.GameServer,
	request *xylona.GameServerFilesDecompressionRequest,
) ([]string, node.ExtractProgress, error) {
	return xs.extractGameServerFileArchiveWithNodeClientProgress(ctx, gameServer, request, nil)
}

func (xs *XylonaService) extractGameServerFileArchiveWithNodeClientProgress(
	ctx context.Context,
	gameServer *models.GameServer,
	request *xylona.GameServerFilesDecompressionRequest,
	onProgress func(node.ExtractProgress) error,
) ([]string, node.ExtractProgress, error) {
	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, node.ExtractProgress{}, errClient
	}
	archivePath, errArchivePath := sanitizeRemoteFileActionPath(request.GetFullFilePath())
	if errArchivePath != nil {
		return nil, node.ExtractProgress{}, errArchivePath
	}
	destinationPath, errDestination := sanitizeRemoteFileActionPath(request.GetDestinationBasePath())
	if errDestination != nil {
		return nil, node.ExtractProgress{}, errDestination
	}
	extractedPaths, progress, errExtract := client.ExtractFileArchiveWithProgress(ctx, gameServer.Directory, archivePath, destinationPath, xs.buildProtectionPolicy(gameServer), onProgress)
	if errExtract != nil {
		return nil, node.ExtractProgress{}, fmt.Errorf("extract game server file archive: %w", errExtract)
	}
	return extractedPaths, progress, nil
}

func sanitizeRemoteFileActionPaths(paths []string) ([]string, error) {
	sanitized := make([]string, 0, len(paths))
	for _, pathValue := range paths {
		cleanedPath, errPath := sanitizeRemoteFileActionPath(pathValue)
		if errPath != nil {
			return nil, errPath
		}
		sanitized = append(sanitized, cleanedPath)
	}
	return sanitized, nil
}

func remoteArchiveCompressionFromXylona(compression xylona.GameServerFilesCompressionType) node.ArchiveCompression {
	switch compression {
	case xylona.GameServerFilesCompressionType_BZIP2:
		return node.ArchiveCompressionBZIP2
	case xylona.GameServerFilesCompressionType_GZIP:
		return node.ArchiveCompressionGZIP
	case xylona.GameServerFilesCompressionType_ZST:
		return node.ArchiveCompressionZST
	case xylona.GameServerFilesCompressionType_XZ:
		return node.ArchiveCompressionXZ
	default:
		return node.ArchiveCompressionZIP
	}
}

func remoteArchiveProgressToXylona(progress node.ArchiveProgress) *xylona.GameServerFilesArchiveProgress {
	return &xylona.GameServerFilesArchiveProgress{
		TotalFiles:      progress.TotalFiles,
		FilesCompressed: progress.FilesCompressed,
		TotalBytes:      progress.TotalBytes,
		BytesCompressed: progress.BytesCompressed,
		CurrentFile:     progress.CurrentFile,
	}
}

func remoteExtractProgressToXylona(progress node.ExtractProgress) *xylona.GameServerFilesExtractProgress {
	return &xylona.GameServerFilesExtractProgress{
		TotalFiles:     progress.TotalFiles,
		FilesExtracted: progress.FilesExtracted,
		TotalBytes:     progress.TotalBytes,
		BytesExtracted: progress.BytesExtracted,
		CurrentFile:    progress.CurrentFile,
	}
}
