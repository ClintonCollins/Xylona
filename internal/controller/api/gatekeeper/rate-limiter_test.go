package gatekeeper

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func TestAuthRateLimiter(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("rate limits Login path after exceeding threshold", func(t *testing.T) {
		handler := AuthRateLimiter()(okHandler)

		var lastStatus int
		for range 15 {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/xylona.Xylona/Login", nil)
			req.RemoteAddr = "192.0.2.1:12345"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			lastStatus = rec.Code
		}

		if lastStatus != http.StatusTooManyRequests {
			t.Fatalf("expected status %d after exceeding rate limit on /Login, got %d",
				http.StatusTooManyRequests, lastStatus)
		}
	})

	t.Run("does not rate limit non-Login paths", func(t *testing.T) {
		handler := AuthRateLimiter()(okHandler)

		for i := range 20 {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/xylona.Xylona/GetGameServers", nil)
			req.RemoteAddr = "192.0.2.2:12345"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status %d for non-Login path on request %d, got %d",
					http.StatusOK, i+1, rec.Code)
			}
		}
	})

	t.Run("localhost Login is never rate limited", func(t *testing.T) {
		handler := AuthRateLimiter()(okHandler)

		for i := range 20 {
			for _, addr := range []string{"127.0.0.1:12345", "[::1]:12345"} {
				req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/xylona.Xylona/Login", nil)
				req.RemoteAddr = addr
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)

				if rec.Code != http.StatusOK {
					t.Fatalf("expected status %d for localhost Login request %d from %s, got %d",
						http.StatusOK, i+1, addr, rec.Code)
				}
			}
		}
	})

	t.Run("first 10 Login requests succeed", func(t *testing.T) {
		handler := AuthRateLimiter()(okHandler)

		for i := range 10 {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/xylona.Xylona/Login", nil)
			req.RemoteAddr = "192.0.2.3:12345"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status %d for Login request %d within limit, got %d",
					http.StatusOK, i+1, rec.Code)
			}
		}
	})

	t.Run("rate limits VerifyNode path after exceeding threshold", func(t *testing.T) {
		handler := AuthRateLimiter()(okHandler)

		var lastStatus int
		for range 15 {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/xylona.Xylona/VerifyNode", nil)
			req.RemoteAddr = "192.0.2.4:12345"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			lastStatus = rec.Code
		}

		if lastStatus != http.StatusTooManyRequests {
			t.Fatalf("expected status %d after exceeding rate limit on /VerifyNode, got %d",
				http.StatusTooManyRequests, lastStatus)
		}
	})

	t.Run("rate limits public Palworld map polling", func(t *testing.T) {
		handler := AuthRateLimiter()(okHandler)

		var lastStatus int
		for range 125 {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/xylona.Xylona/GetPublicPalworldMap", nil)
			req.RemoteAddr = "192.0.2.6:12345"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			lastStatus = rec.Code
		}

		if lastStatus != http.StatusTooManyRequests {
			t.Fatalf("expected status %d after exceeding public map limit, got %d", http.StatusTooManyRequests, lastStatus)
		}
	})

	t.Run("does not trust forwarded client IP headers", func(t *testing.T) {
		handler := middleware.ClientIPFromRemoteAddr(AuthRateLimiter()(okHandler))

		var lastStatus int
		for i := range 15 {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/xylona.Xylona/Login", nil)
			req.RemoteAddr = "192.0.2.5:12345"
			req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i+1))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			lastStatus = rec.Code
		}

		if lastStatus != http.StatusTooManyRequests {
			t.Fatalf("expected spoofed forwarded addresses to share the connection IP rate limit, got %d", lastStatus)
		}
	})

	t.Run("localhost VerifyNode is never rate limited", func(t *testing.T) {
		handler := AuthRateLimiter()(okHandler)

		for i := range 20 {
			for _, addr := range []string{"127.0.0.1:12345", "[::1]:12345"} {
				req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/xylona.Xylona/VerifyNode", nil)
				req.RemoteAddr = addr
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)

				if rec.Code != http.StatusOK {
					t.Fatalf("expected status %d for localhost VerifyNode request %d from %s, got %d",
						http.StatusOK, i+1, addr, rec.Code)
				}
			}
		}
	})
}
