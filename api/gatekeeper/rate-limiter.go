package gatekeeper

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/httprate"
)

// AuthRateLimiter returns middleware that applies IP-based rate limiting only to
// authentication-related RPC paths (Login). All other requests pass through
// without throttling.
func AuthRateLimiter() func(http.Handler) http.Handler {
	limiter := httprate.LimitByIP(10, time.Minute)
	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/Login") {
				limited.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
