package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/securecookie"
	"github.com/olahol/melody"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/supervisor"
)

const (
	sessionKeyUserID        = "userID"
	sessionKeyStreamChannel = "streamChannel"
	sessionKeyUserName      = "userName"
	sessionKeyConnectionID  = "connectionID"
)

type connection struct {
	id                          uuid.UUID
	melodySession               *melody.Session
	outputStreamChannel         chan xylona.Message
	requestedGameServerOutputID *string
	*sync.RWMutex
}

type WebSocket struct {
	melody                       *melody.Melody
	supervisor                   *supervisor.Instance
	db                           *db.Connection
	secureCookie                 *securecookie.SecureCookie
	ctx                          context.Context
	userWebsocketConnections     map[string]map[uuid.UUID]*connection // map[userID]map[connectionID]*connection
	userWebsocketConnectionsLock *sync.RWMutex
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

func getSessionStreamChannel(s *melody.Session) (chan helpers.WebsocketMessage, error) {
	sc, streamChannelExists := s.Get(sessionKeyStreamChannel)
	if !streamChannelExists {
		return nil, fmt.Errorf("failed to get stream channel from session")
	}
	streamChannel := sc.(chan helpers.WebsocketMessage)
	return streamChannel, nil
}

func getSessionConnectionID(s *melody.Session) (uuid.UUID, error) {
	c, connectionIDExists := s.Get(sessionKeyConnectionID)
	if !connectionIDExists {
		return uuid.Nil, fmt.Errorf("failed to get connection ID from session")
	}
	connectionID := c.(uuid.UUID)
	return connectionID, nil
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

func NewInstance(ctx context.Context, supervisorInst *supervisor.Instance, db *db.Connection,
	secureCookie *securecookie.SecureCookie,
) (*WebSocket, http.HandlerFunc) {
	m := melody.New()
	inst := &WebSocket{
		melody:                       m,
		supervisor:                   supervisorInst,
		db:                           db,
		ctx:                          ctx,
		secureCookie:                 secureCookie,
		userWebsocketConnections:     make(map[string]map[uuid.UUID]*connection),
		userWebsocketConnectionsLock: &sync.RWMutex{},
	}
	m.HandleConnect(inst.handleConnect)
	m.HandleDisconnect(inst.handleDisconnect)
	m.HandleMessage(inst.handleMessage)
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
		melodySession:               s,
		outputStreamChannel:         make(chan xylona.Message),
		requestedGameServerOutputID: nil,
		RWMutex:                     &sync.RWMutex{},
	}

	connectionID := uuid.New()

	s.Set(sessionKeyConnectionID, connectionID)
	s.Set(sessionKeyUserID, user.ID)
	s.Set(sessionKeyUserName, user.UserName)
	s.Set(sessionKeyStreamChannel, wsConnection.outputStreamChannel)

	ws.userWebsocketConnectionsLock.Lock()
	_, userExists := ws.userWebsocketConnections[user.ID]
	if !userExists {
		ws.userWebsocketConnections[user.ID] = make(map[uuid.UUID]*connection)
	}
	ws.userWebsocketConnections[user.ID][connectionID] = wsConnection
	ws.userWebsocketConnectionsLock.Unlock()

	go ws.sendUserGameServersStatuses(s, user)

	go ws.handleUserWebsocketConnection(s, user, wsConnection.outputStreamChannel)
}

func (ws *WebSocket) sendUserGameServersStatuses(s *melody.Session, user *models.User) {
	gameServers, errGetServers := ws.db.GetGameServersByUser(user.ID)
	if errGetServers != nil {
		log.Error().Err(errGetServers).Msg("Failed to get game servers by user")
	}
	for _, gameServer := range gameServers {
		command, errGetCommand := ws.supervisor.GetCommandByID(gameServer.ID)
		if errGetCommand != nil {
			continue
		}
		status := command.Status()
		out := &xylona.Message{
			Type: xylona.Message_GameServerStatus,
			GameServerStatusUpdate: &xylona.GameServerStatusUpdate{
				GameServerId: gameServer.ID,
				Status:       status,
			},
		}
		byteOut, errMarshal := json.Marshal(out)
		if errMarshal != nil {
			log.Error().Err(errMarshal).Msg("Failed to marshal game server status update")
			continue
		}
		errWrite := s.Write(byteOut)
		if errWrite != nil {
			log.Error().Err(errWrite).Msg("Failed to write websocket message")
			return
		}
	}
}

func (ws *WebSocket) closeCommandOutputListener(s *melody.Session) {
	sessionConnection, errGetConnection := ws.getSessionConnection(s)
	if errGetConnection != nil {
		// log.Error().Err(errGetConnection).Msg("Failed to get session connection")
		return
	}
	gameServerID := sessionConnection.requestedGameServerOutputID
	if gameServerID == nil {
		return
	}
	command := ws.supervisor.GetCommandByIDOrCreateShell(*gameServerID)
	command.RemoveOutputListener(sessionConnection.id.String())
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
func (ws *WebSocket) handleUserWebsocketConnection(s *melody.Session, user *models.User, streamChan chan xylona.Message) {
	for {
		select {
		case <-ws.ctx.Done():
			log.Debug().Str("User", user.UserName).Msg("Got Xylona shutdown signal. Closing websocket stream.")
			closeSession(s)
			ws.closeCommandOutputListener(s)
			return
		case <-s.Request.Context().Done():
			log.Debug().Str("User", user.UserName).Msg("Websocket connection closed. Closing websocket stream.")
			closeSession(s)
			ws.closeCommandOutputListener(s)
			return
		case output := <-streamChan:
			byteOut, errMarshal := json.Marshal(output)
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

func (ws *WebSocket) addGameServerOutputListener(s *melody.Session, gameServerID string) error {
	sessionConnection, errGetSessionConnection := ws.getSessionConnection(s)
	if errGetSessionConnection != nil {
		log.Error().Err(errGetSessionConnection).Msg("Failed to get session connection")
		return errors.New("failed to get session connection")
	}
	listenerID := sessionConnection.id.String()

	sessionConnection.Lock()
	sessionConnection.requestedGameServerOutputID = &gameServerID
	sessionConnection.Unlock()

	gameServer, errGetGameServer := ws.db.GetGameServerByID(gameServerID)
	if errGetGameServer != nil {
		log.Error().Err(errGetGameServer).Msg("Failed to get game server by ID")
		return errors.New("game server not found")
	}
	command := ws.supervisor.GetCommandByIDOrCreateShell(gameServer.ID)
	command.AddOutputListener(listenerID, sessionConnection.outputStreamChannel)
	return nil
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
	switch websocketRequest.Type {
	case xylona.Request_GetGameServerConsole:
		if websocketRequest.GameServerId == nil {
			log.Error().Msg("Game server ID not set")
			return
		}
		errAddGameServerOutputListener := ws.addGameServerOutputListener(s, *websocketRequest.GameServerId)
		if errAddGameServerOutputListener != nil {
			log.Debug().Err(errAddGameServerOutputListener).Msg("Failed to get game server console")
			errWrite := s.Write([]byte(fmt.Sprintf("Failed to get game server console: %s", errAddGameServerOutputListener)))
			if errWrite != nil {
				log.Error().Err(errWrite).Msg("Failed to write websocket message")
			}
			return
		}
		rawData := &xylona.Message{
			Type:    xylona.Message_Raw,
			RawData: "Subscribed to game server console output",
		}
		b, errMarshal := json.Marshal(rawData)
		if errMarshal != nil {
			log.Error().Err(errMarshal).Msg("Failed to write websocket message")
			return
		}
		_ = s.Write(b)
	default:
		log.Warn().Str("User", username).Msg("Unknown websocket message type")
		log.Debug().Str("User", username).Msgf("Websocket message: %s", string(msg))
		errWrite := s.Write([]byte(fmt.Sprintf("echo: %s", msg)))
		if errWrite != nil {
			log.Error().Err(errWrite).Msg("Failed to write websocket message")
		}
	}

	//websocketRequest := &helpers.WebsocketRequest{}
	//errUnmarshal := json.Unmarshal(msg, websocketRequest)
	//if errUnmarshal != nil {
	//	log.Error().Err(errUnmarshal).Msg("Failed to unmarshal websocket message")
	//	return
	//}
	//switch websocketRequest.Type {
	//case helpers.RequestGetGameServerConsole:
	//	if websocketRequest.GameServerID == nil {
	//		log.Error().Msg("Game server ID not set")
	//		return
	//	}
	//	errAddGameServerOutputListener := ws.addGameServerOutputListener(s, *websocketRequest.GameServerID)
	//	if errAddGameServerOutputListener != nil {
	//		log.Debug().Err(errAddGameServerOutputListener).Msg("Failed to get game server console")
	//		errWrite := s.Write([]byte(fmt.Sprintf("Failed to get game server console: %s", errAddGameServerOutputListener)))
	//		if errWrite != nil {
	//			log.Error().Err(errWrite).Msg("Failed to write websocket message")
	//		}
	//		return
	//	}
	//	rawData := &helpers.WebsocketMessage{
	//		Type:    helpers.WebsocketOutputTypeRaw,
	//		RawData: "Subscribed to game server console output",
	//	}
	//	b, errMarshal := json.Marshal(rawData)
	//	if errMarshal != nil {
	//		log.Error().Err(errMarshal).Msg("Failed to write websocket message")
	//		return
	//	}
	//	_ = s.Write(b)
	//default:
	//	log.Warn().Str("User", username).Msg("Unknown websocket message type")
	//	log.Debug().Str("User", username).Msgf("Websocket message: %s", string(msg))
	//	errWrite := s.Write([]byte(fmt.Sprintf("echo: %s", msg)))
	//	if errWrite != nil {
	//		log.Error().Err(errWrite).Msg("Failed to write websocket message")
	//	}
	//}
}
