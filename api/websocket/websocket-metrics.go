package websocket

import (
	"slices"
	"time"

	"github.com/olahol/melody"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

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

// sendNodeMetrics sends node resource metrics to a superuser connection every 5 seconds.
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

			localNodeID, errLocalID := ws.db.GetLocalNodeID()
			if errLocalID != nil {
				log.Error().Err(errLocalID).Msg("Failed to get local node ID for node metrics")
				continue
			}

			allNodeMetrics := &xylona.AllNodeMetrics{Nodes: make(map[string]*xylona.NodeResourceSnapshot)}
			changed := false

			// Collect local node snapshot.
			localSnap := ws.collectLocalNodeSnapshot()
			if localSnap != nil {
				prev, exists := previousSnapshots[localNodeID]
				if !exists || !nodeSnapshotEqual(prev, localSnap) {
					changed = true
				}
				previousSnapshots[localNodeID] = localSnap
				allNodeMetrics.Nodes[localNodeID] = localSnap
			}

			// Merge remote node snapshots from cache.
			ws.remoteNodeSnapshotCacheLock.RLock()
			for nodeID, snap := range ws.remoteNodeSnapshotCache {
				prev, exists := previousSnapshots[nodeID]
				if !exists || !nodeSnapshotEqual(prev, snap) {
					changed = true
				}
				previousSnapshots[nodeID] = snap
				allNodeMetrics.Nodes[nodeID] = snap
			}
			ws.remoteNodeSnapshotCacheLock.RUnlock()

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
			allMetrics := &xylona.AllServersMetrics{Servers: make(map[string]*xylona.GameServerMetrics)}
			metricsChanged := false
			for _, gameServer := range gameServers {
				cmd, errGetCmd := ws.supervisor.GetCommandByID(gameServer.ID)
				if errGetCmd != nil {
					continue
				}
				cpuPct, memRSS, memVMS, memPct, cpuCores, threads, diskBytes, ioRead, ioWrite, connCount := cmd.Metrics()
				startedAt := cmd.UnixStartedAt()
				var uptimeSeconds int64
				if startedAt > 0 {
					uptimeSeconds = time.Now().Unix() - startedAt
				}
				current := &xylona.GameServerMetrics{
					CpuPercent:            cpuPct,
					MemoryBytes:           helpers.ClampInt64FromUint64(memVMS),
					MemoryWorkingSetBytes: helpers.ClampInt64FromUint64(memRSS),
					MemoryPercent:         float64(memPct),
					CpuCores:              cpuCores,
					NumberOfThreads:       threads,
					DiskUsageBytes:        helpers.ClampInt64FromUint64(diskBytes),
					IoReadRate:            ioRead,
					IoWriteRate:           ioWrite,
					ConnectionCount:       connCount,
					UptimeSeconds:         uptimeSeconds,
				}
				previous, exists := previousMetricsMap[gameServer.ID]
				if !exists || !metricsEqual(previous, current) {
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
	command, errGetCommand := ws.supervisor.GetCommandByID(gameServer.ID)
	if errGetCommand != nil {
		return
	}
	status := command.Status()
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
