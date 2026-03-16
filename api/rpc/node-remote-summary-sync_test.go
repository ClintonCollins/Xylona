package rpc

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type federationSummaryTestHandler struct {
	xylonaconnect.UnimplementedFederationHandler

	mu        sync.RWMutex
	status    xylona.Status
	updatedAt time.Time
}

func (h *federationSummaryTestHandler) SetStatus(status xylona.Status) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status = status
	h.updatedAt = time.Now().UTC()
}

func (h *federationSummaryTestHandler) ListServerSummaries(
	ctx context.Context,
	request *connect.Request[xylona.FederationListServerSummariesRequest],
) (*connect.Response[xylona.FederationListServerSummariesResponse], error) {
	h.mu.RLock()
	status := h.status
	updatedAt := h.updatedAt
	h.mu.RUnlock()

	response := &xylona.FederationListServerSummariesResponse{
		Servers: []*xylona.FederationServerSummary{
			{
				ServerId:    "server-1",
				DisplayName: "Remote Alpha",
				Status:      status,
				GameName:    "Minecraft",
				GameId:      "minecraft",
				IpAddress:   "10.0.0.10",
				Port:        25565,
				QueryPort:   25565,
				MaxPlayers:  20,
				MapName:     "world",
				Version:     "1.21.1",
				UpdatedAt:   timestamppb.New(updatedAt),
			},
		},
	}
	return connect.NewResponse(response), nil
}

func createRemoteSummarySyncCacheTable(t *testing.T, conn *db.Connection) {
	t.Helper()

	_, errCreate := conn.SQLDb.Exec(`
		CREATE TABLE remote_server_cache (
			id TEXT PRIMARY KEY NOT NULL,
			source_node_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			remote_server_id TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'UNKNOWN',
			game_name TEXT NOT NULL DEFAULT '',
			game_id TEXT NOT NULL DEFAULT '',
			ip_address TEXT NOT NULL DEFAULT '',
			port INTEGER NOT NULL DEFAULT 0,
			query_port INTEGER NOT NULL DEFAULT 0,
			max_players INTEGER NOT NULL DEFAULT 0,
			current_players INTEGER NOT NULL DEFAULT 0,
			map_name TEXT NOT NULL DEFAULT '',
			version TEXT NOT NULL DEFAULT '',
			node_name TEXT NOT NULL DEFAULT '',
			node_host TEXT NOT NULL DEFAULT '',
			last_remote_update DATETIME,
			last_synced_at DATETIME,
			is_stale BOOLEAN NOT NULL DEFAULT FALSE,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if errCreate != nil {
		t.Fatalf("failed to create remote_server_cache table: %v", errCreate)
	}

	_, errCreateIndex := conn.SQLDb.Exec(`
		CREATE UNIQUE INDEX remote_server_cache_source_server
		ON remote_server_cache (source_node_id, remote_server_id)
	`)
	if errCreateIndex != nil {
		t.Fatalf("failed to create unique index: %v", errCreateIndex)
	}
}

func createRemoteSummarySyncTrustTable(t *testing.T, conn *db.Connection) {
	t.Helper()

	_, errCreate := conn.SQLDb.Exec(`
		CREATE TABLE federation_trusted_peer (
			node_id TEXT PRIMARY KEY NOT NULL,
			peer_node_id TEXT NOT NULL DEFAULT '',
			peer_fingerprint TEXT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			revoked BOOLEAN NOT NULL DEFAULT FALSE,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if errCreate != nil {
		t.Fatalf("failed to create federation_trusted_peer table: %v", errCreate)
	}
}

func TestListRemoteNodeSummariesFetchesLiveStatusAndSyncsRemoteCache(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "node-remote-summary-sync.sqlite")
	conn := db.NewConnection(context.Background(), dbPath)
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close db: %v", errClose)
		}
	})
	createRemoteSummarySyncCacheTable(t, conn)
	createRemoteSummarySyncTrustTable(t, conn)

	testHandler := &federationSummaryTestHandler{}
	testHandler.SetStatus(xylona.Status_OFFLINE)

	serverMTLS, serverFingerprint, errServerMTLS := helpers.NewFederationMTLS(
		"remote-peer-node",
		1,
		filepath.Join(t.TempDir(), "server.crt"),
		filepath.Join(t.TempDir(), "server.key"),
	)
	if errServerMTLS != nil {
		t.Fatalf("NewFederationMTLS() server error = %v", errServerMTLS)
	}

	clientMTLSPath := filepath.Join(t.TempDir(), "client.crt")
	clientKeyPath := filepath.Join(t.TempDir(), "client.key")
	_, _, errClientMTLS := helpers.NewFederationMTLS("local-node", 1, clientMTLSPath, clientKeyPath)
	if errClientMTLS != nil {
		t.Fatalf("NewFederationMTLS() client error = %v", errClientMTLS)
	}

	serverCertificate, errLoadServerCert := tls.LoadX509KeyPair(serverMTLS.CertPath(), serverMTLS.KeyPath())
	if errLoadServerCert != nil {
		t.Fatalf("tls.LoadX509KeyPair() server cert error = %v", errLoadServerCert)
	}

	federationPath, federationHTTPHandler := xylonaconnect.NewFederationHandler(testHandler)
	mux := http.NewServeMux()
	mux.Handle(federationPath, federationHTTPHandler)
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverCertificate},
		ClientAuth:   tls.RequireAnyClientCert,
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	parsedServerURL, errParseServerURL := url.Parse(server.URL)
	if errParseServerURL != nil {
		t.Fatalf("url.Parse() error = %v", errParseServerURL)
	}
	serverPort, errParsePort := strconv.Atoi(parsedServerURL.Port())
	if errParsePort != nil {
		t.Fatalf("failed to parse server port: %v", errParsePort)
	}

	clientMTLS, _, errClientConfig := helpers.NewFederationMTLS(
		"local-node",
		serverPort,
		clientMTLSPath,
		clientKeyPath,
	)
	if errClientConfig != nil {
		t.Fatalf("NewFederationMTLS() client runtime error = %v", errClientConfig)
	}

	node := &models.Node{
		ID:               "node-1",
		Name:             "Peer One",
		BaseURL:          server.URL,
		AllowInsecureTLS: false,
	}

	_, errInsertTrust := conn.SQLDb.Exec(`
		INSERT INTO federation_trusted_peer (node_id, peer_node_id, peer_fingerprint, enabled, revoked)
		VALUES (?, ?, ?, 1, 0)
	`, node.ID, "remote-peer-node", serverFingerprint)
	if errInsertTrust != nil {
		t.Fatalf("failed to insert trusted peer row: %v", errInsertTrust)
	}

	xylonaService := XylonaService{
		db:             conn,
		federationMTLS: clientMTLS,
		listCache:      newRemoteServerListCache(30 * time.Second),
	}

	firstSummaries, firstUsedStale, errFirst := xylonaService.listRemoteNodeSummaries(context.Background(), node)
	if errFirst != nil {
		t.Fatalf("listRemoteNodeSummaries() first call error = %v", errFirst)
	}
	if firstUsedStale {
		t.Fatalf("listRemoteNodeSummaries() first call used stale data unexpectedly")
	}
	if len(firstSummaries) != 1 {
		t.Fatalf("len(firstSummaries) = %d, want 1", len(firstSummaries))
	}
	if gotStatus := firstSummaries[0].Status; gotStatus != xylona.Status_OFFLINE {
		t.Fatalf("first summary status = %v, want %v", gotStatus, xylona.Status_OFFLINE)
	}

	testHandler.SetStatus(xylona.Status_ONLINE)

	secondSummaries, secondUsedStale, errSecond := xylonaService.listRemoteNodeSummaries(context.Background(), node)
	if errSecond != nil {
		t.Fatalf("listRemoteNodeSummaries() second call error = %v", errSecond)
	}
	if secondUsedStale {
		t.Fatalf("listRemoteNodeSummaries() second call used stale data unexpectedly")
	}
	if len(secondSummaries) != 1 {
		t.Fatalf("len(secondSummaries) = %d, want 1", len(secondSummaries))
	}
	if gotStatus := secondSummaries[0].Status; gotStatus != xylona.Status_ONLINE {
		t.Fatalf("second summary status = %v, want %v (live fetch should bypass fresh cache)", gotStatus, xylona.Status_ONLINE)
	}

	var (
		cachedStatus string
		cachedCount  int
	)
	errCacheStatus := conn.SQLDb.QueryRow(`
		SELECT status
		FROM remote_server_cache
		WHERE source_node_id = ? AND remote_server_id = ?
	`, node.ID, "server-1").Scan(&cachedStatus)
	if errCacheStatus != nil {
		t.Fatalf("failed to read cached status: %v", errCacheStatus)
	}
	if cachedStatus != xylona.Status_ONLINE.String() {
		t.Fatalf("cached status = %q, want %q", cachedStatus, xylona.Status_ONLINE.String())
	}

	errCacheCount := conn.SQLDb.QueryRow(`
		SELECT COUNT(*)
		FROM remote_server_cache
		WHERE source_node_id = ? AND remote_server_id = ?
	`, node.ID, "server-1").Scan(&cachedCount)
	if errCacheCount != nil {
		t.Fatalf("failed to count cached rows: %v", errCacheCount)
	}
	if cachedCount != 1 {
		t.Fatalf("cached row count = %d, want 1", cachedCount)
	}
}
