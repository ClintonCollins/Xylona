package gatekeeper

import (
	"fmt"
	"net"
	"net/http"
	"slices"
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
	for part := range strings.SplitSeq(raw, ",") {
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
	return p.isTrustedIP(ip)
}

func (p *ProxyTrust) isTrustedIP(ip net.IP) bool {
	if p == nil || ip == nil {
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
// when the immediate peer is a configured trusted proxy. Trusted proxy hops
// are removed from right to left so client-supplied leftmost values cannot
// replace the nearest untrusted address.
func (p *ProxyTrust) ClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if p.IsTrustedRemote(r) {
		forwarded := p.forwardedClientIP(r)
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

func (p *ProxyTrust) forwardedClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	values := r.Header.Values("X-Forwarded-For")
	for _, value := range slices.Backward(values) {
		parts := strings.Split(value, ",")
		for _, part := range slices.Backward(parts) {
			candidate := strings.TrimSpace(part)
			ip := net.ParseIP(candidate)
			if ip == nil {
				ip = net.ParseIP(hostFromAddr(candidate))
			}
			if ip == nil {
				return ""
			}
			if !p.isTrustedIP(ip) {
				return ip.String()
			}
		}
	}
	return ""
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
