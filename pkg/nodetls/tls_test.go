package nodetls_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/nodetls"
)

// TestGenerateSelfSigned exercises the cert-generation roundtrip: the
// fingerprint reported at generation time matches the one recovered from the
// PEM, and the cert can be loaded into a server TLS config.
func TestGenerateSelfSigned(t *testing.T) {
	t.Parallel()

	t.Run("emits cert and key with matching fingerprint", func(t *testing.T) {
		t.Parallel()

		certPEM, keyPEM, fingerprint, errGenerate := nodetls.GenerateSelfSigned(context.Background(), "node-test")
		if errGenerate != nil {
			t.Fatalf("GenerateSelfSigned returned error: %v", errGenerate)
		}
		if len(certPEM) == 0 || len(keyPEM) == 0 {
			t.Fatal("GenerateSelfSigned returned empty PEM blocks")
		}
		if fingerprint == "" {
			t.Fatal("GenerateSelfSigned returned empty fingerprint")
		}

		recovered, errRecover := nodetls.FingerprintFromPEM(certPEM)
		if errRecover != nil {
			t.Fatalf("FingerprintFromPEM returned error: %v", errRecover)
		}
		if recovered != fingerprint {
			t.Fatalf("fingerprint roundtrip mismatch: got %s want %s", recovered, fingerprint)
		}

		_, errLoad := tls.X509KeyPair(certPEM, keyPEM)
		if errLoad != nil {
			t.Fatalf("tls.X509KeyPair rejected generated material: %v", errLoad)
		}
	})

	t.Run("subject common name is encoded into certificate", func(t *testing.T) {
		t.Parallel()

		certPEM, _, _, errGenerate := nodetls.GenerateSelfSigned(context.Background(), "node-cn-check")
		if errGenerate != nil {
			t.Fatalf("GenerateSelfSigned returned error: %v", errGenerate)
		}

		block, _ := pem.Decode(certPEM)
		if block == nil {
			t.Fatal("certificate PEM did not decode")
		}
		cert, errParse := x509.ParseCertificate(block.Bytes)
		if errParse != nil {
			t.Fatalf("x509.ParseCertificate returned error: %v", errParse)
		}
		if cert.Subject.CommonName != "node-cn-check" {
			t.Fatalf("unexpected CN: got %q want %q", cert.Subject.CommonName, "node-cn-check")
		}
	})

	t.Run("rejects empty common name", func(t *testing.T) {
		t.Parallel()

		_, _, _, errGenerate := nodetls.GenerateSelfSigned(context.Background(), "  ")
		if errGenerate == nil {
			t.Fatal("expected error for empty common name")
		}
	})

	t.Run("respects canceled context", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, _, errGenerate := nodetls.GenerateSelfSigned(ctx, "node-cancel")
		if errGenerate == nil {
			t.Fatal("expected cancellation error")
		}
	})
}

// TestPinnedClient drives the pinned-client + server TLS config end to end. A
// client built with the matching fingerprint reaches the server; one built
// with a wrong fingerprint fails the TLS handshake.
func TestPinnedClient(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM, fingerprint, errGenerate := nodetls.GenerateSelfSigned(context.Background(), "node-pinning")
	if errGenerate != nil {
		t.Fatalf("GenerateSelfSigned returned error: %v", errGenerate)
	}

	serverTLS, errServerTLS := nodetls.NewServerTLSConfig(certPEM, keyPEM)
	if errServerTLS != nil {
		t.Fatalf("NewServerTLSConfig returned error: %v", errServerTLS)
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	server.TLS = serverTLS
	server.StartTLS()
	t.Cleanup(server.Close)

	t.Run("matching fingerprint reaches the server", func(t *testing.T) {
		t.Parallel()

		client, errClient := nodetls.NewPinnedTLSClient(fingerprint)
		if errClient != nil {
			t.Fatalf("NewPinnedTLSClient returned error: %v", errClient)
		}

		req, errReq := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
		if errReq != nil {
			t.Fatalf("NewRequestWithContext returned error: %v", errReq)
		}
		resp, errDo := client.Do(req)
		if errDo != nil {
			t.Fatalf("client.Do returned error: %v", errDo)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })

		body, errRead := io.ReadAll(resp.Body)
		if errRead != nil {
			t.Fatalf("ReadAll returned error: %v", errRead)
		}
		if string(body) != "ok" {
			t.Fatalf("unexpected body: %q", string(body))
		}
	})

	t.Run("wrong fingerprint fails TLS handshake", func(t *testing.T) {
		t.Parallel()

		// 64-char hex string that will never match any real fingerprint.
		const wrongFingerprint = "0000000000000000000000000000000000000000000000000000000000000000"
		client, errClient := nodetls.NewPinnedTLSClient(wrongFingerprint)
		if errClient != nil {
			t.Fatalf("NewPinnedTLSClient returned error: %v", errClient)
		}

		req, errReq := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
		if errReq != nil {
			t.Fatalf("NewRequestWithContext returned error: %v", errReq)
		}
		resp, errDo := client.Do(req)
		if errDo == nil {
			_ = resp.Body.Close()
			t.Fatal("expected TLS handshake failure for wrong fingerprint")
		}
		if !strings.Contains(errDo.Error(), "fingerprint mismatch") {
			t.Fatalf("expected fingerprint mismatch error, got: %v", errDo)
		}
	})

	t.Run("empty fingerprint is rejected at construction", func(t *testing.T) {
		t.Parallel()

		_, errClient := nodetls.NewPinnedTLSClient("  ")
		if errClient == nil {
			t.Fatal("expected error for empty fingerprint")
		}
	})
}

// TestNewServerTLSConfig covers the surface that does not require a live
// listener: PEM validation and key/cert mismatch detection.
func TestNewServerTLSConfig(t *testing.T) {
	t.Parallel()

	t.Run("requires both PEM blocks", func(t *testing.T) {
		t.Parallel()

		_, errEmpty := nodetls.NewServerTLSConfig(nil, nil)
		if errEmpty == nil {
			t.Fatal("expected error when both PEMs are empty")
		}
	})

	t.Run("rejects garbage PEM", func(t *testing.T) {
		t.Parallel()

		_, errBad := nodetls.NewServerTLSConfig([]byte("not a cert"), []byte("not a key"))
		if errBad == nil {
			t.Fatal("expected error for invalid PEM material")
		}
	})
}
