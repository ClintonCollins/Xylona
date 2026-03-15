package actions

import (
	"context"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
)

const (
	healthCheckInterval    = 30 * time.Second
	syncInterval           = 60 * time.Second
	staleThreshold         = 3 * time.Minute
	maxRetryBackoff        = 5 * time.Minute
	federationRequestTimeout = 15 * time.Second
)

// StatusBroadcaster is called when a remote server status changes in real-time.
type StatusBroadcaster interface {
	BroadcastRemoteServerStatus(serverID string, status xylona.Status)
}

// FederationSyncEngine manages periodic synchronization with peer nodes.
type FederationSyncEngine struct {
	ctx               context.Context
	cancel            context.CancelFunc
	db                *db.Connection
	mu                sync.RWMutex
	peerStops         map[string]context.CancelFunc
	statusBroadcaster StatusBroadcaster
}

func NewFederationSyncEngine(ctx context.Context, dbInst *db.Connection) *FederationSyncEngine {
	engineCtx, engineCancel := context.WithCancel(ctx)
	engine := &FederationSyncEngine{
		ctx:       engineCtx,
		cancel:    engineCancel,
		db:        dbInst,
		peerStops: make(map[string]context.CancelFunc),
	}
	go engine.start()
	return engine
}

// SetStatusBroadcaster sets the callback for real-time remote server status updates.
func (e *FederationSyncEngine) SetStatusBroadcaster(broadcaster StatusBroadcaster) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.statusBroadcaster = broadcaster
}

func (e *FederationSyncEngine) start() {
	// Initial sync on startup with a small delay.
	time.Sleep(5 * time.Second)

	peers, errGetPeers := e.db.GetEnabledPeerNodes()
	if errGetPeers != nil {
		log.Warn().Err(errGetPeers).Msg("Failed to get enabled peer nodes on startup")
		return
	}

	for _, peer := range peers {
		e.startPeerWorker(peer.ID)
	}

	// Periodically check for new/removed peers.
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.reconcilePeerWorkers()
		}
	}
}

func (e *FederationSyncEngine) reconcilePeerWorkers() {
	peers, errGetPeers := e.db.GetEnabledPeerNodes()
	if errGetPeers != nil {
		log.Warn().Err(errGetPeers).Msg("Failed to get enabled peer nodes for reconciliation")
		return
	}

	activePeerIDs := make(map[string]bool)
	for _, peer := range peers {
		activePeerIDs[peer.ID] = true
		e.mu.RLock()
		_, exists := e.peerStops[peer.ID]
		e.mu.RUnlock()
		if !exists {
			e.startPeerWorker(peer.ID)
		}
	}

	// Stop workers for removed/disabled peers.
	e.mu.Lock()
	for peerID, cancelFunc := range e.peerStops {
		if !activePeerIDs[peerID] {
			cancelFunc()
			delete(e.peerStops, peerID)
			log.Info().Str("peer_node_id", peerID).Msg("Stopped sync worker for removed/disabled peer")
		}
	}
	e.mu.Unlock()
}

func (e *FederationSyncEngine) startPeerWorker(peerNodeID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.peerStops[peerNodeID]; exists {
		return
	}

	peerCtx, peerCancel := context.WithCancel(e.ctx)
	e.peerStops[peerNodeID] = peerCancel

	go e.peerSyncLoop(peerCtx, peerNodeID)
	log.Info().Str("peer_node_id", peerNodeID).Msg("Started sync worker for peer")
}

// SyncPeer triggers an immediate sync for a specific peer.
func (e *FederationSyncEngine) SyncPeer(peerNodeID string) {
	go e.syncPeerOnce(peerNodeID)
}

// RemovePeer stops the sync worker for a peer.
func (e *FederationSyncEngine) RemovePeer(peerNodeID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if cancelFunc, exists := e.peerStops[peerNodeID]; exists {
		cancelFunc()
		delete(e.peerStops, peerNodeID)
		log.Info().Str("peer_node_id", peerNodeID).Msg("Removed sync worker for peer")
	}
}

func (e *FederationSyncEngine) peerSyncLoop(ctx context.Context, peerNodeID string) {
	// Add jitter to avoid all peers syncing at the same time.
	jitter := time.Duration(rand.Int63n(int64(10 * time.Second)))
	time.Sleep(jitter)

	// Initial sync.
	e.syncPeerOnce(peerNodeID)

	// Start real-time status streaming in background.
	go e.streamPeerStatuses(ctx, peerNodeID)

	healthTicker := time.NewTicker(healthCheckInterval)
	syncTicker := time.NewTicker(syncInterval)
	defer healthTicker.Stop()
	defer syncTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-healthTicker.C:
			e.healthCheckPeer(peerNodeID)
		case <-syncTicker.C:
			e.syncPeerOnce(peerNodeID)
		}
	}
}

func (e *FederationSyncEngine) streamPeerStatuses(ctx context.Context, peerNodeID string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		e.runStatusStream(ctx, peerNodeID)

		// Backoff before reconnecting.
		select {
		case <-ctx.Done():
			return
		case <-time.After(5*time.Second + time.Duration(rand.Int63n(int64(3*time.Second)))):
		}
	}
}

func (e *FederationSyncEngine) runStatusStream(ctx context.Context, peerNodeID string) {
	peer, errGetPeer := e.db.GetPeerNodeByID(peerNodeID)
	if errGetPeer != nil {
		log.Warn().Err(errGetPeer).Str("peer_node_id", peerNodeID).Msg("Failed to get peer for status stream")
		return
	}

	if !peer.Enabled {
		return
	}

	// Use a long-lived HTTP client without a short timeout for streaming.
	httpClient := &http.Client{}
	client := xylonaconnect.NewFederationClient(httpClient, peer.BaseURL)

	req := connect.NewRequest(&xylona.FederationStreamServerStatusesRequest{
		SecretKey: peer.SecretKey,
	})
	req.Header().Set("X-Federation-Key", peer.SecretKey)

	stream, errStream := client.StreamServerStatuses(ctx, req)
	if errStream != nil {
		log.Debug().Err(errStream).Str("peer_node_id", peerNodeID).Str("peer_name", peer.Name).Msg("Failed to open status stream")
		return
	}
	defer stream.Close()

	log.Debug().Str("peer_node_id", peerNodeID).Str("peer_name", peer.Name).Msg("Status stream connected")

	for stream.Receive() {
		event := stream.Msg()

		// Update the cached status in the database.
		errUpdate := e.db.UpdateRemoteServerCacheStatus(peer.NodeID, event.ServerId, event.Status.String())
		if errUpdate != nil {
			log.Debug().Err(errUpdate).Str("server_id", event.ServerId).Msg("Failed to update remote server cache status")
		}

		// Broadcast to connected WebSocket clients.
		e.mu.RLock()
		broadcaster := e.statusBroadcaster
		e.mu.RUnlock()
		if broadcaster != nil {
			broadcaster.BroadcastRemoteServerStatus(event.ServerId, event.Status)
		}
	}

	if errReceive := stream.Err(); errReceive != nil {
		log.Debug().Err(errReceive).Str("peer_node_id", peerNodeID).Msg("Status stream ended")
	}
}

func (e *FederationSyncEngine) healthCheckPeer(peerNodeID string) {
	peer, errGetPeer := e.db.GetPeerNodeByID(peerNodeID)
	if errGetPeer != nil {
		log.Warn().Err(errGetPeer).Str("peer_node_id", peerNodeID).Msg("Failed to get peer node for health check")
		return
	}

	client := newFederationClient(peer.BaseURL)
	reqCtx, cancel := context.WithTimeout(e.ctx, federationRequestTimeout)
	defer cancel()

	req := connect.NewRequest(&xylona.FederationHandshakeRequest{
		SecretKey: peer.SecretKey,
	})

	resp, errHandshake := client.Handshake(reqCtx, req)
	if errHandshake != nil {
		log.Warn().Err(errHandshake).Str("peer_node_id", peerNodeID).Str("peer_name", peer.Name).Msg("Peer health check failed")
		errUpdate := e.db.UpdatePeerNodeHealth(peerNodeID, "offline", time.Now())
		if errUpdate != nil {
			log.Error().Err(errUpdate).Str("peer_node_id", peerNodeID).Msg("Failed to update peer health status")
		}
		// Mark cached servers as stale.
		errStale := e.db.MarkRemoteServerCacheStaleByPeerNodeID(peerNodeID)
		if errStale != nil {
			log.Error().Err(errStale).Str("peer_node_id", peerNodeID).Msg("Failed to mark remote servers as stale")
		}
		return
	}

	errUpdate := e.db.UpdatePeerNodeHealth(peerNodeID, "healthy", time.Now())
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("peer_node_id", peerNodeID).Msg("Failed to update peer health status")
	}

	// Update identity if it changed.
	errIdentity := e.db.UpdatePeerNodeIdentity(
		peerNodeID,
		resp.Msg.NodeId,
		resp.Msg.NodeName,
		resp.Msg.Version,
		resp.Msg.ProtocolVersion,
		resp.Msg.Capabilities,
	)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("peer_node_id", peerNodeID).Msg("Failed to update peer identity")
	}
}

func (e *FederationSyncEngine) syncPeerOnce(peerNodeID string) {
	peer, errGetPeer := e.db.GetPeerNodeByID(peerNodeID)
	if errGetPeer != nil {
		log.Warn().Err(errGetPeer).Str("peer_node_id", peerNodeID).Msg("Failed to get peer node for sync")
		return
	}

	if !peer.Enabled {
		return
	}

	syncState, errSyncState := e.db.GetOrCreatePeerSyncState(peerNodeID)
	if errSyncState != nil {
		log.Warn().Err(errSyncState).Str("peer_node_id", peerNodeID).Msg("Failed to get sync state")
	}

	// Check backoff.
	if syncState != nil && syncState.RetryCount > 0 {
		nextRetry := syncState.NextRetryAt.GetOr(time.Time{})
		if time.Now().Before(nextRetry) {
			return
		}
	}

	client := newFederationClient(peer.BaseURL)
	reqCtx, cancel := context.WithTimeout(e.ctx, federationRequestTimeout)
	defer cancel()

	req := connect.NewRequest(&xylona.FederationListServerSummariesRequest{
		Limit: 1000,
	})
	req.Header().Set("X-Federation-Key", peer.SecretKey)

	resp, errSync := client.ListServerSummaries(reqCtx, req)
	if errSync != nil {
		log.Warn().Err(errSync).Str("peer_node_id", peerNodeID).Str("peer_name", peer.Name).Msg("Failed to sync server summaries from peer")

		retryCount := int32(0)
		if syncState != nil {
			retryCount = syncState.RetryCount + 1
		}
		backoff := calculateBackoff(retryCount)
		errUpdateSync := e.db.UpdatePeerSyncStateError(peerNodeID, errSync.Error(), retryCount, time.Now().Add(backoff))
		if errUpdateSync != nil {
			log.Error().Err(errUpdateSync).Str("peer_node_id", peerNodeID).Msg("Failed to update sync state error")
		}
		errUpdateStatus := e.db.UpdatePeerNodeSyncStatus(peerNodeID, "error", time.Now())
		if errUpdateStatus != nil {
			log.Error().Err(errUpdateStatus).Str("peer_node_id", peerNodeID).Msg("Failed to update peer sync status")
		}

		// Mark servers as stale on sync failure.
		errStale := e.db.MarkRemoteServerCacheStaleByPeerNodeID(peerNodeID)
		if errStale != nil {
			log.Error().Err(errStale).Str("peer_node_id", peerNodeID).Msg("Failed to mark remote servers as stale")
		}
		return
	}

	// Upsert all server summaries from the peer.
	localSettings, errSettings := e.db.GetLocalSettings()
	if errSettings != nil {
		log.Error().Err(errSettings).Msg("Failed to get local settings for sync")
		return
	}

	for _, server := range resp.Msg.Servers {
		compositeID := peer.NodeID + "/" + server.ServerId
		newID, errID := helpers.GenerateUniqueID()
		if errID != nil {
			log.Error().Err(errID).Msg("Failed to generate ID for remote server cache")
			continue
		}

		_ = localSettings
		errUpsert := e.db.UpsertRemoteServerCache(
			newID.String(),
			peer.NodeID,
			peerNodeID,
			server.ServerId,
			server.DisplayName,
			server.Status.String(),
			server.GameName,
			server.GameId,
			server.IpAddress,
			int32(server.Port),
			int32(server.QueryPort),
			int32(server.MaxPlayers),
			int32(server.CurrentPlayers),
			server.MapName,
			server.Version,
			peer.Name,
			peer.BaseURL,
			server.UpdatedAt.AsTime(),
		)
		if errUpsert != nil {
			log.Error().Err(errUpsert).Str("composite_id", compositeID).Msg("Failed to upsert remote server cache")
		}
	}

	// Update sync state on success.
	errSyncSuccess := e.db.UpdatePeerSyncStateSuccess(peerNodeID, "")
	if errSyncSuccess != nil {
		log.Error().Err(errSyncSuccess).Str("peer_node_id", peerNodeID).Msg("Failed to update sync state success")
	}
	errSyncStatus := e.db.UpdatePeerNodeSyncStatus(peerNodeID, "success", time.Now())
	if errSyncStatus != nil {
		log.Error().Err(errSyncStatus).Str("peer_node_id", peerNodeID).Msg("Failed to update peer sync status")
	}

	log.Debug().Str("peer_node_id", peerNodeID).Str("peer_name", peer.Name).
		Int("server_count", len(resp.Msg.Servers)).Msg("Successfully synced server summaries from peer")
}

func newFederationClient(baseURL string) xylonaconnect.FederationClient {
	httpClient := &http.Client{
		Timeout: federationRequestTimeout,
	}
	return xylonaconnect.NewFederationClient(httpClient, baseURL)
}

func calculateBackoff(retryCount int32) time.Duration {
	base := time.Duration(math.Pow(2, float64(retryCount))) * time.Second
	if base > maxRetryBackoff {
		base = maxRetryBackoff
	}
	// Add jitter.
	jitter := time.Duration(rand.Int63n(int64(5 * time.Second)))
	return base + jitter
}
