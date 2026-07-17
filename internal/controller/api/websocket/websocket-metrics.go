package websocket

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/olahol/melody"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	nodeSnapshotFetchTimeout    = 5 * time.Second
	processSnapshotFetchTimeout = 3 * time.Second
)

// buildNodeResourceSnapshot converts a node.NodeSnapshot plus DB-derived counts
// into the on-the-wire NodeResourceSnapshot shape the websocket broadcasts.
func buildNodeResourceSnapshot(snap *node.NodeSnapshot, gsCount, userCount int) *xylona.NodeResourceSnapshot {
	runningCount := 0
	for _, ps := range snap.Processes {
		switch ps.Status {
		case xylona.Status_ONLINE.String(),
			xylona.Status_INSTALLING.String(),
			xylona.Status_UPDATING.String():
			runningCount++
		}
	}
	return &xylona.NodeResourceSnapshot{
		CpuPercent:             snap.CPUPercent,
		MemoryPercent:          snap.MemoryPercent,
		MemoryUsedBytes:        helpers.ClampInt64FromUint64(snap.MemoryUsed),
		MemoryTotalBytes:       helpers.ClampInt64FromUint64(snap.TotalMemory),
		DiskPercent:            snap.DiskPercent,
		DiskUsedBytes:          helpers.ClampInt64FromUint64(snap.DiskUsed),
		DiskTotalBytes:         helpers.ClampInt64FromUint64(snap.DiskTotal),
		GameServerCount:        helpers.ClampInt32FromInt(gsCount),
		RunningGameServerCount: helpers.ClampInt32FromInt(runningCount),
		UserCount:              helpers.ClampInt32FromInt(userCount),
		RecordedAt:             timestamppb.Now(),
	}
}

// processSnapshotToGameServerMetrics converts a node.ProcessSnapshot to the
// on-the-wire GameServerMetrics shape. Returns nil if snap is nil.
func processSnapshotToGameServerMetrics(snap *node.ProcessSnapshot, collectedAt time.Time) *xylona.GameServerMetrics {
	if snap == nil {
		return nil
	}
	var uptimeSeconds int64
	if snap.UnixStartedAt > 0 {
		uptimeSeconds = time.Now().Unix() - snap.UnixStartedAt
	}
	return &xylona.GameServerMetrics{
		CpuPercent:            snap.CPUPercent,
		CpuValid:              snap.CPUValid,
		MetricsValid:          snap.MetricsValid,
		MemoryBytes:           helpers.ClampInt64FromUint64(snap.MemoryVMS),
		MemoryWorkingSetBytes: helpers.ClampInt64FromUint64(snap.MemoryRSS),
		MemoryPercent:         float64(snap.MemoryPercent),
		CpuCores:              snap.CPUCores,
		NumberOfThreads:       snap.NumThreads,
		DiskUsageBytes:        helpers.ClampInt64FromUint64(snap.DiskUsageBytes),
		DiskTotalBytes:        helpers.ClampInt64FromUint64(snap.DiskTotalBytes),
		DiskFreeBytes:         helpers.ClampInt64FromUint64(snap.DiskFreeBytes),
		DiskPercent:           snap.DiskPercent,
		DiskValid:             snap.DiskValid,
		IoValid:               snap.IOValid,
		IoReadRate:            snap.IOReadRate,
		IoWriteRate:           snap.IOWriteRate,
		ConnectionCount:       snap.ConnectionCount,
		ConnectionCountValid:  snap.ConnectionCountValid,
		ProcessStatus:         snap.Status,
		CollectionStatus:      processMetricsCollectionStatus(snap),
		UptimeSeconds:         uptimeSeconds,
		CollectedAt:           timestamppb.New(collectedAt),
	}
}

func processMetricsCollectionStatus(snap *node.ProcessSnapshot) xylona.GameServerMetricsCollectionStatus {
	if snap == nil {
		return xylona.GameServerMetricsCollectionStatus_GAME_SERVER_METRICS_COLLECTION_STATUS_UNSPECIFIED
	}
	if snap.Status == xylona.Status_OFFLINE.String() {
		return xylona.GameServerMetricsCollectionStatus_GAME_SERVER_METRICS_COLLECTION_STATUS_SERVER_OFFLINE
	}
	if snap.MetricsValid {
		return xylona.GameServerMetricsCollectionStatus_GAME_SERVER_METRICS_COLLECTION_STATUS_AVAILABLE
	}
	return xylona.GameServerMetricsCollectionStatus_GAME_SERVER_METRICS_COLLECTION_STATUS_WARMING_UP
}

// fetchNodeSnapshot wraps NodeClient.GetNodeSnapshot with a bounded timeout.
func fetchNodeSnapshot(ctx context.Context, client nodeclient.NodeClient) (*node.NodeSnapshot, error) {
	snapCtx, cancel := context.WithTimeout(ctx, nodeSnapshotFetchTimeout)
	defer cancel()
	snap, errSnap := client.GetNodeSnapshot(snapCtx)
	if errSnap != nil {
		return nil, fmt.Errorf("websocket: fetch node snapshot: %w", errSnap)
	}
	if snap == nil {
		return nil, errors.New("websocket: fetch node snapshot: node returned an empty snapshot")
	}
	return snap, nil
}

type nodeSnapshotResult struct {
	snapshot *node.NodeSnapshot
	err      error
}

// fetchNodeSnapshots fetches each distinct node concurrently so one or more
// unavailable nodes cost at most one bounded timeout per collection tick.
func fetchNodeSnapshots(
	ctx context.Context,
	clients map[string]nodeclient.NodeClient,
) map[string]nodeSnapshotResult {
	results := make(map[string]nodeSnapshotResult, len(clients))
	var resultsMutex sync.Mutex
	var waitGroup sync.WaitGroup

	for nodeID, client := range clients {
		waitGroup.Add(1)
		go func(nodeID string, client nodeclient.NodeClient) {
			defer waitGroup.Done()

			snapshot, errSnapshot := fetchNodeSnapshot(ctx, client)
			resultsMutex.Lock()
			results[nodeID] = nodeSnapshotResult{
				snapshot: snapshot,
				err:      errSnapshot,
			}
			resultsMutex.Unlock()
		}(nodeID, client)
	}

	waitGroup.Wait()
	return results
}

// resolveGameServerStatus returns the current xylona.Status for a game server,
// routing through the node registry so embedded and remote nodes behave the
// same. Only a successful lookup with no tracked process is authoritative
// OFFLINE; unavailable and malformed snapshots remain UNKNOWN.
func (ws *WebSocket) resolveGameServerStatus(gameServer *models.GameServer) xylona.Status {
	if ws.nodeRegistry == nil || gameServer == nil {
		return xylona.Status_UNKNOWN
	}
	client, errClient := ws.nodeRegistry.Get(gameServer.NodeID)
	if errClient != nil {
		return xylona.Status_UNKNOWN
	}
	ctx, cancel := context.WithTimeout(ws.ctx, processSnapshotFetchTimeout)
	defer cancel()
	snap, found, errSnap := client.GetProcessSnapshot(ctx, gameServer.ID)
	if errSnap != nil {
		return xylona.Status_UNKNOWN
	}
	if !found || snap == nil {
		return xylona.Status_OFFLINE
	}
	statusValue, ok := xylona.Status_value[snap.Status]
	if !ok {
		return xylona.Status_UNKNOWN
	}
	return xylona.Status(statusValue)
}

// queryEqual checks if two ServerQuery objects are equal. Used to save websocket traffic.
func queryEqual(x, y *xylona.ServerQuery) bool {
	if x.GetServerId() != y.GetServerId() || x.GetServerName() != y.GetServerName() || x.GetType() != y.GetType() {
		return false
	}
	switch x.GetType() {
	case xylona.ServerQuery_Minecraft:
		xm := x.GetMinecraft()
		ym := y.GetMinecraft()
		return xm.GetMotd() == ym.GetMotd() &&
			xm.GetGameType() == ym.GetGameType() &&
			xm.GetMap() == ym.GetMap() &&
			xm.GetNumberOfPlayers() == ym.GetNumberOfPlayers() &&
			xm.GetMaxPlayers() == ym.GetMaxPlayers() &&
			slices.Equal(xm.GetPlayerList(), ym.GetPlayerList()) &&
			xm.GetProtocolVersion() == ym.GetProtocolVersion() &&
			xm.GetServerVersion() == ym.GetServerVersion()
	case xylona.ServerQuery_Source:
		xs := x.GetSource()
		ys := y.GetSource()
		return xs.GetName() == ys.GetName() &&
			xs.GetMap() == ys.GetMap() &&
			xs.GetGame() == ys.GetGame() &&
			xs.GetAppId() == ys.GetAppId() &&
			xs.GetSteamId() == ys.GetSteamId() &&
			xs.GetGameId() == ys.GetGameId() &&
			xs.GetPlayers() == ys.GetPlayers() &&
			xs.GetMaxPlayers() == ys.GetMaxPlayers() &&
			xs.GetBots() == ys.GetBots() &&
			xs.GetServerOs() == ys.GetServerOs() &&
			xs.GetVisibility() == ys.GetVisibility() &&
			xs.GetVac() == ys.GetVac() &&
			xs.GetVersion() == ys.GetVersion() &&
			xs.GetProtocol() == ys.GetProtocol() &&
			slices.Equal(xs.GetPlayerList(), ys.GetPlayerList()) &&
			xs.GetPlayerListSupported() == ys.GetPlayerListSupported()
	case xylona.ServerQuery_Palworld:
		xp := x.GetPalworld()
		yp := y.GetPalworld()
		return xp.GetName() == yp.GetName() &&
			xp.GetDescription() == yp.GetDescription() &&
			xp.GetVersion() == yp.GetVersion() &&
			xp.GetWorldGuid() == yp.GetWorldGuid() &&
			xp.GetPlayers() == yp.GetPlayers() &&
			xp.GetMaxPlayers() == yp.GetMaxPlayers() &&
			slices.Equal(xp.GetPlayerList(), yp.GetPlayerList()) &&
			xp.GetUptimeSeconds() == yp.GetUptimeSeconds() &&
			xp.GetServerFps() == yp.GetServerFps() &&
			xp.GetServerFrameTimeMs() == yp.GetServerFrameTimeMs() &&
			xp.GetDays() == yp.GetDays() &&
			xp.GetResponded() == yp.GetResponded()
	}
	return false
}

// metricsEqual checks if two GameServerMetrics snapshots are equal enough to skip sending.
func metricsEqual(a, b *xylona.GameServerMetrics) bool {
	if a == nil || b == nil {
		return a == b
	}
	return int64(a.GetCpuPercent()) == int64(b.GetCpuPercent()) &&
		a.GetCpuValid() == b.GetCpuValid() &&
		a.GetMetricsValid() == b.GetMetricsValid() &&
		a.GetMemoryBytes() == b.GetMemoryBytes() &&
		a.GetMemoryWorkingSetBytes() == b.GetMemoryWorkingSetBytes() &&
		a.GetNumberOfThreads() == b.GetNumberOfThreads() &&
		a.GetDiskUsageBytes() == b.GetDiskUsageBytes() &&
		a.GetDiskValid() == b.GetDiskValid() &&
		a.GetDiskTotalBytes() == b.GetDiskTotalBytes() &&
		a.GetDiskFreeBytes() == b.GetDiskFreeBytes() &&
		a.GetUptimeSeconds() == b.GetUptimeSeconds() &&
		int64(a.GetIoReadRate()) == int64(b.GetIoReadRate()) &&
		int64(a.GetIoWriteRate()) == int64(b.GetIoWriteRate()) &&
		a.GetIoValid() == b.GetIoValid() &&
		a.GetConnectionCount() == b.GetConnectionCount() &&
		a.GetConnectionCountValid() == b.GetConnectionCountValid() &&
		a.GetProcessStatus() == b.GetProcessStatus() &&
		a.GetCollectionStatus() == b.GetCollectionStatus() &&
		timestampsEqual(a.GetCollectedAt(), b.GetCollectedAt())
}

func timestampsEqual(left, right *timestamppb.Timestamp) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.AsTime().Equal(right.AsTime())
}

// nodeSnapshotEqual checks if two NodeResourceSnapshot values are equal enough to skip sending.
func nodeSnapshotEqual(a, b *xylona.NodeResourceSnapshot) bool {
	if a == nil || b == nil {
		return a == b
	}
	return int64(a.GetCpuPercent()) == int64(b.GetCpuPercent()) &&
		a.GetMemoryUsedBytes() == b.GetMemoryUsedBytes() &&
		a.GetDiskUsedBytes() == b.GetDiskUsedBytes() &&
		a.GetGameServerCount() == b.GetGameServerCount() &&
		a.GetRunningGameServerCount() == b.GetRunningGameServerCount() &&
		a.GetUserCount() == b.GetUserCount()
}

type nodeMetricsLoopAction int

const (
	nodeMetricsLoopActionSend nodeMetricsLoopAction = iota
	nodeMetricsLoopActionSkip
	nodeMetricsLoopActionExit
)

func determineNodeMetricsLoopAction(conn *connection, errConnection error) nodeMetricsLoopAction {
	if errConnection != nil || conn == nil {
		return nodeMetricsLoopActionExit
	}
	if !conn.currentlySuperUser() {
		return nodeMetricsLoopActionSkip
	}
	return nodeMetricsLoopActionSend
}

func (ws *WebSocket) gameServerConnectionsWithAccess(serverID string) []*connection {
	ws.userWebsocketConnectionsLock.RLock()
	defer ws.userWebsocketConnectionsLock.RUnlock()

	connections := make([]*connection, 0)
	for _, userConnections := range ws.userWebsocketConnections {
		for _, conn := range userConnections {
			if !conn.hasGameServerAccess(serverID) {
				continue
			}
			connections = append(connections, conn)
		}
	}
	return connections
}

// collectAllNodeSnapshots fetches every registered node concurrently (with
// each request bounded by nodeSnapshotFetchTimeout) and returns a map of
// nodeID -> proto snapshot suitable for websocket broadcast. Errors are
// logged per-node; unreachable nodes are simply omitted.
func (ws *WebSocket) collectAllNodeSnapshots(ctx context.Context) map[string]*xylona.NodeResourceSnapshot {
	if ws.nodeRegistry == nil {
		return map[string]*xylona.NodeResourceSnapshot{}
	}
	clients := ws.nodeRegistry.List()
	out := make(map[string]*xylona.NodeResourceSnapshot, len(clients))

	userCount, errUsers := ws.db.CountUsers()
	if errUsers != nil {
		log.Warn().Err(errUsers).Msg("Failed to count users for websocket snapshot")
	}

	// Pre-compute game server counts per node so we do a single DB scan
	// instead of one per registered node.
	gsCountsByNode := make(map[string]int)
	allServers, errAll := ws.db.GetAllGameServers()
	if errAll != nil {
		log.Warn().Err(errAll).Msg("websocket: failed to enumerate game servers for snapshot")
	} else {
		for _, gs := range allServers {
			gsCountsByNode[gs.NodeID]++
		}
	}

	clientsByNode := make(map[string]nodeclient.NodeClient, len(clients))
	for _, client := range clients {
		clientsByNode[client.ID()] = client
	}
	for nodeID, result := range fetchNodeSnapshots(ctx, clientsByNode) {
		if result.err != nil {
			log.Debug().Err(result.err).Str("node_id", nodeID).
				Msg("websocket: GetNodeSnapshot failed")
			continue
		}
		out[nodeID] = buildNodeResourceSnapshot(result.snapshot, gsCountsByNode[nodeID], userCount)
	}
	return out
}

// sendNodeMetrics sends node resource metrics to a superuser connection every
// 5 seconds. Snapshots are fetched directly through the node registry so
// embedded and remote nodes render identically.
func (ws *WebSocket) sendNodeMetrics(s *melody.Session) {
	previousSnapshots := make(map[string]*xylona.NodeResourceSnapshot)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ws.ctx.Done():
			return
		case <-s.Request.Context().Done():
			return
		case <-ticker.C:
			if s.IsClosed() {
				return
			}
			conn, errConnection := ws.getSessionConnection(s)
			if errConnection != nil {
				log.Error().Err(errConnection).Msg("Failed to get websocket connection for node metrics")
				return
			}
			switch determineNodeMetricsLoopAction(conn, nil) {
			case nodeMetricsLoopActionExit:
				return
			case nodeMetricsLoopActionSkip:
				continue
			}

			snapshots := ws.collectAllNodeSnapshots(ws.ctx)
			allNodeMetrics := &xylona.AllNodeMetrics{Nodes: snapshots}

			// Detect changes (add/drop/modified) so we can skip no-op sends.
			changed := len(snapshots) != len(previousSnapshots)
			if !changed {
				for nodeID, snap := range snapshots {
					prev, exists := previousSnapshots[nodeID]
					if !exists || !nodeSnapshotEqual(prev, snap) {
						changed = true
						break
					}
				}
			}
			previousSnapshots = snapshots

			if !changed {
				continue
			}

			out := &xylona.Message{
				Type:           xylona.Message_NodeMetrics,
				AllNodeMetrics: allNodeMetrics,
			}
			byteOut, errMarshal := protojson.Marshal(out)
			if errMarshal != nil {
				log.Error().Err(errMarshal).Msg("Failed to marshal node metrics")
				continue
			}
			errWrite := s.Write(byteOut)
			if errWrite != nil {
				log.Error().Err(errWrite).Msg("Failed to write node metrics websocket message")
				return
			}
		}
	}
}

// collectOwnedServerMetrics batches per-node GetNodeSnapshot calls so we do at
// most one round-trip per distinct node per tick, then extracts the snapshot
// for each game server the session owns. Returns a map keyed by game server
// ID; missing servers simply don't appear in the result.
func (ws *WebSocket) collectOwnedServerMetrics(ctx context.Context, gameServers []*models.GameServer) map[string]*xylona.GameServerMetrics {
	if ws.nodeRegistry == nil || len(gameServers) == 0 {
		return map[string]*xylona.GameServerMetrics{}
	}

	// Group the session's servers by node so we fetch each node snapshot once.
	serversByNode := make(map[string][]*models.GameServer, len(gameServers))
	for _, gs := range gameServers {
		serversByNode[gs.NodeID] = append(serversByNode[gs.NodeID], gs)
	}

	clientsByNode := make(map[string]nodeclient.NodeClient, len(serversByNode))
	for nodeID := range serversByNode {
		client, errClient := ws.nodeRegistry.Get(nodeID)
		if errClient != nil {
			// Node not currently reachable; leave these servers out of this tick.
			continue
		}
		clientsByNode[nodeID] = client
	}

	out := make(map[string]*xylona.GameServerMetrics, len(gameServers))
	for nodeID, result := range fetchNodeSnapshots(ctx, clientsByNode) {
		if result.err != nil {
			log.Debug().Err(result.err).Str("node_id", nodeID).
				Msg("websocket: GetNodeSnapshot for owned-server metrics failed")
			continue
		}
		processByID := make(map[string]*node.ProcessSnapshot, len(result.snapshot.Processes))
		for i := range result.snapshot.Processes {
			processByID[result.snapshot.Processes[i].ID] = &result.snapshot.Processes[i]
		}
		for _, gs := range serversByNode[nodeID] {
			ps, exists := processByID[gs.ID]
			if !exists {
				continue
			}
			metrics := processSnapshotToGameServerMetrics(ps, result.snapshot.Collected)
			if !ps.DiskMeasuredAt.IsZero() {
				metrics.DiskMeasuredAt = timestamppb.New(ps.DiskMeasuredAt)
			}
			out[gs.ID] = metrics
		}
	}
	return out
}

// collectSubscribedServerMetrics limits both node snapshot work and the
// outbound payload to servers this connection subscribed to and can access.
// A nil result means the connection has no eligible subscriptions.
func (ws *WebSocket) collectSubscribedServerMetrics(
	ctx context.Context,
	conn *connection,
	gameServers []*models.GameServer,
) *xylona.AllServersMetrics {
	subscribedServers := make([]*models.GameServer, 0, len(gameServers))
	for _, gameServer := range gameServers {
		if gameServer == nil || !conn.shouldReceiveMetrics(gameServer.ID) {
			continue
		}
		subscribedServers = append(subscribedServers, gameServer)
	}
	if len(subscribedServers) == 0 {
		return nil
	}

	collected := ws.collectOwnedServerMetrics(ctx, subscribedServers)
	return &xylona.AllServersMetrics{Servers: collected}
}

func (ws *WebSocket) sendOwnedServersMetrics(s *melody.Session) {
	previousMetricsMap := make(map[string]*xylona.GameServerMetrics)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ws.ctx.Done():
			return
		case <-s.Request.Context().Done():
			return
		case <-ticker.C:
			if s.IsClosed() {
				return
			}
			conn, errConnection := ws.getSessionConnection(s)
			if errConnection != nil {
				log.Error().Err(errConnection).Msg("Failed to get websocket connection for metrics")
				return
			}
			gameServers, errGetServers := ws.getSessionGameServers(s)
			if errGetServers != nil {
				log.Error().Err(errGetServers).Msg("Failed to get game servers from session for metrics")
				return
			}
			allMetrics := ws.collectSubscribedServerMetrics(ws.ctx, conn, gameServers)
			if allMetrics == nil {
				continue
			}
			metricsChanged := false
			for serverID, current := range allMetrics.GetServers() {
				previous, seen := previousMetricsMap[serverID]
				if !seen || !metricsEqual(previous, current) {
					previousMetricsMap[serverID] = current
					metricsChanged = true
				}
			}

			if !metricsChanged {
				continue
			}
			out := &xylona.Message{
				Type:              xylona.Message_GameServerMetrics,
				AllServersMetrics: allMetrics,
			}
			byteOut, errMarshal := protojson.Marshal(out)
			if errMarshal != nil {
				log.Error().Err(errMarshal).Msg("Failed to marshal game server metrics")
				continue
			}
			errWrite := s.Write(byteOut)
			if errWrite != nil {
				log.Error().Err(errWrite).Msg("Failed to write metrics websocket message")
				return
			}
		}
	}
}

func (ws *WebSocket) sendOwnedServersQueryInfo(s *melody.Session) {
	// Map of previous queries for each game server.
	previousQueryMap := make(map[string]*xylona.ServerQuery)
	throttle := time.NewTicker(time.Second * 1)
	defer throttle.Stop()
	for {
		select {
		case <-ws.ctx.Done():
			return
		case <-s.Request.Context().Done():
			return
		case <-throttle.C:
			if s.IsClosed() {
				return
			}
			allQueryInfo := ws.actions.GetServerQueries()
			ownedServerQueryInfo := &xylona.AllServersQueryInfo{Servers: make(map[string]*xylona.ServerQuery)}
			serverQueriesInfoChanged := false
			gameServers, err := ws.getSessionGameServers(s)
			if err != nil {
				log.Error().Err(err).Msg("Failed to get game servers from session")
				return
			}
			for _, gameServer := range gameServers {
				// Get the current query info for the game server.
				serverQuery, exists := allQueryInfo.GetServers()[gameServer.ID]
				if !exists {
					continue
				}
				// Get the previous query, so we can see if anything has changed.
				previousQuery, exists := previousQueryMap[gameServer.ID]

				// If the previous query doesn't exist, or the current query is different from the previous query, update the previous query map.
				if !exists || !queryEqual(previousQuery, serverQuery) {
					// Update the previous query map.
					previousQueryMap[gameServer.ID] = serverQuery
					serverQueriesInfoChanged = true
				}
				ownedServerQueryInfo.Servers[gameServer.ID] = serverQuery
			}

			// If the server queries info hasn't changed, skip sending the update. This should cut down on traffic significantly.
			if !serverQueriesInfoChanged {
				continue
			}

			// Send the update to the user.
			out := &xylona.Message{
				Type:                xylona.Message_ServerQueries,
				AllServersQueryInfo: ownedServerQueryInfo,
			}
			byteOut, errMarshal := protojson.Marshal(out)
			if errMarshal != nil {
				log.Error().Err(errMarshal).Msg("Failed to marshal game server queries")
				continue
			}
			errWrite := s.Write(byteOut)
			if errWrite != nil {
				log.Error().Err(errWrite).Msg("Failed to write websocket message")
				return
			}
		}
	}
}

func (ws *WebSocket) sendUserGameServerStatus(s *melody.Session, gameServer *models.GameServer) {
	status := ws.resolveGameServerStatus(gameServer)
	out := &xylona.Message{
		Type: xylona.Message_GameServerStatus,
		GameServerStatusUpdate: &xylona.GameServerStatusUpdate{
			GameServerId:   gameServer.ID,
			Status:         status,
			GameServerName: gameServer.Name,
		},
	}
	byteOut, errMarshal := protojson.Marshal(out)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Msg("Failed to marshal game server status update")
		return
	}
	errWrite := s.Write(byteOut)
	if errWrite != nil {
		log.Error().Err(errWrite).Msg("Failed to write websocket message")
		return
	}
}

func (ws *WebSocket) broadcastGameServerStatus(serverID string, serverName string, status xylona.Status) {
	out := &xylona.Message{
		Type: xylona.Message_GameServerStatus,
		GameServerStatusUpdate: &xylona.GameServerStatusUpdate{
			GameServerId:   serverID,
			Status:         status,
			GameServerName: serverName,
		},
	}
	byteOut, errMarshal := protojson.Marshal(out)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Msg("Failed to marshal game server status update")
		return
	}

	for _, conn := range ws.gameServerConnectionsWithAccess(serverID) {
		if conn.melodySession.IsClosed() {
			continue
		}
		errWrite := conn.melodySession.Write(byteOut)
		if errWrite != nil {
			log.Debug().Err(errWrite).Msg("Failed to write game server status update to WebSocket")
		}
	}
}

// BroadcastRemoteServerStatus sends a status update for a remote server to all connected WebSocket clients.
func (ws *WebSocket) BroadcastRemoteServerStatus(serverID string, serverName string, status xylona.Status) {
	ws.broadcastGameServerStatus(serverID, serverName, status)
}

// BroadcastRemoteServerMetrics sends a remote metrics update to subscribed clients.
func (ws *WebSocket) BroadcastRemoteServerMetrics(serverID string, metrics *xylona.GameServerMetrics) {
	out := &xylona.Message{
		Type: xylona.Message_GameServerMetrics,
		AllServersMetrics: &xylona.AllServersMetrics{
			Servers: map[string]*xylona.GameServerMetrics{
				serverID: metrics,
			},
		},
	}
	byteOut, errMarshal := protojson.Marshal(out)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Msg("Failed to marshal remote server metrics update")
		return
	}

	ws.userWebsocketConnectionsLock.RLock()
	defer ws.userWebsocketConnectionsLock.RUnlock()

	for _, userConnections := range ws.userWebsocketConnections {
		for _, conn := range userConnections {
			if conn.melodySession.IsClosed() {
				continue
			}
			if !conn.shouldReceiveMetrics(serverID) {
				continue
			}
			errWrite := conn.melodySession.Write(byteOut)
			if errWrite != nil {
				log.Debug().Err(errWrite).Msg("Failed to write remote metrics update to WebSocket")
			}
		}
	}
}

// BroadcastGameServerVersion sends refreshed version data to all connected WebSocket clients
// that have access to the given server.
func (ws *WebSocket) BroadcastGameServerVersion(serverID string, version string, versionInfo *xylona.VersionInfo) {
	out := &xylona.Message{
		Type: xylona.Message_GameServerVersion,
		GameServerVersionUpdate: &xylona.GameServerVersionUpdate{
			GameServerId: serverID,
			Version:      version,
			VersionInfo:  versionInfo,
		},
	}
	byteOut, errMarshal := protojson.Marshal(out)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Msg("Failed to marshal game server version update")
		return
	}

	for _, conn := range ws.gameServerConnectionsWithAccess(serverID) {
		if conn.melodySession.IsClosed() {
			continue
		}
		errWrite := conn.melodySession.Write(byteOut)
		if errWrite != nil {
			log.Debug().Err(errWrite).Msg("Failed to write version update to WebSocket")
		}
	}
}

// BroadcastUpdateProgress sends a game server update progress event to all
// connected WebSocket clients that have access to the given server.
func (ws *WebSocket) BroadcastUpdateProgress(
	serverID string,
	serverName string,
	step xylona.UpdateStep,
	stepStatus xylona.StepStatus,
	message string,
) {
	out := &xylona.Message{
		Type: xylona.Message_GameServerUpdateProgress,
		UpdateProgress: &xylona.UpdateProgress{
			GameServerId:   serverID,
			Step:           step,
			StepStatus:     stepStatus,
			Message:        message,
			GameServerName: serverName,
		},
	}
	byteOut, errMarshal := protojson.Marshal(out)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Msg("Failed to marshal update progress message")
		return
	}
	for _, conn := range ws.gameServerConnectionsWithAccess(serverID) {
		if conn.melodySession.IsClosed() {
			continue
		}
		errWrite := conn.melodySession.Write(byteOut)
		if errWrite != nil {
			log.Debug().Err(errWrite).Msg("Failed to write update progress to WebSocket")
		}
	}
}

// BroadcastBackupProgress sends a backup progress event to all connected WebSocket clients
// that have access to the given server.
func (ws *WebSocket) BroadcastBackupProgress(serverID string, progress *xylona.BackupProgress) {
	if progress == nil {
		return
	}

	out := &xylona.Message{
		Type:           xylona.Message_GameServerBackupProgress,
		BackupProgress: progress,
	}
	byteOut, errMarshal := protojson.Marshal(out)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Msg("Failed to marshal backup progress message")
		return
	}

	for _, conn := range ws.gameServerConnectionsWithAccess(serverID) {
		if conn.melodySession.IsClosed() {
			continue
		}
		errWrite := conn.melodySession.Write(byteOut)
		if errWrite != nil {
			log.Debug().Err(errWrite).Msg("Failed to write backup progress update to WebSocket")
		}
	}
}

// BroadcastSystemUpdateProgress sends controller/node update progress to all
// connected superusers.
func (ws *WebSocket) BroadcastSystemUpdateProgress(progress *xylona.SystemUpdateProgress) {
	if progress == nil {
		return
	}

	out := &xylona.Message{
		Type:                 xylona.Message_SystemUpdateProgress,
		SystemUpdateProgress: progress,
	}
	byteOut, errMarshal := protojson.Marshal(out)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Msg("Failed to marshal system update progress message")
		return
	}

	ws.userWebsocketConnectionsLock.RLock()
	defer ws.userWebsocketConnectionsLock.RUnlock()

	for _, userConnections := range ws.userWebsocketConnections {
		for _, conn := range userConnections {
			if conn.melodySession.IsClosed() {
				continue
			}
			if !conn.currentlySuperUser() {
				continue
			}
			errWrite := conn.melodySession.Write(byteOut)
			if errWrite != nil {
				log.Debug().Err(errWrite).Msg("Failed to write system update progress to WebSocket")
			}
		}
	}
}

// BroadcastServerSoftwareInstall sends a server software install status update
// to all connected WebSocket clients that have access to the given server.
func (ws *WebSocket) BroadcastServerSoftwareInstall(
	serverID string,
	serverName string,
	status string,
	softwareID string,
	errMsg string,
) {
	out := &xylona.Message{
		Type: xylona.Message_ServerSoftwareInstall,
		ServerSoftwareInstallUpdate: &xylona.ServerSoftwareInstallUpdate{
			GameServerId:   serverID,
			Status:         status,
			Error:          errMsg,
			SoftwareId:     softwareID,
			GameServerName: serverName,
		},
	}
	byteOut, errMarshal := protojson.Marshal(out)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Msg("Failed to marshal server software install update")
		return
	}

	for _, conn := range ws.gameServerConnectionsWithAccess(serverID) {
		if conn.melodySession.IsClosed() {
			continue
		}
		errWrite := conn.melodySession.Write(byteOut)
		if errWrite != nil {
			log.Debug().Err(errWrite).Msg("Failed to write software install update to WebSocket")
		}
	}
}
