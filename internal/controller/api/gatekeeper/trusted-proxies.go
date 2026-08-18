package gatekeeper

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// ProxyTrust decides whether forwarded headers from a request may be trusted.
type ProxyTrust struct {
	networks []*net.IPNet
}

// ParseTrustedProxies parses a comma-separated list of IPs or CIDR ranges.
// An empty string yields a trust object that never trusts forwarded headers.
func ParseTrustedProxies(raw string) (*ProxyTrust, error) {
	trust := &ProxyTrust{}
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if !strings.Contains(trimmed, "/") {
			ip := net.ParseIP(trimmed)
			if ip == nil {
				return nil, fmt.Errorf("trusted proxy %q is not a valid IP or CIDR", trimmed)
			}
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			trimmed = fmt.Sprintf("%s/%d", ip.String(), bits)
		}
		_, network, errParse := net.ParseCIDR(trimmed)
		if errParse != nil {
			return nil, fmt.Errorf("trusted proxy %q is not a valid IP or CIDR: %w", trimmed, errParse)
		}
		trust.networks = append(trust.networks, network)
	}
	return trust, nil
}

// IsTrustedRemote reports whether r.RemoteAddr belongs to a configured proxy.
func (p *ProxyTrust) IsTrustedRemote(r *http.Request) bool {
	if p == nil || r == nil || len(p.networks) == 0 {
		return false
	}
	host := hostFromAddr(r.RemoteAddr)
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, network := range p.networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// ClientIP returns the effective client IP. Forwarded addresses are used only
// when the immediate peer is a configured trusted proxy.
func (p *ProxyTrust) ClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if p.IsTrustedRemote(r) {
		forwarded := firstForwardedFor(r)
		if forwarded != "" {
			return forwarded
		}
	}
	return hostFromAddr(r.RemoteAddr)
}

// RequestIsHTTPS reports whether the request arrived over HTTPS, either
// directly or via X-Forwarded-Proto from a trusted proxy.
func (p *ProxyTrust) RequestIsHTTPS(r *http.Request) bool {
	if r != nil && r.TLS != nil {
		return true
	}
	if !p.IsTrustedRemote(r) {
		return false
	}
	return strings.EqualFold(firstForwardedProto(r), "https")
}

// ForwardedHost returns X-Forwarded-Host / Forwarded host only when the peer
// is a trusted proxy.
func (p *ProxyTrust) ForwardedHost(r *http.Request) string {
	if !p.IsTrustedRemote(r) {
		return ""
	}
	return requestForwardedHostForSameOrigin(r)
}

// AnnotateRequest strips a client-supplied internal HTTPS marker and sets it
// only after the controller has decided the request is HTTPS.
func (p *ProxyTrust) AnnotateRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del(InternalHTTPSHeader)
		if p.RequestIsHTTPS(r) {
			r.Header.Set(InternalHTTPSHeader, "1")
		}
		next.ServeHTTP(w, r)
	})
}

func firstForwardedFor(r *http.Request) string {
	if r == nil {
		return ""
	}
	values := r.Header.Values("X-Forwarded-For")
	if len(values) == 0 {
		return ""
	}
	parts := strings.Split(values[0], ",")
	if len(parts) == 0 {
		return ""
	}
	candidate := strings.TrimSpace(parts[0])
	parsedIP := net.ParseIP(candidate)
	if parsedIP != nil {
		return parsedIP.String()
	}
	host := hostFromAddr(candidate)
	if net.ParseIP(host) == nil {
		return ""
	}
	return host
}

func firstForwardedProto(r *http.Request) string {
	if r == nil {
		return ""
	}
	value := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if value == "" {
		return ""
	}
	proto, _, found := strings.Cut(value, ",")
	if found {
		return strings.TrimSpace(proto)
	}
	return value
}
