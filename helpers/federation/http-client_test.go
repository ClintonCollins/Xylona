package federation

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewHTTPClientRejectsUntrustedTLSByDefault(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewHTTPClient(2*time.Second, false)
	req, errReq := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if errReq != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", errReq)
	}
	_, errGet := client.Do(req) //nolint:bodyclose // error expected; response will be nil
	if errGet == nil {
		t.Fatalf("NewHTTPClient() default TLS verification should fail for untrusted cert")
	}
}

func TestNewHTTPClientAllowsUntrustedTLSWhenEnabled(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewHTTPClient(2*time.Second, true)
	req, errReq := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if errReq != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", errReq)
	}
	resp, errGet := client.Do(req)
	if errGet != nil {
		t.Fatalf("NewHTTPClient() with insecure TLS should succeed: %v", errGet)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("response status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func TestNewHTTPClientHonorsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewHTTPClient(40*time.Millisecond, false)
	req, errReq := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if errReq != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", errReq)
	}
	_, errGet := client.Do(req) //nolint:bodyclose // timeout expected; response will be nil
	if errGet == nil {
		t.Fatalf("NewHTTPClient() request error = nil, want timeout error")
	}
	if !strings.Contains(strings.ToLower(errGet.Error()), "timeout") && !errors.Is(errGet, http.ErrHandlerTimeout) {
		t.Fatalf("NewHTTPClient() timeout error = %v, want timeout-related error", errGet)
	}
}

func TestNewHTTPClientZeroTimeoutLeavesClientTimeoutUnset(t *testing.T) {
	client := NewHTTPClient(0, false)
	if client.Timeout != 0 {
		t.Fatalf("client.Timeout = %v, want %v", client.Timeout, 0)
	}
}
