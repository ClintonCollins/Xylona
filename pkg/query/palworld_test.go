package query

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestPalworld(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		username   string
		password   string
		infoBody   string
		metricBody string
		playerBody string
		wantErr    string
	}{
		{
			name:       "returns live server data",
			username:   "admin",
			password:   "query-secret",
			infoBody:   `{"version":"v1.2.3","servername":"Xylona Palworld","description":"Test world","worldguid":"WORLD-1"}`,
			metricBody: `{"serverfps":59.5,"currentplayernum":2,"serverframetime":16.8,"maxplayernum":32,"uptime":1234,"days":7}`,
			playerBody: `{"players":[{"name":"Alice","userId":"steam_1"},{"name":"Bob","userId":"steam_2"},{"name":""}]}`,
		},
		{
			name:       "rejects bad credentials",
			username:   "admin",
			password:   "wrong",
			infoBody:   `{}`,
			metricBody: `{}`,
			playerBody: `{}`,
			wantErr:    "unexpected HTTP status 401 Unauthorized",
		},
		{
			name:       "reports malformed metrics",
			username:   "admin",
			password:   "query-secret",
			infoBody:   `{}`,
			metricBody: `{not-json}`,
			playerBody: `{}`,
			wantErr:    "decode response",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				username, password, ok := request.BasicAuth()
				if !ok || username != "admin" || password != "query-secret" {
					http.Error(writer, "unauthorized", http.StatusUnauthorized)
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case "/v1/api/info":
					fmt.Fprint(writer, tc.infoBody)
				case "/v1/api/metrics":
					fmt.Fprint(writer, tc.metricBody)
				case "/v1/api/players":
					fmt.Fprint(writer, tc.playerBody)
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()

			host, portText, errSplit := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
			if errSplit != nil {
				t.Fatalf("SplitHostPort() error = %v", errSplit)
			}
			port, errPort := strconv.Atoi(portText)
			if errPort != nil {
				t.Fatalf("Atoi(port) error = %v", errPort)
			}

			result, errQuery := Palworld(context.Background(), host, port, tc.username, tc.password)
			if tc.wantErr != "" {
				if errQuery == nil || !strings.Contains(errQuery.Error(), tc.wantErr) {
					t.Fatalf("Palworld() error = %v, want containing %q", errQuery, tc.wantErr)
				}
				return
			}
			if errQuery != nil {
				t.Fatalf("Palworld() error = %v", errQuery)
			}
			if !result.GetResponded() {
				t.Fatal("Palworld().Responded = false, want true")
			}
			if result.GetName() != "Xylona Palworld" || result.GetVersion() != "v1.2.3" {
				t.Fatalf("Palworld() identity = (%q, %q), want Xylona Palworld v1.2.3", result.GetName(), result.GetVersion())
			}
			if result.GetPlayers() != 2 || result.GetMaxPlayers() != 32 {
				t.Fatalf("Palworld() players = %d/%d, want 2/32", result.GetPlayers(), result.GetMaxPlayers())
			}
			if len(result.GetPlayerList()) != 2 || result.GetPlayerList()[0] != "Alice" || result.GetPlayerList()[1] != "Bob" {
				t.Fatalf("Palworld() player list = %v, want [Alice Bob]", result.GetPlayerList())
			}
			if result.GetUptimeSeconds() != 1234 || result.GetDays() != 7 {
				t.Fatalf("Palworld() uptime/days = %d/%d, want 1234/7", result.GetUptimeSeconds(), result.GetDays())
			}
		})
	}
}

func TestPalworldDetailedPreservesPlayerIDs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/api/info":
			fmt.Fprint(writer, `{}`)
		case "/v1/api/metrics":
			fmt.Fprint(writer, `{"currentplayernum":2,"maxplayernum":32}`)
		case "/v1/api/players":
			fmt.Fprint(writer, `{"players":[{"name":"Alice","userId":"steam_1"},{"name":"","userId":"steam_2"}]}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	host, port := palworldTestAddress(t, server.URL)

	result, errQuery := PalworldDetailed(t.Context(), host, port, "admin", "secret")
	if errQuery != nil {
		t.Fatalf("PalworldDetailed() error = %v", errQuery)
	}
	if len(result.PlayerDetails) != 2 {
		t.Fatalf("player details = %+v, want 2 players", result.PlayerDetails)
	}
	if result.PlayerDetails[0] != (PalworldPlayer{Name: "Alice", UserID: "steam_1"}) {
		t.Fatalf("first player = %+v", result.PlayerDetails[0])
	}
	if result.PlayerDetails[1] != (PalworldPlayer{Name: "steam_2", UserID: "steam_2"}) {
		t.Fatalf("fallback player = %+v", result.PlayerDetails[1])
	}
}

func TestPerformPalworldPlayerAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		action      PalworldPlayerAction
		wantPath    string
		message     string
		wantMessage string
	}{
		{name: "kick", action: PalworldPlayerActionKick, wantPath: "/v1/api/kick", message: "AFK", wantMessage: "AFK"},
		{name: "ban", action: PalworldPlayerActionBan, wantPath: "/v1/api/ban", message: "Abuse", wantMessage: "Abuse"},
		{name: "unban omits message", action: PalworldPlayerActionUnban, wantPath: "/v1/api/unban", message: "ignored"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			type capturedRequest struct {
				method   string
				path     string
				username string
				password string
				payload  map[string]string
			}
			captured := make(chan capturedRequest, 1)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				username, password, _ := request.BasicAuth()
				payload := make(map[string]string)
				errDecode := json.NewDecoder(request.Body).Decode(&payload)
				if errDecode != nil {
					http.Error(writer, errDecode.Error(), http.StatusBadRequest)
					return
				}
				captured <- capturedRequest{
					method:   request.Method,
					path:     request.URL.Path,
					username: username,
					password: password,
					payload:  payload,
				}
				writer.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			host, port := palworldTestAddress(t, server.URL)

			errAction := PerformPalworldPlayerAction(
				t.Context(),
				host,
				port,
				"admin",
				"secret",
				tc.action,
				"steam_1",
				tc.message,
			)
			if errAction != nil {
				t.Fatalf("PerformPalworldPlayerAction() error = %v", errAction)
			}
			request := <-captured
			if request.method != http.MethodPost || request.path != tc.wantPath {
				t.Fatalf("request = %s %s, want POST %s", request.method, request.path, tc.wantPath)
			}
			if request.username != "admin" || request.password != "secret" {
				t.Fatalf("basic auth = %q/%q", request.username, request.password)
			}
			if request.payload["userid"] != "steam_1" || request.payload["message"] != tc.wantMessage {
				t.Fatalf("payload = %+v, want userid steam_1 and message %q", request.payload, tc.wantMessage)
			}
		})
	}
}

func palworldTestAddress(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	host, portText, errSplit := net.SplitHostPort(strings.TrimPrefix(rawURL, "http://"))
	if errSplit != nil {
		t.Fatalf("SplitHostPort() error = %v", errSplit)
	}
	port, errPort := strconv.Atoi(portText)
	if errPort != nil {
		t.Fatalf("Atoi(port) error = %v", errPort)
	}
	return host, port
}
