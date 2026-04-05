package websocket

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/olahol/melody"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestWebSocket_UnauthenticatedSessionDoesNotLogErrors(t *testing.T) {
	ws := &WebSocket{
		userWebsocketConnections:     make(map[string]map[uuid.UUID]*connection),
		userWebsocketConnectionsLock: &sync.RWMutex{},
	}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/websocket", nil)
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("User-Agent", "test-agent")
	session := &melody.Session{Request: request}

	var logOutput bytes.Buffer
	previousLogger := log.Logger
	log.Logger = zerolog.New(&logOutput).Level(zerolog.DebugLevel)
	defer func() {
		log.Logger = previousLogger
	}()

	ws.handleConnect(session)
	ws.handleDisconnect(session)

	if strings.Contains(logOutput.String(), `"level":"error"`) {
		t.Fatalf("expected no error logs for unauthenticated websocket session, got logs: %s", logOutput.String())
	}
	if !strings.Contains(logOutput.String(), `"method":"GET"`) {
		t.Fatalf("expected rejection log to include method, got logs: %s", logOutput.String())
	}
	if !strings.Contains(logOutput.String(), `"url":"/api/websocket"`) {
		t.Fatalf("expected rejection log to include url, got logs: %s", logOutput.String())
	}
	if !strings.Contains(logOutput.String(), `"remote_addr":"127.0.0.1:54321"`) {
		t.Fatalf("expected rejection log to include remote address, got logs: %s", logOutput.String())
	}
	if !strings.Contains(logOutput.String(), `"user_agent":"test-agent"`) {
		t.Fatalf("expected rejection log to include user agent, got logs: %s", logOutput.String())
	}
}
