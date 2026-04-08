package federation

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// MTLS manages local federation certificates and trusted outbound clients.
type MTLS struct {
	federationPort int
	certPath       string
	keyPath        string
	certificate    tls.Certificate
}

// NewMTLS loads or creates the on-disk federation certificate pair for a node.
func NewMTLS(nodeID string, federationPort int, certPath string, keyPath string) (*MTLS, string, error) {
	errEnsure := ensureFederationCertificateFiles(nodeID, certPath, keyPath)
	if errEnsure != nil {
		return nil, "", errEnsure
	}

	certificate, errLoad := tls.LoadX509KeyPair(certPath, keyPath)
	if errLoad != nil {
		return nil, "", fmt.Errorf("load federation key pair: %w", errLoad)
	}

	fingerprint, errFingerprint := certificateFingerprintFromTLSCertificate(certificate)
	if errFingerprint != nil {
		return nil, "", errFingerprint
	}

	return &MTLS{
		federationPort: federationPort,
		certPath:       certPath,
		keyPath:        keyPath,
		certificate:    certificate,
	}, fingerprint, nil
}

// NewMTLSFromPEM constructs a MTLS instance from in-memory PEM data.
func NewMTLSFromPEM(federationPort int, certPEM []byte, keyPEM []byte) (*MTLS, string, error) {
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return nil, "", errors.New("certificate and key PEM are required")
	}

	certificate, errLoad := tls.X509KeyPair(certPEM, keyPEM)
	if errLoad != nil {
		return nil, "", fmt.Errorf("load federation key pair from PEM: %w", errLoad)
	}

	fingerprint, errFingerprint := certificateFingerprintFromTLSCertificate(certificate)
	if errFingerprint != nil {
		return nil, "", errFingerprint
	}

	return &MTLS{
		federationPort: federationPort,
		certificate:    certificate,
	}, fingerprint, nil
}

// GenerateCertificatePEM creates a self-signed federation certificate and key pair.
func GenerateCertificatePEM(nodeID string) ([]byte, []byte, error) {
	certPEM, keyPEM, errGenerate := buildFederationCertificatePEM(nodeID)
	if errGenerate != nil {
		return nil, nil, errGenerate
	}
	return certPEM, keyPEM, nil
}

// CertPath returns the configured certificate path when the certificate is file-backed.
func (m *MTLS) CertPath() string {
	return m.certPath
}

// KeyPath returns the configured private-key path when the key is file-backed.
func (m *MTLS) KeyPath() string {
	return m.keyPath
}

// FederationPort returns the configured federation listener port.
func (m *MTLS) FederationPort() int {
	return m.federationPort
}

// FederationBaseURL returns the HTTPS federation base URL for the configured federation port.
func (m *MTLS) FederationBaseURL(baseURL string) (string, error) {
	return m.FederationBaseURLWithPort(baseURL, m.federationPort)
}

// FederationBaseURLWithPort returns the HTTPS federation base URL for a specific federation port.
func (m *MTLS) FederationBaseURLWithPort(baseURL string, federationPort int) (string, error) {
	parsedURL, errParse := url.Parse(strings.TrimSpace(baseURL))
	if errParse != nil {
		return "", errors.New("invalid base URL")
	}

	hostName := strings.TrimSpace(parsedURL.Hostname())
	if hostName == "" {
		return "", errors.New("invalid base URL host")
	}

	if federationPort <= 0 {
		return "", errors.New("invalid federation port")
	}

	federationURL := &url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(hostName, strconv.Itoa(federationPort)),
	}

	return federationURL.String(), nil
}

// ServerTLSConfig returns the TLS configuration used by the local federation server.
func (m *MTLS) ServerTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{m.certificate},
		ClientAuth:   tls.RequireAnyClientCert,
	}
}

// NewNodeHTTPClient creates a federation HTTP client for a specific node using the default federation port.
func (m *MTLS) NewNodeHTTPClient(timeout time.Duration, nodeBaseURL string, expectedPeerFingerprint string, expectedPeerNodeID string) (*http.Client, string, error) {
	return m.NewNodeHTTPClientWithPort(timeout, nodeBaseURL, m.federationPort, expectedPeerFingerprint, expectedPeerNodeID)
}

// NewNodeHTTPClientWithPort creates a federation HTTP client pinned to a peer fingerprint and port.
func (m *MTLS) NewNodeHTTPClientWithPort(
	timeout time.Duration,
	nodeBaseURL string,
	federationPort int,
	expectedPeerFingerprint string,
	expectedPeerNodeID string,
) (*http.Client, string, error) {
	if federationPort <= 0 {
		federationPort = m.federationPort
	}

	federationBaseURL, errBaseURL := m.FederationBaseURLWithPort(nodeBaseURL, federationPort)
	if errBaseURL != nil {
		return nil, "", errBaseURL
	}

	expectedPeerFingerprint = strings.TrimSpace(expectedPeerFingerprint)
	if expectedPeerFingerprint == "" {
		return nil, "", errors.New("expected peer fingerprint is required")
	}

	normalizedExpectedNodeID := strings.TrimSpace(expectedPeerNodeID)

	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		baseTransport = &http.Transport{}
	}
	transport := baseTransport.Clone()
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		Certificates:       []tls.Certificate{m.certificate},
		InsecureSkipVerify: true, // #nosec G402 -- peer verification is handled by VerifyConnection with fingerprint pinning.
		VerifyConnection: func(connectionState tls.ConnectionState) error {
			if len(connectionState.PeerCertificates) == 0 {
				return errors.New("peer certificate is required")
			}

			peerCertificate := connectionState.PeerCertificates[0]
			peerFingerprint := CertificateFingerprint(peerCertificate)
			if subtle.ConstantTimeCompare(
				[]byte(strings.ToLower(peerFingerprint)),
				[]byte(strings.ToLower(expectedPeerFingerprint)),
			) != 1 {
				return errors.New("peer certificate fingerprint mismatch")
			}

			if normalizedExpectedNodeID != "" && peerCertificate.Subject.CommonName != "" &&
				!strings.EqualFold(peerCertificate.Subject.CommonName, normalizedExpectedNodeID) {
				return errors.New("peer certificate common name mismatch")
			}

			return nil
		},
	}
	transport.TLSClientConfig = tlsConfig

	client := &http.Client{Transport: transport}
	if timeout > 0 {
		client.Timeout = timeout
	}

	return client, federationBaseURL, nil
}

// ProbeServerFingerprint probes a node and returns the presented certificate fingerprint.
func (m *MTLS) ProbeServerFingerprint(nodeBaseURL string, timeout time.Duration) (string, error) {
	return m.ProbeServerFingerprintWithPort(nodeBaseURL, m.federationPort, timeout)
}

// ProbeServerFingerprintWithPort probes a node on a specific federation port and returns its certificate fingerprint.
func (m *MTLS) ProbeServerFingerprintWithPort(nodeBaseURL string, federationPort int, timeout time.Duration) (string, error) {
	if federationPort <= 0 {
		federationPort = m.federationPort
	}

	federationBaseURL, errBaseURL := m.FederationBaseURLWithPort(nodeBaseURL, federationPort)
	if errBaseURL != nil {
		return "", errBaseURL
	}

	parsedURL, errParse := url.Parse(federationBaseURL)
	if errParse != nil {
		return "", errors.New("invalid federation URL")
	}

	dialTimeout := timeout
	if dialTimeout <= 0 {
		dialTimeout = 15 * time.Second
	}

	dialer := &net.Dialer{
		Timeout: dialTimeout,
	}

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		Certificates:       []tls.Certificate{m.certificate},
		InsecureSkipVerify: true, // #nosec G402 -- fingerprint is read directly from the peer cert for pinning.
	}

	conn, errDial := tls.DialWithDialer(dialer, "tcp", parsedURL.Host, tlsConfig) //nolint:noctx // tls.DialWithDialer doesn't have a context variant that fits here
	if errDial != nil {
		return "", fmt.Errorf("dial federation peer %s: %w", parsedURL.Host, errDial)
	}
	defer func() {
		_ = conn.Close()
	}()

	connectionState := conn.ConnectionState()
	if len(connectionState.PeerCertificates) == 0 {
		return "", errors.New("peer certificate is required")
	}

	return CertificateFingerprint(connectionState.PeerCertificates[0]), nil
}

// TrustedPeerLookup provides the federation trust-store query needed by NewTrustedPeerHTTPClient.
type TrustedPeerLookup interface {
	GetFederationTrustedPeerLookup(nodeID string) (*TrustedPeerInfo, error)
}

// TrustedPeerInfo holds the fields that NewTrustedPeerHTTPClient needs from a trusted peer record.
type TrustedPeerInfo struct {
	PeerNodeID      string
	PeerFingerprint string
	Enabled         bool
	Revoked         bool
}

// NewTrustedPeerHTTPClient creates an mTLS HTTP client for a trusted remote node.
// It looks up the peer in the trust store, verifies it is enabled and not revoked,
// and returns a client configured with fingerprint-pinned TLS.
func (m *MTLS) NewTrustedPeerHTTPClient(
	timeout time.Duration,
	nodeID string,
	nodeBaseURL string,
	trustLookup TrustedPeerLookup,
) (*http.Client, string, error) {
	return m.NewTrustedPeerHTTPClientWithPort(timeout, nodeID, nodeBaseURL, m.federationPort, trustLookup)
}

// NewTrustedPeerHTTPClientWithPort creates an mTLS HTTP client for a trusted remote node on a specific federation port.
func (m *MTLS) NewTrustedPeerHTTPClientWithPort(
	timeout time.Duration,
	nodeID string,
	nodeBaseURL string,
	federationPort int,
	trustLookup TrustedPeerLookup,
) (*http.Client, string, error) {
	if federationPort <= 0 {
		federationPort = m.federationPort
	}

	peer, errPeer := trustLookup.GetFederationTrustedPeerLookup(nodeID)
	if errPeer != nil {
		return nil, "", fmt.Errorf("lookup federation trusted peer %s: %w", nodeID, errPeer)
	}
	if !peer.Enabled || peer.Revoked {
		return nil, "", errors.New("remote federation peer is disabled or revoked")
	}

	return m.NewNodeHTTPClientWithPort(timeout, nodeBaseURL, federationPort, peer.PeerFingerprint, peer.PeerNodeID)
}

// CertificateFingerprint returns the SHA-256 fingerprint for a certificate.
func CertificateFingerprint(certificate *x509.Certificate) string {
	fingerprint := sha256.Sum256(certificate.Raw)
	return hex.EncodeToString(fingerprint[:])
}

func certificateFingerprintFromTLSCertificate(certificate tls.Certificate) (string, error) {
	if len(certificate.Certificate) == 0 {
		return "", errors.New("certificate chain is empty")
	}

	parsedCertificate, errParse := x509.ParseCertificate(certificate.Certificate[0])
	if errParse != nil {
		return "", fmt.Errorf("parse federation certificate: %w", errParse)
	}

	return CertificateFingerprint(parsedCertificate), nil
}

func ensureFederationCertificateFiles(nodeID string, certPath string, keyPath string) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return errors.New("node ID is required")
	}

	if certPath == "" || keyPath == "" {
		return errors.New("certificate and key paths are required")
	}

	certExists := fileExists(certPath)
	keyExists := fileExists(keyPath)
	if certExists && keyExists {
		return nil
	}

	if certExists != keyExists {
		log.Warn().Bool("cert_exists", certExists).Bool("key_exists", keyExists).Msg("Federation certificate files are in an inconsistent state — regenerating both. This node will get a new identity.")
	}

	errMkdirCert := os.MkdirAll(filepath.Dir(certPath), 0700)
	if errMkdirCert != nil {
		return fmt.Errorf("create certificate directory %s: %w", filepath.Dir(certPath), errMkdirCert)
	}
	errMkdirKey := os.MkdirAll(filepath.Dir(keyPath), 0700)
	if errMkdirKey != nil {
		return fmt.Errorf("create key directory %s: %w", filepath.Dir(keyPath), errMkdirKey)
	}

	certPEM, keyPEM, errGenerate := buildFederationCertificatePEM(nodeID)
	if errGenerate != nil {
		return errGenerate
	}

	errWriteCert := os.WriteFile(certPath, certPEM, 0600)
	if errWriteCert != nil {
		return fmt.Errorf("write federation certificate %s: %w", certPath, errWriteCert)
	}
	errWriteKey := os.WriteFile(keyPath, keyPEM, 0600)
	if errWriteKey != nil {
		// Clean up the cert file to avoid leaving an inconsistent state on disk.
		_ = os.Remove(certPath)
		return fmt.Errorf("write federation key %s: %w", keyPath, errWriteKey)
	}

	return nil
}

func buildFederationCertificatePEM(nodeID string) ([]byte, []byte, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, nil, errors.New("node ID is required")
	}

	privateKey, errGenerateKey := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if errGenerateKey != nil {
		return nil, nil, fmt.Errorf("generate federation private key: %w", errGenerateKey)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, errSerial := rand.Int(rand.Reader, serialLimit)
	if errSerial != nil {
		return nil, nil, fmt.Errorf("generate certificate serial number: %w", errSerial)
	}

	now := time.Now().UTC()
	certificateTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   nodeID,
			Organization: []string{"Xylona Federation"},
		},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	derBytes, errCreateCertificate := x509.CreateCertificate(rand.Reader, certificateTemplate, certificateTemplate, &privateKey.PublicKey, privateKey)
	if errCreateCertificate != nil {
		return nil, nil, fmt.Errorf("create federation certificate: %w", errCreateCertificate)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})
	if certPEM == nil {
		return nil, nil, errors.New("failed to encode certificate PEM")
	}

	keyBytes, errMarshalKey := x509.MarshalECPrivateKey(privateKey)
	if errMarshalKey != nil {
		return nil, nil, fmt.Errorf("marshal federation private key: %w", errMarshalKey)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})
	if keyPEM == nil {
		return nil, nil, errors.New("failed to encode key PEM")
	}

	return certPEM, keyPEM, nil
}

func fileExists(path string) bool {
	_, errStat := os.Stat(path)
	return errStat == nil
}

// LocalFingerprint returns the SHA-256 fingerprint of the local federation certificate.
func (m *MTLS) LocalFingerprint() (string, error) {
	fingerprint, errFingerprint := certificateFingerprintFromTLSCertificate(m.certificate)
	if errFingerprint != nil {
		return "", fmt.Errorf("failed to get local certificate fingerprint: %w", errFingerprint)
	}
	return fingerprint, nil
}
