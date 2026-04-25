package nodetls

import (
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DefaultClientTimeout is the per-request timeout applied to clients
// constructed via NewPinnedTLSClient when the caller does not override it.
const DefaultClientTimeout = 30 * time.Second

// NewPinnedTLSClient returns an *http.Client whose TLS config trusts ONLY
// peers presenting a leaf certificate matching the provided lowercase hex
// SHA-256 fingerprint. The system trust store is intentionally not consulted —
// pinning is the sole authentication.
//
// The returned client uses a clone of http.DefaultTransport so callers can
// further customize per-call timeouts via context without affecting the
// shared base transport.
func NewPinnedTLSClient(fingerprint string) (*http.Client, error) {
	normalizedFingerprint := strings.ToLower(strings.TrimSpace(fingerprint))
	if normalizedFingerprint == "" {
		return nil, errors.New("nodetls: fingerprint is required")
	}

	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		baseTransport = &http.Transport{}
	}
	transport := baseTransport.Clone()

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true, // #nosec G402 -- pinning is enforced by VerifyConnection below.
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errors.New("nodetls: peer presented no certificate")
			}
			peerFingerprint := FingerprintFromCertificate(state.PeerCertificates[0])
			match := subtle.ConstantTimeCompare(
				[]byte(strings.ToLower(peerFingerprint)),
				[]byte(normalizedFingerprint),
			)
			if match != 1 {
				return fmt.Errorf("nodetls: peer fingerprint mismatch: got %s", peerFingerprint)
			}
			return nil
		},
	}
	transport.TLSClientConfig = tlsConfig
	transport.ForceAttemptHTTP2 = true

	return &http.Client{
		Transport: transport,
		Timeout:   DefaultClientTimeout,
	}, nil
}
