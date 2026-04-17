package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/nodetls"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1/nodeprotoconnect"
	"github.com/ClintonCollins/Xylona/supervisor"
)

// newTestServer wires up a pinned-TLS httptest server with a nodeServiceServer
// wrapping a real *pkg/node.Node (backed by a fresh supervisor, no DB). Returns
// the server URL and the cert fingerprint.
func newTestServer(t *testing.T, sharedSecret string) (string, string) {
	t.Helper()

	certPEM, keyPEM, fingerprint, errGen := nodetls.GenerateSelfSigned(context.Background(), "test-node")
	if errGen != nil {
		t.Fatalf("GenerateSelfSigned: %v", errGen)
	}
	tlsConfig, errTLS := nodetls.NewServerTLSConfig(certPEM, keyPEM)
	if errTLS != nil {
		t.Fatalf("NewServerTLSConfig: %v", errTLS)
	}

	supInst, errSup := supervisor.New(t.Context())
	if errSup != nil {
		t.Fatalf("supervisor.New: %v", errSup)
	}
	n := node.New(t.Context(), supInst, nil)

	svc := newNodeServiceServer(n, sharedSecret)
	mux := http.NewServeMux()
	path, handler := nodeprotoconnect.NewNodeServiceHandler(svc)
	mux.Handle(path, handler)

	server := httptest.NewUnstartedServer(mux)
	server.TLS = tlsConfig
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	return server.URL, fingerprint
}

// TestServerAuthorization verifies that requests without a bearer token or
// with the wrong token are rejected, and a matching token is accepted.
func TestServerAuthorization(t *testing.T) {
	t.Parallel()

	const secret = "correct-horse-battery-staple"

	t.Run("valid secret is accepted", func(t *testing.T) {
		t.Parallel()
		url, fingerprint := newTestServer(t, secret)
		client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, secret)
		if errNew != nil {
			t.Fatalf("NewGRPCClient: %v", errNew)
		}
		errPing := client.Ping(t.Context())
		if errPing != nil {
			t.Fatalf("Ping with valid secret failed: %v", errPing)
		}
	})

	t.Run("wrong secret is rejected", func(t *testing.T) {
		t.Parallel()
		url, fingerprint := newTestServer(t, secret)
		client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, "bad-secret")
		if errNew != nil {
			t.Fatalf("NewGRPCClient: %v", errNew)
		}
		errPing := client.Ping(t.Context())
		if errPing == nil {
			t.Fatal("expected rejection with wrong secret")
		}
		if !strings.Contains(errPing.Error(), "unauthenticated") {
			t.Fatalf("expected unauthenticated error, got %v", errPing)
		}
	})

	t.Run("missing header is rejected via direct connect client", func(t *testing.T) {
		t.Parallel()
		url, fingerprint := newTestServer(t, secret)

		httpClient, errClient := nodetls.NewPinnedTLSClient(fingerprint)
		if errClient != nil {
			t.Fatalf("NewPinnedTLSClient: %v", errClient)
		}
		connectClient := nodeprotoconnect.NewNodeServiceClient(httpClient, url)

		_, errPing := connectClient.Ping(t.Context(), connect.NewRequest(&nodeprotov1.PingRequest{}))
		if errPing == nil {
			t.Fatal("expected error for missing auth header")
		}
	})
}

// TestNodeServiceServerListFiles drives the full client->server round trip for
// ListFiles, using a directory materialized in the test filesystem.
func TestNodeServiceServerListFiles(t *testing.T) {
	t.Parallel()

	const secret = "test-secret"
	url, fingerprint := newTestServer(t, secret)
	client, errNew := nodeclient.NewGRPCClient("node", url, fingerprint, secret)
	if errNew != nil {
		t.Fatalf("NewGRPCClient: %v", errNew)
	}

	// Materialize a single file so ListFiles has something to return.
	dir := t.TempDir()
	contents := []byte("hello from listfiles test")
	errWrite := os.WriteFile(filepath.Join(dir, "sample.txt"), contents, 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile: %v", errWrite)
	}

	entries, errList := client.ListFiles(t.Context(), dir, "")
	if errList != nil {
		t.Fatalf("ListFiles: %v", errList)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "sample.txt" || entries[0].Size != int64(len(contents)) {
		t.Fatalf("entry mismatch: %+v", entries[0])
	}
}
