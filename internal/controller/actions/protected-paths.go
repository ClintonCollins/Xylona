package actions

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/startargs"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// ErrProtectedPath is returned when a write targets a protected server file.
var ErrProtectedPath = errors.New("path is protected")

func validateLocalServerPath(gameServer *models.GameServer, relativePath string) (string, error) {
	trimmedPath := strings.TrimPrefix(relativePath, "/")
	if trimmedPath != "" && !filepath.IsLocal(trimmedPath) {
		log.Error().Str("Game Server ID", gameServer.ID).Str("path", relativePath).Msg("Invalid path")
		return "", ErrInvalidPath
	}

	return trimmedPath, nil
}

// baseCommandForProtectedPath picks the base command used by the protected-
// path check. Callers reach this via validateWritableServerPath from the
// controller's file-operation RPCs, where we don't have a cheap way to
// dispatch to the target node for its OS. We use the controller's own
// OperatingSystem here as a best-effort hint; the real protection check
// still runs on the node (which knows its own OS) so the executable match
// remains accurate end-to-end.
func baseCommandForProtectedPath(gameServer *models.GameServer) string {
	if gameServer == nil {
		return ""
	}
	override := strings.TrimSpace(gameServer.BaseCommandOverride)
	if override != "" {
		return override
	}
	if gameServer.R.Game == nil {
		return ""
	}

	return gameBaseCommand(gameServer.R.Game, OperatingSystem)
}

func validateWritableServerPath(gameServer *models.GameServer, relativePath string) (string, error) {
	trimmedPath, errPath := validateLocalServerPath(gameServer, relativePath)
	if errPath != nil {
		return "", errPath
	}

	if startargs.IsProtectedServerPath(trimmedPath, baseCommandForProtectedPath(gameServer), gameServer.ServerExecutable.GetOr("")) {
		log.Warn().Str("Game Server ID", gameServer.ID).Str("path", trimmedPath).Msg("Blocked mutation of protected server path")
		return "", ErrProtectedPath
	}

	return trimmedPath, nil
}
