package rpc

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (xs *XylonaService) gameServerUsesEmbeddedNode(gameServer *models.GameServer) bool {
	selfID := strings.TrimSpace(xs.selfNodeID())
	nodeID := strings.TrimSpace(gameServer.NodeID)
	return selfID == "" || nodeID == "" || nodeID == selfID
}

func cloneGameServerWithDirectory(gameServer *models.GameServer, directory string) *models.GameServer {
	cloned := *gameServer
	cloned.Directory = directory
	return &cloned
}

func (xs *XylonaService) stageRemoteGameServerPaths(
	ctx context.Context,
	gameServer *models.GameServer,
	relativePaths []string,
) (*models.GameServer, nodeclient.NodeClient, func(), error) {
	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, nil, nil, errClient
	}

	stagingDir, errMkdirTemp := os.MkdirTemp("", "xylona-remote-files-*")
	if errMkdirTemp != nil {
		return nil, nil, nil, fmt.Errorf("create remote file staging directory: %w", errMkdirTemp)
	}

	cleanup := func() {
		_ = os.RemoveAll(stagingDir)
	}

	for _, relativePath := range relativePaths {
		errStage := stageRemoteGameServerPath(ctx, client, gameServer.Directory, stagingDir, relativePath)
		if errStage != nil {
			cleanup()
			return nil, nil, nil, errStage
		}
	}

	return cloneGameServerWithDirectory(gameServer, stagingDir), client, cleanup, nil
}

func stageRemoteGameServerPath(
	ctx context.Context,
	client nodeclient.NodeClient,
	remoteRoot string,
	stagingDir string,
	relativePath string,
) error {
	normalizedPath, errPath := sanitizeRemoteFileActionPath(relativePath)
	if errPath != nil {
		return errPath
	}

	fileData, errRead := client.ReadFile(ctx, remoteRoot, normalizedPath)
	if errRead == nil {
		return writeStagedRemoteFile(stagingDir, normalizedPath, fileData)
	}

	return stageRemoteDirectoryTree(ctx, client, remoteRoot, stagingDir, normalizedPath)
}

func stageRemoteDirectoryTree(
	ctx context.Context,
	client nodeclient.NodeClient,
	remoteRoot string,
	stagingDir string,
	relativePath string,
) error {
	entries, errList := client.ListFiles(ctx, remoteRoot, relativePath)
	if errList != nil {
		return fmt.Errorf("stage remote path %q: %w", relativePath, errList)
	}

	localDir := stagingDir
	if relativePath != "" {
		localDir = filepath.Join(stagingDir, filepath.FromSlash(relativePath))
	}

	errMkdirAll := os.MkdirAll(localDir, 0o750)
	if errMkdirAll != nil {
		return fmt.Errorf("create staging directory %q: %w", localDir, errMkdirAll)
	}

	for _, entry := range entries {
		childPath := entry.Name
		if relativePath != "" {
			childPath = path.Join(relativePath, entry.Name)
		}

		if entry.IsDirectory {
			errStageDir := stageRemoteDirectoryTree(ctx, client, remoteRoot, stagingDir, childPath)
			if errStageDir != nil {
				return errStageDir
			}
			continue
		}

		fileData, errRead := client.ReadFile(ctx, remoteRoot, childPath)
		if errRead != nil {
			return fmt.Errorf("read remote file %q: %w", childPath, errRead)
		}
		errWrite := writeStagedRemoteFile(stagingDir, childPath, fileData)
		if errWrite != nil {
			return errWrite
		}
	}

	return nil
}

func writeStagedRemoteFile(stagingDir string, relativePath string, fileData []byte) error {
	localPath := filepath.Join(stagingDir, filepath.FromSlash(relativePath))
	errMkdirAll := os.MkdirAll(filepath.Dir(localPath), 0o750)
	if errMkdirAll != nil {
		return fmt.Errorf("create staged file directory %q: %w", localPath, errMkdirAll)
	}

	errWrite := os.WriteFile(localPath, fileData, 0o600)
	if errWrite != nil {
		return fmt.Errorf("write staged file %q: %w", localPath, errWrite)
	}

	return nil
}

func uploadStagedFileToNode(
	ctx context.Context,
	client nodeclient.NodeClient,
	remoteRoot string,
	localRoot string,
	relativePath string,
) error {
	normalizedPath, errPath := sanitizeRemoteFileActionPath(relativePath)
	if errPath != nil {
		return errPath
	}

	localPath := filepath.Join(localRoot, filepath.FromSlash(normalizedPath))
	fileData, errRead := os.ReadFile(localPath)
	if errRead != nil {
		return fmt.Errorf("read staged output %q: %w", localPath, errRead)
	}

	errEnsureDir := ensureRemoteDirectory(ctx, client, remoteRoot, path.Dir(normalizedPath))
	if errEnsureDir != nil {
		return errEnsureDir
	}

	errWrite := client.WriteFile(ctx, remoteRoot, normalizedPath, fileData, node.ProtectionPolicy{})
	if errWrite != nil {
		return fmt.Errorf("upload staged file %q: %w", normalizedPath, errWrite)
	}

	return nil
}

func uploadStagedDirectoryTreeToNode(
	ctx context.Context,
	client nodeclient.NodeClient,
	remoteRoot string,
	localRoot string,
	relativeBase string,
	excludedRelativePaths ...string,
) error {
	normalizedBase, errBase := sanitizeRemoteFileActionPath(relativeBase)
	if errBase != nil {
		return errBase
	}

	excludedPaths := make(map[string]struct{}, len(excludedRelativePaths))
	for _, relativePath := range excludedRelativePaths {
		normalizedPath, errPath := sanitizeRemoteFileActionPath(relativePath)
		if errPath != nil {
			return errPath
		}
		if normalizedPath == "" {
			continue
		}
		excludedPaths[normalizedPath] = struct{}{}
	}

	localBasePath := localRoot
	if normalizedBase != "" {
		localBasePath = filepath.Join(localRoot, filepath.FromSlash(normalizedBase))
	}

	_, errStat := os.Stat(localBasePath)
	if errors.Is(errStat, os.ErrNotExist) {
		return nil
	}
	if errStat != nil {
		return fmt.Errorf("stat staged extraction path %q: %w", localBasePath, errStat)
	}

	errWalk := filepath.WalkDir(localBasePath, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if currentPath == localBasePath {
			return ensureRemoteDirectory(ctx, client, remoteRoot, normalizedBase)
		}

		relativeChild, errRelative := filepath.Rel(localBasePath, currentPath)
		if errRelative != nil {
			return fmt.Errorf("resolve staged relative path: %w", errRelative)
		}

		remotePath := filepath.ToSlash(relativeChild)
		if normalizedBase != "" {
			remotePath = path.Join(normalizedBase, remotePath)
		}
		if _, excluded := excludedPaths[remotePath]; excluded {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if entry.IsDir() {
			return ensureRemoteDirectory(ctx, client, remoteRoot, remotePath)
		}

		errUpload := uploadStagedFileToNode(ctx, client, remoteRoot, localRoot, remotePath)
		if errUpload != nil {
			return errUpload
		}
		return nil
	})
	if errWalk != nil {
		return fmt.Errorf("walk staged extraction path %q: %w", localBasePath, errWalk)
	}

	return nil
}

func ensureRemoteDirectory(ctx context.Context, client nodeclient.NodeClient, remoteRoot string, relativePath string) error {
	trimmedPath := strings.TrimSpace(relativePath)
	if trimmedPath == "" || trimmedPath == "." {
		return nil
	}

	errCreate := client.CreateFileOrDirectory(ctx, remoteRoot, trimmedPath, "", true, node.ProtectionPolicy{})
	if errCreate != nil {
		return fmt.Errorf("create remote directory %q: %w", trimmedPath, errCreate)
	}
	return nil
}

func sanitizeRemoteFileActionPath(relativePath string) (string, error) {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(relativePath))
	normalizedPath = strings.TrimPrefix(normalizedPath, "/")
	if normalizedPath != "" && !filepath.IsLocal(normalizedPath) {
		return "", actions.ErrInvalidPath
	}
	return normalizedPath, nil
}

func archiveCompressionExtension(compression xylona.GameServerFilesCompressionType) string {
	switch compression {
	case xylona.GameServerFilesCompressionType_GZIP:
		return ".tar.gz"
	case xylona.GameServerFilesCompressionType_BZIP2:
		return ".tar.bz2"
	case xylona.GameServerFilesCompressionType_ZST:
		return ".tar.zst"
	case xylona.GameServerFilesCompressionType_XZ:
		return ".tar.xz"
	default:
		return ".zip"
	}
}

func ensureStagedArchiveDestination(localRoot string, destinationPath string) error {
	normalizedPath, errPath := sanitizeRemoteFileActionPath(destinationPath)
	if errPath != nil {
		return errPath
	}

	parentDir := filepath.Dir(filepath.Join(localRoot, filepath.FromSlash(normalizedPath)))
	errMkdirAll := os.MkdirAll(parentDir, 0o750)
	if errMkdirAll != nil {
		return fmt.Errorf("create staged archive destination %q: %w", parentDir, errMkdirAll)
	}

	return nil
}

func (xs *XylonaService) archiveRemoteGameServerFiles(
	ctx context.Context,
	gameServer *models.GameServer,
	request *connect.Request[xylona.GameServerFilesCompressionRequest],
	stream *connect.ServerStream[xylona.GameServerFilesArchiveProgress],
) error {
	stagedServer, client, cleanup, errStage := xs.stageRemoteGameServerPaths(ctx, gameServer, request.Msg.GetFullFilePaths())
	if errStage != nil {
		return fileMutationError(errStage)
	}
	defer cleanup()

	errDestination := ensureStagedArchiveDestination(stagedServer.Directory, request.Msg.GetFullDestinationFilePath())
	if errDestination != nil {
		return fileMutationError(errDestination)
	}

	resultsChan := make(chan *xylona.GameServerFilesArchiveProgress, 32)
	go func() {
		for result := range resultsChan {
			if stream == nil || result == nil {
				continue
			}
			errSend := stream.Send(result)
			if errSend != nil {
				return
			}
		}
	}()

	lastResult, errArchive := xs.actionsInst.ArchiveFiles(
		ctx,
		stagedServer,
		request.Msg.GetFullDestinationFilePath(),
		request.Msg.GetFullFilePaths(),
		request.Msg.GetCompressionType(),
		resultsChan,
	)
	close(resultsChan)
	if errArchive != nil {
		return fileMutationError(errArchive)
	}

	archivePath := request.Msg.GetFullDestinationFilePath() + archiveCompressionExtension(request.Msg.GetCompressionType())
	errUpload := uploadStagedFileToNode(ctx, client, gameServer.Directory, stagedServer.Directory, archivePath)
	if errUpload != nil {
		return fileMutationError(errUpload)
	}

	if stream != nil && lastResult != nil {
		errSend := stream.Send(lastResult)
		if errSend != nil {
			return connect.NewError(connect.CodeInternal, errSend)
		}
	}

	return nil
}

func (xs *XylonaService) extractRemoteGameServerFiles(
	ctx context.Context,
	gameServer *models.GameServer,
	request *connect.Request[xylona.GameServerFilesDecompressionRequest],
	stream *connect.ServerStream[xylona.GameServerFilesExtractProgress],
) error {
	stagedServer, client, cleanup, errStage := xs.stageRemoteGameServerPaths(ctx, gameServer, []string{request.Msg.GetFullFilePath()})
	if errStage != nil {
		return fileMutationError(errStage)
	}
	defer cleanup()

	resultsChan := make(chan *xylona.GameServerFilesExtractProgress, 32)
	go func() {
		for result := range resultsChan {
			if stream == nil || result == nil {
				continue
			}
			errSend := stream.Send(result)
			if errSend != nil {
				return
			}
		}
	}()

	lastResult, errExtract := xs.actionsInst.ExtractFiles(
		ctx,
		stagedServer,
		request.Msg.GetFullFilePath(),
		request.Msg.GetDestinationBasePath(),
		resultsChan,
	)
	close(resultsChan)
	if errExtract != nil {
		return fileMutationError(errExtract)
	}

	errUpload := uploadStagedDirectoryTreeToNode(
		ctx,
		client,
		gameServer.Directory,
		stagedServer.Directory,
		request.Msg.GetDestinationBasePath(),
		request.Msg.GetFullFilePath(),
	)
	if errUpload != nil {
		return fileMutationError(errUpload)
	}

	if stream != nil && lastResult != nil {
		errSend := stream.Send(lastResult)
		if errSend != nil {
			return connect.NewError(connect.CodeInternal, errSend)
		}
	}

	return nil
}

func (xs *XylonaService) compressRemoteGameServerFiles(
	ctx context.Context,
	gameServer *models.GameServer,
	request *connect.Request[xylona.GameServerFilesCompressionRequest],
) (*connect.Response[xylona.GameServerFilesCompressionResponse], error) {
	stagedServer, client, cleanup, errStage := xs.stageRemoteGameServerPaths(ctx, gameServer, request.Msg.GetFullFilePaths())
	if errStage != nil {
		return nil, fileMutationError(errStage)
	}
	defer cleanup()

	errDestination := ensureStagedArchiveDestination(stagedServer.Directory, request.Msg.GetFullDestinationFilePath())
	if errDestination != nil {
		return nil, fileMutationError(errDestination)
	}

	archivePath, errCompress := xs.actionsInst.ArchiveAndCompressFiles(
		ctx,
		stagedServer,
		request.Msg.GetFullDestinationFilePath(),
		request.Msg.GetFullFilePaths(),
		request.Msg.GetCompressionType(),
	)
	if errCompress != nil {
		return nil, fileMutationError(errCompress)
	}

	relativeArchivePath, errRelative := filepath.Rel(stagedServer.Directory, archivePath)
	if errRelative != nil {
		return nil, fileMutationError(fmt.Errorf("resolve staged archive path: %w", errRelative))
	}
	relativeArchivePath = filepath.ToSlash(relativeArchivePath)

	errUpload := uploadStagedFileToNode(ctx, client, gameServer.Directory, stagedServer.Directory, relativeArchivePath)
	if errUpload != nil {
		return nil, fileMutationError(errUpload)
	}

	return connect.NewResponse(&xylona.GameServerFilesCompressionResponse{FullFilePath: relativeArchivePath}), nil
}

func (xs *XylonaService) decompressRemoteGameServerFiles(
	ctx context.Context,
	gameServer *models.GameServer,
	request *connect.Request[xylona.GameServerFilesDecompressionRequest],
) (*connect.Response[xylona.GameServerFilesDecompressionResponse], error) {
	stagedServer, client, cleanup, errStage := xs.stageRemoteGameServerPaths(ctx, gameServer, []string{request.Msg.GetFullFilePath()})
	if errStage != nil {
		return nil, fileMutationError(errStage)
	}
	defer cleanup()

	extractedFiles, errDecompress := xs.actionsInst.ExtractArchive(
		ctx,
		stagedServer,
		request.Msg.GetFullFilePath(),
		request.Msg.GetDestinationBasePath(),
	)
	if errDecompress != nil {
		return nil, fileMutationError(errDecompress)
	}

	errUpload := uploadStagedDirectoryTreeToNode(
		ctx,
		client,
		gameServer.Directory,
		stagedServer.Directory,
		request.Msg.GetDestinationBasePath(),
		request.Msg.GetFullFilePath(),
	)
	if errUpload != nil {
		return nil, fileMutationError(errUpload)
	}

	return connect.NewResponse(&xylona.GameServerFilesDecompressionResponse{FullFilePaths: extractedFiles}), nil
}
