package websocket

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/olahol/melody"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/sysinfo"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// newFederationClientForNode creates a trusted-peer mTLS federation client for
// the given remote node, mirroring newRemoteFederationClient in api/rpc/common.go.
func (ws *WebSocket) newFederationClientForNode(node *models.Node) (xylonaconnect.FederationClient, error) {
	federationPort := ws.federationMTLS.FederationPort()
	if node.Port > 0 {
		federationPort = int(node.Port)
	}
	httpClient, baseURL, errClient := ws.federationMTLS.NewTrustedPeerHTTPClientWithPort(
		15*time.Second, node.ID, node.BaseURL, federationPort, ws.db,
	)
	if errClient != nil {
		return nil, fmt.Errorf("websocket: create trusted federation client: %w", errClient)
	}
	return xylonaconnect.NewFederationClient(httpClient, baseURL), nil
}

// startRemoteNodeMetricsPoller runs a shared background poller that refreshes
// the remote node resource snapshot cache every 5 seconds.
func (ws *WebSocket) startRemoteNodeMetricsPoller() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ws.ctx.Done():
			return
		case <-ticker.C:
			ws.pollRemoteNodeSnapshots()
		}
	}
}

func (ws *WebSocket) pollRemoteNodeSnapshots() {
	if ws.federationMTLS == nil {
		return
	}

	nodes, errNodes := ws.db.GetEnabledRemoteNodes()
	if errNodes != nil {
		log.Error().Err(errNodes).Msg("Failed to get enabled remote nodes for node metrics poll")
		return
	}
	if len(nodes) == 0 {
		ws.remoteNodeSnapshotCacheLock.Lock()
		ws.remoteNodeSnapshotCache = make(map[string]*xylona.NodeResourceSnapshot)
		ws.remoteNodeSnapshotCacheLock.Unlock()
		return
	}

	type nodeResult struct {
		nodeID   string
		snapshot *xylona.NodeResourceSnapshot
	}

	resultsCh := make(chan nodeResult, len(nodes))

	for _, n := range nodes {
		node := n
		go func() {
			client, errClient := ws.newFederationClientForNode(node)
			if errClient != nil {
				log.Debug().Err(errClient).Str("node_id", node.ID).Msg("Node metrics poll: failed to create federation client")
				resultsCh <- nodeResult{nodeID: node.ID}
				return
			}
			ctx, cancel := context.WithTimeout(ws.ctx, 10*time.Second)
			defer cancel()
			resp, errRPC := client.FederationGetNodeResourceSnapshot(ctx, connect.NewRequest(&xylona.FederationGetNodeResourceSnapshotRequest{}))
			if errRPC != nil {
				log.Debug().Err(errRPC).Str("node_id", node.ID).Msg("Node metrics poll: FederationGetNodeResourceSnapshot failed")
				resultsCh <- nodeResult{nodeID: node.ID}
				return
			}
			resultsCh <- nodeResult{nodeID: node.ID, snapshot: resp.Msg.GetSnapshot()}
		}()
	}

	newCache := make(map[string]*xylona.NodeResourceSnapshot, len(nodes))
	for range nodes {
		result := <-resultsCh
		if result.snapshot != nil {
			newCache[result.nodeID] = result.snapshot
		}
	}

	ws.remoteNodeSnapshotCacheLock.Lock()
	ws.remoteNodeSnapshotCache = newCache
	ws.remoteNodeSnapshotCacheLock.Unlock()
}

// collectLocalNodeSnapshot collects resource metrics for the local node.
func (ws *WebSocket) collectLocalNodeSnapshot() *xylona.NodeResourceSnapshot {
	snapshot, errSnap := sysinfo.CollectResourceSnapshot()
	if errSnap != nil {
		log.Error().Err(errSnap).Msg("Failed to collect local resource snapshot for node metrics")
		return nil
	}

	gsCount, errGS := ws.db.CountGameServers()
	if errGS != nil {
		log.Error().Err(errGS).Msg("Failed to count game servers for node metrics")
	}

	runningCount := 0
	for _, cmd := range ws.supervisor.ListCommands() {
		if cmd.Status() == xylona.Status_ONLINE || cmd.Status() == xylona.Status_INSTALLING || cmd.Status() == xylona.Status_UPDATING {
			runningCount++
		}
	}

	userCount, errUsers := ws.db.CountUsers()
	if errUsers != nil {
		log.Error().Err(errUsers).Msg("Failed to count users for node metrics")
	}

	return &xylona.NodeResourceSnapshot{
		CpuPercent:             snapshot.CPUPercent,
		MemoryPercent:          snapshot.MemoryPercent,
		MemoryUsedBytes:        helpers.ClampInt64FromUint64(snapshot.MemoryUsed),
		MemoryTotalBytes:       helpers.ClampInt64FromUint64(snapshot.MemoryTotal),
		DiskPercent:            snapshot.DiskPercent,
		DiskUsedBytes:          helpers.ClampInt64FromUint64(snapshot.DiskUsed),
		DiskTotalBytes:         helpers.ClampInt64FromUint64(snapshot.DiskTotal),
		GameServerCount:        helpers.ClampInt32FromInt(gsCount),
		RunningGameServerCount: helpers.ClampInt32FromInt(runningCount),
		UserCount:              helpers.ClampInt32FromInt(userCount),
		RecordedAt:             timestamppb.Now(),
	}
}

func (ws *WebSocket) startRemoteConsoleStream(s *melody.Session, conn *connection, serverID string) {
	remoteCache, errGetRemote := ws.db.GetRemoteServerCacheByRemoteServerID(serverID)
	if errGetRemote != nil {
		if errors.Is(errGetRemote, db.ErrAmbiguousRemoteServerCache) {
			log.Error().Err(errGetRemote).Str("server_id", serverID).Msg("Ambiguous remote server mapping for console stream")
			return
		}
		log.Error().Err(errGetRemote).Str("server_id", serverID).Msg("Remote server not found for console stream")
		return
	}

	peerNode, errGetPeer := ws.db.GetRemoteNodeByID(remoteCache.NodeID)
	if errGetPeer != nil {
		log.Error().Err(errGetPeer).Str("node_id", remoteCache.NodeID).Msg("Peer node not found for console stream")
		return
	}
	if ws.federationMTLS == nil {
		log.Error().Msg("Federation mTLS is not configured for remote console stream")
		return
	}

	trustedPeer, errTrustedPeer := ws.db.GetFederationTrustedPeerByNodeID(peerNode.ID)
	if errTrustedPeer != nil {
		log.Error().Err(errTrustedPeer).Str("node_id", peerNode.ID).Msg("Failed to load trusted peer for console stream")
		return
	}
	if !trustedPeer.Enabled || trustedPeer.Revoked {
		log.Warn().Str("node_id", peerNode.ID).Msg("Remote peer is disabled or revoked for console stream")
		return
	}

	streamCtx, cancel := context.WithCancel(ws.ctx)

	conn.Lock()
	// Cancel any existing stream for this server.
	if existingCancel, exists := conn.remoteConsoleCancels[serverID]; exists {
		existingCancel()
	}
	conn.remoteConsoleCancels[serverID] = cancel
	conn.Unlock()

	federationPort := ws.federationMTLS.FederationPort()
	if peerNode.Port > 0 {
		federationPort = int(peerNode.Port)
	}

	httpClient, federationBaseURL, errClient := ws.federationMTLS.NewNodeHTTPClientWithPort(
		0,
		peerNode.BaseURL,
		federationPort,
		trustedPeer.PeerFingerprint,
		trustedPeer.PeerNodeID,
	)
	if errClient != nil {
		log.Error().Err(errClient).Str("node_id", peerNode.ID).Msg("Failed to create federation client for remote console stream")
		cancel()
		return
	}
	client := xylonaconnect.NewFederationClient(httpClient, federationBaseURL)

	req := connect.NewRequest(&xylona.FederationStreamConsoleRequest{
		ServerId: serverID,
	})

	userID, errGetUserID := getSessionUserID(s)
	if errGetUserID != nil {
		log.Error().Err(errGetUserID).Str("server_id", serverID).Msg("Failed to get session user ID for remote console stream")
		cancel()
		return
	}
	errIdentity := ws.applyFederatedActingIdentity(req.Header(), userID)
	if errIdentity != nil {
		log.Error().Err(errIdentity).Str("server_id", serverID).Str("peer", peerNode.Name).Msg("Failed to apply federation identity headers for remote console stream")
		cancel()
		return
	}

	stream, errStream := client.StreamConsoleOutput(streamCtx, req)
	if errStream != nil {
		log.Error().Err(errStream).Str("server_id", serverID).Str("peer", peerNode.Name).Msg("Failed to open remote console stream")
		cancel()
		return
	}
	defer func() { _ = stream.Close() }()

	log.Debug().Str("server_id", serverID).Str("peer", peerNode.Name).Msg("Remote console stream started")

	for stream.Receive() {
		chunk := stream.Msg()
		msg := &xylona.Message{
			Type: xylona.Message_GameServerConsole,
			GameServerConsoleOutput: &xylona.GameServerConsoleOutput{
				GameServerId: serverID,
				Output:       chunk.GetOutput(),
			},
		}

		select {
		case conn.outputStreamChannel <- msg:
		case <-streamCtx.Done():
			return
		case <-s.Request.Context().Done():
			return
		default:
			// Channel buffer full, skip this message to avoid blocking.
		}
	}

	if errReceive := stream.Err(); errReceive != nil {
		log.Debug().Err(errReceive).Str("server_id", serverID).Msg("Remote console stream ended")
	}
}
