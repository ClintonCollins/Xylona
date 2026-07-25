package restinput

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestExecutePalworld(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		command      string
		wantMethod   string
		wantPath     string
		wantPayload  map[string]any
		responseBody string
		wantOutput   string
	}{
		{
			name:         "gets formatted server information case insensitively",
			command:      "  /iNfO  ",
			wantMethod:   http.MethodGet,
			wantPath:     "/v1/api/info",
			responseBody: `{"version":"v1.2.3","servername":"Xylona Palworld","description":"Test world","worldguid":"WORLD-1","ip":"203.0.113.9"}`,
			wantOutput: "Server: Xylona Palworld\nVersion: v1.2.3\n" +
				"Description: Test world\nWorld GUID: WORLD-1",
		},
		{
			name:       "gets a deterministic display safe player list",
			command:    "/ShowPlayers",
			wantMethod: http.MethodGet,
			wantPath:   "/v1/api/players",
			responseBody: `{"players":[` +
				`{"name":"Zoe","accountName":"zoe-account","playerId":"player-2","userId":"steam_2","ip":"203.0.113.2","level":12},` +
				`{"name":"Alice","accountName":"alice-account","playerId":"player-1","userId":"steam_1","ip":"203.0.113.1","level":20}` +
				`]}`,
			wantOutput: "Players online: 2\n" +
				"- Alice | Account: alice-account | Player ID: player-1 | User ID: steam_1 | Level: 20\n" +
				"- Zoe | Account: zoe-account | Player ID: player-2 | User ID: steam_2 | Level: 12",
		},
		{
			name:        "broadcasts a whitespace normalized message",
			command:     " /Broadcast   Server\t restart soon ",
			wantMethod:  http.MethodPost,
			wantPath:    "/v1/api/announce",
			wantPayload: map[string]any{"message": "Server restart soon"},
			wantOutput:  "Broadcast sent.",
		},
		{
			name:       "kicks a player with an optional message",
			command:    "/KickPlayer steam_1 AFK too long",
			wantMethod: http.MethodPost,
			wantPath:   "/v1/api/kick",
			wantPayload: map[string]any{
				"userid":  "steam_1",
				"message": "AFK too long",
			},
			wantOutput: "Player kick requested.",
		},
		{
			name:        "bans a player without a message",
			command:     "/BanPlayer steam_2",
			wantMethod:  http.MethodPost,
			wantPath:    "/v1/api/ban",
			wantPayload: map[string]any{"userid": "steam_2"},
			wantOutput:  "Player ban requested.",
		},
		{
			name:        "unbans a player",
			command:     "/UnBanPlayer steam_3",
			wantMethod:  http.MethodPost,
			wantPath:    "/v1/api/unban",
			wantPayload: map[string]any{"userid": "steam_3"},
			wantOutput:  "Player unban requested.",
		},
		{
			name:       "saves the world without a request body",
			command:    "/Save",
			wantMethod: http.MethodPost,
			wantPath:   "/v1/api/save",
			wantOutput: "World save requested.",
		},
		{
			name:       "schedules shutdown with an optional message",
			command:    "/Shutdown 30 Maintenance begins",
			wantMethod: http.MethodPost,
			wantPath:   "/v1/api/shutdown",
			wantPayload: map[string]any{
				"waittime": float64(30),
				"message":  "Maintenance begins",
			},
			wantOutput: "Server shutdown requested in 30 seconds.",
		},
		{
			name:       "force stops the server without a request body",
			command:    "/DoExit",
			wantMethod: http.MethodPost,
			wantPath:   "/v1/api/stop",
			wantOutput: "Immediate server stop requested.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != tc.wantMethod || request.URL.Path != tc.wantPath {
					t.Errorf(
						"request = %s %s, want %s %s",
						request.Method,
						request.URL.Path,
						tc.wantMethod,
						tc.wantPath,
					)
				}
				username, password, ok := request.BasicAuth()
				if !ok || username != "admin" || password != "custom-admin-password" {
					t.Errorf("BasicAuth() = %q/%q/%t, want admin/custom-admin-password/true", username, password, ok)
				}
				if request.Header.Get("Accept") != "application/json" {
					t.Errorf("Accept = %q, want application/json", request.Header.Get("Accept"))
				}

				if tc.wantPayload == nil {
					if request.Body != nil && request.ContentLength != 0 {
						t.Errorf("request body length = %d, want 0", request.ContentLength)
					}
				} else {
					var payload map[string]any
					errDecode := json.NewDecoder(request.Body).Decode(&payload)
					if errDecode != nil {
						t.Errorf("decode request body: %v", errDecode)
					}
					if fmt.Sprint(payload) != fmt.Sprint(tc.wantPayload) {
						t.Errorf("payload = %+v, want %+v", payload, tc.wantPayload)
					}
					if request.Header.Get("Content-Type") != "application/json" {
						t.Errorf("Content-Type = %q, want application/json", request.Header.Get("Content-Type"))
					}
				}
				if tc.responseBody != "" {
					writer.Header().Set("Content-Type", "application/json")
					_, errWrite := fmt.Fprint(writer, tc.responseBody)
					if errWrite != nil {
						t.Errorf("write response: %v", errWrite)
					}
					return
				}
				writer.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(server.Close)
			host, port := palworldRESTTestAddress(t, server.URL)

			output, errExecute := ExecutePalworld(
				t.Context(),
				host,
				port,
				"custom-admin-password",
				tc.command,
			)
			if errExecute != nil {
				t.Fatalf("ExecutePalworld() error = %v", errExecute)
			}
			if output != tc.wantOutput {
				t.Fatalf("ExecutePalworld() output = %q, want %q", output, tc.wantOutput)
			}
			if strings.Contains(output, "203.0.113.") {
				t.Fatalf("ExecutePalworld() output leaked an IP address: %q", output)
			}
		})
	}
}

func TestExecutePalworldValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		host     string
		port     int
		password string
		command  string
		wantErr  string
		rejected bool
	}{
		{name: "rejects empty command", host: "127.0.0.1", port: 8212, password: "secret", wantErr: "command is empty", rejected: true},
		{name: "requires leading slash", host: "127.0.0.1", port: 8212, password: "secret", command: "Info", wantErr: "must begin with a slash", rejected: true},
		{name: "rejects unsupported command", host: "127.0.0.1", port: 8212, password: "secret", command: "/TeleportToMe steam_1", wantErr: "unsupported Palworld command", rejected: true},
		{name: "requires broadcast message", host: "127.0.0.1", port: 8212, password: "secret", command: "/Broadcast", wantErr: "missing required arguments", rejected: true},
		{name: "requires kick user ID", host: "127.0.0.1", port: 8212, password: "secret", command: "/KickPlayer", wantErr: "missing required arguments", rejected: true},
		{name: "rejects extra unban arguments", host: "127.0.0.1", port: 8212, password: "secret", command: "/UnBanPlayer steam_1 reason", wantErr: "unexpected arguments", rejected: true},
		{name: "requires shutdown wait time", host: "127.0.0.1", port: 8212, password: "secret", command: "/Shutdown", wantErr: "missing required arguments", rejected: true},
		{name: "rejects nonnumeric shutdown wait time", host: "127.0.0.1", port: 8212, password: "secret", command: "/Shutdown soon", wantErr: "non-negative integer", rejected: true},
		{name: "rejects negative shutdown wait time", host: "127.0.0.1", port: 8212, password: "secret", command: "/Shutdown -1", wantErr: "non-negative integer", rejected: true},
		{name: "rejects invalid host", host: "palworld.example.com", port: 8212, password: "secret", command: "/Save", wantErr: "host is invalid"},
		{name: "rejects invalid port", host: "127.0.0.1", port: 65536, password: "secret", command: "/Save", wantErr: "port is invalid"},
		{name: "rejects blank password", host: "127.0.0.1", port: 8212, password: " \t ", command: "/Save", wantErr: "password is empty"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, errExecute := ExecutePalworld(
				t.Context(),
				tc.host,
				tc.port,
				tc.password,
				tc.command,
			)
			if errExecute == nil || !strings.Contains(errExecute.Error(), tc.wantErr) {
				t.Fatalf("ExecutePalworld() error = %v, want containing %q", errExecute, tc.wantErr)
			}
			if errors.Is(errExecute, ErrPalworldCommandRejected) != tc.rejected {
				t.Fatalf(
					"ExecutePalworld() rejected = %t, want %t (error %v)",
					errors.Is(errExecute, ErrPalworldCommandRejected),
					tc.rejected,
					errExecute,
				)
			}
		})
	}
}

func TestExecutePalworldResponseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		status       int
		responseBody string
		command      string
		wantErr      string
	}{
		{
			name:         "includes bounded error response text and redacts password",
			status:       http.StatusBadRequest,
			responseBody: "invalid request for super-secret",
			command:      "/Save",
			wantErr:      "400 Bad Request: invalid request for [redacted]",
		},
		{
			name:         "reports malformed server information",
			status:       http.StatusOK,
			responseBody: "{not-json}",
			command:      "/Info",
			wantErr:      "decode Palworld server info",
		},
		{
			name:         "reports malformed player list",
			status:       http.StatusOK,
			responseBody: "{not-json}",
			command:      "/ShowPlayers",
			wantErr:      "decode Palworld player list",
		},
		{
			name:         "rejects oversized response",
			status:       http.StatusOK,
			responseBody: strings.Repeat("x", maxResponseBytes+1),
			command:      "/Save",
			wantErr:      "response exceeds size limit",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(tc.status)
				_, errWrite := fmt.Fprint(writer, tc.responseBody)
				if errWrite != nil {
					t.Errorf("write response: %v", errWrite)
				}
			}))
			t.Cleanup(server.Close)
			host, port := palworldRESTTestAddress(t, server.URL)

			_, errExecute := ExecutePalworld(t.Context(), host, port, "super-secret", tc.command)
			if errExecute == nil || !strings.Contains(errExecute.Error(), tc.wantErr) {
				t.Fatalf("ExecutePalworld() error = %v, want containing %q", errExecute, tc.wantErr)
			}
			if !errors.Is(errExecute, ErrPalworldCommandRejected) {
				t.Fatalf("ExecutePalworld() error = %v, want ErrPalworldCommandRejected", errExecute)
			}
			if errExecute != nil && strings.Contains(errExecute.Error(), "super-secret") {
				t.Fatalf("ExecutePalworld() error leaked password: %v", errExecute)
			}
		})
	}
}

func TestExecutePalworldBoundsRejectedCommandDetails(t *testing.T) {
	t.Parallel()

	command := "/" + strings.Repeat("x", 2048)
	_, errExecute := ExecutePalworld(t.Context(), "127.0.0.1", 8212, "secret", command)
	if !errors.Is(errExecute, ErrPalworldCommandRejected) {
		t.Fatalf("ExecutePalworld() error = %v, want ErrPalworldCommandRejected", errExecute)
	}
	if len([]rune(errExecute.Error())) > 512 {
		t.Fatalf("ExecutePalworld() error runes = %d, want at most 512", len([]rune(errExecute.Error())))
	}
}

func TestExecutePalworldPreservesTransportAndContextErrors(t *testing.T) {
	t.Parallel()

	var listenConfig net.ListenConfig
	listener, errListen := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if errListen != nil {
		t.Fatalf("net.Listen() error = %v", errListen)
	}
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address = %T, want *net.TCPAddr", listener.Addr())
	}
	errClose := listener.Close()
	if errClose != nil {
		t.Fatalf("listener.Close() error = %v", errClose)
	}

	_, errExecute := ExecutePalworld(t.Context(), "127.0.0.1", address.Port, "secret", "/Save")
	if errExecute == nil || errors.Is(errExecute, ErrPalworldCommandRejected) {
		t.Fatalf("ExecutePalworld() error = %v, want unclassified transport error", errExecute)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, errCanceled := ExecutePalworld(ctx, "127.0.0.1", address.Port, "secret", "/Save")
	if !errors.Is(errCanceled, context.Canceled) {
		t.Fatalf("ExecutePalworld() canceled error = %v, want context.Canceled", errCanceled)
	}
	if errors.Is(errCanceled, ErrPalworldCommandRejected) {
		t.Fatalf("ExecutePalworld() canceled error = %v, must not be rejected", errCanceled)
	}
}

func TestPalworldHTTPTransportDisablesEnvironmentProxy(t *testing.T) {
	t.Parallel()

	transport, errTransport := palworldHTTPTransport()
	if errTransport != nil {
		t.Fatalf("palworldHTTPTransport() error = %v", errTransport)
	}
	t.Cleanup(transport.CloseIdleConnections)
	if transport.Proxy != nil {
		t.Fatal("palworldHTTPTransport().Proxy is set, want direct node-local transport")
	}
}

func TestExecutePalworldNormalizesUnspecifiedHost(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/api/save" {
			t.Errorf("path = %q, want /v1/api/save", request.URL.Path)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	_, port := palworldRESTTestAddress(t, server.URL)

	output, errExecute := ExecutePalworld(t.Context(), "0.0.0.0", port, "secret", "/Save")
	if errExecute != nil {
		t.Fatalf("ExecutePalworld() error = %v", errExecute)
	}
	if output != "World save requested." {
		t.Fatalf("ExecutePalworld() output = %q", output)
	}
}

func palworldRESTTestAddress(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	host, portText, errSplit := net.SplitHostPort(strings.TrimPrefix(rawURL, "http://"))
	if errSplit != nil {
		t.Fatalf("net.SplitHostPort() error = %v", errSplit)
	}
	port, errPort := strconv.Atoi(portText)
	if errPort != nil {
		t.Fatalf("strconv.Atoi() error = %v", errPort)
	}
	return host, port
}
