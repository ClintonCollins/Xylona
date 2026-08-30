package rpc

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ClintonCollins/Xylona/internal/nodeclient"
)

func TestSevenDaysToDieOperationItemIconRejectsInvalidPaths(t *testing.T) {
	t.Parallel()

	for _, icon := range []string{"../secret.png", `..\secret.png`, "secret.jpg"} {
		t.Run(icon, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			routeContext := chi.NewRouteContext()
			routeContext.URLParams.Add("gameServerId", "server-1")
			routeContext.URLParams.Add("icon", icon)
			request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
			response := httptest.NewRecorder()

			new(XylonaService).SevenDaysToDieOperationItemIcon(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestSevenDaysToDieOperationItemIconAccessAndStreaming(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	_, errNode := fixture.conn.SQLDb.ExecContext(
		t.Context(),
		"insert into node (id, name, listen_url, enabled) values (?, ?, ?, ?)",
		"node-remote", "Remote Node", "https://node.example", true,
	)
	if errNode != nil {
		t.Fatalf("insert remote node: %v", errNode)
	}
	_, errIP := fixture.conn.SQLDb.ExecContext(
		t.Context(),
		"insert into ip (address, usable, external, node_id) values (?, ?, ?, ?)",
		"127.0.0.2", true, false, "node-remote",
	)
	if errIP != nil {
		t.Fatalf("insert remote IP: %v", errIP)
	}
	_, errServer := fixture.conn.SQLDb.ExecContext(
		t.Context(),
		"update game_server set node_id = ?, ip = ?, directory = ? where id = ?",
		"node-remote", "127.0.0.2", "C:/servers/7dtd", "server-local-1",
	)
	if errServer != nil {
		t.Fatalf("update game server: %v", errServer)
	}
	localClient := &nodeclient.FakeNodeClient{NodeID: "node-local"}
	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:           "node-remote",
		StreamFileReader: io.NopCloser(bytes.NewReader([]byte("png-data"))),
	}
	fixture.service.nodeRegistry = testParityRegistry(localClient, remoteClient)

	t.Run("requires authentication", func(t *testing.T) {
		request := operationItemIconRequest(t, "resourceWood.png")
		response := httptest.NewRecorder()
		fixture.service.SevenDaysToDieOperationItemIcon(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
		}
	})

	t.Run("requires console permission", func(t *testing.T) {
		request := operationItemIconRequest(t, "resourceWood.png")
		addSessionCookieHeaderHTTP(t, fixture.conn, fixture.secureCookie, request, "user-other")
		response := httptest.NewRecorder()
		fixture.service.SevenDaysToDieOperationItemIcon(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
		}
	})

	t.Run("streams from the owning remote node", func(t *testing.T) {
		request := operationItemIconRequest(t, "resourceWood.png")
		addSessionCookieHeaderHTTP(t, fixture.conn, fixture.secureCookie, request, "user-owner")
		response := httptest.NewRecorder()
		fixture.service.SevenDaysToDieOperationItemIcon(response, request)
		if response.Code != http.StatusOK || response.Body.String() != "png-data" ||
			response.Header().Get("Content-Type") != "image/png" || response.Header().Get("Cache-Control") != "private, max-age=86400" {
			t.Fatalf("response = %d %q %v", response.Code, response.Body.String(), response.Header())
		}
		if len(localClient.StreamFileCalls) != 0 || len(remoteClient.StreamFileCalls) != 1 ||
			remoteClient.StreamFileCalls[0].Directory != "C:/servers/7dtd" ||
			remoteClient.StreamFileCalls[0].RelativePath != filepath.Join("Data", "ItemIcons", "resourceWood.png") {
			t.Fatalf("stream calls: local=%+v remote=%+v", localClient.StreamFileCalls, remoteClient.StreamFileCalls)
		}
	})
}

func operationItemIconRequest(t *testing.T, icon string) *http.Request {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("gameServerId", "server-local-1")
	routeContext.URLParams.Add("icon", icon)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}
