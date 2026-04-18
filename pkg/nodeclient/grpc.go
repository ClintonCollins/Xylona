package nodeclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/nodetls"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1/nodeprotoconnect"
	"github.com/ClintonCollins/Xylona/supervisor"
)

// AuthorizationHeader is the header the controller uses to present its
// shared secret to a node. Exposed so the node-side service can read the
// same constant when validating requests.
const AuthorizationHeader = "Authorization"

// authorizationScheme is the value prefix paired with AuthorizationHeader.
const authorizationScheme = "Bearer "

// GRPCNodeClient is the controller-side NodeClient implementation that talks
// to a remote node over Connect-RPC. The transport TLS layer pins the node's
// self-signed cert fingerprint; application-layer auth is the bearer token.
//
// Step 4 introduces this implementation but the controller does not yet
// construct any. Step 6 wires it into noderegistry as part of the bootstrap
// pairing flow.
type GRPCNodeClient struct {
	nodeID        string
	listenURL     string
	sharedSecret  string
	httpClient    *http.Client
	connectClient nodeprotoconnect.NodeServiceClient
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

	connectClient := nodeprotoconnect.NewNodeServiceClient(httpClient, listenURL)

	return &GRPCNodeClient{
		nodeID:        nodeID,
		listenURL:     listenURL,
		sharedSecret:  sharedSecret,
		httpClient:    httpClient,
		connectClient: connectClient,
	}, nil
}

// ID returns the configured node identifier.
func (c *GRPCNodeClient) ID() string {
	return c.nodeID
}

// Close releases idle TLS connections held by the underlying transport. Safe
// to call multiple times.
func (c *GRPCNodeClient) Close(_ context.Context) error {
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

// ErrRemoteStartProcessHandle is returned by the gRPC client's StartProcess to
// indicate success without a live *supervisor.Command handle. The controller
// cannot reconstruct one from the wire, and Step 9 will collapse the return
// signature to drop the Command entirely. Callers treat this sentinel as a
// success signal and fall back to StreamEvents for lifecycle observation.
var ErrRemoteStartProcessHandle = errors.New("nodeclient: remote start process has no supervisor handle; subscribe via StreamEvents")

// StartProcess sends the StartProcess RPC. For the gRPC client the returned
// *supervisor.Command is always nil and the error is ErrRemoteStartProcessHandle
// on success — the controller cannot reconstruct a live supervisor handle from
// the wire. Callers should rely on StreamEvents for lifecycle observation; this
// is the temporary shape called out by the TODO in client.go and lives until
// Step 9 collapses the return signature.
func (c *GRPCNodeClient) StartProcess(ctx context.Context, cfg node.ProcessConfig, status xylona.Status) (*supervisor.Command, error) {
	req := newReq(c, &nodeprotov1.StartProcessRequest{
		Id:                 cfg.ID,
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
	})

	_, errRPC := c.connectClient.StartProcess(ctx, req)
	if errRPC != nil {
		return nil, translateError("start process", errRPC)
	}
	return nil, ErrRemoteStartProcessHandle
}

// StopProcess invokes the StopProcess RPC.
func (c *GRPCNodeClient) StopProcess(ctx context.Context, processID string, stopInputCommand string) error {
	req := newReq(c, &nodeprotov1.StopProcessRequest{
		ProcessId:        processID,
		StopInputCommand: stopInputCommand,
	})
	_, errRPC := c.connectClient.StopProcess(ctx, req)
	if errRPC != nil {
		return translateError("stop process", errRPC)
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
		return translateError("send console input", errRPC)
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
		return node.ConsoleChunk{ProcessID: processID}, nil
	}
	return node.ConsoleChunk{
		ProcessID: chunk.GetGameServerId(),
		Data:      chunk.GetText(),
	}, nil
}

// StreamConsoleOutput subscribes to live console chunks for one process.
func (c *GRPCNodeClient) StreamConsoleOutput(ctx context.Context, processID string) (<-chan node.ConsoleChunk, error) {
	req := newReq(c, &nodeprotov1.StreamConsoleOutputRequest{ProcessId: processID})
	stream, errOpen := c.connectClient.StreamConsoleOutput(ctx, req)
	if errOpen != nil {
		return nil, translateError("stream console output", errOpen)
	}

	out := make(chan node.ConsoleChunk, 64)
	go func() {
		defer close(out)
		defer func() { _ = stream.Close() }()

		for stream.Receive() {
			msg := stream.Msg()
			chunk := node.ConsoleChunk{
				ProcessID: msg.GetGameServerId(),
				Data:      msg.GetText(),
			}

			select {
			case <-ctx.Done():
				return
			case out <- chunk:
			}
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
		out = append(out, node.FileEntry{
			Name:         entry.GetName(),
			Size:         entry.GetSize(),
			IsDirectory:  entry.GetIsDirectory(),
			LastModified: entry.GetLastModified().AsTime(),
		})
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

// DownloadFileFromURL invokes the DownloadFileFromURL RPC.
func (c *GRPCNodeClient) DownloadFileFromURL(ctx context.Context, directory string, rawURL string, destinationDirectoryPath string, policy node.ProtectionPolicy) (string, error) {
	req := newReq(c, &nodeprotov1.DownloadFileFromURLRequest{
		Directory:                directory,
		Url:                      rawURL,
		DestinationDirectoryPath: destinationDirectoryPath,
		ServerExecutable:         policy.ServerExecutable,
		BaseCommand:              policy.BaseCommand,
	})
	resp, errRPC := c.connectClient.DownloadFileFromURL(ctx, req)
	if errRPC != nil {
		return "", translateError("download file from URL", errRPC)
	}
	return resp.Msg.GetRelativePath(), nil
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

func extractModeToProto(mode node.ExtractMode) nodeprotov1.ExtractMode {
	switch mode {
	case node.ExtractModeExact:
		return nodeprotov1.ExtractMode_EXTRACT_MODE_EXACT
	default:
		return nodeprotov1.ExtractMode_EXTRACT_MODE_OVERLAY
	}
}

// processSnapshotFromProto converts a v1.ProcessSnapshot into the Go domain
// ProcessSnapshot. Used by GetProcessSnapshot and the process list inside
// nodeSnapshotFromProto (indirectly).
func processSnapshotFromProto(p *nodeprotov1.ProcessSnapshot) *node.ProcessSnapshot {
	if p == nil {
		return nil
	}
	return &node.ProcessSnapshot{
		ID:              p.GetId(),
		Name:            p.GetName(),
		Status:          p.GetStatus(),
		UnixStartedAt:   p.GetUnixStartedAt(),
		CPUPercent:      p.GetCpuPercent(),
		CPUCores:        p.GetCpuCores(),
		MemoryRSS:       p.GetMemoryRss(),
		MemoryVMS:       p.GetMemoryVms(),
		MemoryPercent:   p.GetMemoryPercent(),
		NumThreads:      p.GetNumThreads(),
		DiskUsageBytes:  p.GetDiskUsageBytes(),
		IOReadRate:      p.GetIoReadRate(),
		IOWriteRate:     p.GetIoWriteRate(),
		ConnectionCount: p.GetConnectionCount(),
		WorkingDir:      p.GetWorkingDir(),
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

// StreamEvents subscribes to the node's event stream and returns a channel
// that closes when ctx is canceled or the underlying stream errors. A failure
// to open the stream is reported synchronously via the error return; failures
// during streaming are logged via the closed channel.
func (c *GRPCNodeClient) StreamEvents(ctx context.Context) (<-chan node.Event, error) {
	req := newReq(c, &nodeprotov1.StreamEventsRequest{})
	stream, errOpen := c.connectClient.StreamEvents(ctx, req)
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
// type that pkg/node consumers expect.
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
		out.Processes = append(out.Processes, node.ProcessSnapshot{
			ID:              p.GetId(),
			Name:            p.GetName(),
			Status:          p.GetStatus(),
			UnixStartedAt:   p.GetUnixStartedAt(),
			CPUPercent:      p.GetCpuPercent(),
			CPUCores:        p.GetCpuCores(),
			MemoryRSS:       p.GetMemoryRss(),
			MemoryVMS:       p.GetMemoryVms(),
			MemoryPercent:   p.GetMemoryPercent(),
			NumThreads:      p.GetNumThreads(),
			DiskUsageBytes:  p.GetDiskUsageBytes(),
			IOReadRate:      p.GetIoReadRate(),
			IOWriteRate:     p.GetIoWriteRate(),
			ConnectionCount: p.GetConnectionCount(),
			WorkingDir:      p.GetWorkingDir(),
		})
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
		out.Type = node.EventTypeProcessStatus
		out.ProcessID = payload.ProcessStatus.GetProcessId()
		out.Status = payload.ProcessStatus.GetStatus()
	case *nodeprotov1.Event_ConsoleOutput:
		out.Type = node.EventTypeConsoleOutput
		out.ProcessID = payload.ConsoleOutput.GetGameServerId()
		out.Payload = node.ConsoleChunk{
			ProcessID: payload.ConsoleOutput.GetGameServerId(),
			Data:      payload.ConsoleOutput.GetText(),
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

// translateError maps a Connect error back into a pkg/node sentinel where
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

// mapNodeErrorCode looks at the connect error's metadata for a typed
// NodeErrorCode and maps it to a Go sentinel. Returns nil when no mapping is
// available.
func mapNodeErrorCode(connectErr *connect.Error) error {
	for _, detail := range connectErr.Details() {
		value, errValue := detail.Value()
		if errValue != nil {
			continue
		}
		// Detail values arrive as proto messages; for now we only inspect the
		// raw type URL to detect the NodeErrorCode wrapper. Step 6 will define
		// the wrapper message; until then this loop is intentionally empty.
		_ = value
	}

	// Fallback heuristics based on connect.Code so the embedded node and
	// future wrappers both behave sensibly.
	switch connectErr.Code() {
	case connect.CodeNotFound:
		return node.ErrInvalidPath
	case connect.CodePermissionDenied:
		return node.ErrProtectedPath
	}
	return nil
}
