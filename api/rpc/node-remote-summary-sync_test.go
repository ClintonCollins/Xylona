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

	"github.com/ClintonCollins/Xylona/db/dbtest"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type federationSummaryTestHandler struct {
	xylonaconnect.UnimplementedFederationHandler

	mu                sync.RWMutex
	status            xylona.Status
	updatedAt         time.Time
	lastActingUserID  string
	lastOriginNodeID  string
	lastActingIsSuper string
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
	actingUserID, originNodeID := helpers.GetFederatedActingIdentity(request.Header())
	actingIsSuper := request.Header().Get(helpers.FederationActingSuperHeader)

	h.mu.Lock()
	status := h.status
	updatedAt := h.updatedAt
	h.lastActingUserID = actingUserID
	h.lastOriginNodeID = originNodeID
	h.lastActingIsSuper = actingIsSuper
	h.mu.Unlock()

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

func (h *federationSummaryTestHandler) LastActingHeaders() (string, string, string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastActingUserID, h.lastOriginNodeID, h.lastActingIsSuper
}

func TestListRemoteNodeSummariesFetchesLiveStatusAndSyncsRemoteCache(t *testing.T) {
	conn := dbtest.NewMigratedConnection(t, "node-remote-summary-sync.sqlite")

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

	_, errInsertNode := conn.SQLDb.Exec(
		`INSERT INTO node (id, name, is_local, host, port, base_url, enabled) VALUES (?, ?, 0, '', 0, ?, 1)`,
		node.ID, node.Name, node.BaseURL,
	)
	if errInsertNode != nil {
		t.Fatalf("failed to insert node row: %v", errInsertNode)
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

	firstSummaries, firstUsedStale, errFirst := xylonaService.listRemoteNodeSummaries(context.Background(), node, nil)
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

	secondSummaries, secondUsedStale, errSecond := xylonaService.listRemoteNodeSummaries(context.Background(), node, nil)
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
	const cacheStatusQuery = `SELECT status FROM remote_server_cache WHERE source_node_id = ? AND remote_server_id = ?`
	errCacheStatus := conn.SQLDb.QueryRow(cacheStatusQuery, node.ID, "server-1").Scan(&cachedStatus)
	if errCacheStatus != nil {
		t.Fatalf("failed to read cached status: %v", errCacheStatus)
	}
	if cachedStatus != xylona.Status_ONLINE.String() {
		t.Fatalf("cached status = %q, want %q", cachedStatus, xylona.Status_ONLINE.String())
	}

	const cacheCountQuery = `SELECT COUNT(*) FROM remote_server_cache WHERE source_node_id = ? AND remote_server_id = ?`
	errCacheCount := conn.SQLDb.QueryRow(cacheCountQuery, node.ID, "server-1").Scan(&cachedCount)
	if errCacheCount != nil {
		t.Fatalf("failed to count cached rows: %v", errCacheCount)
	}
	if cachedCount != 1 {
		t.Fatalf("cached row count = %d, want 1", cachedCount)
	}

	superUser := &models.User{
		ID:        "user-admin",
		SuperUser: true,
	}
	thirdSummaries, thirdUsedStale, errThird := xylonaService.listRemoteNodeSummaries(context.Background(), node, superUser)
	if errThird != nil {
		t.Fatalf("listRemoteNodeSummaries() super-user call error = %v", errThird)
	}
	if thirdUsedStale {
		t.Fatalf("listRemoteNodeSummaries() super-user call used stale data unexpectedly")
	}
	if len(thirdSummaries) != 1 {
		t.Fatalf("len(thirdSummaries) = %d, want 1", len(thirdSummaries))
	}
	gotActingUserID, gotOriginNodeID, gotActingIsSuper := testHandler.LastActingHeaders()
	if gotActingUserID != "" {
		t.Fatalf("acting user header = %q, want empty for super-user summary fetch", gotActingUserID)
	}
	if gotOriginNodeID != "" {
		t.Fatalf("origin node header = %q, want empty for super-user summary fetch", gotOriginNodeID)
	}
	if gotActingIsSuper != "" {
		t.Fatalf("acting super header = %q, want empty for super-user summary fetch", gotActingIsSuper)
	}
}
