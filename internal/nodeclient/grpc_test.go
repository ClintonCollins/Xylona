package nodeclient_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
	mu               sync.Mutex
	authHeaders      []string
	listFilesReq     *nodeprotov1.ListFilesRequest
	startProcessReq  *nodeprotov1.StartProcessRequest
	readFileResponse []byte
	streamFileChunks [][]byte
	streamWriteReqs  []*nodeprotov1.StreamWriteFileRequest
	streamWriteResp  *nodeprotov1.StreamWriteFileResponse
	listFilesResp    *nodeprotov1.ListFilesResponse
	statFileResp     *nodeprotov1.StatFileResponse
	bindableIPsResp  *nodeprotov1.ListBindableIPsResponse
	copyFilesReq     *nodeprotov1.CopyFilesRequest
	copyFilesResp    *nodeprotov1.CopyFilesResponse
	probeReq         *nodeprotov1.ProbeInstalledVersionRequest
	probeResp        *nodeprotov1.ProbeInstalledVersionResponse
	queryReq         *nodeprotov1.QueryGameServerRequest
	queryResp        *nodeprotov1.QueryGameServerResponse
	fileArchiveReq   *nodeprotov1.CreateFileArchiveRequest
	fileArchiveResp  []*nodeprotov1.CreateFileArchiveResponse
	fileExtractReq   *nodeprotov1.ExtractFileArchiveRequest
	fileExtractResp  []*nodeprotov1.ExtractFileArchiveResponse
	streamEvents     []*nodeprotov1.Event
	streamConsole    []*nodeprotov1.ConsoleChunk
	nodeSnapshot     *nodeprotov1.NodeSnapshot
	runtimeCapsResp  *nodeprotov1.GetRuntimeCapabilitiesResponse
	runtimeCapsErr   error
	errOverride      error
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
func TestGRPCClientIDIsStable(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{}
	url, fingerprint := newPinnedTestServer(t, rec)

	client, errNew := nodeclient.NewGRPCClient("my-node", url, fingerprint, "secret")
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}
	if client.ID() != "my-node" {
		t.Fatalf("ID: got %q want %q", client.ID(), "my-node")
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
}

func TestGRPCClientRuntimeCapabilities(t *testing.T) {
	t.Parallel()
	rec := &callRecorder{
		runtimeCapsResp: &nodeprotov1.GetRuntimeCapabilitiesResponse{
			ProtocolVersion: 7,
			LaunchEnv:       true,
		},
	}
	url, fingerprint := newPinnedTestServer(t, rec)
	client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

	caps, errCaps := client.GetRuntimeCapabilities(t.Context())
	if errCaps != nil {
		t.Fatalf("GetRuntimeCapabilities: %v", errCaps)
	}
	if caps.ProtocolVersion != 7 || !caps.LaunchEnv {
		t.Fatalf("runtime capabilities = %+v, want protocol 7 and launch env", caps)
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
	if caps != (node.RuntimeCapabilities{}) {
		t.Fatalf("runtime capabilities = %+v, want empty capabilities", caps)
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
	rec := &callRecorder{
		nodeSnapshot: &nodeprotov1.NodeSnapshot{
			CpuModel:    "stub-cpu",
			CpuCores:    8,
			TotalMemory: 16 * 1024 * 1024,
			Processes: []*nodeprotov1.ProcessSnapshot{
				{Id: "p1", Name: "proc-1", Status: "ONLINE"},
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
	if len(snap.Processes) != 1 || snap.Processes[0].ID != "p1" {
		t.Fatalf("processes mismatch: %+v", snap.Processes)
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
	rec := &callRecorder{
		streamEvents: []*nodeprotov1.Event{
			{
				Timestamp: timestamppb.Now(),
				Payload: &nodeprotov1.Event_ProcessStatus{
					ProcessStatus: &nodeprotov1.ProcessStatusEvent{ProcessId: "p1", Status: "ONLINE"},
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
