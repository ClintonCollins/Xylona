package rpc

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
)

// nodeBootstrapMaxBodyBytes bounds the request body so a misbehaving node
// can't flood the controller with giant certificate payloads.
const nodeBootstrapMaxBodyBytes = 1 << 20 // 1 MiB is ample for an ECDSA cert.

// NodeBootstrapRequest is the JSON payload a xylona-node binary sends to the
// controller's bootstrap endpoint. It contains everything the controller
// needs to register the node and trust its future gRPC calls.
type NodeBootstrapRequest struct {
	JoinToken       string `json:"join_token"`
	NodeName        string `json:"node_name"`
	ListenURL       string `json:"listen_url"`
	CertPEM         string `json:"cert_pem"`
	CertFingerprint string `json:"cert_fingerprint"`
}

// NodeBootstrapResponse is the JSON payload returned to the node after a
// successful registration. The shared secret is delivered exactly once — the
// controller stores only an encrypted copy.
type NodeBootstrapResponse struct {
	NodeID       string `json:"node_id"`
	SharedSecret string `json:"shared_secret"`
}

// NodeBootstrapHandler handles POST /api/node/bootstrap. Authentication is by
// the one-shot join token inside the JSON body; the handler itself does not
// require session cookies. If registry is non-nil, a gRPC client for the new
// node is registered immediately after the DB row lands so the controller can
// start routing RPCs through the node without a restart.
func NodeBootstrapHandler(dbInst *db.Connection, registry *noderegistry.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, errRead := io.ReadAll(io.LimitReader(r.Body, nodeBootstrapMaxBodyBytes))
		if errRead != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}

		req := &NodeBootstrapRequest{}
		errDecode := json.Unmarshal(body, req)
		if errDecode != nil {
			http.Error(w, "failed to parse bootstrap request", http.StatusBadRequest)
			return
		}

		validationErr := validateBootstrapRequest(req)
		if validationErr != nil {
			http.Error(w, validationErr.Error(), http.StatusBadRequest)
			return
		}

		nodeID := uuid.NewString()
		sharedSecret, errSecret := generateSharedSecret()
		if errSecret != nil {
			log.Error().Err(errSecret).Msg("failed to generate shared secret for bootstrapping node")
			http.Error(w, "failed to generate shared secret", http.StatusInternalServerError)
			return
		}

		encryptedSecret, errEncrypt := dbInst.EncryptText(sharedSecret)
		if errEncrypt != nil {
			log.Error().Err(errEncrypt).Msg("failed to encrypt node shared secret")
			http.Error(w, "failed to encrypt shared secret", http.StatusInternalServerError)
			return
		}

		_, errConsume := dbInst.ConsumeNodeJoinToken(req.JoinToken, nodeID)
		if errConsume != nil {
			if errors.Is(errConsume, db.ErrJoinTokenInvalid) {
				http.Error(w, "invalid or expired join token", http.StatusUnauthorized)
				return
			}
			log.Error().Err(errConsume).Msg("failed to consume node join token")
			http.Error(w, "failed to consume join token", http.StatusInternalServerError)
			return
		}

		displayName := strings.TrimSpace(req.NodeName)
		if displayName == "" {
			displayName = "Remote Node"
		}

		_, errRegister := dbInst.RegisterRemoteNode(
			nodeID,
			displayName,
			strings.TrimSpace(req.ListenURL),
			strings.TrimSpace(req.CertFingerprint),
			encryptedSecret,
		)
		if errRegister != nil {
			log.Error().Err(errRegister).Msg("failed to persist bootstrapped node")
			http.Error(w, "failed to persist node", http.StatusInternalServerError)
			return
		}

		// Register a live NodeClient so subsequent RPCs can route through the
		// newly bootstrapped node without waiting for the controller to
		// restart. Failures are logged but not propagated: the DB row is
		// committed, the next controller start-up will pick it up via
		// dialRegisteredRemoteNodes, and the node's own bootstrap has
		// already succeeded.
		if registry != nil {
			client, errClient := nodeclient.NewGRPCClient(
				nodeID,
				strings.TrimSpace(req.ListenURL),
				strings.TrimSpace(req.CertFingerprint),
				sharedSecret,
			)
			if errClient != nil {
				log.Warn().Err(errClient).Str("node_id", nodeID).Msg("failed to build gRPC client for bootstrapped node")
			} else {
				registry.Register(client)
				log.Info().Str("node_id", nodeID).Str("listen_url", req.ListenURL).Msg("Registered bootstrapped node in noderegistry")
			}
		}

		resp := &NodeBootstrapResponse{
			NodeID:       nodeID,
			SharedSecret: sharedSecret,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		errEncode := json.NewEncoder(w).Encode(resp)
		if errEncode != nil {
			log.Error().Err(errEncode).Msg("failed to encode bootstrap response")
		}
	}
}

func validateBootstrapRequest(req *NodeBootstrapRequest) error {
	if strings.TrimSpace(req.JoinToken) == "" {
		return errors.New("join_token is required")
	}
	if strings.TrimSpace(req.ListenURL) == "" {
		return errors.New("listen_url is required")
	}
	if strings.TrimSpace(req.CertPEM) == "" {
		return errors.New("cert_pem is required")
	}
	if strings.TrimSpace(req.CertFingerprint) == "" {
		return errors.New("cert_fingerprint is required")
	}
	return nil
}

func generateSharedSecret() (string, error) {
	// 32 bytes of randomness → 64 hex chars. Plenty of entropy for a bearer
	// token that's also stored encrypted at rest.
	secretBytes := make([]byte, 32)
	_, errRand := rand.Read(secretBytes)
	if errRand != nil {
		return "", fmt.Errorf("generate shared secret bytes: %w", errRand)
	}
	return hex.EncodeToString(secretBytes), nil
}
