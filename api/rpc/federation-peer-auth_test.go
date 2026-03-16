package rpc

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
)

func TestFederationPeerAuthMiddlewareAcceptsTrustedPeer(t *testing.T) {
	conn := newFederationAuthTestConnection(t)

	clientCertPath := filepath.Join(t.TempDir(), "client.crt")
	clientKeyPath := filepath.Join(t.TempDir(), "client.key")
	clientMTLS, clientFingerprint, errClientMTLS := helpers.NewFederationMTLS("peer-node-id", 1, clientCertPath, clientKeyPath)
	if errClientMTLS != nil {
		t.Fatalf("NewFederationMTLS() client error = %v", errClientMTLS)
	}

	errInsertNode := insertRemoteNodeForFederationAuth(t, conn, "remote-node-row")
	if errInsertNode != nil {
		t.Fatalf("failed to insert remote node row: %v", errInsertNode)
	}

	_, errInsertTrust := conn.SQLDb.Exec(`
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
	resp, errGet := httpClient.Get(testServer.URL)
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
	conn := newFederationAuthTestConnection(t)

	errInsertNode := insertRemoteNodeForFederationAuth(t, conn, "remote-node-row")
	if errInsertNode != nil {
		t.Fatalf("failed to insert remote node row: %v", errInsertNode)
	}

	clientCertificate := loadTLSCertificate(t, filepath.Join(t.TempDir(), "client.crt"), filepath.Join(t.TempDir(), "client.key"), "peer-node-id")
	serverCertificate := loadTLSCertificate(t, filepath.Join(t.TempDir(), "server.crt"), filepath.Join(t.TempDir(), "server.key"), "server-node-id")

	handler := FederationPeerAuthMiddleware(conn)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	resp, errGet := httpClient.Get(testServer.URL)
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

	conn := db.NewConnection(context.Background(), filepath.Join(t.TempDir(), "federation-auth.sqlite"))
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close db: %v", errClose)
		}
	})

	_, errCreateNode := conn.SQLDb.Exec(`
		create table node (
			id TEXT PRIMARY KEY NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			secret_key TEXT,
			is_local BOOLEAN NOT NULL DEFAULT FALSE,
			host TEXT NOT NULL DEFAULT '',
			port INTEGER NOT NULL DEFAULT 0,
			base_url TEXT NOT NULL DEFAULT '',
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			last_seen_at DATETIME,
			last_sync_at DATETIME,
			last_sync_status TEXT NOT NULL DEFAULT '',
			health_status TEXT NOT NULL DEFAULT '',
			version TEXT NOT NULL DEFAULT '',
			protocol_version INTEGER NOT NULL DEFAULT 0,
			capabilities TEXT NOT NULL DEFAULT '',
			created_at DATETIME,
			updated_at DATETIME,
			sync_interval_seconds INTEGER NOT NULL DEFAULT 60,
			allow_insecure_tls BOOLEAN NOT NULL DEFAULT FALSE
		)
	`)
	if errCreateNode != nil {
		t.Fatalf("failed to create node table: %v", errCreateNode)
	}

	_, errCreateTrust := conn.SQLDb.Exec(`
		create table federation_trusted_peer (
			node_id text primary key not null references node (id) on delete cascade,
			peer_node_id text not null default '',
			peer_fingerprint text not null,
			enabled boolean not null default true,
			revoked boolean not null default false,
			created_at datetime not null default current_timestamp,
			updated_at datetime not null default current_timestamp
		)
	`)
	if errCreateTrust != nil {
		t.Fatalf("failed to create federation_trusted_peer table: %v", errCreateTrust)
	}

	return conn
}

func insertRemoteNodeForFederationAuth(t *testing.T, conn *db.Connection, nodeID string) error {
	t.Helper()

	_, errInsert := conn.SQLDb.Exec(`
		insert into node (
			id, name, is_local, base_url, enabled, health_status, last_sync_status, version, protocol_version, capabilities, sync_interval_seconds, allow_insecure_tls
		) values (?, 'Remote Node', 0, 'https://node.example.com', 1, 'healthy', '', '', 0, '', 60, 0)
	`, nodeID)
	return errInsert
}

func loadTLSCertificate(t *testing.T, certPath string, keyPath string, nodeID string) tls.Certificate {
	t.Helper()

	if nodeID != "" {
		_, _, errMTLS := helpers.NewFederationMTLS(nodeID, 1, certPath, keyPath)
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
