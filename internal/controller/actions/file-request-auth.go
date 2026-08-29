package actions

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/controller/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// authorizeFileRequest authorizes a legacy HTTP file-transfer request against
// the controller-side permission model. Hub-spoke file transfers only travel
// through the controller's embedded node, so the authorization path is just
// the local user+permission check.
func (inst *Instance) authorizeFileRequest(r *http.Request, gameServer *models.GameServer, permissionID string) (int, error) {
	user, okUser := gatekeeper.UserFromContext(r.Context())
	if !okUser || user == nil {
		return http.StatusUnauthorized, errors.New("unauthenticated")
	}

	allowed, errPermission := db.HasPermission(inst.db, user, gameServer.ID, gameServer.UserID, permissionID)
	if errPermission != nil {
		log.Error().
			Err(errPermission).
			Str("server_id", gameServer.ID).
			Str("permission_id", permissionID).
			Str("user_id", user.ID).
			Msg("failed to check file permission")
		return http.StatusInternalServerError, errors.New("failed to check permissions")
	}
	if !allowed {
		return http.StatusForbidden, errors.New("insufficient permissions")
	}
	return http.StatusOK, nil
}

// serveLocalFileRequest performs a controller-local file-transfer operation
// after verifying authorization for the given game server. Hub-spoke removed
// the remote-node HTTP proxy path; large file transfers to remote nodes will
// come back as a Phase 2 HTTP streaming proxy.
func (inst *Instance) serveLocalFileRequest(
	w http.ResponseWriter,
	r *http.Request,
	gameServerID string,
	permissionID string,
	failureMessage string,
	localHandler func(*models.GameServer) error,
) bool {
	gameServer, errLookup := inst.db.GetGameServerByID(gameServerID)
	if errLookup != nil {
		if errors.Is(errLookup, sql.ErrNoRows) {
			http.Error(w, "game server not found", http.StatusNotFound)
			return false
		}
		log.Error().Err(errLookup).Msg("Failed to get game server")
		http.Error(w, "failed to look up game server", http.StatusInternalServerError)
		return false
	}

	statusCode, errAuth := inst.authorizeFileRequest(r, gameServer, permissionID)
	if errAuth != nil {
		http.Error(w, errAuth.Error(), statusCode)
		return false
	}

	errLocal := localHandler(gameServer)
	if errLocal != nil {
		if isRequestBodyTooLarge(errLocal) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return false
		}
		if errors.Is(errLocal, ErrProtectedPath) {
			http.Error(w, ErrProtectedPath.Error(), http.StatusForbidden)
			return false
		}
		log.Error().Err(errLocal).Msg(failureMessage)
		http.Error(w, failureMessage, http.StatusInternalServerError)
		return false
	}
	return true
}
