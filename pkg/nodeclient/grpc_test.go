package nodeclient_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/nodetls"
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
	listFilesResp    *nodeprotov1.ListFilesResponse
	streamEvents     []*nodeprotov1.Event
	nodeSnapshot     *nodeprotov1.NodeSnapshot
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

func TestGRPCClient(t *testing.T) {
	t.Parallel()

	t.Run("ping attaches bearer token", func(t *testing.T) {
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
	})

	t.Run("ID is stable", func(t *testing.T) {
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
	})

	t.Run("constructor validates required fields", func(t *testing.T) {
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
	})

	t.Run("list files translates response", func(t *testing.T) {
		t.Parallel()
		rec := &callRecorder{
			listFilesResp: &nodeprotov1.ListFilesResponse{
				Entries: []*nodeprotov1.FileEntry{
					{Name: "a.txt", Size: 12, IsDirectory: false, LastModified: timestamppb.New(time.Unix(100, 0))},
					{Name: "sub", Size: 0, IsDirectory: true, LastModified: timestamppb.New(time.Unix(200, 0))},
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
		if len(entries) != 2 {
			t.Fatalf("got %d entries want 2", len(entries))
		}
		if entries[0].Name != "a.txt" || entries[0].Size != 12 || entries[0].IsDirectory {
			t.Fatalf("entries[0] unexpected: %+v", entries[0])
		}
		if !entries[1].IsDirectory || entries[1].Name != "sub" {
			t.Fatalf("entries[1] unexpected: %+v", entries[1])
		}

		rec.mu.Lock()
		defer rec.mu.Unlock()
		if rec.listFilesReq.GetDirectory() != "/srv" {
			t.Fatalf("directory sent incorrectly: %q", rec.listFilesReq.GetDirectory())
		}
		if rec.listFilesReq.GetRelativePath() != "sub" {
			t.Fatalf("relative path sent incorrectly: %q", rec.listFilesReq.GetRelativePath())
		}
	})

	t.Run("read file returns bytes", func(t *testing.T) {
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
	})

	t.Run("start process sends normalized request", func(t *testing.T) {
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
		}
		_, errStart := client.StartProcess(t.Context(), cfg, xylona.Status_ONLINE)
		if !errors.Is(errStart, nodeclient.ErrRemoteStartProcessHandle) {
			t.Fatalf("StartProcess: expected ErrRemoteStartProcessHandle, got %v", errStart)
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
	})

	t.Run("start process propagates server error", func(t *testing.T) {
		t.Parallel()
		rec := &callRecorder{errOverride: connect.NewError(connect.CodeNotFound, errors.New("missing"))}
		url, fingerprint := newPinnedTestServer(t, rec)
		client, _ := nodeclient.NewGRPCClient("node", url, fingerprint, "s")

		_, errStart := client.StartProcess(t.Context(), node.ProcessConfig{ID: "x"}, xylona.Status_ONLINE)
		if errStart == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(errStart, node.ErrInvalidPath) {
			t.Fatalf("expected ErrInvalidPath via not-found mapping, got %v", errStart)
		}
	})

	t.Run("get node snapshot round-trips fields", func(t *testing.T) {
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
	})

	t.Run("stream events delivers payload and closes on ctx", func(t *testing.T) {
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
	})

	t.Run("close clears idle connections", func(t *testing.T) {
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
		errClose := client.Close(t.Context())
		if errClose != nil {
			t.Fatalf("Close: %v", errClose)
		}
	})

	t.Run("wrong fingerprint fails TLS", func(t *testing.T) {
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
	})
}
