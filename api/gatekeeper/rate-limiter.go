package gatekeeper

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/httprate"
)

// AuthRateLimiter returns middleware that applies IP-based rate limiting only to
// authentication-related RPC paths (Login). All other requests pass through
// without throttling. Localhost addresses are exempt to allow E2E test suites
// to run without hitting the limit.
func AuthRateLimiter() func(http.Handler) http.Handler {
	limiter := httprate.LimitByIP(10, time.Minute)
	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/Login") && !isLocalhost(r.RemoteAddr) {
				limited.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isLocalhost reports whether addr (in host:port or host form) is a loopback address.
func isLocalhost(addr string) bool {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
