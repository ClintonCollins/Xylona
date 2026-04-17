package nodetls

import (
	"crypto/tls"
	"errors"
	"fmt"
)

// NewServerTLSConfig builds a *tls.Config the node binary mounts on its HTTPS
// listener. Server presents the supplied certificate; client authentication is
// NOT required — controllers authenticate themselves via the
// "Authorization: Bearer <shared_secret>" header validated at the application
// layer.
func NewServerTLSConfig(certPEM []byte, keyPEM []byte) (*tls.Config, error) {
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return nil, errors.New("nodetls: certificate and key PEM are required")
	}

	certificate, errLoad := tls.X509KeyPair(certPEM, keyPEM)
	if errLoad != nil {
		return nil, fmt.Errorf("nodetls: load key pair: %w", errLoad)
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate},
		// HTTP/2 over TLS requires the ALPN proto. ConnectRPC negotiates HTTP/2
		// when available; falling back to HTTP/1.1 keeps it compatible with
		// callers that have not enabled HTTP/2 client transports.
		NextProtos: []string{"h2", "http/1.1"},
	}, nil
}
