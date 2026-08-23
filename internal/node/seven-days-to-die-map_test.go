package node

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"gopkg.in/yaml.v3"
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
		case "/api/openapi/openapi.yaml", "/api/markers", "/api/getlandclaims", "/api/bloodmoon", "/api/hostile", "/api/animal":
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

func TestSevenDaysToDieTacticalOverlays(t *testing.T) {
	const markerID = "f4c2d4ea-7e4d-46b0-aaf2-26ea769951d4"
	fullOpenAPI := `openapi: 3.1.0
info:
  version: "3.0"
paths:
  /api/markers:
    get: {}
  /api/getlandclaims:
    get: {}
  /api/bloodmoon:
    get: {}
  /api/hostile:
    get: {}
  /api/animal:
    get: {}
`
	defaultBodies := map[string]string{
		"/api/markers":       `{"data":[{"id":"` + markerID + `","name":"Trader","x":10,"y":20,"icon":"https://example.invalid/icon.png"}]}`,
		"/api/getlandclaims": `{"data":{"claimsize":41,"claimowners":[{"steamid":"Steam_1","claimactive":true,"playername":"Alex","claims":[{"x":30,"y":5,"z":40}]}]}}`,
		"/api/bloodmoon":     `{"data":{"gameTime":{"days":7,"hours":22,"minutes":0},"bloodmoonActive":true,"nextBloodmoon":{"days":14,"hours":22,"minutes":0},"nextBloodmoonEnd":{"days":15,"hours":4,"minutes":30}}}`,
		"/api/hostile":       `{"data":[{"id":1,"name":"Zombie","position":{"x":50,"y":6,"z":60}}]}`,
		"/api/animal":        `{"data":[{"id":2,"name":"Wolf","position":{"x":70,"y":7,"z":80}}]}`,
	}
	deep := `{"data":[{"id":"` + markerID + `","x":1,"y":2,"extra":` + strings.Repeat("[", 13) + `0` + strings.Repeat("]", 13) + `}]}`
	wide := `{"data":[{"id":"` + markerID + `","x":1,"y":2,` + strings.Repeat(`"a":0,`, sevenDaysToDieMapContainerEntryLimit) + `"z":0}]}`
	overCount := `{"data":[` + strings.TrimSuffix(strings.Repeat(`{"id":"`+markerID+`","x":1,"y":2},`, sevenDaysToDieMapItemLimit+1), ",") + `]}`
	overEntityCount := `{"data":[` + strings.TrimSuffix(strings.Repeat(`{"id":1,"name":"Wolf","position":{"x":1,"y":2,"z":3}},`, sevenDaysToDieMapItemLimit+1), ",") + `]}`
	tests := []struct {
		name       string
		openAPI    string
		path       string
		statusCode int
		body       string
		wantState  SevenDaysToDieWebAPIValueState
	}{
		{name: "available", openAPI: fullOpenAPI, wantState: SevenDaysToDieWebAPIValueStateAvailable},
		{name: "unsupported", openAPI: strings.Replace(fullOpenAPI, "  /api/markers:\n    get: {}\n", "", 1), path: "/api/markers", wantState: SevenDaysToDieWebAPIValueStateUnsupported},
		{name: "upstream permission", openAPI: fullOpenAPI, path: "/api/markers", statusCode: http.StatusForbidden, wantState: SevenDaysToDieWebAPIValueStatePermissionDenied},
		{name: "hostile permission is independent", openAPI: fullOpenAPI, path: "/api/hostile", statusCode: http.StatusForbidden, wantState: SevenDaysToDieWebAPIValueStatePermissionDenied},
		{name: "marker schema", openAPI: fullOpenAPI, path: "/api/markers", body: `{"data":[{"x":1,"y":2}]}`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "claim schema", openAPI: fullOpenAPI, path: "/api/getlandclaims", body: `{"data":{"claimowners":[]}}`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "blood moon schema", openAPI: fullOpenAPI, path: "/api/bloodmoon", body: `{"data":{"gameTime":{"days":7,"hours":22,"minutes":0},"bloodmoonActive":true}}`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "hostile schema", openAPI: fullOpenAPI, path: "/api/hostile", body: `{"data":[{"id":1,"name":"Zombie"}]}`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "animal schema", openAPI: fullOpenAPI, path: "/api/animal", body: `{"data":[{"id":2,"name":"","position":{"x":1,"y":2,"z":3}}]}`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "malformed JSON", openAPI: fullOpenAPI, path: "/api/markers", body: `{"data":[`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "oversized claim response", openAPI: fullOpenAPI, path: "/api/getlandclaims", body: strings.Repeat("x", sevenDaysToDieWebAPIResponseLimit+1), wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "wide", openAPI: fullOpenAPI, path: "/api/markers", body: wide, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "deep hostile response", openAPI: fullOpenAPI, path: "/api/hostile", body: deep, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "marker count", openAPI: fullOpenAPI, path: "/api/markers", body: overCount, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "animal count", openAPI: fullOpenAPI, path: "/api/animal", body: overEntityCount, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "string", openAPI: fullOpenAPI, path: "/api/markers", body: `{"data":[{"id":"` + markerID + `","name":"` + strings.Repeat("x", sevenDaysToDieMapTextLimit+1) + `","x":1,"y":2}]}`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "non finite hostile", openAPI: fullOpenAPI, path: "/api/hostile", body: `{"data":[{"id":1,"name":"Zombie","position":{"x":1e1000,"y":2,"z":3}}]}`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "non integer compatibility shape", openAPI: fullOpenAPI, path: "/api/markers", body: `{"data":[{"id":"` + markerID + `","x":1.5,"y":2}]}`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "out of map claim", openAPI: fullOpenAPI, path: "/api/getlandclaims", body: `{"data":{"claimsize":41,"claimowners":[{"steamid":"Steam_1","claimactive":true,"playername":"Alex","claims":[{"x":4000,"y":5,"z":40}]}]}}`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "out of map animal", openAPI: fullOpenAPI, path: "/api/animal", body: `{"data":[{"id":2,"name":"Wolf","position":{"x":4000,"y":2,"z":3}}]}`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
		{name: "blood moon time range", openAPI: fullOpenAPI, path: "/api/bloodmoon", body: `{"data":{"gameTime":{"days":7,"hours":24,"minutes":0},"bloodmoonActive":true,"nextBloodmoon":{"days":14,"hours":22,"minutes":0},"nextBloodmoonEnd":{"days":15,"hours":4,"minutes":30}}}`, wantState: SevenDaysToDieWebAPIValueStateUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requested := make(map[string]int)
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				requested[request.Method+" "+request.URL.Path]++
				if request.Header.Get("X-SDTD-API-TOKENNAME") != "map-token" || request.Header.Get("X-SDTD-API-SECRET") != "map-secret" {
					http.Error(response, "missing credentials", http.StatusUnauthorized)
					return
				}
				switch request.URL.Path {
				case "/api/map/config":
					writeSevenDaysToDieTestResponse(t, response, `{"data":{"enabled":true,"mapBlockSize":128,"maxZoom":4,"mapSize":{"x":6144,"y":255,"z":6144}}}`)
				case "/api/player":
					writeSevenDaysToDieTestResponse(t, response, `{"data":{"players":[{"entityId":7,"name":"Alex","position":{"x":1,"y":2,"z":3}}]}}`)
				case "/api/openapi/openapi.yaml":
					writeSevenDaysToDieTestResponse(t, response, test.openAPI)
				default:
					if request.URL.Path == test.path && test.statusCode != 0 {
						response.WriteHeader(test.statusCode)
						return
					}
					body := defaultBodies[request.URL.Path]
					if request.URL.Path == test.path && test.body != "" {
						body = test.body
					}
					if body == "" {
						http.NotFound(response, request)
						return
					}
					writeSevenDaysToDieTestResponse(t, response, body)
				}
			}))
			t.Cleanup(server.Close)

			workingDirectory := t.TempDir()
			serverURL, errURL := url.Parse(server.URL)
			if errURL != nil {
				t.Fatalf("parse server URL: %v", errURL)
			}
			_, port, found := strings.Cut(serverURL.Host, ":")
			if !found {
				t.Fatalf("test server host %q has no port", serverURL.Host)
			}
			config := fmt.Sprintf(`<ServerSettings><property name="WebDashboardEnabled" value="true"/><property name="WebDashboardPort" value="%s"/></ServerSettings>`, port)
			errWrite := os.WriteFile(filepath.Join(workingDirectory, sevenDaysToDieServerConfigName), []byte(config), 0o600)
			if errWrite != nil {
				t.Fatalf("write server config: %v", errWrite)
			}

			snapshot, errQuery := new(Node).QuerySevenDaysToDieMap(t.Context(), SevenDaysToDieMapQueryRequest{
				WorkingDirectory: workingDirectory, TokenName: "map-token", TokenSecret: "map-secret", IncludeTactical: true,
			})
			if errQuery != nil {
				t.Fatalf("QuerySevenDaysToDieMap() error = %v", errQuery)
			}
			if len(snapshot.Players) != 1 {
				t.Fatalf("base/independent overlays unavailable after optional failure: %+v", snapshot)
			}
			targetPath := test.path
			if targetPath == "" {
				targetPath = "/api/markers"
			}
			states := map[string]SevenDaysToDieWebAPIValueState{
				"/api/markers": snapshot.NativeMarkerState, "/api/getlandclaims": snapshot.ClaimsState,
				"/api/bloodmoon": snapshot.BloodMoonState, "/api/hostile": snapshot.HostileState, "/api/animal": snapshot.AnimalState,
			}
			counts := map[string]int{
				"/api/markers": len(snapshot.NativeMarkers), "/api/getlandclaims": len(snapshot.Claims),
				"/api/bloodmoon": 0, "/api/hostile": len(snapshot.Hostiles), "/api/animal": len(snapshot.Animals),
			}
			if snapshot.BloodMoon != nil {
				counts["/api/bloodmoon"] = 1
			}
			for path := range defaultBodies {
				wantState := SevenDaysToDieWebAPIValueStateAvailable
				wantCount := 1
				if path == targetPath {
					wantState = test.wantState
					if wantState != SevenDaysToDieWebAPIValueStateAvailable {
						wantCount = 0
					}
				}
				if states[path] != wantState || counts[path] != wantCount {
					t.Fatalf("overlay %s = state %v count %d, want state %v count %d; snapshot %+v", path, states[path], counts[path], wantState, wantCount, snapshot)
				}
			}
			for key := range requested {
				if !strings.HasPrefix(key, http.MethodGet+" ") {
					t.Fatalf("unexpected non-GET request %q", key)
				}
			}
			if test.name == "unsupported" && requested[http.MethodGet+" /api/markers"] != 0 {
				t.Fatal("unsupported marker endpoint was requested")
			}
		})
	}
}

func TestSevenDaysToDieTacticalOverlaysDoNotRenewExhaustedDiscoveryBudget(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeSevenDaysToDieTestResponse(t, response, `{}`)
	}))
	t.Cleanup(server.Close)
	serverURL, errURL := url.Parse(server.URL)
	if errURL != nil {
		t.Fatalf("parse server URL: %v", errURL)
	}
	_, portText, found := strings.Cut(serverURL.Host, ":")
	if !found {
		t.Fatalf("test server host %q has no port", serverURL.Host)
	}
	port, errPort := strconv.ParseUint(portText, 10, 16)
	if errPort != nil {
		t.Fatalf("parse server port: %v", errPort)
	}

	budgetCtx, cancel := context.WithTimeout(t.Context(), 0)
	defer cancel()
	discovery := &sevenDaysToDieWebAPIDiscovery{
		ctx:      budgetCtx,
		settings: sevenDaysToDieWebAPISettings{enabled: true, port: port},
		resolver: &sevenDaysToDieOpenAPIResolver{document: sevenDaysToDieOpenAPI{
			Paths: map[string]map[string]yaml.Node{
				string(sevenDaysToDieWebAPIEndpointMarkers): {"get": {}},
			},
		}},
	}
	state := querySevenDaysToDieMapOverlay(
		discovery,
		SevenDaysToDieMapQueryRequest{},
		sevenDaysToDieWebAPIEndpointMarkers,
		func([]byte) error { return nil },
	)
	if state != SevenDaysToDieWebAPIValueStateUnavailable {
		t.Fatalf("querySevenDaysToDieMapOverlay() state = %v, want unavailable", state)
	}
	if requests.Load() != 0 {
		t.Fatalf("querySevenDaysToDieMapOverlay() made %d requests after its shared budget expired", requests.Load())
	}
}

func writeSevenDaysToDieTestResponse(t *testing.T, response http.ResponseWriter, body string) {
	t.Helper()
	_, errWrite := response.Write([]byte(body))
	if errWrite != nil {
		t.Errorf("write test response: %v", errWrite)
	}
}
