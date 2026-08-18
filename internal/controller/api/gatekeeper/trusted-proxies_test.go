package gatekeeper

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseTrustedProxies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		addr    string
		want    bool
		wantErr bool
	}{
		{name: "empty list trusts nothing", raw: "", addr: "127.0.0.1:1", want: false},
		{name: "loopback IP", raw: "127.0.0.1", addr: "127.0.0.1:443", want: true},
		{name: "CIDR match", raw: "10.0.0.0/8", addr: "10.1.2.3:80", want: true},
		{name: "CIDR miss", raw: "10.0.0.0/8", addr: "11.0.0.1:80", want: false},
		{name: "invalid entry", raw: "not-an-ip", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			trust, errParse := ParseTrustedProxies(tt.raw)
			if tt.wantErr {
				if errParse == nil {
					t.Fatal("ParseTrustedProxies() error = nil, want error")
				}
				return
			}
			if errParse != nil {
				t.Fatalf("ParseTrustedProxies() error = %v", errParse)
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.addr
			got := trust.IsTrustedRemote(req)
			if got != tt.want {
				t.Fatalf("IsTrustedRemote(%q) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

func TestProxyTrustRequestIsHTTPS(t *testing.T) {
	t.Parallel()

	trust, errTrust := ParseTrustedProxies("127.0.0.1")
	if errTrust != nil {
		t.Fatalf("ParseTrustedProxies() error = %v", errTrust)
	}

	directTLS := httptest.NewRequest(http.MethodGet, "/", nil)
	directTLS.TLS = &tls.ConnectionState{}
	if !(*ProxyTrust)(nil).RequestIsHTTPS(directTLS) {
		t.Fatal("direct TLS should be treated as HTTPS without trusted proxies")
	}

	spoofed := httptest.NewRequest(http.MethodGet, "/", nil)
	spoofed.RemoteAddr = "203.0.113.10:1"
	spoofed.Header.Set("X-Forwarded-Proto", "https")
	if trust.RequestIsHTTPS(spoofed) {
		t.Fatal("untrusted X-Forwarded-Proto should be ignored")
	}

	proxied := httptest.NewRequest(http.MethodGet, "/", nil)
	proxied.RemoteAddr = "127.0.0.1:443"
	proxied.Header.Set("X-Forwarded-Proto", "https")
	if !trust.RequestIsHTTPS(proxied) {
		t.Fatal("trusted X-Forwarded-Proto https should be honored")
	}
}

func TestProxyTrustAnnotateRequestStripsClientHeader(t *testing.T) {
	t.Parallel()

	handler := (*ProxyTrust)(nil).AnnotateRequest(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(InternalHTTPSHeader) != "" {
			t.Fatal("client-supplied internal HTTPS header was not stripped")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(InternalHTTPSHeader, "1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
