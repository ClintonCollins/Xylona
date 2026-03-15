package helpers

import (
	"crypto/tls"
	"net/http"
	"time"
)

func NewFederationHTTPClient(timeout time.Duration, allowInsecureTLS bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
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
