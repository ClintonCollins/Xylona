package nodeclient_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/nodetls"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1/nodeprotoconnect"
)

// stubHandler is a NodeServiceHandler test double driven by a shared recorder.
// Individual tests override any method via the function fields before wiring
// the handler into an httptest server.
type stubHandler struct {
	nodeprotoconnect.UnimplementedNodeServiceHandler
	rec *callRecorder
}

type callRecorder struct {
	mu                sync.Mutex
	authHeaders       []string
	listFilesReq      *nodeprotov1.ListFilesRequest
	startProcessReq   *nodeprotov1.StartProcessRequest
	readFileResponse  []byte
	streamFileChunks  [][]byte
	streamWriteReqs   []*nodeprotov1.StreamWriteFileRequest
	streamWriteResp   *nodeprotov1.StreamWriteFileResponse
	listFilesResp     *nodeprotov1.ListFilesResponse
	statFileResp      *nodeprotov1.StatFileResponse
	bindableIPsResp   *nodeprotov1.ListBindableIPsResponse
	copyFilesReq      *nodeprotov1.CopyFilesRequest
	copyFilesResp     *nodeprotov1.CopyFilesResponse
	probeReq          *nodeprotov1.ProbeInstalledVersionRequest
	probeResp         *nodeprotov1.ProbeInstalledVersionResponse
	queryReq          *nodeprotov1.QueryGameServerRequest
	queryResp         *nodeprotov1.QueryGameServerResponse
	palworldMapReq    *nodeprotov1.QueryPalworldMapRequest
	palworldMapResp   *nodeprotov1.QueryPalworldMapResponse
	sevenDaysMapReq   *nodeprotov1.QuerySevenDaysToDieMapRequest
	sevenDaysMapResp  *nodeprotov1.QuerySevenDaysToDieMapResponse
	webAPIStatusReq   *nodeprotov1.QuerySevenDaysToDieWebAPIStatusRequest
	webAPIStatusResp  *nodeprotov1.QuerySevenDaysToDieWebAPIStatusResponse
	metadataReq       *nodeprotov1.QuerySevenDaysToDieOperationMetadataRequest
	metadataResp      *nodeprotov1.QuerySevenDaysToDieOperationMetadataResponse
	playersReq        *nodeprotov1.QuerySevenDaysToDiePlayersRequest
	playersResp       *nodeprotov1.QuerySevenDaysToDiePlayersResponse
	reportedModsReq   *nodeprotov1.QuerySevenDaysToDieReportedModsRequest
	reportedModsResp  *nodeprotov1.QuerySevenDaysToDieReportedModsResponse
	sandboxReq        *nodeprotov1.QuerySevenDaysToDieSandboxSettingsRequest
	sandboxResp       *nodeprotov1.QuerySevenDaysToDieSandboxSettingsResponse
	playerActionReq   *nodeprotov1.PerformGameServerPlayerActionRequest
	playerActionErr   error
	gameOperationReq  *nodeprotov1.ExecuteGameOperationRequest
	gameOperationResp *nodeprotov1.ExecuteGameOperationResponse
	consoleInputErr   error
	fileArchiveReq    *nodeprotov1.CreateFileArchiveRequest
	fileArchiveResp   []*nodeprotov1.CreateFileArchiveResponse
	fileExtractReq    *nodeprotov1.ExtractFileArchiveRequest
	fileExtractResp   []*nodeprotov1.ExtractFileArchiveResponse
	streamEvents      []*nodeprotov1.Event
	streamConsole     []*nodeprotov1.ConsoleChunk
	nodeSnapshot      *nodeprotov1.NodeSnapshot
	runtimeCapsResp   *nodeprotov1.GetRuntimeCapabilitiesResponse
	runtimeCapsErr    error
	errOverride       error
}

func (r *callRecorder) recordAuth(headers http.Header) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.authHeaders = append(r.authHeaders, headers.Get(nodeclient.AuthorizationHeader))
}

func (s *stubHandler) Ping(_ context.Context, req *connect.Request[nodeprotov1.PingRequest]) (*connect.Response[nodeprotov1.PingResponse], error) {
	s.rec.recordAuth(req.Header())
	return connect.NewResponse(&nodeprotov1.PingResponse{ServerTime: timestamppb.Now()}), nil
}

func (s *stubHandler) ListFiles(_ context.Context, req *connect.Request[nodeprotov1.ListFilesRequest]) (*connect.Response[nodeprotov1.ListFilesResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.listFilesReq = req.Msg
	resp := s.rec.listFilesResp
	s.rec.mu.Unlock()
	if resp == nil {
		resp = &nodeprotov1.ListFilesResponse{}
	}
	return connect.NewResponse(resp), nil
}

func (s *stubHandler) ReadFile(_ context.Context, req *connect.Request[nodeprotov1.ReadFileRequest]) (*connect.Response[nodeprotov1.ReadFileResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	body := s.rec.readFileResponse
	s.rec.mu.Unlock()
	return connect.NewResponse(&nodeprotov1.ReadFileResponse{Content: body}), nil
}

func (s *stubHandler) StatFile(_ context.Context, req *connect.Request[nodeprotov1.StatFileRequest]) (*connect.Response[nodeprotov1.StatFileResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	resp := s.rec.statFileResp
	s.rec.mu.Unlock()
	if resp == nil {
		resp = &nodeprotov1.StatFileResponse{}
	}
	return connect.NewResponse(resp), nil
}

func (s *stubHandler) StreamFile(_ context.Context, req *connect.Request[nodeprotov1.StreamFileRequest], stream *connect.ServerStream[nodeprotov1.StreamFileResponse]) error {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	chunks := append([][]byte(nil), s.rec.streamFileChunks...)
	s.rec.mu.Unlock()
	for _, chunk := range chunks {
		errSend := stream.Send(&nodeprotov1.StreamFileResponse{Content: chunk})
		if errSend != nil {
			return fmt.Errorf("stub file stream send: %w", errSend)
		}
	}
	return nil
}

func (s *stubHandler) StreamWriteFile(_ context.Context, stream *connect.ClientStream[nodeprotov1.StreamWriteFileRequest]) (*connect.Response[nodeprotov1.StreamWriteFileResponse], error) {
	s.rec.recordAuth(stream.RequestHeader())
	reqs := []*nodeprotov1.StreamWriteFileRequest{}
	for stream.Receive() {
		msg := stream.Msg()
		reqs = append(reqs, msg)
	}
	errStream := stream.Err()
	if errStream != nil {
		return nil, fmt.Errorf("stub stream write receive: %w", errStream)
	}

	s.rec.mu.Lock()
	s.rec.streamWriteReqs = append(s.rec.streamWriteReqs, reqs...)
	resp := s.rec.streamWriteResp
	s.rec.mu.Unlock()
	if resp == nil {
		resp = &nodeprotov1.StreamWriteFileResponse{}
	}
	return connect.NewResponse(resp), nil
}

func (s *stubHandler) CopyFiles(_ context.Context, req *connect.Request[nodeprotov1.CopyFilesRequest]) (*connect.Response[nodeprotov1.CopyFilesResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.copyFilesReq = req.Msg
	resp := s.rec.copyFilesResp
	s.rec.mu.Unlock()
	if resp == nil {
		resp = &nodeprotov1.CopyFilesResponse{}
	}
	return connect.NewResponse(resp), nil
}

func (s *stubHandler) ProbeInstalledVersion(_ context.Context, req *connect.Request[nodeprotov1.ProbeInstalledVersionRequest]) (*connect.Response[nodeprotov1.ProbeInstalledVersionResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.probeReq = req.Msg
	resp := s.rec.probeResp
	s.rec.mu.Unlock()
	if resp == nil {
		resp = &nodeprotov1.ProbeInstalledVersionResponse{}
	}
	return connect.NewResponse(resp), nil
}

func (s *stubHandler) QueryGameServer(_ context.Context, req *connect.Request[nodeprotov1.QueryGameServerRequest]) (*connect.Response[nodeprotov1.QueryGameServerResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.queryReq = req.Msg
	resp := s.rec.queryResp
	s.rec.mu.Unlock()
	if resp == nil {
		resp = &nodeprotov1.QueryGameServerResponse{}
	}
	return connect.NewResponse(resp), nil
}

func (s *stubHandler) QueryPalworldMap(_ context.Context, req *connect.Request[nodeprotov1.QueryPalworldMapRequest]) (*connect.Response[nodeprotov1.QueryPalworldMapResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.palworldMapReq = req.Msg
	resp := s.rec.palworldMapResp
	s.rec.mu.Unlock()
	if resp == nil {
		resp = &nodeprotov1.QueryPalworldMapResponse{}
	}
	return connect.NewResponse(resp), nil
}

func (s *stubHandler) QuerySevenDaysToDieMap(_ context.Context, req *connect.Request[nodeprotov1.QuerySevenDaysToDieMapRequest]) (*connect.Response[nodeprotov1.QuerySevenDaysToDieMapResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.sevenDaysMapReq = req.Msg
	resp := s.rec.sevenDaysMapResp
	s.rec.mu.Unlock()
	if resp == nil {
		resp = &nodeprotov1.QuerySevenDaysToDieMapResponse{}
	}
	return connect.NewResponse(resp), nil
}

func (s *stubHandler) QuerySevenDaysToDieWebAPIStatus(_ context.Context, req *connect.Request[nodeprotov1.QuerySevenDaysToDieWebAPIStatusRequest]) (*connect.Response[nodeprotov1.QuerySevenDaysToDieWebAPIStatusResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.webAPIStatusReq = req.Msg
	resp := s.rec.webAPIStatusResp
	s.rec.mu.Unlock()
	if resp == nil {
		resp = &nodeprotov1.QuerySevenDaysToDieWebAPIStatusResponse{}
	}
	return connect.NewResponse(resp), nil
}

func (s *stubHandler) QuerySevenDaysToDieOperationMetadata(_ context.Context, req *connect.Request[nodeprotov1.QuerySevenDaysToDieOperationMetadataRequest]) (*connect.Response[nodeprotov1.QuerySevenDaysToDieOperationMetadataResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.metadataReq = req.Msg
	resp := s.rec.metadataResp
	s.rec.mu.Unlock()
	if resp == nil {
		resp = &nodeprotov1.QuerySevenDaysToDieOperationMetadataResponse{}
	}
	return connect.NewResponse(resp), nil
}

func (s *stubHandler) QuerySevenDaysToDiePlayers(_ context.Context, req *connect.Request[nodeprotov1.QuerySevenDaysToDiePlayersRequest]) (*connect.Response[nodeprotov1.QuerySevenDaysToDiePlayersResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.playersReq = req.Msg
	resp := s.rec.playersResp
	s.rec.mu.Unlock()
	if resp == nil {
		resp = &nodeprotov1.QuerySevenDaysToDiePlayersResponse{}
	}
	return connect.NewResponse(resp), nil
}

func (s *stubHandler) QuerySevenDaysToDieReportedMods(_ context.Context, req *connect.Request[nodeprotov1.QuerySevenDaysToDieReportedModsRequest]) (*connect.Response[nodeprotov1.QuerySevenDaysToDieReportedModsResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.reportedModsReq = req.Msg
	resp := s.rec.reportedModsResp
	s.rec.mu.Unlock()
	if resp == nil {
		resp = &nodeprotov1.QuerySevenDaysToDieReportedModsResponse{}
	}
	return connect.NewResponse(resp), nil
}

func (s *stubHandler) QuerySevenDaysToDieSandboxSettings(_ context.Context, req *connect.Request[nodeprotov1.QuerySevenDaysToDieSandboxSettingsRequest]) (*connect.Response[nodeprotov1.QuerySevenDaysToDieSandboxSettingsResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.sandboxReq = req.Msg
	resp := s.rec.sandboxResp
	s.rec.mu.Unlock()
	if resp == nil {
		resp = &nodeprotov1.QuerySevenDaysToDieSandboxSettingsResponse{}
	}
	return connect.NewResponse(resp), nil
}

func (s *stubHandler) PerformGameServerPlayerAction(_ context.Context, req *connect.Request[nodeprotov1.PerformGameServerPlayerActionRequest]) (*connect.Response[nodeprotov1.PerformGameServerPlayerActionResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.playerActionReq = req.Msg
	errAction := s.rec.playerActionErr
	s.rec.mu.Unlock()
	if errAction != nil {
		return nil, errAction
	}
	return connect.NewResponse(&nodeprotov1.PerformGameServerPlayerActionResponse{}), nil
}

func (s *stubHandler) ExecuteGameOperation(_ context.Context, req *connect.Request[nodeprotov1.ExecuteGameOperationRequest]) (*connect.Response[nodeprotov1.ExecuteGameOperationResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.gameOperationReq = req.Msg
	response := s.rec.gameOperationResp
	s.rec.mu.Unlock()
	if response == nil {
		response = &nodeprotov1.ExecuteGameOperationResponse{}
	}
	return connect.NewResponse(response), nil
}

func (s *stubHandler) SendConsoleInput(_ context.Context, req *connect.Request[nodeprotov1.SendConsoleInputRequest]) (*connect.Response[nodeprotov1.SendConsoleInputResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	errInput := s.rec.consoleInputErr
	s.rec.mu.Unlock()
	if errInput != nil {
		return nil, errInput
	}
	return connect.NewResponse(&nodeprotov1.SendConsoleInputResponse{}), nil
}

func (s *stubHandler) StreamCreateFileArchive(_ context.Context, req *connect.Request[nodeprotov1.CreateFileArchiveRequest], stream *connect.ServerStream[nodeprotov1.CreateFileArchiveResponse]) error {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.fileArchiveReq = req.Msg
	responses := append([]*nodeprotov1.CreateFileArchiveResponse(nil), s.rec.fileArchiveResp...)
	s.rec.mu.Unlock()
	for _, response := range responses {
		errSend := stream.Send(response)
		if errSend != nil {
			return fmt.Errorf("stub archive stream send: %w", errSend)
		}
	}
	return nil
}

func (s *stubHandler) StreamExtractFileArchive(_ context.Context, req *connect.Request[nodeprotov1.ExtractFileArchiveRequest], stream *connect.ServerStream[nodeprotov1.ExtractFileArchiveResponse]) error {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.fileExtractReq = req.Msg
	responses := append([]*nodeprotov1.ExtractFileArchiveResponse(nil), s.rec.fileExtractResp...)
	s.rec.mu.Unlock()
	for _, response := range responses {
		errSend := stream.Send(response)
		if errSend != nil {
			return fmt.Errorf("stub extract stream send: %w", errSend)
		}
	}
	return nil
}

func (s *stubHandler) StartProcess(_ context.Context, req *connect.Request[nodeprotov1.StartProcessRequest]) (*connect.Response[nodeprotov1.StartProcessResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	s.rec.startProcessReq = req.Msg
	errOverride := s.rec.errOverride
	s.rec.mu.Unlock()
	if errOverride != nil {
		return nil, errOverride
	}
	return connect.NewResponse(&nodeprotov1.StartProcessResponse{ProcessId: req.Msg.GetId()}), nil
}

func (s *stubHandler) GetNodeSnapshot(_ context.Context, req *connect.Request[nodeprotov1.GetNodeSnapshotRequest]) (*connect.Response[nodeprotov1.NodeSnapshot], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	snap := s.rec.nodeSnapshot
	s.rec.mu.Unlock()
	if snap == nil {
		snap = &nodeprotov1.NodeSnapshot{}
	}
	return connect.NewResponse(snap), nil
}

func (s *stubHandler) GetRuntimeCapabilities(_ context.Context, req *connect.Request[nodeprotov1.GetRuntimeCapabilitiesRequest]) (*connect.Response[nodeprotov1.GetRuntimeCapabilitiesResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	resp := s.rec.runtimeCapsResp
	errResp := s.rec.runtimeCapsErr
	s.rec.mu.Unlock()
	if errResp != nil {
		return nil, errResp
	}
	if resp == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("runtime capabilities unavailable"))
	}
	return connect.NewResponse(resp), nil
}

func (s *stubHandler) ListBindableIPs(_ context.Context, req *connect.Request[nodeprotov1.ListBindableIPsRequest]) (*connect.Response[nodeprotov1.ListBindableIPsResponse], error) {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	resp := s.rec.bindableIPsResp
	s.rec.mu.Unlock()
	if resp == nil {
		resp = &nodeprotov1.ListBindableIPsResponse{}
	}
	return connect.NewResponse(resp), nil
}

func (s *stubHandler) StreamEvents(_ context.Context, req *connect.Request[nodeprotov1.StreamEventsRequest], stream *connect.ServerStream[nodeprotov1.Event]) error {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	events := append([]*nodeprotov1.Event(nil), s.rec.streamEvents...)
	s.rec.mu.Unlock()
	for _, ev := range events {
		errSend := stream.Send(ev)
		if errSend != nil {
			return fmt.Errorf("stub stream send: %w", errSend)
		}
	}
	return nil
}

func (s *stubHandler) StreamConsoleOutput(_ context.Context, req *connect.Request[nodeprotov1.StreamConsoleOutputRequest], stream *connect.ServerStream[nodeprotov1.ConsoleChunk]) error {
	s.rec.recordAuth(req.Header())
	s.rec.mu.Lock()
	chunks := append([]*nodeprotov1.ConsoleChunk(nil), s.rec.streamConsole...)
	s.rec.mu.Unlock()
	for _, chunk := range chunks {
		errSend := stream.Send(chunk)
		if errSend != nil {
			return fmt.Errorf("stub console stream send: %w", errSend)
		}
	}
	return nil
}

// newPinnedTestServer starts a TLS httptest server backed by the NodeService
// handler, with cleanup registered. It returns the server URL and the
// fingerprint callers should pass to NewGRPCClient.
func newPinnedTestServer(t *testing.T, rec *callRecorder) (string, string) {
	t.Helper()

	certPEM, keyPEM, fingerprint, errGen := nodetls.GenerateSelfSigned(context.Background(), "stub-node")
	if errGen != nil {
		t.Fatalf("GenerateSelfSigned: %v", errGen)
	}
	tlsConfig, errTLS := nodetls.NewServerTLSConfig(certPEM, keyPEM)
	if errTLS != nil {
		t.Fatalf("NewServerTLSConfig: %v", errTLS)
	}

	handler := &stubHandler{rec: rec}
	mux := http.NewServeMux()
	path, svc := nodeprotoconnect.NewNodeServiceHandler(handler)
	mux.Handle(path, svc)

	server := httptest.NewUnstartedServer(mux)
	server.TLS = tlsConfig
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	return server.URL, fingerprint
}

func TestGRPCClientPingAttachesBearerToken(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{}
	url, fingerprint := newPinnedTestServer(t, rec)

	client, errNew := nodeclient.NewGRPCClient("node-1", url, fingerprint, "s3cret")
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	errPing := client.Ping(t.Context())
	if errPing != nil {
		t.Fatalf("Ping: %v", errPing)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.authHeaders) != 1 {
		t.Fatalf("expected 1 auth header, got %d", len(rec.authHeaders))
	}
	if rec.authHeaders[0] != "Bearer s3cret" {
		t.Fatalf("unexpected auth header: %q", rec.authHeaders[0])
	}
}
func TestGRPCClientConstructorValidatesRequiredFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		nodeID      string
		url         string
		fingerprint string
		secret      string
	}{
		{name: "empty node ID", url: "https://x", fingerprint: "abc", secret: "s"},
		{name: "empty URL", nodeID: "id", fingerprint: "abc", secret: "s"},
		{name: "empty secret", nodeID: "id", url: "https://x", fingerprint: "abc"},
		{name: "empty fingerprint", nodeID: "id", url: "https://x", secret: "s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errNew := nodeclient.NewGRPCClient(tc.nodeID, tc.url, tc.fingerprint, tc.secret)
			if errNew == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}
func TestGRPCClientListFilesTranslatesResponse(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{
		listFilesResp: &nodeprotov1.ListFilesResponse{
			Entries: []*nodeprotov1.FileEntry{
				{Name: "a.txt", Size: 12, IsDirectory: false, LastModified: timestamppb.New(time.Unix(100, 0))},
				{Name: "sub", Size: 0, IsDirectory: true, LastModified: timestamppb.New(time.Unix(200, 0))},
				{Name: "run.sh", Size: 4, IsExecutable: true, LastModified: timestamppb.New(time.Unix(300, 0))},
			},
		},
	}
	url, fingerprint := newPinnedTestServer(t, rec)

	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, "s")
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	entries, errList := client.ListFiles(t.Context(), "/srv", "sub")
	if errList != nil {
		t.Fatalf("ListFiles: %v", errList)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries want 3", len(entries))
	}
	if entries[0].Name != "a.txt" || entries[0].Size != 12 || entries[0].IsDirectory {
		t.Fatalf("entries[0] unexpected: %+v", entries[0])
	}
	if !entries[1].IsDirectory || entries[1].Name != "sub" {
		t.Fatalf("entries[1] unexpected: %+v", entries[1])
	}
	if !entries[2].IsExecutable || entries[2].Name != "run.sh" {
		t.Fatalf("entries[2] unexpected: %+v", entries[2])
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.listFilesReq.GetDirectory() != "/srv" {
		t.Fatalf("directory sent incorrectly: %q", rec.listFilesReq.GetDirectory())
	}
	if rec.listFilesReq.GetRelativePath() != "sub" {
		t.Fatalf("relative path sent incorrectly: %q", rec.listFilesReq.GetRelativePath())
	}
}
func TestGRPCClientStreamWriteSendsMetadataAndChunks(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{
		streamWriteResp: &nodeprotov1.StreamWriteFileResponse{BytesWritten: 11, Sha256: "digest"},
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	result, errWrite := client.StreamWriteFile(t.Context(), "/srv", "upload.bin", strings.NewReader("hello world"), node.ProtectionPolicy{ServerExecutable: "server.jar"})
	if errWrite != nil {
		t.Fatalf("StreamWriteFile: %v", errWrite)
	}
	if result.BytesWritten != 11 || result.SHA256 != "digest" {
		t.Fatalf("StreamWriteFile result = %+v, want configured response", result)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.streamWriteReqs) < 2 {
		t.Fatalf("stream write requests = %d, want metadata plus content", len(rec.streamWriteReqs))
	}
	if rec.streamWriteReqs[0].GetDirectory() != "/srv" || rec.streamWriteReqs[0].GetRelativePath() != "upload.bin" {
		t.Fatalf("metadata request = %+v, want directory and relative path", rec.streamWriteReqs[0])
	}
	if rec.streamWriteReqs[0].GetServerExecutable() != "server.jar" {
		t.Fatalf("server executable = %q, want server.jar", rec.streamWriteReqs[0].GetServerExecutable())
	}
	var body strings.Builder
	for _, msg := range rec.streamWriteReqs[1:] {
		_, errBody := body.Write(msg.GetContent())
		if errBody != nil {
			t.Fatalf("body.Write: %v", errBody)
		}
	}
	gotBody := body.String()
	if gotBody != "hello world" {
		t.Fatalf("streamed body = %q, want hello world", gotBody)
	}
}
func TestGRPCClientCopyFilesSendsOperationsAndPolicy(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{
		copyFilesResp: &nodeprotov1.CopyFilesResponse{Copied: []string{"dst.txt"}},
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	copied, errCopy := client.CopyFiles(t.Context(), "/srv", []node.CopyFileOperation{
		{SourceRelativePath: "src.txt", DestinationRelativePath: "dst.txt"},
	}, node.ProtectionPolicy{BaseCommand: "./run.sh"})
	if errCopy != nil {
		t.Fatalf("CopyFiles: %v", errCopy)
	}
	if len(copied) != 1 || copied[0] != "dst.txt" {
		t.Fatalf("CopyFiles copied = %v, want [dst.txt]", copied)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.copyFilesReq.GetDirectory() != "/srv" {
		t.Fatalf("directory = %q, want /srv", rec.copyFilesReq.GetDirectory())
	}
	if rec.copyFilesReq.GetOperations()[0].GetDestinationRelativePath() != "dst.txt" {
		t.Fatalf("operations = %+v, want destination", rec.copyFilesReq.GetOperations())
	}
	if rec.copyFilesReq.GetBaseCommand() != "./run.sh" {
		t.Fatalf("base command = %q, want ./run.sh", rec.copyFilesReq.GetBaseCommand())
	}
}
func TestGRPCClientProbeInstalledVersionRoundTrips(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{
		probeResp: &nodeprotov1.ProbeInstalledVersionResponse{Found: true, Version: "123", SourcePath: "steamapps/appmanifest_90.acf"},
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	result, errProbe := client.ProbeInstalledVersion(t.Context(), node.InstalledVersionProbeRequest{
		Directory:           "/srv",
		Kind:                node.InstalledVersionProbeKindSteamManifest,
		PreferredSteamAppID: "90",
	})
	if errProbe != nil {
		t.Fatalf("ProbeInstalledVersion: %v", errProbe)
	}
	if !result.Found || result.Version != "123" || result.SourcePath != "steamapps/appmanifest_90.acf" {
		t.Fatalf("ProbeInstalledVersion result = %+v, want configured response", result)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.probeReq.GetKind() != nodeprotov1.InstalledVersionProbeKind_INSTALLED_VERSION_PROBE_KIND_STEAM_MANIFEST {
		t.Fatalf("probe kind = %v, want steam", rec.probeReq.GetKind())
	}
	if rec.probeReq.GetPreferredSteamAppId() != "90" {
		t.Fatalf("preferred app = %q, want 90", rec.probeReq.GetPreferredSteamAppId())
	}
}
func TestGRPCClientQueryGameServerRoundTrips(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{
		queryResp: &nodeprotov1.QueryGameServerResponse{
			Kind: nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_PALWORLD,
			Palworld: &nodeprotov1.GameServerPalworldQueryInfo{
				Name:          "Remote Palworld",
				Version:       "v1.2.3",
				Players:       3,
				MaxPlayers:    20,
				UptimeSeconds: 120,
				Responded:     true,
				PlayerDetails: []*nodeprotov1.GameServerPlayer{{Name: "Alex", Id: "steam_1"}},
			},
		},
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	result, errQuery := client.QueryGameServer(t.Context(), node.GameServerQueryRequest{
		Kind:       node.GameServerQueryKindPalworld,
		IP:         "203.0.113.10",
		QueryPort:  8212,
		MaxPlayers: 20,
		Username:   "admin",
		Password:   "query-secret",
	})
	if errQuery != nil {
		t.Fatalf("QueryGameServer: %v", errQuery)
	}
	if result.Kind != node.GameServerQueryKindPalworld {
		t.Fatalf("kind = %v, want Palworld", result.Kind)
	}
	if result.Palworld.Players != 3 || result.Palworld.Version != "v1.2.3" || !result.Palworld.Responded {
		t.Fatalf("Palworld result = %+v, want configured response", result.Palworld)
	}
	if len(result.Palworld.PlayerDetails) != 1 || result.Palworld.PlayerDetails[0] != (node.GameServerPlayer{Name: "Alex", ID: "steam_1"}) {
		t.Fatalf("Palworld player details = %+v", result.Palworld.PlayerDetails)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.queryReq.GetKind() != nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_PALWORLD {
		t.Fatalf("query kind = %v, want Palworld", rec.queryReq.GetKind())
	}
	if rec.queryReq.GetIp() != "203.0.113.10" || rec.queryReq.GetQueryPort() != 8212 || rec.queryReq.GetMaxPlayers() != 20 {
		t.Fatalf("query request = %+v, want address and defaults", rec.queryReq)
	}
	if rec.queryReq.GetUsername() != "admin" || rec.queryReq.GetPassword() != "query-secret" {
		t.Fatal("query request did not preserve Palworld credentials")
	}
}

func TestGRPCClientQueryGameServerAcceptsLegacyRespondedFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		response  *nodeprotov1.QueryGameServerResponse
		responded func(node.GameServerQueryResult) bool
	}{
		{
			name: "minecraft",
			response: &nodeprotov1.QueryGameServerResponse{
				Kind:      nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_MINECRAFT,
				Minecraft: &nodeprotov1.GameServerMinecraftQueryInfo{},
			},
			responded: func(result node.GameServerQueryResult) bool { return result.Minecraft.Responded },
		},
		{
			name: "source",
			response: &nodeprotov1.QueryGameServerResponse{
				Kind:   nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_SOURCE,
				Source: &nodeprotov1.GameServerSourceQueryInfo{},
			},
			responded: func(result node.GameServerQueryResult) bool { return result.Source.Responded },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			recorder := &callRecorder{queryResp: test.response}
			url, fingerprint := newPinnedTestServer(t, recorder)
			client, errClient := nodeclient.NewGRPCClient("node", url, fingerprint, "s")
			if errClient != nil {
				t.Fatalf("NewGRPCClient: %v", errClient)
			}

			result, errQuery := client.QueryGameServer(t.Context(), node.GameServerQueryRequest{})
			if errQuery != nil {
				t.Fatalf("QueryGameServer: %v", errQuery)
			}
			if !test.responded(result) {
				t.Fatal("legacy query response was treated as a failed probe")
			}
		})
	}
}

func TestGRPCClientPerformGameServerPlayerActionRoundTrips(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, "s")
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	errAction := client.PerformGameServerPlayerAction(t.Context(), node.GameServerPlayerActionRequest{
		Kind:      node.GameServerQueryKindPalworld,
		Action:    node.GameServerPlayerActionBan,
		ProcessID: "server-1",
		IP:        "203.0.113.10",
		QueryPort: 8212,
		Username:  "admin",
		Password:  "secret",
		PlayerID:  "steam_1",
		Reason:    "Abuse",
	})
	if errAction != nil {
		t.Fatalf("PerformGameServerPlayerAction: %v", errAction)
	}

	rec.mu.Lock()
	request := rec.playerActionReq
	rec.mu.Unlock()
	if request.GetKind() != nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_PALWORLD ||
		request.GetAction() != nodeprotov1.GameServerPlayerAction_GAME_SERVER_PLAYER_ACTION_BAN ||
		request.GetPlayerId() != "steam_1" || request.GetReason() != "Abuse" {
		t.Fatalf("player action request = %+v", request)
	}
}

func TestGRPCClientPerformGameServerPlayerActionTranslatesValidationErrors(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{
		playerActionErr: connect.NewError(connect.CodeInvalidArgument, errors.New("invalid player")),
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, "s")
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	errAction := client.PerformGameServerPlayerAction(t.Context(), node.GameServerPlayerActionRequest{})
	if !errors.Is(errAction, node.ErrInvalidPlayerAction) {
		t.Fatalf("PerformGameServerPlayerAction error = %v, want invalid player action", errAction)
	}
}

func TestGRPCClientExecuteGameOperationRoundTrips(t *testing.T) {
	t.Parallel()
	recorder := &callRecorder{
		gameOperationResp: &nodeprotov1.ExecuteGameOperationResponse{
			Classification: nodeprotov1.GameOperationResultClassification_GAME_OPERATION_RESULT_CLASSIFICATION_CONFIRMED,
			Message:        "confirmed",
			TransportDetails: &nodeprotov1.GameOperationTransportDetails{
				Method:       "native dashboard",
				Verification: "permission read-back",
			},
		},
	}
	serverURL, fingerprint := newPinnedTestServer(t, recorder)
	client, errNew := nodeclient.NewGRPCClient("node", serverURL, fingerprint, "shared-secret")
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	result, errExecute := client.ExecuteGameOperation(t.Context(), node.GameOperationRequest{
		WorkingDirectory: "/srv/game",
		TokenName:        "operator",
		TokenSecret:      "native-secret",
		OperationID:      "player_access.add_administrator",
		Values: []node.GameOperationValue{
			{FieldID: "player", StringValue: new("EOS_PLAYER_1")},
			{FieldID: "permission_level", IntegerValue: new(int64(0))},
		},
	})
	if errExecute != nil {
		t.Fatalf("ExecuteGameOperation: %v", errExecute)
	}
	if result.Classification != node.GameOperationResultConfirmed || result.Message != "confirmed" ||
		result.TransportDetails.Verification != "permission read-back" {
		t.Fatalf("result = %+v", result)
	}

	recorder.mu.Lock()
	request := recorder.gameOperationReq
	authHeaders := append([]string(nil), recorder.authHeaders...)
	recorder.mu.Unlock()
	if request.GetWorkingDirectory() != "/srv/game" || request.GetTokenName() != "operator" ||
		request.GetTokenSecret() != "native-secret" || request.GetOperationId() != "player_access.add_administrator" ||
		len(request.GetValues()) != 2 || request.GetValues()[0].GetStringValue() != "EOS_PLAYER_1" ||
		request.GetValues()[1].GetIntegerValue() != 0 {
		t.Fatalf("request = %+v", request)
	}
	if !slices.Contains(authHeaders, "Bearer shared-secret") {
		t.Fatalf("authorization headers = %v", authHeaders)
	}
}

func TestGRPCClientQueryGameServerPreservesSourcePlayerList(t *testing.T) {
	t.Parallel()

	rec := &callRecorder{
		queryResp: &nodeprotov1.QueryGameServerResponse{
			Kind: nodeprotov1.GameServerQueryKind_GAME_SERVER_QUERY_KIND_SOURCE,
			Source: &nodeprotov1.GameServerSourceQueryInfo{
				Name:                "Remote Source",
				Players:             2,
				MaxPlayers:          24,
				PlayerList:          []string{"Alyx", "Gordon"},
				PlayerListSupported: true,
			},
		},
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, "s")
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	result, errQuery := client.QueryGameServer(t.Context(), node.GameServerQueryRequest{
		Kind:       node.GameServerQueryKindSource,
		IP:         "203.0.113.11",
		QueryPort:  27015,
		MaxPlayers: 24,
	})
	if errQuery != nil {
		t.Fatalf("QueryGameServer: %v", errQuery)
	}
	if result.Kind != node.GameServerQueryKindSource || result.Source == nil {
		t.Fatalf("Source query result = %+v, want Source payload", result)
	}
	if !result.Source.PlayerListSupported || !slices.Equal(result.Source.PlayerList, []string{"Alyx", "Gordon"}) {
		t.Fatalf("Source player data = %+v, want supported [Alyx Gordon]", result.Source)
	}
}

func TestGRPCClientQuerySevenDaysToDieMapMapsRequestAndTacticalResponse(t *testing.T) {
	t.Parallel()
	available := nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE
	rec := &callRecorder{sevenDaysMapResp: &nodeprotov1.QuerySevenDaysToDieMapResponse{
		Snapshot: &nodeprotov1.SevenDaysToDieMapSnapshot{
			Enabled: true, TileSize: 128, MaxZoom: 4,
			MapSize: &nodeprotov1.SevenDaysToDieMapVector{X: 6144, Y: 255, Z: 6144},
			Players: []*nodeprotov1.SevenDaysToDieMapPlayer{{
				Id: "player-1", Name: "Alex", Online: true,
				Position: &nodeprotov1.SevenDaysToDieMapVector{X: 1, Y: 2, Z: 3},
			}},
			Markers: []*nodeprotov1.SevenDaysToDieMapMarker{{
				Id: "f4c2d4ea-7e4d-46b0-aaf2-26ea769951d4", Name: "Trader", X: 10, Z: 20,
			}},
			NativeMarkerState: available,
			Claims: []*nodeprotov1.SevenDaysToDieLandClaim{{
				OwnerId: "Steam_1", OwnerName: "Owner", Active: true,
				Position: &nodeprotov1.SevenDaysToDieMapVector{X: 30, Y: 4, Z: 40}, Size: 41,
			}},
			ClaimsState: available,
			BloodMoon: &nodeprotov1.SevenDaysToDieMapBloodMoon{
				GameTime: &nodeprotov1.SevenDaysToDieGameTime{Day: 7, Hour: 22}, Active: true,
				NextBloodMoon:    &nodeprotov1.SevenDaysToDieGameTime{Day: 14, Hour: 22},
				NextBloodMoonEnd: &nodeprotov1.SevenDaysToDieGameTime{Day: 15, Hour: 4},
			},
			BloodMoonState: available,
			Hostiles: []*nodeprotov1.SevenDaysToDieMapEntity{{
				Name: "Zombie", Position: &nodeprotov1.SevenDaysToDieMapVector{X: 50, Z: 60},
			}},
			HostileState: available,
			Animals: []*nodeprotov1.SevenDaysToDieMapEntity{{
				Name: "Wolf", Position: &nodeprotov1.SevenDaysToDieMapVector{X: 70, Z: 80},
			}},
			AnimalState: available,
		},
	}}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, errClient := nodeclient.NewGRPCClient("node", url, fingerprint, "node-secret")
	if errClient != nil {
		t.Fatalf("NewGRPCClient: %v", errClient)
	}

	snapshot, errQuery := client.QuerySevenDaysToDieMap(t.Context(), node.SevenDaysToDieMapQueryRequest{
		WorkingDirectory: "C:/servers/7dtd", TokenName: "controller", TokenSecret: "web-api-secret", IncludeTactical: true,
	})
	if errQuery != nil {
		t.Fatalf("QuerySevenDaysToDieMap: %v", errQuery)
	}
	if snapshot == nil || len(snapshot.Players) != 1 || len(snapshot.NativeMarkers) != 1 || len(snapshot.Claims) != 1 ||
		snapshot.BloodMoon == nil || len(snapshot.Hostiles) != 1 || len(snapshot.Animals) != 1 {
		t.Fatalf("mapped snapshot = %+v", snapshot)
	}
	rec.mu.Lock()
	request := rec.sevenDaysMapReq
	rec.mu.Unlock()
	if request.GetWorkingDirectory() != "C:/servers/7dtd" || request.GetTokenName() != "controller" ||
		request.GetTokenSecret() != "web-api-secret" || !request.GetIncludeTactical() {
		t.Fatalf("map request = %+v", request)
	}
}

func TestGRPCClientQuerySevenDaysToDieWebAPIStatus(t *testing.T) {
	t.Parallel()

	active := false
	observedAt := time.Unix(1_700_000_000, 0).UTC()
	tests := []struct {
		name           string
		observedAt     *timestamppb.Timestamp
		wantObservedAt time.Time
	}{
		{name: "maps status and optional values", observedAt: timestamppb.New(observedAt), wantObservedAt: observedAt},
		{name: "invalid timestamp becomes zero", observedAt: &timestamppb.Timestamp{Seconds: 253_402_300_800}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			rec := &callRecorder{
				webAPIStatusResp: &nodeprotov1.QuerySevenDaysToDieWebAPIStatusResponse{
					Status: &nodeprotov1.SevenDaysToDieWebAPIStatus{
						ConnectionState: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
						ApiVersion:      "3.1.0",
						Capabilities: &nodeprotov1.SevenDaysToDieWebAPICapabilities{
							PlayerData: true, RuntimeSettings: true, NativeLog: true, WorldPopulation: true,
							HostileAndAnimalPositions: true, AccessControl: true, GamePermissions: true, CommandPermissions: true, ReportedMods: true,
							CommandExecution: true,
						},
						WorldTimeState:  nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
						WorldTime:       &nodeprotov1.SevenDaysToDieGameTime{Day: 12, Hour: 5, Minute: 30},
						BloodMoonState:  nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
						BloodMoonActive: &active,
						NextBloodMoon:   &nodeprotov1.SevenDaysToDieGameTime{Day: 14, Hour: 22},
						NextBloodMoonEnd: &nodeprotov1.SevenDaysToDieGameTime{
							Day: 15, Hour: 4,
						},
						KnownCommands: []string{"say", "teleport", "version"},
						ObservedAt:    test.observedAt,
					},
				},
			}
			url, fingerprint := newPinnedTestServer(t, rec)
			client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, "node-secret")
			if errNew != nil {
				t.Fatalf("NewGRPCClient: %v", errNew)
			}

			status, errQuery := client.QuerySevenDaysToDieWebAPIStatus(t.Context(), node.SevenDaysToDieWebAPIStatusQueryRequest{
				WorkingDirectory: "C:/servers/7dtd",
				TokenName:        "controller",
				TokenSecret:      "web-api-secret",
				IncludeTactical:  true,
			})
			if errQuery != nil {
				t.Fatalf("QuerySevenDaysToDieWebAPIStatus: %v", errQuery)
			}
			if status == nil || status.ConnectionState != node.SevenDaysToDieWebAPIConnectionStateAvailable || status.APIVersion != "3.1.0" {
				t.Fatalf("status = %+v", status)
			}
			if status.WorldTime == nil || *status.WorldTime != (node.SevenDaysToDieGameTime{Day: 12, Hour: 5, Minute: 30}) {
				t.Fatalf("world time = %+v", status.WorldTime)
			}
			if status.BloodMoonActive == nil || *status.BloodMoonActive {
				t.Fatalf("blood moon active = %v, want present false", status.BloodMoonActive)
			}
			if status.NextBloodMoon == nil || status.NextBloodMoon.Day != 14 || status.NextBloodMoonEnd == nil || status.NextBloodMoonEnd.Day != 15 {
				t.Fatalf("blood moon times = next %+v end %+v", status.NextBloodMoon, status.NextBloodMoonEnd)
			}
			if !slices.Equal(status.KnownCommands, []string{"say", "teleport", "version"}) {
				t.Fatalf("known commands = %v", status.KnownCommands)
			}
			if status.Capabilities != (node.SevenDaysToDieWebAPICapabilities{
				PlayerData: true, RuntimeSettings: true, NativeLog: true, WorldPopulation: true,
				HostileAndAnimalPositions: true, HostilePositions: true, AnimalPositions: true,
				AccessControl: true, GamePermissions: true, CommandPermissions: true, ReportedMods: true, CommandExecution: true,
			}) {
				t.Fatalf("capabilities = %+v", status.Capabilities)
			}
			if !status.ObservedAt.Equal(test.wantObservedAt) {
				t.Fatalf("observed at = %v, want %v", status.ObservedAt, test.wantObservedAt)
			}

			rec.mu.Lock()
			request := rec.webAPIStatusReq
			authHeaders := append([]string(nil), rec.authHeaders...)
			rec.mu.Unlock()
			if request.GetWorkingDirectory() != "C:/servers/7dtd" || request.GetTokenName() != "controller" ||
				request.GetTokenSecret() != "web-api-secret" || !request.GetIncludeTactical() {
				t.Fatal("request did not preserve the expected directory and credentials")
			}
			if !slices.Equal(authHeaders, []string{"Bearer node-secret"}) {
				t.Fatalf("authorization headers = %q", authHeaders)
			}
		})
	}
}

func TestGRPCClientQuerySevenDaysToDieOperationMetadata(t *testing.T) {
	t.Parallel()

	rec := &callRecorder{
		metadataResp: &nodeprotov1.QuerySevenDaysToDieOperationMetadataResponse{
			Result: &nodeprotov1.SevenDaysToDieOperationMetadata{
				Players: []*nodeprotov1.SevenDaysToDieOperationOption{{Label: "Alice", Value: "EOS_abc123", Description: "Saved player"}},
				Items: []*nodeprotov1.SevenDaysToDieOperationOption{{
					Label: "Wood", Value: "resourceWood", IconName: "resourceWood",
					Category: "Resources", AccentColor: "#8b5a2b",
				}},
			},
		},
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, "node-secret")
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	metadata, errQuery := client.QuerySevenDaysToDieOperationMetadata(t.Context(), node.SevenDaysToDieOperationMetadataQueryRequest{
		WorkingDirectory: "C:/servers/7dtd",
	})
	if errQuery != nil {
		t.Fatalf("QuerySevenDaysToDieOperationMetadata: %v", errQuery)
	}
	if metadata == nil || len(metadata.Players) != 1 || metadata.Players[0].Value != "EOS_abc123" ||
		len(metadata.Items) != 1 || metadata.Items[0].IconName != "resourceWood" ||
		metadata.Items[0].Category != "Resources" || metadata.Items[0].AccentColor != "#8b5a2b" {
		t.Fatalf("metadata = %+v", metadata)
	}
	rec.mu.Lock()
	request := rec.metadataReq
	rec.mu.Unlock()
	if request.GetWorkingDirectory() != "C:/servers/7dtd" {
		t.Fatalf("metadata request = %+v", request)
	}
}

func TestGRPCClientQuerySevenDaysToDiePlayers(t *testing.T) {
	t.Parallel()
	falseValue := false
	zeroInt := int32(0)
	zeroFloat := float32(0)
	recorder := &callRecorder{
		playersResp: &nodeprotov1.QuerySevenDaysToDiePlayersResponse{
			Result: &nodeprotov1.SevenDaysToDiePlayers{
				ConnectionState: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
				State:           nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
				Players: []*nodeprotov1.SevenDaysToDiePlayer{{
					Name: "Player", ActionId: "Steam_1", EntityId: "1", PlatformId: "Steam_1", CrossPlatformId: "EOS_1",
					Online: &falseValue, Ping: &zeroInt, Level: &zeroInt, Health: &zeroInt, Stamina: &zeroFloat,
					Score: &zeroInt, Deaths: &zeroInt, ZombieKills: &zeroInt, PlayerKills: &zeroInt, Banned: &falseValue,
				}},
			},
		},
	}
	url, fingerprint := newPinnedTestServer(t, recorder)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, "node-secret")
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	result, errQuery := client.QuerySevenDaysToDiePlayers(t.Context(), node.SevenDaysToDiePlayersQueryRequest{
		WorkingDirectory: "C:/servers/7dtd",
		TokenName:        "controller",
		TokenSecret:      "web-api-secret",
	})
	if errQuery != nil {
		t.Fatalf("QuerySevenDaysToDiePlayers: %v", errQuery)
	}
	if result == nil || result.ConnectionState != node.SevenDaysToDieWebAPIConnectionStateAvailable ||
		result.State != node.SevenDaysToDieWebAPIValueStateAvailable || len(result.Players) != 1 {
		t.Fatalf("result = %+v", result)
	}
	player := result.Players[0]
	if player.ActionID != "Steam_1" || player.Online == nil || *player.Online || player.Ping == nil || *player.Ping != 0 ||
		player.Stamina == nil || *player.Stamina != 0 || player.Banned == nil || *player.Banned {
		t.Fatalf("player = %+v", player)
	}
	recorder.mu.Lock()
	request := recorder.playersReq
	authHeaders := append([]string(nil), recorder.authHeaders...)
	recorder.mu.Unlock()
	if request.GetWorkingDirectory() != "C:/servers/7dtd" || request.GetTokenName() != "controller" || request.GetTokenSecret() != "web-api-secret" {
		t.Fatal("request did not preserve the expected directory and credentials")
	}
	if !slices.Equal(authHeaders, []string{"Bearer node-secret"}) {
		t.Fatalf("authorization headers = %q", authHeaders)
	}
}

func TestGRPCClientQuerySevenDaysToDieReportedMods(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mods    []*nodeprotov1.SevenDaysToDieReportedMod
		wantErr bool
	}{
		{
			name: "maps bounded inventory",
			mods: []*nodeprotov1.SevenDaysToDieReportedMod{{
				Name: "Example", DisplayName: "Example Mod", Description: "Description", Author: "Author", Version: "1.0",
			}},
		},
		{name: "rejects over-count inventory", mods: make([]*nodeprotov1.SevenDaysToDieReportedMod, node.SevenDaysToDieReportedModCountLimit+1), wantErr: true},
		{name: "rejects over-limit field", mods: []*nodeprotov1.SevenDaysToDieReportedMod{{Name: strings.Repeat("x", node.SevenDaysToDieReportedModFieldByteLimit+1)}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &callRecorder{
				reportedModsResp: &nodeprotov1.QuerySevenDaysToDieReportedModsResponse{
					Result: &nodeprotov1.SevenDaysToDieReportedMods{
						ConnectionState: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
						State:           nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
						Mods:            test.mods,
					},
				},
			}
			url, fingerprint := newPinnedTestServer(t, recorder)
			client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, "node-secret")
			if errNew != nil {
				t.Fatalf("NewGRPCClient: %v", errNew)
			}
			result, errQuery := client.QuerySevenDaysToDieReportedMods(t.Context(), node.SevenDaysToDieReportedModsQueryRequest{
				WorkingDirectory: "C:/servers/7dtd", TokenName: "controller", TokenSecret: "web-api-secret",
			})
			if (errQuery != nil) != test.wantErr {
				t.Fatalf("QuerySevenDaysToDieReportedMods() error = %v, want error %t", errQuery, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if result == nil || result.State != node.SevenDaysToDieWebAPIValueStateAvailable || len(result.Mods) != 1 ||
				result.Mods[0] != (node.SevenDaysToDieReportedMod{Name: "Example", DisplayName: "Example Mod", Description: "Description", Author: "Author", Version: "1.0"}) {
				t.Fatalf("result = %+v", result)
			}
			recorder.mu.Lock()
			request := recorder.reportedModsReq
			recorder.mu.Unlock()
			if request.GetWorkingDirectory() != "C:/servers/7dtd" || request.GetTokenName() != "controller" || request.GetTokenSecret() != "web-api-secret" {
				t.Fatal("request did not preserve the expected directory and credentials")
			}
		})
	}
}

func TestGRPCClientQuerySevenDaysToDieSandboxSettings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		settings []*nodeprotov1.SevenDaysToDieSandboxSetting
		wantErr  bool
	}{
		{name: "maps bounded settings", settings: []*nodeprotov1.SevenDaysToDieSandboxSetting{{Key: "EnemySpawn", Label: "Enemy spawning", EffectiveValue: "true"}}},
		{name: "rejects over-count settings", settings: make([]*nodeprotov1.SevenDaysToDieSandboxSetting, node.SevenDaysToDieSandboxSettingCountLimit+1), wantErr: true},
		{name: "rejects oversized text", settings: []*nodeprotov1.SevenDaysToDieSandboxSetting{{Key: "EnemySpawn", Description: strings.Repeat("x", node.SevenDaysToDieSandboxTextByteLimit+1)}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &callRecorder{sandboxResp: &nodeprotov1.QuerySevenDaysToDieSandboxSettingsResponse{
				Result: &nodeprotov1.SevenDaysToDieSandboxSettings{
					ConnectionState: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
					State:           nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
					ComparisonState: nodeprotov1.SevenDaysToDieSandboxComparisonState_SEVEN_DAYS_TO_DIE_SANDBOX_COMPARISON_STATE_MATCH,
					ConfiguredCode:  "ABC", EffectiveCode: "ABC", Settings: test.settings,
				},
			}}
			url, fingerprint := newPinnedTestServer(t, recorder)
			client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, "node-secret")
			if errNew != nil {
				t.Fatalf("NewGRPCClient: %v", errNew)
			}
			result, errQuery := client.QuerySevenDaysToDieSandboxSettings(t.Context(), node.SevenDaysToDieSandboxSettingsQueryRequest{
				WorkingDirectory: "C:/servers/7dtd", TokenName: "controller", TokenSecret: "web-api-secret",
			})
			if (errQuery != nil) != test.wantErr {
				t.Fatalf("QuerySevenDaysToDieSandboxSettings() error = %v, want error %t", errQuery, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if result == nil || result.ComparisonState != node.SevenDaysToDieSandboxComparisonStateMatch || len(result.Settings) != 1 || result.Settings[0].Key != "EnemySpawn" {
				t.Fatalf("result = %+v", result)
			}
			recorder.mu.Lock()
			request := recorder.sandboxReq
			authHeaders := append([]string(nil), recorder.authHeaders...)
			recorder.mu.Unlock()
			if request.GetWorkingDirectory() != "C:/servers/7dtd" || request.GetTokenName() != "controller" || request.GetTokenSecret() != "web-api-secret" {
				t.Fatal("request did not preserve the expected directory and credentials")
			}
			if !slices.Equal(authHeaders, []string{"Bearer node-secret"}) {
				t.Fatalf("authorization headers = %q", authHeaders)
			}
		})
	}
}

func TestGRPCClientQuerySevenDaysToDieWebAPIStatusStateMapping(t *testing.T) {
	t.Parallel()

	query := func(t *testing.T, status *nodeprotov1.SevenDaysToDieWebAPIStatus) *node.SevenDaysToDieWebAPIStatus {
		t.Helper()
		recorder := &callRecorder{
			webAPIStatusResp: &nodeprotov1.QuerySevenDaysToDieWebAPIStatusResponse{Status: status},
		}
		url, fingerprint := newPinnedTestServer(t, recorder)
		client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, "node-secret")
		if errNew != nil {
			t.Fatalf("NewGRPCClient: %v", errNew)
		}
		result, errQuery := client.QuerySevenDaysToDieWebAPIStatus(t.Context(), node.SevenDaysToDieWebAPIStatusQueryRequest{})
		if errQuery != nil {
			t.Fatalf("QuerySevenDaysToDieWebAPIStatus: %v", errQuery)
		}
		return result
	}

	t.Run("connection state", func(t *testing.T) {
		tests := []struct {
			name  string
			input nodeprotov1.SevenDaysToDieWebAPIConnectionState
			want  node.SevenDaysToDieWebAPIConnectionState
		}{
			{name: "unspecified", input: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_UNSPECIFIED, want: node.SevenDaysToDieWebAPIConnectionStateUnspecified},
			{name: "available", input: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE, want: node.SevenDaysToDieWebAPIConnectionStateAvailable},
			{name: "server offline", input: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE, want: node.SevenDaysToDieWebAPIConnectionStateServerOffline},
			{name: "dashboard disabled", input: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DASHBOARD_DISABLED, want: node.SevenDaysToDieWebAPIConnectionStateDashboardDisabled},
			{name: "misconfigured", input: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_MISCONFIGURED, want: node.SevenDaysToDieWebAPIConnectionStateMisconfigured},
			{name: "node unavailable", input: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_NODE_UNAVAILABLE, want: node.SevenDaysToDieWebAPIConnectionStateNodeUnavailable},
			{name: "unreachable", input: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_WEB_API_UNREACHABLE, want: node.SevenDaysToDieWebAPIConnectionStateUnreachable},
			{name: "discovery unsupported", input: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DISCOVERY_UNSUPPORTED, want: node.SevenDaysToDieWebAPIConnectionStateDiscoveryUnsupported},
			{name: "authentication denied", input: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AUTHENTICATION_DENIED, want: node.SevenDaysToDieWebAPIConnectionStateAuthenticationDenied},
			{name: "invalid response", input: nodeprotov1.SevenDaysToDieWebAPIConnectionState_SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_INVALID_RESPONSE, want: node.SevenDaysToDieWebAPIConnectionStateInvalidResponse},
			{name: "unknown", input: nodeprotov1.SevenDaysToDieWebAPIConnectionState(99), want: node.SevenDaysToDieWebAPIConnectionStateUnspecified},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got := query(t, &nodeprotov1.SevenDaysToDieWebAPIStatus{ConnectionState: test.input})
				if got.ConnectionState != test.want {
					t.Fatalf("connection state = %v, want %v", got.ConnectionState, test.want)
				}
			})
		}
	})

	t.Run("value state", func(t *testing.T) {
		tests := []struct {
			name  string
			input nodeprotov1.SevenDaysToDieWebAPIValueState
			want  node.SevenDaysToDieWebAPIValueState
		}{
			{name: "unspecified", input: nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED, want: node.SevenDaysToDieWebAPIValueStateUnspecified},
			{name: "available", input: nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE, want: node.SevenDaysToDieWebAPIValueStateAvailable},
			{name: "unsupported", input: nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED, want: node.SevenDaysToDieWebAPIValueStateUnsupported},
			{name: "permission denied", input: nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED, want: node.SevenDaysToDieWebAPIValueStatePermissionDenied},
			{name: "unavailable", input: nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE, want: node.SevenDaysToDieWebAPIValueStateUnavailable},
			{name: "unknown", input: nodeprotov1.SevenDaysToDieWebAPIValueState(99), want: node.SevenDaysToDieWebAPIValueStateUnspecified},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got := query(t, &nodeprotov1.SevenDaysToDieWebAPIStatus{
					WorldTimeState: test.input,
					BloodMoonState: test.input,
				})
				if got.WorldTimeState != test.want || got.BloodMoonState != test.want {
					t.Fatalf("value states = world %v, Blood Moon %v; want %v", got.WorldTimeState, got.BloodMoonState, test.want)
				}
			})
		}
	})
}

func TestGRPCClientReadFileReturnsBytes(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{readFileResponse: []byte("hello world")}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	data, errRead := client.ReadFile(t.Context(), "/srv", "hello.txt")
	if errRead != nil {
		t.Fatalf("ReadFile: %v", errRead)
	}
	if string(data) != "hello world" {
		t.Fatalf("unexpected body: %q", string(data))
	}
}
func TestGRPCClientStatFileReturnsMetadata(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{
		statFileResp: &nodeprotov1.StatFileResponse{
			Entry: &nodeprotov1.FileEntry{
				Name:         "archive.zip",
				Size:         42,
				LastModified: timestamppb.New(time.Unix(300, 0)),
			},
		},
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	entry, errStat := client.StatFile(t.Context(), "/srv", "archive.zip")
	if errStat != nil {
		t.Fatalf("StatFile: %v", errStat)
	}
	if entry.Name != "archive.zip" || entry.Size != 42 || entry.IsDirectory {
		t.Fatalf("StatFile entry mismatch: %+v", entry)
	}
}
func TestGRPCClientStreamFileReturnsReader(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{streamFileChunks: [][]byte{[]byte("hello "), []byte("stream")}}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	reader, errStream := client.StreamFile(t.Context(), "/srv", "archive.zip")
	if errStream != nil {
		t.Fatalf("StreamFile: %v", errStream)
	}
	data, errRead := io.ReadAll(reader)
	errClose := reader.Close()
	if errRead != nil {
		t.Fatalf("ReadAll stream: %v", errRead)
	}
	if errClose != nil {
		t.Fatalf("Close stream: %v", errClose)
	}
	if string(data) != "hello stream" {
		t.Fatalf("StreamFile data = %q, want %q", string(data), "hello stream")
	}
}
func TestGRPCClientFileArchiveStreamsProgress(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{
		fileArchiveResp: []*nodeprotov1.CreateFileArchiveResponse{
			{TotalFiles: 2, FilesCompressed: 0, TotalBytes: 20, CurrentFile: "a.log"},
			{TotalFiles: 2, FilesCompressed: 1, TotalBytes: 20, BytesCompressed: 10, CurrentFile: "a.log"},
			{RelativePath: "archives/logs.zip", TotalFiles: 2, FilesCompressed: 2, TotalBytes: 20, BytesCompressed: 20, CurrentFile: "b.log"},
		},
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	var progressEvents []node.ArchiveProgress
	archivePath, progress, errArchive := client.CreateFileArchiveWithProgress(
		t.Context(),
		"/srv",
		"archives/logs",
		[]string{"a.log", "b.log"},
		node.ArchiveCompressionZIP,
		node.ProtectionPolicy{ServerExecutable: "server.jar"},
		func(progress node.ArchiveProgress) error {
			progressEvents = append(progressEvents, progress)
			return nil
		},
	)
	if errArchive != nil {
		t.Fatalf("CreateFileArchiveWithProgress: %v", errArchive)
	}
	if archivePath != "archives/logs.zip" {
		t.Fatalf("archive path = %q, want %q", archivePath, "archives/logs.zip")
	}
	if progress.FilesCompressed != 2 || progress.BytesCompressed != 20 {
		t.Fatalf("archive final progress = %+v, want completed", progress)
	}
	if len(progressEvents) != 3 {
		t.Fatalf("progress event count = %d, want 3", len(progressEvents))
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.fileArchiveReq.GetDestinationArchivePath() != "archives/logs" {
		t.Fatalf("destination = %q, want %q", rec.fileArchiveReq.GetDestinationArchivePath(), "archives/logs")
	}
	if rec.fileArchiveReq.GetServerExecutable() != "server.jar" {
		t.Fatalf("server executable = %q, want server.jar", rec.fileArchiveReq.GetServerExecutable())
	}
}
func TestGRPCClientFileExtractStreamsProgress(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{
		fileExtractResp: []*nodeprotov1.ExtractFileArchiveResponse{
			{TotalFiles: 1, FilesExtracted: 0, TotalBytes: 10, CurrentFile: "server.properties"},
			{ExtractedPaths: []string{"server.properties"}, TotalFiles: 1, FilesExtracted: 1, TotalBytes: 10, BytesExtracted: 10, CurrentFile: "server.properties"},
		},
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	var progressEvents []node.ExtractProgress
	extracted, progress, errExtract := client.ExtractFileArchiveWithProgress(
		t.Context(),
		"/srv",
		"imports/bundle.zip",
		"restored",
		node.ProtectionPolicy{BaseCommand: "./run.sh"},
		func(progress node.ExtractProgress) error {
			progressEvents = append(progressEvents, progress)
			return nil
		},
	)
	if errExtract != nil {
		t.Fatalf("ExtractFileArchiveWithProgress: %v", errExtract)
	}
	if len(extracted) != 1 || extracted[0] != "server.properties" {
		t.Fatalf("extracted paths = %v, want [server.properties]", extracted)
	}
	if progress.FilesExtracted != 1 || progress.BytesExtracted != 10 {
		t.Fatalf("extract final progress = %+v, want completed", progress)
	}
	if len(progressEvents) != 2 {
		t.Fatalf("progress event count = %d, want 2", len(progressEvents))
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.fileExtractReq.GetDestinationDirectoryPath() != "restored" {
		t.Fatalf("destination = %q, want restored", rec.fileExtractReq.GetDestinationDirectoryPath())
	}
	if rec.fileExtractReq.GetBaseCommand() != "./run.sh" {
		t.Fatalf("base command = %q, want ./run.sh", rec.fileExtractReq.GetBaseCommand())
	}
}
func TestGRPCClientStartProcessSendsNormalizedRequest(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	cfg := node.ProcessConfig{
		ID:               "gs-1",
		ExecutionID:      "execution-1",
		Name:             "server",
		BaseCommand:      "./run.sh",
		Args:             []string{"-p", "27015"},
		WorkingDirectory: "/games/gs-1",
		User:             "xylona",
		NodeID:           "node",
		ServiceID:        "svc-1",
		StopTimeout:      20 * time.Second,
		LaunchEnv: map[string]string{
			"XYLONA_TEST_TOKEN": "secret-value",
		},
		InputTelnet: &node.TelnetInput{Port: 8081, Password: t.Name()},
	}
	errStart := client.StartProcess(t.Context(), cfg, xylona.Status_ONLINE)
	if errStart != nil {
		t.Fatalf("StartProcess: %v", errStart)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	got := rec.startProcessReq
	if got.GetId() != "gs-1" || got.GetBaseCommand() != "./run.sh" {
		t.Fatalf("StartProcess request missing fields: %+v", got)
	}
	if got.GetInitialStatus() != nodeprotov1.ProcessStatus_PROCESS_STATUS_ONLINE {
		t.Fatalf("initial status: got %v want ONLINE", got.GetInitialStatus())
	}
	if got.GetStopTimeoutSeconds() != 20 {
		t.Fatalf("stop timeout seconds: got %d want 20", got.GetStopTimeoutSeconds())
	}
	if got.GetLaunchEnv()["XYLONA_TEST_TOKEN"] != "secret-value" {
		t.Fatalf("launch env was not sent: %+v", got.GetLaunchEnv())
	}
	if got.GetExecutionId() != "execution-1" {
		t.Fatalf("execution ID = %q, want execution-1", got.GetExecutionId())
	}
	if got.GetTelnetInput().GetPort() != 8081 || got.GetTelnetInput().GetPassword() != t.Name() {
		t.Fatalf("telnet input = %+v", got.GetTelnetInput())
	}
}

func TestGRPCClientRuntimeCapabilities(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{
		runtimeCapsResp: &nodeprotov1.GetRuntimeCapabilitiesResponse{
			ProtocolVersion:          node.RuntimeProtocolVersion,
			LaunchEnv:                true,
			ReliableProcessLifecycle: true,
			TelnetInput:              true,
			RconInput:                true,
			RestInput:                true,
			PlayerActions:            true,
			PalworldMap:              true,
			SevenDaysToDieMap:        true,
			MinecraftMap:             true,
			GameOperations: []*nodeprotov1.GameOperationSupport{
				{GameId: "7_days_to_die", OperationIds: []string{"player_access.add_administrator"}},
			},
		},
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	caps, errCaps := client.GetRuntimeCapabilities(t.Context())
	if errCaps != nil {
		t.Fatalf("GetRuntimeCapabilities: %v", errCaps)
	}
	if caps.ProtocolVersion != node.RuntimeProtocolVersion || !caps.LaunchEnv || !caps.ReliableProcessLifecycle ||
		!caps.TelnetInput || !caps.RCONInput || !caps.RESTInput || !caps.PlayerActions || !caps.PalworldMap ||
		!caps.SevenDaysToDieMap || !caps.MinecraftMap ||
		!caps.SupportsGameOperation("7_days_to_die", "player_access.add_administrator") {
		t.Fatalf("runtime capabilities = %+v, want all optional features", caps)
	}
}

func TestGRPCClientRemoteConsoleInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config node.ProcessConfig
		check  func(*testing.T, *nodeprotov1.StartProcessRequest)
	}{
		{
			name: "RCON",
			config: node.ProcessConfig{
				InputRCON: &node.RCONInput{
					Host:     "127.0.0.1",
					Port:     27015,
					Password: "secret",
					Protocol: node.RCONProtocolSource,
				},
			},
			check: func(t *testing.T, request *nodeprotov1.StartProcessRequest) {
				t.Helper()
				input := request.GetRconInput()
				if input.GetHost() != "127.0.0.1" || input.GetPort() != 27015 ||
					input.GetPassword() != "secret" ||
					input.GetProtocol() != nodeprotov1.RCONProtocol_RCON_PROTOCOL_SOURCE {
					t.Fatalf("RCON input = %+v", input)
				}
			},
		},
		{
			name: "Satisfactory REST",
			config: node.ProcessConfig{
				InputREST: &node.RESTInput{
					Host:              "127.0.0.1",
					Port:              7777,
					Kind:              node.RESTInputKindSatisfactory,
					Password:          "admin-password",
					PreviousPasswords: []string{"older-password", "previous-password"},
				},
			},
			check: func(t *testing.T, request *nodeprotov1.StartProcessRequest) {
				t.Helper()
				input := request.GetRestInput()
				if input.GetHost() != "127.0.0.1" || input.GetPort() != 7777 ||
					input.GetKind() != nodeprotov1.RESTInputKind_REST_INPUT_KIND_SATISFACTORY ||
					input.GetPassword() != "admin-password" ||
					!slices.Equal(input.GetPreviousPasswords(), []string{"older-password", "previous-password"}) {
					t.Fatalf("REST input = %+v", input)
				}
			},
		},
		{
			name: "Palworld REST",
			config: node.ProcessConfig{
				InputREST: &node.RESTInput{
					Host:     "127.0.0.1",
					Port:     8212,
					Kind:     node.RESTInputKindPalworld,
					Password: "palworld-admin-password",
				},
			},
			check: func(t *testing.T, request *nodeprotov1.StartProcessRequest) {
				t.Helper()
				input := request.GetRestInput()
				if input.GetHost() != "127.0.0.1" || input.GetPort() != 8212 ||
					input.GetKind() != nodeprotov1.RESTInputKind_REST_INPUT_KIND_PALWORLD ||
					input.GetPassword() != "palworld-admin-password" ||
					len(input.GetPreviousPasswords()) != 0 {
					t.Fatalf("Palworld REST input = %+v", input)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			recorder := &callRecorder{}
			url, fingerprint := newPinnedTestServer(t, recorder)
			client, errClient := nodeclient.NewGRPCClient("node", url, fingerprint, "s")
			if errClient != nil {
				t.Fatalf("NewGRPCClient() error = %v", errClient)
			}
			tc.config.ID = "server-1"
			tc.config.BaseCommand = "server"
			errStart := client.StartProcess(t.Context(), tc.config, xylona.Status_ONLINE)
			if errStart != nil {
				t.Fatalf("StartProcess() error = %v", errStart)
			}
			recorder.mu.Lock()
			defer recorder.mu.Unlock()
			tc.check(t, recorder.startProcessReq)
		})
	}
}

func TestGRPCClientConsoleInputErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		remoteErr  error
		want       error
		wantDetail string
		notWant    error
	}{
		{
			name: "preserves rejected command detail",
			remoteErr: connect.NewError(
				connect.CodeInvalidArgument,
				errors.New("Palworld API returned 401 Unauthorized"),
			),
			want:       node.ErrConsoleInputRejected,
			wantDetail: "Palworld API returned 401 Unauthorized",
			notWant:    node.ErrConsoleInputUnavailable,
		},
		{
			name:      "maps reconnectable transport failure",
			remoteErr: connect.NewError(connect.CodeFailedPrecondition, errors.New("input unavailable")),
			want:      node.ErrConsoleInputUnavailable,
			notWant:   node.ErrConsoleInputRejected,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			recorder := &callRecorder{consoleInputErr: tc.remoteErr}
			url, fingerprint := newPinnedTestServer(t, recorder)
			client, errNew := nodeclient.NewGRPCClient("node-1", url, fingerprint, "secret")
			if errNew != nil {
				t.Fatalf("NewGRPCClient() error = %v", errNew)
			}

			errSend := client.SendConsoleInput(t.Context(), "server-1", "/Save")
			if !errors.Is(errSend, tc.want) {
				t.Fatalf("SendConsoleInput() error = %v, want %v", errSend, tc.want)
			}
			if tc.notWant != nil && errors.Is(errSend, tc.notWant) {
				t.Fatalf("SendConsoleInput() error = %v, must not match %v", errSend, tc.notWant)
			}
			if tc.wantDetail != "" {
				var rejectedError *node.ConsoleInputRejectedError
				if !errors.As(errSend, &rejectedError) || rejectedError.Detail() != tc.wantDetail {
					t.Fatalf("SendConsoleInput() error = %v, want detail %q", errSend, tc.wantDetail)
				}
			}
		})
	}
}

func TestGRPCClientRuntimeCapabilitiesUnimplementedMeansNoOptionalFeatures(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	caps, errCaps := client.GetRuntimeCapabilities(t.Context())
	if errCaps != nil {
		t.Fatalf("GetRuntimeCapabilities: %v", errCaps)
	}
	if caps.ProtocolVersion != 0 || caps.LaunchEnv || caps.ReliableProcessLifecycle || caps.TelnetInput ||
		caps.RCONInput || caps.RESTInput || caps.PlayerActions || caps.PalworldMap || caps.SevenDaysToDieMap ||
		caps.MinecraftMap || len(caps.GameOperations) != 0 {
		t.Fatalf("runtime capabilities = %+v, want empty capabilities", caps)
	}
}

func TestGRPCClientQueryPalworldMapRoundTripsExactActors(t *testing.T) {
	t.Parallel()
	collectedAt := time.Now().UTC().Truncate(time.Second)
	recorder := &callRecorder{
		palworldMapResp: &nodeprotov1.QueryPalworldMapResponse{
			Snapshot: &nodeprotov1.PalworldMapSnapshot{
				Source:      "game-data",
				CollectedAt: timestamppb.New(collectedAt),
				Health: &nodeprotov1.PalworldMapHealth{
					ServerFps:         60,
					ServerFrameTimeMs: 16.67,
					CurrentPlayers:    4,
					MaxPlayers:        32,
					UptimeSeconds:     3600,
					BaseCampCount:     3,
					Days:              99,
				},
				Actors: []*nodeprotov1.PalworldMapActor{
					{
						Key:       "player-1",
						Kind:      nodeprotov1.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_PLAYER,
						Name:      "Alex",
						GuildKey:  "guild-key",
						LocationX: 123.456,
						LocationY: -987.654,
						LocationZ: 42.25,
					},
				},
			},
		},
	}
	url, fingerprint := newPinnedTestServer(t, recorder)
	client, errClient := nodeclient.NewGRPCClient("node", url, fingerprint, "s")
	if errClient != nil {
		t.Fatalf("NewGRPCClient() error = %v", errClient)
	}

	snapshot, errQuery := client.QueryPalworldMap(t.Context(), node.PalworldMapQueryRequest{
		IP:        "127.0.0.1",
		QueryPort: 8212,
		Username:  "admin",
		Password:  "secret",
	})
	if errQuery != nil {
		t.Fatalf("QueryPalworldMap() error = %v", errQuery)
	}
	if snapshot == nil || !snapshot.CollectedAt.Equal(collectedAt) || len(snapshot.Actors) != 1 {
		t.Fatalf("QueryPalworldMap() = %+v", snapshot)
	}
	actor := snapshot.Actors[0]
	if actor.Name != "Alex" || actor.GuildKey != "guild-key" || actor.LocationX != 123.456 || actor.LocationY != -987.654 || actor.LocationZ != 42.25 {
		t.Fatalf("QueryPalworldMap() actor = %+v", actor)
	}
	wantHealth := node.PalworldMapHealth{
		ServerFPS:         60,
		ServerFrameTimeMS: 16.67,
		CurrentPlayers:    4,
		MaxPlayers:        32,
		UptimeSeconds:     3600,
		BaseCampCount:     3,
		Days:              99,
	}
	if snapshot.Health == nil || *snapshot.Health != wantHealth {
		t.Fatalf("QueryPalworldMap() health = %+v", snapshot.Health)
	}
	recorder.mu.Lock()
	recordedRequest := recorder.palworldMapReq
	recorder.mu.Unlock()
	if recordedRequest.GetQueryPort() != 8212 || recordedRequest.GetUsername() != "admin" || recordedRequest.GetPassword() != "secret" {
		t.Fatalf("QueryPalworldMap() request = %+v", recordedRequest)
	}
}

func TestGRPCClientStartProcessPropagatesServerError(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{errOverride: connect.NewError(connect.CodeNotFound, errors.New("missing"))}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	errStart := client.StartProcess(t.Context(), node.ProcessConfig{ID: "x"}, xylona.Status_ONLINE)
	if errStart == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(errStart, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist via not-found mapping, got %v", errStart)
	}
}
func TestGRPCClientInvalidArgumentMapsToInvalidPath(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{errOverride: connect.NewError(connect.CodeInvalidArgument, node.ErrInvalidPath)}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	errStart := client.StartProcess(t.Context(), node.ProcessConfig{ID: "x"}, xylona.Status_ONLINE)
	if errStart == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(errStart, node.ErrInvalidPath) {
		t.Fatalf("expected ErrInvalidPath via invalid-argument mapping, got %v", errStart)
	}
}
func TestGRPCClientGetNodeSnapshotRoundTripsFields(t *testing.T) {
	t.Parallel()
	exitCode := int32(9)
	rec := &callRecorder{
		nodeSnapshot: &nodeprotov1.NodeSnapshot{
			CpuModel:    "stub-cpu",
			CpuCores:    8,
			TotalMemory: 16 * 1024 * 1024,
			Processes: []*nodeprotov1.ProcessSnapshot{
				{
					Id:                   "p1",
					ExecutionId:          "execution-1",
					Name:                 "proc-1",
					Status:               "OFFLINE",
					PreviousStatus:       "UPDATING",
					TransitionSequence:   2,
					ExitCode:             &exitCode,
					DiskValid:            new(true),
					CpuValid:             new(true),
					MetricsValid:         new(true),
					IoValid:              new(true),
					ConnectionCountValid: new(true),
				},
				{
					Id:     "legacy-process",
					Status: "ONLINE",
				},
				{
					Id:     "legacy-offline-process",
					Status: "OFFLINE",
				},
			},
			Collected: timestamppb.Now(),
		},
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	snap, errSnap := client.GetNodeSnapshot(t.Context())
	if errSnap != nil {
		t.Fatalf("GetNodeSnapshot: %v", errSnap)
	}
	if snap.CPUModel != "stub-cpu" || snap.CPUCores != 8 {
		t.Fatalf("snapshot mismatch: %+v", snap)
	}
	if len(snap.Processes) != 3 || snap.Processes[0].ID != "p1" {
		t.Fatalf("processes mismatch: %+v", snap.Processes)
	}
	process := snap.Processes[0]
	if process.ExecutionID != "execution-1" || process.PreviousStatus != "UPDATING" ||
		process.TransitionSequence != 2 || !process.ExitCodeKnown || process.ExitCode != 9 {
		t.Fatalf("process lifecycle mismatch: %+v", process)
	}
	if !process.DiskValid || !process.CPUValid || !process.MetricsValid ||
		!process.IOValid || !process.ConnectionCountValid {
		t.Fatalf("process metric validity mismatch: %+v", process)
	}
	legacyProcess := snap.Processes[1]
	if !legacyProcess.DiskValid || !legacyProcess.CPUValid || !legacyProcess.MetricsValid ||
		!legacyProcess.IOValid || !legacyProcess.ConnectionCountValid {
		t.Fatalf("legacy process metric validity should default to available: %+v", legacyProcess)
	}
	legacyOfflineProcess := snap.Processes[2]
	if legacyOfflineProcess.DiskValid || legacyOfflineProcess.CPUValid || legacyOfflineProcess.MetricsValid ||
		legacyOfflineProcess.IOValid || legacyOfflineProcess.ConnectionCountValid {
		t.Fatalf("legacy offline process metric validity should default to unavailable: %+v", legacyOfflineProcess)
	}
}
func TestGRPCClientListBindableIPsRoundTripsFields(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{
		bindableIPsResp: &nodeprotov1.ListBindableIPsResponse{
			Ips: []*nodeprotov1.BindableIP{
				{Address: "127.0.0.1", Usable: true},
				{Address: "203.0.113.42", Usable: true, External: true},
			},
		},
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	ips, errList := client.ListBindableIPs(t.Context())
	if errList != nil {
		t.Fatalf("ListBindableIPs: %v", errList)
	}
	if len(ips) != 2 {
		t.Fatalf("got %d IPs, want 2", len(ips))
	}
	if ips[0].Address != "127.0.0.1" || !ips[0].Usable {
		t.Fatalf("first IP mismatch: %+v", ips[0])
	}
	if ips[1].Address != "203.0.113.42" || !ips[1].External {
		t.Fatalf("second IP mismatch: %+v", ips[1])
	}
}
func TestGRPCClientStreamEventsDeliversPayloadAndClosesOnCtx(t *testing.T) {
	t.Parallel()
	exitCode := int32(17)
	rec := &callRecorder{
		streamEvents: []*nodeprotov1.Event{
			{
				Timestamp: timestamppb.Now(),
				Payload: &nodeprotov1.Event_ProcessStatus{
					ProcessStatus: &nodeprotov1.ProcessStatusEvent{
						ProcessId:          "p1",
						Status:             "OFFLINE",
						OldStatus:          "ONLINE",
						IntentionalStop:    false,
						ExitCode:           &exitCode,
						ExecutionId:        "execution-1",
						TransitionSequence: 2,
						Replayed:           true,
					},
				},
			},
			{
				Timestamp: timestamppb.Now(),
				Payload: &nodeprotov1.Event_ConsoleOutput{
					ConsoleOutput: &nodeprotov1.ConsoleChunk{GameServerId: "p1", Text: "ready"},
				},
			},
		},
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	events, errStream := client.StreamEvents(ctx)
	if errStream != nil {
		t.Fatalf("StreamEvents: %v", errStream)
	}

	seen := make([]node.Event, 0, 2)
	for ev := range events {
		seen = append(seen, ev)
	}
	if len(seen) != 2 {
		t.Fatalf("got %d events, want 2", len(seen))
	}
	if seen[0].Type != node.EventTypeProcessStatus || seen[0].ProcessID != "p1" {
		t.Fatalf("first event: %+v", seen[0])
	}
	if seen[0].OldStatus != "ONLINE" || seen[0].ExecutionID != "execution-1" ||
		seen[0].TransitionSequence != 2 || !seen[0].ExitCodeKnown || seen[0].ExitCode != 17 || !seen[0].Replayed {
		t.Fatalf("first lifecycle event: %+v", seen[0])
	}
	if seen[1].Type != node.EventTypeConsoleOutput {
		t.Fatalf("second event type: %v", seen[1].Type)
	}
}
func TestGRPCClientStreamConsoleOutputDeliversChunks(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{
		streamConsole: []*nodeprotov1.ConsoleChunk{
			{GameServerId: "p1", Text: "line 1"},
			{GameServerId: "p1", Text: "line 2"},
		},
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	stream, errStream := client.StreamConsoleOutput(ctx, "p1")
	if errStream != nil {
		t.Fatalf("StreamConsoleOutput: %v", errStream)
	}

	var seen []node.ConsoleChunk
	for chunk := range stream {
		seen = append(seen, chunk)
	}

	if len(seen) != 2 {
		t.Fatalf("got %d chunks, want 2", len(seen))
	}
	if seen[0].ProcessID != "p1" || seen[0].Data != "line 1" {
		t.Fatalf("first chunk mismatch: %+v", seen[0])
	}
}
func TestGRPCClientCloseClearsIdleConnections(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, "s")
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}
	errPing := client.Ping(t.Context())
	if errPing != nil {
		t.Fatalf("Ping: %v", errPing)
	}
	var closer io.Closer = client
	errClose := closer.Close()
	if errClose != nil {
		t.Fatalf("Close: %v", errClose)
	}
}
func TestGRPCClientWrongFingerprintFailsTLS(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{}
	url, _ := newPinnedTestServer(t, rec)
	wrongFingerprint := strings.Repeat("0", 64)

	client, errNew := nodeclient.NewGRPCClient("node", url, wrongFingerprint, "s")
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}
	errPing := client.Ping(t.Context())
	if errPing == nil {
		t.Fatal("expected TLS handshake failure with wrong fingerprint")
	}
}
