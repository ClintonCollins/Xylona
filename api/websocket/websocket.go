package websocket

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/gorilla/securecookie"
	"github.com/olahol/melody"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/api/federation"
	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/pkg/sysinfo"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

const (
	sessionKeyUserID        = "userID"
	sessionKeyStreamChannel = "streamChannel"
	sessionKeyUserName      = "userName"
	sessionKeyConnectionID  = "connectionID"
	sessionKeyGamesServers  = "gameServers"
	sessionKeySuperUser     = "superUser"
)

type connection struct {
	id                           uuid.UUID
	melodySession                *melody.Session
	outputStreamChannel          chan *xylona.Message
	allGameServerIDs             []string
	requestedGameServerOutputIDs map[string]struct{}
	subscribedMetricsServerIDs   map[string]struct{}
	remoteConsoleCancels         map[string]context.CancelFunc
	*sync.RWMutex
}

// shouldReceiveMetrics returns true if the connection is subscribed to metrics for the given server ID.
func (c *connection) shouldReceiveMetrics(serverID string) bool {
	c.RLock()
	defer c.RUnlock()
	_, ok := c.subscribedMetricsServerIDs[serverID]
	return ok
}

// hasGameServerAccess returns true if the connection has access to the given game server ID.
func (c *connection) hasGameServerAccess(serverID string) bool {
	c.RLock()
	defer c.RUnlock()
	for _, id := range c.allGameServerIDs {
		if id == serverID {
			return true
		}
	}
	return false
}

type WebSocket struct {
	melody                       *melody.Melody
	supervisor                   *supervisor.Instance
	actions                      *actions.Instance
	db                           *db.Connection
	federationMTLS               *helpers.FederationMTLS
	secureCookie                 *securecookie.SecureCookie
	ctx                          context.Context
	userWebsocketConnections     map[string]map[uuid.UUID]*connection // map[userID]map[connectionID]*connection
	userWebsocketConnectionsLock *sync.RWMutex
	sessionLock                  *sync.RWMutex
	remoteNodeSnapshotCache      map[string]*xylona.NodeResourceSnapshot // keyed by remote node ID
	remoteNodeSnapshotCacheLock  sync.RWMutex
}

func getSessionUsername(s *melody.Session) (string, error) {
	u, usernameExists := s.Get(sessionKeyUserName)
	if !usernameExists {
		return "", fmt.Errorf("failed to get username from session")
	}
	username := u.(string)
	return username, nil
}

func getSessionUserID(s *melody.Session) (string, error) {
	u, userIDExists := s.Get(sessionKeyUserID)
	if !userIDExists {
		return "", fmt.Errorf("failed to get user ID from session")
	}
	userID := u.(string)
	return userID, nil
}

func getSessionConnectionID(s *melody.Session) (uuid.UUID, error) {
	c, connectionIDExists := s.Get(sessionKeyConnectionID)
	if !connectionIDExists {
		return uuid.Nil, fmt.Errorf("failed to get connection ID from session")
	}
	connectionID := c.(uuid.UUID)
	return connectionID, nil
}

func (ws *WebSocket) getSessionGameServers(s *melody.Session) ([]*models.GameServer, error) {
	ws.sessionLock.RLock()
	defer ws.sessionLock.RUnlock()
	gs, gameServersExists := s.Get(sessionKeyGamesServers)
	if !gameServersExists {
		return nil, fmt.Errorf("failed to get game servers from session")
	}
	gameServers := gs.([]*models.GameServer)
	return gameServers, nil
}

func (ws *WebSocket) setSessionGameServers(s *melody.Session, gameServers []*models.GameServer) {
	ws.sessionLock.Lock()
	defer ws.sessionLock.Unlock()
	s.Set(sessionKeyGamesServers, gameServers)
}

func (ws *WebSocket) getSessionConnection(s *melody.Session) (*connection, error) {
	ws.userWebsocketConnectionsLock.RLock()
	defer ws.userWebsocketConnectionsLock.RUnlock()
	userID, errGetUserID := getSessionUserID(s)
	if errGetUserID != nil {
		log.Error().Msg("Failed to get user ID from session")
		return nil, errGetUserID
	}
	_, userExists := ws.userWebsocketConnections[userID]
	if !userExists {
		// log.Error().Str("User", userID).Msg("User not found in websocket connections")
		return nil, errors.New("user not found")
	}

	connectionID, errGetConnectionID := getSessionConnectionID(s)
	if errGetConnectionID != nil {
		log.Error().Msg("Failed to get connection ID from session")
		return nil, errGetConnectionID
	}

	sessionConnection, connectionExists := ws.userWebsocketConnections[userID][connectionID]
	if !connectionExists {
		log.Error().Str("User", userID).Msg("Connection not found in websocket connections")
		return nil, errors.New("connection not found")
	}
	return sessionConnection, nil
}

func NewInstance(
	ctx context.Context,
	supervisorInst *supervisor.Instance,
	actionsInst *actions.Instance,
	db *db.Connection,
	secureCookie *securecookie.SecureCookie,
	federationMTLS *helpers.FederationMTLS,
) (*WebSocket, http.HandlerFunc) {
	m := melody.New()
	inst := &WebSocket{
		melody:                       m,
		supervisor:                   supervisorInst,
		actions:                      actionsInst,
		db:                           db,
		federationMTLS:               federationMTLS,
		ctx:                          ctx,
		secureCookie:                 secureCookie,
		userWebsocketConnections:     make(map[string]map[uuid.UUID]*connection),
		userWebsocketConnectionsLock: &sync.RWMutex{},
		sessionLock:                  &sync.RWMutex{},
		remoteNodeSnapshotCache:      make(map[string]*xylona.NodeResourceSnapshot),
	}
	m.HandleConnect(inst.handleConnect)
	m.HandleDisconnect(inst.handleDisconnect)
	m.HandleMessage(inst.handleMessage)
	go inst.startRemoteNodeMetricsPoller()
	return inst, inst.handleRequest
}

func (ws *WebSocket) handleRequest(w http.ResponseWriter, r *http.Request) {
	err := ws.melody.HandleRequest(w, r)
	if err != nil {
		log.Error().Err(err).Msg("Failed to handle websocket request")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (ws *WebSocket) handleConnect(s *melody.Session) {
	log.Debug().Msg("Websocket connected")
	sessionCookies, errGetSession := gatekeeper.GetSessionFromCookies(s.Request.Cookies())
	if errGetSession != nil {
		log.Error().Err(errGetSession).Msg("Failed to get session from cookies")
		_ = s.CloseWithMsg([]byte("Unauthorized"))
		return
	}

	log.Debug().Str("SessionID", sessionCookies.SessionID).Msg("Got session from cookies")
	user, errGetUser := gatekeeper.GetUserFromSession(sessionCookies.SessionID, sessionCookies.SessionToken,
		ws.db, ws.secureCookie)
	if errGetUser != nil {
		log.Error().Err(errGetUser).Msg("Failed to get user from session")
		_ = s.CloseWithMsg([]byte("Unauthorized"))
		return
	}

	log.Debug().Str("User", user.UserName).Msg("Websocket connected")
	wsConnection := &connection{
		id:                           uuid.New(),
		melodySession:                s,
		outputStreamChannel:          make(chan *xylona.Message),
		allGameServerIDs:             []string{},
		requestedGameServerOutputIDs: make(map[string]struct{}),
		subscribedMetricsServerIDs:   make(map[string]struct{}),
		remoteConsoleCancels:         make(map[string]context.CancelFunc),
		RWMutex:                      &sync.RWMutex{},
	}

	s.Set(sessionKeyConnectionID, wsConnection.id)
	s.Set(sessionKeyUserID, user.ID)
	s.Set(sessionKeyUserName, user.UserName)
	s.Set(sessionKeyStreamChannel, wsConnection.outputStreamChannel)
	s.Set(sessionKeySuperUser, user.SuperUser)

	ws.userWebsocketConnectionsLock.Lock()
	_, userExists := ws.userWebsocketConnections[user.ID]
	if !userExists {
		ws.userWebsocketConnections[user.ID] = make(map[uuid.UUID]*connection)
	}
	ws.userWebsocketConnections[user.ID][wsConnection.id] = wsConnection
	ws.userWebsocketConnectionsLock.Unlock()

	gameServers, errGetServers := ws.db.GetGameServersByUser(user.ID)
	if errGetServers != nil {
		log.Error().Err(errGetServers).Msg("Failed to get game servers by user")
		return
	}

	wsConnection.allGameServerIDs = make([]string, len(gameServers))
	for i, gameServer := range gameServers {
		wsConnection.allGameServerIDs[i] = gameServer.ID
	}

	ws.setSessionGameServers(s, gameServers)
	for _, gameServer := range gameServers {
		go ws.sendUserGameServerStatus(s, gameServer)
		go ws.subscribeUserToOwnedGameServerNotifications(s, gameServer)
	}

	go ws.handleUserWebsocketConnection(s, user, wsConnection.outputStreamChannel)
	go ws.sendOwnedServersQueryInfo(s)
	go ws.sendOwnedServersMetrics(s)

	if user.SuperUser {
		go ws.sendNodeMetrics(s)
	}

	// Listen for game server added and removed events.
	go ws.listenForGameServersAdded(s)
	go ws.listenForGameServersRemoved(s)
}

func (ws *WebSocket) listenForGameServersAdded(s *melody.Session) {
	eb := eventbus.Get()
	serverRemoved := eb.Subscribe(eventbus.TopicGameServerRemoved)
	defer eb.Unsubscribe(eventbus.TopicGameServerRemoved, serverRemoved)
	for {
		select {
		case <-ws.ctx.Done():
			return
		case <-s.Request.Context().Done():
			return
		case data := <-serverRemoved:
			gameServer, ok := data.(*models.GameServer)
			if !ok {
				log.Error().Msg("Failed to cast data to game server")
				continue
			}
			gameServers, err := ws.getSessionGameServers(s)
			if err != nil {
				log.Error().Err(err).Msg("Failed to get game servers from session")
				return
			}
			for i, gs := range gameServers {
				if gs.ID == gameServer.ID {
					gameServers = append(gameServers[:i], gameServers[i+1:]...)
					break
				}
			}
			ws.setSessionGameServers(s, gameServers)
		}
	}
}

func (ws *WebSocket) listenForGameServersRemoved(s *melody.Session) {
	eb := eventbus.Get()
	serverCreated := eb.Subscribe(eventbus.TopicGameServerCreated)
	defer eb.Unsubscribe(eventbus.TopicGameServerCreated, serverCreated)
	for {
		select {
		case <-ws.ctx.Done():
			return
		case <-s.Request.Context().Done():
			return
		case data := <-serverCreated:
			gameServer, ok := data.(*models.GameServer)
			if !ok {
				log.Error().Msg("Failed to cast data to game server")
				continue
			}
			go ws.sendUserGameServerStatus(s, gameServer)
			go ws.subscribeUserToOwnedGameServerNotifications(s, gameServer)
			gameServers, err := ws.getSessionGameServers(s)
			if err != nil {
				log.Error().Err(err).Msg("Failed to get game servers from session")
				return
			}
			gameServers = append(gameServers, gameServer)
			ws.setSessionGameServers(s, gameServers)
		}
	}
}

// queryEqual checks if two ServerQuery objects are equal. Used to save websocket traffic.
func queryEqual(x, y *xylona.ServerQuery) bool {
	if x.ServerId != y.ServerId || x.ServerName != y.ServerName || x.Type != y.Type {
		return false
	}
	switch x.Type {
	case xylona.ServerQuery_Minecraft:
		xm := x.Minecraft
		ym := y.Minecraft
		return xm.Motd == ym.Motd &&
			xm.GameType == ym.GameType &&
			xm.Map == ym.Map &&
			xm.NumberOfPlayers == ym.NumberOfPlayers &&
			xm.MaxPlayers == ym.MaxPlayers &&
			slices.Equal(xm.PlayerList, ym.PlayerList) &&
			xm.ProtocolVersion == ym.ProtocolVersion &&
			xm.ServerVersion == ym.ServerVersion
	case xylona.ServerQuery_Source:
		xs := x.Source
		ys := y.Source
		return xs.Name == ys.Name &&
			xs.Map == ys.Map &&
			xs.Game == ys.Game &&
			xs.AppId == ys.AppId &&
			xs.SteamId == ys.SteamId &&
			xs.GameId == ys.GameId &&
			xs.Players == ys.Players &&
			xs.MaxPlayers == ys.MaxPlayers &&
			xs.Bots == ys.Bots &&
			xs.ServerOs == ys.ServerOs &&
			xs.Visibility == ys.Visibility &&
			xs.Vac == ys.Vac &&
			xs.Version == ys.Version &&
			xs.Protocol == ys.Protocol
	}
	return false
}

func (ws *WebSocket) AddGameServerToUserID() {

}

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
		return nil, errClient
	}
	return xylonaconnect.NewFederationClient(httpClient, baseURL), nil
}

// metricsEqual checks if two GameServerMetrics snapshots are equal enough to skip sending.
func metricsEqual(a, b *xylona.GameServerMetrics) bool {
	if a == nil || b == nil {
		return a == b
	}
	return int64(a.CpuPercent) == int64(b.CpuPercent) &&
		a.MemoryBytes == b.MemoryBytes &&
		a.MemoryWorkingSetBytes == b.MemoryWorkingSetBytes &&
		a.NumberOfThreads == b.NumberOfThreads &&
		a.DiskUsageBytes == b.DiskUsageBytes &&
		a.UptimeSeconds == b.UptimeSeconds &&
		int64(a.IoReadRate) == int64(b.IoReadRate) &&
		int64(a.IoWriteRate) == int64(b.IoWriteRate) &&
		a.ConnectionCount == b.ConnectionCount
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
		MemoryUsedBytes:        int64(snapshot.MemoryUsed),
		MemoryTotalBytes:       int64(snapshot.MemoryTotal),
		DiskPercent:            snapshot.DiskPercent,
		DiskUsedBytes:          int64(snapshot.DiskUsed),
		DiskTotalBytes:         int64(snapshot.DiskTotal),
		GameServerCount:        int32(gsCount),
		RunningGameServerCount: int32(runningCount),
		UserCount:              int32(userCount),
		RecordedAt:             timestamppb.Now(),
	}
}

// nodeSnapshotEqual checks if two NodeResourceSnapshot values are equal enough to skip sending.
func nodeSnapshotEqual(a, b *xylona.NodeResourceSnapshot) bool {
	if a == nil || b == nil {
		return a == b
	}
	return int64(a.CpuPercent) == int64(b.CpuPercent) &&
		a.MemoryUsedBytes == b.MemoryUsedBytes &&
		a.DiskUsedBytes == b.DiskUsedBytes &&
		a.GameServerCount == b.GameServerCount &&
		a.RunningGameServerCount == b.RunningGameServerCount &&
		a.UserCount == b.UserCount
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
					MemoryBytes:           int64(memVMS),
					MemoryWorkingSetBytes: int64(memRSS),
					MemoryPercent:         float64(memPct),
					CpuCores:              cpuCores,
					NumberOfThreads:       threads,
					DiskUsageBytes:        int64(diskBytes),
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
				serverQuery, exists := allQueryInfo.Servers[gameServer.ID]
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

func (ws *WebSocket) subscribeUserToOwnedGameServerNotifications(s *melody.Session, gameServer *models.GameServer) {
	errAddGameServerOutputListener := ws.addGameServerNotificationListener(s, gameServer.ID)
	if errAddGameServerOutputListener != nil {
		log.Debug().Err(errAddGameServerOutputListener).Msg("Failed to get game server console")
		errWrite := s.Write(fmt.Appendf(nil, "Failed to get game server console: %s", errAddGameServerOutputListener))
		if errWrite != nil {
			log.Error().Err(errWrite).Msg("Failed to write websocket message")
		}
		return
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
			GameServerId: gameServer.ID,
			Status:       status,
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

func (ws *WebSocket) closeCommandOutputListeners(s *melody.Session) {
	sessionConnection, errGetConnection := ws.getSessionConnection(s)
	if errGetConnection != nil {
		// log.Error().Err(errGetConnection).Msg("Failed to get session connection")
		return
	}
	gameServerIDs := sessionConnection.allGameServerIDs
	for _, gameServerID := range gameServerIDs {
		if gameServerID == "" {
			return
		}
		command := ws.supervisor.GetCommandByIDOrCreateShell(gameServerID)
		command.RemoveOutputListener(sessionConnection.id.String())
	}
}

func closeSession(s *melody.Session) {
	if s.IsClosed() {
		return
	}
	errClose := s.Close()
	if errClose != nil {
		log.Error().Err(errClose).Msg("Failed to close websocket connection")
	}
}

// handleUserWebsocketConnection handles a single websocket connection for a user. It's designed to be run in a Go routine.
// It listens for messages on the streamChan channel and writes them to the websocket connection.
func (ws *WebSocket) handleUserWebsocketConnection(s *melody.Session, user *models.User, streamChan chan *xylona.Message) {
	for {
		select {
		case <-ws.ctx.Done():
			log.Debug().Str("User", user.UserName).Msg("Got Xylona shutdown signal. Closing websocket stream.")
			closeSession(s)
			ws.closeCommandOutputListeners(s)
			return
		case <-s.Request.Context().Done():
			log.Debug().Str("User", user.UserName).Msg("Websocket connection closed. Closing websocket stream.")
			closeSession(s)
			ws.closeCommandOutputListeners(s)
			return
		case output := <-streamChan:
			// Only write game server console output if this connection is subscribed.
			if output.Type == xylona.Message_GameServerConsole {
				sessionConnection, errGetSessionConnection := ws.getSessionConnection(s)
				if errGetSessionConnection != nil {
					log.Error().Err(errGetSessionConnection).Msg("Failed to get session connection")
					continue
				}
				sessionConnection.RLock()
				_, exists := sessionConnection.requestedGameServerOutputIDs[output.GameServerConsoleOutput.GameServerId]
				sessionConnection.RUnlock()
				if !exists {
					log.Debug().Str("User", user.UserName).Msg("Not subscribed to game server console output")
					continue
				}
			}
			byteOut, errMarshal := protojson.Marshal(output)
			if errMarshal != nil {
				log.Error().Err(errMarshal).Msg("Failed to marshal game server output")
				return
			}
			errWrite := s.Write(byteOut)
			if errWrite != nil {
				log.Error().Err(errWrite).Msg("Failed to write websocket message")
				return
			}
		}
	}
}

func (ws *WebSocket) addGameServerNotificationListener(s *melody.Session, gameServerID string) error {
	sessionConnection, errGetSessionConnection := ws.getSessionConnection(s)
	if errGetSessionConnection != nil {
		log.Error().Err(errGetSessionConnection).Msg("Failed to get session connection")
		return errors.New("failed to get session connection")
	}
	listenerID := sessionConnection.id.String()

	gameServer, errGetGameServer := ws.db.GetGameServerByID(gameServerID)
	if errGetGameServer != nil {
		log.Error().Err(errGetGameServer).Msg("Failed to get game server by ID")
		return errors.New("game server not found")
	}
	command := ws.supervisor.GetCommandByIDOrCreateShell(gameServer.ID)
	command.AddOutputListener(listenerID, sessionConnection.outputStreamChannel)
	return nil
}

func (ws *WebSocket) applyFederatedActingIdentity(header http.Header, userID string) error {
	return federation.ApplyActingIdentityHeadersForUserID(ws.db, header, userID)
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
				Output:       chunk.Output,
			},
		}

		select {
		case conn.outputStreamChannel <- msg:
		case <-streamCtx.Done():
			return
		case <-s.Request.Context().Done():
			return
		default:
			// Channel full, skip this message to avoid blocking.
		}
	}

	if errReceive := stream.Err(); errReceive != nil {
		log.Debug().Err(errReceive).Str("server_id", serverID).Msg("Remote console stream ended")
	}
}

// BroadcastRemoteServerStatus sends a status update for a remote server to all connected WebSocket clients.
func (ws *WebSocket) BroadcastRemoteServerStatus(serverID string, status xylona.Status) {
	out := &xylona.Message{
		Type: xylona.Message_GameServerStatus,
		GameServerStatusUpdate: &xylona.GameServerStatusUpdate{
			GameServerId: serverID,
			Status:       status,
		},
	}
	byteOut, errMarshal := protojson.Marshal(out)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Msg("Failed to marshal remote server status update")
		return
	}

	ws.userWebsocketConnectionsLock.RLock()
	defer ws.userWebsocketConnectionsLock.RUnlock()

	for _, userConnections := range ws.userWebsocketConnections {
		for _, conn := range userConnections {
			if conn.melodySession.IsClosed() {
				continue
			}
			errWrite := conn.melodySession.Write(byteOut)
			if errWrite != nil {
				log.Debug().Err(errWrite).Msg("Failed to write remote status update to WebSocket")
			}
		}
	}
}

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

// BroadcastUpdateProgress sends a game server update progress event to all
// connected WebSocket clients that have access to the given server.
func (ws *WebSocket) BroadcastUpdateProgress(serverID string, step xylona.UpdateStep, stepStatus xylona.StepStatus, message string) {
	out := &xylona.Message{
		Type: xylona.Message_GameServerUpdateProgress,
		UpdateProgress: &xylona.UpdateProgress{
			GameServerId: serverID,
			Step:         step,
			StepStatus:   stepStatus,
			Message:      message,
		},
	}
	byteOut, errMarshal := protojson.Marshal(out)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Msg("Failed to marshal update progress message")
		return
	}

	ws.userWebsocketConnectionsLock.RLock()
	defer ws.userWebsocketConnectionsLock.RUnlock()

	for _, userConnections := range ws.userWebsocketConnections {
		for _, conn := range userConnections {
			if conn.melodySession.IsClosed() {
				continue
			}
			if !conn.hasGameServerAccess(serverID) {
				continue
			}
			errWrite := conn.melodySession.Write(byteOut)
			if errWrite != nil {
				log.Debug().Err(errWrite).Msg("Failed to write update progress to WebSocket")
			}
		}
	}
}

// BroadcastServerSoftwareInstall sends a server software install status update
// to all connected WebSocket clients. Authorization is enforced at the RPC
// level, so we broadcast to all authenticated connections here — the frontend
// composable filters by game server ID.
func (ws *WebSocket) BroadcastServerSoftwareInstall(serverID string, status string, softwareID string, errMsg string) {
	out := &xylona.Message{
		Type: xylona.Message_ServerSoftwareInstall,
		ServerSoftwareInstallUpdate: &xylona.ServerSoftwareInstallUpdate{
			GameServerId: serverID,
			Status:       status,
			Error:        errMsg,
			SoftwareId:   softwareID,
		},
	}
	byteOut, errMarshal := protojson.Marshal(out)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Msg("Failed to marshal server software install update")
		return
	}

	ws.userWebsocketConnectionsLock.RLock()
	defer ws.userWebsocketConnectionsLock.RUnlock()

	for _, userConnections := range ws.userWebsocketConnections {
		for _, conn := range userConnections {
			if conn.melodySession.IsClosed() {
				continue
			}
			errWrite := conn.melodySession.Write(byteOut)
			if errWrite != nil {
				log.Debug().Err(errWrite).Msg("Failed to write software install update to WebSocket")
			}
		}
	}
}

func (ws *WebSocket) cancelRemoteConsoleStreams(conn *connection) {
	conn.Lock()
	defer conn.Unlock()
	for serverID, cancel := range conn.remoteConsoleCancels {
		cancel()
		delete(conn.remoteConsoleCancels, serverID)
	}
}

func (ws *WebSocket) deleteConnection(sess *melody.Session) {
	userID, errGetUserID := getSessionUserID(sess)
	if errGetUserID != nil {
		log.Error().Msg("Failed to get user ID from session")
		return
	}
	connectionID, errGetConnectionID := getSessionConnectionID(sess)
	if errGetConnectionID != nil {
		log.Error().Msg("Failed to get connection ID from session")
		return
	}

	ws.userWebsocketConnectionsLock.Lock()
	defer ws.userWebsocketConnectionsLock.Unlock()

	if _, userExists := ws.userWebsocketConnections[userID]; !userExists {
		log.Error().Str("User", userID).Msg("User not found in websocket connections")
		return
	}
	if _, connectionExists := ws.userWebsocketConnections[userID][connectionID]; !connectionExists {
		log.Error().Str("User", userID).Msg("Connection not found in websocket connections")
		if len(ws.userWebsocketConnections[userID]) == 0 {
			delete(ws.userWebsocketConnections, userID)
		}
		return
	}
	delete(ws.userWebsocketConnections[userID], connectionID)
	if len(ws.userWebsocketConnections[userID]) == 0 {
		delete(ws.userWebsocketConnections, userID)
	}
}

func (ws *WebSocket) handleDisconnect(s *melody.Session) {
	log.Debug().Msg("Websocket disconnected")
	// Cancel any active remote console streams for this connection.
	sessionConnection, errGetConnection := ws.getSessionConnection(s)
	if errGetConnection == nil {
		ws.cancelRemoteConsoleStreams(sessionConnection)
	}
	ws.deleteConnection(s)
	if s.IsClosed() {
		return
	}
	errClose := s.Close()
	if errClose != nil {
		log.Error().Err(errClose).Msg("Failed to close websocket connection")
	}
}

func (ws *WebSocket) handleMessage(s *melody.Session, msg []byte) {
	if msg == nil {
		return
	}
	// Handle heartbeat from websocket. Ensures a connection is still alive.
	if strings.ToLower(string(msg)) == "ping" {
		errWrite := s.Write([]byte("pong"))
		if errWrite != nil {
			log.Error().Err(errWrite).Msg("Failed to write websocket message")
		}
		return
	}
	log.Debug().Msgf("Websocket message: %s", string(msg))

	username, errGetUsername := getSessionUsername(s)
	if errGetUsername != nil {
		log.Error().Msg("Failed to get username from session")
		return
	}

	sessionConnection, errGetSessionConnection := ws.getSessionConnection(s)
	if errGetSessionConnection != nil {
		log.Error().Err(errGetSessionConnection).Msg("Failed to get session connection")
		return
	}

	websocketRequest := &xylona.Request{}
	errUnmarshal := protojson.Unmarshal(msg, websocketRequest)
	if errUnmarshal != nil {
		log.Error().Err(errUnmarshal).Msg("Failed to unmarshal websocket message")
		return
	}
	switch websocketRequest.Type {
	case xylona.Request_GetGameServerConsole:
		if websocketRequest.GameServerId == nil {
			log.Error().Msg("Game server ID not set")
			return
		}
		serverID := *websocketRequest.GameServerId
		sessionConnection.Lock()
		sessionConnection.requestedGameServerOutputIDs[serverID] = struct{}{}
		sessionConnection.Unlock()

		// Check if this is a remote server and start a federation console stream.
		_, errGetLocal := ws.db.GetGameServerByID(serverID)
		if errGetLocal != nil && errors.Is(errGetLocal, sql.ErrNoRows) {
			go ws.startRemoteConsoleStream(s, sessionConnection, serverID)
		}

		rawData := &xylona.Message{
			Type:    xylona.Message_Raw,
			RawData: "Subscribed to game server console output",
		}
		b, errMarshal := protojson.Marshal(rawData)
		if errMarshal != nil {
			log.Error().Err(errMarshal).Msg("Failed to write websocket message")
			return
		}
		_ = s.Write(b)
	case xylona.Request_SubscribeServerMetrics:
		if websocketRequest.GameServerId == nil {
			return
		}
		serverID := *websocketRequest.GameServerId
		sessionConnection.Lock()
		sessionConnection.subscribedMetricsServerIDs[serverID] = struct{}{}
		sessionConnection.Unlock()

	case xylona.Request_UnsubscribeServerMetrics:
		if websocketRequest.GameServerId == nil {
			return
		}
		serverID := *websocketRequest.GameServerId
		sessionConnection.Lock()
		delete(sessionConnection.subscribedMetricsServerIDs, serverID)
		sessionConnection.Unlock()

	default:
		log.Warn().Str("User", username).Msg("Unknown websocket message type")
	}
}
