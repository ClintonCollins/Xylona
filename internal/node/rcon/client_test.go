package rcon

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestClientExecute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		protocol Protocol
		want     string
	}{
		{name: "Source multi-packet response", protocol: ProtocolSource, want: "first second"},
		{name: "Minecraft idle-delimited response", protocol: ProtocolMinecraft, want: "minecraft response"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			listenConfig := net.ListenConfig{}
			listener, errListen := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
			if errListen != nil {
				t.Fatalf("Listen() error = %v", errListen)
			}
			t.Cleanup(func() {
				errClose := listener.Close()
				if errClose != nil && !errors.Is(errClose, net.ErrClosed) {
					t.Errorf("listener.Close() error = %v", errClose)
				}
			})

			serverResult := make(chan error, 1)
			go func() {
				connection, errAccept := listener.Accept()
				if errAccept != nil {
					serverResult <- fmt.Errorf("accept: %w", errAccept)
					return
				}
				serverResult <- serveTestRCONConnection(connection, tc.protocol)
			}()

			client := Client{
				Address:  listener.Addr().String(),
				Password: "test-password",
				Protocol: tc.protocol,
				Timeout:  2 * time.Second,
			}
			response, errExecute := client.Execute(t.Context(), "status")
			if errExecute != nil {
				t.Fatalf("Execute() error = %v", errExecute)
			}
			if response != tc.want {
				t.Fatalf("Execute() response = %q, want %q", response, tc.want)
			}
			errServer := <-serverResult
			if errServer != nil {
				t.Fatalf("RCON test server error = %v", errServer)
			}
		})
	}
}

func TestClientExecuteRustWeb(t *testing.T) {
	t.Parallel()

	serverResult := make(chan error, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/test-password" {
			serverResult <- fmt.Errorf("request path = %q", request.URL.Path)
			return
		}
		connection, errUpgrade := upgrader.Upgrade(writer, request, nil)
		if errUpgrade != nil {
			serverResult <- fmt.Errorf("upgrade: %w", errUpgrade)
			return
		}
		defer func() {
			errClose := connection.Close()
			if errClose != nil {
				serverResult <- fmt.Errorf("close WebRCON: %w", errClose)
			}
		}()

		var command rustWebRequest
		errRead := connection.ReadJSON(&command)
		if errRead != nil {
			serverResult <- fmt.Errorf("read WebRCON command: %w", errRead)
			return
		}
		if command.Identifier != 1 || command.Message != "status" || command.Name != "Xylona" {
			serverResult <- fmt.Errorf("WebRCON command = %+v", command)
			return
		}
		errWrite := connection.WriteJSON(struct {
			Identifier int64  `json:"Identifier"`
			Message    string `json:"Message"`
		}{Identifier: 1, Message: "rust response"})
		if errWrite != nil {
			serverResult <- fmt.Errorf("write WebRCON response: %w", errWrite)
			return
		}
		serverResult <- nil
	}))
	t.Cleanup(server.Close)

	client := Client{
		Address:  strings.TrimPrefix(server.URL, "http://"),
		Password: "test-password",
		Protocol: ProtocolRustWeb,
		Timeout:  2 * time.Second,
	}
	response, errExecute := client.Execute(t.Context(), "status")
	if errExecute != nil {
		t.Fatalf("Execute() error = %v", errExecute)
	}
	if response != "rust response" {
		t.Fatalf("Execute() response = %q", response)
	}
	errServer := <-serverResult
	if errServer != nil {
		t.Fatalf("WebRCON server error = %v", errServer)
	}
}

func serveTestRCONConnection(connection net.Conn, protocol Protocol) (err error) {
	defer func() {
		errClose := connection.Close()
		if errClose != nil {
			err = errors.Join(err, fmt.Errorf("close connection: %w", errClose))
		}
	}()

	auth, errReadAuth := readPacket(connection)
	if errReadAuth != nil {
		return fmt.Errorf("read auth: %w", errReadAuth)
	}
	if auth.id != 1 || auth.kind != packetTypeAuth || auth.body != "test-password" {
		return fmt.Errorf("unexpected auth packet: %+v", auth)
	}
	errWriteAuth := writePacket(connection, packet{id: 1, kind: packetTypeAuthResponse})
	if errWriteAuth != nil {
		return fmt.Errorf("write auth response: %w", errWriteAuth)
	}

	command, errReadCommand := readPacket(connection)
	if errReadCommand != nil {
		return fmt.Errorf("read command: %w", errReadCommand)
	}
	if command.id != 2 || command.kind != packetTypeExecuteCommand || command.body != "status" {
		return fmt.Errorf("unexpected command packet: %+v", command)
	}

	switch protocol {
	case ProtocolSource:
		for _, response := range []packet{
			{id: 2, kind: packetTypeResponseValue, body: "first "},
			{id: 2, kind: packetTypeResponseValue, body: "second"},
		} {
			errWrite := writePacket(connection, response)
			if errWrite != nil {
				return fmt.Errorf("write Source response: %w", errWrite)
			}
		}
	case ProtocolMinecraft:
		errWrite := writePacket(connection, packet{id: 2, kind: packetTypeResponseValue, body: "minecraft response"})
		if errWrite != nil {
			return fmt.Errorf("write Minecraft response: %w", errWrite)
		}
		time.Sleep(2 * responseIdleTimeout)
	default:
		return errors.New("unsupported test protocol")
	}
	return nil
}
