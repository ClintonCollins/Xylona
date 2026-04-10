package actions

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/helpers/federation"
	"github.com/ClintonCollins/Xylona/pkg/alerts"
	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	healthCheckInterval           = 30 * time.Second
	peerReconcileInterval         = 60 * time.Second
	remoteCacheCleanupInterval    = 60 * time.Second
	defaultNodeSyncInterval       = 5 * time.Minute
	minNodeSyncInterval           = 60 * time.Second
	maxNodeSyncInterval           = 10 * time.Minute
	staleThreshold                = 3 * time.Minute
	maxRetryBackoff               = 5 * time.Minute
	federationRequestTimeout      = 15 * time.Second
	federationFileTransferTimeout = 2 * time.Hour
)

// StatusBroadcaster is called when a remote server status changes in real-time.
type StatusBroadcaster interface {
	BroadcastRemoteServerStatus(serverID string, serverName string, status xylona.Status)
}

// MetricsBroadcaster is called when remote server metrics are received in real-time.
type MetricsBroadcaster interface {
	BroadcastRemoteServerMetrics(serverID string, metrics *xylona.GameServerMetrics)
}

// RemoteVersionBroadcaster is called when remote server version info changes in real-time.
type RemoteVersionBroadcaster interface {
	BroadcastGameServerVersion(serverID string, version string, versionInfo *xylona.VersionInfo)
}

// FederationSyncEngine manages periodic synchronization with peer nodes.
type FederationSyncEngine struct {
	ctx                context.Context
	db                 *db.Connection
	federationMTLS     *federation.MTLS
	mu                 sync.RWMutex
	peerStops          map[string]context.CancelFunc
	statusBroadcaster  StatusBroadcaster
	metricsBroadcaster MetricsBroadcaster
	versionBroadcaster RemoteVersionBroadcaster
	actionsInst        *Instance
}

// NewFederationSyncEngine creates and starts the federation sync engine.
func NewFederationSyncEngine(ctx context.Context, dbInst *db.Connection, federationMTLS *federation.MTLS) *FederationSyncEngine {
	engine := &FederationSyncEngine{
		ctx:            ctx,
		db:             dbInst,
		federationMTLS: federationMTLS,
		peerStops:      make(map[string]context.CancelFunc),
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

// SetMetricsBroadcaster sets the callback for real-time remote server metrics updates.
func (e *FederationSyncEngine) SetMetricsBroadcaster(broadcaster MetricsBroadcaster) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.metricsBroadcaster = broadcaster
}

// SetVersionBroadcaster sets the callback for real-time remote server version updates.
func (e *FederationSyncEngine) SetVersionBroadcaster(broadcaster RemoteVersionBroadcaster) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.versionBroadcaster = broadcaster
}

// SetActionsInstance sets the actions instance for peer list operations.
func (e *FederationSyncEngine) SetActionsInstance(inst *Instance) {
	e.actionsInst = inst
}

func randomJitter(maxDelay time.Duration) time.Duration {
	if maxDelay <= 0 {
		return 0
	}

	maxNanos := big.NewInt(maxDelay.Nanoseconds())
	jitter, errJitter := rand.Int(rand.Reader, maxNanos)
	if errJitter != nil {
		return 0
	}
	return time.Duration(jitter.Int64())
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

	// Keep worker membership and remote cache cleanup in sync over time.
	reconcileTicker := time.NewTicker(peerReconcileInterval)
	cleanupTicker := time.NewTicker(remoteCacheCleanupInterval)
	defer reconcileTicker.Stop()
	defer cleanupTicker.Stop()

	e.cleanupRemoteServerCache()
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-reconcileTicker.C:
			e.reconcilePeerWorkers()
		case <-cleanupTicker.C:
			e.cleanupRemoteServerCache()
		}
	}
}

func (e *FederationSyncEngine) cleanupRemoteServerCache() {
	errOrphanCleanup := e.db.DeleteOrphanedRemoteServerCacheByNodeReferences()
	if errOrphanCleanup != nil {
		if e.db.IsBusyError(errOrphanCleanup) {
			log.Debug().Err(errOrphanCleanup).Msg("Database busy during orphaned remote server cache cleanup, will retry later")
		} else {
			log.Warn().Err(errOrphanCleanup).Msg("Failed to clean orphaned remote server cache rows")
		}
	}

	remoteNodes, errGetRemoteNodes := e.db.GetAllRemoteNodes()
	if errGetRemoteNodes != nil {
		if e.db.IsBusyError(errGetRemoteNodes) {
			log.Debug().Err(errGetRemoteNodes).Msg("Database busy while listing remote nodes for stale cache cleanup, will retry later")
		} else {
			log.Warn().Err(errGetRemoteNodes).Msg("Failed to list remote nodes for stale cache cleanup")
		}
		return
	}

	olderThan := time.Now().Add(-staleThreshold)
	for _, remoteNode := range remoteNodes {
		errDeleteStale := e.db.DeleteStaleRemoteServerCacheByNodeID(remoteNode.ID, olderThan)
		if errDeleteStale != nil {
			if e.db.IsBusyError(errDeleteStale) {
				log.Debug().
					Err(errDeleteStale).
					Str("node_id", remoteNode.ID).
					Msg("Database busy during stale remote server cache cleanup, will retry later")
			} else {
				log.Warn().
					Err(errDeleteStale).
					Str("node_id", remoteNode.ID).
					Msg("Failed to delete stale remote server cache rows")
			}
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

	//nolint:gosec // The cancel func is stored for later worker teardown in RemovePeer/reconcilePeerWorkers.
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
	jitter := randomJitter(10 * time.Second)
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

		e.runUpdatesStream(ctx, nodeID)

		// Backoff before reconnecting.
		select {
		case <-ctx.Done():
			return
		case <-time.After(5*time.Second + randomJitter(3*time.Second)):
		}
	}
}

func (e *FederationSyncEngine) runUpdatesStream(ctx context.Context, nodeID string) {
	node, errGetNode := e.db.GetRemoteNodeByID(nodeID)
	if errGetNode != nil {
		log.Warn().Err(errGetNode).Str("node_id", nodeID).Msg("Failed to get remote node for updates stream")
		return
	}
	if !node.Enabled {
		return
	}
	client, errClient := newFederationClient(e.db, e.federationMTLS, node, 0)
	if errClient != nil {
		log.Debug().Err(errClient).Str("node_id", nodeID).Msg("Failed to create federation client for updates stream")
		return
	}
	req := connect.NewRequest(&xylona.FederationStreamServerUpdatesRequest{})
	stream, errStream := client.StreamServerUpdates(ctx, req)
	if errStream != nil {
		log.Debug().Err(errStream).Str("node_id", nodeID).Msg("Failed to open updates stream")
		return
	}
	defer func() { _ = stream.Close() }()

	// Feed received messages into a channel so the select loop can also handle
	// a staleness timer without blocking on Receive.
	type recvResult struct {
		msg *xylona.FederationServerUpdateEvent
		ok  bool
	}
	recvCh := make(chan recvResult, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			ok := stream.Receive()
			if !ok {
				select {
				case recvCh <- recvResult{ok: false}:
				case <-done:
				}
				return
			}
			select {
			case recvCh <- recvResult{msg: stream.Msg(), ok: true}:
			case <-done:
				return
			}
		}
	}()

	staleTimer := time.NewTimer(90 * time.Second)
	defer staleTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-staleTimer.C:
			log.Warn().Str("node_id", nodeID).Msg("Updates stream stale, no heartbeat received within 90s")
			return
		case result := <-recvCh:
			if !result.ok {
				return
			}
			// Any message resets the staleness timer.
			if !staleTimer.Stop() {
				select {
				case <-staleTimer.C:
				default:
				}
			}
			staleTimer.Reset(90 * time.Second)

			switch evt := result.msg.GetEvent().(type) {
			case *xylona.FederationServerUpdateEvent_Snapshot:
				e.handleSnapshot(node, evt.Snapshot)
			case *xylona.FederationServerUpdateEvent_StatusChange:
				e.handleStatusChange(node, evt.StatusChange)
			case *xylona.FederationServerUpdateEvent_MetricsUpdate:
				e.handleMetricsUpdate(evt.MetricsUpdate)
			case *xylona.FederationServerUpdateEvent_VersionChange:
				e.handleVersionChange(node, evt.VersionChange)
			case *xylona.FederationServerUpdateEvent_AlertEvent:
				e.handleAlertEvent(nodeID, evt.AlertEvent)
			case *xylona.FederationServerUpdateEvent_Heartbeat:
				log.Debug().Str("node_id", nodeID).Msg("Received heartbeat from peer")
			}
		}
	}
}

func (e *FederationSyncEngine) healthCheckPeer(nodeID string) {
	node, errGetNode := e.db.GetRemoteNodeByID(nodeID)
	if errGetNode != nil {
		log.Warn().Err(errGetNode).Str("node_id", nodeID).Msg("Failed to get node for health check")
		return
	}

	client, errClient := newFederationClient(e.db, e.federationMTLS, node, federationRequestTimeout)
	if errClient != nil {
		log.Warn().Err(errClient).Str("node_id", nodeID).Str("node_name", node.Name).Msg("Node health check failed")
		errUpdate := e.db.UpdateNodeHealth(nodeID, "offline", time.Now())
		if errUpdate != nil {
			log.Error().Err(errUpdate).Str("node_id", nodeID).Msg("Failed to update node health status")
		}
		errStale := e.db.MarkRemoteServerCacheStaleByNodeID(nodeID)
		if errStale != nil {
			log.Error().Err(errStale).Str("node_id", nodeID).Msg("Failed to mark remote servers as stale")
		}
		return
	}

	reqCtx, cancel := context.WithTimeout(e.ctx, federationRequestTimeout)
	defer cancel()

	req := connect.NewRequest(&xylona.FederationHandshakeRequest{})

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

	// Check if the peer has departed the federation.
	if resp.Msg.GetDeparted() {
		log.Info().Str("node_id", nodeID).Msg("Peer has departed the federation, removing")
		_ = e.db.DeleteNodeByID(nodeID)
		e.RemovePeer(nodeID)
		return
	}

	// Capture previous health status for reconnect detection.
	previousHealth := node.HealthStatus

	errUpdate := e.db.UpdateNodeHealth(nodeID, "healthy", time.Now())
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("node_id", nodeID).Msg("Failed to update node health status")
	}

	// Update identity if it changed.
	errIdentity := e.db.UpdateNodeIdentity(
		nodeID,
		resp.Msg.GetVersion(),
		resp.Msg.GetProtocolVersion(),
		resp.Msg.GetCapabilities(),
		resp.Msg.GetSystemInfo().GetOs(),
	)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("node_id", nodeID).Msg("Failed to update node identity")
	}

	// Reconnect reconciliation: exchange peer lists when transitioning to healthy.
	if previousHealth == "offline" || previousHealth == "unknown" || previousHealth == "" {
		go e.reconcilePeerListOnReconnect(nodeID)
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

	client, errClient := newFederationClient(e.db, e.federationMTLS, node, federationRequestTimeout)
	if errClient != nil {
		log.Warn().Err(errClient).Str("node_id", nodeID).Str("node_name", node.Name).Msg("Failed to create federation client for node sync")

		retryCount := int64(0)
		if syncState != nil {
			retryCount = syncState.RetryCount + 1
		}
		backoff := calculateBackoff(retryCount)
		errUpdateSync := e.db.UpdatePeerSyncStateError(nodeID, errClient.Error(), retryCount, time.Now().Add(backoff))
		if errUpdateSync != nil {
			log.Error().Err(errUpdateSync).Str("node_id", nodeID).Msg("Failed to update sync state error")
		}
		errUpdateStatus := e.db.UpdateNodeSyncStatus(nodeID, "error", time.Now())
		if errUpdateStatus != nil {
			log.Error().Err(errUpdateStatus).Str("node_id", nodeID).Msg("Failed to update node sync status")
		}

		errStale := e.db.MarkRemoteServerCacheStaleByNodeID(nodeID)
		if errStale != nil {
			log.Error().Err(errStale).Str("node_id", nodeID).Msg("Failed to mark remote servers as stale")
		}
		return
	}

	reqCtx, cancel := context.WithTimeout(e.ctx, federationRequestTimeout)
	defer cancel()

	req := connect.NewRequest(&xylona.FederationListServerSummariesRequest{
		Limit: 1000,
	})

	resp, errSync := client.ListServerSummaries(reqCtx, req)
	if errSync != nil {
		log.Warn().Err(errSync).Str("node_id", nodeID).Str("node_name", node.Name).Msg("Failed to sync server summaries from node")

		retryCount := int64(0)
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
	for _, server := range resp.Msg.GetServers() {
		newID, errID := helpers.GenerateUniqueID()
		if errID != nil {
			log.Error().Err(errID).Msg("Failed to generate ID for remote server cache")
			continue
		}

		errUpsert := e.db.UpsertRemoteServerCache(
			newID.String(),
			node.ID,
			nodeID,
			server.GetServerId(),
			server.GetDisplayName(),
			server.GetStatus().String(),
			server.GetGameName(),
			server.GetGameId(),
			server.GetIpAddress(),
			helpers.ClampInt32FromInt64(server.GetPort()),
			helpers.ClampInt32FromInt64(server.GetQueryPort()),
			helpers.ClampInt32FromInt64(server.GetMaxPlayers()),
			helpers.ClampInt32FromInt64(server.GetCurrentPlayers()),
			server.GetMapName(),
			server.GetVersion(),
			node.Name,
			node.BaseURL,
			server.GetUpdatedAt().AsTime(),
		)
		if errUpsert != nil {
			log.Error().Err(errUpsert).Str("server_id", server.GetServerId()).Msg("Failed to upsert remote server cache")
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
		Int("server_count", len(resp.Msg.GetServers())).Msg("Successfully synced server summaries from node")
}

func newFederationClient(dbConn *db.Connection, federationMTLS *federation.MTLS, node *models.Node, timeout time.Duration) (xylonaconnect.FederationClient, error) {
	if federationMTLS == nil {
		return nil, errors.New("federation mTLS is not configured")
	}

	federationPort := federationMTLS.FederationPort()
	if node != nil && node.Port > 0 {
		federationPort = int(node.Port)
	}

	httpClient, federationBaseURL, errClient := federationMTLS.NewTrustedPeerHTTPClientWithPort(
		timeout,
		node.ID,
		node.BaseURL,
		federationPort,
		dbConn,
	)
	if errClient != nil {
		return nil, fmt.Errorf("actions: create federation client: %w", errClient)
	}

	return xylonaconnect.NewFederationClient(httpClient, federationBaseURL), nil
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

func calculateBackoff(retryCount int64) time.Duration {
	base := time.Duration(math.Pow(2, float64(retryCount))) * time.Second
	base = min(base, maxRetryBackoff)
	// Add jitter.
	jitter := randomJitter(5 * time.Second)
	return base + jitter
}

// handleSnapshot processes a full server snapshot from a peer node.
func (e *FederationSyncEngine) handleSnapshot(node *models.Node, snapshot *xylona.FederationServerSnapshot) {
	for _, server := range snapshot.GetServers() {
		newID, errID := helpers.GenerateUniqueID()
		if errID != nil {
			log.Error().Err(errID).Msg("Failed to generate ID for remote server cache during snapshot")
			continue
		}

		lastRemoteUpdate := time.Now()

		errUpsert := e.db.UpsertRemoteServerCache(
			newID.String(),
			node.ID,
			node.ID,
			server.GetServerId(),
			server.GetDisplayName(),
			server.GetStatus().String(),
			server.GetGameName(),
			server.GetGameId(),
			server.GetIpAddress(),
			server.GetPort(),
			server.GetQueryPort(),
			server.GetMaxPlayers(),
			server.GetCurrentPlayers(),
			server.GetMapName(),
			server.GetVersion(),
			node.Name,
			node.BaseURL,
			lastRemoteUpdate,
		)
		if errUpsert != nil {
			log.Error().Err(errUpsert).Str("server_id", server.GetServerId()).Msg("Failed to upsert remote server cache from snapshot")
		}
	}
}

// handleStatusChange processes a single server status change from a peer node.
func (e *FederationSyncEngine) handleStatusChange(node *models.Node, change *xylona.FederationServerStatusChange) {
	errUpdate := e.db.UpdateRemoteServerCacheStatus(node.ID, change.GetServerId(), change.GetStatus().String())
	if errUpdate != nil {
		log.Error().Err(errUpdate).
			Str("node_id", node.ID).
			Str("server_id", change.GetServerId()).
			Msg("Failed to update remote server cache status")
	}

	e.mu.RLock()
	broadcaster := e.statusBroadcaster
	e.mu.RUnlock()

	if broadcaster != nil {
		broadcaster.BroadcastRemoteServerStatus(change.GetServerId(), change.GetDisplayName(), change.GetStatus())
	}
}

// handleMetricsUpdate processes a server metrics update from a peer node.
func (e *FederationSyncEngine) handleMetricsUpdate(update *xylona.FederationServerMetricsUpdate) {
	e.mu.RLock()
	broadcaster := e.metricsBroadcaster
	e.mu.RUnlock()

	if broadcaster != nil {
		broadcaster.BroadcastRemoteServerMetrics(update.GetServerId(), update.GetMetrics())
	}
}

func (e *FederationSyncEngine) handleVersionChange(node *models.Node, change *xylona.FederationServerVersionChange) {
	errUpdate := e.db.UpdateRemoteServerCacheVersion(node.ID, change.GetServerId(), change.GetVersion())
	if errUpdate != nil {
		log.Error().Err(errUpdate).
			Str("node_id", node.ID).
			Str("server_id", change.GetServerId()).
			Msg("Failed to update remote server cache version")
	}

	e.mu.RLock()
	broadcaster := e.versionBroadcaster
	e.mu.RUnlock()

	if broadcaster != nil {
		broadcaster.BroadcastGameServerVersion(change.GetServerId(), change.GetVersion(), change.GetVersionInfo())
	}
}

// handleAlertEvent processes a federation alert event received from a remote
// peer by deserializing the payload and republishing it on the local event bus
// so the local alert evaluator can evaluate rules against it.
func (e *FederationSyncEngine) handleAlertEvent(nodeID string, evt *xylona.FederationAlertEvent) {
	if _, ok := alerts.AlertProtoTypeToTopic[evt.GetEventType()]; !ok {
		log.Warn().
			Str("node_id", nodeID).
			Str("event_type", evt.GetEventType().String()).
			Msg("Received federation alert event with unknown event type, skipping")
		return
	}

	alerts.RepublishFederationAlertEvent(eventbus.Get(), evt)
}

// BroadcastPeerChange sends a NotifyPeerChange to all connected peers concurrently.
func (e *FederationSyncEngine) BroadcastPeerChange(
	changeType xylona.PeerChangeType,
	peer *xylona.PeerInfo,
	initiatedByNodeID string,
	initiatedByNodeName string,
) {
	nodes, errNodes := e.db.GetEnabledRemoteNodes()
	if errNodes != nil {
		log.Error().Err(errNodes).Msg("Failed to get remote nodes for peer change broadcast")
		return
	}

	msg := &xylona.NotifyPeerChangeRequest{
		ChangeType:          changeType,
		Peer:                peer,
		InitiatedByNodeId:   initiatedByNodeID,
		InitiatedByNodeName: initiatedByNodeName,
	}

	var wg sync.WaitGroup
	for _, node := range nodes {
		// Don't notify the node the change is about.
		if node.ID == peer.GetNodeId() {
			continue
		}
		wg.Add(1)
		go func(n *models.Node) {
			defer wg.Done()
			client, errClient := newFederationClient(e.db, e.federationMTLS, n, federationRequestTimeout)
			if errClient != nil {
				log.Warn().Err(errClient).Str("node_id", n.ID).Msg("Failed to create client for peer change broadcast")
				return
			}
			reqCtx, cancel := context.WithTimeout(e.ctx, federationRequestTimeout)
			defer cancel()
			_, errNotify := client.NotifyPeerChange(reqCtx, connect.NewRequest(msg))
			if errNotify != nil {
				log.Warn().Err(errNotify).Str("node_id", n.ID).Msg("Failed to broadcast peer change")
			}
		}(node)
	}
	wg.Wait()
}

// reconcilePeerListOnReconnect exchanges peer lists with a peer that just came back online.
func (e *FederationSyncEngine) reconcilePeerListOnReconnect(nodeID string) {
	if e.actionsInst == nil {
		return
	}

	node, errNode := e.db.GetRemoteNodeByID(nodeID)
	if errNode != nil {
		log.Warn().Err(errNode).Str("node_id", nodeID).Msg("Failed to get node for peer list reconciliation")
		return
	}

	client, errClient := newFederationClient(e.db, e.federationMTLS, node, federationRequestTimeout)
	if errClient != nil {
		log.Warn().Err(errClient).Str("node_id", nodeID).Msg("Failed to create client for peer list reconciliation")
		return
	}

	localPeers, errBuild := e.actionsInst.BuildLocalPeerList()
	if errBuild != nil {
		log.Error().Err(errBuild).Msg("Failed to build local peer list for reconciliation")
		return
	}

	localSettings, errSettings := e.db.GetLocalSettings()
	if errSettings != nil {
		log.Error().Err(errSettings).Msg("Failed to get local settings for reconciliation")
		return
	}

	reqCtx, cancel := context.WithTimeout(e.ctx, federationRequestTimeout)
	defer cancel()

	resp, errExchange := client.ExchangePeerList(reqCtx, connect.NewRequest(&xylona.ExchangePeerListRequest{
		SenderNodeId: localSettings.NodeID,
		Peers:        localPeers,
	}))
	if errExchange != nil {
		log.Warn().Err(errExchange).Str("node_id", nodeID).Msg("Peer list exchange failed on reconnect")
		return
	}

	// Process received peer list for auto-pairing.
	e.actionsInst.ProcessReceivedPeerList(resp.Msg.GetPeers(), nodeID)

	log.Info().Str("node_id", nodeID).Int("remote_peers", len(resp.Msg.GetPeers())).Msg("Peer list exchanged on reconnect")
}
