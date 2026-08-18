package rpc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/internal/nodetls"
)

const testNodeBootstrapFingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestNodeBootstrapHandler(t *testing.T) {
	t.Parallel()

	t.Run("rejects non-POST methods", func(t *testing.T) {
		t.Parallel()

		conn := newEncryptedRPCConnection(t, "node-bootstrap-method.sqlite")
		handler := NodeBootstrapHandler(conn, nil)
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/node/bootstrap", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("rejects malformed JSON", func(t *testing.T) {
		t.Parallel()

		conn := newEncryptedRPCConnection(t, "node-bootstrap-json.sqlite")
		handler := NodeBootstrapHandler(conn, nil)
		request := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/api/node/bootstrap",
			strings.NewReader("{"),
		)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
		}
		if !strings.Contains(response.Body.String(), "failed to parse bootstrap request") {
			t.Fatalf("body = %q, want parse error", response.Body.String())
		}
	})

	t.Run("rejects incomplete payloads", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name    string
			payload NodeBootstrapRequest
			want    string
		}{
			{
				name: "missing join token",
				payload: NodeBootstrapRequest{
					ListenURL:       "https://node.example:9500",
					CertPEM:         "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----",
					CertFingerprint: testNodeBootstrapFingerprint,
				},
				want: "join_token is required",
			},
			{
				name: "missing listen URL",
				payload: NodeBootstrapRequest{
					JoinToken:       "token",
					CertPEM:         "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----",
					CertFingerprint: testNodeBootstrapFingerprint,
				},
				want: "listen_url is required",
			},
			{
				name: "http listen URL",
				payload: NodeBootstrapRequest{
					JoinToken:       "token",
					ListenURL:       "http://node.example:9500",
					CertPEM:         "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----",
					CertFingerprint: testNodeBootstrapFingerprint,
				},
				want: "listen_url must be an https URL",
			},
			{
				name: "missing cert PEM",
				payload: NodeBootstrapRequest{
					JoinToken:       "token",
					ListenURL:       "https://node.example:9500",
					CertFingerprint: testNodeBootstrapFingerprint,
				},
				want: "cert_pem is required",
			},
			{
				name: "missing cert fingerprint",
				payload: NodeBootstrapRequest{
					JoinToken: "token",
					ListenURL: "https://node.example:9500",
					CertPEM:   "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----",
				},
				want: "cert_fingerprint is required",
			},
			{
				name: "invalid cert PEM",
				payload: NodeBootstrapRequest{
					JoinToken:       "token",
					ListenURL:       "https://node.example:9500",
					CertPEM:         "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----",
					CertFingerprint: testNodeBootstrapFingerprint,
				},
				want: "cert_pem is invalid",
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()

				conn := newEncryptedRPCConnection(t, "node-bootstrap-"+strings.ReplaceAll(testCase.name, " ", "-")+".sqlite")
				handler := NodeBootstrapHandler(conn, nil)
				response := serveNodeBootstrap(t, handler, testCase.payload)

				if response.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
				}
				if !strings.Contains(response.Body.String(), testCase.want) {
					t.Fatalf("body = %q, want %q", response.Body.String(), testCase.want)
				}
			})
		}
	})

	t.Run("rejects unknown consumed and expired tokens", func(t *testing.T) {
		t.Parallel()

		conn := newEncryptedRPCConnection(t, "node-bootstrap-tokens.sqlite")
		validToken, _, errGenerate := conn.GenerateNodeJoinToken("Remote Node", time.Hour)
		if errGenerate != nil {
			t.Fatalf("GenerateNodeJoinToken() error = %v", errGenerate)
		}
		expiredToken, _, errExpired := conn.GenerateNodeJoinToken("Expired Node", time.Hour)
		if errExpired != nil {
			t.Fatalf("GenerateNodeJoinToken(expired) error = %v", errExpired)
		}
		expiredHash := sha256.Sum256([]byte(expiredToken))
		_, errExpire := conn.SQLDb.ExecContext(
			context.Background(),
			`update node_join_token set expires_at = ? where token_hash = ?`,
			time.Now().UTC().Add(-time.Hour),
			hex.EncodeToString(expiredHash[:]),
		)
		if errExpire != nil {
			t.Fatalf("expire join token error = %v", errExpire)
		}

		handler := NodeBootstrapHandler(conn, nil)
		firstSuccess := serveNodeBootstrap(t, handler, validBootstrapRequest(t, validToken, "Paired Node"))
		if firstSuccess.Code != http.StatusOK {
			t.Fatalf("first consume status = %d, want %d body = %q", firstSuccess.Code, http.StatusOK, firstSuccess.Body.String())
		}

		testCases := []struct {
			name  string
			token string
		}{
			{name: "unknown token", token: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			{name: "already consumed token", token: validToken},
			{name: "expired token", token: expiredToken},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				response := serveNodeBootstrap(t, handler, validBootstrapRequest(t, testCase.token, "Rejected Node"))
				if response.Code != http.StatusUnauthorized {
					t.Fatalf("status = %d, want %d body = %q", response.Code, http.StatusUnauthorized, response.Body.String())
				}
				if !strings.Contains(response.Body.String(), "invalid or expired join token") {
					t.Fatalf("body = %q, want invalid token error", response.Body.String())
				}
			})
		}
	})

	t.Run("registers node returns secret and defaults unnamed nodes", func(t *testing.T) {
		t.Parallel()

		conn := newEncryptedRPCConnection(t, "node-bootstrap-success.sqlite")
		token, _, errGenerate := conn.GenerateNodeJoinToken("", time.Hour)
		if errGenerate != nil {
			t.Fatalf("GenerateNodeJoinToken() error = %v", errGenerate)
		}

		registry := noderegistry.New("controller-self", &nodeclient.FakeNodeClient{NodeID: "controller-self"})
		handler := NodeBootstrapHandler(conn, registry)
		payload := validBootstrapRequest(t, token, "   ")
		response := serveNodeBootstrap(t, handler, payload)

		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body = %q", response.Code, http.StatusOK, response.Body.String())
		}
		if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", contentType)
		}

		var bootstrapResponse NodeBootstrapResponse
		errDecode := json.Unmarshal(response.Body.Bytes(), &bootstrapResponse)
		if errDecode != nil {
			t.Fatalf("decode response error = %v body = %q", errDecode, response.Body.String())
		}
		if strings.TrimSpace(bootstrapResponse.NodeID) == "" {
			t.Fatal("NodeID is empty")
		}
		if len(bootstrapResponse.SharedSecret) != 64 {
			t.Fatalf("SharedSecret length = %d, want 64 hex characters", len(bootstrapResponse.SharedSecret))
		}

		node, errNode := conn.GetNodeByID(bootstrapResponse.NodeID)
		if errNode != nil {
			t.Fatalf("GetNodeByID() error = %v", errNode)
		}
		if node.Name != "Remote Node" {
			t.Fatalf("node name = %q, want %q", node.Name, "Remote Node")
		}
		if node.ListenURL != "https://node.example:9500" {
			t.Fatalf("listen URL = %q", node.ListenURL)
		}
		if node.CertFingerprint != payload.CertFingerprint {
			t.Fatalf("fingerprint = %q, want %q", node.CertFingerprint, payload.CertFingerprint)
		}
		if strings.TrimSpace(node.SharedSecretEncrypted) == "" {
			t.Fatal("shared secret was not stored encrypted")
		}
		if node.SharedSecretEncrypted == bootstrapResponse.SharedSecret {
			t.Fatal("stored shared secret is plaintext")
		}

		storedSecret, errDecrypt := conn.DecryptText(node.SharedSecretEncrypted)
		if errDecrypt != nil {
			t.Fatalf("DecryptText() error = %v", errDecrypt)
		}
		if storedSecret != bootstrapResponse.SharedSecret {
			t.Fatal("decrypted shared secret does not match bootstrap response")
		}

		registeredClient, errGet := registry.Get(bootstrapResponse.NodeID)
		if errGet != nil {
			t.Fatalf("registry.Get() error = %v", errGet)
		}
		if registeredClient.ID() != bootstrapResponse.NodeID {
			t.Fatalf("registered client ID = %q, want %q", registeredClient.ID(), bootstrapResponse.NodeID)
		}

		replay := serveNodeBootstrap(t, handler, validBootstrapRequest(t, token, "Replay Node"))
		if replay.Code != http.StatusUnauthorized {
			t.Fatalf("replay status = %d, want %d", replay.Code, http.StatusUnauthorized)
		}
	})
}

func TestValidateBootstrapRequest(t *testing.T) {
	t.Parallel()

	valid := validBootstrapRequest(t, "token", "node")
	errValidate := validateBootstrapRequest(&valid)
	if errValidate != nil {
		t.Fatalf("validateBootstrapRequest() error = %v", errValidate)
	}

	mismatched := valid
	mismatched.CertFingerprint = testNodeBootstrapFingerprint
	errMismatch := validateBootstrapRequest(&mismatched)
	if errMismatch == nil || !strings.Contains(errMismatch.Error(), "cert_fingerprint does not match cert_pem") {
		t.Fatalf("validateBootstrapRequest(mismatch) error = %v", errMismatch)
	}
}

func newEncryptedRPCConnection(t *testing.T, sqliteFileName string) *db.Connection {
	t.Helper()

	conn := newRPCFixtureConnection(t, sqliteFileName)
	conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	return conn
}

func validBootstrapRequest(t *testing.T, token string, nodeName string) NodeBootstrapRequest {
	t.Helper()

	certPEM, _, fingerprint, errGenerate := nodetls.GenerateSelfSigned(t.Context(), "bootstrap-test")
	if errGenerate != nil {
		t.Fatalf("GenerateSelfSigned() error = %v", errGenerate)
	}

	return NodeBootstrapRequest{
		JoinToken:       token,
		NodeName:        nodeName,
		ListenURL:       "https://node.example:9500",
		CertPEM:         string(certPEM),
		CertFingerprint: fingerprint,
	}
}

func serveNodeBootstrap(t *testing.T, handler http.Handler, payload NodeBootstrapRequest) *httptest.ResponseRecorder {
	t.Helper()

	body, errEncode := json.Marshal(payload)
	if errEncode != nil {
		t.Fatalf("marshal bootstrap request error = %v", errEncode)
	}

	request := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/api/node/bootstrap",
		bytes.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	_, _ = io.Copy(io.Discard, response.Result().Body)
	return response
}
