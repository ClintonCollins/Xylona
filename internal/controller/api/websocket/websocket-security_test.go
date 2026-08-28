package websocket

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/olahol/melody"

	"github.com/ClintonCollins/Xylona/internal/controller/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestNewInstanceChecksEffectiveSameOrigin(t *testing.T) {
	trust, errTrust := gatekeeper.ParseTrustedProxies("127.0.0.1")
	if errTrust != nil {
		t.Fatalf("ParseTrustedProxies() error = %v", errTrust)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	ws, _ := NewInstance(ctx, nil, nil, nil, nil, trust)

	tests := []struct {
		name           string
		origin         string
		forwardedHost  string
		forwardedProto string
		want           bool
	}{
		{
			name:           "trusted effective origin",
			origin:         "https://xylona.test",
			forwardedHost:  "xylona.test",
			forwardedProto: "https",
			want:           true,
		},
		{
			name:           "same-site sibling origin",
			origin:         "https://sibling.xylona.test",
			forwardedHost:  "xylona.test",
			forwardedProto: "https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://internal.proxy/api/websocket", nil)
			req.Host = "internal.proxy"
			req.RemoteAddr = "127.0.0.1:443"
			req.Header.Set("Origin", tt.origin)
			req.Header.Set("X-Forwarded-Host", tt.forwardedHost)
			req.Header.Set("X-Forwarded-Proto", tt.forwardedProto)

			got := ws.melody.Upgrader.CheckOrigin(req)
			if got != tt.want {
				t.Fatalf("CheckOrigin() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestWebSocketRevalidatesSessionOnInboundMessage(t *testing.T) {
	t.Run("valid session receives heartbeat response", func(t *testing.T) {
		session, clientConn, cleanup := newConnectedMelodySession(t)
		defer cleanup()

		conn := newTestConnection()
		conn.userID = "user-1"
		conn.melodySession = session
		conn.sessionUserLookup = func() (*models.User, error) {
			return &models.User{ID: "user-1"}, nil
		}
		ws := websocketWithConnection(session, conn)

		ws.handleMessage(session, []byte("ping"))
		errDeadline := clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if errDeadline != nil {
			t.Fatalf("SetReadDeadline() error = %v", errDeadline)
		}
		_, payload, errRead := clientConn.ReadMessage()
		if errRead != nil {
			t.Fatalf("ReadMessage() error = %v", errRead)
		}
		if string(payload) != "pong" {
			t.Fatalf("heartbeat response = %q, want pong", payload)
		}
	})

	t.Run("deleted session closes connection", func(t *testing.T) {
		session, clientConn, cleanup := newConnectedMelodySession(t)
		defer cleanup()

		conn := newTestConnection()
		conn.userID = "user-1"
		conn.melodySession = session
		conn.sessionUserLookup = func() (*models.User, error) {
			return nil, errors.New("session does not exist")
		}
		ws := websocketWithConnection(session, conn)

		ws.handleMessage(session, []byte("ping"))
		expectSessionExpiredClose(t, clientConn)
	})
}

func TestWebSocketRevalidatesPassiveSession(t *testing.T) {
	session, clientConn, cleanup := newConnectedMelodySession(t)
	defer cleanup()

	conn := newTestConnection()
	conn.userID = "user-1"
	conn.melodySession = session
	conn.outputStreamChannel = make(chan *xylona.Message)
	conn.sessionUserValidation = func() (*models.User, error) {
		return nil, errors.New("session expired")
	}
	ws := websocketWithConnection(session, conn)
	ws.ctx = t.Context()
	ws.sessionValidationInterval = 10 * time.Millisecond

	go ws.handleUserWebsocketConnection(session, &models.User{ID: "user-1", UserName: "user"}, conn)
	expectSessionExpiredClose(t, clientConn)
}

func TestWebSocketCloseScopes(t *testing.T) {
	sessionA, clientA, cleanupA := newConnectedMelodySession(t)
	defer cleanupA()
	sessionB, clientB, cleanupB := newConnectedMelodySession(t)
	defer cleanupB()
	sessionC, clientC, cleanupC := newConnectedMelodySession(t)
	defer cleanupC()

	connA := newTestConnection()
	connA.userID = "user-1"
	connA.sessionID = "session-a"
	connA.melodySession = sessionA
	streamCtx, cancelStream := context.WithCancel(t.Context())
	connA.consoleStreamCancels["server-1"] = cancelStream
	connB := newTestConnection()
	connB.userID = "user-1"
	connB.sessionID = "session-b"
	connB.melodySession = sessionB
	connC := newTestConnection()
	connC.userID = "user-2"
	connC.sessionID = "session-c"
	connC.melodySession = sessionC

	ws := &WebSocket{
		userWebsocketConnections: map[string]map[uuid.UUID]*connection{
			"user-1": {connA.id: connA, connB.id: connB},
			"user-2": {connC.id: connC},
		},
		userWebsocketConnectionsLock: &sync.RWMutex{},
	}

	ws.CloseSession("session-a")
	expectPolicyViolationClose(t, clientA)
	select {
	case <-streamCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("CloseSession() did not cancel the connection console stream")
	}

	ws.CloseUser("user-1")
	expectPolicyViolationClose(t, clientB)

	ws.CloseAll()
	expectPolicyViolationClose(t, clientC)
}

func websocketWithConnection(session *melody.Session, conn *connection) *WebSocket {
	session.Set(sessionKeyConnectionID, conn.id)
	session.Set(sessionKeyUserID, conn.userID)
	session.Set(sessionKeyUserName, "user")
	return &WebSocket{
		userWebsocketConnections: map[string]map[uuid.UUID]*connection{
			conn.userID: {conn.id: conn},
		},
		userWebsocketConnectionsLock: &sync.RWMutex{},
	}
}

func expectPolicyViolationClose(t *testing.T, conn *gorillawebsocket.Conn) {
	expectWebSocketClose(t, conn, gorillawebsocket.ClosePolicyViolation)
}

func expectSessionExpiredClose(t *testing.T, conn *gorillawebsocket.Conn) {
	expectWebSocketClose(t, conn, sessionExpiredCloseCode)
}

func expectWebSocketClose(t *testing.T, conn *gorillawebsocket.Conn, wantCode int) {
	t.Helper()
	errDeadline := conn.SetReadDeadline(time.Now().Add(time.Second))
	if errDeadline != nil {
		t.Fatalf("SetReadDeadline() error = %v", errDeadline)
	}
	_, _, errRead := conn.ReadMessage()
	closeErr, ok := errors.AsType[*gorillawebsocket.CloseError](errRead)
	if !ok {
		t.Fatalf("ReadMessage() error = %v, want websocket close", errRead)
	}
	if closeErr.Code != wantCode {
		t.Fatalf("close code = %d, want %d", closeErr.Code, wantCode)
	}
}
