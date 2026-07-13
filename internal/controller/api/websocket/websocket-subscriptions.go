package websocket

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/olahol/melody"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (ws *WebSocket) subscribeLocalGameServerStatusChanges() {
	eb := eventbus.Get()
	statusChanged := eb.SubscribeReliable(eventbus.TopicGameServerStatusChanged)
	defer eb.Unsubscribe(eventbus.TopicGameServerStatusChanged, statusChanged)

	ws.listenForLocalGameServerStatusChanges(statusChanged)
}

func (ws *WebSocket) listenForLocalGameServerStatusChanges(statusChanged <-chan any) {
	for {
		select {
		case <-ws.ctx.Done():
			return
		case data, ok := <-statusChanged:
			if !ok {
				return
			}

			statusEvent, ok := data.(eventbus.StatusChangedEvent)
			if !ok {
				log.Error().Msg("Failed to cast status event to StatusChangedEvent")
				continue
			}

			statusValue, statusExists := xylona.Status_value[statusEvent.NewStatus]
			if !statusExists {
				log.Warn().Str("status", statusEvent.NewStatus).Msg("Unknown game server status update")
				continue
			}

			ws.broadcastGameServerStatus(statusEvent.ServerID, statusEvent.ServerName, xylona.Status(statusValue))
		}
	}
}

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

func (ws *WebSocket) closeConsoleOutputStreams(s *melody.Session) {
	sessionConnection, errGetConnection := ws.getSessionConnection(s)
	if errGetConnection != nil {
		return
	}
	gameServerIDs := sessionConnection.consumeRequestedGameServerOutputIDs()

	for _, gameServerID := range gameServerIDs {
		ws.cancelConsoleStream(sessionConnection, gameServerID)
	}
}

func (ws *WebSocket) cancelConsoleStreams(conn *connection) {
	conn.Lock()
	defer conn.Unlock()
	for serverID, cancel := range conn.consoleStreamCancels {
		cancel()
		delete(conn.consoleStreamCancels, serverID)
		delete(conn.consoleStreamTokens, serverID)
	}
}

func (ws *WebSocket) cancelConsoleStream(conn *connection, serverID string) {
	conn.Lock()
	cancel, exists := conn.consoleStreamCancels[serverID]
	if exists {
		delete(conn.consoleStreamCancels, serverID)
		delete(conn.consoleStreamTokens, serverID)
	}
	conn.Unlock()

	if exists {
		cancel()
	}
}

func (ws *WebSocket) isLocalNode(nodeID string) bool {
	if ws == nil || ws.nodeRegistry == nil {
		return true
	}
	return nodeID == "" || nodeID == ws.nodeRegistry.SelfID()
}

func (ws *WebSocket) subscribeGameServerConsole(conn *connection, gameServer *models.GameServer) error {
	if conn == nil || gameServer == nil {
		return errors.New("game server console subscription requires connection and server")
	}

	return ws.startConsoleStream(conn, gameServer)
}

func (ws *WebSocket) startConsoleStream(conn *connection, gameServer *models.GameServer) error {
	if ws == nil || ws.nodeRegistry == nil {
		return errors.New("node registry unavailable")
	}

	nodeID := gameServer.NodeID
	if ws.isLocalNode(nodeID) {
		nodeID = ws.nodeRegistry.SelfID()
	}

	baseCtx := ws.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	streamCtx, cancel := context.WithCancel(baseCtx) //nolint:gosec // cancel is retained in conn.consoleStreamCancels and called on unsubscribe/disconnect.
	token := &struct{}{}

	conn.Lock()
	existingCancel, exists := conn.consoleStreamCancels[gameServer.ID]
	if exists {
		existingCancel()
	}
	if conn.consoleStreamTokens == nil {
		conn.consoleStreamTokens = make(map[string]*struct{})
	}
	conn.consoleStreamCancels[gameServer.ID] = cancel
	conn.consoleStreamTokens[gameServer.ID] = token
	conn.Unlock()

	go ws.runConsoleStream(streamCtx, conn, gameServer.ID, nodeID, token)
	return nil
}

func (ws *WebSocket) runConsoleStream(
	streamCtx context.Context,
	conn *connection,
	gameServerID string,
	nodeID string,
	token *struct{},
) {
	defer func() {
		conn.Lock()
		if conn.consoleStreamTokens[gameServerID] == token {
			delete(conn.consoleStreamCancels, gameServerID)
			delete(conn.consoleStreamTokens, gameServerID)
		}
		conn.Unlock()
	}()

	backoff := 250 * time.Millisecond
	const maxBackoff = 5 * time.Second
	disconnectedNoticeSent := false
	for {
		if streamCtx.Err() != nil {
			return
		}

		client, errClient := ws.nodeRegistry.Get(nodeID)
		if errClient != nil {
			log.Warn().Err(errClient).Str("node_id", nodeID).Str("game_server_id", gameServerID).
				Dur("retry_in", backoff).Msg("Console node client is unavailable; will re-resolve")
			if !disconnectedNoticeSent {
				disconnectedNoticeSent = ws.forwardConsoleConnectionState(streamCtx, conn, gameServerID, true)
			}
			if waitForConsoleRetry(streamCtx, backoff) {
				return
			}
			backoff = min(backoff*2, maxBackoff)
			continue
		}

		attemptCtx, attemptCancel := context.WithCancel(streamCtx)
		stream, errStream := client.StreamConsoleOutput(attemptCtx, gameServerID)
		if errStream != nil {
			attemptCancel()
			log.Warn().Err(errStream).Str("node_id", nodeID).Str("game_server_id", gameServerID).
				Dur("retry_in", backoff).Msg("Console stream failed; will retry")
			if !disconnectedNoticeSent {
				disconnectedNoticeSent = ws.forwardConsoleConnectionState(streamCtx, conn, gameServerID, true)
			}
			if waitForConsoleRetry(streamCtx, backoff) {
				return
			}
			backoff = min(backoff*2, maxBackoff)
			continue
		}
		if !ws.forwardConsoleConnectionState(streamCtx, conn, gameServerID, false) {
			attemptCancel()
			log.Warn().Str("node_id", nodeID).Str("game_server_id", gameServerID).
				Dur("retry_in", backoff).Msg("Console connection-state update was backpressured; will retry")
			if waitForConsoleRetry(streamCtx, backoff) {
				return
			}
			backoff = min(backoff*2, maxBackoff)
			continue
		}
		disconnectedNoticeSent = false
		backoff = 250 * time.Millisecond

		receivedOutput := false
		for {
			select {
			case <-streamCtx.Done():
				attemptCancel()
				return
			case chunk, ok := <-stream:
				if !ok {
					attemptCancel()
					goto retry
				}

				processID := chunk.ProcessID
				if processID == "" {
					processID = gameServerID
				}

				payload := &xylona.Message{
					Type: xylona.Message_GameServerConsole,
					GameServerConsoleOutput: &xylona.GameServerConsoleOutput{
						GameServerId: processID,
						Output:       chunk.Data,
						Sequence:     chunk.Sequence,
						ResetBuffer:  chunk.ResetBuffer,
					},
				}

				select {
				case <-streamCtx.Done():
					attemptCancel()
					return
				case conn.outputStreamChannel <- payload:
					receivedOutput = true
					disconnectedNoticeSent = false
					backoff = 250 * time.Millisecond
				case <-time.After(time.Second):
					log.Warn().Str("game_server_id", gameServerID).
						Msg("Console websocket consumer is slow; restarting stream for a reset replay")
					attemptCancel()
					goto retry
				}
			}
		}

	retry:
		if !disconnectedNoticeSent {
			disconnectedNoticeSent = ws.forwardConsoleConnectionState(streamCtx, conn, gameServerID, true)
		}
		if !receivedOutput {
			backoff = min(backoff*2, maxBackoff)
		}
		if waitForConsoleRetry(streamCtx, backoff) {
			return
		}
	}
}

func (ws *WebSocket) forwardConsoleConnectionState(
	ctx context.Context,
	conn *connection,
	gameServerID string,
	reconnecting bool,
) bool {
	output := ""
	if reconnecting {
		output = fmt.Sprintf(
			"[%s] [Xylona]: Console connection lost; reconnecting...\n",
			time.Now().Format("2006-01-02 15:04:05"),
		)
	}
	reconnectingState := reconnecting
	payload := &xylona.Message{
		Type: xylona.Message_GameServerConsole,
		GameServerConsoleOutput: &xylona.GameServerConsoleOutput{
			GameServerId: gameServerID,
			Output:       output,
			Reconnecting: &reconnectingState,
		},
	}
	select {
	case <-ctx.Done():
		return false
	case conn.outputStreamChannel <- payload:
		return true
	case <-time.After(time.Second):
		log.Warn().Str("game_server_id", gameServerID).Bool("reconnecting", reconnecting).
			Msg("Unable to deliver console connection state")
		return false
	}
}

func waitForConsoleRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return true
	case <-timer.C:
		return false
	}
}
