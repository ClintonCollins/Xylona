package helpers

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

type FederationMTLS struct {
	federationPort int
	certPath       string
	keyPath        string
	certificate    tls.Certificate
}

func NewFederationMTLS(nodeID string, federationPort int, certPath string, keyPath string) (*FederationMTLS, string, error) {
	errEnsure := ensureFederationCertificateFiles(nodeID, certPath, keyPath)
	if errEnsure != nil {
		return nil, "", errEnsure
	}

	certificate, errLoad := tls.LoadX509KeyPair(certPath, keyPath)
	if errLoad != nil {
		return nil, "", errLoad
	}

	fingerprint, errFingerprint := certificateFingerprintFromTLSCertificate(certificate)
	if errFingerprint != nil {
		return nil, "", errFingerprint
	}

	return &FederationMTLS{
		federationPort: federationPort,
		certPath:       certPath,
		keyPath:        keyPath,
		certificate:    certificate,
	}, fingerprint, nil
}

func NewFederationMTLSFromPEM(federationPort int, certPEM []byte, keyPEM []byte) (*FederationMTLS, string, error) {
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return nil, "", errors.New("certificate and key PEM are required")
	}

	certificate, errLoad := tls.X509KeyPair(certPEM, keyPEM)
	if errLoad != nil {
		return nil, "", errLoad
	}

	fingerprint, errFingerprint := certificateFingerprintFromTLSCertificate(certificate)
	if errFingerprint != nil {
		return nil, "", errFingerprint
	}

	return &FederationMTLS{
		federationPort: federationPort,
		certificate:    certificate,
	}, fingerprint, nil
}

func GenerateFederationCertificatePEM(nodeID string) ([]byte, []byte, error) {
	certPEM, keyPEM, errGenerate := generateFederationCertificatePEM(nodeID)
	if errGenerate != nil {
		return nil, nil, errGenerate
	}
	return certPEM, keyPEM, nil
}

func (m *FederationMTLS) CertPath() string {
	return m.certPath
}

func (m *FederationMTLS) KeyPath() string {
	return m.keyPath
}

func (m *FederationMTLS) FederationPort() int {
	return m.federationPort
}

func (m *FederationMTLS) FederationBaseURL(baseURL string) (string, error) {
	return m.FederationBaseURLWithPort(baseURL, m.federationPort)
}

func (m *FederationMTLS) FederationBaseURLWithPort(baseURL string, federationPort int) (string, error) {
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

func (m *FederationMTLS) ServerTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{m.certificate},
		ClientAuth:   tls.RequireAnyClientCert,
	}
}

func (m *FederationMTLS) NewNodeHTTPClient(timeout time.Duration, nodeBaseURL string, expectedPeerFingerprint string, expectedPeerNodeID string) (*http.Client, string, error) {
	return m.NewNodeHTTPClientWithPort(timeout, nodeBaseURL, m.federationPort, expectedPeerFingerprint, expectedPeerNodeID)
}

func (m *FederationMTLS) NewNodeHTTPClientWithPort(
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

	transport := http.DefaultTransport.(*http.Transport).Clone()
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

func (m *FederationMTLS) ProbeServerFingerprint(nodeBaseURL string, timeout time.Duration) (string, error) {
	return m.ProbeServerFingerprintWithPort(nodeBaseURL, m.federationPort, timeout)
}

func (m *FederationMTLS) ProbeServerFingerprintWithPort(nodeBaseURL string, federationPort int, timeout time.Duration) (string, error) {
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
		return "", errDial
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
func (m *FederationMTLS) NewTrustedPeerHTTPClient(
	timeout time.Duration,
	nodeID string,
	nodeBaseURL string,
	trustLookup TrustedPeerLookup,
) (*http.Client, string, error) {
	return m.NewTrustedPeerHTTPClientWithPort(timeout, nodeID, nodeBaseURL, m.federationPort, trustLookup)
}

func (m *FederationMTLS) NewTrustedPeerHTTPClientWithPort(
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
		return nil, "", errPeer
	}
	if !peer.Enabled || peer.Revoked {
		return nil, "", errors.New("remote federation peer is disabled or revoked")
	}

	return m.NewNodeHTTPClientWithPort(timeout, nodeBaseURL, federationPort, peer.PeerFingerprint, peer.PeerNodeID)
}

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
		return "", errParse
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
		return errMkdirCert
	}
	errMkdirKey := os.MkdirAll(filepath.Dir(keyPath), 0700)
	if errMkdirKey != nil {
		return errMkdirKey
	}

	certPEM, keyPEM, errGenerate := generateFederationCertificatePEM(nodeID)
	if errGenerate != nil {
		return errGenerate
	}

	errWriteCert := os.WriteFile(certPath, certPEM, 0600)
	if errWriteCert != nil {
		return errWriteCert
	}
	errWriteKey := os.WriteFile(keyPath, keyPEM, 0600)
	if errWriteKey != nil {
		// Clean up the cert file to avoid leaving an inconsistent state on disk.
		_ = os.Remove(certPath)
		return errWriteKey
	}

	return nil
}

func generateFederationCertificatePEM(nodeID string) ([]byte, []byte, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, nil, errors.New("node ID is required")
	}

	privateKey, errGenerateKey := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if errGenerateKey != nil {
		return nil, nil, errGenerateKey
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, errSerial := rand.Int(rand.Reader, serialLimit)
	if errSerial != nil {
		return nil, nil, errSerial
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
		return nil, nil, errCreateCertificate
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
		return nil, nil, errMarshalKey
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

func (m *FederationMTLS) LocalFingerprint() (string, error) {
	fingerprint, errFingerprint := certificateFingerprintFromTLSCertificate(m.certificate)
	if errFingerprint != nil {
		return "", fmt.Errorf("failed to get local certificate fingerprint: %w", errFingerprint)
	}
	return fingerprint, nil
}
