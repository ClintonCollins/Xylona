package websocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/olahol/melody"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func newConnectedMelodySession(t *testing.T) (*melody.Session, *websocket.Conn, func()) {
	t.Helper()

	m := melody.New()
	connectedSession := make(chan *melody.Session, 1)
	m.HandleConnect(func(s *melody.Session) {
		connectedSession <- s
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errHandle := m.HandleRequest(w, r)
		if errHandle != nil {
			t.Errorf("HandleRequest() error = %v", errHandle)
		}
	}))

	wsURL := `ws` + strings.TrimPrefix(server.URL, `http`)
	clientConn, response, errDial := websocket.DefaultDialer.Dial(wsURL, nil)
	if response != nil {
		_ = response.Body.Close()
	}
	if errDial != nil {
		server.Close()
		t.Fatalf("Dial(%q) error = %v", wsURL, errDial)
	}

	var session *melody.Session
	select {
	case session = <-connectedSession:
	case <-time.After(2 * time.Second):
		_ = clientConn.Close()
		server.Close()
		t.Fatal("timed out waiting for Melody session")
	}

	cleanup := func() {
		_ = clientConn.Close()
		_ = m.Close()
		server.Close()
	}

	return session, clientConn, cleanup
}

func readGameServerStatusMessage(t *testing.T, clientConn *websocket.Conn) *xylona.Message {
	t.Helper()

	errDeadline := clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if errDeadline != nil {
		t.Fatalf("SetReadDeadline() error = %v", errDeadline)
	}

	_, payload, errRead := clientConn.ReadMessage()
	if errRead != nil {
		t.Fatalf("ReadMessage() error = %v", errRead)
	}

	message := &xylona.Message{}
	errUnmarshal := protojson.Unmarshal(payload, message)
	if errUnmarshal != nil {
		t.Fatalf("protojson.Unmarshal() error = %v", errUnmarshal)
	}

	return message
}

func expectNoWebsocketMessage(t *testing.T, clientConn *websocket.Conn) {
	t.Helper()

	errDeadline := clientConn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
	if errDeadline != nil {
		t.Fatalf("SetReadDeadline() error = %v", errDeadline)
	}

	_, _, errRead := clientConn.ReadMessage()
	if errRead == nil {
		t.Fatal("ReadMessage() succeeded, want timeout with no websocket message")
	}

	netErr, ok := errRead.(interface{ Timeout() bool })
	if !ok || !netErr.Timeout() {
		t.Fatalf("ReadMessage() error = %v, want timeout", errRead)
	}
}

func TestWebSocket_LocalStatusChangeBroadcastsToConnectedObservers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session, clientConn, cleanup := newConnectedMelodySession(t)
	defer cleanup()

	ws := &WebSocket{
		ctx:                          ctx,
		userWebsocketConnections:     make(map[string]map[uuid.UUID]*connection),
		userWebsocketConnectionsLock: &sync.RWMutex{},
	}

	allowed := newTestConnection()
	allowed.userID = "user-1"
	allowed.allGameServerIDs = []string{"server-1"}
	allowed.melodySession = session

	ws.userWebsocketConnections[allowed.userID] = map[uuid.UUID]*connection{
		allowed.id: allowed,
	}

	statusChanged := make(chan any, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ws.listenForLocalGameServerStatusChanges(statusChanged)
	}()

	statusChanged <- eventbus.StatusChangedEvent{
		ServerID:   "server-1",
		ServerName: "Alpha",
		OldStatus:  xylona.Status_OFFLINE.String(),
		NewStatus:  xylona.Status_ONLINE.String(),
	}

	message := readGameServerStatusMessage(t, clientConn)
	if message.GetType() != xylona.Message_GameServerStatus {
		t.Fatalf("message type = %v, want %v", message.GetType(), xylona.Message_GameServerStatus)
	}

	update := message.GetGameServerStatusUpdate()
	if update == nil {
		t.Fatal("GameServerStatusUpdate = nil, want status payload")
	}
	if update.GetGameServerId() != "server-1" {
		t.Fatalf("GameServerId = %q, want %q", update.GetGameServerId(), "server-1")
	}
	if update.GetGameServerName() != "Alpha" {
		t.Fatalf("GameServerName = %q, want %q", update.GetGameServerName(), "Alpha")
	}
	if update.GetStatus() != xylona.Status_ONLINE {
		t.Fatalf("Status = %v, want %v", update.GetStatus(), xylona.Status_ONLINE)
	}

	cancel()
	<-done
}

func TestWebSocket_LocalStatusChangeSkipsUnauthorizedConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	allowedSession, allowedClient, allowedCleanup := newConnectedMelodySession(t)
	defer allowedCleanup()

	deniedSession, deniedClient, deniedCleanup := newConnectedMelodySession(t)
	defer deniedCleanup()

	ws := &WebSocket{
		ctx:                          ctx,
		userWebsocketConnections:     make(map[string]map[uuid.UUID]*connection),
		userWebsocketConnectionsLock: &sync.RWMutex{},
	}

	allowed := newTestConnection()
	allowed.userID = "user-1"
	allowed.allGameServerIDs = []string{"server-1"}
	allowed.melodySession = allowedSession

	denied := newTestConnection()
	denied.userID = "user-2"
	denied.allGameServerIDs = []string{"server-2"}
	denied.melodySession = deniedSession

	ws.userWebsocketConnections[allowed.userID] = map[uuid.UUID]*connection{
		allowed.id: allowed,
	}
	ws.userWebsocketConnections[denied.userID] = map[uuid.UUID]*connection{
		denied.id: denied,
	}

	statusChanged := make(chan any, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ws.listenForLocalGameServerStatusChanges(statusChanged)
	}()

	statusChanged <- eventbus.StatusChangedEvent{
		ServerID:   "server-1",
		ServerName: "Alpha",
		OldStatus:  xylona.Status_OFFLINE.String(),
		NewStatus:  xylona.Status_ONLINE.String(),
	}

	message := readGameServerStatusMessage(t, allowedClient)
	if message.GetGameServerStatusUpdate().GetGameServerId() != "server-1" {
		t.Fatalf("GameServerId = %q, want %q", message.GetGameServerStatusUpdate().GetGameServerId(), "server-1")
	}
	if message.GetGameServerStatusUpdate().GetGameServerName() != "Alpha" {
		t.Fatalf("GameServerName = %q, want %q", message.GetGameServerStatusUpdate().GetGameServerName(), "Alpha")
	}

	expectNoWebsocketMessage(t, deniedClient)

	cancel()
	<-done
}

// StatusChangedEvent is now source-agnostic, so status broadcast coverage lives
// in the authorization tests above.
