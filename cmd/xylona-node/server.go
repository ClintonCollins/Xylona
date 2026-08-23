package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/launchenv"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/selfupdate"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1/nodeprotoconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// nodeServiceServer is the Connect-RPC handler implementation that wraps a
// *internal/node.Node. Every method validates the bearer token, translates proto
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

// translate translates a Go-side error into a Connect error. internal/node sentinel
// errors are mapped to explicit Connect codes; everything else surfaces as
// CodeInternal so the controller's retry/back-off logic can react uniformly.
func translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, node.ErrInvalidPath) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if errors.Is(err, node.ErrInvalidPlayerAction) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if errors.Is(err, node.ErrPlayerActionUnsupported) {
		return connect.NewError(connect.CodeUnimplemented, err)
	}
	if errors.Is(err, node.ErrPlayerActionUnavailable) {
		return connect.NewError(connect.CodeUnavailable, err)
	}
	validationError := &launchenv.ValidationError{}
	if errors.As(err, &validationError) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if errors.Is(err, os.ErrNotExist) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if errors.Is(err, node.ErrProcessNotFound) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if errors.Is(err, node.ErrConsoleInputRejected) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if errors.Is(err, node.ErrConsoleInputUnavailable) {
		return connect.NewError(connect.CodeFailedPrecondition, err)
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
// nodeSnapshotFromProto (internal/nodeclient/grpc.go).
func nodeSnapshotToProto(snap *node.NodeSnapshot) *nodeprotov1.NodeSnapshot {
	if snap == nil {
		return &nodeprotov1.NodeSnapshot{}
	}
	processes := make([]*nodeprotov1.ProcessSnapshot, 0, len(snap.Processes))
	for _, p := range snap.Processes {
		processes = append(processes, processSnapshotToProto(&p))
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
		ExecutionID:      msg.GetExecutionId(),
		Name:             msg.GetName(),
		BaseCommand:      msg.GetBaseCommand(),
		Args:             append([]string(nil), msg.GetArgs()...),
		WorkingDirectory: msg.GetWorkingDirectory(),
		User:             msg.GetUser(),
		NodeID:           msg.GetNodeId(),
		ServiceID:        msg.GetServiceId(),
		StopTimeout:      time.Duration(msg.GetStopTimeoutSeconds()) * time.Second,
		LaunchEnv:        cloneStringMap(msg.GetLaunchEnv()),
	}
	telnetInput := msg.GetTelnetInput()
	if telnetInput != nil {
		cfg.InputTelnet = &node.TelnetInput{
			Port:     int(telnetInput.GetPort()),
			Password: telnetInput.GetPassword(),
		}
	}
	rconInput := msg.GetRconInput()
	if rconInput != nil {
		protocol, errProtocol := nodeRCONProtocolFromProto(rconInput.GetProtocol())
		if errProtocol != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errProtocol)
		}
		cfg.InputRCON = &node.RCONInput{
			Host:     rconInput.GetHost(),
			Port:     int(rconInput.GetPort()),
			Password: rconInput.GetPassword(),
			Protocol: protocol,
		}
	}
	restInput := msg.GetRestInput()
	if restInput != nil {
		kind, errKind := nodeRESTInputKindFromProto(restInput.GetKind())
		if errKind != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errKind)
		}
		cfg.InputREST = &node.RESTInput{
			Host:              restInput.GetHost(),
			Port:              int(restInput.GetPort()),
			Kind:              kind,
			Password:          restInput.GetPassword(),
			PreviousPasswords: slices.Clone(restInput.GetPreviousPasswords()),
		}
	}
	if msg.GetInternalCommand() {
		// Internal commands dispatch to a registered Game implementation
		// (see internal/gameintegrations). The supervisor needs a *models.GameServer
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

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	return maps.Clone(values)
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

func (s *nodeServiceServer) SendConsoleInput(ctx context.Context, req *connect.Request[nodeprotov1.SendConsoleInputRequest]) (*connect.Response[nodeprotov1.SendConsoleInputResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	errSend := s.n.SendConsoleInputContext(ctx, req.Msg.GetProcessId(), req.Msg.GetInput())
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
			Sequence:     chunk.Sequence,
			ResetBuffer:  chunk.ResetBuffer,
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
		Username:   req.Msg.GetUsername(),
		Password:   req.Msg.GetPassword(),
	})
	if errQuery != nil {
		return nil, translate(errQuery)
	}
	return connect.NewResponse(&nodeprotov1.QueryGameServerResponse{
		Kind:      gameServerQueryKindToProto(result.Kind),
		Minecraft: minecraftQueryToProto(result.Minecraft),
		Source:    sourceQueryToProto(result.Source),
		Palworld:  palworldQueryToProto(result.Palworld),
	}), nil
}

func (s *nodeServiceServer) QueryPalworldMap(ctx context.Context, req *connect.Request[nodeprotov1.QueryPalworldMapRequest]) (*connect.Response[nodeprotov1.QueryPalworldMapResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	snapshot, errQuery := s.n.QueryPalworldMap(ctx, node.PalworldMapQueryRequest{
		IP:        req.Msg.GetIp(),
		QueryPort: req.Msg.GetQueryPort(),
		Username:  req.Msg.GetUsername(),
		Password:  req.Msg.GetPassword(),
	})
	if errQuery != nil {
		return nil, translate(errQuery)
	}
	return connect.NewResponse(&nodeprotov1.QueryPalworldMapResponse{
		Snapshot: palworldMapSnapshotToProto(snapshot),
	}), nil
}

func (s *nodeServiceServer) QuerySevenDaysToDieMap(ctx context.Context, req *connect.Request[nodeprotov1.QuerySevenDaysToDieMapRequest]) (*connect.Response[nodeprotov1.QuerySevenDaysToDieMapResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	snapshot, errQuery := s.n.QuerySevenDaysToDieMap(ctx, node.SevenDaysToDieMapQueryRequest{
		WorkingDirectory: req.Msg.GetWorkingDirectory(),
		TokenName:        req.Msg.GetTokenName(),
		TokenSecret:      req.Msg.GetTokenSecret(),
		IncludeTactical:  req.Msg.GetIncludeTactical(),
	})
	if errQuery != nil {
		return nil, translate(errQuery)
	}
	return connect.NewResponse(&nodeprotov1.QuerySevenDaysToDieMapResponse{
		Snapshot: sevenDaysToDieMapSnapshotToProto(snapshot),
	}), nil
}

func (s *nodeServiceServer) QuerySevenDaysToDieWebAPIStatus(ctx context.Context, req *connect.Request[nodeprotov1.QuerySevenDaysToDieWebAPIStatusRequest]) (*connect.Response[nodeprotov1.QuerySevenDaysToDieWebAPIStatusResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	status, errQuery := s.n.QuerySevenDaysToDieWebAPIStatus(ctx, node.SevenDaysToDieWebAPIStatusQueryRequest{
		WorkingDirectory: req.Msg.GetWorkingDirectory(),
		TokenName:        req.Msg.GetTokenName(),
		TokenSecret:      req.Msg.GetTokenSecret(),
		IncludeTactical:  req.Msg.GetIncludeTactical(),
	})
	if errQuery != nil {
		return nil, translate(errQuery)
	}
	return connect.NewResponse(&nodeprotov1.QuerySevenDaysToDieWebAPIStatusResponse{
		Status: sevenDaysToDieWebAPIStatusToProto(status),
	}), nil
}

func (s *nodeServiceServer) QuerySevenDaysToDiePlayers(ctx context.Context, req *connect.Request[nodeprotov1.QuerySevenDaysToDiePlayersRequest]) (*connect.Response[nodeprotov1.QuerySevenDaysToDiePlayersResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	result, errQuery := s.n.QuerySevenDaysToDiePlayers(ctx, node.SevenDaysToDiePlayersQueryRequest{
		WorkingDirectory: req.Msg.GetWorkingDirectory(),
		TokenName:        req.Msg.GetTokenName(),
		TokenSecret:      req.Msg.GetTokenSecret(),
	})
	if errQuery != nil {
		return nil, translate(errQuery)
	}
	return connect.NewResponse(&nodeprotov1.QuerySevenDaysToDiePlayersResponse{
		Result: sevenDaysToDiePlayersToProto(result),
	}), nil
}

func (s *nodeServiceServer) QuerySevenDaysToDieReportedMods(ctx context.Context, req *connect.Request[nodeprotov1.QuerySevenDaysToDieReportedModsRequest]) (*connect.Response[nodeprotov1.QuerySevenDaysToDieReportedModsResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	result, errQuery := s.n.QuerySevenDaysToDieReportedMods(ctx, node.SevenDaysToDieReportedModsQueryRequest{
		WorkingDirectory: req.Msg.GetWorkingDirectory(),
		TokenName:        req.Msg.GetTokenName(),
		TokenSecret:      req.Msg.GetTokenSecret(),
	})
	if errQuery != nil {
		return nil, translate(errQuery)
	}
	return connect.NewResponse(&nodeprotov1.QuerySevenDaysToDieReportedModsResponse{
		Result: sevenDaysToDieReportedModsToProto(result),
	}), nil
}

func (s *nodeServiceServer) QuerySevenDaysToDieSandboxSettings(ctx context.Context, req *connect.Request[nodeprotov1.QuerySevenDaysToDieSandboxSettingsRequest]) (*connect.Response[nodeprotov1.QuerySevenDaysToDieSandboxSettingsResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	result, errQuery := s.n.QuerySevenDaysToDieSandboxSettings(ctx, node.SevenDaysToDieSandboxSettingsQueryRequest{
		WorkingDirectory: req.Msg.GetWorkingDirectory(),
		TokenName:        req.Msg.GetTokenName(),
		TokenSecret:      req.Msg.GetTokenSecret(),
	})
	if errQuery != nil {
		return nil, translate(errQuery)
	}
	return connect.NewResponse(&nodeprotov1.QuerySevenDaysToDieSandboxSettingsResponse{
		Result: sevenDaysToDieSandboxSettingsToProto(result),
	}), nil
}

func (s *nodeServiceServer) GetSevenDaysToDieMapTile(ctx context.Context, req *connect.Request[nodeprotov1.GetSevenDaysToDieMapTileRequest]) (*connect.Response[nodeprotov1.GetSevenDaysToDieMapTileResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	content, errTile := s.n.GetSevenDaysToDieMapTile(ctx, node.SevenDaysToDieMapTileRequest{
		WorkingDirectory: req.Msg.GetWorkingDirectory(),
		TokenName:        req.Msg.GetTokenName(),
		TokenSecret:      req.Msg.GetTokenSecret(),
		Zoom:             req.Msg.GetZoom(),
		X:                req.Msg.GetX(),
		Y:                req.Msg.GetY(),
	})
	if errTile != nil {
		return nil, translate(errTile)
	}
	return connect.NewResponse(&nodeprotov1.GetSevenDaysToDieMapTileResponse{Content: content}), nil
}

func (s *nodeServiceServer) EnsureMinecraftMap(ctx context.Context, req *connect.Request[nodeprotov1.EnsureMinecraftMapRequest]) (*connect.Response[nodeprotov1.EnsureMinecraftMapResponse], error) {
	errAuthorize := s.authorize(req.Header())
	if errAuthorize != nil {
		return nil, errAuthorize
	}
	message := req.Msg
	status, errEnsure := s.n.EnsureMinecraftMap(ctx, node.MinecraftMapEnsureRequest{
		ProcessID:        message.GetProcessId(),
		WorkingDirectory: message.GetWorkingDirectory(),
		WorldName:        message.GetWorldName(),
		JavaExecutable:   message.GetJavaExecutable(),
		MinecraftVersion: message.GetMinecraftVersion(),
	})
	if errEnsure != nil {
		return nil, translate(errEnsure)
	}
	return connect.NewResponse(&nodeprotov1.EnsureMinecraftMapResponse{
		Installed:            status.Installed,
		Running:              status.Running,
		Ready:                status.Ready,
		Provider:             status.Provider,
		Status:               status.Status,
		StatusMessage:        status.StatusMessage,
		BluemapVersion:       status.BlueMapVersion,
		LivePlayersAvailable: status.LivePlayersAvailable,
	}), nil
}

func (s *nodeServiceServer) StopMinecraftMap(ctx context.Context, req *connect.Request[nodeprotov1.StopMinecraftMapRequest]) (*connect.Response[nodeprotov1.StopMinecraftMapResponse], error) {
	errAuthorize := s.authorize(req.Header())
	if errAuthorize != nil {
		return nil, errAuthorize
	}
	errStop := s.n.StopMinecraftMap(ctx, req.Msg.GetProcessId())
	if errStop != nil {
		return nil, translate(errStop)
	}
	return connect.NewResponse(&nodeprotov1.StopMinecraftMapResponse{}), nil
}

func (s *nodeServiceServer) GetMinecraftMapAsset(ctx context.Context, req *connect.Request[nodeprotov1.GetMinecraftMapAssetRequest]) (*connect.Response[nodeprotov1.GetMinecraftMapAssetResponse], error) {
	errAuthorize := s.authorize(req.Header())
	if errAuthorize != nil {
		return nil, errAuthorize
	}
	asset, errAsset := s.n.GetMinecraftMapAsset(ctx, node.MinecraftMapAssetRequest{
		ProcessID:        req.Msg.GetProcessId(),
		WorkingDirectory: req.Msg.GetWorkingDirectory(),
		AssetPath:        req.Msg.GetAssetPath(),
	})
	if errAsset != nil {
		return nil, translate(errAsset)
	}
	return connect.NewResponse(&nodeprotov1.GetMinecraftMapAssetResponse{
		Content:         asset.Content,
		ContentType:     asset.ContentType,
		ContentEncoding: asset.ContentEncoding,
		CacheControl:    asset.CacheControl,
	}), nil
}

func (s *nodeServiceServer) PerformGameServerPlayerAction(ctx context.Context, req *connect.Request[nodeprotov1.PerformGameServerPlayerActionRequest]) (*connect.Response[nodeprotov1.PerformGameServerPlayerActionResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	errAction := s.n.PerformGameServerPlayerAction(ctx, node.GameServerPlayerActionRequest{
		Kind:      nodeGameServerQueryKindFromProto(req.Msg.GetKind()),
		Action:    nodeGameServerPlayerActionFromProto(req.Msg.GetAction()),
		ProcessID: req.Msg.GetProcessId(),
		IP:        req.Msg.GetIp(),
		QueryPort: req.Msg.GetQueryPort(),
		Username:  req.Msg.GetUsername(),
		Password:  req.Msg.GetPassword(),
		PlayerID:  req.Msg.GetPlayerId(),
		Reason:    req.Msg.GetReason(),
	})
	if errAction != nil {
		return nil, translate(errAction)
	}
	return connect.NewResponse(&nodeprotov1.PerformGameServerPlayerActionResponse{}), nil
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

func nodeGameServerPlayerActionFromProto(action nodeprotov1.GameServerPlayerAction) node.GameServerPlayerAction {
	switch action {
	case nodeprotov1.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_KICK:
		return node.GameServerPlayerActionKick
	case nodeprotov1.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_BAN:
		return node.GameServerPlayerActionBan
	case nodeprotov1.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_UNBAN:
		return node.GameServerPlayerActionUnban
	case nodeprotov1.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_ALLOWLIST_ADD:
		return node.GameServerPlayerActionAllowlistAdd
	case nodeprotov1.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_ALLOWLIST_REMOVE:
		return node.GameServerPlayerActionAllowlistRemove
	default:
		return node.GameServerPlayerActionUnknown
	}
}

func minecraftQueryToProto(info *node.MinecraftQueryInfo) *nodeprotov1.GameServerMinecraftQueryInfo {
	if info == nil {
		return nil
	}
	return &nodeprotov1.GameServerMinecraftQueryInfo{
		Motd:                info.MOTD,
		GameType:            info.GameType,
		Map:                 info.Map,
		NumberOfPlayers:     info.NumberOfPlayers,
		MaxPlayers:          info.MaxPlayers,
		PlayerList:          append([]string(nil), info.PlayerList...),
		ProtocolVersion:     info.ProtocolVersion,
		ServerVersion:       info.ServerVersion,
		PlayerDetails:       gameServerPlayersToProto(info.PlayerDetails),
		PlayerListSupported: info.PlayerListSupported,
		Responded:           new(info.Responded),
	}
}

func sourceQueryToProto(info *node.SourceQueryInfo) *nodeprotov1.GameServerSourceQueryInfo {
	if info == nil {
		return nil
	}
	return &nodeprotov1.GameServerSourceQueryInfo{
		Name:                info.Name,
		Map:                 info.Map,
		Game:                info.Game,
		AppId:               info.AppID,
		SteamId:             info.SteamID,
		GameId:              info.GameID,
		Players:             info.Players,
		MaxPlayers:          info.MaxPlayers,
		Bots:                info.Bots,
		ServerOs:            info.ServerOS,
		Visibility:          info.Visibility,
		Vac:                 info.VAC,
		Version:             info.Version,
		Protocol:            info.Protocol,
		PlayerList:          append([]string(nil), info.PlayerList...),
		PlayerListSupported: info.PlayerListSupported,
		Responded:           new(info.Responded),
	}
}

func palworldQueryToProto(info *node.PalworldQueryInfo) *nodeprotov1.GameServerPalworldQueryInfo {
	if info == nil {
		return nil
	}
	return &nodeprotov1.GameServerPalworldQueryInfo{
		Name:              info.Name,
		Description:       info.Description,
		Version:           info.Version,
		WorldGuid:         info.WorldGUID,
		Players:           info.Players,
		MaxPlayers:        info.MaxPlayers,
		PlayerList:        append([]string(nil), info.PlayerList...),
		UptimeSeconds:     info.UptimeSeconds,
		ServerFps:         info.ServerFPS,
		ServerFrameTimeMs: info.ServerFrameTimeMS,
		Days:              info.Days,
		Responded:         info.Responded,
		PlayerDetails:     gameServerPlayersToProto(info.PlayerDetails),
	}
}

func palworldMapSnapshotToProto(snapshot *node.PalworldMapSnapshot) *nodeprotov1.PalworldMapSnapshot {
	if snapshot == nil {
		return nil
	}
	actors := make([]*nodeprotov1.PalworldMapActor, 0, len(snapshot.Actors))
	for _, actor := range snapshot.Actors {
		actors = append(actors, &nodeprotov1.PalworldMapActor{
			Key:         actor.Key,
			Kind:        palworldMapActorKindToProto(actor.Kind),
			Name:        actor.Name,
			GuildKey:    actor.GuildKey,
			GuildName:   actor.GuildName,
			TrainerName: actor.TrainerName,
			ClassName:   actor.ClassName,
			LocationX:   actor.LocationX,
			LocationY:   actor.LocationY,
			LocationZ:   actor.LocationZ,
			RotationZ:   actor.RotationZ,
			Level:       actor.Level,
			Hp:          actor.HP,
			MaxHp:       actor.MaxHP,
			Action:      actor.Action,
			AiAction:    actor.AIAction,
			Active:      actor.Active,
		})
	}
	return &nodeprotov1.PalworldMapSnapshot{
		SourceTime:    snapshot.SourceTime,
		CollectedAt:   timestamppb.New(snapshot.CollectedAt),
		Source:        snapshot.Source,
		Partial:       snapshot.Partial,
		Truncated:     snapshot.Truncated,
		PartialReason: snapshot.PartialReason,
		Actors:        actors,
		Health:        palworldMapHealthToProto(snapshot.Health),
	}
}

func palworldMapHealthToProto(health *node.PalworldMapHealth) *nodeprotov1.PalworldMapHealth {
	if health == nil {
		return nil
	}
	return &nodeprotov1.PalworldMapHealth{
		ServerFps:         health.ServerFPS,
		ServerFrameTimeMs: health.ServerFrameTimeMS,
		CurrentPlayers:    health.CurrentPlayers,
		MaxPlayers:        health.MaxPlayers,
		UptimeSeconds:     health.UptimeSeconds,
		BaseCampCount:     health.BaseCampCount,
		Days:              health.Days,
	}
}

func sevenDaysToDieMapSnapshotToProto(snapshot *node.SevenDaysToDieMapSnapshot) *nodeprotov1.SevenDaysToDieMapSnapshot {
	if snapshot == nil {
		return nil
	}
	players := make([]*nodeprotov1.SevenDaysToDieMapPlayer, 0, len(snapshot.Players))
	for _, player := range snapshot.Players {
		players = append(players, &nodeprotov1.SevenDaysToDieMapPlayer{
			Id: player.ID, Name: player.Name, Online: player.Online,
			Position: sevenDaysToDieMapVectorToProto(player.Position),
		})
	}
	markers := make([]*nodeprotov1.SevenDaysToDieMapMarker, 0, len(snapshot.NativeMarkers))
	for _, marker := range snapshot.NativeMarkers {
		markers = append(markers, &nodeprotov1.SevenDaysToDieMapMarker{
			Id: marker.ID, Name: marker.Name, X: marker.Position.X, Z: marker.Position.Z,
		})
	}
	claims := make([]*nodeprotov1.SevenDaysToDieLandClaim, 0, len(snapshot.Claims))
	for _, claim := range snapshot.Claims {
		claims = append(claims, &nodeprotov1.SevenDaysToDieLandClaim{
			OwnerId: claim.OwnerID, OwnerName: claim.OwnerName, Active: claim.Active,
			Position: sevenDaysToDieMapVectorToProto(claim.Position), Size: claim.Size,
		})
	}
	return &nodeprotov1.SevenDaysToDieMapSnapshot{
		Enabled: snapshot.Enabled, TileSize: snapshot.TileSize, MaxZoom: snapshot.MaxZoom,
		MapSize: sevenDaysToDieMapVectorToProto(snapshot.MapSize), SourceTime: snapshot.SourceTime,
		Players: players, Markers: markers, Claims: claims,
		ClaimsSupported:   snapshot.ClaimsState == node.SevenDaysToDieWebAPIValueStateAvailable,
		NativeMarkerState: sevenDaysToDieWebAPIValueStateToProto(snapshot.NativeMarkerState),
		ClaimsState:       sevenDaysToDieWebAPIValueStateToProto(snapshot.ClaimsState),
		BloodMoon:         sevenDaysToDieMapBloodMoonToProto(snapshot.BloodMoon),
		BloodMoonState:    sevenDaysToDieWebAPIValueStateToProto(snapshot.BloodMoonState),
		Hostiles:          sevenDaysToDieMapEntitiesToProto(snapshot.Hostiles),
		HostileState:      sevenDaysToDieWebAPIValueStateToProto(snapshot.HostileState),
		Animals:           sevenDaysToDieMapEntitiesToProto(snapshot.Animals),
		AnimalState:       sevenDaysToDieWebAPIValueStateToProto(snapshot.AnimalState),
	}
}

func sevenDaysToDieMapBloodMoonToProto(value *node.SevenDaysToDieBloodMoon) *nodeprotov1.SevenDaysToDieMapBloodMoon {
	if value == nil {
		return nil
	}
	return &nodeprotov1.SevenDaysToDieMapBloodMoon{
		GameTime: sevenDaysToDieGameTimeToProto(&value.GameTime), Active: value.Active,
		NextBloodMoon:    sevenDaysToDieGameTimeToProto(&value.NextBloodMoon),
		NextBloodMoonEnd: sevenDaysToDieGameTimeToProto(&value.NextBloodMoonEnd),
	}
}

func sevenDaysToDieMapEntitiesToProto(values []node.SevenDaysToDieMapEntity) []*nodeprotov1.SevenDaysToDieMapEntity {
	result := make([]*nodeprotov1.SevenDaysToDieMapEntity, 0, len(values))
	for _, value := range values {
		result = append(result, &nodeprotov1.SevenDaysToDieMapEntity{
			Name: value.Name, Position: sevenDaysToDieMapVectorToProto(value.Position),
		})
	}
	return result
}

func sevenDaysToDieMapVectorToProto(vector node.SevenDaysToDieMapVector) *nodeprotov1.SevenDaysToDieMapVector {
	return &nodeprotov1.SevenDaysToDieMapVector{X: vector.X, Y: vector.Y, Z: vector.Z}
}

func sevenDaysToDieWebAPIConnectionStateToProto(
	state node.SevenDaysToDieWebAPIConnectionState,
) nodeprotov1.SevenDaysToDieWebAPIConnectionState {
	switch state {
	case node.SevenDaysToDieWebAPIConnectionStateAvailable:
		return nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE
	case node.SevenDaysToDieWebAPIConnectionStateServerOffline:
		return nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE
	case node.SevenDaysToDieWebAPIConnectionStateDashboardDisabled:
		return nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DASHBOARD_DISABLED
	case node.SevenDaysToDieWebAPIConnectionStateMisconfigured:
		return nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_MISCONFIGURED
	case node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable:
		return nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_NODE_UNAVAILABLE
	case node.SevenDaysToDieWebAPIConnectionStateUnreachable:
		return nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_WEB_API_UNREACHABLE
	case node.SevenDaysToDieWebAPIConnectionStateDiscoveryUnsupported:
		return nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DISCOVERY_UNSUPPORTED
	case node.SevenDaysToDieWebAPIConnectionStateAuthenticationDenied:
		return nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AUTHENTICATION_DENIED
	case node.SevenDaysToDieWebAPIConnectionStateInvalidResponse:
		return nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_INVALID_RESPONSE
	default:
		return nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_UNSPECIFIED
	}
}

func sevenDaysToDieWebAPIValueStateToProto(
	state node.SevenDaysToDieWebAPIValueState,
) nodeprotov1.SevenDaysToDieWebAPIValueState {
	switch state {
	case node.SevenDaysToDieWebAPIValueStateAvailable:
		return nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE
	case node.SevenDaysToDieWebAPIValueStateUnsupported:
		return nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED
	case node.SevenDaysToDieWebAPIValueStatePermissionDenied:
		return nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED
	case node.SevenDaysToDieWebAPIValueStateUnavailable:
		return nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE
	default:
		return nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED
	}
}

func sevenDaysToDieWebAPIStatusToProto(status *node.SevenDaysToDieWebAPIStatus) *nodeprotov1.SevenDaysToDieWebAPIStatus {
	if status == nil {
		return nil
	}
	result := &nodeprotov1.SevenDaysToDieWebAPIStatus{
		ConnectionState:  sevenDaysToDieWebAPIConnectionStateToProto(status.ConnectionState),
		ApiVersion:       status.APIVersion,
		Capabilities:     sevenDaysToDieWebAPICapabilitiesToProto(status.Capabilities),
		WorldTimeState:   sevenDaysToDieWebAPIValueStateToProto(status.WorldTimeState),
		WorldTime:        sevenDaysToDieGameTimeToProto(status.WorldTime),
		BloodMoonState:   sevenDaysToDieWebAPIValueStateToProto(status.BloodMoonState),
		BloodMoonActive:  status.BloodMoonActive,
		NextBloodMoon:    sevenDaysToDieGameTimeToProto(status.NextBloodMoon),
		NextBloodMoonEnd: sevenDaysToDieGameTimeToProto(status.NextBloodMoonEnd),
	}
	if !status.ObservedAt.IsZero() {
		observedAt := timestamppb.New(status.ObservedAt)
		errTimestamp := observedAt.CheckValid()
		if errTimestamp == nil {
			result.ObservedAt = observedAt
		}
	}
	return result
}

func sevenDaysToDiePlayersToProto(result *node.SevenDaysToDiePlayers) *nodeprotov1.SevenDaysToDiePlayers {
	if result == nil {
		return nil
	}
	players := make([]*nodeprotov1.SevenDaysToDiePlayer, 0, len(result.Players))
	for _, player := range result.Players {
		playerProto := &nodeprotov1.SevenDaysToDiePlayer{
			Name:            player.Name,
			ActionId:        player.ActionID,
			EntityId:        player.EntityID,
			PlatformId:      player.PlatformID,
			CrossPlatformId: player.CrossPlatformID,
			Online:          player.Online,
			Ping:            player.Ping,
			Level:           player.Level,
			Health:          player.Health,
			Stamina:         player.Stamina,
			Score:           player.Score,
			Deaths:          player.Deaths,
			ZombieKills:     player.ZombieKills,
			PlayerKills:     player.PlayerKills,
			Banned:          player.Banned,
		}
		players = append(players, playerProto)
	}
	return &nodeprotov1.SevenDaysToDiePlayers{
		ConnectionState: sevenDaysToDieWebAPIConnectionStateToProto(result.ConnectionState),
		State:           sevenDaysToDieWebAPIValueStateToProto(result.State),
		Players:         players,
	}
}

func sevenDaysToDieReportedModsToProto(result *node.SevenDaysToDieReportedMods) *nodeprotov1.SevenDaysToDieReportedMods {
	if result == nil {
		return nil
	}
	mods := make([]*nodeprotov1.SevenDaysToDieReportedMod, 0, len(result.Mods))
	for _, mod := range result.Mods {
		mods = append(mods, &nodeprotov1.SevenDaysToDieReportedMod{
			Name: mod.Name, DisplayName: mod.DisplayName, Description: mod.Description, Author: mod.Author, Version: mod.Version,
		})
	}
	return &nodeprotov1.SevenDaysToDieReportedMods{
		ConnectionState: sevenDaysToDieWebAPIConnectionStateToProto(result.ConnectionState),
		State:           sevenDaysToDieWebAPIValueStateToProto(result.State),
		Mods:            mods,
	}
}

func sevenDaysToDieSandboxSettingsToProto(result *node.SevenDaysToDieSandboxSettings) *nodeprotov1.SevenDaysToDieSandboxSettings {
	if result == nil {
		return nil
	}
	settings := make([]*nodeprotov1.SevenDaysToDieSandboxSetting, 0, len(result.Settings))
	for _, setting := range result.Settings {
		settings = append(settings, &nodeprotov1.SevenDaysToDieSandboxSetting{
			Key: setting.Key, Label: setting.Label, Description: setting.Description, Group: setting.Group,
			EffectiveValue: setting.EffectiveValue, EffectiveLabel: setting.EffectiveLabel,
		})
	}
	protoResult := &nodeprotov1.SevenDaysToDieSandboxSettings{
		ConnectionState: sevenDaysToDieWebAPIConnectionStateToProto(result.ConnectionState),
		State:           sevenDaysToDieWebAPIValueStateToProto(result.State),
		ComparisonState: sevenDaysToDieSandboxComparisonStateToProto(result.ComparisonState),
		ConfiguredCode:  result.ConfiguredCode,
		EffectiveCode:   result.EffectiveCode,
		Settings:        settings,
	}
	if !result.ObservedAt.IsZero() {
		observedAt := timestamppb.New(result.ObservedAt)
		errTimestamp := observedAt.CheckValid()
		if errTimestamp == nil {
			protoResult.ObservedAt = observedAt
		}
	}
	return protoResult
}

func sevenDaysToDieSandboxComparisonStateToProto(state node.SevenDaysToDieSandboxComparisonState) nodeprotov1.SevenDaysToDieSandboxComparisonState {
	switch state {
	case node.SevenDaysToDieSandboxComparisonStateMatch:
		return nodeprotov1.SevenDaysToDieSandboxComparisonState_SEVEN_DAYS_TO_DIE_SANDBOX_COMPARISON_STATE_MATCH
	case node.SevenDaysToDieSandboxComparisonStateMismatch:
		return nodeprotov1.SevenDaysToDieSandboxComparisonState_SEVEN_DAYS_TO_DIE_SANDBOX_COMPARISON_STATE_MISMATCH
	case node.SevenDaysToDieSandboxComparisonStateStale:
		return nodeprotov1.SevenDaysToDieSandboxComparisonState_SEVEN_DAYS_TO_DIE_SANDBOX_COMPARISON_STATE_STALE
	default:
		return nodeprotov1.SevenDaysToDieSandboxComparisonState_SEVEN_DAYS_TO_DIE_SANDBOX_COMPARISON_STATE_UNSPECIFIED
	}
}

func sevenDaysToDieWebAPICapabilitiesToProto(capabilities node.SevenDaysToDieWebAPICapabilities) *nodeprotov1.SevenDaysToDieWebAPICapabilities {
	return &nodeprotov1.SevenDaysToDieWebAPICapabilities{
		PlayerData:                capabilities.PlayerData,
		RuntimeSettings:           capabilities.RuntimeSettings,
		NativeLog:                 capabilities.NativeLog,
		WorldPopulation:           capabilities.WorldPopulation,
		HostileAndAnimalPositions: capabilities.HostileAndAnimalPositions,
		HostilePositions:          capabilities.HostilePositions,
		AnimalPositions:           capabilities.AnimalPositions,
		AccessControl:             capabilities.AccessControl,
		GamePermissions:           capabilities.GamePermissions,
		ReportedMods:              capabilities.ReportedMods,
	}
}

func sevenDaysToDieGameTimeToProto(gameTime *node.SevenDaysToDieGameTime) *nodeprotov1.SevenDaysToDieGameTime {
	if gameTime == nil {
		return nil
	}
	return &nodeprotov1.SevenDaysToDieGameTime{Day: gameTime.Day, Hour: gameTime.Hour, Minute: gameTime.Minute}
}

func palworldMapActorKindToProto(kind node.PalworldMapActorKind) nodeprotov1.PalworldMapActorKind {
	switch kind {
	case node.PalworldMapActorKindPlayer:
		return nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_PLAYER
	case node.PalworldMapActorKindBase:
		return nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_BASE
	case node.PalworldMapActorKindBaseWorker:
		return nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_BASE_WORKER
	case node.PalworldMapActorKindCompanionPal:
		return nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_COMPANION_PAL
	case node.PalworldMapActorKindWildPal:
		return nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_WILD_PAL
	case node.PalworldMapActorKindNPC:
		return nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_NPC
	case node.PalworldMapActorKindOther:
		return nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_OTHER
	default:
		return nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_UNSPECIFIED
	}
}

func gameServerPlayersToProto(players []node.GameServerPlayer) []*nodeprotov1.GameServerPlayer {
	result := make([]*nodeprotov1.GameServerPlayer, 0, len(players))
	for _, player := range players {
		result = append(result, &nodeprotov1.GameServerPlayer{
			Name: player.Name,
			Id:   player.ID,
		})
	}
	return result
}

func processSnapshotToProto(p *node.ProcessSnapshot) *nodeprotov1.ProcessSnapshot {
	if p == nil {
		return &nodeprotov1.ProcessSnapshot{}
	}
	out := &nodeprotov1.ProcessSnapshot{
		Id:                   p.ID,
		ExecutionId:          p.ExecutionID,
		Name:                 p.Name,
		Status:               p.Status,
		PreviousStatus:       p.PreviousStatus,
		TransitionSequence:   p.TransitionSequence,
		IntentionalStop:      p.IntentionalStop,
		UnixStartedAt:        p.UnixStartedAt,
		CpuPercent:           p.CPUPercent,
		CpuValid:             new(p.CPUValid),
		MetricsValid:         new(p.MetricsValid),
		CpuCores:             p.CPUCores,
		MemoryRss:            p.MemoryRSS,
		MemoryVms:            p.MemoryVMS,
		MemoryPercent:        p.MemoryPercent,
		NumThreads:           p.NumThreads,
		DiskUsageBytes:       p.DiskUsageBytes,
		DiskTotalBytes:       p.DiskTotalBytes,
		DiskFreeBytes:        p.DiskFreeBytes,
		DiskPercent:          p.DiskPercent,
		DiskValid:            new(p.DiskValid),
		IoValid:              new(p.IOValid),
		IoReadRate:           p.IOReadRate,
		IoWriteRate:          p.IOWriteRate,
		ConnectionCount:      p.ConnectionCount,
		ConnectionCountValid: new(p.ConnectionCountValid),
		WorkingDir:           p.WorkingDir,
	}
	if !p.DiskMeasuredAt.IsZero() {
		out.DiskMeasuredAt = timestamppb.New(p.DiskMeasuredAt)
	}
	if p.ExitCodeKnown {
		exitCode := clampToInt32(p.ExitCode)
		out.ExitCode = &exitCode
	}
	return out
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

	subscription := emitter.SubscribeWithReplay(req.Msg.GetReplayProcessStatus())
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

	chunks, errStream := s.n.StreamConsoleOutput(ctx, req.Msg.GetProcessId(), req.Msg.GetReplayBuffer())
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
				Sequence:     chunk.Sequence,
				ResetBuffer:  chunk.ResetBuffer,
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

func (s *nodeServiceServer) GetRuntimeCapabilities(_ context.Context, req *connect.Request[nodeprotov1.GetRuntimeCapabilitiesRequest]) (*connect.Response[nodeprotov1.GetRuntimeCapabilitiesResponse], error) {
	errAuth := s.authorize(req.Header())
	if errAuth != nil {
		return nil, errAuth
	}
	if s.n == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("node not initialized"))
	}
	caps := s.n.RuntimeCapabilities()
	return connect.NewResponse(&nodeprotov1.GetRuntimeCapabilitiesResponse{
		ProtocolVersion:          caps.ProtocolVersion,
		LaunchEnv:                caps.LaunchEnv,
		ReliableProcessLifecycle: caps.ReliableProcessLifecycle,
		TelnetInput:              caps.TelnetInput,
		RconInput:                caps.RCONInput,
		RestInput:                caps.RESTInput,
		PlayerActions:            caps.PlayerActions,
		PalworldMap:              caps.PalworldMap,
		SevenDaysToDieMap:        caps.SevenDaysToDieMap,
		MinecraftMap:             caps.MinecraftMap,
	}), nil
}

func nodeRCONProtocolFromProto(protocol nodeprotov1.RCONProtocol) (node.RCONProtocol, error) {
	switch protocol {
	case nodeprotov1.RCONProtocol_RCON_PROTOCOL_SOURCE:
		return node.RCONProtocolSource, nil
	case nodeprotov1.RCONProtocol_RCON_PROTOCOL_MINECRAFT:
		return node.RCONProtocolMinecraft, nil
	case nodeprotov1.RCONProtocol_RCON_PROTOCOL_RUST_WEB:
		return node.RCONProtocolRustWeb, nil
	default:
		return node.RCONProtocolUnknown, errors.New("unsupported RCON protocol")
	}
}

func nodeRESTInputKindFromProto(kind nodeprotov1.RESTInputKind) (node.RESTInputKind, error) {
	switch kind {
	case nodeprotov1.RESTInputKind_REST_INPUT_KIND_SATISFACTORY:
		return node.RESTInputKindSatisfactory, nil
	case nodeprotov1.RESTInputKind_REST_INPUT_KIND_PALWORLD:
		return node.RESTInputKindPalworld, nil
	default:
		return node.RESTInputKindUnknown, errors.New("unsupported REST input kind")
	}
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
		status := &nodeprotov1.ProcessStatusEvent{
			ProcessId:          ev.ProcessID,
			Status:             ev.Status,
			OldStatus:          ev.OldStatus,
			IntentionalStop:    ev.IntentionalStop,
			ExecutionId:        ev.ExecutionID,
			TransitionSequence: ev.TransitionSequence,
			Replayed:           ev.Replayed,
		}
		if ev.ExitCodeKnown {
			exitCode := clampToInt32(ev.ExitCode)
			status.ExitCode = &exitCode
		}
		out.Payload = &nodeprotov1.Event_ProcessStatus{
			ProcessStatus: status,
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
