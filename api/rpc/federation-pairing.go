package rpc

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
)

const (
	federationPairingPath = "/federation/complete-pairing"
)

type completePairingRequest struct {
	PairingToken    string `json:"pairing_token"`
	PeerFingerprint string `json:"peer_fingerprint"`
	PeerNodeID      string `json:"peer_node_id"`
	PeerBaseURL     string `json:"peer_base_url,omitempty"`
}

type completePairingResponse struct {
	NodeID         string `json:"node_id"`
	NodeName       string `json:"node_name"`
	Fingerprint    string `json:"fingerprint"`
	FederationPort int    `json:"federation_port"`
	Success        bool   `json:"success"`
	Error          string `json:"error,omitempty"`
}

// CompletePairingHandler handles the pairing completion request on the federation port.
// This endpoint does NOT require mTLS trust-store validation — the pairing token
// authenticates the request. It accepts any client certificate presented during TLS,
// validates the pairing token, and then establishes mutual trust.
func CompletePairingHandler(dbInst *db.Connection, federationMTLS *helpers.FederationMTLS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Require a client certificate even though we don't validate it against the trust store yet.
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			writeCompletePairingError(w, "client certificate is required", http.StatusUnauthorized)
			return
		}

		var req completePairingRequest
		errDecode := json.NewDecoder(r.Body).Decode(&req)
		if errDecode != nil {
			writeCompletePairingError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		pairingToken := strings.TrimSpace(req.PairingToken)
		peerFingerprint := strings.TrimSpace(req.PeerFingerprint)
		peerNodeID := strings.TrimSpace(req.PeerNodeID)
		peerBaseURL := strings.TrimSpace(req.PeerBaseURL)

		if pairingToken == "" {
			writeCompletePairingError(w, "pairing_token is required", http.StatusBadRequest)
			return
		}
		if peerFingerprint == "" {
			writeCompletePairingError(w, "peer_fingerprint is required", http.StatusBadRequest)
			return
		}

		// Also verify the presented client certificate fingerprint matches the claimed one.
		presentedCert := r.TLS.PeerCertificates[0]
		presentedFingerprint := helpers.CertificateFingerprint(presentedCert)
		if !fingerprintsEqual(presentedFingerprint, peerFingerprint) {
			log.Warn().
				Str("presented", presentedFingerprint).
				Str("claimed", peerFingerprint).
				Str("remote_addr", r.RemoteAddr).
				Msg("Pairing request fingerprint mismatch between TLS certificate and request body")
			writeCompletePairingError(w, "certificate fingerprint does not match claimed fingerprint", http.StatusForbidden)
			return
		}

		// Validate and consume the pairing token.
		_, errValidate := dbInst.ValidateAndConsumePairingTokenForTarget(pairingToken, peerBaseURL)
		if errValidate != nil {
			log.Warn().
				Err(errValidate).
				Str("remote_addr", r.RemoteAddr).
				Msg("Pairing token validation failed")
			writeCompletePairingError(w, "invalid or expired pairing token", http.StatusForbidden)
			return
		}

		// Get local identity to return to the peer.
		localIdentity, errIdentity := dbInst.GetFederationLocalIdentity()
		if errIdentity != nil {
			log.Error().Err(errIdentity).Msg("Failed to get local federation identity during pairing")
			writeCompletePairingError(w, "internal error", http.StatusInternalServerError)
			return
		}

		localFingerprint := localIdentity.CertFingerprint

		localSettings, errSettings := dbInst.GetLocalSettings()
		if errSettings != nil {
			log.Error().Err(errSettings).Msg("Failed to get local settings during pairing")
			writeCompletePairingError(w, "internal error", http.StatusInternalServerError)
			return
		}

		localNodeName := "Xylona Node"
		localNode, errLocalNode := dbInst.GetNodeByID(localSettings.NodeID)
		if errLocalNode == nil && localNode != nil {
			localNodeName = localNode.Name
		}

		log.Info().
			Str("peer_fingerprint", peerFingerprint).
			Str("peer_node_id", peerNodeID).
			Str("remote_addr", r.RemoteAddr).
			Msg("Federation pairing completed successfully — peer certificate fingerprint accepted via pairing token")

		resp := completePairingResponse{
			NodeID:         localSettings.NodeID,
			NodeName:       localNodeName,
			Fingerprint:    localFingerprint,
			FederationPort: federationMTLS.FederationPort(),
			Success:        true,
		}

		w.Header().Set("Content-Type", "application/json")
		errEncode := json.NewEncoder(w).Encode(resp)
		if errEncode != nil {
			log.Error().Err(errEncode).Msg("Failed to encode pairing response")
		}
	}
}

func writeCompletePairingError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	resp := completePairingResponse{
		Success: false,
		Error:   message,
	}
	errEncode := json.NewEncoder(w).Encode(resp)
	if errEncode != nil {
		log.Error().Err(errEncode).Msg("Failed to encode pairing error response")
	}
}

func fingerprintsEqual(a string, b string) bool {
	aBytes := sha256PadForCompare(strings.ToLower(strings.TrimSpace(a)))
	bBytes := sha256PadForCompare(strings.ToLower(strings.TrimSpace(b)))
	return aBytes == bBytes
}

// sha256PadForCompare hashes the input to a fixed-size value for constant-time-safe
// comparison without importing crypto/subtle for string equality.
func sha256PadForCompare(s string) [sha256.Size]byte {
	return sha256.Sum256([]byte(s))
}

// formatPairingToken formats a hex token as XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX for readability.
func formatPairingToken(token string) string {
	token = strings.ReplaceAll(token, "-", "")
	var parts []string
	for i := 0; i < len(token); i += 8 {
		end := i + 8
		if end > len(token) {
			end = len(token)
		}
		parts = append(parts, token[i:end])
	}
	return strings.Join(parts, "-")
}

// normalizePairingToken strips dashes from a formatted pairing token for validation.
func normalizePairingToken(token string) string {
	return strings.ReplaceAll(strings.TrimSpace(token), "-", "")
}

// ProbePeerAndCompletePairing probes the remote federation port, presents a pairing token,
// and returns the remote node's identity and fingerprint. This is called from addRemoteNode.
func ProbePeerAndCompletePairing(
	federationMTLS *helpers.FederationMTLS,
	remoteBaseURL string,
	pairingToken string,
	localBaseURL string,
	remoteFederationPort int,
) (*completePairingResponse, string, error) {
	normalizedLocalBaseURL := ""
	if strings.TrimSpace(localBaseURL) != "" {
		var errNormalizeLocalBaseURL error
		normalizedLocalBaseURL, errNormalizeLocalBaseURL = normalizeBaseURL(localBaseURL)
		if errNormalizeLocalBaseURL != nil {
			return nil, "", errNormalizeLocalBaseURL
		}
	}

	// Get the federation URL for the remote node.
	federationBaseURL, errFederationURL := federationMTLS.FederationBaseURLWithPort(remoteBaseURL, remoteFederationPort)
	if errFederationURL != nil {
		return nil, "", errFederationURL
	}

	// Probe the remote server's certificate fingerprint.
	remoteFingerprint, errProbe := federationMTLS.ProbeServerFingerprintWithPort(remoteBaseURL, remoteFederationPort, federationRequestTimeout)
	if errProbe != nil {
		return nil, "", errProbe
	}

	// Get our own fingerprint to send in the request.
	localFingerprint, errLocal := federationMTLS.LocalFingerprint()
	if errLocal != nil {
		return nil, "", errLocal
	}

	// Create an HTTP client that trusts the probed fingerprint for this one request.
	httpClient, _, errClient := federationMTLS.NewNodeHTTPClientWithPort(
		federationRequestTimeout,
		remoteBaseURL,
		remoteFederationPort,
		remoteFingerprint,
		"", // We don't know their node ID yet.
	)
	if errClient != nil {
		return nil, "", errClient
	}

	reqBody := completePairingRequest{
		PairingToken:    normalizePairingToken(pairingToken),
		PeerFingerprint: localFingerprint,
		PeerNodeID:      "",
		PeerBaseURL:     normalizedLocalBaseURL,
	}

	bodyBytes, errMarshal := json.Marshal(reqBody)
	if errMarshal != nil {
		return nil, "", errMarshal
	}

	pairingURL := strings.TrimSuffix(federationBaseURL, "/") + federationPairingPath
	resp, errDo := httpClient.Post(pairingURL, "application/json", strings.NewReader(string(bodyBytes)))
	if errDo != nil {
		return nil, "", errDo
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var pairingResp completePairingResponse
	errDecode := json.NewDecoder(resp.Body).Decode(&pairingResp)
	if errDecode != nil {
		return nil, "", errDecode
	}

	if !pairingResp.Success {
		return nil, "", &pairingError{message: pairingResp.Error}
	}

	return &pairingResp, remoteFingerprint, nil
}

type pairingError struct {
	message string
}

func (e *pairingError) Error() string {
	return e.message
}
