package websocket

import (
	"database/sql"
	"errors"

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
			sessionConn, errConn := ws.getSessionConnection(s)
			if errConn == nil {
				sessionConn.removeGameServerAccess(gameServer.ID)
			}
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

			// Re-check whether this session's user actually has access to the
			// newly created server before adding it to the session or access list.
			sessionConn, errConn := ws.getSessionConnection(s)
			if errConn != nil {
				continue
			}
			accessible, errAccess := ws.db.GetGameServersAccessibleByUser(sessionConn.userID)
			if errAccess != nil {
				log.Error().Err(errAccess).Msg("Failed to check user access for new game server")
				continue
			}
			hasAccess := false
			for _, gs := range accessible {
				if gs.ID == gameServer.ID {
					hasAccess = true
					break
				}
			}
			if !hasAccess && !sessionConn.currentlySuperUser() {
				continue
			}

			go ws.sendUserGameServerStatus(s, gameServer)
			gameServers, err := ws.getSessionGameServers(s)
			if err != nil {
				log.Error().Err(err).Msg("Failed to get game servers from session")
				return
			}
			gameServers = append(gameServers, gameServer)
			ws.setSessionGameServers(s, gameServers)
			sessionConn.addGameServerAccess(gameServer.ID)
		}
	}
}

func (ws *WebSocket) closeCommandOutputListeners(s *melody.Session) {
	sessionConnection, errGetConnection := ws.getSessionConnection(s)
	if errGetConnection != nil {
		// log.Error().Err(errGetConnection).Msg("Failed to get session connection")
		return
	}
	gameServerIDs := sessionConnection.consumeRequestedGameServerOutputIDs()

	for _, gameServerID := range gameServerIDs {
		gameServer, errGetServer := ws.db.GetGameServerByID(gameServerID)
		if errGetServer != nil {
			if errors.Is(errGetServer, sql.ErrNoRows) {
				continue
			}
			log.Error().Err(errGetServer).Str("server_id", gameServerID).Msg("Failed to get game server for console listener cleanup")
			continue
		}
		command := ws.supervisor.GetCommandByIDOrCreateShell(gameServer.ID)
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
