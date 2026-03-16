package rpc

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
)

type federationPeerIdentityContextKey string

const federationPeerIdentityKey federationPeerIdentityContextKey = "federation-peer-identity"

type FederationPeerIdentity struct {
	NodeID      string
	PeerNodeID  string
	Fingerprint string
}

func FederationPeerAuthMiddleware(dbInst *db.Connection) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.TLS == nil {
				http.Error(w, "federation mTLS is required", http.StatusUnauthorized)
				return
			}
			if len(r.TLS.PeerCertificates) == 0 {
				http.Error(w, "client certificate is required", http.StatusUnauthorized)
				return
			}

			peerCertificate := r.TLS.PeerCertificates[0]
			peerFingerprint := helpers.CertificateFingerprint(peerCertificate)

			trustedPeer, errGetTrustedPeer := dbInst.GetFederationTrustedPeerByFingerprint(peerFingerprint)
			if errGetTrustedPeer != nil {
				if errors.Is(errGetTrustedPeer, sql.ErrNoRows) {
					log.Warn().
						Str("fingerprint", peerFingerprint).
						Str("remote_addr", r.RemoteAddr).
						Msg("Rejected federation request from unknown peer certificate")
					http.Error(w, "unknown federation peer", http.StatusForbidden)
					return
				}

				log.Error().Err(errGetTrustedPeer).Msg("Failed to load federation trusted peer")
				http.Error(w, "failed to authorize federation peer", http.StatusInternalServerError)
				return
			}

			if !trustedPeer.Enabled || trustedPeer.Revoked {
				log.Warn().
					Str("node_id", trustedPeer.NodeID).
					Str("peer_node_id", trustedPeer.PeerNodeID).
					Bool("enabled", trustedPeer.Enabled).
					Bool("revoked", trustedPeer.Revoked).
					Msg("Rejected federation request from disabled or revoked peer")
				http.Error(w, "federation peer is disabled", http.StatusForbidden)
				return
			}

			peerCommonName := strings.TrimSpace(peerCertificate.Subject.CommonName)
			expectedPeerNodeID := strings.TrimSpace(trustedPeer.PeerNodeID)
			if expectedPeerNodeID != "" && peerCommonName != "" && !strings.EqualFold(expectedPeerNodeID, peerCommonName) {
				log.Warn().
					Str("node_id", trustedPeer.NodeID).
					Str("expected_peer_node_id", expectedPeerNodeID).
					Str("certificate_common_name", peerCommonName).
					Msg("Rejected federation request due to peer identity mismatch")
				http.Error(w, "federation peer identity mismatch", http.StatusForbidden)
				return
			}

			remoteNode, errGetRemoteNode := dbInst.GetRemoteNodeByID(trustedPeer.NodeID)
			if errGetRemoteNode != nil {
				if errors.Is(errGetRemoteNode, sql.ErrNoRows) {
					log.Warn().
						Str("node_id", trustedPeer.NodeID).
						Msg("Rejected federation request for missing remote node configuration")
					http.Error(w, "federation peer not configured", http.StatusForbidden)
					return
				}

				log.Error().
					Err(errGetRemoteNode).
					Str("node_id", trustedPeer.NodeID).
					Msg("Failed to load configured remote node during federation auth")
				http.Error(w, "failed to authorize federation peer", http.StatusInternalServerError)
				return
			}

			if !remoteNode.Enabled {
				log.Warn().
					Str("node_id", remoteNode.ID).
					Msg("Rejected federation request from disabled remote node")
				http.Error(w, "federation peer is disabled", http.StatusForbidden)
				return
			}

			identity := FederationPeerIdentity{
				NodeID:      trustedPeer.NodeID,
				PeerNodeID:  trustedPeer.PeerNodeID,
				Fingerprint: peerFingerprint,
			}
			ctxWithIdentity := context.WithValue(r.Context(), federationPeerIdentityKey, identity)
			next.ServeHTTP(w, r.WithContext(ctxWithIdentity))
		})
	}
}

func federationPeerIdentityFromContext(ctx context.Context) (FederationPeerIdentity, bool) {
	value := ctx.Value(federationPeerIdentityKey)
	if value == nil {
		return FederationPeerIdentity{}, false
	}
	identity, ok := value.(FederationPeerIdentity)
	if !ok {
		return FederationPeerIdentity{}, false
	}
	return identity, true
}
