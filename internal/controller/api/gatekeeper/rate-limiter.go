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
	"/CompleteSetup",
}

var publicMapRPCPaths = []string{
	"/GetPublicPalworldMap",
	"/GetPublicSevenDaysToDieMap",
	"/GetPublicMinecraftMap",
	"/ResolvePublicGameServerMap",
}

// AuthRateLimiter applies a strict IP limit to authentication RPCs and
// separate limits to public reads. All other requests pass through without
// throttling. Direct localhost peers remain exempt for E2E suites.
func AuthRateLimiter() func(http.Handler) http.Handler {
	return AuthRateLimiterForProxies(nil)
}

// AuthRateLimiterForProxies is AuthRateLimiter with a trusted-proxy list used
// to derive the client IP from X-Forwarded-For.
func AuthRateLimiterForProxies(trust *ProxyTrust) func(http.Handler) http.Handler {
	authLimiter := httprate.LimitBy(10, time.Minute, func(r *http.Request) (string, error) {
		return httprate.CanonicalizeIP(requestClientIP(r, trust)), nil
	})
	publicMapLimiter := httprate.LimitBy(120, time.Minute, func(r *http.Request) (string, error) {
		return httprate.CanonicalizeIP(requestClientIP(r, trust)), nil
	})
	publicStatusEventLimiter := httprate.LimitBy(30, time.Minute, func(r *http.Request) (string, error) {
		return httprate.CanonicalizeIP(requestClientIP(r, trust)), nil
	})
	return func(next http.Handler) http.Handler {
		authLimited := authLimiter(next)
		publicMapLimited := publicMapLimiter(next)
		publicStatusEventLimited := publicStatusEventLimiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isLocalhost(r.RemoteAddr) && !trust.IsTrustedRemote(r) {
				next.ServeHTTP(w, r)
				return
			}
			if shouldRateLimitPath(r.URL.Path) {
				authLimited.ServeHTTP(w, r)
				return
			}
			if shouldRateLimitPublicMapPath(r.URL.Path) || isPublicGameServerMapPath(r.URL.Path) {
				publicMapLimited.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/public/status-pages/") && strings.HasSuffix(r.URL.Path, "/events") {
				publicStatusEventLimited.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/status/") || strings.HasPrefix(r.URL.Path, "/api/public/status-pages/") ||
				strings.Contains(r.URL.Path, "/GetPublicGameServerStatusPage") {
				publicMapLimited.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isPublicGameServerMapPath(path string) bool {
	return path == "/maps" || strings.HasPrefix(path, "/maps/")
}

func shouldRateLimitPublicMapPath(path string) bool {
	for _, publicMapPath := range publicMapRPCPaths {
		if strings.Contains(path, publicMapPath) {
			return true
		}
	}
	return false
}

func shouldRateLimitPath(path string) bool {
	for _, limitedPath := range authRateLimitedRPCPaths {
		if strings.Contains(path, limitedPath) {
			return true
		}
	}

	return false
}

func requestClientIP(r *http.Request, trust *ProxyTrust) string {
	if trust != nil {
		return trust.ClientIP(r)
	}
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
