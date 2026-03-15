package helpers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewFederationHTTPClientRejectsUntrustedTLSByDefault(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewFederationHTTPClient(2*time.Second, false)
	_, errGet := client.Get(server.URL)
	if errGet == nil {
		t.Fatalf("NewFederationHTTPClient() default TLS verification should fail for untrusted cert")
	}
}

func TestNewFederationHTTPClientAllowsUntrustedTLSWhenEnabled(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewFederationHTTPClient(2*time.Second, true)
	resp, errGet := client.Get(server.URL)
	if errGet != nil {
		t.Fatalf("NewFederationHTTPClient() with insecure TLS should succeed: %v", errGet)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("response status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}
