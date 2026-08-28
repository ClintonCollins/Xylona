package nodeclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodetls"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1/nodeprotoconnect"
)

type delayedRPCHandler struct {
	nodeprotoconnect.UnimplementedNodeServiceHandler
	delay time.Duration
}

func (h *delayedRPCHandler) wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return errors.New("delayed RPC context canceled")
	case <-time.After(h.delay):
		return nil
	}
}

func (h *delayedRPCHandler) StreamEvents(ctx context.Context, _ *connect.Request[nodeprotov1.StreamEventsRequest], stream *connect.ServerStream[nodeprotov1.Event]) error {
	errWait := h.wait(ctx)
	if errWait != nil {
		return errWait
	}

	errSend := stream.Send(&nodeprotov1.Event{Timestamp: timestamppb.Now()})
	if errSend != nil {
		return fmt.Errorf("send delayed event: %w", errSend)
	}
	return nil
}

func (h *delayedRPCHandler) StreamConsoleOutput(ctx context.Context, _ *connect.Request[nodeprotov1.StreamConsoleOutputRequest], stream *connect.ServerStream[nodeprotov1.ConsoleChunk]) error {
	errWait := h.wait(ctx)
	if errWait != nil {
		return errWait
	}

	errSend := stream.Send(&nodeprotov1.ConsoleChunk{})
	if errSend != nil {
		return fmt.Errorf("send delayed console chunk: %w", errSend)
	}
	return nil
}

func (h *delayedRPCHandler) DownloadFileFromURL(ctx context.Context, _ *connect.Request[nodeprotov1.DownloadFileFromURLRequest]) (*connect.Response[nodeprotov1.DownloadFileFromURLResponse], error) {
	errWait := h.wait(ctx)
	if errWait != nil {
		return nil, errWait
	}
	return connect.NewResponse(&nodeprotov1.DownloadFileFromURLResponse{}), nil
}

func (h *delayedRPCHandler) CreateBackupArchive(ctx context.Context, _ *connect.Request[nodeprotov1.CreateBackupArchiveRequest]) (*connect.Response[nodeprotov1.CreateBackupArchiveResponse], error) {
	errWait := h.wait(ctx)
	if errWait != nil {
		return nil, errWait
	}
	return connect.NewResponse(&nodeprotov1.CreateBackupArchiveResponse{}), nil
}

func (h *delayedRPCHandler) ExtractBackupArchive(ctx context.Context, _ *connect.Request[nodeprotov1.ExtractBackupArchiveRequest]) (*connect.Response[nodeprotov1.ExtractBackupArchiveResponse], error) {
	errWait := h.wait(ctx)
	if errWait != nil {
		return nil, errWait
	}
	return connect.NewResponse(&nodeprotov1.ExtractBackupArchiveResponse{}), nil
}

func (h *delayedRPCHandler) EnsureMinecraftMap(ctx context.Context, _ *connect.Request[nodeprotov1.EnsureMinecraftMapRequest]) (*connect.Response[nodeprotov1.EnsureMinecraftMapResponse], error) {
	errWait := h.wait(ctx)
	if errWait != nil {
		return nil, errWait
	}
	return connect.NewResponse(&nodeprotov1.EnsureMinecraftMapResponse{}), nil
}

func newDelayedRPCServer(t *testing.T, delay time.Duration) (string, string) {
	t.Helper()

	certPEM, keyPEM, fingerprint, errGenerate := nodetls.GenerateSelfSigned(context.Background(), "stream-node")
	if errGenerate != nil {
		t.Fatalf("GenerateSelfSigned: %v", errGenerate)
	}

	tlsConfig, errTLS := nodetls.NewServerTLSConfig(certPEM, keyPEM)
	if errTLS != nil {
		t.Fatalf("NewServerTLSConfig: %v", errTLS)
	}

	handler := &delayedRPCHandler{delay: delay}
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

func TestGRPCClientLongRunningOperationsBypassHTTPClientTimeout(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *GRPCNodeClient) error
	}{
		{
			name: "event stream",
			run: func(ctx context.Context, client *GRPCNodeClient) error {
				stream, errStream := client.StreamEvents(ctx)
				if errStream != nil {
					return errStream
				}

				_, ok := <-stream
				if !ok {
					return errors.New("stream closed before first event")
				}
				return nil
			},
		},
		{
			name: "console stream",
			run: func(ctx context.Context, client *GRPCNodeClient) error {
				stream, errStream := client.StreamConsoleOutput(ctx, "stream-server")
				if errStream != nil {
					return errStream
				}

				_, ok := <-stream
				if !ok {
					return errors.New("stream closed before first console chunk")
				}
				return nil
			},
		},
		{
			name: "URL download",
			run: func(ctx context.Context, client *GRPCNodeClient) error {
				_, errDownload := client.DownloadFileFromURL(ctx, "server", "https://example.com/file", "", node.DownloadIntegrity{}, node.ProtectionPolicy{})
				return errDownload
			},
		},
		{
			name: "backup creation",
			run: func(ctx context.Context, client *GRPCNodeClient) error {
				_, _, errCreate := client.CreateBackupArchive(ctx, "server", nil, "backup.zip")
				return errCreate
			},
		},
		{
			name: "backup extraction",
			run: func(ctx context.Context, client *GRPCNodeClient) error {
				return client.ExtractBackupArchive(ctx, "server", "backup.zip", node.ExtractModeOverlay)
			},
		},
		{
			name: "Minecraft map setup",
			run: func(ctx context.Context, client *GRPCNodeClient) error {
				_, errEnsure := client.EnsureMinecraftMap(ctx, node.MinecraftMapEnsureRequest{})
				return errEnsure
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, fingerprint := newDelayedRPCServer(t, 40*time.Millisecond)
			client, errNew := NewGRPCClient("node-1", url, fingerprint, "secret")
			if errNew != nil {
				t.Fatalf("NewGRPCClient: %v", errNew)
			}

			client.httpClient.Timeout = 10 * time.Millisecond

			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()

			errRun := tt.run(ctx, client)
			if errRun != nil {
				t.Fatalf("long-running operation: %v", errRun)
			}
		})
	}
}
