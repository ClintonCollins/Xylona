// Package websocket provides Xylona's websocket server and subscription flows.
package websocket

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/securecookie"
	"github.com/olahol/melody"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/controller/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	sessionKeyUserID        = "userID"
	sessionKeyStreamChannel = "streamChannel"
	sessionKeyUserName      = "userName"
	sessionKeyConnectionID  = "connectionID"
	sessionKeyGamesServers  = "gameServers"
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
	userID                       string
	userLookup                   func(string) (*models.User, error)
	allGameServerIDs             []string
	requestedGameServerOutputIDs map[string]struct{}
	subscribedMetricsServerIDs   map[string]struct{}
	consoleStreamCancels         map[string]context.CancelFunc
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
) (*WebSocket, http.HandlerFunc) {
	m := melody.New()
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
	}
	m.HandleConnect(inst.handleConnect)
	m.HandleDisconnect(inst.handleDisconnect)
	m.HandleMessage(inst.handleMessage)
	go inst.subscribeLocalGameServerStatusChanges()
	return inst, inst.handleRequest
}

// AddGameServerToUserID reserves a hook for future ownership cache updates.
func (ws *WebSocket) AddGameServerToUserID() {}

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
		_ = s.CloseWithMsg([]byte("Unauthorized"))
		return
	}

	log.Debug().Msg("Got session from cookies")
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
		outputStreamChannel:          make(chan *xylona.Message, 256),
		userID:                       user.ID,
		userLookup:                   ws.db.GetUserByID,
		allGameServerIDs:             []string{},
		requestedGameServerOutputIDs: make(map[string]struct{}),
		subscribedMetricsServerIDs:   make(map[string]struct{}),
		consoleStreamCancels:         make(map[string]context.CancelFunc),
		isSuperUser:                  user.SuperUser,
		lastSuperUserCheck:           time.Now(),
		RWMutex:                      &sync.RWMutex{},
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

	go ws.handleUserWebsocketConnection(s, user, wsConnection.outputStreamChannel)
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
func (ws *WebSocket) handleUserWebsocketConnection(s *melody.Session, user *models.User, streamChan chan *xylona.Message) {
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
		case output := <-streamChan:
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

		sessionConnection.Lock()
		sessionConnection.requestedGameServerOutputIDs[serverID] = struct{}{}
		sessionConnection.Unlock()

		gameServer, errGetServer := ws.db.GetGameServerByID(serverID)
		if errGetServer != nil {
			if !errors.Is(errGetServer, sql.ErrNoRows) {
				log.Error().Err(errGetServer).Str("server_id", serverID).Msg("Failed to get game server for console stream")
			}
			return
		}

		errSubscribe := ws.subscribeGameServerConsole(sessionConnection, gameServer)
		if errSubscribe != nil {
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
