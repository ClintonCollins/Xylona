package actions

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/ClintonCollins/Xylona/startargs"
)

var ErrProtectedPath = errors.New("path is protected")

func validateLocalServerPath(gameServer *models.GameServer, relativePath string) (string, error) {
	trimmedPath := strings.TrimPrefix(relativePath, "/")
	if trimmedPath != "" && !filepath.IsLocal(trimmedPath) {
		log.Error().Str("Game Server ID", gameServer.ID).Str("path", relativePath).Msg("Invalid path")
		return "", ErrInvalidPath
	}

	return trimmedPath, nil
}

func baseCommandForProtectedPath(gameServer *models.GameServer) string {
	if gameServer == nil || gameServer.R.Game == nil {
		return ""
	}

	return gameBaseCommand(gameServer.R.Game)
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
