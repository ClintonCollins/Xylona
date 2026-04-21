package nodeclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/pkg/nodetls"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1/nodeprotoconnect"
)

type delayedStreamHandler struct {
	nodeprotoconnect.UnimplementedNodeServiceHandler
	delay time.Duration
}

func (h *delayedStreamHandler) StreamEvents(ctx context.Context, _ *connect.Request[nodeprotov1.StreamEventsRequest], stream *connect.ServerStream[nodeprotov1.Event]) error {
	select {
	case <-ctx.Done():
		return errors.New("stream events context canceled")
	case <-time.After(h.delay):
	}

	errSend := stream.Send(&nodeprotov1.Event{
		Timestamp: timestamppb.Now(),
		Payload: &nodeprotov1.Event_ProcessStatus{
			ProcessStatus: &nodeprotov1.ProcessStatusEvent{
				ProcessId: "stream-server",
				Status:    "ONLINE",
			},
		},
	})
	if errSend != nil {
		return errors.New("stream events send failed")
	}
	return nil
}

func (h *delayedStreamHandler) StreamConsoleOutput(ctx context.Context, _ *connect.Request[nodeprotov1.StreamConsoleOutputRequest], stream *connect.ServerStream[nodeprotov1.ConsoleChunk]) error {
	select {
	case <-ctx.Done():
		return errors.New("stream console output context canceled")
	case <-time.After(h.delay):
	}

	errSend := stream.Send(&nodeprotov1.ConsoleChunk{
		GameServerId: "stream-server",
		Text:         "ready",
	})
	if errSend != nil {
		return errors.New("stream console output send failed")
	}
	return nil
}

func newDelayedStreamServer(t *testing.T, delay time.Duration) (string, string) {
	t.Helper()

	certPEM, keyPEM, fingerprint, errGenerate := nodetls.GenerateSelfSigned(context.Background(), "stream-node")
	if errGenerate != nil {
		t.Fatalf("GenerateSelfSigned: %v", errGenerate)
	}

	tlsConfig, errTLS := nodetls.NewServerTLSConfig(certPEM, keyPEM)
	if errTLS != nil {
		t.Fatalf("NewServerTLSConfig: %v", errTLS)
	}

	handler := &delayedStreamHandler{delay: delay}
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

func TestGRPCClientStreamingBypassesHTTPClientTimeout(t *testing.T) {
	tests := []struct {
		name string
		read func(context.Context, *GRPCNodeClient) (string, error)
		want string
	}{
		{
			name: "event stream",
			read: func(ctx context.Context, client *GRPCNodeClient) (string, error) {
				stream, errStream := client.StreamEvents(ctx)
				if errStream != nil {
					return "", errStream
				}

				event, ok := <-stream
				if !ok {
					return "", errors.New("stream closed before first event")
				}
				return event.ProcessID, nil
			},
			want: "stream-server",
		},
		{
			name: "console stream",
			read: func(ctx context.Context, client *GRPCNodeClient) (string, error) {
				stream, errStream := client.StreamConsoleOutput(ctx, "stream-server")
				if errStream != nil {
					return "", errStream
				}

				chunk, ok := <-stream
				if !ok {
					return "", errors.New("stream closed before first console chunk")
				}
				return chunk.ProcessID, nil
			},
			want: "stream-server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, fingerprint := newDelayedStreamServer(t, 40*time.Millisecond)
			client, errNew := NewGRPCClient("node-1", url, fingerprint, "secret")
			if errNew != nil {
				t.Fatalf("NewGRPCClient: %v", errNew)
			}

			client.httpClient.Timeout = 10 * time.Millisecond

			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()

			got, errRead := tt.read(ctx, client)
			if errRead != nil {
				t.Fatalf("stream read: %v", errRead)
			}
			if got != tt.want {
				t.Fatalf("stream read = %q, want %q", got, tt.want)
			}
		})
	}
}
