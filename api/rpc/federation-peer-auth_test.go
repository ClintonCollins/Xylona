package rpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/db/dbtest"
	"github.com/ClintonCollins/Xylona/helpers/federation"
)

func TestFederationPeerAuthMiddlewareAcceptsTrustedPeer(t *testing.T) {
	t.Parallel()

	conn := newFederationAuthTestConnection(t)

	clientCertPath := filepath.Join(t.TempDir(), "client.crt")
	clientKeyPath := filepath.Join(t.TempDir(), "client.key")
	clientMTLS, clientFingerprint, errClientMTLS := federation.NewMTLS("peer-node-id", 1, clientCertPath, clientKeyPath)
	if errClientMTLS != nil {
		t.Fatalf("NewFederationMTLS() client error = %v", errClientMTLS)
	}

	errInsertNode := insertRemoteNodeForFederationAuth(t, conn, "remote-node-row")
	if errInsertNode != nil {
		t.Fatalf("failed to insert remote node row: %v", errInsertNode)
	}

	_, errInsertTrust := conn.SQLDb.ExecContext(context.Background(), `
		insert into federation_trusted_peer (node_id, peer_node_id, peer_fingerprint, enabled, revoked)
		values (?, ?, ?, 1, 0)
	`, "remote-node-row", "peer-node-id", clientFingerprint)
	if errInsertTrust != nil {
		t.Fatalf("failed to insert trusted peer row: %v", errInsertTrust)
	}

	serverCertificate := loadTLSCertificate(t, filepath.Join(t.TempDir(), "server.crt"), filepath.Join(t.TempDir(), "server.key"), "server-node-id")
	clientCertificate := loadTLSCertificate(t, clientMTLS.CertPath(), clientMTLS.KeyPath(), "")

	handler := FederationPeerAuthMiddleware(conn)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, okIdentity := federationPeerIdentityFromContext(r.Context())
		if !okIdentity {
			t.Fatalf("expected federation peer identity in request context")
		}
		if identity.NodeID != "remote-node-row" {
			t.Fatalf("identity.NodeID = %q, want %q", identity.NodeID, "remote-node-row")
		}
		if identity.PeerNodeID != "peer-node-id" {
			t.Fatalf("identity.PeerNodeID = %q, want %q", identity.PeerNodeID, "peer-node-id")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	testServer := httptest.NewUnstartedServer(handler)
	testServer.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverCertificate},
		ClientAuth:   tls.RequireAnyClientCert,
	}
	testServer.StartTLS()
	t.Cleanup(testServer.Close)

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				Certificates:       []tls.Certificate{clientCertificate},
				InsecureSkipVerify: true, // #nosec G402 -- test-only server trust.
			},
		},
	}
	req, errReq := http.NewRequestWithContext(context.Background(), http.MethodGet, testServer.URL, nil)
	if errReq != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", errReq)
	}
	resp, errGet := httpClient.Do(req)
	if errGet != nil {
		t.Fatalf("GET trusted peer request error = %v", errGet)
	}
	t.Cleanup(func() {
		if errClose := resp.Body.Close(); errClose != nil {
			t.Errorf("failed to close response body: %v", errClose)
		}
	})

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func TestFederationPeerAuthMiddlewareRejectsUnknownPeer(t *testing.T) {
	t.Parallel()

	conn := newFederationAuthTestConnection(t)

	errInsertNode := insertRemoteNodeForFederationAuth(t, conn, "remote-node-row")
	if errInsertNode != nil {
		t.Fatalf("failed to insert remote node row: %v", errInsertNode)
	}

	clientCertificate := loadTLSCertificate(t, filepath.Join(t.TempDir(), "client.crt"), filepath.Join(t.TempDir(), "client.key"), "peer-node-id")
	serverCertificate := loadTLSCertificate(t, filepath.Join(t.TempDir(), "server.crt"), filepath.Join(t.TempDir(), "server.key"), "server-node-id")

	handler := FederationPeerAuthMiddleware(conn)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatalf("handler should not be invoked for unknown peer certificate")
	}))

	testServer := httptest.NewUnstartedServer(handler)
	testServer.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverCertificate},
		ClientAuth:   tls.RequireAnyClientCert,
	}
	testServer.StartTLS()
	t.Cleanup(testServer.Close)

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				Certificates:       []tls.Certificate{clientCertificate},
				InsecureSkipVerify: true, // #nosec G402 -- test-only server trust.
			},
		},
	}
	req, errReq := http.NewRequestWithContext(context.Background(), http.MethodGet, testServer.URL, nil)
	if errReq != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", errReq)
	}
	resp, errGet := httpClient.Do(req)
	if errGet != nil {
		t.Fatalf("GET unknown peer request error = %v", errGet)
	}
	t.Cleanup(func() {
		if errClose := resp.Body.Close(); errClose != nil {
			t.Errorf("failed to close response body: %v", errClose)
		}
	})

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func newFederationAuthTestConnection(t *testing.T) *db.Connection {
	t.Helper()
	return dbtest.NewMigratedConnection(t, "federation-auth.sqlite")
}

func insertRemoteNodeForFederationAuth(t *testing.T, conn *db.Connection, nodeID string) error {
	t.Helper()

	_, errInsert := conn.SQLDb.ExecContext(context.Background(), `
		insert into node (
			id, name, is_local, host, port, base_url, enabled, health_status, last_sync_status, version, protocol_version, capabilities, sync_interval_seconds, allow_insecure_tls
		) values (?, 'Remote Node', 0, '', 0, 'https://node.example.com', 1, 'healthy', '', '', 0, '', 60, 0)
	`, nodeID)
	if errInsert != nil {
		return fmt.Errorf("rpc: insert remote node for federation auth: %w", errInsert)
	}
	return nil
}

func loadTLSCertificate(t *testing.T, certPath string, keyPath string, nodeID string) tls.Certificate {
	t.Helper()

	if nodeID != "" {
		_, _, errMTLS := federation.NewMTLS(nodeID, 1, certPath, keyPath)
		if errMTLS != nil {
			t.Fatalf("NewFederationMTLS() error = %v", errMTLS)
		}
	}

	certificate, errLoad := tls.LoadX509KeyPair(certPath, keyPath)
	if errLoad != nil {
		t.Fatalf("tls.LoadX509KeyPair() error = %v", errLoad)
	}
	return certificate
}
