package rpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestNewGameServerStatusPageHTTPHandlerAllowsMissingShell(t *testing.T) {
	handler := NewGameServerStatusPageHTTPHandler(fstest.MapFS{}, nil, nil)
	if handler == nil {
		t.Fatal("NewGameServerStatusPageHTTPHandler() returned nil")
	}

	request := httptest.NewRequest(http.MethodGet, "http://status.example/status/Owner_Page", nil)
	response := httptest.NewRecorder()
	handler.StatusPage(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("missing shell status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}

func TestGameServerStatusPageHTTP(t *testing.T) {
	newHandler := func(t *testing.T, enabled bool) http.Handler {
		t.Helper()
		fixture := newRBACRPCFixture(t)
		_, errCreate := fixture.conn.CreateGameServerStatusPage("user-owner", `Fleet & <friends>`, "Owner_Page")
		if errCreate != nil {
			t.Fatalf("CreateGameServerStatusPage() error = %v", errCreate)
		}
		if enabled {
			_, errEnable := fixture.conn.SQLDb.ExecContext(t.Context(), "update game_server_status_page set enabled = true where user_id = ?", "user-owner")
			if errEnable != nil {
				t.Fatalf("enable status page: %v", errEnable)
			}
		}
		frontend := fstest.MapFS{
			"index.html": {Data: []byte(`<!doctype html><html><head><title>Xylona</title><meta name="description" content="Panel"></head><body><div id="q-app"></div></body></html>`)},
		}
		handler := NewGameServerStatusPageHTTPHandler(frontend, fixture.service, nil)
		router := chi.NewRouter()
		RegisterGameServerStatusPageRoutes(router, handler)
		return router
	}

	t.Run("injects escaped discovery metadata for enabled pages", func(t *testing.T) {
		handler := newHandler(t, true)
		request := httptest.NewRequest(http.MethodGet, "http://status.example/status/Owner_Page", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
		}
		body := response.Body.String()
		for _, expected := range []string{
			`<title>Fleet &amp; &lt;friends&gt; · Xylona status</title>`,
			`rel="canonical" href="http://status.example/status/Owner_Page"`,
			`name="robots" content="index, follow"`,
		} {
			if !strings.Contains(body, expected) {
				t.Fatalf("body does not contain %q: %s", expected, body)
			}
		}
		if response.Header().Get("Cache-Control") != "no-cache" || response.Header().Get("ETag") == "" {
			t.Fatalf("cache headers = %v", response.Header())
		}
		if response.Header().Get("Vary") != "Host, X-Forwarded-Host, X-Forwarded-Proto" {
			t.Fatalf("vary header = %q", response.Header().Get("Vary"))
		}
	})

	t.Run("does not disclose disabled pages", func(t *testing.T) {
		handler := newHandler(t, false)
		request := httptest.NewRequest(http.MethodGet, "http://status.example/status/Owner_Page", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "Fleet") || !strings.Contains(response.Body.String(), `<div id="q-app"></div>`) {
			t.Fatalf("disabled response = %d %q", response.Code, response.Body.String())
		}
		if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Robots-Tag") != "noindex, nofollow" {
			t.Fatalf("disabled headers = %v", response.Header())
		}
	})

	t.Run("streams an immediate complete snapshot", func(t *testing.T) {
		handler := newHandler(t, true)
		request := httptest.NewRequest(http.MethodGet, "http://status.example/api/public/status-pages/Owner_Page/events", nil)
		ctx, cancel := context.WithTimeout(request.Context(), 25*time.Millisecond)
		defer cancel()
		request = request.WithContext(ctx)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		body := response.Body.String()
		if !strings.Contains(body, "retry: 3000") || !strings.Contains(body, "event: snapshot") || !strings.Contains(body, `"title":"Fleet & <friends>"`) {
			t.Fatalf("event stream = %q", body)
		}
		if response.Header().Get("Content-Type") != "text/event-stream" || response.Header().Get("X-Accel-Buffering") != "no" {
			t.Fatalf("event headers = %v", response.Header())
		}
		if response.Header().Get("Cache-Control") != "no-store, no-transform" {
			t.Fatalf("event cache = %q", response.Header().Get("Cache-Control"))
		}
	})

	t.Run("lists enabled pages in discovery files", func(t *testing.T) {
		handler := newHandler(t, true)
		for _, test := range []struct {
			path string
			want string
		}{
			{path: "/robots.txt", want: "Sitemap: http://status.example/sitemap.xml"},
			{path: "/sitemap.xml", want: "http://status.example/status/Owner_Page"},
		} {
			request := httptest.NewRequest(http.MethodGet, "http://status.example"+test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.want) {
				t.Fatalf("GET %s = %d %q", test.path, response.Code, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "public, max-age=300" {
				t.Fatalf("GET %s cache = %q", test.path, response.Header().Get("Cache-Control"))
			}
			if test.path == "/robots.txt" {
				for _, directive := range []string{"Disallow: /", "Disallow: /api/", "Disallow: /shared/", "Allow: /status/"} {
					if !strings.Contains(response.Body.String(), directive) {
						t.Fatalf("robots.txt does not contain %q: %q", directive, response.Body.String())
					}
				}
			}
		}
	})
}

func TestDiscardResolvedStatusOverrides(t *testing.T) {
	now := time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC)
	statuses := map[string]xylona.Status{
		"caught-up": xylona.Status_ONLINE,
		"expired":   xylona.Status_OFFLINE,
		"waiting":   xylona.Status_PRE_START,
	}
	expiresAt := map[string]time.Time{
		"caught-up": now.Add(time.Second),
		"expired":   now,
		"waiting":   now.Add(time.Second),
	}
	cached := map[string]xylona.Status{
		"caught-up": xylona.Status_ONLINE,
		"expired":   xylona.Status_UNKNOWN,
		"waiting":   xylona.Status_OFFLINE,
	}

	discardResolvedStatusOverrides(statuses, expiresAt, now, func(serverID string) xylona.Status {
		return cached[serverID]
	})

	if len(statuses) != 1 || statuses["waiting"] != xylona.Status_PRE_START {
		t.Fatalf("remaining overrides = %v, want waiting", statuses)
	}
	if len(expiresAt) != 1 || !expiresAt["waiting"].Equal(now.Add(time.Second)) {
		t.Fatalf("remaining expirations = %v, want waiting", expiresAt)
	}
}
