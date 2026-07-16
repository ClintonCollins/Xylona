package query

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"slices"
	"testing"
	"time"
)

func TestSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		playerResponse          []byte
		wantPlayerList          []string
		wantPlayerListSupported bool
	}{
		{
			name:                    "returns player names when A2S player query succeeds",
			playerResponse:          sourcePlayerResponse("Alyx", "Gordon"),
			wantPlayerList:          []string{"Alyx", "Gordon"},
			wantPlayerListSupported: true,
		},
		{
			name:                    "preserves server info when A2S player query is unsupported",
			playerResponse:          []byte{0xff, 0xff, 0xff, 0xff, 0x45},
			wantPlayerList:          nil,
			wantPlayerListSupported: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			host, port, waitForServer := startSourceQueryTestServer(t, tt.playerResponse)
			result, errQuery := Source(host, port)
			waitForServer()
			if errQuery != nil {
				t.Fatalf("Source: %v", errQuery)
			}
			if result.GetName() != "Test Source Server" || result.GetPlayers() != 2 || result.GetMaxPlayers() != 16 {
				t.Fatalf("Source server info = %+v, want configured response", result)
			}
			if !slices.Equal(result.GetPlayerList(), tt.wantPlayerList) {
				t.Fatalf("Source player list = %v, want %v", result.GetPlayerList(), tt.wantPlayerList)
			}
			if result.GetPlayerListSupported() != tt.wantPlayerListSupported {
				t.Fatalf("Source player list supported = %v, want %v", result.GetPlayerListSupported(), tt.wantPlayerListSupported)
			}
		})
	}
}

func startSourceQueryTestServer(t *testing.T, playerResponse []byte) (string, int, func()) {
	t.Helper()

	conn, errListen := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if errListen != nil {
		t.Fatalf("ListenUDP: %v", errListen)
	}
	t.Cleanup(func() {
		errClose := conn.Close()
		if errClose != nil && !errors.Is(errClose, net.ErrClosed) {
			t.Errorf("close source query test server: %v", errClose)
		}
	})

	serverDone := make(chan error, 1)
	responses := [][]byte{sourceInfoResponse(), playerResponse}
	go func() {
		requestBuffer := make([]byte, 1400)
		for _, response := range responses {
			_, remoteAddress, errRead := conn.ReadFromUDP(requestBuffer)
			if errRead != nil {
				serverDone <- fmt.Errorf("read source query request: %w", errRead)
				return
			}
			_, errWrite := conn.WriteToUDP(response, remoteAddress)
			if errWrite != nil {
				serverDone <- fmt.Errorf("write source query response: %w", errWrite)
				return
			}
		}
		serverDone <- nil
	}()

	address, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		t.Fatalf("source query test address type = %T, want *net.UDPAddr", conn.LocalAddr())
	}

	waitForServer := func() {
		t.Helper()
		select {
		case errServer := <-serverDone:
			if errServer != nil {
				t.Fatalf("source query test server: %v", errServer)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for source query test server")
		}
	}

	return address.IP.String(), address.Port, waitForServer
}

func sourceInfoResponse() []byte {
	response := []byte{0xff, 0xff, 0xff, 0xff, 0x49, 0x11}
	response = appendCString(response, "Test Source Server")
	response = appendCString(response, "de_dust2")
	response = appendCString(response, "csgo")
	response = appendCString(response, "Counter-Strike 2")
	response = binary.LittleEndian.AppendUint16(response, 730)
	response = append(response, 2, 16, 0, 'd', 'l', 0, 1)
	response = appendCString(response, "1.0.0")
	return response
}

func sourcePlayerResponse(names ...string) []byte {
	if len(names) > 255 {
		panic("source player response supports at most 255 players")
	}

	playerCount := byte(0)
	for range names {
		playerCount++
	}

	response := []byte{0xff, 0xff, 0xff, 0xff, 0x44, playerCount}
	for index, name := range names {
		response = append(response, byte(index))
		response = appendCString(response, name)
		response = binary.LittleEndian.AppendUint32(response, 0)
		response = binary.LittleEndian.AppendUint32(response, math.Float32bits(60))
	}
	return response
}

func appendCString(buffer []byte, value string) []byte {
	buffer = append(buffer, value...)
	return append(buffer, 0)
}
