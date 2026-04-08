package federation

import (
	"crypto/tls"
	"net/http"
	"time"
)

// NewHTTPClient creates a federation HTTP client with optional insecure TLS and timeout settings.
func NewHTTPClient(timeout time.Duration, allowInsecureTLS bool) *http.Client {
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		baseTransport = &http.Transport{}
	}
	transport := baseTransport.Clone()
	tlsConfig := &tls.Config{}
	if transport.TLSClientConfig != nil {
		tlsConfig = transport.TLSClientConfig.Clone()
	}
	// #nosec G402 -- this is explicitly controlled per node by admin configuration.
	tlsConfig.InsecureSkipVerify = allowInsecureTLS
	transport.TLSClientConfig = tlsConfig

	client := &http.Client{
		Transport: transport,
	}
	if timeout > 0 {
		client.Timeout = timeout
	}

	return client
}
