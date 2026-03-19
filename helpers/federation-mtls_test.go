package helpers

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestNewFederationMTLSFromPEM(t *testing.T) {
	certPEM, keyPEM, errGenerate := GenerateFederationCertificatePEM("node-a")
	if errGenerate != nil {
		t.Fatalf("GenerateFederationCertificatePEM() error = %v", errGenerate)
	}

	mtlsConfig, fingerprint, errCreate := NewFederationMTLSFromPEM(8443, certPEM, keyPEM)
	if errCreate != nil {
		t.Fatalf("NewFederationMTLSFromPEM() error = %v", errCreate)
	}
	if mtlsConfig == nil {
		t.Fatalf("NewFederationMTLSFromPEM() returned nil config")
	}
	if fingerprint == "" {
		t.Fatalf("fingerprint should not be empty")
	}

	localFingerprint, errLocalFingerprint := mtlsConfig.LocalFingerprint()
	if errLocalFingerprint != nil {
		t.Fatalf("LocalFingerprint() error = %v", errLocalFingerprint)
	}
	if localFingerprint != fingerprint {
		t.Fatalf("LocalFingerprint() = %q, want %q", localFingerprint, fingerprint)
	}
}

func TestNewFederationMTLSGeneratesAndReusesCertificate(t *testing.T) {
	certPath := filepath.Join(t.TempDir(), "federation", "node.crt")
	keyPath := filepath.Join(t.TempDir(), "federation", "node.key")

	mtlsConfig, firstFingerprint, errCreate := NewFederationMTLS("node-a", 8443, certPath, keyPath)
	if errCreate != nil {
		t.Fatalf("NewFederationMTLS() first call error = %v", errCreate)
	}
	if mtlsConfig == nil {
		t.Fatalf("NewFederationMTLS() returned nil config")
	}
	if firstFingerprint == "" {
		t.Fatalf("fingerprint should not be empty")
	}

	secondConfig, secondFingerprint, errSecond := NewFederationMTLS("node-a", 8443, certPath, keyPath)
	if errSecond != nil {
		t.Fatalf("NewFederationMTLS() second call error = %v", errSecond)
	}
	if secondConfig == nil {
		t.Fatalf("second config should not be nil")
	}
	if firstFingerprint != secondFingerprint {
		t.Fatalf("fingerprints differ across loads: first=%q second=%q", firstFingerprint, secondFingerprint)
	}
}

func TestFederationBaseURLUsesDedicatedPort(t *testing.T) {
	mtlsConfig, _, errCreate := NewFederationMTLS(
		"node-a",
		9443,
		filepath.Join(t.TempDir(), "node.crt"),
		filepath.Join(t.TempDir(), "node.key"),
	)
	if errCreate != nil {
		t.Fatalf("NewFederationMTLS() error = %v", errCreate)
	}

	gotURL, errBaseURL := mtlsConfig.FederationBaseURL("http://panel.example.com:8080/path")
	if errBaseURL != nil {
		t.Fatalf("FederationBaseURL() error = %v", errBaseURL)
	}

	if gotURL != "https://panel.example.com:9443" {
		t.Fatalf("FederationBaseURL() = %q, want %q", gotURL, "https://panel.example.com:9443")
	}
}

func TestFederationBaseURLWithPortOverridesDefault(t *testing.T) {
	mtlsConfig, _, errCreate := NewFederationMTLS(
		"node-a",
		9443,
		filepath.Join(t.TempDir(), "node.crt"),
		filepath.Join(t.TempDir(), "node.key"),
	)
	if errCreate != nil {
		t.Fatalf("NewFederationMTLS() error = %v", errCreate)
	}

	gotURL, errBaseURL := mtlsConfig.FederationBaseURLWithPort("http://panel.example.com:8080/path", 12443)
	if errBaseURL != nil {
		t.Fatalf("FederationBaseURLWithPort() error = %v", errBaseURL)
	}

	if gotURL != "https://panel.example.com:12443" {
		t.Fatalf("FederationBaseURLWithPort() = %q, want %q", gotURL, "https://panel.example.com:12443")
	}
}

func TestNewNodeHTTPClientPinsPeerFingerprint(t *testing.T) {
	serverCertPath := filepath.Join(t.TempDir(), "peer.crt")
	serverKeyPath := filepath.Join(t.TempDir(), "peer.key")
	serverMTLS, serverFingerprint, errServer := NewFederationMTLS("peer-node-id", 1, serverCertPath, serverKeyPath)
	if errServer != nil {
		t.Fatalf("NewFederationMTLS() server error = %v", errServer)
	}

	serverCertificate, errLoadServerCert := tls.LoadX509KeyPair(serverMTLS.CertPath(), serverMTLS.KeyPath())
	if errLoadServerCert != nil {
		t.Fatalf("tls.LoadX509KeyPair() server cert error = %v", errLoadServerCert)
	}

	testServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	testServer.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverCertificate},
		ClientAuth:   tls.RequireAnyClientCert,
	}
	testServer.StartTLS()
	t.Cleanup(testServer.Close)

	parsedURL, errParse := url.Parse(testServer.URL)
	if errParse != nil {
		t.Fatalf("url.Parse() error = %v", errParse)
	}
	portNumber, errPort := strconv.Atoi(parsedURL.Port())
	if errPort != nil {
		t.Fatalf("failed to parse test server port: %v", errPort)
	}

	clientMTLS, _, errClient := NewFederationMTLS(
		"client-node-id",
		portNumber,
		filepath.Join(t.TempDir(), "client.crt"),
		filepath.Join(t.TempDir(), "client.key"),
	)
	if errClient != nil {
		t.Fatalf("NewFederationMTLS() client error = %v", errClient)
	}

	httpClient, federationBaseURL, errPinned := clientMTLS.NewNodeHTTPClient(
		2*time.Second,
		testServer.URL,
		serverFingerprint,
		"peer-node-id",
	)
	if errPinned != nil {
		t.Fatalf("NewNodeHTTPClient() error = %v", errPinned)
	}

	resp, errGet := httpClient.Get(federationBaseURL)
	if errGet != nil {
		t.Fatalf("httpClient.Get() with pinned fingerprint error = %v", errGet)
	}
	if errClose := resp.Body.Close(); errClose != nil {
		t.Fatalf("failed to close response body: %v", errClose)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	badClient, badFederationBaseURL, errBadClient := clientMTLS.NewNodeHTTPClient(
		2*time.Second,
		testServer.URL,
		"not-the-right-fingerprint",
		"peer-node-id",
	)
	if errBadClient != nil {
		t.Fatalf("NewNodeHTTPClient() bad fingerprint setup error = %v", errBadClient)
	}

	badResp, errBadGet := badClient.Get(badFederationBaseURL)
	if badResp != nil {
		_ = badResp.Body.Close()
	}
	if errBadGet == nil {
		t.Fatalf("expected pinned-fingerprint client request to fail, got nil error")
	}
}

func TestNewNodeHTTPClientWithPortOverride(t *testing.T) {
	serverCertPath := filepath.Join(t.TempDir(), "peer.crt")
	serverKeyPath := filepath.Join(t.TempDir(), "peer.key")
	serverMTLS, serverFingerprint, errServer := NewFederationMTLS("peer-node-id", 1, serverCertPath, serverKeyPath)
	if errServer != nil {
		t.Fatalf("NewFederationMTLS() server error = %v", errServer)
	}

	serverCertificate, errLoadServerCert := tls.LoadX509KeyPair(serverMTLS.CertPath(), serverMTLS.KeyPath())
	if errLoadServerCert != nil {
		t.Fatalf("tls.LoadX509KeyPair() server cert error = %v", errLoadServerCert)
	}

	testServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	testServer.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverCertificate},
		ClientAuth:   tls.RequireAnyClientCert,
	}
	testServer.StartTLS()
	t.Cleanup(testServer.Close)

	parsedURL, errParse := url.Parse(testServer.URL)
	if errParse != nil {
		t.Fatalf("url.Parse() error = %v", errParse)
	}
	actualPort, errPort := strconv.Atoi(parsedURL.Port())
	if errPort != nil {
		t.Fatalf("failed to parse test server port: %v", errPort)
	}

	// Set an intentionally incorrect default federation port.
	clientMTLS, _, errClient := NewFederationMTLS(
		"client-node-id",
		9443,
		filepath.Join(t.TempDir(), "client.crt"),
		filepath.Join(t.TempDir(), "client.key"),
	)
	if errClient != nil {
		t.Fatalf("NewFederationMTLS() client error = %v", errClient)
	}

	httpClient, federationBaseURL, errPinned := clientMTLS.NewNodeHTTPClientWithPort(
		2*time.Second,
		testServer.URL,
		actualPort,
		serverFingerprint,
		"peer-node-id",
	)
	if errPinned != nil {
		t.Fatalf("NewNodeHTTPClientWithPort() error = %v", errPinned)
	}

	resp, errGet := httpClient.Get(federationBaseURL)
	if errGet != nil {
		t.Fatalf("httpClient.Get() error = %v", errGet)
	}
	if errClose := resp.Body.Close(); errClose != nil {
		t.Fatalf("failed to close response body: %v", errClose)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
