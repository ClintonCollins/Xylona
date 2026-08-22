package rpc

import (
	"errors"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

const publicGameServerMapPath = "/maps/{identifier}"

var legacyPublicGameServerMapPaths = []string{
	"/shared/palworld-map",
	"/shared/7-days-to-die-map",
	"/shared/minecraft-map",
}

// GameServerMapHTTPHandler serves the public map SPA shell.
type GameServerMapHTTPHandler struct {
	service  *XylonaService
	frontend fs.FS
}

// NewGameServerMapHTTPHandler prepares the public map handler.
func NewGameServerMapHTTPHandler(frontend fs.FS, service *XylonaService) *GameServerMapHTTPHandler {
	return &GameServerMapHTTPHandler{service: service, frontend: frontend}
}

// RegisterGameServerMapRoutes registers public map routes before the SPA fallback.
func RegisterGameServerMapRoutes(router chi.Router, handler *GameServerMapHTTPHandler) {
	router.Get("/maps", handler.LegacyMap)
	router.Get("/maps/", handler.LegacyMap)
	router.Get(publicGameServerMapPath, handler.Map)
	router.Get(publicGameServerMapPath+"/*", handler.LegacyMap)
	for _, path := range legacyPublicGameServerMapPaths {
		router.Get(path, handler.LegacyMap)
	}
}

// Map serves the SPA shell only for a currently enabled supported map share.
func (h *GameServerMapHTTPHandler) Map(w http.ResponseWriter, r *http.Request) {
	index, errRead := fs.ReadFile(h.frontend, "index.html")
	if errRead != nil {
		writeGameServerMapError(w)
		return
	}

	identifier := chi.URLParam(r, "identifier")
	_, errResolve := h.service.resolvePublicGameServerMapKind(identifier)
	if errors.Is(errResolve, errPublicGameServerMapUnavailable) {
		writeUnavailableGameServerMap(w, index)
		return
	}
	if errResolve != nil {
		writeGameServerMapError(w)
		return
	}

	writeGameServerMapResponse(w, http.StatusOK, index)
}

// LegacyMap makes retired token routes fail identically to unavailable slugs.
func (h *GameServerMapHTTPHandler) LegacyMap(w http.ResponseWriter, _ *http.Request) {
	index, errRead := fs.ReadFile(h.frontend, "index.html")
	if errRead != nil {
		writeGameServerMapError(w)
		return
	}
	writeUnavailableGameServerMap(w, index)
}

func writeUnavailableGameServerMap(w http.ResponseWriter, index []byte) {
	writeGameServerMapResponse(w, http.StatusNotFound, index)
}

func writeGameServerMapResponse(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.WriteHeader(status)
	_, errWrite := w.Write(body)
	if errWrite != nil {
		log.Debug().Err(errWrite).Int("status", status).Msg("Public game server map response closed early")
		return
	}
}

func writeGameServerMapError(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	http.Error(w, "map unavailable", http.StatusInternalServerError)
}
