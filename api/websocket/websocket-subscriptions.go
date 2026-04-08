package websocket

import (
	"errors"
	"fmt"

	"github.com/olahol/melody"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (ws *WebSocket) listenForGameServerRemoved(s *melody.Session) {
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

func (ws *WebSocket) listenForGameServerCreated(s *melody.Session) {
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

func (ws *WebSocket) cancelRemoteConsoleStreams(conn *connection) {
	conn.Lock()
	defer conn.Unlock()
	for serverID, cancel := range conn.remoteConsoleCancels {
		cancel()
		delete(conn.remoteConsoleCancels, serverID)
	}
}
