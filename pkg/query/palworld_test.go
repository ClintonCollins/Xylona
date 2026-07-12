package query

import (
	"context"
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
			playerBody: `{"players":[{"name":"Alice"},{"name":"Bob"},{"name":""}]}`,
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
