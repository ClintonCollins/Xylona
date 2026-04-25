package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/selfupdate"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1/nodeprotoconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// nodeServiceServer is the Connect-RPC handler implementation that wraps a
// *pkg/node.Node. Every method validates the bearer token, translates proto
// input into the Go domain types, invokes the node, and translates the result
// back out.
type nodeServiceServer struct {
	nodeprotoconnect.UnimplementedNodeServiceHandler

	n            *node.Node
	sharedSecret string
	updater      selfUpdater
}

type selfUpdater interface {
	Capabilities() node.UpdateCapabilities
	Stage(ctx context.Context, req node.StageSelfUpdateRequest) (node.StageSelfUpdateResult, error)
	Apply(ctx context.Context, req node.ApplySelfUpdateRequest) (node.ApplySelfUpdateResult, error)
}

// newNodeServiceServer constructs a handler. sharedSecret is the bearer token
// the caller must present on every RPC.
func newNodeServiceServer(n *node.Node, sharedSecret string, updaters ...selfUpdater) *nodeServiceServer {
	var updater selfUpdater
	if len(updaters) > 0 {
		updater = updaters[0]
	}
	return &nodeServiceServer{n: n, sharedSecret: sharedSecret, updater: updater}
}

// authorize inspects the Connect request headers and returns a connect.Error
// with CodeUnauthenticated when the bearer token is missing or does not match
// the configured secret. All comparisons are constant-time.
func (s *nodeServiceServer) authorize(header interface {
	Get(string) string
}) error {
	raw := strings.TrimSpace(header.Get(nodeclient.AuthorizationHeader))
	if raw == "" {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("authorization header required"))
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(raw, prefix) {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("authorization scheme must be Bearer"))
	}
	presented := strings.TrimSpace(raw[len(prefix):])
	match := subtle.ConstantTimeCompare([]byte(presented), []byte(s.sharedSecret))
	if match != 1 {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("invalid shared secret"))
	}
	return nil
}

// translate translates a Go-side error into a Connect error. pkg/node sentinel
// errors are mapped to explicit Connect codes; everything else surfaces as
// CodeInternal so the controller's retry/back-off logic can react uniformly.
func translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, node.ErrInvalidPath) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if errors.Is(err, os.ErrNotExist) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if errors.Is(err, node.ErrProcessNotFound) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if errors.Is(err, node.ErrProtectedPath) {
		return connect.NewError(connect.CodePermissionDenied, err)
	}
	if errors.Is(err, node.ErrUnexpectedHTTPStatus) || errors.Is(err, node.ErrDownloadIntegrityMismatch) {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	if errors.Is(err, selfupdate.ErrInvalidStage) {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}

// xylonaStatusFromProto maps the internal ProcessStatus enum back to the
// public xylona.Status. The two enums intentionally duplicate semantics — see
// the comment on ProcessStatus in node.proto.
func xylonaStatusFromProto(s nodeprotov1.ProcessStatus) xylona.Status {
	switch s {
	case nodeprotov1.ProcessStatus_PROCESS_STATUS_OFFLINE:
		return xylona.Status_OFFLINE
	case nodeprotov1.ProcessStatus_PROCESS_STATUS_ONLINE:
		return xylona.Status_ONLINE
	case nodeprotov1.ProcessStatus_PROCESS_STATUS_INSTALLING:
		return xylona.Status_INSTALLING
	case nodeprotov1.ProcessStatus_PROCESS_STATUS_UPDATING:
		return xylona.Status_UPDATING
	case nodeprotov1.ProcessStatus_PROCESS_STATUS_PRE_START:
		return xylona.Status_PRE_START
	default:
		return xylona.Status_UNKNOWN
	}
}

// clampToInt32 converts an int to int32, saturating at the int32 bounds so
// overly large system-info values (e.g. CPU core counts on exotic hosts) do
// not wrap around during proto marshaling.
func clampToInt32(v int) int32 {
	const maxI32 = int(^uint32(0) >> 1)
	const minI32 = -maxI32 - 1
	if v > maxI32 {
		return int32(maxI32)
	}
	if v < minI32 {
		return int32(minI32)
	}
	return int32(v)
}

// nodeSnapshotToProto is the node-side counterpart of the client-side
// nodeSnapshotFromProto (pkg/nodeclient/grpc.go).
func nodeSnapshotToProto(snap *node.NodeSnapshot) *nodeprotov1.NodeSnapshot {
	if snap == nil {
		return &nodeprotov1.NodeSnapshot{}
	}
	processes := make([]*nodeprotov1.ProcessSnapshot, 0, len(snap.Processes))
	for _, p := range snap.Processes {
		processes = append(processes, &nodeprotov1.ProcessSnapshot{
			Id:              p.ID,
			Name:            p.Name,
			Status:          p.Status,
			UnixStartedAt:   p.UnixStartedAt,
			CpuPercent:      p.CPUPercent,
			CpuCores:        p.CPUCores,
			MemoryRss:       p.MemoryRSS,
			MemoryVms:       p.MemoryVMS,
			MemoryPercent:   p.MemoryPercent,
			NumThreads:      p.NumThreads,
			DiskUsageBytes:  p.DiskUsageBytes,
			IoReadRate:      p.IOReadRate,
			IoWriteRate:     p.IOWriteRate,
			ConnectionCount: p.ConnectionCount,
			WorkingDir:      p.WorkingDir,
		})
	}
	return &nodeprotov1.NodeSnapshot{
		CpuModel:           snap.CPUModel,
		CpuCores:           clampToInt32(snap.CPUCores),
		CpuThreads:         clampToInt32(snap.CPUThreads),
		TotalMemory:        snap.TotalMemory,
		Os:                 snap.OS,
		OsVersion:          snap.OSVersion,
		Architecture:       snap.Architecture,
		XylonaVersion:      snap.XylonaVersion,
		CpuPercent:         snap.CPUPercent,
		MemoryUsed:         snap.MemoryUsed,
		MemoryPercent:      snap.MemoryPercent,
		DiskUsed:           snap.DiskUsed,
		DiskTotal:          snap.DiskTotal,
		DiskPercent:        snap.DiskPercent,
		DefaultInstallPath: snap.DefaultInstallPath,
		Processes:          processes,
		Collected:          timestamppb.New(snap.Collected),
	}
}

// --- RPC method implementations --------------------------------------------

func (s *nodeServiceServer) StartProcess(ctx context.Context, req *connect.Request[nodeprotov1.StartProcessRequest]) (*connect.Response[nodeprotov1.StartProcessResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}

	msg := req.Msg
	cfg := node.ProcessConfig{
		ID:               msg.GetId(),
		Name:             msg.GetName(),
		BaseCommand:      msg.GetBaseCommand(),
		Args:             append([]string(nil), msg.GetArgs()...),
		WorkingDirectory: msg.GetWorkingDirectory(),
		User:             msg.GetUser(),
		NodeID:           msg.GetNodeId(),
		ServiceID:        msg.GetServiceId(),
		StopTimeout:      time.Duration(msg.GetStopTimeoutSeconds()) * time.Second,
	}
	if msg.GetInternalCommand() {
		// Internal commands dispatch to a registered Game implementation
		// (see api/xylona-internal). The supervisor needs a *models.GameServer
		// to pass to the installer; the node has no DB so we synthesize one
		// from the fields the wire carries. Current built-in installers
		// (e.g. Minecraft) only read ID/Name/Directory, so this minimal shape
		// is sufficient. Extend the proto if a future installer needs more.
		cfg.InternalCommand = true
		cfg.InternalGameID = msg.GetInternalGameId()
		cfg.InternalGameServerID = msg.GetId()
		cfg.InternalGameServer = &models.GameServer{
			ID:        msg.GetId(),
			Name:      msg.GetName(),
			Directory: msg.GetWorkingDirectory(),
		}
	}
	_ = ctx // ctx is unused by StartProcess but accepted for future cancellation

	_, errStart := s.n.StartProcess(cfg, xylonaStatusFromProto(msg.GetInitialStatus()))
	if errStart != nil {
		return nil, translate(errStart)
	}
	return connect.NewResponse(&nodeprotov1.StartProcessResponse{ProcessId: cfg.ID}), nil
}

func (s *nodeServiceServer) StopProcess(_ context.Context, req *connect.Request[nodeprotov1.StopProcessRequest]) (*connect.Response[nodeprotov1.StopProcessResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	errStop := s.n.StopProcess(req.Msg.GetProcessId(), req.Msg.GetStopInputCommand())
	if errStop != nil {
		return nil, translate(errStop)
	}
	return connect.NewResponse(&nodeprotov1.StopProcessResponse{}), nil
}

func (s *nodeServiceServer) SendConsoleInput(_ context.Context, req *connect.Request[nodeprotov1.SendConsoleInputRequest]) (*connect.Response[nodeprotov1.SendConsoleInputResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	errSend := s.n.SendConsoleInput(req.Msg.GetProcessId(), req.Msg.GetInput())
	if errSend != nil {
		return nil, translate(errSend)
	}
	return connect.NewResponse(&nodeprotov1.SendConsoleInputResponse{}), nil
}

func (s *nodeServiceServer) ReadConsoleBuffer(_ context.Context, req *connect.Request[nodeprotov1.ReadConsoleBufferRequest]) (*connect.Response[nodeprotov1.ReadConsoleBufferResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	chunk := s.n.ReadConsoleBuffer(req.Msg.GetProcessId())
	return connect.NewResponse(&nodeprotov1.ReadConsoleBufferResponse{
		Chunk: &nodeprotov1.ConsoleChunk{
			GameServerId: chunk.ProcessID,
			Text:         chunk.Data,
			Timestamp:    timestamppb.Now(),
		},
	}), nil
}

func (s *nodeServiceServer) ListFiles(_ context.Context, req *connect.Request[nodeprotov1.ListFilesRequest]) (*connect.Response[nodeprotov1.ListFilesResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}

	entries, errList := s.n.ListFiles(req.Msg.GetDirectory(), req.Msg.GetRelativePath())
	if errList != nil {
		return nil, translate(errList)
	}
	out := make([]*nodeprotov1.FileEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, fileEntryToProto(entry))
	}
	return connect.NewResponse(&nodeprotov1.ListFilesResponse{Entries: out}), nil
}

func (s *nodeServiceServer) ListBindableIPs(_ context.Context, req *connect.Request[nodeprotov1.ListBindableIPsRequest]) (*connect.Response[nodeprotov1.ListBindableIPsResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}

	rawIPs, errIPs := helpers.GetBindableIPs()
	if errIPs != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list bindable IPs: %w", errIPs))
	}

	ips := make([]*nodeprotov1.BindableIP, 0, len(rawIPs))
	for _, rawIP := range rawIPs {
		if rawIP == nil {
			continue
		}
		ips = append(ips, &nodeprotov1.BindableIP{
			Address:  rawIP.String(),
			Usable:   true,
			External: !rawIP.IsPrivate(),
		})
	}

	return connect.NewResponse(&nodeprotov1.ListBindableIPsResponse{Ips: ips}), nil
}

func (s *nodeServiceServer) ReadFile(_ context.Context, req *connect.Request[nodeprotov1.ReadFileRequest]) (*connect.Response[nodeprotov1.ReadFileResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	data, errRead := s.n.ReadFile(req.Msg.GetDirectory(), req.Msg.GetRelativePath())
	if errRead != nil {
		return nil, translate(errRead)
	}
	return connect.NewResponse(&nodeprotov1.ReadFileResponse{Content: data}), nil
}

func (s *nodeServiceServer) StatFile(_ context.Context, req *connect.Request[nodeprotov1.StatFileRequest]) (*connect.Response[nodeprotov1.StatFileResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	entry, errStat := s.n.StatFile(req.Msg.GetDirectory(), req.Msg.GetRelativePath())
	if errStat != nil {
		return nil, translate(errStat)
	}
	return connect.NewResponse(&nodeprotov1.StatFileResponse{Entry: fileEntryToProto(entry)}), nil
}

func (s *nodeServiceServer) StreamFile(ctx context.Context, req *connect.Request[nodeprotov1.StreamFileRequest], stream *connect.ServerStream[nodeprotov1.StreamFileResponse]) error {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return errAuth
	}
	if s.n == nil {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	reader, errOpen := s.n.OpenFile(req.Msg.GetDirectory(), req.Msg.GetRelativePath())
	if errOpen != nil {
		return translate(errOpen)
	}
	return streamFileContent(ctx, reader, stream)
}

func fileEntryToProto(entry node.FileEntry) *nodeprotov1.FileEntry {
	return &nodeprotov1.FileEntry{
		Name:         entry.Name,
		Size:         entry.Size,
		IsDirectory:  entry.IsDirectory,
		IsExecutable: entry.IsExecutable,
		LastModified: timestamppb.New(entry.LastModified),
	}
}

const streamFileChunkBytes = 64 * 1024

func streamFileContent(ctx context.Context, reader io.ReadCloser, stream *connect.ServerStream[nodeprotov1.StreamFileResponse]) (err error) {
	defer func() {
		errClose := reader.Close()
		if err == nil && errClose != nil {
			err = fmt.Errorf("stream file close: %w", errClose)
		}
	}()

	buf := make([]byte, streamFileChunkBytes)
	for {
		errCtx := ctx.Err()
		if errCtx != nil {
			return fmt.Errorf("stream file canceled: %w", errCtx)
		}

		n, errRead := reader.Read(buf)
		if n > 0 {
			content := append([]byte(nil), buf[:n]...)
			errSend := stream.Send(&nodeprotov1.StreamFileResponse{Content: content})
			if errSend != nil {
				return fmt.Errorf("stream file send: %w", errSend)
			}
		}
		if errors.Is(errRead, io.EOF) {
			return nil
		}
		if errRead != nil {
			return fmt.Errorf("stream file read: %w", errRead)
		}
	}
}

type streamWriteFileReader struct {
	ctx    context.Context
	stream *connect.ClientStream[nodeprotov1.StreamWriteFileRequest]
	buffer []byte
}

func (r *streamWriteFileReader) Read(p []byte) (int, error) {
	for len(r.buffer) == 0 {
		errCtx := r.ctx.Err()
		if errCtx != nil {
			return 0, fmt.Errorf("stream write file canceled: %w", errCtx)
		}
		if !r.stream.Receive() {
			errStream := r.stream.Err()
			if errStream != nil {
				return 0, fmt.Errorf("stream write file receive: %w", errStream)
			}
			return 0, io.EOF
		}
		r.buffer = r.stream.Msg().GetContent()
	}

	n := copy(p, r.buffer)
	r.buffer = r.buffer[n:]
	return n, nil
}

// policyFromWriteRequest extracts the controller-supplied protection context
// from a write-path request so the node can run the same
// IsProtectedServerPath check the controller runs. Extracted into a helper so
// the six handlers below stay uniform.
func policyFromFields(serverExecutable, baseCommand string) node.ProtectionPolicy {
	return node.ProtectionPolicy{
		ServerExecutable: serverExecutable,
		BaseCommand:      baseCommand,
	}
}

func (s *nodeServiceServer) WriteFile(_ context.Context, req *connect.Request[nodeprotov1.WriteFileRequest]) (*connect.Response[nodeprotov1.WriteFileResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	policy := policyFromFields(req.Msg.GetServerExecutable(), req.Msg.GetBaseCommand())
	errWrite := s.n.WriteFile(req.Msg.GetDirectory(), req.Msg.GetRelativePath(), req.Msg.GetContent(), policy)
	if errWrite != nil {
		return nil, translate(errWrite)
	}
	return connect.NewResponse(&nodeprotov1.WriteFileResponse{}), nil
}

func (s *nodeServiceServer) StreamWriteFile(ctx context.Context, stream *connect.ClientStream[nodeprotov1.StreamWriteFileRequest]) (*connect.Response[nodeprotov1.StreamWriteFileResponse], error) {
	errAuth := s.authorize(stream.RequestHeader())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	if !stream.Receive() {
		errStream := stream.Err()
		if errStream != nil {
			return nil, connect.NewError(connect.CodeInternal, errStream)
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("stream write file requires an initial message"))
	}

	first := stream.Msg()
	policy := policyFromFields(first.GetServerExecutable(), first.GetBaseCommand())
	reader := &streamWriteFileReader{
		ctx:    ctx,
		stream: stream,
		buffer: append([]byte(nil), first.GetContent()...),
	}
	result, errWrite := s.n.WriteFileFromReader(first.GetDirectory(), first.GetRelativePath(), reader, policy)
	if errWrite != nil {
		return nil, translate(errWrite)
	}
	return connect.NewResponse(&nodeprotov1.StreamWriteFileResponse{
		BytesWritten: result.BytesWritten,
		Sha256:       result.SHA256,
	}), nil
}

func (s *nodeServiceServer) CreateFileOrDirectory(_ context.Context, req *connect.Request[nodeprotov1.CreateFileOrDirectoryRequest]) (*connect.Response[nodeprotov1.CreateFileOrDirectoryResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	policy := policyFromFields(req.Msg.GetServerExecutable(), req.Msg.GetBaseCommand())
	errCreate := s.n.CreateFileOrDirectory(req.Msg.GetDirectory(), req.Msg.GetRelativePath(), req.Msg.GetContent(), req.Msg.GetIsDirectory(), policy)
	if errCreate != nil {
		return nil, translate(errCreate)
	}
	return connect.NewResponse(&nodeprotov1.CreateFileOrDirectoryResponse{}), nil
}

func (s *nodeServiceServer) DeleteFiles(ctx context.Context, req *connect.Request[nodeprotov1.DeleteFilesRequest]) (*connect.Response[nodeprotov1.DeleteFilesResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	policy := policyFromFields(req.Msg.GetServerExecutable(), req.Msg.GetBaseCommand())
	deleted, errDelete := s.n.DeleteFiles(ctx, req.Msg.GetDirectory(), req.Msg.GetFiles(), policy)
	if errDelete != nil {
		return nil, translate(errDelete)
	}
	return connect.NewResponse(&nodeprotov1.DeleteFilesResponse{Deleted: deleted}), nil
}

func (s *nodeServiceServer) RenameFile(_ context.Context, req *connect.Request[nodeprotov1.RenameFileRequest]) (*connect.Response[nodeprotov1.RenameFileResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	policy := policyFromFields(req.Msg.GetServerExecutable(), req.Msg.GetBaseCommand())
	newPath, errRename := s.n.RenameFile(req.Msg.GetDirectory(), req.Msg.GetOldRelativePath(), req.Msg.GetNewRelativePath(), policy)
	if errRename != nil {
		return nil, translate(errRename)
	}
	return connect.NewResponse(&nodeprotov1.RenameFileResponse{NewRelativePath: newPath}), nil
}

func (s *nodeServiceServer) MoveFiles(ctx context.Context, req *connect.Request[nodeprotov1.MoveFilesRequest]) (*connect.Response[nodeprotov1.MoveFilesResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	policy := policyFromFields(req.Msg.GetServerExecutable(), req.Msg.GetBaseCommand())
	moved, errMove := s.n.MoveFiles(ctx, req.Msg.GetDirectory(), req.Msg.GetFiles(), req.Msg.GetDestination(), policy)
	if errMove != nil {
		return nil, translate(errMove)
	}
	return connect.NewResponse(&nodeprotov1.MoveFilesResponse{Moved: moved}), nil
}

func (s *nodeServiceServer) CopyFiles(ctx context.Context, req *connect.Request[nodeprotov1.CopyFilesRequest]) (*connect.Response[nodeprotov1.CopyFilesResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	operations := make([]node.CopyFileOperation, 0, len(req.Msg.GetOperations()))
	for _, operation := range req.Msg.GetOperations() {
		operations = append(operations, node.CopyFileOperation{
			SourceRelativePath:      operation.GetSourceRelativePath(),
			DestinationRelativePath: operation.GetDestinationRelativePath(),
		})
	}
	policy := policyFromFields(req.Msg.GetServerExecutable(), req.Msg.GetBaseCommand())
	copied, errCopy := s.n.CopyFiles(ctx, req.Msg.GetDirectory(), operations, policy)
	if errCopy != nil {
		return nil, translate(errCopy)
	}
	return connect.NewResponse(&nodeprotov1.CopyFilesResponse{Copied: copied}), nil
}

func (s *nodeServiceServer) DownloadFileFromURL(ctx context.Context, req *connect.Request[nodeprotov1.DownloadFileFromURLRequest]) (*connect.Response[nodeprotov1.DownloadFileFromURLResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	policy := policyFromFields(req.Msg.GetServerExecutable(), req.Msg.GetBaseCommand())
	result, errDownload := s.n.DownloadFileFromURL(ctx, req.Msg.GetDirectory(), req.Msg.GetUrl(), req.Msg.GetDestinationDirectoryPath(), node.DownloadIntegrity{
		ExpectedSize:   req.Msg.GetExpectedSize(),
		ExpectedSHA256: req.Msg.GetExpectedSha256(),
		ExpectedSHA1:   req.Msg.GetExpectedSha1(),
	}, policy)
	if errDownload != nil {
		return nil, translate(errDownload)
	}
	return connect.NewResponse(&nodeprotov1.DownloadFileFromURLResponse{
		RelativePath:  result.RelativePath,
		BytesWritten:  result.BytesWritten,
		Sha256:        result.SHA256,
		Sha1:          result.SHA1,
		ExpectedMatch: result.ExpectedMatch,
	}), nil
}

func (s *nodeServiceServer) CreateFileArchive(ctx context.Context, req *connect.Request[nodeprotov1.CreateFileArchiveRequest]) (*connect.Response[nodeprotov1.CreateFileArchiveResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	policy := policyFromFields(req.Msg.GetServerExecutable(), req.Msg.GetBaseCommand())
	compression := nodeArchiveCompressionFromProto(req.Msg.GetCompression())
	archivePath, progress, errArchive := s.n.CreateFileArchive(ctx, req.Msg.GetDirectory(), req.Msg.GetDestinationArchivePath(), req.Msg.GetIncludePaths(), compression, policy)
	if errArchive != nil {
		return nil, translate(errArchive)
	}
	return connect.NewResponse(fileArchiveResponse(archivePath, progress)), nil
}

func (s *nodeServiceServer) StreamCreateFileArchive(ctx context.Context, req *connect.Request[nodeprotov1.CreateFileArchiveRequest], stream *connect.ServerStream[nodeprotov1.CreateFileArchiveResponse]) error {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return errAuth
	}
	if s.n == nil {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	policy := policyFromFields(req.Msg.GetServerExecutable(), req.Msg.GetBaseCommand())
	compression := nodeArchiveCompressionFromProto(req.Msg.GetCompression())
	onProgress := func(progress node.ArchiveProgress) error {
		errSend := stream.Send(fileArchiveResponse("", progress))
		if errSend != nil {
			return fmt.Errorf("send file archive progress: %w", errSend)
		}
		return nil
	}
	archivePath, progress, errArchive := s.n.CreateFileArchiveWithProgress(ctx, req.Msg.GetDirectory(), req.Msg.GetDestinationArchivePath(), req.Msg.GetIncludePaths(), compression, policy, onProgress)
	if errArchive != nil {
		return translate(errArchive)
	}
	errSend := stream.Send(fileArchiveResponse(archivePath, progress))
	if errSend != nil {
		return connect.NewError(connect.CodeInternal, errSend)
	}
	return nil
}

func (s *nodeServiceServer) ExtractFileArchive(ctx context.Context, req *connect.Request[nodeprotov1.ExtractFileArchiveRequest]) (*connect.Response[nodeprotov1.ExtractFileArchiveResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	policy := policyFromFields(req.Msg.GetServerExecutable(), req.Msg.GetBaseCommand())
	extracted, progress, errExtract := s.n.ExtractFileArchive(ctx, req.Msg.GetDirectory(), req.Msg.GetArchivePath(), req.Msg.GetDestinationDirectoryPath(), policy)
	if errExtract != nil {
		return nil, translate(errExtract)
	}
	return connect.NewResponse(fileExtractResponse(extracted, progress)), nil
}

func (s *nodeServiceServer) StreamExtractFileArchive(ctx context.Context, req *connect.Request[nodeprotov1.ExtractFileArchiveRequest], stream *connect.ServerStream[nodeprotov1.ExtractFileArchiveResponse]) error {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return errAuth
	}
	if s.n == nil {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	policy := policyFromFields(req.Msg.GetServerExecutable(), req.Msg.GetBaseCommand())
	onProgress := func(progress node.ExtractProgress) error {
		errSend := stream.Send(fileExtractResponse(nil, progress))
		if errSend != nil {
			return fmt.Errorf("send file extract progress: %w", errSend)
		}
		return nil
	}
	extracted, progress, errExtract := s.n.ExtractFileArchiveWithProgress(ctx, req.Msg.GetDirectory(), req.Msg.GetArchivePath(), req.Msg.GetDestinationDirectoryPath(), policy, onProgress)
	if errExtract != nil {
		return translate(errExtract)
	}
	errSend := stream.Send(fileExtractResponse(extracted, progress))
	if errSend != nil {
		return connect.NewError(connect.CodeInternal, errSend)
	}
	return nil
}

func (s *nodeServiceServer) CreateBackupArchive(ctx context.Context, req *connect.Request[nodeprotov1.CreateBackupArchiveRequest]) (*connect.Response[nodeprotov1.CreateBackupArchiveResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	archiveBytes, archiveSHA256, errArchive := s.n.CreateBackupArchive(ctx, req.Msg.GetDirectory(), req.Msg.GetIncludePaths(), req.Msg.GetDestinationArchivePath())
	if errArchive != nil {
		return nil, translate(errArchive)
	}
	return connect.NewResponse(&nodeprotov1.CreateBackupArchiveResponse{
		ArchiveBytes:  archiveBytes,
		ArchiveSha256: archiveSHA256,
	}), nil
}

func (s *nodeServiceServer) ExtractBackupArchive(ctx context.Context, req *connect.Request[nodeprotov1.ExtractBackupArchiveRequest]) (*connect.Response[nodeprotov1.ExtractBackupArchiveResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	mode := extractModeFromProto(req.Msg.GetMode())
	errExtract := s.n.ExtractBackupArchive(ctx, req.Msg.GetDirectory(), req.Msg.GetArchivePath(), mode)
	if errExtract != nil {
		return nil, translate(errExtract)
	}
	return connect.NewResponse(&nodeprotov1.ExtractBackupArchiveResponse{}), nil
}

func (s *nodeServiceServer) ProbeInstalledVersion(_ context.Context, req *connect.Request[nodeprotov1.ProbeInstalledVersionRequest]) (*connect.Response[nodeprotov1.ProbeInstalledVersionResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	result, errProbe := s.n.ProbeInstalledVersion(node.InstalledVersionProbeRequest{
		Directory:           req.Msg.GetDirectory(),
		Kind:                nodeInstalledVersionProbeKindFromProto(req.Msg.GetKind()),
		RelativePaths:       append([]string(nil), req.Msg.GetRelativePaths()...),
		PreferredSteamAppID: req.Msg.GetPreferredSteamAppId(),
	})
	if errProbe != nil {
		return nil, translate(errProbe)
	}
	return connect.NewResponse(&nodeprotov1.ProbeInstalledVersionResponse{
		Found:      result.Found,
		Version:    result.Version,
		SourcePath: result.SourcePath,
	}), nil
}

func (s *nodeServiceServer) QueryGameServer(ctx context.Context, req *connect.Request[nodeprotov1.QueryGameServerRequest]) (*connect.Response[nodeprotov1.QueryGameServerResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	result, errQuery := s.n.QueryGameServer(ctx, node.GameServerQueryRequest{
		Kind:       nodeGameServerQueryKindFromProto(req.Msg.GetKind()),
		IP:         req.Msg.GetIp(),
		QueryPort:  req.Msg.GetQueryPort(),
		MaxPlayers: req.Msg.GetMaxPlayers(),
	})
	if errQuery != nil {
		return nil, translate(errQuery)
	}
	return connect.NewResponse(&nodeprotov1.QueryGameServerResponse{
		Kind:      gameServerQueryKindToProto(result.Kind),
		Minecraft: minecraftQueryToProto(result.Minecraft),
		Source:    sourceQueryToProto(result.Source),
	}), nil
}

func (s *nodeServiceServer) SendConsoleOutput(_ context.Context, req *connect.Request[nodeprotov1.SendConsoleOutputRequest]) (*connect.Response[nodeprotov1.SendConsoleOutputResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	errSend := s.n.SendConsoleOutput(req.Msg.GetProcessId(), req.Msg.GetLine())
	if errSend != nil {
		return nil, translate(errSend)
	}
	return connect.NewResponse(&nodeprotov1.SendConsoleOutputResponse{}), nil
}

func (s *nodeServiceServer) GetProcessSnapshot(_ context.Context, req *connect.Request[nodeprotov1.GetProcessSnapshotRequest]) (*connect.Response[nodeprotov1.GetProcessSnapshotResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	snap, found, errSnap := s.n.GetProcessSnapshot(req.Msg.GetProcessId())
	if errSnap != nil {
		return nil, translate(errSnap)
	}
	resp := &nodeprotov1.GetProcessSnapshotResponse{Found: found}
	if found && snap != nil {
		resp.Snapshot = processSnapshotToProto(snap)
	}
	return connect.NewResponse(resp), nil
}

func fileArchiveResponse(archivePath string, progress node.ArchiveProgress) *nodeprotov1.CreateFileArchiveResponse {
	return &nodeprotov1.CreateFileArchiveResponse{
		RelativePath:    archivePath,
		TotalFiles:      progress.TotalFiles,
		FilesCompressed: progress.FilesCompressed,
		TotalBytes:      progress.TotalBytes,
		BytesCompressed: progress.BytesCompressed,
		CurrentFile:     progress.CurrentFile,
	}
}

func fileExtractResponse(extractedPaths []string, progress node.ExtractProgress) *nodeprotov1.ExtractFileArchiveResponse {
	return &nodeprotov1.ExtractFileArchiveResponse{
		ExtractedPaths: extractedPaths,
		TotalFiles:     progress.TotalFiles,
		FilesExtracted: progress.FilesExtracted,
		TotalBytes:     progress.TotalBytes,
		BytesExtracted: progress.BytesExtracted,
		CurrentFile:    progress.CurrentFile,
	}
}

func nodeArchiveCompressionFromProto(compression nodeprotov1.FileArchiveCompression) node.ArchiveCompression {
	switch compression {
	case nodeprotov1.FileArchiveCompression_FILE_ARCHIVE_COMPRESSION_BZIP2:
		return node.ArchiveCompressionBZIP2
	case nodeprotov1.FileArchiveCompression_FILE_ARCHIVE_COMPRESSION_GZIP:
		return node.ArchiveCompressionGZIP
	case nodeprotov1.FileArchiveCompression_FILE_ARCHIVE_COMPRESSION_ZST:
		return node.ArchiveCompressionZST
	case nodeprotov1.FileArchiveCompression_FILE_ARCHIVE_COMPRESSION_XZ:
		return node.ArchiveCompressionXZ
	default:
		return node.ArchiveCompressionZIP
	}
}

func extractModeFromProto(m nodeprotov1.ExtractMode) node.ExtractMode {
	if m == nodeprotov1.ExtractMode_EXTRACT_MODE_EXACT {
		return node.ExtractModeExact
	}
	return node.ExtractModeOverlay
}

func nodeInstalledVersionProbeKindFromProto(kind nodeprotov1.InstalledVersionProbeKind) node.InstalledVersionProbeKind {
	switch kind {
	case nodeprotov1.InstalledVersionProbeKind_INSTALLED_VERSION_PROBE_KIND_MINECRAFT_JAR:
		return node.InstalledVersionProbeKindMinecraftJar
	case nodeprotov1.InstalledVersionProbeKind_INSTALLED_VERSION_PROBE_KIND_STEAM_MANIFEST:
		return node.InstalledVersionProbeKindSteamManifest
	default:
		return node.InstalledVersionProbeKindUnspecified
	}
}

func nodeGameServerQueryKindFromProto(kind nodeprotov1.GameServerQueryKind) node.GameServerQueryKind {
	switch kind {
	case nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_MINECRAFT:
		return node.GameServerQueryKindMinecraft
	case nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_SOURCE:
		return node.GameServerQueryKindSource
	default:
		return node.GameServerQueryKindUnknown
	}
}

func gameServerQueryKindToProto(kind node.GameServerQueryKind) nodeprotov1.GameServerQueryKind {
	switch kind {
	case node.GameServerQueryKindMinecraft:
		return nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_MINECRAFT
	case node.GameServerQueryKindSource:
		return nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_SOURCE
	default:
		return nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_UNSPECIFIED
	}
}

func minecraftQueryToProto(info *node.MinecraftQueryInfo) *nodeprotov1.GameServerMinecraftQueryInfo {
	if info == nil {
		return nil
	}
	return &nodeprotov1.GameServerMinecraftQueryInfo{
		Motd:            info.MOTD,
		GameType:        info.GameType,
		Map:             info.Map,
		NumberOfPlayers: info.NumberOfPlayers,
		MaxPlayers:      info.MaxPlayers,
		PlayerList:      append([]string(nil), info.PlayerList...),
		ProtocolVersion: info.ProtocolVersion,
		ServerVersion:   info.ServerVersion,
	}
}

func sourceQueryToProto(info *node.SourceQueryInfo) *nodeprotov1.GameServerSourceQueryInfo {
	if info == nil {
		return nil
	}
	return &nodeprotov1.GameServerSourceQueryInfo{
		Name:       info.Name,
		Map:        info.Map,
		Game:       info.Game,
		AppId:      info.AppID,
		SteamId:    info.SteamID,
		GameId:     info.GameID,
		Players:    info.Players,
		MaxPlayers: info.MaxPlayers,
		Bots:       info.Bots,
		ServerOs:   info.ServerOS,
		Visibility: info.Visibility,
		Vac:        info.VAC,
		Version:    info.Version,
		Protocol:   info.Protocol,
	}
}

func processSnapshotToProto(p *node.ProcessSnapshot) *nodeprotov1.ProcessSnapshot {
	if p == nil {
		return &nodeprotov1.ProcessSnapshot{}
	}
	return &nodeprotov1.ProcessSnapshot{
		Id:              p.ID,
		Name:            p.Name,
		Status:          p.Status,
		UnixStartedAt:   p.UnixStartedAt,
		CpuPercent:      p.CPUPercent,
		CpuCores:        p.CPUCores,
		MemoryRss:       p.MemoryRSS,
		MemoryVms:       p.MemoryVMS,
		MemoryPercent:   p.MemoryPercent,
		NumThreads:      p.NumThreads,
		DiskUsageBytes:  p.DiskUsageBytes,
		IoReadRate:      p.IOReadRate,
		IoWriteRate:     p.IOWriteRate,
		ConnectionCount: p.ConnectionCount,
		WorkingDir:      p.WorkingDir,
	}
}

func (s *nodeServiceServer) GetNodeSnapshot(ctx context.Context, req *connect.Request[nodeprotov1.GetNodeSnapshotRequest]) (*connect.Response[nodeprotov1.NodeSnapshot], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	snap, errSnap := s.n.GetNodeSnapshot(ctx)
	if errSnap != nil {
		return nil, translate(errSnap)
	}
	return connect.NewResponse(nodeSnapshotToProto(snap)), nil
}

func (s *nodeServiceServer) StreamEvents(ctx context.Context, req *connect.Request[nodeprotov1.StreamEventsRequest], stream *connect.ServerStream[nodeprotov1.Event]) error {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return errAuth
	}
	if s.n == nil {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}

	emitter := s.n.Events()
	if emitter == nil {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("node has no event emitter"))
	}

	subscription := emitter.Subscribe()
	defer emitter.Unsubscribe(subscription)

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-subscription:
			if !ok {
				return nil
			}
			errSend := stream.Send(nodeEventToProto(event))
			if errSend != nil {
				return fmt.Errorf("stream events send: %w", errSend)
			}
		}
	}
}

// StreamConsoleOutput is the per-process console stream.
func (s *nodeServiceServer) StreamConsoleOutput(ctx context.Context, req *connect.Request[nodeprotov1.StreamConsoleOutputRequest], stream *connect.ServerStream[nodeprotov1.ConsoleChunk]) error {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return errAuth
	}

	if s.n == nil {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}

	chunks, errStream := s.n.StreamConsoleOutput(ctx, req.Msg.GetProcessId())
	if errStream != nil {
		return connect.NewError(connect.CodeFailedPrecondition, errStream)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case chunk, ok := <-chunks:
			if !ok {
				return nil
			}
			errSend := stream.Send(&nodeprotov1.ConsoleChunk{
				GameServerId: chunk.ProcessID,
				Text:         chunk.Data,
				Timestamp:    timestamppb.Now(),
			})
			if errSend != nil {
				return fmt.Errorf("stream console output send: %w", errSend)
			}
		}
	}
}

func (s *nodeServiceServer) Ping(_ context.Context, req *connect.Request[nodeprotov1.PingRequest]) (*connect.Response[nodeprotov1.PingResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	return connect.NewResponse(&nodeprotov1.PingResponse{ServerTime: timestamppb.Now()}), nil
}

func (s *nodeServiceServer) GetUpdateCapabilities(_ context.Context, req *connect.Request[nodeprotov1.GetUpdateCapabilitiesRequest]) (*connect.Response[nodeprotov1.GetUpdateCapabilitiesResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.updater == nil {
		return connect.NewResponse(&nodeprotov1.GetUpdateCapabilitiesResponse{
			Supported: false,
			Reason:    "node binary does not expose self-update support",
			Component: "node",
		}), nil
	}
	caps := s.updater.Capabilities()
	return connect.NewResponse(&nodeprotov1.GetUpdateCapabilitiesResponse{
		Supported:               caps.Supported,
		Reason:                  caps.Reason,
		Component:               caps.Component,
		CurrentVersion:          caps.CurrentVersion,
		Os:                      caps.OS,
		Architecture:            caps.Architecture,
		ProtocolVersion:         caps.ProtocolVersion,
		ServiceManagerSupported: caps.ServiceManagerSupported,
		InstallPathWritable:     caps.InstallPathWritable,
		InstallPath:             caps.InstallPath,
	}), nil
}

type stageSelfUpdateReader struct {
	ctx    context.Context
	stream *connect.ClientStream[nodeprotov1.StageSelfUpdateRequest]
	buffer []byte
}

func (r *stageSelfUpdateReader) Read(p []byte) (int, error) {
	for len(r.buffer) == 0 {
		errCtx := r.ctx.Err()
		if errCtx != nil {
			return 0, fmt.Errorf("stage self-update canceled: %w", errCtx)
		}
		if !r.stream.Receive() {
			errStream := r.stream.Err()
			if errStream != nil {
				return 0, fmt.Errorf("stage self-update receive: %w", errStream)
			}
			return 0, io.EOF
		}
		r.buffer = r.stream.Msg().GetContent()
	}

	n := copy(p, r.buffer)
	r.buffer = r.buffer[n:]
	return n, nil
}

func (s *nodeServiceServer) StageSelfUpdate(ctx context.Context, stream *connect.ClientStream[nodeprotov1.StageSelfUpdateRequest]) (*connect.Response[nodeprotov1.StageSelfUpdateResponse], error) {
	errAuth := s.authorize(stream.RequestHeader())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.updater == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("self-update is not supported by this node"))
	}
	if !stream.Receive() {
		errStream := stream.Err()
		if errStream != nil {
			return nil, connect.NewError(connect.CodeInternal, errStream)
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("stage self-update requires an initial metadata message"))
	}

	first := stream.Msg()
	reader := &stageSelfUpdateReader{
		ctx:    ctx,
		stream: stream,
		buffer: append([]byte(nil), first.GetContent()...),
	}
	result, errStage := s.updater.Stage(ctx, node.StageSelfUpdateRequest{
		Component:      first.GetComponent(),
		TargetVersion:  first.GetTargetVersion(),
		OS:             first.GetOs(),
		Architecture:   first.GetArchitecture(),
		ExpectedSize:   first.GetExpectedSize(),
		ExpectedSHA256: first.GetExpectedSha256(),
		Reader:         reader,
	})
	if errStage != nil {
		return nil, translate(errStage)
	}
	return connect.NewResponse(&nodeprotov1.StageSelfUpdateResponse{
		StageId:      result.StageID,
		BytesWritten: result.BytesWritten,
		Sha256:       result.SHA256,
	}), nil
}

func (s *nodeServiceServer) ApplySelfUpdate(ctx context.Context, req *connect.Request[nodeprotov1.ApplySelfUpdateRequest]) (*connect.Response[nodeprotov1.ApplySelfUpdateResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.updater == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("self-update is not supported by this node"))
	}
	result, errApply := s.updater.Apply(ctx, node.ApplySelfUpdateRequest{
		StageID:        req.Msg.GetStageId(),
		TargetVersion:  req.Msg.GetTargetVersion(),
		ExpectedSHA256: req.Msg.GetExpectedSha256(),
	})
	if errApply != nil {
		return nil, translate(errApply)
	}
	return connect.NewResponse(&nodeprotov1.ApplySelfUpdateResponse{
		Accepted: result.Accepted,
		Message:  result.Message,
	}), nil
}

// nodeEventToProto converts a node.Event to its wire form.
func nodeEventToProto(ev node.Event) *nodeprotov1.Event {
	out := &nodeprotov1.Event{
		Timestamp: timestamppb.New(ev.Timestamp),
	}
	switch ev.Type {
	case node.EventTypeProcessStatus:
		out.Payload = &nodeprotov1.Event_ProcessStatus{
			ProcessStatus: &nodeprotov1.ProcessStatusEvent{
				ProcessId: ev.ProcessID,
				Status:    ev.Status,
			},
		}
	case node.EventTypeConsoleOutput:
		chunk, _ := ev.Payload.(node.ConsoleChunk)
		out.Payload = &nodeprotov1.Event_ConsoleOutput{
			ConsoleOutput: &nodeprotov1.ConsoleChunk{
				GameServerId: chunk.ProcessID,
				Text:         chunk.Data,
				Timestamp:    out.GetTimestamp(),
			},
		}
	case node.EventTypeMetrics:
		snap, _ := ev.Payload.(*node.NodeSnapshot)
		out.Payload = &nodeprotov1.Event_MetricsUpdate{
			MetricsUpdate: &nodeprotov1.MetricsUpdateEvent{
				Snapshot: nodeSnapshotToProto(snap),
			},
		}
	}
	return out
}
