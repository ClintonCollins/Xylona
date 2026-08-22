package rpc

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"
)

func TestGameServerMapHTTP(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	errMapConfig := fixture.conn.UpdateGameServerMinecraftMapConfig("server-local-1", true, "world", true, "user-owner")
	if errMapConfig != nil {
		t.Fatalf("enable Minecraft map: %v", errMapConfig)
	}
	_, errCreate := fixture.conn.GetOrCreateGameServerMapShare("server-local-1", "Current_Map")
	if errCreate != nil {
		t.Fatalf("GetOrCreateGameServerMapShare() error = %v", errCreate)
	}
	_, errEnable := fixture.conn.UpdateGameServerMapShare("server-local-1", "Current_Map", true)
	if errEnable != nil {
		t.Fatalf("UpdateGameServerMapShare() error = %v", errEnable)
	}

	shell := `<!doctype html><html><head><title>Xylona</title></head><body><div id="q-app"></div></body></html>`
	frontend := fstest.MapFS{"index.html": {Data: []byte(shell)}}
	handler := NewGameServerMapHTTPHandler(frontend, fixture.service)
	router := chi.NewRouter()
	RegisterGameServerMapRoutes(router, handler)

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{name: "enabled supported map", path: "/maps/Current_Map", wantStatus: http.StatusOK},
		{name: "unknown identifier", path: "/maps/Unknown_Map", wantStatus: http.StatusNotFound},
		{name: "malformed identifier", path: "/maps/a", wantStatus: http.StatusNotFound},
		{name: "missing identifier", path: "/maps/", wantStatus: http.StatusNotFound},
		{name: "nested identifier", path: "/maps/Current_Map/extra", wantStatus: http.StatusNotFound},
		{name: "legacy Palworld route", path: "/shared/palworld-map", wantStatus: http.StatusNotFound},
		{name: "legacy seven days route", path: "/shared/7-days-to-die-map", wantStatus: http.StatusNotFound},
		{name: "legacy Minecraft route", path: "/shared/minecraft-map", wantStatus: http.StatusNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://maps.example"+test.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("GET %s status = %d, want %d", test.path, response.Code, test.wantStatus)
			}
			if response.Body.String() != shell {
				t.Fatalf("GET %s body = %q, want identical SPA shell", test.path, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" ||
				response.Header().Get("Referrer-Policy") != "no-referrer" ||
				response.Header().Get("X-Robots-Tag") != "noindex, nofollow" {
				t.Fatalf("GET %s privacy headers = %v", test.path, response.Header())
			}
		})
	}

	_, errUnsupportedGame := fixture.conn.SQLDb.ExecContext(
		t.Context(),
		`insert into game (id, name, default_port, default_query_port, default_max_players, windows_support)
		 values (?, ?, ?, ?, ?, ?)`,
		"unsupported", "Unsupported", 27015, 27015, 16, true,
	)
	if errUnsupportedGame != nil {
		t.Fatalf("insert unsupported game: %v", errUnsupportedGame)
	}
	_, errUnsupported := fixture.conn.SQLDb.ExecContext(t.Context(), "update game_server set game_id = ? where id = ?", "unsupported", "server-local-1")
	if errUnsupported != nil {
		t.Fatalf("set unsupported game: %v", errUnsupported)
	}
	unsupportedRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://maps.example/maps/Current_Map", nil)
	unsupportedResponse := httptest.NewRecorder()
	router.ServeHTTP(unsupportedResponse, unsupportedRequest)
	if unsupportedResponse.Code != http.StatusNotFound || unsupportedResponse.Body.String() != shell {
		t.Fatalf("unsupported map response = %d %q", unsupportedResponse.Code, unsupportedResponse.Body.String())
	}

	_, errRestore := fixture.conn.SQLDb.ExecContext(t.Context(), "update game_server set game_id = ? where id = ?", "minecraft", "server-local-1")
	if errRestore != nil {
		t.Fatalf("restore supported game: %v", errRestore)
	}
	_, errRename := fixture.conn.UpdateGameServerMapShare("server-local-1", "Renamed_Map", true)
	if errRename != nil {
		t.Fatalf("rename map share: %v", errRename)
	}
	for _, test := range []struct {
		path       string
		wantStatus int
	}{
		{path: "/maps/Current_Map", wantStatus: http.StatusNotFound},
		{path: "/maps/Renamed_Map", wantStatus: http.StatusOK},
	} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://maps.example"+test.path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != test.wantStatus || response.Body.String() != shell {
			t.Fatalf("renamed map GET %s = %d %q", test.path, response.Code, response.Body.String())
		}
	}

	_, errDisable := fixture.conn.UpdateGameServerMapShare("server-local-1", "Renamed_Map", false)
	if errDisable != nil {
		t.Fatalf("disable map share: %v", errDisable)
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://maps.example/maps/Renamed_Map", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled map status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestNewGameServerMapHTTPHandlerAllowsMissingShell(t *testing.T) {
	handler := NewGameServerMapHTTPHandler(fstest.MapFS{}, nil)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://maps.example/maps/Current_Map", nil)
	response := httptest.NewRecorder()
	handler.Map(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("missing shell status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}
