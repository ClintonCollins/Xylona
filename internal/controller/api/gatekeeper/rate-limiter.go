package gatekeeper

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/httprate"
)

var authRateLimitedRPCPaths = []string{
	"/Login",
	"/VerifyNode",
}

// AuthRateLimiter returns middleware that applies IP-based rate limiting only to
// authentication-related RPC paths. All other requests pass through without
// throttling. Localhost addresses are exempt to allow E2E test suites to run
// without hitting the limit.
func AuthRateLimiter() func(http.Handler) http.Handler {
	limiter := httprate.LimitByIP(10, time.Minute)
	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shouldRateLimitPath(r.URL.Path) && !isLocalhost(r.RemoteAddr) {
				limited.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func shouldRateLimitPath(path string) bool {
	for _, limitedPath := range authRateLimitedRPCPaths {
		if strings.Contains(path, limitedPath) {
			return true
		}
	}

	return false
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
