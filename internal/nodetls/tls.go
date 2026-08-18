// Package nodetls provides the certificate generation, server, and pinned
// HTTPS client primitives the hub-spoke node binary uses to authenticate the
// controller <-> node channel.
//
// It is intentionally narrow: there is no trusted-peer store and no
// acting-identity header layer. The model is: a node generates a self-signed certificate,
// publishes its SHA-256 fingerprint to the controller during bootstrap pairing
// (Step 6), and the controller pins that fingerprint on every subsequent
// connection. Application-layer authorization travels in the
// "Authorization: Bearer <shared_secret>" header.
package nodetls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// CertificateTTL is the validity window for generated node certificates.
// Ten years keeps self-hosters from having to rotate by hand.
const CertificateTTL = 10 * 365 * 24 * time.Hour

// GenerateSelfSigned produces a fresh ECDSA P-256 self-signed certificate for
// a node. The returned PEM blocks are suitable for both ServerTLS use and for
// loading via NewPinnedTLSClient on the controller side; fingerprint is the
// hex-encoded SHA-256 of the DER certificate that callers should ship to the
// controller during bootstrap pairing.
//
// ctx is accepted for forward compatibility (future hardware-backed key paths
// may need it); the current implementation only consults it for cancellation
// before doing any expensive work.
func GenerateSelfSigned(ctx context.Context, subjectCN string) (certPEM []byte, keyPEM []byte, fingerprint string, err error) {
	subjectCN = strings.TrimSpace(subjectCN)
	if subjectCN == "" {
		return nil, nil, "", errors.New("nodetls: subject common name is required")
	}

	errCtx := ctx.Err()
	if errCtx != nil {
		return nil, nil, "", fmt.Errorf("nodetls: generate canceled: %w", errCtx)
	}

	privateKey, errKey := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if errKey != nil {
		return nil, nil, "", fmt.Errorf("nodetls: generate private key: %w", errKey)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, errSerial := rand.Int(rand.Reader, serialLimit)
	if errSerial != nil {
		return nil, nil, "", fmt.Errorf("nodetls: generate serial number: %w", errSerial)
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   subjectCN,
			Organization: []string{"Xylona Node"},
		},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(CertificateTTL),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	derBytes, errCreate := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if errCreate != nil {
		return nil, nil, "", fmt.Errorf("nodetls: create certificate: %w", errCreate)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})
	if certPEM == nil {
		return nil, nil, "", errors.New("nodetls: encode certificate PEM")
	}

	keyDER, errMarshalKey := x509.MarshalECPrivateKey(privateKey)
	if errMarshalKey != nil {
		return nil, nil, "", fmt.Errorf("nodetls: marshal private key: %w", errMarshalKey)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})
	if keyPEM == nil {
		return nil, nil, "", errors.New("nodetls: encode key PEM")
	}

	fingerprint = FingerprintFromDER(derBytes)
	return certPEM, keyPEM, fingerprint, nil
}

// FingerprintFromDER returns the lowercase hex SHA-256 fingerprint of a DER
// certificate. Exposed so callers that already have the parsed cert (e.g. the
// VerifyConnection callback) avoid re-encoding.
func FingerprintFromDER(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

// FingerprintFromCertificate returns the lowercase hex SHA-256 fingerprint of
// a parsed *x509.Certificate.
func FingerprintFromCertificate(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	return FingerprintFromDER(cert.Raw)
}

// FingerprintFromPEM returns the SHA-256 fingerprint of the first CERTIFICATE
// block in pemBytes. Useful for callers that have stored the cert PEM and need
// to recompute the pin (for example, after reloading node identity from disk).
func FingerprintFromPEM(pemBytes []byte) (string, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", errors.New("nodetls: PEM does not contain a CERTIFICATE block")
	}
	cert, errParse := x509.ParseCertificate(block.Bytes)
	if errParse != nil {
		return "", fmt.Errorf("nodetls: parse certificate: %w", errParse)
	}
	return FingerprintFromDER(cert.Raw), nil
}
