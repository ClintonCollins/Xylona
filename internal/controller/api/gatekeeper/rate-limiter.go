package gatekeeper

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
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
	limiter := httprate.LimitBy(10, time.Minute, func(r *http.Request) (string, error) {
		return httprate.CanonicalizeIP(requestClientIP(r)), nil
	})
	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shouldRateLimitPath(r.URL.Path) && !isLocalhost(requestClientIP(r)) {
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

func requestClientIP(r *http.Request) string {
	clientIP := middleware.GetClientIP(r.Context())
	if clientIP != "" {
		return clientIP
	}

	return hostFromAddr(r.RemoteAddr)
}

// isLocalhost reports whether addr (in host:port or host form) is a loopback address.
func isLocalhost(addr string) bool {
	host := hostFromAddr(addr)
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func hostFromAddr(addr string) string {
	host, _, errSplit := net.SplitHostPort(addr)
	if errSplit == nil {
		return host
	}

	return addr
}
