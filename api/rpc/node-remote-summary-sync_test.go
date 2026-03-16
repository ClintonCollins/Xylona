package rpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/null"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/db"
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

func TestListRemoteNodeSummariesFetchesLiveStatusAndSyncsRemoteCache(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "node-remote-summary-sync.sqlite")
	conn := db.NewConnection(context.Background(), dbPath)
	t.Cleanup(func() {
		if errClose := conn.SQLDb.Close(); errClose != nil {
			t.Errorf("failed to close db: %v", errClose)
		}
	})
	createRemoteSummarySyncCacheTable(t, conn)

	testHandler := &federationSummaryTestHandler{}
	testHandler.SetStatus(xylona.Status_OFFLINE)

	federationPath, federationHTTPHandler := xylonaconnect.NewFederationHandler(testHandler)
	mux := http.NewServeMux()
	mux.Handle(federationPath, federationHTTPHandler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	node := &models.Node{
		ID:               "node-1",
		Name:             "Peer One",
		BaseURL:          server.URL,
		SecretKey:        null.From("test-secret-key"),
		AllowInsecureTLS: false,
	}

	xylonaService := XylonaService{
		db:        conn,
		listCache: newRemoteServerListCache(30 * time.Second),
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
