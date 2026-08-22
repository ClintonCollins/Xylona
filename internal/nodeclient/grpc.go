package nodeclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodetls"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1/nodeprotoconnect"
)

// AuthorizationHeader is the header the controller uses to present its
// shared secret to a node. Exposed so the node-side service can read the
// same constant when validating requests.
const AuthorizationHeader = "Authorization"

// authorizationScheme is the value prefix paired with AuthorizationHeader.
const authorizationScheme = "Bearer "

const (
	sevenDaysToDieReportedModsResponseLimit    = 3 << 20
	sevenDaysToDieSandboxSettingsResponseLimit = 10 << 20
)

// GRPCNodeClient is the controller-side NodeClient implementation that talks
// to a remote node over Connect-RPC. The transport TLS layer pins the node's
// self-signed cert fingerprint; application-layer auth is the bearer token.
type GRPCNodeClient struct {
	nodeID             string
	listenURL          string
	sharedSecret       string
	httpClient         *http.Client
	connectClient      nodeprotoconnect.NodeServiceClient
	reportedModsClient nodeprotoconnect.NodeServiceClient
	sandboxClient      nodeprotoconnect.NodeServiceClient
}

// NewGRPCClient constructs a remote NodeClient. listenURL is the node's HTTPS
// base URL ("https://host:port"); certFingerprint is the lowercase hex SHA-256
// fingerprint published by the node during bootstrap pairing; sharedSecret is
// the long-lived bearer token also exchanged during bootstrap.
func NewGRPCClient(nodeID string, listenURL string, certFingerprint string, sharedSecret string) (*GRPCNodeClient, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, errors.New("nodeclient: node ID is required")
	}
	listenURL = strings.TrimSpace(listenURL)
	if listenURL == "" {
		return nil, errors.New("nodeclient: listen URL is required")
	}
	sharedSecret = strings.TrimSpace(sharedSecret)
	if sharedSecret == "" {
		return nil, errors.New("nodeclient: shared secret is required")
	}

	httpClient, errClient := nodetls.NewPinnedTLSClient(certFingerprint)
	if errClient != nil {
		return nil, fmt.Errorf("nodeclient: build pinned client: %w", errClient)
	}

	connectClient := nodeprotoconnect.NewNodeServiceClient(
		httpClient,
		listenURL,
		connect.WithReadMaxBytes(32<<20),
	)
	reportedModsClient := nodeprotoconnect.NewNodeServiceClient(
		httpClient,
		listenURL,
		connect.WithReadMaxBytes(sevenDaysToDieReportedModsResponseLimit),
		connect.WithCodec(reportedModsProtoCodec{}),
	)
	sandboxClient := nodeprotoconnect.NewNodeServiceClient(
		httpClient,
		listenURL,
		connect.WithReadMaxBytes(sevenDaysToDieSandboxSettingsResponseLimit),
		connect.WithCodec(sandboxSettingsProtoCodec{}),
	)

	return &GRPCNodeClient{
		nodeID:             nodeID,
		listenURL:          listenURL,
		sharedSecret:       sharedSecret,
		httpClient:         httpClient,
		connectClient:      connectClient,
		reportedModsClient: reportedModsClient,
		sandboxClient:      sandboxClient,
	}, nil
}

// ID returns the configured node identifier.
func (c *GRPCNodeClient) ID() string {
	return c.nodeID
}

// Close releases idle TLS connections held by the underlying transport. Safe
// to call multiple times.
func (c *GRPCNodeClient) Close() error {
	transport, ok := c.httpClient.Transport.(*http.Transport)
	if ok {
		transport.CloseIdleConnections()
	}
	return nil
}

// authorize stamps the bearer token on req before it is dispatched.
func (c *GRPCNodeClient) authorize(header http.Header) {
	header.Set(AuthorizationHeader, authorizationScheme+c.sharedSecret)
}

// newReq is a small helper that wraps connect.NewRequest and stamps the
// bearer token. Centralizing it keeps every method below one liner-shorter.
func newReq[T any](c *GRPCNodeClient, msg *T) *connect.Request[T] {
	req := connect.NewRequest(msg)
	c.authorize(req.Header())
	return req
}

func (c *GRPCNodeClient) streamConnectClient() nodeprotoconnect.NodeServiceClient {
	streamHTTPClient := *c.httpClient
	streamHTTPClient.Timeout = 0
	return nodeprotoconnect.NewNodeServiceClient(
		&streamHTTPClient,
		c.listenURL,
		connect.WithReadMaxBytes(32<<20),
	)
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	return maps.Clone(values)
}

// StartProcess sends the StartProcess RPC. Callers should rely on StreamEvents,
// StreamConsoleOutput, and snapshots for lifecycle observation.
func (c *GRPCNodeClient) StartProcess(ctx context.Context, cfg node.ProcessConfig, status xylona.Status) error {
	msg := &nodeprotov1.StartProcessRequest{
		Id:                 cfg.ID,
		ExecutionId:        cfg.ExecutionID,
		Name:               cfg.Name,
		BaseCommand:        cfg.BaseCommand,
		Args:               append([]string(nil), cfg.Args...),
		WorkingDirectory:   cfg.WorkingDirectory,
		User:               cfg.User,
		NodeId:             cfg.NodeID,
		ServiceId:          cfg.ServiceID,
		StopTimeoutSeconds: int64(cfg.StopTimeout / time.Second),
		InitialStatus:      processStatusFromXylona(status),
		InternalCommand:    cfg.InternalCommand,
		InternalGameId:     cfg.InternalGameID,
		LaunchEnv:          cloneStringMap(cfg.LaunchEnv),
	}
	if cfg.InputTelnet != nil {
		if cfg.InputTelnet.Port < 0 || cfg.InputTelnet.Port > 65535 {
			return errors.New("nodeclient: start process: telnet port is outside the valid range")
		}
		msg.TelnetInput = &nodeprotov1.TelnetInput{
			Port:     int32(cfg.InputTelnet.Port),
			Password: cfg.InputTelnet.Password,
		}
	}
	if cfg.InputRCON != nil {
		if cfg.InputRCON.Port <= 0 || cfg.InputRCON.Port > 65535 {
			return errors.New("nodeclient: start process: RCON port is outside the valid range")
		}
		protocol, errProtocol := rconProtocolToProto(cfg.InputRCON.Protocol)
		if errProtocol != nil {
			return errProtocol
		}
		msg.RconInput = &nodeprotov1.RCONInput{
			Host:     cfg.InputRCON.Host,
			Port:     int32(cfg.InputRCON.Port),
			Password: cfg.InputRCON.Password,
			Protocol: protocol,
		}
	}
	if cfg.InputREST != nil {
		if cfg.InputREST.Port <= 0 || cfg.InputREST.Port > 65535 {
			return errors.New("nodeclient: start process: REST port is outside the valid range")
		}
		kind, errKind := restInputKindToProto(cfg.InputREST.Kind)
		if errKind != nil {
			return errKind
		}
		msg.RestInput = &nodeprotov1.RESTInput{
			Host:              cfg.InputREST.Host,
			Port:              int32(cfg.InputREST.Port),
			Kind:              kind,
			Password:          cfg.InputREST.Password,
			PreviousPasswords: slices.Clone(cfg.InputREST.PreviousPasswords),
		}
	}
	req := newReq(c, msg)

	_, errRPC := c.connectClient.StartProcess(ctx, req)
	if errRPC != nil {
		return translateError("start process", errRPC)
	}
	return nil
}

// StopProcess invokes the StopProcess RPC.
func (c *GRPCNodeClient) StopProcess(ctx context.Context, processID string, stopInputCommand string) error {
	req := newReq(c, &nodeprotov1.StopProcessRequest{
		ProcessId:        processID,
		StopInputCommand: stopInputCommand,
	})
	_, errRPC := c.connectClient.StopProcess(ctx, req)
	if errRPC != nil {
		return translateProcessError("stop process", errRPC)
	}
	return nil
}

// SendConsoleInput invokes the SendConsoleInput RPC.
func (c *GRPCNodeClient) SendConsoleInput(ctx context.Context, processID string, input string) error {
	req := newReq(c, &nodeprotov1.SendConsoleInputRequest{
		ProcessId: processID,
		Input:     input,
	})
	_, errRPC := c.connectClient.SendConsoleInput(ctx, req)
	if errRPC != nil {
		return translateConsoleInputError("send console input", errRPC)
	}
	return nil
}

// ReadConsoleBuffer invokes the ReadConsoleBuffer RPC.
func (c *GRPCNodeClient) ReadConsoleBuffer(ctx context.Context, processID string) (node.ConsoleChunk, error) {
	req := newReq(c, &nodeprotov1.ReadConsoleBufferRequest{ProcessId: processID})
	resp, errRPC := c.connectClient.ReadConsoleBuffer(ctx, req)
	if errRPC != nil {
		return node.ConsoleChunk{ProcessID: processID}, translateError("read console buffer", errRPC)
	}
	chunk := resp.Msg.GetChunk()
	if chunk == nil {
		return node.ConsoleChunk{ProcessID: processID}, errors.New("nodeclient: read console buffer: response chunk is missing")
	}
	return node.ConsoleChunk{
		ProcessID:   chunk.GetGameServerId(),
		Data:        chunk.GetText(),
		Sequence:    chunk.GetSequence(),
		ResetBuffer: chunk.GetResetBuffer(),
	}, nil
}

// StreamConsoleOutput subscribes to live console chunks for one process.
func (c *GRPCNodeClient) StreamConsoleOutput(ctx context.Context, processID string) (<-chan node.ConsoleChunk, error) {
	req := newReq(c, &nodeprotov1.StreamConsoleOutputRequest{
		ProcessId:    processID,
		ReplayBuffer: true,
	})
	stream, errOpen := c.streamConnectClient().StreamConsoleOutput(ctx, req)
	if errOpen != nil {
		return nil, translateError("stream console output", errOpen)
	}

	out := make(chan node.ConsoleChunk, 64)
	go func() {
		defer close(out)
		defer func() {
			errClose := stream.Close()
			if errClose != nil && ctx.Err() == nil {
				log.Debug().Err(errClose).Str("process_id", processID).
					Msg("Failed to close remote console stream")
			}
		}()

		for stream.Receive() {
			msg := stream.Msg()
			chunk := node.ConsoleChunk{
				ProcessID:   msg.GetGameServerId(),
				Data:        msg.GetText(),
				Sequence:    msg.GetSequence(),
				ResetBuffer: msg.GetResetBuffer(),
			}

			select {
			case <-ctx.Done():
				return
			case out <- chunk:
			}
		}
		errReceive := stream.Err()
		if errReceive != nil && ctx.Err() == nil {
			log.Warn().Err(errReceive).Str("process_id", processID).
				Msg("Remote console stream ended with an error; controller will resubscribe")
		}
	}()

	return out, nil
}

// ListFiles invokes the ListFiles RPC.
func (c *GRPCNodeClient) ListFiles(ctx context.Context, directory string, relativePath string) ([]node.FileEntry, error) {
	req := newReq(c, &nodeprotov1.ListFilesRequest{
		Directory:    directory,
		RelativePath: relativePath,
	})
	resp, errRPC := c.connectClient.ListFiles(ctx, req)
	if errRPC != nil {
		return nil, translateError("list files", errRPC)
	}
	entries := resp.Msg.GetEntries()
	out := make([]node.FileEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, fileEntryFromProto(entry))
	}
	return out, nil
}

// ReadFile invokes the ReadFile RPC.
func (c *GRPCNodeClient) ReadFile(ctx context.Context, directory string, relativePath string) ([]byte, error) {
	req := newReq(c, &nodeprotov1.ReadFileRequest{
		Directory:    directory,
		RelativePath: relativePath,
	})
	resp, errRPC := c.connectClient.ReadFile(ctx, req)
	if errRPC != nil {
		return nil, translateError("read file", errRPC)
	}
	return resp.Msg.GetContent(), nil
}

// StatFile invokes the StatFile RPC.
func (c *GRPCNodeClient) StatFile(ctx context.Context, directory string, relativePath string) (node.FileEntry, error) {
	req := newReq(c, &nodeprotov1.StatFileRequest{
		Directory:    directory,
		RelativePath: relativePath,
	})
	resp, errRPC := c.connectClient.StatFile(ctx, req)
	if errRPC != nil {
		return node.FileEntry{}, translateError("stat file", errRPC)
	}
	return fileEntryFromProto(resp.Msg.GetEntry()), nil
}

// StreamFile invokes the StreamFile RPC and exposes the response as an
// io.ReadCloser for callers that proxy node-resident content.
func (c *GRPCNodeClient) StreamFile(ctx context.Context, directory string, relativePath string) (io.ReadCloser, error) {
	req := newReq(c, &nodeprotov1.StreamFileRequest{
		Directory:    directory,
		RelativePath: relativePath,
	})
	stream, errOpen := c.streamConnectClient().StreamFile(ctx, req)
	if errOpen != nil {
		return nil, translateError("stream file", errOpen)
	}

	reader, writer := io.Pipe()
	go func() {
		var errStream error
		defer func() {
			errCloseStream := stream.Close()
			if errStream != nil {
				errClosePipe := writer.CloseWithError(errStream)
				if errClosePipe != nil {
					return
				}
				return
			}
			if errCloseStream != nil {
				errClosePipe := writer.CloseWithError(fmt.Errorf("nodeclient: stream file close: %w", errCloseStream))
				if errClosePipe != nil {
					return
				}
				return
			}
			errCloseWriter := writer.Close()
			if errCloseWriter != nil {
				return
			}
		}()

		for stream.Receive() {
			content := stream.Msg().GetContent()
			if len(content) == 0 {
				continue
			}
			_, errWrite := writer.Write(content)
			if errWrite != nil {
				errStream = fmt.Errorf("nodeclient: stream file pipe write: %w", errWrite)
				return
			}
		}

		errReceive := stream.Err()
		if errReceive != nil {
			errStream = translateError("stream file", errReceive)
		}
	}()
	return reader, nil
}

// WriteFile invokes the WriteFile RPC.
func (c *GRPCNodeClient) WriteFile(ctx context.Context, directory string, relativePath string, content []byte, policy node.ProtectionPolicy) error {
	req := newReq(c, &nodeprotov1.WriteFileRequest{
		Directory:        directory,
		RelativePath:     relativePath,
		Content:          content,
		ServerExecutable: policy.ServerExecutable,
		BaseCommand:      policy.BaseCommand,
	})
	_, errRPC := c.connectClient.WriteFile(ctx, req)
	if errRPC != nil {
		return translateError("write file", errRPC)
	}
	return nil
}

const streamWriteFileChunkBytes = 64 * 1024

// StreamWriteFile invokes the client-streaming StreamWriteFile RPC.
func (c *GRPCNodeClient) StreamWriteFile(ctx context.Context, directory string, relativePath string, reader io.Reader, policy node.ProtectionPolicy) (node.WriteFileResult, error) {
	stream := c.streamConnectClient().StreamWriteFile(ctx)
	c.authorize(stream.RequestHeader())

	errSend := stream.Send(&nodeprotov1.StreamWriteFileRequest{
		Directory:        directory,
		RelativePath:     relativePath,
		ServerExecutable: policy.ServerExecutable,
		BaseCommand:      policy.BaseCommand,
	})
	if errSend != nil {
		return node.WriteFileResult{}, translateError("stream write file", errSend)
	}

	if reader != nil {
		buf := make([]byte, streamWriteFileChunkBytes)
		for {
			n, errRead := reader.Read(buf)
			if n > 0 {
				content := append([]byte(nil), buf[:n]...)
				errSend = stream.Send(&nodeprotov1.StreamWriteFileRequest{Content: content})
				if errSend != nil {
					return node.WriteFileResult{}, translateError("stream write file", errSend)
				}
			}
			if errors.Is(errRead, io.EOF) {
				break
			}
			if errRead != nil {
				return node.WriteFileResult{}, fmt.Errorf("nodeclient: read stream write file content: %w", errRead)
			}
		}
	}

	resp, errRPC := stream.CloseAndReceive()
	if errRPC != nil {
		return node.WriteFileResult{}, translateError("stream write file", errRPC)
	}
	return node.WriteFileResult{
		BytesWritten: resp.Msg.GetBytesWritten(),
		SHA256:       resp.Msg.GetSha256(),
	}, nil
}

// CreateFileOrDirectory invokes the CreateFileOrDirectory RPC.
func (c *GRPCNodeClient) CreateFileOrDirectory(ctx context.Context, directory string, relativePath string, content string, isDirectory bool, policy node.ProtectionPolicy) error {
	req := newReq(c, &nodeprotov1.CreateFileOrDirectoryRequest{
		Directory:        directory,
		RelativePath:     relativePath,
		Content:          content,
		IsDirectory:      isDirectory,
		ServerExecutable: policy.ServerExecutable,
		BaseCommand:      policy.BaseCommand,
	})
	_, errRPC := c.connectClient.CreateFileOrDirectory(ctx, req)
	if errRPC != nil {
		return translateError("create file or directory", errRPC)
	}
	return nil
}

// DeleteFiles invokes the DeleteFiles RPC.
func (c *GRPCNodeClient) DeleteFiles(ctx context.Context, directory string, files []string, policy node.ProtectionPolicy) ([]string, error) {
	req := newReq(c, &nodeprotov1.DeleteFilesRequest{
		Directory:        directory,
		Files:            append([]string(nil), files...),
		ServerExecutable: policy.ServerExecutable,
		BaseCommand:      policy.BaseCommand,
	})
	resp, errRPC := c.connectClient.DeleteFiles(ctx, req)
	if errRPC != nil {
		return nil, translateError("delete files", errRPC)
	}
	return resp.Msg.GetDeleted(), nil
}

// RenameFile invokes the RenameFile RPC.
func (c *GRPCNodeClient) RenameFile(ctx context.Context, directory string, oldRelativePath string, newRelativePath string, policy node.ProtectionPolicy) (string, error) {
	req := newReq(c, &nodeprotov1.RenameFileRequest{
		Directory:        directory,
		OldRelativePath:  oldRelativePath,
		NewRelativePath:  newRelativePath,
		ServerExecutable: policy.ServerExecutable,
		BaseCommand:      policy.BaseCommand,
	})
	resp, errRPC := c.connectClient.RenameFile(ctx, req)
	if errRPC != nil {
		return "", translateError("rename file", errRPC)
	}
	return resp.Msg.GetNewRelativePath(), nil
}

// MoveFiles invokes the MoveFiles RPC.
func (c *GRPCNodeClient) MoveFiles(ctx context.Context, directory string, files []string, destination string, policy node.ProtectionPolicy) ([]string, error) {
	req := newReq(c, &nodeprotov1.MoveFilesRequest{
		Directory:        directory,
		Files:            append([]string(nil), files...),
		Destination:      destination,
		ServerExecutable: policy.ServerExecutable,
		BaseCommand:      policy.BaseCommand,
	})
	resp, errRPC := c.connectClient.MoveFiles(ctx, req)
	if errRPC != nil {
		return nil, translateError("move files", errRPC)
	}
	return resp.Msg.GetMoved(), nil
}

// CopyFiles invokes the CopyFiles RPC.
func (c *GRPCNodeClient) CopyFiles(ctx context.Context, directory string, operations []node.CopyFileOperation, policy node.ProtectionPolicy) ([]string, error) {
	protoOperations := make([]*nodeprotov1.CopyFileOperation, 0, len(operations))
	for _, operation := range operations {
		protoOperations = append(protoOperations, &nodeprotov1.CopyFileOperation{
			SourceRelativePath:      operation.SourceRelativePath,
			DestinationRelativePath: operation.DestinationRelativePath,
		})
	}

	req := newReq(c, &nodeprotov1.CopyFilesRequest{
		Directory:        directory,
		Operations:       protoOperations,
		ServerExecutable: policy.ServerExecutable,
		BaseCommand:      policy.BaseCommand,
	})
	resp, errRPC := c.connectClient.CopyFiles(ctx, req)
	if errRPC != nil {
		return nil, translateError("copy files", errRPC)
	}
	return resp.Msg.GetCopied(), nil
}

// DownloadFileFromURL invokes the DownloadFileFromURL RPC.
func (c *GRPCNodeClient) DownloadFileFromURL(ctx context.Context, directory string, rawURL string, destinationDirectoryPath string, integrity node.DownloadIntegrity, policy node.ProtectionPolicy) (node.DownloadFileResult, error) {
	req := newReq(c, &nodeprotov1.DownloadFileFromURLRequest{
		Directory:                directory,
		Url:                      rawURL,
		DestinationDirectoryPath: destinationDirectoryPath,
		ServerExecutable:         policy.ServerExecutable,
		BaseCommand:              policy.BaseCommand,
		ExpectedSize:             integrity.ExpectedSize,
		ExpectedSha256:           integrity.ExpectedSHA256,
		ExpectedSha1:             integrity.ExpectedSHA1,
	})
	resp, errRPC := c.connectClient.DownloadFileFromURL(ctx, req)
	if errRPC != nil {
		return node.DownloadFileResult{}, translateError("download file from URL", errRPC)
	}
	return node.DownloadFileResult{
		RelativePath:  resp.Msg.GetRelativePath(),
		BytesWritten:  resp.Msg.GetBytesWritten(),
		SHA256:        resp.Msg.GetSha256(),
		SHA1:          resp.Msg.GetSha1(),
		ExpectedMatch: resp.Msg.GetExpectedMatch(),
	}, nil
}

// CreateFileArchive invokes the CreateFileArchive RPC.
func (c *GRPCNodeClient) CreateFileArchive(ctx context.Context, directory string, destinationArchivePath string, includePaths []string, compression node.ArchiveCompression, policy node.ProtectionPolicy) (string, node.ArchiveProgress, error) {
	return c.CreateFileArchiveWithProgress(ctx, directory, destinationArchivePath, includePaths, compression, policy, nil)
}

// CreateFileArchiveWithProgress invokes the streaming CreateFileArchive RPC.
func (c *GRPCNodeClient) CreateFileArchiveWithProgress(ctx context.Context, directory string, destinationArchivePath string, includePaths []string, compression node.ArchiveCompression, policy node.ProtectionPolicy, onProgress func(node.ArchiveProgress) error) (string, node.ArchiveProgress, error) {
	req := newReq(c, &nodeprotov1.CreateFileArchiveRequest{
		Directory:              directory,
		DestinationArchivePath: destinationArchivePath,
		IncludePaths:           append([]string(nil), includePaths...),
		Compression:            archiveCompressionToProto(compression),
		ServerExecutable:       policy.ServerExecutable,
		BaseCommand:            policy.BaseCommand,
	})
	stream, errRPC := c.streamConnectClient().StreamCreateFileArchive(ctx, req)
	if errRPC != nil {
		return "", node.ArchiveProgress{}, translateError("create file archive", errRPC)
	}

	archivePath := ""
	var progress node.ArchiveProgress
	for stream.Receive() {
		msg := stream.Msg()
		progress = archiveProgressFromProto(msg)
		if msg.GetRelativePath() != "" {
			archivePath = msg.GetRelativePath()
		}
		if onProgress != nil {
			errProgress := onProgress(progress)
			if errProgress != nil {
				return "", node.ArchiveProgress{}, fmt.Errorf("nodeclient: create file archive progress: %w", errProgress)
			}
		}
	}
	errStream := stream.Err()
	if errStream != nil {
		return "", node.ArchiveProgress{}, translateError("create file archive stream", errStream)
	}
	if archivePath == "" {
		return "", node.ArchiveProgress{}, errors.New("nodeclient: create file archive stream ended without archive path")
	}
	return archivePath, progress, nil
}

// ExtractFileArchive invokes the ExtractFileArchive RPC.
func (c *GRPCNodeClient) ExtractFileArchive(ctx context.Context, directory string, archivePath string, destinationDirectoryPath string, policy node.ProtectionPolicy) ([]string, node.ExtractProgress, error) {
	return c.ExtractFileArchiveWithProgress(ctx, directory, archivePath, destinationDirectoryPath, policy, nil)
}

// ExtractFileArchiveWithProgress invokes the streaming ExtractFileArchive RPC.
func (c *GRPCNodeClient) ExtractFileArchiveWithProgress(ctx context.Context, directory string, archivePath string, destinationDirectoryPath string, policy node.ProtectionPolicy, onProgress func(node.ExtractProgress) error) ([]string, node.ExtractProgress, error) {
	req := newReq(c, &nodeprotov1.ExtractFileArchiveRequest{
		Directory:                directory,
		ArchivePath:              archivePath,
		DestinationDirectoryPath: destinationDirectoryPath,
		ServerExecutable:         policy.ServerExecutable,
		BaseCommand:              policy.BaseCommand,
	})
	stream, errRPC := c.streamConnectClient().StreamExtractFileArchive(ctx, req)
	if errRPC != nil {
		return nil, node.ExtractProgress{}, translateError("extract file archive", errRPC)
	}

	var extractedPaths []string
	var progress node.ExtractProgress
	for stream.Receive() {
		msg := stream.Msg()
		progress = extractProgressFromProto(msg)
		if len(msg.GetExtractedPaths()) > 0 {
			extractedPaths = append([]string(nil), msg.GetExtractedPaths()...)
		}
		if onProgress != nil {
			errProgress := onProgress(progress)
			if errProgress != nil {
				return nil, node.ExtractProgress{}, fmt.Errorf("nodeclient: extract file archive progress: %w", errProgress)
			}
		}
	}
	errStream := stream.Err()
	if errStream != nil {
		return nil, node.ExtractProgress{}, translateError("extract file archive stream", errStream)
	}
	return extractedPaths, progress, nil
}

// CreateBackupArchive invokes the CreateBackupArchive RPC.
func (c *GRPCNodeClient) CreateBackupArchive(ctx context.Context, directory string, includePaths []string, destinationArchivePath string) (int64, string, error) {
	req := newReq(c, &nodeprotov1.CreateBackupArchiveRequest{
		Directory:              directory,
		IncludePaths:           append([]string(nil), includePaths...),
		DestinationArchivePath: destinationArchivePath,
	})
	resp, errRPC := c.connectClient.CreateBackupArchive(ctx, req)
	if errRPC != nil {
		return 0, "", translateError("create backup archive", errRPC)
	}
	return resp.Msg.GetArchiveBytes(), resp.Msg.GetArchiveSha256(), nil
}

// ExtractBackupArchive invokes the ExtractBackupArchive RPC.
func (c *GRPCNodeClient) ExtractBackupArchive(ctx context.Context, directory string, archivePath string, mode node.ExtractMode) error {
	req := newReq(c, &nodeprotov1.ExtractBackupArchiveRequest{
		Directory:   directory,
		ArchivePath: archivePath,
		Mode:        extractModeToProto(mode),
	})
	_, errRPC := c.connectClient.ExtractBackupArchive(ctx, req)
	if errRPC != nil {
		return translateError("extract backup archive", errRPC)
	}
	return nil
}

// ProbeInstalledVersion invokes the ProbeInstalledVersion RPC.
func (c *GRPCNodeClient) ProbeInstalledVersion(ctx context.Context, probe node.InstalledVersionProbeRequest) (node.InstalledVersionProbeResult, error) {
	req := newReq(c, &nodeprotov1.ProbeInstalledVersionRequest{
		Directory:           probe.Directory,
		Kind:                installedVersionProbeKindToProto(probe.Kind),
		RelativePaths:       append([]string(nil), probe.RelativePaths...),
		PreferredSteamAppId: probe.PreferredSteamAppID,
	})
	resp, errRPC := c.connectClient.ProbeInstalledVersion(ctx, req)
	if errRPC != nil {
		return node.InstalledVersionProbeResult{}, translateError("probe installed version", errRPC)
	}
	return node.InstalledVersionProbeResult{
		Found:      resp.Msg.GetFound(),
		Version:    resp.Msg.GetVersion(),
		SourcePath: resp.Msg.GetSourcePath(),
	}, nil
}

// QueryGameServer invokes the QueryGameServer RPC.
func (c *GRPCNodeClient) QueryGameServer(ctx context.Context, queryReq node.GameServerQueryRequest) (node.GameServerQueryResult, error) {
	req := newReq(c, &nodeprotov1.QueryGameServerRequest{
		Kind:       gameServerQueryKindToProto(queryReq.Kind),
		Ip:         queryReq.IP,
		QueryPort:  queryReq.QueryPort,
		MaxPlayers: queryReq.MaxPlayers,
		Username:   queryReq.Username,
		Password:   queryReq.Password,
	})
	resp, errRPC := c.connectClient.QueryGameServer(ctx, req)
	if errRPC != nil {
		return node.GameServerQueryResult{}, translateError("query game server", errRPC)
	}
	return node.GameServerQueryResult{
		Kind:      gameServerQueryKindFromProto(resp.Msg.GetKind()),
		Minecraft: minecraftQueryFromProto(resp.Msg.GetMinecraft()),
		Source:    sourceQueryFromProto(resp.Msg.GetSource()),
		Palworld:  palworldQueryFromProto(resp.Msg.GetPalworld()),
	}, nil
}

// QueryPalworldMap invokes the dedicated sanitized live-map RPC.
func (c *GRPCNodeClient) QueryPalworldMap(ctx context.Context, mapReq node.PalworldMapQueryRequest) (*node.PalworldMapSnapshot, error) {
	req := newReq(c, &nodeprotov1.QueryPalworldMapRequest{
		Ip:        mapReq.IP,
		QueryPort: mapReq.QueryPort,
		Username:  mapReq.Username,
		Password:  mapReq.Password,
	})
	resp, errRPC := c.connectClient.QueryPalworldMap(ctx, req)
	if errRPC != nil {
		return nil, translateError("query palworld map", errRPC)
	}
	return palworldMapSnapshotFromProto(resp.Msg.GetSnapshot()), nil
}

// QuerySevenDaysToDieMap invokes the dedicated native-map RPC.
func (c *GRPCNodeClient) QuerySevenDaysToDieMap(ctx context.Context, mapReq node.SevenDaysToDieMapQueryRequest) (*node.SevenDaysToDieMapSnapshot, error) {
	req := newReq(c, &nodeprotov1.QuerySevenDaysToDieMapRequest{
		WorkingDirectory: mapReq.WorkingDirectory,
		TokenName:        mapReq.TokenName,
		TokenSecret:      mapReq.TokenSecret,
	})
	resp, errRPC := c.connectClient.QuerySevenDaysToDieMap(ctx, req)
	if errRPC != nil {
		return nil, translateError("query 7 Days to Die map", errRPC)
	}
	return sevenDaysToDieMapSnapshotFromProto(resp.Msg.GetSnapshot()), nil
}

// QuerySevenDaysToDieWebAPIStatus invokes the bounded native WebAPI diagnostics RPC.
func (c *GRPCNodeClient) QuerySevenDaysToDieWebAPIStatus(ctx context.Context, statusReq node.SevenDaysToDieWebAPIStatusQueryRequest) (*node.SevenDaysToDieWebAPIStatus, error) {
	req := newReq(c, &nodeprotov1.QuerySevenDaysToDieWebAPIStatusRequest{
		WorkingDirectory: statusReq.WorkingDirectory,
		TokenName:        statusReq.TokenName,
		TokenSecret:      statusReq.TokenSecret,
	})
	resp, errRPC := c.connectClient.QuerySevenDaysToDieWebAPIStatus(ctx, req)
	if errRPC != nil {
		return nil, translateError("query 7 Days to Die WebAPI status", errRPC)
	}
	return sevenDaysToDieWebAPIStatusFromProto(resp.Msg.GetStatus()), nil
}

// QuerySevenDaysToDiePlayers invokes the private native player-roster RPC.
func (c *GRPCNodeClient) QuerySevenDaysToDiePlayers(ctx context.Context, playersReq node.SevenDaysToDiePlayersQueryRequest) (*node.SevenDaysToDiePlayers, error) {
	req := newReq(c, &nodeprotov1.QuerySevenDaysToDiePlayersRequest{
		WorkingDirectory: playersReq.WorkingDirectory,
		TokenName:        playersReq.TokenName,
		TokenSecret:      playersReq.TokenSecret,
	})
	resp, errRPC := c.connectClient.QuerySevenDaysToDiePlayers(ctx, req)
	if errRPC != nil {
		return nil, translateError("query 7 Days to Die players", errRPC)
	}
	return sevenDaysToDiePlayersFromProto(resp.Msg.GetResult()), nil
}

// QuerySevenDaysToDieReportedMods invokes the private native reported-mod RPC.
func (c *GRPCNodeClient) QuerySevenDaysToDieReportedMods(ctx context.Context, modsReq node.SevenDaysToDieReportedModsQueryRequest) (*node.SevenDaysToDieReportedMods, error) {
	req := newReq(c, &nodeprotov1.QuerySevenDaysToDieReportedModsRequest{
		WorkingDirectory: modsReq.WorkingDirectory,
		TokenName:        modsReq.TokenName,
		TokenSecret:      modsReq.TokenSecret,
	})
	resp, errRPC := c.reportedModsClient.QuerySevenDaysToDieReportedMods(ctx, req)
	if errRPC != nil {
		return nil, translateError("query 7 Days to Die reported mods", errRPC)
	}
	result, errResult := sevenDaysToDieReportedModsFromProto(resp.Msg.GetResult())
	if errResult != nil {
		return nil, fmt.Errorf("query 7 Days to Die reported mods: invalid node response: %w", errResult)
	}
	return result, nil
}

// QuerySevenDaysToDieSandboxSettings invokes the private native sandbox-settings RPC.
func (c *GRPCNodeClient) QuerySevenDaysToDieSandboxSettings(ctx context.Context, sandboxReq node.SevenDaysToDieSandboxSettingsQueryRequest) (*node.SevenDaysToDieSandboxSettings, error) {
	req := newReq(c, &nodeprotov1.QuerySevenDaysToDieSandboxSettingsRequest{
		WorkingDirectory: sandboxReq.WorkingDirectory,
		TokenName:        sandboxReq.TokenName,
		TokenSecret:      sandboxReq.TokenSecret,
	})
	resp, errRPC := c.sandboxClient.QuerySevenDaysToDieSandboxSettings(ctx, req)
	if errRPC != nil {
		return nil, translateError("query 7 Days to Die sandbox settings", errRPC)
	}
	result, errResult := sevenDaysToDieSandboxSettingsFromProto(resp.Msg.GetResult())
	if errResult != nil {
		return nil, fmt.Errorf("query 7 Days to Die sandbox settings: invalid node response: %w", errResult)
	}
	return result, nil
}

// GetSevenDaysToDieMapTile invokes the bounded native-tile RPC.
func (c *GRPCNodeClient) GetSevenDaysToDieMapTile(ctx context.Context, tileReq node.SevenDaysToDieMapTileRequest) ([]byte, error) {
	req := newReq(c, &nodeprotov1.GetSevenDaysToDieMapTileRequest{
		WorkingDirectory: tileReq.WorkingDirectory,
		TokenName:        tileReq.TokenName,
		TokenSecret:      tileReq.TokenSecret,
		Zoom:             tileReq.Zoom,
		X:                tileReq.X,
		Y:                tileReq.Y,
	})
	resp, errRPC := c.connectClient.GetSevenDaysToDieMapTile(ctx, req)
	if errRPC != nil {
		return nil, translateError("get 7 Days to Die map tile", errRPC)
	}
	return append([]byte(nil), resp.Msg.GetContent()...), nil
}

// EnsureMinecraftMap invokes the node-local BlueMap lifecycle RPC.
func (c *GRPCNodeClient) EnsureMinecraftMap(ctx context.Context, mapReq node.MinecraftMapEnsureRequest) (node.MinecraftMapStatus, error) {
	req := newReq(c, &nodeprotov1.EnsureMinecraftMapRequest{
		ProcessId:        mapReq.ProcessID,
		WorkingDirectory: mapReq.WorkingDirectory,
		WorldName:        mapReq.WorldName,
		JavaExecutable:   mapReq.JavaExecutable,
		MinecraftVersion: mapReq.MinecraftVersion,
	})
	resp, errRPC := c.connectClient.EnsureMinecraftMap(ctx, req)
	if errRPC != nil {
		return node.MinecraftMapStatus{}, translateError("ensure Minecraft map", errRPC)
	}
	message := resp.Msg
	return node.MinecraftMapStatus{
		Installed:            message.GetInstalled(),
		Running:              message.GetRunning(),
		Ready:                message.GetReady(),
		Provider:             message.GetProvider(),
		Status:               message.GetStatus(),
		StatusMessage:        message.GetStatusMessage(),
		BlueMapVersion:       message.GetBluemapVersion(),
		LivePlayersAvailable: message.GetLivePlayersAvailable(),
	}, nil
}

// StopMinecraftMap invokes the node-local BlueMap stop RPC.
func (c *GRPCNodeClient) StopMinecraftMap(ctx context.Context, processID string) error {
	req := newReq(c, &nodeprotov1.StopMinecraftMapRequest{ProcessId: processID})
	_, errRPC := c.connectClient.StopMinecraftMap(ctx, req)
	if errRPC != nil {
		return translateProcessError("stop Minecraft map", errRPC)
	}
	return nil
}

// GetMinecraftMapAsset reads one bounded BlueMap web asset over Connect-RPC.
func (c *GRPCNodeClient) GetMinecraftMapAsset(ctx context.Context, mapReq node.MinecraftMapAssetRequest) (node.MinecraftMapAsset, error) {
	req := newReq(c, &nodeprotov1.GetMinecraftMapAssetRequest{
		ProcessId:        mapReq.ProcessID,
		WorkingDirectory: mapReq.WorkingDirectory,
		AssetPath:        mapReq.AssetPath,
	})
	resp, errRPC := c.connectClient.GetMinecraftMapAsset(ctx, req)
	if errRPC != nil {
		return node.MinecraftMapAsset{}, translateError("get Minecraft map asset", errRPC)
	}
	return node.MinecraftMapAsset{
		Content:         append([]byte(nil), resp.Msg.GetContent()...),
		ContentType:     resp.Msg.GetContentType(),
		ContentEncoding: resp.Msg.GetContentEncoding(),
		CacheControl:    resp.Msg.GetCacheControl(),
	}, nil
}

// PerformGameServerPlayerAction invokes the typed player-administration RPC.
func (c *GRPCNodeClient) PerformGameServerPlayerAction(ctx context.Context, actionReq node.GameServerPlayerActionRequest) error {
	req := newReq(c, &nodeprotov1.PerformGameServerPlayerActionRequest{
		Kind:      gameServerQueryKindToProto(actionReq.Kind),
		Action:    gameServerPlayerActionToProto(actionReq.Action),
		ProcessId: actionReq.ProcessID,
		Ip:        actionReq.IP,
		QueryPort: actionReq.QueryPort,
		Username:  actionReq.Username,
		Password:  actionReq.Password,
		PlayerId:  actionReq.PlayerID,
		Reason:    actionReq.Reason,
	})
	_, errRPC := c.connectClient.PerformGameServerPlayerAction(ctx, req)
	if errRPC != nil {
		return translatePlayerActionError("perform game server player action", errRPC)
	}
	return nil
}

// SendConsoleOutput invokes the SendConsoleOutput RPC.
func (c *GRPCNodeClient) SendConsoleOutput(ctx context.Context, processID, line string) error {
	req := newReq(c, &nodeprotov1.SendConsoleOutputRequest{
		ProcessId: processID,
		Line:      line,
	})
	_, errRPC := c.connectClient.SendConsoleOutput(ctx, req)
	if errRPC != nil {
		return translateError("send console output", errRPC)
	}
	return nil
}

// GetProcessSnapshot invokes the GetProcessSnapshot RPC.
func (c *GRPCNodeClient) GetProcessSnapshot(ctx context.Context, processID string) (*node.ProcessSnapshot, bool, error) {
	req := newReq(c, &nodeprotov1.GetProcessSnapshotRequest{ProcessId: processID})
	resp, errRPC := c.connectClient.GetProcessSnapshot(ctx, req)
	if errRPC != nil {
		return nil, false, translateError("get process snapshot", errRPC)
	}
	if !resp.Msg.GetFound() {
		return nil, false, nil
	}
	return processSnapshotFromProto(resp.Msg.GetSnapshot()), true, nil
}

func archiveProgressFromProto(msg *nodeprotov1.CreateFileArchiveResponse) node.ArchiveProgress {
	if msg == nil {
		return node.ArchiveProgress{}
	}
	return node.ArchiveProgress{
		TotalFiles:      msg.GetTotalFiles(),
		FilesCompressed: msg.GetFilesCompressed(),
		TotalBytes:      msg.GetTotalBytes(),
		BytesCompressed: msg.GetBytesCompressed(),
		CurrentFile:     msg.GetCurrentFile(),
	}
}

func extractProgressFromProto(msg *nodeprotov1.ExtractFileArchiveResponse) node.ExtractProgress {
	if msg == nil {
		return node.ExtractProgress{}
	}
	return node.ExtractProgress{
		TotalFiles:     msg.GetTotalFiles(),
		FilesExtracted: msg.GetFilesExtracted(),
		TotalBytes:     msg.GetTotalBytes(),
		BytesExtracted: msg.GetBytesExtracted(),
		CurrentFile:    msg.GetCurrentFile(),
	}
}

func archiveCompressionToProto(compression node.ArchiveCompression) nodeprotov1.FileArchiveCompression {
	switch compression {
	case node.ArchiveCompressionBZIP2:
		return nodeprotov1.FileArchiveCompression_FILE_ARCHIVE_COMPRESSION_BZIP2
	case node.ArchiveCompressionGZIP:
		return nodeprotov1.FileArchiveCompression_FILE_ARCHIVE_COMPRESSION_GZIP
	case node.ArchiveCompressionZST:
		return nodeprotov1.FileArchiveCompression_FILE_ARCHIVE_COMPRESSION_ZST
	case node.ArchiveCompressionXZ:
		return nodeprotov1.FileArchiveCompression_FILE_ARCHIVE_COMPRESSION_XZ
	default:
		return nodeprotov1.FileArchiveCompression_FILE_ARCHIVE_COMPRESSION_ZIP
	}
}

func extractModeToProto(mode node.ExtractMode) nodeprotov1.ExtractMode {
	switch mode {
	case node.ExtractModeExact:
		return nodeprotov1.ExtractMode_EXTRACT_MODE_EXACT
	default:
		return nodeprotov1.ExtractMode_EXTRACT_MODE_OVERLAY
	}
}

func installedVersionProbeKindToProto(kind node.InstalledVersionProbeKind) nodeprotov1.InstalledVersionProbeKind {
	switch kind {
	case node.InstalledVersionProbeKindMinecraftJar:
		return nodeprotov1.InstalledVersionProbeKind_INSTALLED_VERSION_PROBE_KIND_MINECRAFT_JAR
	case node.InstalledVersionProbeKindSteamManifest:
		return nodeprotov1.InstalledVersionProbeKind_INSTALLED_VERSION_PROBE_KIND_STEAM_MANIFEST
	default:
		return nodeprotov1.InstalledVersionProbeKind_INSTALLED_VERSION_PROBE_KIND_UNSPECIFIED
	}
}

func gameServerQueryKindToProto(kind node.GameServerQueryKind) nodeprotov1.GameServerQueryKind {
	switch kind {
	case node.GameServerQueryKindMinecraft:
		return nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_MINECRAFT
	case node.GameServerQueryKindSource:
		return nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_SOURCE
	case node.GameServerQueryKindPalworld:
		return nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_PALWORLD
	case node.GameServerQueryKindSevenDaysToDie:
		return nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_SEVEN_DAYS_TO_DIE
	case node.GameServerQueryKindFactorio:
		return nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_FACTORIO
	case node.GameServerQueryKindHytale:
		return nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_HYTALE
	case node.GameServerQueryKindProjectZomboid:
		return nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_PROJECT_ZOMBOID
	case node.GameServerQueryKindTerraria:
		return nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_TERRARIA
	case node.GameServerQueryKindSourceRCON:
		return nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_SOURCE_RCON
	case node.GameServerQueryKindRust:
		return nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_RUST
	default:
		return nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_UNSPECIFIED
	}
}

func gameServerQueryKindFromProto(kind nodeprotov1.GameServerQueryKind) node.GameServerQueryKind {
	switch kind {
	case nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_MINECRAFT:
		return node.GameServerQueryKindMinecraft
	case nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_SOURCE:
		return node.GameServerQueryKindSource
	case nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_PALWORLD:
		return node.GameServerQueryKindPalworld
	case nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_SEVEN_DAYS_TO_DIE:
		return node.GameServerQueryKindSevenDaysToDie
	case nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_FACTORIO:
		return node.GameServerQueryKindFactorio
	case nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_HYTALE:
		return node.GameServerQueryKindHytale
	case nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_PROJECT_ZOMBOID:
		return node.GameServerQueryKindProjectZomboid
	case nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_TERRARIA:
		return node.GameServerQueryKindTerraria
	case nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_SOURCE_RCON:
		return node.GameServerQueryKindSourceRCON
	case nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_RUST:
		return node.GameServerQueryKindRust
	default:
		return node.GameServerQueryKindUnknown
	}
}

func gameServerPlayerActionToProto(action node.GameServerPlayerAction) nodeprotov1.GameServerPlayerAction {
	switch action {
	case node.GameServerPlayerActionKick:
		return nodeprotov1.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_KICK
	case node.GameServerPlayerActionBan:
		return nodeprotov1.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_BAN
	case node.GameServerPlayerActionUnban:
		return nodeprotov1.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_UNBAN
	case node.GameServerPlayerActionAllowlistAdd:
		return nodeprotov1.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_ALLOWLIST_ADD
	case node.GameServerPlayerActionAllowlistRemove:
		return nodeprotov1.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_ALLOWLIST_REMOVE
	default:
		return nodeprotov1.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_UNSPECIFIED
	}
}

func minecraftQueryFromProto(info *nodeprotov1.GameServerMinecraftQueryInfo) *node.MinecraftQueryInfo {
	if info == nil {
		return nil
	}
	return &node.MinecraftQueryInfo{
		MOTD:                info.GetMotd(),
		GameType:            info.GetGameType(),
		Map:                 info.GetMap(),
		NumberOfPlayers:     info.GetNumberOfPlayers(),
		MaxPlayers:          info.GetMaxPlayers(),
		PlayerList:          append([]string(nil), info.GetPlayerList()...),
		ProtocolVersion:     info.GetProtocolVersion(),
		ServerVersion:       info.GetServerVersion(),
		PlayerDetails:       gameServerPlayersFromProto(info.GetPlayerDetails()),
		PlayerListSupported: info.GetPlayerListSupported(),
		Responded:           optionalBoolOrDefault(info.Responded, true),
	}
}

func sourceQueryFromProto(info *nodeprotov1.GameServerSourceQueryInfo) *node.SourceQueryInfo {
	if info == nil {
		return nil
	}
	return &node.SourceQueryInfo{
		Name:                info.GetName(),
		Map:                 info.GetMap(),
		Game:                info.GetGame(),
		AppID:               info.GetAppId(),
		SteamID:             info.GetSteamId(),
		GameID:              info.GetGameId(),
		Players:             info.GetPlayers(),
		MaxPlayers:          info.GetMaxPlayers(),
		Bots:                info.GetBots(),
		ServerOS:            info.GetServerOs(),
		Visibility:          info.GetVisibility(),
		VAC:                 info.GetVac(),
		Version:             info.GetVersion(),
		Protocol:            info.GetProtocol(),
		PlayerList:          append([]string(nil), info.GetPlayerList()...),
		PlayerListSupported: info.GetPlayerListSupported(),
		Responded:           optionalBoolOrDefault(info.Responded, true),
	}
}

func palworldQueryFromProto(info *nodeprotov1.GameServerPalworldQueryInfo) *node.PalworldQueryInfo {
	if info == nil {
		return nil
	}
	return &node.PalworldQueryInfo{
		Name:              info.GetName(),
		Description:       info.GetDescription(),
		Version:           info.GetVersion(),
		WorldGUID:         info.GetWorldGuid(),
		Players:           info.GetPlayers(),
		MaxPlayers:        info.GetMaxPlayers(),
		PlayerList:        append([]string(nil), info.GetPlayerList()...),
		UptimeSeconds:     info.GetUptimeSeconds(),
		ServerFPS:         info.GetServerFps(),
		ServerFrameTimeMS: info.GetServerFrameTimeMs(),
		Days:              info.GetDays(),
		Responded:         info.GetResponded(),
		PlayerDetails:     gameServerPlayersFromProto(info.GetPlayerDetails()),
	}
}

func gameServerPlayersFromProto(players []*nodeprotov1.GameServerPlayer) []node.GameServerPlayer {
	result := make([]node.GameServerPlayer, 0, len(players))
	for _, player := range players {
		if player == nil {
			continue
		}
		result = append(result, node.GameServerPlayer{
			Name: player.GetName(),
			ID:   player.GetId(),
		})
	}
	return result
}

func fileEntryFromProto(entry *nodeprotov1.FileEntry) node.FileEntry {
	if entry == nil {
		return node.FileEntry{}
	}
	var lastModified time.Time
	timestamp := entry.GetLastModified()
	if timestamp != nil {
		lastModified = timestamp.AsTime()
	}
	return node.FileEntry{
		Name:         entry.GetName(),
		Size:         entry.GetSize(),
		IsDirectory:  entry.GetIsDirectory(),
		IsExecutable: entry.GetIsExecutable(),
		LastModified: lastModified,
	}
}

// processSnapshotFromProto converts a v1.ProcessSnapshot into the Go domain
// ProcessSnapshot. Used by GetProcessSnapshot and the process list inside
// nodeSnapshotFromProto (indirectly).
func processSnapshotFromProto(p *nodeprotov1.ProcessSnapshot) *node.ProcessSnapshot {
	if p == nil {
		return nil
	}
	status := p.GetStatus()
	legacyMetricsValid := legacyProcessMetricsValid(status)
	out := &node.ProcessSnapshot{
		ID:                   p.GetId(),
		ExecutionID:          p.GetExecutionId(),
		Name:                 p.GetName(),
		Status:               status,
		PreviousStatus:       p.GetPreviousStatus(),
		TransitionSequence:   p.GetTransitionSequence(),
		IntentionalStop:      p.GetIntentionalStop(),
		UnixStartedAt:        p.GetUnixStartedAt(),
		CPUPercent:           p.GetCpuPercent(),
		CPUValid:             optionalBoolOrDefault(p.CpuValid, legacyMetricsValid),
		MetricsValid:         optionalBoolOrDefault(p.MetricsValid, legacyMetricsValid),
		CPUCores:             p.GetCpuCores(),
		MemoryRSS:            p.GetMemoryRss(),
		MemoryVMS:            p.GetMemoryVms(),
		MemoryPercent:        p.GetMemoryPercent(),
		NumThreads:           p.GetNumThreads(),
		DiskUsageBytes:       p.GetDiskUsageBytes(),
		DiskTotalBytes:       p.GetDiskTotalBytes(),
		DiskFreeBytes:        p.GetDiskFreeBytes(),
		DiskPercent:          p.GetDiskPercent(),
		DiskValid:            optionalBoolOrDefault(p.DiskValid, legacyMetricsValid),
		IOValid:              optionalBoolOrDefault(p.IoValid, legacyMetricsValid),
		IOReadRate:           p.GetIoReadRate(),
		IOWriteRate:          p.GetIoWriteRate(),
		ConnectionCount:      p.GetConnectionCount(),
		ConnectionCountValid: optionalBoolOrDefault(p.ConnectionCountValid, legacyMetricsValid),
		WorkingDir:           p.GetWorkingDir(),
	}
	diskMeasuredAt := p.GetDiskMeasuredAt()
	if diskMeasuredAt != nil {
		out.DiskMeasuredAt = diskMeasuredAt.AsTime()
	}
	if p.ExitCode != nil {
		out.ExitCode = int(p.GetExitCode())
		out.ExitCodeKnown = true
	}
	return out
}

func optionalBoolOrDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func legacyProcessMetricsValid(status string) bool {
	switch status {
	case xylona.Status_ONLINE.String(), xylona.Status_INSTALLING.String(), xylona.Status_UPDATING.String():
		return true
	default:
		return false
	}
}

// GetNodeSnapshot invokes the GetNodeSnapshot RPC.
func (c *GRPCNodeClient) GetNodeSnapshot(ctx context.Context) (*node.NodeSnapshot, error) {
	req := newReq(c, &nodeprotov1.GetNodeSnapshotRequest{})
	resp, errRPC := c.connectClient.GetNodeSnapshot(ctx, req)
	if errRPC != nil {
		return nil, translateError("get node snapshot", errRPC)
	}
	return nodeSnapshotFromProto(resp.Msg), nil
}

// ListBindableIPs invokes the ListBindableIPs RPC.
func (c *GRPCNodeClient) ListBindableIPs(ctx context.Context) ([]node.BindableIP, error) {
	req := newReq(c, &nodeprotov1.ListBindableIPsRequest{})
	resp, errRPC := c.connectClient.ListBindableIPs(ctx, req)
	if errRPC != nil {
		return nil, translateError("list bindable IPs", errRPC)
	}

	ipProtos := resp.Msg.GetIps()
	ips := make([]node.BindableIP, 0, len(ipProtos))
	for _, ipProto := range ipProtos {
		ips = append(ips, node.BindableIP{
			Address:  ipProto.GetAddress(),
			Usable:   ipProto.GetUsable(),
			External: ipProto.GetExternal(),
		})
	}
	return ips, nil
}

// GetUpdateCapabilities invokes the GetUpdateCapabilities RPC.
func (c *GRPCNodeClient) GetUpdateCapabilities(ctx context.Context) (node.UpdateCapabilities, error) {
	req := newReq(c, &nodeprotov1.GetUpdateCapabilitiesRequest{})
	resp, errRPC := c.connectClient.GetUpdateCapabilities(ctx, req)
	if errRPC != nil {
		return node.UpdateCapabilities{}, translateError("get update capabilities", errRPC)
	}
	msg := resp.Msg
	return node.UpdateCapabilities{
		Supported:               msg.GetSupported(),
		Reason:                  msg.GetReason(),
		Component:               msg.GetComponent(),
		CurrentVersion:          msg.GetCurrentVersion(),
		OS:                      msg.GetOs(),
		Architecture:            msg.GetArchitecture(),
		ProtocolVersion:         msg.GetProtocolVersion(),
		ServiceManagerSupported: msg.GetServiceManagerSupported(),
		InstallPathWritable:     msg.GetInstallPathWritable(),
		InstallPath:             msg.GetInstallPath(),
	}, nil
}

// GetRuntimeCapabilities invokes the runtime capability RPC. Older nodes that
// do not implement the RPC are treated as having no optional runtime features.
func (c *GRPCNodeClient) GetRuntimeCapabilities(ctx context.Context) (node.RuntimeCapabilities, error) {
	req := newReq(c, &nodeprotov1.GetRuntimeCapabilitiesRequest{})
	resp, errRPC := c.connectClient.GetRuntimeCapabilities(ctx, req)
	if errRPC != nil {
		if connect.CodeOf(errRPC) == connect.CodeUnimplemented {
			return node.RuntimeCapabilities{}, nil
		}
		return node.RuntimeCapabilities{}, translateError("get runtime capabilities", errRPC)
	}
	msg := resp.Msg
	return node.RuntimeCapabilities{
		ProtocolVersion:          msg.GetProtocolVersion(),
		LaunchEnv:                msg.GetLaunchEnv(),
		ReliableProcessLifecycle: msg.GetReliableProcessLifecycle(),
		TelnetInput:              msg.GetTelnetInput(),
		RCONInput:                msg.GetRconInput(),
		RESTInput:                msg.GetRestInput(),
		PlayerActions:            msg.GetPlayerActions(),
		PalworldMap:              msg.GetPalworldMap(),
		SevenDaysToDieMap:        msg.GetSevenDaysToDieMap(),
		MinecraftMap:             msg.GetMinecraftMap(),
	}, nil
}

func sevenDaysToDieMapSnapshotFromProto(snapshot *nodeprotov1.SevenDaysToDieMapSnapshot) *node.SevenDaysToDieMapSnapshot {
	if snapshot == nil {
		return nil
	}
	players := make([]node.SevenDaysToDieMapPlayer, 0, len(snapshot.GetPlayers()))
	for _, player := range snapshot.GetPlayers() {
		if player == nil {
			continue
		}
		players = append(players, node.SevenDaysToDieMapPlayer{
			ID:       player.GetId(),
			Name:     player.GetName(),
			Online:   player.GetOnline(),
			Position: sevenDaysToDieMapVectorFromProto(player.GetPosition()),
		})
	}
	return &node.SevenDaysToDieMapSnapshot{
		Enabled: snapshot.GetEnabled(), TileSize: snapshot.GetTileSize(), MaxZoom: snapshot.GetMaxZoom(),
		MapSize: sevenDaysToDieMapVectorFromProto(snapshot.GetMapSize()), SourceTime: snapshot.GetSourceTime(),
		Players: players,
	}
}

func sevenDaysToDieMapVectorFromProto(vector *nodeprotov1.SevenDaysToDieMapVector) node.SevenDaysToDieMapVector {
	if vector == nil {
		return node.SevenDaysToDieMapVector{}
	}
	return node.SevenDaysToDieMapVector{X: vector.GetX(), Y: vector.GetY(), Z: vector.GetZ()}
}

func sevenDaysToDieWebAPIConnectionStateFromProto(
	state nodeprotov1.SevenDaysToDieWebAPIConnectionState,
) node.SevenDaysToDieWebAPIConnectionState {
	switch state {
	case nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE:
		return node.SevenDaysToDieWebAPIConnectionStateAvailable
	case nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE:
		return node.SevenDaysToDieWebAPIConnectionStateServerOffline
	case nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DASHBOARD_DISABLED:
		return node.SevenDaysToDieWebAPIConnectionStateDashboardDisabled
	case nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_MISCONFIGURED:
		return node.SevenDaysToDieWebAPIConnectionStateMisconfigured
	case nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_NODE_UNAVAILABLE:
		return node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable
	case nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_WEB_API_UNREACHABLE:
		return node.SevenDaysToDieWebAPIConnectionStateUnreachable
	case nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DISCOVERY_UNSUPPORTED:
		return node.SevenDaysToDieWebAPIConnectionStateDiscoveryUnsupported
	case nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AUTHENTICATION_DENIED:
		return node.SevenDaysToDieWebAPIConnectionStateAuthenticationDenied
	case nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_INVALID_RESPONSE:
		return node.SevenDaysToDieWebAPIConnectionStateInvalidResponse
	default:
		return node.SevenDaysToDieWebAPIConnectionStateUnspecified
	}
}

func sevenDaysToDieWebAPIValueStateFromProto(
	state nodeprotov1.SevenDaysToDieWebAPIValueState,
) node.SevenDaysToDieWebAPIValueState {
	switch state {
	case nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE:
		return node.SevenDaysToDieWebAPIValueStateAvailable
	case nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED:
		return node.SevenDaysToDieWebAPIValueStateUnsupported
	case nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED:
		return node.SevenDaysToDieWebAPIValueStatePermissionDenied
	case nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE:
		return node.SevenDaysToDieWebAPIValueStateUnavailable
	default:
		return node.SevenDaysToDieWebAPIValueStateUnspecified
	}
}

func sevenDaysToDieWebAPIStatusFromProto(status *nodeprotov1.SevenDaysToDieWebAPIStatus) *node.SevenDaysToDieWebAPIStatus {
	if status == nil {
		return nil
	}
	return &node.SevenDaysToDieWebAPIStatus{
		ConnectionState:  sevenDaysToDieWebAPIConnectionStateFromProto(status.GetConnectionState()),
		APIVersion:       status.GetApiVersion(),
		Capabilities:     sevenDaysToDieWebAPICapabilitiesFromProto(status.GetCapabilities()),
		WorldTimeState:   sevenDaysToDieWebAPIValueStateFromProto(status.GetWorldTimeState()),
		WorldTime:        sevenDaysToDieGameTimeFromProto(status.GetWorldTime()),
		BloodMoonState:   sevenDaysToDieWebAPIValueStateFromProto(status.GetBloodMoonState()),
		BloodMoonActive:  cloneBool(status.BloodMoonActive),
		NextBloodMoon:    sevenDaysToDieGameTimeFromProto(status.GetNextBloodMoon()),
		NextBloodMoonEnd: sevenDaysToDieGameTimeFromProto(status.GetNextBloodMoonEnd()),
		ObservedAt:       validProtoTime(status.GetObservedAt()),
	}
}

func sevenDaysToDiePlayersFromProto(result *nodeprotov1.SevenDaysToDiePlayers) *node.SevenDaysToDiePlayers {
	if result == nil {
		return nil
	}
	players := make([]node.SevenDaysToDiePlayer, 0, len(result.GetPlayers()))
	for _, player := range result.GetPlayers() {
		if player == nil {
			continue
		}
		players = append(players, node.SevenDaysToDiePlayer{
			Name:            player.GetName(),
			ActionID:        player.GetActionId(),
			EntityID:        player.GetEntityId(),
			PlatformID:      player.GetPlatformId(),
			CrossPlatformID: player.GetCrossPlatformId(),
			Online:          cloneBool(player.Online),
			Ping:            cloneInt32(player.Ping),
			Level:           cloneInt32(player.Level),
			Health:          cloneInt32(player.Health),
			Stamina:         cloneFloat32(player.Stamina),
			Score:           cloneInt32(player.Score),
			Deaths:          cloneInt32(player.Deaths),
			ZombieKills:     cloneInt32(player.ZombieKills),
			PlayerKills:     cloneInt32(player.PlayerKills),
			Banned:          cloneBool(player.Banned),
		})
	}
	return &node.SevenDaysToDiePlayers{
		ConnectionState: sevenDaysToDieWebAPIConnectionStateFromProto(result.GetConnectionState()),
		State:           sevenDaysToDieWebAPIValueStateFromProto(result.GetState()),
		Players:         players,
	}
}

func sevenDaysToDieReportedModsFromProto(result *nodeprotov1.SevenDaysToDieReportedMods) (*node.SevenDaysToDieReportedMods, error) {
	if result == nil {
		return nil, errors.New("reported mod result is missing")
	}
	protoMods := result.GetMods()
	if len(protoMods) > node.SevenDaysToDieReportedModCountLimit {
		return nil, errors.New("reported mod count exceeds limit")
	}
	mods := make([]node.SevenDaysToDieReportedMod, 0, len(protoMods))
	for _, mod := range protoMods {
		if mod == nil {
			return nil, errors.New("reported mod is missing")
		}
		mods = append(mods, node.SevenDaysToDieReportedMod{
			Name: mod.GetName(), DisplayName: mod.GetDisplayName(), Description: mod.GetDescription(), Author: mod.GetAuthor(), Version: mod.GetVersion(),
		})
	}
	errMods := node.ValidateSevenDaysToDieReportedMods(mods)
	if errMods != nil {
		return nil, fmt.Errorf("validate reported mods: %w", errMods)
	}
	return &node.SevenDaysToDieReportedMods{
		ConnectionState: sevenDaysToDieWebAPIConnectionStateFromProto(result.GetConnectionState()),
		State:           sevenDaysToDieWebAPIValueStateFromProto(result.GetState()),
		Mods:            mods,
	}, nil
}

func sevenDaysToDieSandboxSettingsFromProto(result *nodeprotov1.SevenDaysToDieSandboxSettings) (*node.SevenDaysToDieSandboxSettings, error) {
	if result == nil {
		return nil, errors.New("sandbox settings result is missing")
	}
	protoSettings := result.GetSettings()
	if len(protoSettings) > node.SevenDaysToDieSandboxSettingCountLimit {
		return nil, errors.New("sandbox setting count exceeds limit")
	}
	settings := make([]node.SevenDaysToDieSandboxSetting, 0, len(protoSettings))
	for _, setting := range protoSettings {
		if setting == nil {
			return nil, errors.New("sandbox setting is missing")
		}
		settings = append(settings, node.SevenDaysToDieSandboxSetting{
			Key:            setting.GetKey(),
			Label:          setting.GetLabel(),
			Description:    setting.GetDescription(),
			Group:          setting.GetGroup(),
			EffectiveValue: setting.GetEffectiveValue(),
			EffectiveLabel: setting.GetEffectiveLabel(),
		})
	}
	converted := &node.SevenDaysToDieSandboxSettings{
		ConnectionState: sevenDaysToDieWebAPIConnectionStateFromProto(result.GetConnectionState()),
		State:           sevenDaysToDieWebAPIValueStateFromProto(result.GetState()),
		ComparisonState: sevenDaysToDieSandboxComparisonStateFromProto(result.GetComparisonState()),
		ConfiguredCode:  result.GetConfiguredCode(),
		EffectiveCode:   result.GetEffectiveCode(),
		Settings:        settings,
		ObservedAt:      validProtoTime(result.GetObservedAt()),
	}
	errValidate := node.ValidateSevenDaysToDieSandboxSettings(converted)
	if errValidate != nil {
		return nil, fmt.Errorf("validate sandbox settings: %w", errValidate)
	}
	return converted, nil
}

func sevenDaysToDieSandboxComparisonStateFromProto(state nodeprotov1.SevenDaysToDieSandboxComparisonState) node.SevenDaysToDieSandboxComparisonState {
	switch state {
	case nodeprotov1.SevenDaysToDieSandboxComparisonState_SEVEN_DAYS_TO_DIE_SANDBOX_COMPARISON_STATE_MATCH:
		return node.SevenDaysToDieSandboxComparisonStateMatch
	case nodeprotov1.SevenDaysToDieSandboxComparisonState_SEVEN_DAYS_TO_DIE_SANDBOX_COMPARISON_STATE_MISMATCH:
		return node.SevenDaysToDieSandboxComparisonStateMismatch
	case nodeprotov1.SevenDaysToDieSandboxComparisonState_SEVEN_DAYS_TO_DIE_SANDBOX_COMPARISON_STATE_STALE:
		return node.SevenDaysToDieSandboxComparisonStateStale
	default:
		return node.SevenDaysToDieSandboxComparisonStateUnspecified
	}
}

func sevenDaysToDieWebAPICapabilitiesFromProto(capabilities *nodeprotov1.SevenDaysToDieWebAPICapabilities) node.SevenDaysToDieWebAPICapabilities {
	if capabilities == nil {
		return node.SevenDaysToDieWebAPICapabilities{}
	}
	return node.SevenDaysToDieWebAPICapabilities{
		PlayerData:                capabilities.GetPlayerData(),
		RuntimeSettings:           capabilities.GetRuntimeSettings(),
		NativeLog:                 capabilities.GetNativeLog(),
		WorldPopulation:           capabilities.GetWorldPopulation(),
		HostileAndAnimalPositions: capabilities.GetHostileAndAnimalPositions(),
		AccessControl:             capabilities.GetAccessControl(),
		GamePermissions:           capabilities.GetGamePermissions(),
		ReportedMods:              capabilities.GetReportedMods(),
	}
}

func sevenDaysToDieGameTimeFromProto(gameTime *nodeprotov1.SevenDaysToDieGameTime) *node.SevenDaysToDieGameTime {
	if gameTime == nil {
		return nil
	}
	return &node.SevenDaysToDieGameTime{Day: gameTime.GetDay(), Hour: gameTime.GetHour(), Minute: gameTime.GetMinute()}
}

func validProtoTime(timestamp *timestamppb.Timestamp) time.Time {
	if timestamp == nil {
		return time.Time{}
	}
	errValid := timestamp.CheckValid()
	if errValid != nil {
		return time.Time{}
	}
	return timestamp.AsTime()
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	return new(*value)
}

func cloneInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	return new(*value)
}

func cloneFloat32(value *float32) *float32 {
	if value == nil {
		return nil
	}
	return new(*value)
}

func palworldMapSnapshotFromProto(snapshot *nodeprotov1.PalworldMapSnapshot) *node.PalworldMapSnapshot {
	if snapshot == nil {
		return nil
	}
	actors := make([]node.PalworldMapActor, 0, len(snapshot.GetActors()))
	for _, actor := range snapshot.GetActors() {
		if actor == nil {
			continue
		}
		actors = append(actors, node.PalworldMapActor{
			Key:         actor.GetKey(),
			Kind:        palworldMapActorKindFromProto(actor.GetKind()),
			Name:        actor.GetName(),
			GuildKey:    actor.GetGuildKey(),
			GuildName:   actor.GetGuildName(),
			TrainerName: actor.GetTrainerName(),
			ClassName:   actor.GetClassName(),
			LocationX:   actor.GetLocationX(),
			LocationY:   actor.GetLocationY(),
			LocationZ:   actor.GetLocationZ(),
			RotationZ:   actor.GetRotationZ(),
			Level:       actor.GetLevel(),
			HP:          actor.GetHp(),
			MaxHP:       actor.GetMaxHp(),
			Action:      actor.GetAction(),
			AIAction:    actor.GetAiAction(),
			Active:      actor.GetActive(),
		})
	}
	collectedAt := time.Time{}
	if snapshot.GetCollectedAt() != nil {
		collectedAt = snapshot.GetCollectedAt().AsTime()
	}
	return &node.PalworldMapSnapshot{
		SourceTime:    snapshot.GetSourceTime(),
		CollectedAt:   collectedAt,
		Source:        snapshot.GetSource(),
		Partial:       snapshot.GetPartial(),
		Truncated:     snapshot.GetTruncated(),
		PartialReason: snapshot.GetPartialReason(),
		Actors:        actors,
		Health:        palworldMapHealthFromProto(snapshot.GetHealth()),
	}
}

func palworldMapHealthFromProto(health *nodeprotov1.PalworldMapHealth) *node.PalworldMapHealth {
	if health == nil {
		return nil
	}
	return &node.PalworldMapHealth{
		ServerFPS:         health.GetServerFps(),
		ServerFrameTimeMS: health.GetServerFrameTimeMs(),
		CurrentPlayers:    health.GetCurrentPlayers(),
		MaxPlayers:        health.GetMaxPlayers(),
		UptimeSeconds:     health.GetUptimeSeconds(),
		BaseCampCount:     health.GetBaseCampCount(),
		Days:              health.GetDays(),
	}
}

func palworldMapActorKindFromProto(kind nodeprotov1.PalworldMapActorKind) node.PalworldMapActorKind {
	switch kind {
	case nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_PLAYER:
		return node.PalworldMapActorKindPlayer
	case nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_BASE:
		return node.PalworldMapActorKindBase
	case nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_BASE_WORKER:
		return node.PalworldMapActorKindBaseWorker
	case nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_COMPANION_PAL:
		return node.PalworldMapActorKindCompanionPal
	case nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_WILD_PAL:
		return node.PalworldMapActorKindWildPal
	case nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_NPC:
		return node.PalworldMapActorKindNPC
	case nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_OTHER:
		return node.PalworldMapActorKindOther
	default:
		return node.PalworldMapActorKindUnknown
	}
}

func rconProtocolToProto(protocol node.RCONProtocol) (nodeprotov1.RCONProtocol, error) {
	switch protocol {
	case node.RCONProtocolSource:
		return nodeprotov1.RCONProtocol_RCON_PROTOCOL_SOURCE, nil
	case node.RCONProtocolMinecraft:
		return nodeprotov1.RCONProtocol_RCON_PROTOCOL_MINECRAFT, nil
	case node.RCONProtocolRustWeb:
		return nodeprotov1.RCONProtocol_RCON_PROTOCOL_RUST_WEB, nil
	default:
		return nodeprotov1.RCONProtocol_RCON_PROTOCOL_UNSPECIFIED, errors.New("nodeclient: unsupported RCON protocol")
	}
}

func restInputKindToProto(kind node.RESTInputKind) (nodeprotov1.RESTInputKind, error) {
	switch kind {
	case node.RESTInputKindSatisfactory:
		return nodeprotov1.RESTInputKind_REST_INPUT_KIND_SATISFACTORY, nil
	case node.RESTInputKindPalworld:
		return nodeprotov1.RESTInputKind_REST_INPUT_KIND_PALWORLD, nil
	default:
		return nodeprotov1.RESTInputKind_REST_INPUT_KIND_UNSPECIFIED, errors.New("nodeclient: unsupported REST input kind")
	}
}

const stageSelfUpdateChunkBytes = 256 * 1024

// StageSelfUpdate streams a self-update artifact to the node.
func (c *GRPCNodeClient) StageSelfUpdate(ctx context.Context, req node.StageSelfUpdateRequest) (node.StageSelfUpdateResult, error) {
	stream := c.streamConnectClient().StageSelfUpdate(ctx)
	c.authorize(stream.RequestHeader())

	errSend := stream.Send(&nodeprotov1.StageSelfUpdateRequest{
		Component:      req.Component,
		TargetVersion:  req.TargetVersion,
		Os:             req.OS,
		Architecture:   req.Architecture,
		ExpectedSize:   req.ExpectedSize,
		ExpectedSha256: req.ExpectedSHA256,
	})
	if errSend != nil {
		return node.StageSelfUpdateResult{}, translateError("stage self-update", errSend)
	}

	if req.Reader != nil {
		buf := make([]byte, stageSelfUpdateChunkBytes)
		for {
			n, errRead := req.Reader.Read(buf)
			if n > 0 {
				content := append([]byte(nil), buf[:n]...)
				errSend = stream.Send(&nodeprotov1.StageSelfUpdateRequest{Content: content})
				if errSend != nil {
					return node.StageSelfUpdateResult{}, translateError("stage self-update", errSend)
				}
			}
			if errors.Is(errRead, io.EOF) {
				break
			}
			if errRead != nil {
				return node.StageSelfUpdateResult{}, fmt.Errorf("nodeclient: read self-update artifact: %w", errRead)
			}
		}
	}

	resp, errRPC := stream.CloseAndReceive()
	if errRPC != nil {
		return node.StageSelfUpdateResult{}, translateError("stage self-update", errRPC)
	}
	return node.StageSelfUpdateResult{
		StageID:      resp.Msg.GetStageId(),
		BytesWritten: resp.Msg.GetBytesWritten(),
		SHA256:       resp.Msg.GetSha256(),
	}, nil
}

// ApplySelfUpdate invokes the ApplySelfUpdate RPC.
func (c *GRPCNodeClient) ApplySelfUpdate(ctx context.Context, req node.ApplySelfUpdateRequest) (node.ApplySelfUpdateResult, error) {
	rpcReq := newReq(c, &nodeprotov1.ApplySelfUpdateRequest{
		StageId:        req.StageID,
		TargetVersion:  req.TargetVersion,
		ExpectedSha256: req.ExpectedSHA256,
	})
	resp, errRPC := c.connectClient.ApplySelfUpdate(ctx, rpcReq)
	if errRPC != nil {
		return node.ApplySelfUpdateResult{}, translateError("apply self-update", errRPC)
	}
	return node.ApplySelfUpdateResult{
		Accepted: resp.Msg.GetAccepted(),
		Message:  resp.Msg.GetMessage(),
	}, nil
}

// StreamEvents subscribes to the node's event stream and returns a channel
// that closes when ctx is canceled or the underlying stream errors. A failure
// to open the stream is reported synchronously via the error return; failures
// during streaming are logged via the closed channel.
func (c *GRPCNodeClient) StreamEvents(ctx context.Context) (<-chan node.Event, error) {
	req := newReq(c, &nodeprotov1.StreamEventsRequest{ReplayProcessStatus: true})
	stream, errOpen := c.streamConnectClient().StreamEvents(ctx, req)
	if errOpen != nil {
		return nil, translateError("stream events", errOpen)
	}

	out := make(chan node.Event, 64)
	go func() {
		defer close(out)
		defer func() { _ = stream.Close() }()

		for stream.Receive() {
			ev := nodeEventFromProto(stream.Msg())
			select {
			case <-ctx.Done():
				return
			case out <- ev:
			}
		}
	}()
	return out, nil
}

// Ping invokes the Ping RPC.
func (c *GRPCNodeClient) Ping(ctx context.Context) error {
	req := newReq(c, &nodeprotov1.PingRequest{})
	_, errRPC := c.connectClient.Ping(ctx, req)
	if errRPC != nil {
		return translateError("ping", errRPC)
	}
	return nil
}

// processStatusFromXylona converts the controller's xylona.Status into the
// node-internal ProcessStatus enum value.
func processStatusFromXylona(status xylona.Status) nodeprotov1.ProcessStatus {
	switch status {
	case xylona.Status_OFFLINE:
		return nodeprotov1.ProcessStatus_PROCESS_STATUS_OFFLINE
	case xylona.Status_ONLINE:
		return nodeprotov1.ProcessStatus_PROCESS_STATUS_ONLINE
	case xylona.Status_INSTALLING:
		return nodeprotov1.ProcessStatus_PROCESS_STATUS_INSTALLING
	case xylona.Status_UPDATING:
		return nodeprotov1.ProcessStatus_PROCESS_STATUS_UPDATING
	case xylona.Status_PRE_START:
		return nodeprotov1.ProcessStatus_PROCESS_STATUS_PRE_START
	default:
		return nodeprotov1.ProcessStatus_PROCESS_STATUS_UNKNOWN
	}
}

// nodeSnapshotFromProto converts a v1.NodeSnapshot back into the Go domain
// type that internal/node consumers expect.
func nodeSnapshotFromProto(snap *nodeprotov1.NodeSnapshot) *node.NodeSnapshot {
	if snap == nil {
		return nil
	}
	processes := snap.GetProcesses()
	out := &node.NodeSnapshot{
		CPUModel:      snap.GetCpuModel(),
		CPUCores:      int(snap.GetCpuCores()),
		CPUThreads:    int(snap.GetCpuThreads()),
		TotalMemory:   snap.GetTotalMemory(),
		OS:            snap.GetOs(),
		OSVersion:     snap.GetOsVersion(),
		Architecture:  snap.GetArchitecture(),
		XylonaVersion: snap.GetXylonaVersion(),

		CPUPercent:    snap.GetCpuPercent(),
		MemoryUsed:    snap.GetMemoryUsed(),
		MemoryPercent: snap.GetMemoryPercent(),
		DiskUsed:      snap.GetDiskUsed(),
		DiskTotal:     snap.GetDiskTotal(),
		DiskPercent:   snap.GetDiskPercent(),

		DefaultInstallPath: snap.GetDefaultInstallPath(),

		Processes: make([]node.ProcessSnapshot, 0, len(processes)),
		Collected: snap.GetCollected().AsTime(),
	}
	for _, p := range processes {
		processSnapshot := processSnapshotFromProto(p)
		if processSnapshot == nil {
			out.Processes = append(out.Processes, node.ProcessSnapshot{})
			continue
		}
		out.Processes = append(out.Processes, *processSnapshot)
	}
	return out
}

// nodeEventFromProto converts a v1.Event into the Go domain Event.
func nodeEventFromProto(ev *nodeprotov1.Event) node.Event {
	out := node.Event{
		Timestamp: ev.GetTimestamp().AsTime(),
	}
	switch payload := ev.GetPayload().(type) {
	case *nodeprotov1.Event_ProcessStatus:
		status := payload.ProcessStatus
		if status == nil {
			break
		}
		out.Type = node.EventTypeProcessStatus
		out.ProcessID = status.GetProcessId()
		out.Status = status.GetStatus()
		out.OldStatus = status.GetOldStatus()
		out.ExecutionID = status.GetExecutionId()
		out.TransitionSequence = status.GetTransitionSequence()
		out.IntentionalStop = status.GetIntentionalStop()
		out.Replayed = status.GetReplayed()
		if status.ExitCode != nil {
			out.ExitCode = int(status.GetExitCode())
			out.ExitCodeKnown = true
		}
	case *nodeprotov1.Event_ConsoleOutput:
		out.Type = node.EventTypeConsoleOutput
		out.ProcessID = payload.ConsoleOutput.GetGameServerId()
		out.Payload = node.ConsoleChunk{
			ProcessID:   payload.ConsoleOutput.GetGameServerId(),
			Data:        payload.ConsoleOutput.GetText(),
			Sequence:    payload.ConsoleOutput.GetSequence(),
			ResetBuffer: payload.ConsoleOutput.GetResetBuffer(),
		}
	case *nodeprotov1.Event_MetricsUpdate:
		out.Type = node.EventTypeMetrics
		out.Payload = nodeSnapshotFromProto(payload.MetricsUpdate.GetSnapshot())
	case *nodeprotov1.Event_ProcessCrash:
		// EventTypeProcessStatus is the closest existing type; payload carries
		// the exit reason. Step 9 is expected to add a dedicated EventTypeCrash.
		out.Type = node.EventTypeProcessStatus
		out.ProcessID = payload.ProcessCrash.GetProcessId()
		out.Status = "crashed"
		out.Payload = payload.ProcessCrash.GetReason()
	}
	return out
}

// translateError maps a Connect error back into an internal/node sentinel where
// possible, so callers can use errors.Is() to react. Errors with no mapping
// are wrapped with the call name for log readability.
func translateError(call string, err error) error {
	if err == nil {
		return nil
	}

	connectErr := new(connect.Error)
	if errors.As(err, &connectErr) {
		mapped := mapNodeErrorCode(connectErr)
		if mapped != nil {
			return fmt.Errorf("nodeclient: %s: %w", call, mapped)
		}
	}
	return fmt.Errorf("nodeclient: %s: %w", call, err)
}

func translateProcessError(call string, err error) error {
	if err == nil {
		return nil
	}

	connectErr := new(connect.Error)
	if errors.As(err, &connectErr) && connectErr.Code() == connect.CodeNotFound {
		return fmt.Errorf("nodeclient: %s: %w", call, node.ErrProcessNotFound)
	}
	return translateError(call, err)
}

func translateConsoleInputError(call string, err error) error {
	if err == nil {
		return nil
	}

	connectErr := new(connect.Error)
	if errors.As(err, &connectErr) {
		switch connectErr.Code() {
		case connect.CodeInvalidArgument:
			return fmt.Errorf(
				"nodeclient: %s: %w",
				call,
				node.NewConsoleInputRejectedError(connectErr.Message()),
			)
		case connect.CodeFailedPrecondition:
			return fmt.Errorf("nodeclient: %s: %w", call, node.ErrConsoleInputUnavailable)
		}
	}
	return translateProcessError(call, err)
}

func translatePlayerActionError(call string, err error) error {
	if err == nil {
		return nil
	}

	connectErr := new(connect.Error)
	if errors.As(err, &connectErr) {
		var mapped error
		switch connectErr.Code() {
		case connect.CodeInvalidArgument:
			mapped = node.ErrInvalidPlayerAction
		case connect.CodeNotFound:
			mapped = node.ErrProcessNotFound
		case connect.CodeFailedPrecondition:
			mapped = node.ErrConsoleInputUnavailable
		case connect.CodeUnimplemented:
			mapped = node.ErrPlayerActionUnsupported
		case connect.CodeUnavailable:
			mapped = node.ErrPlayerActionUnavailable
		}
		if mapped != nil {
			return fmt.Errorf("nodeclient: %s: %w", call, mapped)
		}
	}
	return fmt.Errorf("nodeclient: %s: %w", call, err)
}

// mapNodeErrorCode maps Connect status codes to Go sentinels. It returns nil
// when no mapping is available.
func mapNodeErrorCode(connectErr *connect.Error) error {
	switch connectErr.Code() {
	case connect.CodeInvalidArgument:
		return node.ErrInvalidPath
	case connect.CodeNotFound:
		return os.ErrNotExist
	case connect.CodePermissionDenied:
		return node.ErrProtectedPath
	}
	return nil
}
