// Package websocket provides Xylona's websocket server and subscription flows.
package websocket

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/securecookie"
	"github.com/olahol/melody"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/controller/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	sessionKeyUserID                           = "userID"
	sessionKeyStreamChannel                    = "streamChannel"
	sessionKeyUserName                         = "userName"
	sessionKeyConnectionID                     = "connectionID"
	sessionKeyGamesServers                     = "gameServers"
	sessionExpiredCloseCode                    = 4003
	defaultSessionValidationInterval           = 30 * time.Second
	defaultConsolePermissionValidationInterval = 5 * time.Second
)

var (
	errSessionUsernameMissing     = errors.New("failed to get username from session")
	errSessionUserIDMissing       = errors.New("failed to get user ID from session")
	errSessionConnectionIDMissing = errors.New("failed to get connection ID from session")
	errSessionGameServersMissing  = errors.New("failed to get game servers from session")
)

type connection struct {
	id                           uuid.UUID
	melodySession                *melody.Session
	outputStreamChannel          chan *xylona.Message
	sessionID                    string
	sessionToken                 string
	userID                       string
	sessionUserLookup            func() (*models.User, error)
	sessionUserValidation        func() (*models.User, error)
	userLookup                   func(string) (*models.User, error)
	metricsPermissionLookup      func(string) (bool, error)
	consolePermissionLookup      func(string) (bool, error)
	allGameServerIDs             []string
	requestedGameServerOutputIDs map[string]struct{}
	subscribedMetricsServerIDs   map[string]struct{}
	consoleStreamCancels         map[string]context.CancelFunc
	consoleStreamTokens          map[string]*struct{}
	isSuperUser                  bool
	lastSuperUserCheck           time.Time
	*sync.RWMutex
}

func (c *connection) consumeRequestedGameServerOutputIDs() []string {
	c.Lock()
	defer c.Unlock()

	gameServerIDs := make([]string, 0, len(c.requestedGameServerOutputIDs))
	for gameServerID := range c.requestedGameServerOutputIDs {
		gameServerIDs = append(gameServerIDs, gameServerID)
	}
	clear(c.requestedGameServerOutputIDs)

	return gameServerIDs
}

// WebSocket coordinates websocket sessions and broadcasts real-time updates.
type WebSocket struct {
	melody                       *melody.Melody
	actions                      *actions.Instance
	db                           *db.Connection
	nodeRegistry                 *noderegistry.Registry
	secureCookie                 *securecookie.SecureCookie
	ctx                          context.Context
	userWebsocketConnections     map[string]map[uuid.UUID]*connection // map[userID]map[connectionID]*connection
	userWebsocketConnectionsLock *sync.RWMutex
	sessionLock                  *sync.RWMutex
	sessionValidationInterval    time.Duration
	consolePermissionInterval    time.Duration
}

func (ws *WebSocket) getSessionGameServers(s *melody.Session) ([]*models.GameServer, error) {
	ws.sessionLock.RLock()
	defer ws.sessionLock.RUnlock()
	gs, gameServersExists := s.Get(sessionKeyGamesServers)
	if !gameServersExists {
		return nil, errSessionGameServersMissing
	}
	gameServers, ok := gs.([]*models.GameServer)
	if !ok {
		return nil, errSessionGameServersMissing
	}
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

// NewInstance builds the websocket manager and returns its HTTP upgrade handler.
func NewInstance(
	ctx context.Context,
	actionsInst *actions.Instance,
	database *db.Connection,
	nodeRegistry *noderegistry.Registry,
	secureCookie *securecookie.SecureCookie,
	proxyTrust *gatekeeper.ProxyTrust,
) (*WebSocket, http.HandlerFunc) {
	m := melody.New()
	m.Upgrader.CheckOrigin = func(r *http.Request) bool {
		return gatekeeper.IsSameOriginRequest(r, proxyTrust)
	}
	inst := &WebSocket{
		melody:                       m,
		actions:                      actionsInst,
		db:                           database,
		nodeRegistry:                 nodeRegistry,
		ctx:                          ctx,
		secureCookie:                 secureCookie,
		userWebsocketConnections:     make(map[string]map[uuid.UUID]*connection),
		userWebsocketConnectionsLock: &sync.RWMutex{},
		sessionLock:                  &sync.RWMutex{},
		sessionValidationInterval:    defaultSessionValidationInterval,
	}
	m.HandleConnect(inst.handleConnect)
	m.HandleDisconnect(inst.handleDisconnect)
	m.HandleMessage(inst.handleMessage)
	go inst.subscribeLocalGameServerStatusChanges()
	return inst, inst.handleRequest
}

// AddGameServerToUserID reserves a hook for future ownership cache updates.
func (ws *WebSocket) AddGameServerToUserID() {}

// CloseSession closes every websocket authenticated by sessionID.
func (ws *WebSocket) CloseSession(sessionID string) {
	if ws == nil || sessionID == "" {
		return
	}

	ws.userWebsocketConnectionsLock.RLock()
	connections := make([]*connection, 0)
	for _, userConnections := range ws.userWebsocketConnections {
		for _, conn := range userConnections {
			if conn.sessionID == sessionID {
				connections = append(connections, conn)
			}
		}
	}
	ws.userWebsocketConnectionsLock.RUnlock()
	ws.closeConnections(connections)
}

// CloseUser closes every websocket authenticated as userID.
func (ws *WebSocket) CloseUser(userID string) {
	if ws == nil || userID == "" {
		return
	}

	ws.userWebsocketConnectionsLock.RLock()
	userConnections := ws.userWebsocketConnections[userID]
	connections := make([]*connection, 0, len(userConnections))
	for _, conn := range userConnections {
		connections = append(connections, conn)
	}
	ws.userWebsocketConnectionsLock.RUnlock()
	ws.closeConnections(connections)
}

// CloseAll closes every active websocket connection.
func (ws *WebSocket) CloseAll() {
	if ws == nil {
		return
	}

	ws.userWebsocketConnectionsLock.RLock()
	connections := make([]*connection, 0)
	for _, userConnections := range ws.userWebsocketConnections {
		for _, conn := range userConnections {
			connections = append(connections, conn)
		}
	}
	ws.userWebsocketConnectionsLock.RUnlock()
	ws.closeConnections(connections)
}

func (ws *WebSocket) closeConnections(connections []*connection) {
	for _, conn := range connections {
		ws.cancelConsoleStreams(conn)
		closeSessionWithPolicyViolation(conn.melodySession)
	}
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
		log.Debug().
			Err(errGetSession).
			Str("method", s.Request.Method).
			Str("url", s.Request.URL.String()).
			Str("remote_addr", s.Request.RemoteAddr).
			Str("user_agent", s.Request.UserAgent()).
			Msg("Rejected websocket connection without session cookies")
		closeSessionExpired(s)
		return
	}

	log.Debug().Msg("Got session from cookies")
	user, errGetUser := gatekeeper.GetUserFromSession(sessionCookies.SessionID, sessionCookies.SessionToken,
		ws.db, ws.secureCookie)
	if errGetUser != nil {
		log.Error().Err(errGetUser).Msg("Failed to get user from session")
		closeSessionExpired(s)
		return
	}

	log.Debug().Str("User", user.UserName).Msg("Websocket connected")
	wsConnection := &connection{
		id:                  uuid.New(),
		melodySession:       s,
		outputStreamChannel: make(chan *xylona.Message, 256),
		sessionID:           sessionCookies.SessionID,
		sessionToken:        sessionCookies.SessionToken,
		userID:              user.ID,
		userLookup:          ws.db.GetUserByID,
		metricsPermissionLookup: func(serverID string) (bool, error) {
			currentUser, errUser := ws.db.GetUserByID(user.ID)
			if errUser != nil {
				return false, fmt.Errorf("load websocket user for metrics authorization: %w", errUser)
			}
			gameServer, errGameServer := ws.db.GetGameServerByID(serverID)
			if errGameServer != nil {
				return false, fmt.Errorf("load game server for metrics authorization: %w", errGameServer)
			}
			return db.HasPermission(
				ws.db,
				currentUser,
				gameServer.ID,
				gameServer.UserID,
				"game_server.metrics",
			)
		},
		consolePermissionLookup: func(serverID string) (bool, error) {
			currentUser, errUser := ws.db.GetUserByID(user.ID)
			if errUser != nil {
				return false, fmt.Errorf("load websocket user for console authorization: %w", errUser)
			}
			gameServer, errGameServer := ws.db.GetGameServerByID(serverID)
			if errGameServer != nil {
				return false, fmt.Errorf("load game server for console authorization: %w", errGameServer)
			}
			return db.HasPermission(
				ws.db,
				currentUser,
				gameServer.ID,
				gameServer.UserID,
				"game_server.console",
			)
		},
		allGameServerIDs:             []string{},
		requestedGameServerOutputIDs: make(map[string]struct{}),
		subscribedMetricsServerIDs:   make(map[string]struct{}),
		consoleStreamCancels:         make(map[string]context.CancelFunc),
		consoleStreamTokens:          make(map[string]*struct{}),
		isSuperUser:                  user.SuperUser,
		lastSuperUserCheck:           time.Now(),
		RWMutex:                      &sync.RWMutex{},
	}
	wsConnection.sessionUserLookup = func() (*models.User, error) {
		return gatekeeper.GetUserFromSession(
			wsConnection.sessionID,
			wsConnection.sessionToken,
			ws.db,
			ws.secureCookie,
		)
	}
	wsConnection.sessionUserValidation = func() (*models.User, error) {
		return gatekeeper.ValidateUserFromSession(
			wsConnection.sessionID,
			wsConnection.sessionToken,
			ws.db,
			ws.secureCookie,
		)
	}

	s.Set(sessionKeyConnectionID, wsConnection.id)
	s.Set(sessionKeyUserID, user.ID)
	s.Set(sessionKeyUserName, user.UserName)
	s.Set(sessionKeyStreamChannel, wsConnection.outputStreamChannel)

	ws.userWebsocketConnectionsLock.Lock()
	_, userExists := ws.userWebsocketConnections[user.ID]
	if !userExists {
		ws.userWebsocketConnections[user.ID] = make(map[uuid.UUID]*connection)
	}
	ws.userWebsocketConnections[user.ID][wsConnection.id] = wsConnection
	ws.userWebsocketConnectionsLock.Unlock()

	// Mirror the RPC visibility model so access-filtered websocket broadcasts
	// reach delegated users with server-scoped RBAC grants as well as owners.
	gameServers, errGetServers := ws.db.GetGameServersAccessibleByUser(user.ID)
	if errGetServers != nil {
		log.Error().Err(errGetServers).Msg("Failed to get game servers accessible by user")
		return
	}

	wsConnection.allGameServerIDs = make([]string, len(gameServers))
	for i, gameServer := range gameServers {
		wsConnection.allGameServerIDs[i] = gameServer.ID
	}

	ws.setSessionGameServers(s, gameServers)
	for _, gameServer := range gameServers {
		go ws.sendUserGameServerStatus(s, gameServer)
	}

	go ws.handleUserWebsocketConnection(s, user, wsConnection)
	go ws.sendOwnedServersQueryInfo(s)
	go ws.sendOwnedServersMetrics(s)

	if user.SuperUser {
		go ws.sendNodeMetrics(s)
	}

	// Listen for game server lifecycle events that affect this session's owned set.
	go ws.listenForGameServerRemoved(s)
	go ws.listenForGameServerCreated(s)
}

// handleUserWebsocketConnection handles a single websocket connection for a user. It's designed to be run in a Go routine.
// It listens for messages on the streamChan channel and writes them to the websocket connection.

func (ws *WebSocket) handleUserWebsocketConnection(s *melody.Session, user *models.User, conn *connection) {
	validationInterval := ws.sessionValidationInterval
	if validationInterval <= 0 {
		validationInterval = defaultSessionValidationInterval
	}
	validationTicker := time.NewTicker(validationInterval)
	defer validationTicker.Stop()

	for {
		select {
		case <-ws.ctx.Done():
			log.Debug().Str("User", user.UserName).Msg("Got Xylona shutdown signal. Closing websocket stream.")
			closeSession(s)
			ws.closeConsoleOutputStreams(s)
			return
		case <-s.Request.Context().Done():
			log.Debug().Str("User", user.UserName).Msg("Websocket connection closed. Closing websocket stream.")
			closeSession(s)
			ws.closeConsoleOutputStreams(s)
			return
		case <-validationTicker.C:
			errSession := conn.validateSession()
			if errSession != nil {
				log.Warn().Err(errSession).Str("user_id", conn.userID).
					Msg("Closing websocket with an invalid passive session")
				ws.cancelConsoleStreams(conn)
				closeSessionExpired(s)
				return
			}
		case output := <-conn.outputStreamChannel:
			// Only write game server console output if this connection is subscribed.
			if output.GetType() == xylona.Message_GameServerConsole {
				sessionConnection, errGetSessionConnection := ws.getSessionConnection(s)
				if errGetSessionConnection != nil {
					log.Error().Err(errGetSessionConnection).Msg("Failed to get session connection")
					continue
				}
				sessionConnection.RLock()
				_, exists := sessionConnection.requestedGameServerOutputIDs[output.GetGameServerConsoleOutput().GetGameServerId()]
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
	if !hasSessionConnectionState(s) {
		if s.IsClosed() {
			return
		}
		errClose := s.Close()
		if errClose != nil {
			log.Error().Err(errClose).Msg("Failed to close websocket connection")
		}
		return
	}
	// Cancel any active console streams for this connection.
	sessionConnection, errGetConnection := ws.getSessionConnection(s)
	if errGetConnection == nil {
		ws.cancelConsoleStreams(sessionConnection)
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
	sessionConnection, errGetSessionConnection := ws.getSessionConnection(s)
	if errGetSessionConnection != nil {
		log.Error().Err(errGetSessionConnection).Msg("Failed to get session connection")
		closeSessionWithPolicyViolation(s)
		return
	}
	// Automatic heartbeat pings count as session activity by design. An open,
	// connected UI keeps its session active until absolute expiry or revocation.
	errSession := sessionConnection.revalidateSession()
	if errSession != nil {
		log.Warn().Err(errSession).Str("user_id", sessionConnection.userID).
			Msg("Closing websocket with an invalid session")
		ws.cancelConsoleStreams(sessionConnection)
		closeSessionExpired(s)
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

	websocketRequest := &xylona.Request{}
	errUnmarshal := protojson.Unmarshal(msg, websocketRequest)
	if errUnmarshal != nil {
		log.Error().Err(errUnmarshal).Msg("Failed to unmarshal websocket message")
		return
	}
	switch websocketRequest.GetType() {
	case xylona.Request_GetGameServerConsole:
		if websocketRequest.GameServerId == nil {
			log.Error().Msg("Game server ID not set")
			return
		}
		serverID := websocketRequest.GetGameServerId()
		if !sessionConnection.hasGameServerAccess(serverID) {
			log.Warn().Str("User", username).Str("server_id", serverID).
				Msg("Rejected console subscription: user does not have access to server")
			return
		}
		if !sessionConnection.hasConsolePermission(serverID) {
			log.Warn().Str("User", username).Str("server_id", serverID).
				Msg("Rejected console subscription: user does not have console permission")
			return
		}

		gameServer, errGetServer := ws.db.GetGameServerByID(serverID)
		if errGetServer != nil {
			if !errors.Is(errGetServer, sql.ErrNoRows) {
				log.Error().Err(errGetServer).Str("server_id", serverID).Msg("Failed to get game server for console stream")
			}
			return
		}

		sessionConnection.Lock()
		sessionConnection.requestedGameServerOutputIDs[serverID] = struct{}{}
		sessionConnection.Unlock()

		errSubscribe := ws.subscribeGameServerConsole(sessionConnection, gameServer)
		if errSubscribe != nil {
			sessionConnection.removeConsoleSubscription(serverID)
			log.Error().Err(errSubscribe).Str("server_id", serverID).Msg("Failed to subscribe to game server console output")
			return
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
	case xylona.Request_RemoveGameServerConsole:
		if websocketRequest.GameServerId == nil {
			return
		}
		serverID := websocketRequest.GetGameServerId()
		sessionConnection.Lock()
		delete(sessionConnection.requestedGameServerOutputIDs, serverID)
		sessionConnection.Unlock()

		ws.cancelConsoleStream(sessionConnection, serverID)
	case xylona.Request_SubscribeServerMetrics:
		if websocketRequest.GameServerId == nil {
			return
		}
		serverID := websocketRequest.GetGameServerId()
		if !sessionConnection.canSubscribeToServerMetrics(serverID) {
			log.Warn().Str("User", username).Str("server_id", serverID).
				Msg("Rejected metrics subscription: user does not have metrics permission")
			return
		}
		sessionConnection.Lock()
		sessionConnection.subscribedMetricsServerIDs[serverID] = struct{}{}
		sessionConnection.Unlock()

	case xylona.Request_UnsubscribeServerMetrics:
		if websocketRequest.GameServerId == nil {
			return
		}
		serverID := websocketRequest.GetGameServerId()
		sessionConnection.Lock()
		delete(sessionConnection.subscribedMetricsServerIDs, serverID)
		sessionConnection.Unlock()

	default:
		log.Warn().Str("User", username).Msg("Unknown websocket message type")
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

func closeSessionWithPolicyViolation(s *melody.Session) {
	if s.IsClosed() {
		return
	}
	errClose := s.CloseWithMsg(melody.FormatCloseMessage(melody.ClosePolicyViolation, "Unauthorized"))
	if errClose != nil {
		log.Error().Err(errClose).Msg("Failed to close unauthorized websocket connection")
	}
}

func closeSessionExpired(s *melody.Session) {
	if s.IsClosed() {
		return
	}
	errClose := s.CloseWithMsg(melody.FormatCloseMessage(sessionExpiredCloseCode, "Session expired"))
	if errClose != nil {
		log.Error().Err(errClose).Msg("Failed to close websocket with an expired session")
	}
}
