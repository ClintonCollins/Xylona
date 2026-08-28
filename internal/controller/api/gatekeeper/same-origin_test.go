package gatekeeper

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsSameOriginRequest(t *testing.T) {
	t.Parallel()

	trust, errTrust := ParseTrustedProxies("127.0.0.1")
	if errTrust != nil {
		t.Fatalf("ParseTrustedProxies() error = %v", errTrust)
	}

	tests := []struct {
		name           string
		requestURL     string
		requestHost    string
		remoteAddr     string
		origin         string
		forwardedHost  string
		forwardedProto string
		trust          *ProxyTrust
		want           bool
	}{
		{
			name:        "direct same origin",
			requestURL:  "https://xylona.test/api/websocket",
			requestHost: "xylona.test",
			origin:      "https://xylona.test",
			want:        true,
		},
		{
			name:        "same-site sibling is rejected",
			requestURL:  "https://xylona.test/api/websocket",
			requestHost: "xylona.test",
			origin:      "https://sibling.xylona.test",
		},
		{
			name:           "trusted proxy effective origin",
			requestURL:     "http://internal.proxy/api/websocket",
			requestHost:    "internal.proxy",
			remoteAddr:     "127.0.0.1:443",
			origin:         "https://xylona.test",
			forwardedHost:  "xylona.test",
			forwardedProto: "https",
			trust:          trust,
			want:           true,
		},
		{
			name:           "untrusted forwarded origin is rejected",
			requestURL:     "http://internal.proxy/api/websocket",
			requestHost:    "internal.proxy",
			remoteAddr:     "192.0.2.1:443",
			origin:         "https://xylona.test",
			forwardedHost:  "xylona.test",
			forwardedProto: "https",
			trust:          trust,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.requestURL, nil)
			req.Host = tt.requestHost
			req.RemoteAddr = tt.remoteAddr
			req.Header.Set("Origin", tt.origin)
			req.Header.Set("X-Forwarded-Host", tt.forwardedHost)
			req.Header.Set("X-Forwarded-Proto", tt.forwardedProto)

			got := IsSameOriginRequest(req, tt.trust)
			if got != tt.want {
				t.Fatalf("IsSameOriginRequest() = %t, want %t", got, tt.want)
			}
		})
	}
}
