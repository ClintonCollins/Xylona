package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/olahol/melody"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/supervisor"
)

type WebSocket struct {
	melody     *melody.Melody
	supervisor *supervisor.Instance
	db         *db.Connection
	ctx        context.Context
}

func NewInstance(ctx context.Context, supervisorInst *supervisor.Instance, db *db.Connection) (*WebSocket, http.HandlerFunc) {
	m := melody.New()
	inst := &WebSocket{
		melody:     m,
		supervisor: supervisorInst,
		db:         db,
		ctx:        ctx,
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
	urlPath := strings.TrimPrefix(s.Request.URL.Path, "/api/websocket/")
	splitPath := strings.Split(urlPath, "/")
	// example path: /api/websocket/some_path
	if len(splitPath) < 1 {
		return
	}
	log.Debug().Msg("Websocket connected")
	requestType := splitPath[0]
	switch requestType {
	case "game_server_stream_output":
		if len(splitPath) < 2 {
			return
		}
		gameServerID := splitPath[1]
		go ws.handleGameServerStreamOutputRequest(s, gameServerID)
		return
	default:
		log.Debug().Msgf("Request type: %s", requestType)
		log.Debug().Msgf("Websocket URI: %s", s.Request.URL)
		log.Debug().Msg("Websocket connected")
	}
}

func (ws *WebSocket) handleGameServerStreamOutputRequest(s *melody.Session, gameServerID string) {
	gameServer, errGetGameServer := ws.db.GetGameServerByID(gameServerID)
	if errGetGameServer != nil {
		log.Error().Err(errGetGameServer).Msg("Failed to get game server by ID")
		return
	}
	command := ws.supervisor.GetCommandByIDOrCreateShell(gameServer.ID)
	streamChan := make(chan helpers.WebsocketOutputPayload)
	listenerID := uuid.NewString()
	command.AddOutputListener(listenerID, streamChan)
	defer command.RemoveOutputListener(listenerID)
	for {
		select {
		case <-ws.ctx.Done():
			log.Debug().Str("Game Server ID", gameServerID).Msg("Got Xylona shutdown signal. Closing websocket stream.")
			errClose := s.Close()
			if errClose != nil {
				log.Error().Err(errClose).Msg("Failed to close websocket connection")
			}
			return
		case <-s.Request.Context().Done():
			log.Debug().Str("Game Server ID", gameServerID).Msg("Websocket connection closed. Closing websocket stream.")
			errClose := s.Close()
			if errClose != nil {
				log.Error().Err(errClose).Msg("Failed to close websocket connection")
			}
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

func (ws *WebSocket) handleDisconnect(s *melody.Session) {
	log.Debug().Msg("Websocket disconnected")
	errClose := s.Close()
	if errClose != nil {
		log.Error().Err(errClose).Msg("Failed to close websocket connection")
	}
}

func (ws *WebSocket) handleMessage(s *melody.Session, msg []byte) {
	log.Debug().Msgf("Websocket message: %s", string(msg))
	errWrite := s.Write([]byte(fmt.Sprintf("echo: %s", msg)))
	if errWrite != nil {
		log.Error().Err(errWrite).Msg("Failed to write websocket message")
	}
}
