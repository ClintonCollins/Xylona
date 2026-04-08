package rpc

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/helpers/federation"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type federationPeerIdentityContextKey string

const federationPeerIdentityKey federationPeerIdentityContextKey = "federation-peer-identity"

// FederationPeerIdentity describes the authenticated remote peer on a federation request.
type FederationPeerIdentity struct {
	NodeID      string
	PeerNodeID  string
	Fingerprint string
}

// FederationPeerAuthMiddleware authenticates inbound federation requests using mTLS state.
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
			peerFingerprint := federation.CertificateFingerprint(peerCertificate)

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

func (fs FederationService) authenticateRequest(ctx context.Context) (FederationPeerIdentity, error) {
	identity, okIdentity := federationPeerIdentityFromContext(ctx)
	if !okIdentity {
		return FederationPeerIdentity{}, errors.New("federation peer identity is required")
	}
	if strings.TrimSpace(identity.NodeID) == "" {
		return FederationPeerIdentity{}, errors.New("federation peer node configuration is required")
	}
	return identity, nil
}

func (fs FederationService) resolveFederatedServerStatus(gameServer *models.GameServer) xylona.Status {
	status := helpers.GameServerModelStatusToProtoStatus(gameServer.Status)
	if fs.supervisorInst == nil {
		if status == xylona.Status_ONLINE {
			return xylona.Status_OFFLINE
		}
		return status
	}

	gameServerCmd, errGetCommand := fs.supervisorInst.GetCommandByID(gameServer.ID)
	if errGetCommand == nil {
		return gameServerCmd.Status()
	}

	// Prevent stale persisted ONLINE from surviving process restarts.
	if status == xylona.Status_ONLINE {
		return xylona.Status_OFFLINE
	}

	return status
}

func (fs FederationService) authorizeFederatedPermission(
	ctx context.Context,
	header http.Header,
	actingUserID string,
	originNodeID string,
	serverID string,
	permissionID string,
) error {
	peerIdentity, okIdentity := federationPeerIdentityFromContext(ctx)
	if !okIdentity || strings.TrimSpace(peerIdentity.NodeID) == "" {
		log.Warn().
			Str("server_id", serverID).
			Str("permission_id", permissionID).
			Msg("federation request missing authenticated peer identity")
		return permissionDenied("authenticated federation peer identity is required")
	}

	if strings.TrimSpace(actingUserID) == "" || strings.TrimSpace(originNodeID) == "" {
		headerUserID, headerOriginNodeID := federation.GetActingIdentity(header)
		actingUserID = strings.TrimSpace(headerUserID)
		originNodeID = strings.TrimSpace(headerOriginNodeID)
	}

	if actingUserID == "" {
		log.Warn().
			Str("server_id", serverID).
			Str("permission_id", permissionID).
			Msg("federation request missing acting user identity, denying by default")
		return permissionDenied("acting user identity is required for federated actions")
	}

	if originNodeID != "" && originNodeID != peerIdentity.NodeID && originNodeID != peerIdentity.PeerNodeID {
		log.Warn().
			Str("server_id", serverID).
			Str("permission_id", permissionID).
			Str("origin_node_id", originNodeID).
			Str("authenticated_node_id", peerIdentity.NodeID).
			Str("authenticated_peer_node_id", peerIdentity.PeerNodeID).
			Msg("federation request acting origin does not match authenticated peer")
		return permissionDenied("acting origin node is invalid")
	}

	if federation.ActingIsSuperUser(header) {
		if originNodeID == "" {
			log.Warn().
				Str("server_id", serverID).
				Str("permission_id", permissionID).
				Str("acting_user_id", actingUserID).
				Msg("federation super-user request missing origin node")
			return permissionDenied("acting origin node is required for super-user federated actions")
		}
		return nil
	}

	allowed, errPermission := fs.db.FederatedUserHasPermissionOnServer(peerIdentity.NodeID, actingUserID, serverID, permissionID)
	if errPermission != nil {
		log.Error().
			Err(errPermission).
			Str("server_id", serverID).
			Str("origin_node_id", originNodeID).
			Str("authenticated_node_id", peerIdentity.NodeID).
			Str("acting_user_id", actingUserID).
			Str("permission_id", permissionID).
			Msg("failed to verify federated permission")
		return connect.NewError(connect.CodeInternal, errors.New("failed to verify federated permission"))
	}
	if !allowed {
		return permissionDenied("federated user does not have permission")
	}

	return nil
}
