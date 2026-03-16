package actions

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// newMTLSFileProxyTestSetup creates an mTLS-configured Instance with a TLS test server.
// The handler argument is installed on the remote TLS server.
// Returns the Instance, the remote node model, and a cleanup function.
func newMTLSFileProxyTestSetup(t *testing.T, handler http.Handler) (*Instance, *models.Node) {
	t.Helper()

	tmpDir := t.TempDir()

	// Create server mTLS identity (using a placeholder port; the real port comes from the test server).
	serverCertPath := filepath.Join(tmpDir, "server.crt")
	serverKeyPath := filepath.Join(tmpDir, "server.key")
	serverMTLS, serverFP, errServer := helpers.NewFederationMTLS("server-node", 1, serverCertPath, serverKeyPath)
	if errServer != nil {
		t.Fatalf("NewFederationMTLS(server) error = %v", errServer)
	}
	_ = serverFP

	// Start a TLS test server with the server certificate.
	serverTLSCert, errLoadServer := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if errLoadServer != nil {
		t.Fatalf("LoadX509KeyPair(server) error = %v", errLoadServer)
	}

	testServer := httptest.NewUnstartedServer(handler)
	testServer.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{serverTLSCert},
		ClientAuth:   tls.RequireAnyClientCert,
	}
	testServer.StartTLS()
	t.Cleanup(testServer.Close)

	// Extract the actual port from the test server URL to create the client mTLS with the correct federation port.
	serverURL := testServer.URL // e.g., "https://127.0.0.1:PORT"
	colonIdx := strings.LastIndex(serverURL, ":")
	serverPort := 0
	if colonIdx >= 0 {
		fmt.Sscanf(serverURL[colonIdx+1:], "%d", &serverPort)
	}
	if serverPort == 0 {
		t.Fatalf("failed to extract port from test server URL: %s", serverURL)
	}

	// Create client mTLS identity using the actual test server port.
	clientCertPath := filepath.Join(tmpDir, "client.crt")
	clientKeyPath := filepath.Join(tmpDir, "client.key")
	clientMTLS, clientFP, errClient := helpers.NewFederationMTLS("client-node", serverPort, clientCertPath, clientKeyPath)
	if errClient != nil {
		t.Fatalf("NewFederationMTLS(client) error = %v", errClient)
	}
	_ = clientFP

	// Set up a test DB with the trust entry.
	dbPath := filepath.Join(tmpDir, "test.sqlite")
	conn := db.NewConnection(context.Background(), dbPath)
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close db: %v", errClose)
		}
	})

	_, _ = conn.SQLDb.Exec(`create table node (id text primary key not null)`)
	_, _ = conn.SQLDb.Exec(`insert into node (id) values ('remote-node-1')`)
	_, _ = conn.SQLDb.Exec(`
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

	// Get the server fingerprint from its certificate.
	serverFPFromCert := helpers.CertificateFingerprint(testServer.Certificate())

	_, errInsertTrust := conn.SQLDb.Exec(`
		insert into federation_trusted_peer (node_id, peer_node_id, peer_fingerprint, enabled, revoked)
		values ('remote-node-1', 'server-node', ?, true, false)
	`, serverFPFromCert)
	if errInsertTrust != nil {
		t.Fatalf("failed to insert trust: %v", errInsertTrust)
	}

	_ = serverMTLS
	inst := &Instance{
		db:             conn,
		federationMTLS: clientMTLS,
	}

	remoteNode := &models.Node{
		ID:      "remote-node-1",
		BaseURL: testServer.URL,
	}

	return inst, remoteNode
}

func TestResolveFileRequestTargetWithLookups(t *testing.T) {
	errDBUnavailable := errors.New("db unavailable")

	tests := []struct {
		name              string
		localLookup       func(string) (*models.GameServer, error)
		remoteCacheLookup func(string) (*models.RemoteServerCache, error)
		remoteNodeLookup  func(string) (*models.Node, error)
		wantLocal         bool
		wantRemoteID      string
		wantErr           error
	}{
		{
			name: "uses local game server when present",
			localLookup: func(string) (*models.GameServer, error) {
				return &models.GameServer{ID: "local-server"}, nil
			},
			remoteCacheLookup: func(string) (*models.RemoteServerCache, error) {
				t.Fatalf("remote cache lookup should not be called when local server exists")
				return nil, nil
			},
			remoteNodeLookup: func(string) (*models.Node, error) {
				t.Fatalf("remote node lookup should not be called when local server exists")
				return nil, nil
			},
			wantLocal: true,
		},
		{
			name: "falls back to remote server when local lookup is not found",
			localLookup: func(string) (*models.GameServer, error) {
				return nil, sql.ErrNoRows
			},
			remoteCacheLookup: func(string) (*models.RemoteServerCache, error) {
				return &models.RemoteServerCache{RemoteServerID: "remote-server", NodeID: "node-1"}, nil
			},
			remoteNodeLookup: func(string) (*models.Node, error) {
				return &models.Node{ID: "node-1", Enabled: true}, nil
			},
			wantRemoteID: "remote-server",
		},
		{
			name: "returns not found when remote node is disabled",
			localLookup: func(string) (*models.GameServer, error) {
				return nil, sql.ErrNoRows
			},
			remoteCacheLookup: func(string) (*models.RemoteServerCache, error) {
				return &models.RemoteServerCache{RemoteServerID: "remote-server", NodeID: "node-1"}, nil
			},
			remoteNodeLookup: func(string) (*models.Node, error) {
				return &models.Node{ID: "node-1", Enabled: false}, nil
			},
			wantErr: sql.ErrNoRows,
		},
		{
			name: "returns local lookup error",
			localLookup: func(string) (*models.GameServer, error) {
				return nil, errDBUnavailable
			},
			remoteCacheLookup: func(string) (*models.RemoteServerCache, error) {
				t.Fatalf("remote cache lookup should not be called when local lookup fails")
				return nil, nil
			},
			remoteNodeLookup: func(string) (*models.Node, error) {
				t.Fatalf("remote node lookup should not be called when local lookup fails")
				return nil, nil
			},
			wantErr: errDBUnavailable,
		},
		{
			name: "returns not found when remote cache is missing",
			localLookup: func(string) (*models.GameServer, error) {
				return nil, sql.ErrNoRows
			},
			remoteCacheLookup: func(string) (*models.RemoteServerCache, error) {
				return nil, sql.ErrNoRows
			},
			remoteNodeLookup: func(string) (*models.Node, error) {
				t.Fatalf("remote node lookup should not be called when remote cache is missing")
				return nil, nil
			},
			wantErr: sql.ErrNoRows,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			target, errResolve := resolveFileRequestTargetWithLookups("server-id", tt.localLookup, tt.remoteCacheLookup, tt.remoteNodeLookup)
			if tt.wantErr != nil {
				if errResolve == nil {
					t.Fatalf("resolveFileRequestTargetWithLookups() error = nil, want non-nil")
				}
				if !errors.Is(errResolve, tt.wantErr) {
					t.Fatalf("resolveFileRequestTargetWithLookups() error = %v, want %v", errResolve, tt.wantErr)
				}
				return
			}

			if errResolve != nil {
				t.Fatalf("resolveFileRequestTargetWithLookups() error = %v, want nil", errResolve)
			}
			if target.isLocal() != tt.wantLocal {
				t.Errorf("target.isLocal() = %t, want %t", target.isLocal(), tt.wantLocal)
			}
			if target.remoteServerID != tt.wantRemoteID {
				t.Errorf("target.remoteServerID = %q, want %q", target.remoteServerID, tt.wantRemoteID)
			}
		})
	}
}

func TestProxyRemoteFileGet(t *testing.T) {
	inst, remoteNode := newMTLSFileProxyTestSetup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != fileGetPath {
			t.Fatalf("request path = %q, want %q", r.URL.Path, fileGetPath)
		}

		bodyBytes, errRead := io.ReadAll(r.Body)
		if errRead != nil {
			t.Fatalf("failed to read request body: %v", errRead)
		}

		request := xylona.DownloadFileRequest{}
		errDecode := protojson.Unmarshal(bodyBytes, &request)
		if errDecode != nil {
			t.Fatalf("failed to decode request body: %v", errDecode)
		}

		if request.GameServerId != "remote-server-id" {
			t.Fatalf("request.GameServerId = %q, want %q", request.GameServerId, "remote-server-id")
		}
		if request.Path != "server.properties" {
			t.Fatalf("request.Path = %q, want %q", request.Path, "server.properties")
		}

		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("remote-content"))
	}))

	target := fileRequestTarget{
		remoteServerID: "remote-server-id",
		remoteNode:     remoteNode,
	}

	responseRecorder := httptest.NewRecorder()
	errProxy := inst.proxyRemoteFileGet(context.Background(), target, "server.properties", responseRecorder)
	if errProxy != nil {
		t.Fatalf("proxyRemoteFileGet() error = %v", errProxy)
	}

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("response status = %d, want %d", responseRecorder.Code, http.StatusOK)
	}
	if responseRecorder.Body.String() != "remote-content" {
		t.Errorf("response body = %q, want %q", responseRecorder.Body.String(), "remote-content")
	}
}

func TestProxyRemoteFileDownload(t *testing.T) {
	inst, remoteNode := newMTLSFileProxyTestSetup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != fileDownloadPath {
			t.Fatalf("request path = %q, want %q", r.URL.Path, fileDownloadPath)
		}

		errParse := r.ParseMultipartForm(1024)
		if errParse != nil {
			t.Fatalf("failed to parse multipart form: %v", errParse)
		}

		if gotServerID := r.FormValue("gameServerId"); gotServerID != "remote-server-id" {
			t.Fatalf("gameServerId = %q, want %q", gotServerID, "remote-server-id")
		}
		if gotPath := r.FormValue("path"); gotPath != "logs/latest.log" {
			t.Fatalf("path = %q, want %q", gotPath, "logs/latest.log")
		}

		_, _ = w.Write([]byte("download-content"))
	}))

	target := fileRequestTarget{
		remoteServerID: "remote-server-id",
		remoteNode:     remoteNode,
	}

	responseRecorder := httptest.NewRecorder()
	errProxy := inst.proxyRemoteFileDownload(context.Background(), target, "logs/latest.log", responseRecorder)
	if errProxy != nil {
		t.Fatalf("proxyRemoteFileDownload() error = %v", errProxy)
	}

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("response status = %d, want %d", responseRecorder.Code, http.StatusOK)
	}
	if responseRecorder.Body.String() != "download-content" {
		t.Errorf("response body = %q, want %q", responseRecorder.Body.String(), "download-content")
	}
}

func TestProxyRemoteFileUpload(t *testing.T) {
	inst, remoteNode := newMTLSFileProxyTestSetup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != fileUploadPath {
			t.Fatalf("request path = %q, want %q", r.URL.Path, fileUploadPath)
		}

		reader, errReader := r.MultipartReader()
		if errReader != nil {
			t.Fatalf("failed to create multipart reader: %v", errReader)
		}

		foundFile := false
		for {
			part, errPart := reader.NextPart()
			if errors.Is(errPart, io.EOF) {
				break
			}
			if errPart != nil {
				t.Fatalf("failed to get next multipart part: %v", errPart)
			}

			switch part.FormName() {
			case "gameServerId":
				bodyBytes, errRead := io.ReadAll(part)
				if errRead != nil {
					t.Fatalf("failed to read gameServerId part: %v", errRead)
				}
				if string(bodyBytes) != "remote-server-id" {
					t.Fatalf("gameServerId part = %q, want %q", string(bodyBytes), "remote-server-id")
				}
			case "path":
				bodyBytes, errRead := io.ReadAll(part)
				if errRead != nil {
					t.Fatalf("failed to read path part: %v", errRead)
				}
				if string(bodyBytes) != "uploads" {
					t.Fatalf("path part = %q, want %q", string(bodyBytes), "uploads")
				}
			case "file":
				fileBytes, errRead := io.ReadAll(part)
				if errRead != nil {
					t.Fatalf("failed to read file part: %v", errRead)
				}
				if string(fileBytes) != "file-content" {
					t.Fatalf("file content = %q, want %q", string(fileBytes), "file-content")
				}
				if part.FileName() != "test.txt" {
					t.Fatalf("file name = %q, want %q", part.FileName(), "test.txt")
				}
				foundFile = true
			}
		}

		if !foundFile {
			t.Fatalf("expected multipart file part")
		}

		_, _ = w.Write([]byte("uploaded"))
	}))

	target := fileRequestTarget{
		remoteServerID: "remote-server-id",
		remoteNode:     remoteNode,
	}

	responseRecorder := httptest.NewRecorder()
	errProxy := inst.proxyRemoteFileUpload(
		context.Background(),
		target,
		"uploads",
		"test.txt",
		strings.NewReader("file-content"),
		responseRecorder,
	)
	if errProxy != nil {
		t.Fatalf("proxyRemoteFileUpload() error = %v", errProxy)
	}

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("response status = %d, want %d", responseRecorder.Code, http.StatusOK)
	}
	if responseRecorder.Body.String() != "uploaded" {
		t.Errorf("response body = %q, want %q", responseRecorder.Body.String(), "uploaded")
	}
}

func TestProxyRemoteFileGetForwardsErrorStatusAndHeaders(t *testing.T) {
	inst, remoteNode := newMTLSFileProxyTestSetup(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Remote-Reason", "upstream-failure")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))

	target := fileRequestTarget{
		remoteServerID: "remote-server-id",
		remoteNode:     remoteNode,
	}

	responseRecorder := httptest.NewRecorder()
	errProxy := inst.proxyRemoteFileGet(context.Background(), target, "server.properties", responseRecorder)
	if errProxy != nil {
		t.Fatalf("proxyRemoteFileGet() error = %v", errProxy)
	}

	if responseRecorder.Code != http.StatusBadGateway {
		t.Fatalf("response status = %d, want %d", responseRecorder.Code, http.StatusBadGateway)
	}
	if responseRecorder.Body.String() != "bad gateway" {
		t.Fatalf("response body = %q, want %q", responseRecorder.Body.String(), "bad gateway")
	}
	if gotHeader := responseRecorder.Header().Get("X-Remote-Reason"); gotHeader != "upstream-failure" {
		t.Fatalf("X-Remote-Reason = %q, want %q", gotHeader, "upstream-failure")
	}
	if gotHeader := responseRecorder.Header().Get("Connection"); gotHeader != "" {
		t.Fatalf("Connection header = %q, want empty", gotHeader)
	}
	if gotHeader := responseRecorder.Header().Get("Transfer-Encoding"); gotHeader != "" {
		t.Fatalf("Transfer-Encoding header = %q, want empty", gotHeader)
	}
}

func TestProxyRemoteFileGetRespectsCanceledContext(t *testing.T) {
	inst, remoteNode := newMTLSFileProxyTestSetup(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	target := fileRequestTarget{
		remoteServerID: "remote-server-id",
		remoteNode:     remoteNode,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	responseRecorder := httptest.NewRecorder()
	errProxy := inst.proxyRemoteFileGet(ctx, target, "server.properties", responseRecorder)
	if errProxy == nil {
		t.Fatalf("proxyRemoteFileGet() error = nil, want context cancellation error")
	}
	if !errors.Is(errProxy, context.Canceled) && !strings.Contains(strings.ToLower(errProxy.Error()), "context canceled") {
		t.Fatalf("proxyRemoteFileGet() error = %v, want context canceled", errProxy)
	}
}

func TestWriteGameServerLookupError(t *testing.T) {
	tests := []struct {
		name       string
		lookupErr  error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "not found maps to 404",
			lookupErr:  sql.ErrNoRows,
			wantStatus: http.StatusNotFound,
			wantBody:   "Game server not found",
		},
		{
			name:       "other error maps to 500",
			lookupErr:  errors.New("db unavailable"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Failed to get game server",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			responseRecorder := httptest.NewRecorder()

			writeGameServerLookupError(responseRecorder, tt.lookupErr)

			if responseRecorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", responseRecorder.Code, tt.wantStatus)
			}
			if !strings.Contains(responseRecorder.Body.String(), tt.wantBody) {
				t.Fatalf("body = %q, want to contain %q", responseRecorder.Body.String(), tt.wantBody)
			}
		})
	}
}
