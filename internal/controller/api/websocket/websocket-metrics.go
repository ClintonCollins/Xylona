package websocket

import (
	"context"
	"fmt"
	"slices"
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
func processSnapshotToGameServerMetrics(snap *node.ProcessSnapshot) *xylona.GameServerMetrics {
	if snap == nil {
		return nil
	}
	var uptimeSeconds int64
	if snap.UnixStartedAt > 0 {
		uptimeSeconds = time.Now().Unix() - snap.UnixStartedAt
	}
	return &xylona.GameServerMetrics{
		CpuPercent:            snap.CPUPercent,
		MemoryBytes:           helpers.ClampInt64FromUint64(snap.MemoryVMS),
		MemoryWorkingSetBytes: helpers.ClampInt64FromUint64(snap.MemoryRSS),
		MemoryPercent:         float64(snap.MemoryPercent),
		CpuCores:              snap.CPUCores,
		NumberOfThreads:       snap.NumThreads,
		DiskUsageBytes:        helpers.ClampInt64FromUint64(snap.DiskUsageBytes),
		IoReadRate:            snap.IOReadRate,
		IoWriteRate:           snap.IOWriteRate,
		ConnectionCount:       snap.ConnectionCount,
		UptimeSeconds:         uptimeSeconds,
	}
}

// fetchNodeSnapshot wraps NodeClient.GetNodeSnapshot with a bounded timeout.
func fetchNodeSnapshot(ctx context.Context, client nodeclient.NodeClient) (*node.NodeSnapshot, error) {
	snapCtx, cancel := context.WithTimeout(ctx, nodeSnapshotFetchTimeout)
	defer cancel()
	snap, errSnap := client.GetNodeSnapshot(snapCtx)
	if errSnap != nil {
		return nil, fmt.Errorf("websocket: fetch node snapshot: %w", errSnap)
	}
	return snap, nil
}

// resolveGameServerStatus returns the current xylona.Status for a game server,
// routing through the node registry so embedded and remote nodes behave the
// same. Falls back to OFFLINE when the node is unreachable or the process is
// not currently tracked.
func (ws *WebSocket) resolveGameServerStatus(gameServer *models.GameServer) xylona.Status {
	if ws.nodeRegistry == nil || gameServer == nil {
		return xylona.Status_OFFLINE
	}
	client, errClient := ws.nodeRegistry.Get(gameServer.NodeID)
	if errClient != nil {
		return xylona.Status_OFFLINE
	}
	ctx, cancel := context.WithTimeout(ws.ctx, processSnapshotFetchTimeout)
	defer cancel()
	snap, found, errSnap := client.GetProcessSnapshot(ctx, gameServer.ID)
	if errSnap != nil || !found || snap == nil {
		return xylona.Status_OFFLINE
	}
	statusValue, ok := xylona.Status_value[snap.Status]
	if !ok {
		return xylona.Status_OFFLINE
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
			xs.GetProtocol() == ys.GetProtocol()
	}
	return false
}

// metricsEqual checks if two GameServerMetrics snapshots are equal enough to skip sending.
func metricsEqual(a, b *xylona.GameServerMetrics) bool {
	if a == nil || b == nil {
		return a == b
	}
	return int64(a.GetCpuPercent()) == int64(b.GetCpuPercent()) &&
		a.GetMemoryBytes() == b.GetMemoryBytes() &&
		a.GetMemoryWorkingSetBytes() == b.GetMemoryWorkingSetBytes() &&
		a.GetNumberOfThreads() == b.GetNumberOfThreads() &&
		a.GetDiskUsageBytes() == b.GetDiskUsageBytes() &&
		a.GetUptimeSeconds() == b.GetUptimeSeconds() &&
		int64(a.GetIoReadRate()) == int64(b.GetIoReadRate()) &&
		int64(a.GetIoWriteRate()) == int64(b.GetIoWriteRate()) &&
		a.GetConnectionCount() == b.GetConnectionCount()
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

// collectAllNodeSnapshots iterates the node registry, calls GetNodeSnapshot
// on each client (each request bounded by nodeSnapshotFetchTimeout), and
// returns a map of nodeID -> proto snapshot suitable for websocket broadcast.
// Errors are logged per-node; unreachable nodes are simply omitted.
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

	for _, client := range clients {
		nodeID := client.ID()
		snap, errSnap := fetchNodeSnapshot(ctx, client)
		if errSnap != nil {
			log.Debug().Err(errSnap).Str("node_id", nodeID).
				Msg("websocket: GetNodeSnapshot failed")
			continue
		}
		out[nodeID] = buildNodeResourceSnapshot(snap, gsCountsByNode[nodeID], userCount)
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

	out := make(map[string]*xylona.GameServerMetrics, len(gameServers))
	for nodeID, nodeServers := range serversByNode {
		client, errClient := ws.nodeRegistry.Get(nodeID)
		if errClient != nil {
			// Node not currently reachable; leave these servers out of this tick.
			continue
		}
		snap, errSnap := fetchNodeSnapshot(ctx, client)
		if errSnap != nil {
			log.Debug().Err(errSnap).Str("node_id", nodeID).
				Msg("websocket: GetNodeSnapshot for owned-server metrics failed")
			continue
		}
		processByID := make(map[string]*node.ProcessSnapshot, len(snap.Processes))
		for i := range snap.Processes {
			processByID[snap.Processes[i].ID] = &snap.Processes[i]
		}
		for _, gs := range nodeServers {
			ps, exists := processByID[gs.ID]
			if !exists {
				continue
			}
			out[gs.ID] = processSnapshotToGameServerMetrics(ps)
		}
	}
	return out
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
			gameServers, errGetServers := ws.getSessionGameServers(s)
			if errGetServers != nil {
				log.Error().Err(errGetServers).Msg("Failed to get game servers from session for metrics")
				return
			}
			collected := ws.collectOwnedServerMetrics(ws.ctx, gameServers)
			allMetrics := &xylona.AllServersMetrics{Servers: make(map[string]*xylona.GameServerMetrics, len(collected))}
			metricsChanged := false
			for _, gameServer := range gameServers {
				current, exists := collected[gameServer.ID]
				if !exists {
					continue
				}
				previous, seen := previousMetricsMap[gameServer.ID]
				if !seen || !metricsEqual(previous, current) {
					previousMetricsMap[gameServer.ID] = current
					metricsChanged = true
				}
				allMetrics.Servers[gameServer.ID] = current
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
