package rpc

import (
	"errors"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
)

type alertResourceDeleteConfig struct {
	logIDField       string
	deleteLogMessage string
	internalMessage  string
	deleteFn         func(id string, userID string) error
}

func (xs *XylonaService) deleteOwnedAlertResource(header http.Header, rawID string, config alertResourceDeleteConfig) error {
	user, errUser := xs.getUserFromHeader(header)
	if errUser != nil {
		return unauthenticated()
	}

	allowed, errPerm := xs.hasGlobalPermission(user)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return permissionDenied("insufficient permissions")
	}

	id := strings.TrimSpace(rawID)
	if id == "" {
		return invalidArg("id is required")
	}

	errDelete := config.deleteFn(id, user.ID)
	if errDelete != nil {
		log.Error().Err(errDelete).Str(config.logIDField, id).Msg(config.deleteLogMessage)
		return connect.NewError(connect.CodeInternal, errors.New(config.internalMessage))
	}

	return nil
}
