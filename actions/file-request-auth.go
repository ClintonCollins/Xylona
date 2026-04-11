package actions

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	apifederation "github.com/ClintonCollins/Xylona/api/federation"
	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/helpers"
	fedheaders "github.com/ClintonCollins/Xylona/helpers/federation"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type legacyFileProxyStatusError interface {
	error
	StatusCode() int
}

type legacyFileProxyError struct {
	statusCode int
	err        error
}

func (e *legacyFileProxyError) Error() string {
	return e.err.Error()
}

func (e *legacyFileProxyError) Unwrap() error {
	return e.err
}

func (e *legacyFileProxyError) StatusCode() int {
	return e.statusCode
}

func validateFederatedActingIdentity(header http.Header, peerIdentity apifederation.PeerIdentity) (string, error) {
	actingUserID, originNodeID := fedheaders.GetActingIdentity(header)
	actingUserID = strings.TrimSpace(actingUserID)
	originNodeID = strings.TrimSpace(originNodeID)

	if strings.TrimSpace(peerIdentity.NodeID) == "" {
		return "", errors.New("authenticated federation peer identity is required")
	}
	if actingUserID == "" {
		return "", errors.New("acting user identity is required for federated actions")
	}
	if originNodeID == "" {
		return "", errors.New("acting origin node is required for federated actions")
	}
	if originNodeID != peerIdentity.NodeID && originNodeID != peerIdentity.PeerNodeID {
		return "", errors.New("acting origin node is invalid")
	}

	return actingUserID, nil
}

func (inst *Instance) authorizeLegacyFileRequest(r *http.Request, gameServer *models.GameServer, permissionID string) (int, error) {
	peerIdentity, okPeerIdentity := apifederation.PeerIdentityFromContext(r.Context())
	if okPeerIdentity {
		actingUserID, errValidate := validateFederatedActingIdentity(r.Header, peerIdentity)
		if errValidate != nil {
			log.Warn().
				Str("server_id", gameServer.ID).
				Str("permission_id", permissionID).
				Err(errValidate).
				Msg("federation request acting identity is invalid")
			return http.StatusForbidden, errValidate
		}
		if fedheaders.ActingIsSuperUser(r.Header) {
			return http.StatusOK, nil
		}

		allowed, errPermission := inst.db.FederatedUserHasPermissionOnServer(peerIdentity.NodeID, actingUserID, gameServer.ID, permissionID)
		if errPermission != nil {
			log.Error().
				Err(errPermission).
				Str("server_id", gameServer.ID).
				Str("permission_id", permissionID).
				Str("acting_user_id", actingUserID).
				Str("authenticated_node_id", peerIdentity.NodeID).
				Msg("failed to verify federated permission")
			return http.StatusInternalServerError, errors.New("failed to verify federated permission")
		}
		if !allowed {
			return http.StatusForbidden, errors.New("federated user does not have permission")
		}
		return http.StatusOK, nil
	}

	user, okUser := gatekeeper.UserFromContext(r.Context())
	if !okUser || user == nil {
		return http.StatusUnauthorized, errors.New("unauthenticated")
	}

	allowed, errPermission := helpers.HasPermission(inst.db, user, gameServer.ID, gameServer.UserID, permissionID)
	if errPermission != nil {
		log.Error().
			Err(errPermission).
			Str("server_id", gameServer.ID).
			Str("permission_id", permissionID).
			Str("user_id", user.ID).
			Msg("failed to check local file permission")
		return http.StatusInternalServerError, errors.New("failed to check permissions")
	}
	if !allowed {
		return http.StatusForbidden, errors.New("insufficient permissions")
	}
	return http.StatusOK, nil
}

func (inst *Instance) applyProxyActingIdentityHeaders(r *http.Request, header http.Header) error {
	peerIdentity, okPeerIdentity := apifederation.PeerIdentityFromContext(r.Context())
	if okPeerIdentity {
		_, errValidate := validateFederatedActingIdentity(r.Header, peerIdentity)
		if errValidate != nil {
			return &legacyFileProxyError{
				statusCode: http.StatusForbidden,
				err:        errValidate,
			}
		}
		if !fedheaders.CopyActingIdentityHeaders(header, r.Header) {
			return &legacyFileProxyError{
				statusCode: http.StatusForbidden,
				err:        errors.New("federated acting identity is required"),
			}
		}
		return nil
	}

	user, okUser := gatekeeper.UserFromContext(r.Context())
	if !okUser || user == nil {
		return &legacyFileProxyError{
			statusCode: http.StatusUnauthorized,
			err:        errors.New("unauthenticated"),
		}
	}

	errApply := apifederation.ApplyActingIdentityHeadersForUser(inst.db, header, user)
	if errApply != nil {
		return fmt.Errorf("apply acting identity headers: %w", errApply)
	}
	return nil
}

func (inst *Instance) serveLegacyFileRequest(
	w http.ResponseWriter,
	r *http.Request,
	gameServerID string,
	permissionID string,
	failureMessage string,
	localHandler func(*models.GameServer) error,
	remoteHandler func(fileRequestTarget) error,
) bool {
	target, errResolve := inst.resolveFileRequestTarget(gameServerID)
	if errResolve != nil {
		log.Error().Err(errResolve).Msg("Failed to get game server")
		writeGameServerLookupError(w, errResolve)
		return false
	}

	return inst.handleLegacyFileTransferTarget(w, r, target, permissionID, failureMessage, localHandler, remoteHandler)
}

func (inst *Instance) handleLegacyFileTransferTarget(
	w http.ResponseWriter,
	r *http.Request,
	target fileRequestTarget,
	permissionID string,
	failureMessage string,
	localHandler func(*models.GameServer) error,
	remoteHandler func(fileRequestTarget) error,
) bool {
	if target.isLocal() {
		statusCode, errAuth := inst.authorizeLegacyFileRequest(r, target.gameServer, permissionID)
		if errAuth != nil {
			http.Error(w, errAuth.Error(), statusCode)
			return false
		}

		errLocal := localHandler(target.gameServer)
		if errLocal != nil {
			if isRequestBodyTooLarge(errLocal) {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return false
			}
			log.Error().Err(errLocal).Msg(failureMessage)
			http.Error(w, failureMessage, http.StatusInternalServerError)
			return false
		}
		return true
	}

	errRemote := remoteHandler(target)
	if errRemote != nil {
		statusCode := http.StatusInternalServerError
		var statusErr legacyFileProxyStatusError
		if errors.As(errRemote, &statusErr) {
			statusCode = statusErr.StatusCode()
		}
		log.Error().Err(errRemote).Msg(failureMessage)
		if statusCode == http.StatusInternalServerError {
			http.Error(w, failureMessage, statusCode)
			return false
		}
		http.Error(w, errRemote.Error(), statusCode)
		return false
	}
	return true
}
