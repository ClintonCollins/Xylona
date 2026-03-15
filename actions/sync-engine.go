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
	healthCheckInterval      = 30 * time.Second
	peerReconcileInterval    = 60 * time.Second
	defaultNodeSyncInterval  = 60 * time.Second
	minNodeSyncInterval      = 15 * time.Second
	maxNodeSyncInterval      = 10 * time.Minute
	staleThreshold           = 3 * time.Minute
	maxRetryBackoff          = 5 * time.Minute
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

	peers, errGetPeers := e.db.GetEnabledRemoteNodes()
	if errGetPeers != nil {
		log.Warn().Err(errGetPeers).Msg("Failed to get enabled remote nodes on startup")
		return
	}

	for _, peer := range peers {
		e.startPeerWorker(peer.ID)
	}

	// Periodically check for new/removed peers.
	ticker := time.NewTicker(peerReconcileInterval)
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
	peers, errGetPeers := e.db.GetEnabledRemoteNodes()
	if errGetPeers != nil {
		log.Warn().Err(errGetPeers).Msg("Failed to get enabled remote nodes for reconciliation")
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
			log.Info().Str("node_id", peerID).Msg("Stopped sync worker for removed/disabled node")
		}
	}
	e.mu.Unlock()
}

func (e *FederationSyncEngine) startPeerWorker(nodeID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.peerStops[nodeID]; exists {
		return
	}

	peerCtx, peerCancel := context.WithCancel(e.ctx)
	e.peerStops[nodeID] = peerCancel

	go e.peerSyncLoop(peerCtx, nodeID)
	log.Info().Str("node_id", nodeID).Msg("Started sync worker for node")
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
		log.Info().Str("node_id", peerNodeID).Msg("Removed sync worker for node")
	}
}

func (e *FederationSyncEngine) peerSyncLoop(ctx context.Context, nodeID string) {
	// Add jitter to avoid all peers syncing at the same time.
	jitter := time.Duration(rand.Int63n(int64(10 * time.Second)))
	time.Sleep(jitter)

	// Initial sync.
	e.syncPeerOnce(nodeID)

	// Start real-time status streaming in background.
	go e.streamPeerStatuses(ctx, nodeID)

	healthTicker := time.NewTicker(healthCheckInterval)
	syncTimer := time.NewTimer(e.getNodeSyncInterval(nodeID))
	defer healthTicker.Stop()
	defer syncTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-healthTicker.C:
			e.healthCheckPeer(nodeID)
		case <-syncTimer.C:
			e.syncPeerOnce(nodeID)
			syncTimer.Reset(e.getNodeSyncInterval(nodeID))
		}
	}
}

func (e *FederationSyncEngine) streamPeerStatuses(ctx context.Context, nodeID string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		e.runStatusStream(ctx, nodeID)

		// Backoff before reconnecting.
		select {
		case <-ctx.Done():
			return
		case <-time.After(5*time.Second + time.Duration(rand.Int63n(int64(3*time.Second)))):
		}
	}
}

func (e *FederationSyncEngine) runStatusStream(ctx context.Context, nodeID string) {
	node, errGetNode := e.db.GetRemoteNodeByID(nodeID)
	if errGetNode != nil {
		log.Warn().Err(errGetNode).Str("node_id", nodeID).Msg("Failed to get node for status stream")
		return
	}

	if !node.Enabled {
		return
	}

	secretKey := node.SecretKey.GetOr("")

	// Use a long-lived HTTP client without a short timeout for streaming.
	httpClient := &http.Client{}
	client := xylonaconnect.NewFederationClient(httpClient, node.BaseURL)

	req := connect.NewRequest(&xylona.FederationStreamServerStatusesRequest{
		SecretKey: secretKey,
	})
	req.Header().Set("X-Federation-Key", secretKey)

	stream, errStream := client.StreamServerStatuses(ctx, req)
	if errStream != nil {
		log.Debug().Err(errStream).Str("node_id", nodeID).Str("node_name", node.Name).Msg("Failed to open status stream")
		return
	}
	defer stream.Close()

	log.Debug().Str("node_id", nodeID).Str("node_name", node.Name).Msg("Status stream connected")

	for stream.Receive() {
		event := stream.Msg()

		// Update the cached status in the database.
		errUpdate := e.db.UpdateRemoteServerCacheStatus(node.ID, event.ServerId, event.Status.String())
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
		log.Debug().Err(errReceive).Str("node_id", nodeID).Msg("Status stream ended")
	}
}

func (e *FederationSyncEngine) healthCheckPeer(nodeID string) {
	node, errGetNode := e.db.GetRemoteNodeByID(nodeID)
	if errGetNode != nil {
		log.Warn().Err(errGetNode).Str("node_id", nodeID).Msg("Failed to get node for health check")
		return
	}

	secretKey := node.SecretKey.GetOr("")
	client := newFederationClient(node.BaseURL)
	reqCtx, cancel := context.WithTimeout(e.ctx, federationRequestTimeout)
	defer cancel()

	req := connect.NewRequest(&xylona.FederationHandshakeRequest{
		SecretKey: secretKey,
	})

	resp, errHandshake := client.Handshake(reqCtx, req)
	if errHandshake != nil {
		log.Warn().Err(errHandshake).Str("node_id", nodeID).Str("node_name", node.Name).Msg("Node health check failed")
		errUpdate := e.db.UpdateNodeHealth(nodeID, "offline", time.Now())
		if errUpdate != nil {
			log.Error().Err(errUpdate).Str("node_id", nodeID).Msg("Failed to update node health status")
		}
		// Mark cached servers as stale.
		errStale := e.db.MarkRemoteServerCacheStaleByNodeID(nodeID)
		if errStale != nil {
			log.Error().Err(errStale).Str("node_id", nodeID).Msg("Failed to mark remote servers as stale")
		}
		return
	}

	errUpdate := e.db.UpdateNodeHealth(nodeID, "healthy", time.Now())
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("node_id", nodeID).Msg("Failed to update node health status")
	}

	// Update identity if it changed.
	errIdentity := e.db.UpdateNodeIdentity(
		nodeID,
		resp.Msg.Version,
		resp.Msg.ProtocolVersion,
		resp.Msg.Capabilities,
	)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("node_id", nodeID).Msg("Failed to update node identity")
	}
}

func (e *FederationSyncEngine) syncPeerOnce(nodeID string) {
	node, errGetNode := e.db.GetRemoteNodeByID(nodeID)
	if errGetNode != nil {
		log.Warn().Err(errGetNode).Str("node_id", nodeID).Msg("Failed to get node for sync")
		return
	}

	if !node.Enabled {
		return
	}

	secretKey := node.SecretKey.GetOr("")

	syncState, errSyncState := e.db.GetOrCreatePeerSyncState(nodeID)
	if errSyncState != nil {
		log.Warn().Err(errSyncState).Str("node_id", nodeID).Msg("Failed to get sync state")
	}

	// Check backoff.
	if syncState != nil && syncState.RetryCount > 0 {
		nextRetry := syncState.NextRetryAt.GetOr(time.Time{})
		if time.Now().Before(nextRetry) {
			return
		}
	}

	client := newFederationClient(node.BaseURL)
	reqCtx, cancel := context.WithTimeout(e.ctx, federationRequestTimeout)
	defer cancel()

	req := connect.NewRequest(&xylona.FederationListServerSummariesRequest{
		Limit: 1000,
	})
	req.Header().Set("X-Federation-Key", secretKey)

	resp, errSync := client.ListServerSummaries(reqCtx, req)
	if errSync != nil {
		log.Warn().Err(errSync).Str("node_id", nodeID).Str("node_name", node.Name).Msg("Failed to sync server summaries from node")

		retryCount := int32(0)
		if syncState != nil {
			retryCount = syncState.RetryCount + 1
		}
		backoff := calculateBackoff(retryCount)
		errUpdateSync := e.db.UpdatePeerSyncStateError(nodeID, errSync.Error(), retryCount, time.Now().Add(backoff))
		if errUpdateSync != nil {
			log.Error().Err(errUpdateSync).Str("node_id", nodeID).Msg("Failed to update sync state error")
		}
		errUpdateStatus := e.db.UpdateNodeSyncStatus(nodeID, "error", time.Now())
		if errUpdateStatus != nil {
			log.Error().Err(errUpdateStatus).Str("node_id", nodeID).Msg("Failed to update node sync status")
		}

		// Mark servers as stale on sync failure.
		errStale := e.db.MarkRemoteServerCacheStaleByNodeID(nodeID)
		if errStale != nil {
			log.Error().Err(errStale).Str("node_id", nodeID).Msg("Failed to mark remote servers as stale")
		}
		return
	}

	// Upsert all server summaries from the node.
	for _, server := range resp.Msg.Servers {
		newID, errID := helpers.GenerateUniqueID()
		if errID != nil {
			log.Error().Err(errID).Msg("Failed to generate ID for remote server cache")
			continue
		}

		errUpsert := e.db.UpsertRemoteServerCache(
			newID.String(),
			node.ID,
			nodeID,
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
			node.Name,
			node.BaseURL,
			server.UpdatedAt.AsTime(),
		)
		if errUpsert != nil {
			log.Error().Err(errUpsert).Str("server_id", server.ServerId).Msg("Failed to upsert remote server cache")
		}
	}

	// Update sync state on success.
	errSyncSuccess := e.db.UpdatePeerSyncStateSuccess(nodeID, "")
	if errSyncSuccess != nil {
		log.Error().Err(errSyncSuccess).Str("node_id", nodeID).Msg("Failed to update sync state success")
	}
	errSyncStatus := e.db.UpdateNodeSyncStatus(nodeID, "success", time.Now())
	if errSyncStatus != nil {
		log.Error().Err(errSyncStatus).Str("node_id", nodeID).Msg("Failed to update node sync status")
	}

	log.Debug().Str("node_id", nodeID).Str("node_name", node.Name).
		Int("server_count", len(resp.Msg.Servers)).Msg("Successfully synced server summaries from node")
}

func newFederationClient(baseURL string) xylonaconnect.FederationClient {
	httpClient := &http.Client{
		Timeout: federationRequestTimeout,
	}
	return xylonaconnect.NewFederationClient(httpClient, baseURL)
}

func (e *FederationSyncEngine) getNodeSyncInterval(nodeID string) time.Duration {
	syncIntervalSeconds, errSyncInterval := e.db.GetNodeSyncIntervalSeconds(nodeID)
	if errSyncInterval != nil {
		log.Debug().Err(errSyncInterval).Str("node_id", nodeID).
			Dur("fallback_interval", defaultNodeSyncInterval).
			Msg("Failed to get node sync interval, using default")
		return defaultNodeSyncInterval
	}
	return normalizeNodeSyncInterval(syncIntervalSeconds)
}

func normalizeNodeSyncInterval(syncIntervalSeconds int32) time.Duration {
	if syncIntervalSeconds <= 0 {
		return defaultNodeSyncInterval
	}
	interval := time.Duration(syncIntervalSeconds) * time.Second
	if interval < minNodeSyncInterval {
		return minNodeSyncInterval
	}
	if interval > maxNodeSyncInterval {
		return maxNodeSyncInterval
	}
	return interval
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
