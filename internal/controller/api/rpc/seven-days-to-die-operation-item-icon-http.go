package rpc

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

// SevenDaysToDieOperationItemIconPathPrefix is the authenticated item-icon route.
const SevenDaysToDieOperationItemIconPathPrefix = "/api/game-servers/operation-item-icons"

// SevenDaysToDieOperationItemIcon streams one server-owned item icon.
func (xs *XylonaService) SevenDaysToDieOperationItemIcon(response http.ResponseWriter, request *http.Request) {
	gameServerID := strings.TrimSpace(chi.URLParam(request, "gameServerId"))
	iconFile := strings.TrimSpace(chi.URLParam(request, "icon"))
	if gameServerID == "" || len(iconFile) > 260 || strings.ContainsAny(iconFile, "/\\\x00") ||
		!strings.EqualFold(filepath.Ext(iconFile), ".png") {
		http.Error(response, "Invalid item icon path", http.StatusBadRequest)
		return
	}
	user, errUser := xs.getUserFromHeader(request.Header)
	if errUser != nil {
		http.Error(response, "Unauthorized", http.StatusUnauthorized)
		return
	}
	gameServer, errServer := xs.getGameServerFromID(gameServerID)
	if errServer != nil {
		http.Error(response, "Item icon not found", http.StatusNotFound)
		return
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, "game_server.console")
	if errPermission != nil {
		http.Error(response, "Forbidden", http.StatusForbidden)
		return
	}
	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		http.Error(response, "Server node unavailable", http.StatusServiceUnavailable)
		return
	}
	reader, errStream := client.StreamFile(
		request.Context(),
		gameServer.Directory,
		filepath.Join("Data", "ItemIcons", iconFile),
	)
	if errStream != nil {
		http.Error(response, "Item icon not found", http.StatusNotFound)
		return
	}

	response.Header().Set("Cache-Control", "private, max-age=86400")
	response.Header().Set("Content-Type", "image/png")
	_, errCopy := io.Copy(response, reader)
	errClose := reader.Close()
	if errCopy != nil {
		log.Debug().Err(errCopy).Str("server_id", gameServer.ID).Str("icon", iconFile).Msg("failed to stream operation item icon")
	}
	if errClose != nil {
		log.Debug().Err(errClose).Str("server_id", gameServer.ID).Str("icon", iconFile).Msg("failed to close operation item icon stream")
	}
}
