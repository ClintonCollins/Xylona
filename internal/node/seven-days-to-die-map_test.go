package node

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNodeSevenDaysToDieMap(t *testing.T) {
	const tokenName = "xylona-map"
	const tokenSecret = "map-secret"

	var receivedTilePath string
	var receivedUnusedAPI string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-SDTD-API-TOKENNAME") != tokenName || request.Header.Get("X-SDTD-API-SECRET") != tokenSecret {
			http.Error(response, "missing credentials", http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/api/map/config":
			writeSevenDaysToDieTestResponse(t, response, `{"data":{"enabled":true,"mapBlockSize":128,"maxZoom":4,"mapSize":{"x":6144,"y":255,"z":6144}},"meta":{"serverTime":"Day 2, 09:15"}}`)
		case "/api/player":
			writeSevenDaysToDieTestResponse(t, response, `{"data":{"players":[{"entityId":7,"name":"Clinton","online":true,"platformId":{"combinedString":"Steam_123"},"position":{"x":10.5,"y":42,"z":-9.25}}]}}`)
		case "/api/markers":
			receivedUnusedAPI = request.URL.Path
			http.NotFound(response, request)
		case "/api/getlandclaims":
			receivedUnusedAPI = request.URL.Path
			http.NotFound(response, request)
		case "/map/4/1/-2.png":
			receivedTilePath = request.URL.Path
			writeSevenDaysToDieTestResponse(t, response, "\x89PNG\r\n\x1a\nfixture")
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(server.Close)

	workingDirectory := t.TempDir()
	serverURL, errParseURL := url.Parse(server.URL)
	if errParseURL != nil {
		t.Fatalf("parse test server URL: %v", errParseURL)
	}
	_, port, found := strings.Cut(serverURL.Host, ":")
	if !found {
		t.Fatalf("test server host %q has no port", serverURL.Host)
	}
	config := fmt.Sprintf(`<ServerSettings><property name="WebDashboardPort" value="%s" /></ServerSettings>`, port)
	errWrite := os.WriteFile(filepath.Join(workingDirectory, sevenDaysToDieServerConfigName), []byte(config), 0o600)
	if errWrite != nil {
		t.Fatalf("write server config: %v", errWrite)
	}

	n := new(Node)
	t.Run("reads the current native snapshot without arbitrary proxying", func(t *testing.T) {
		snapshot, errQuery := n.QuerySevenDaysToDieMap(context.Background(), SevenDaysToDieMapQueryRequest{
			WorkingDirectory: workingDirectory,
			TokenName:        tokenName,
			TokenSecret:      tokenSecret,
		})
		if errQuery != nil {
			t.Fatalf("QuerySevenDaysToDieMap() error = %v", errQuery)
		}
		if !snapshot.Enabled || snapshot.TileSize != 128 || snapshot.MaxZoom != 4 {
			t.Fatalf("QuerySevenDaysToDieMap() config = %+v", snapshot)
		}
		if receivedUnusedAPI != "" {
			t.Errorf("QuerySevenDaysToDieMap() requested unused API %q", receivedUnusedAPI)
		}
		if len(snapshot.Players) != 1 || snapshot.Players[0].ID != "Steam_123" || snapshot.Players[0].Position.Z != -9.25 {
			t.Errorf("QuerySevenDaysToDieMap() players = %+v", snapshot.Players)
		}
	})

	t.Run("fetches only a bounded PNG tile with the native credentials", func(t *testing.T) {
		tile, errTile := n.GetSevenDaysToDieMapTile(context.Background(), SevenDaysToDieMapTileRequest{
			WorkingDirectory: workingDirectory,
			TokenName:        tokenName,
			TokenSecret:      tokenSecret,
			Zoom:             4,
			X:                1,
			Y:                -2,
		})
		if errTile != nil {
			t.Fatalf("GetSevenDaysToDieMapTile() error = %v", errTile)
		}
		if receivedTilePath != "/map/4/1/-2.png" || !strings.HasPrefix(string(tile), "\x89PNG") {
			t.Errorf("GetSevenDaysToDieMapTile() path = %q tile = %q", receivedTilePath, tile)
		}

		_, errInvalid := n.GetSevenDaysToDieMapTile(context.Background(), SevenDaysToDieMapTileRequest{
			WorkingDirectory: workingDirectory,
			Zoom:             31,
		})
		if errInvalid == nil {
			t.Error("GetSevenDaysToDieMapTile() error = nil for unbounded zoom")
		}
	})
}

func writeSevenDaysToDieTestResponse(t *testing.T, response http.ResponseWriter, body string) {
	t.Helper()
	_, errWrite := response.Write([]byte(body))
	if errWrite != nil {
		t.Errorf("write test response: %v", errWrite)
	}
}
