package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	sevenDaysToDieGameID         = "7_days_to_die"
	sevenDaysToDieMapShareHeader = "X-Xylona-Map-Share"
	// SevenDaysToDieMapTilePathPrefix is the controller-local authenticated tile route.
	SevenDaysToDieMapTilePathPrefix = "/seven-days-to-die-map-tiles"
	sevenDaysToDieMapNoteLimit      = 100
)

type sevenDaysToDieMapNoteConfig struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Note string  `json:"note,omitempty"`
	Icon string  `json:"icon,omitempty"`
	X    float64 `json:"x"`
	Z    float64 `json:"z"`
}

type storedSevenDaysToDieMapPlayer struct {
	ID         string                       `json:"id"`
	Name       string                       `json:"name"`
	Online     bool                         `json:"online"`
	Position   node.SevenDaysToDieMapVector `json:"position"`
	LastSeenAt time.Time                    `json:"last_seen_at"`
}

type storedSevenDaysToDieMapSnapshot struct {
	Enabled    bool                            `json:"enabled"`
	TileSize   int32                           `json:"tile_size"`
	MaxZoom    int32                           `json:"max_zoom"`
	MapSize    node.SevenDaysToDieMapVector    `json:"map_size"`
	SourceTime string                          `json:"source_time"`
	Players    []storedSevenDaysToDieMapPlayer `json:"players"`
}

// GetSevenDaysToDieMap returns the current or last-known map view for a 7 Days to Die server.
func (xs *XylonaService) GetSevenDaysToDieMap(
	ctx context.Context,
	request *connect.Request[xylona.GetSevenDaysToDieMapRequest],
) (*connect.Response[xylona.GetSevenDaysToDieMapResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errServer := xs.sevenDaysToDieMapServer(request.Msg.GetGameServerId())
	if errServer != nil {
		return nil, errServer
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerView)
	if errPermission != nil {
		return nil, errPermission
	}
	view, errView := xs.buildSevenDaysToDieMapView(ctx, gameServer)
	if errView != nil {
		return nil, errView
	}
	return connect.NewResponse(&xylona.GetSevenDaysToDieMapResponse{Map: view}), nil
}

// UpdateSevenDaysToDieMapNotes replaces the locally managed markers for a 7 Days to Die map.
func (xs *XylonaService) UpdateSevenDaysToDieMapNotes(
	_ context.Context,
	request *connect.Request[xylona.UpdateSevenDaysToDieMapNotesRequest],
) (*connect.Response[xylona.UpdateSevenDaysToDieMapNotesResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errServer := xs.sevenDaysToDieMapServer(request.Msg.GetGameServerId())
	if errServer != nil {
		return nil, errServer
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerSettings)
	if errPermission != nil {
		return nil, errPermission
	}
	notes, errNotes := validateSevenDaysToDieMapNotes(request.Msg.GetMarkers())
	if errNotes != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errNotes)
	}
	encoded, errEncode := json.Marshal(notes)
	if errEncode != nil {
		return nil, internalErrf("failed to encode map notes")
	}
	errUpdate := xs.db.UpdateGameServerSevenDaysToDieMapNotes(gameServer.ID, string(encoded), user.ID)
	if errUpdate != nil {
		return nil, internalErrf("failed to store map notes")
	}
	return connect.NewResponse(&xylona.UpdateSevenDaysToDieMapNotesResponse{
		Markers: publicSevenDaysToDieMapNotes(notes),
	}), nil
}

// ListSevenDaysToDieMapShares returns every active public map link.
func (xs *XylonaService) ListSevenDaysToDieMapShares(
	_ context.Context,
	request *connect.Request[xylona.ListSevenDaysToDieMapSharesRequest],
) (*connect.Response[xylona.ListSevenDaysToDieMapSharesResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errServer := xs.sevenDaysToDieMapServer(request.Msg.GetGameServerId())
	if errServer != nil {
		return nil, errServer
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerSettings)
	if errPermission != nil {
		return nil, errPermission
	}
	shares, errShares := xs.db.ListGameServerSevenDaysToDieMapShares(gameServer.ID)
	if errShares != nil {
		return nil, internalErrf("failed to list map shares")
	}
	publicShares := make([]*xylona.SevenDaysToDieMapShare, 0, len(shares))
	for _, share := range shares {
		publicShares = append(publicShares, publicSevenDaysToDieMapShare(share))
	}
	response := connect.NewResponse(&xylona.ListSevenDaysToDieMapSharesResponse{Shares: publicShares})
	response.Header().Set("Cache-Control", "no-store")
	return response, nil
}

// RegenerateSevenDaysToDieMapShare creates and returns a public map capability token.
func (xs *XylonaService) RegenerateSevenDaysToDieMapShare(
	_ context.Context,
	request *connect.Request[xylona.RegenerateSevenDaysToDieMapShareRequest],
) (*connect.Response[xylona.RegenerateSevenDaysToDieMapShareResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errServer := xs.sevenDaysToDieMapServer(request.Msg.GetGameServerId())
	if errServer != nil {
		return nil, errServer
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerSettings)
	if errPermission != nil {
		return nil, errPermission
	}
	share, errGenerate := xs.db.CreateGameServerSevenDaysToDieMapShare(gameServer.ID, user.ID)
	if errGenerate != nil {
		return nil, internalErrf("failed to create map share")
	}
	response := connect.NewResponse(&xylona.RegenerateSevenDaysToDieMapShareResponse{
		ShareToken: share.Token,
		Share:      publicSevenDaysToDieMapShare(share),
	})
	response.Header().Set("Cache-Control", "no-store")
	return response, nil
}

// RevokeSevenDaysToDieMapShare removes a public map capability token.
func (xs *XylonaService) RevokeSevenDaysToDieMapShare(
	_ context.Context,
	request *connect.Request[xylona.RevokeSevenDaysToDieMapShareRequest],
) (*connect.Response[xylona.RevokeSevenDaysToDieMapShareResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errServer := xs.sevenDaysToDieMapServer(request.Msg.GetGameServerId())
	if errServer != nil {
		return nil, errServer
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerSettings)
	if errPermission != nil {
		return nil, errPermission
	}
	errRevoke := xs.db.RevokeGameServerSevenDaysToDieMapShare(gameServer.ID, request.Msg.GetShareId())
	if errRevoke != nil {
		return nil, internalErrf("failed to revoke map share")
	}
	return connect.NewResponse(&xylona.RevokeSevenDaysToDieMapShareResponse{}), nil
}

// GetPublicSevenDaysToDieMap returns a shared map view after validating its capability token.
func (xs *XylonaService) GetPublicSevenDaysToDieMap(
	ctx context.Context,
	request *connect.Request[xylona.GetPublicSevenDaysToDieMapRequest],
) (*connect.Response[xylona.GetPublicSevenDaysToDieMapResponse], error) {
	settings, errSettings := xs.db.GetGameServerSevenDaysToDieMapByShareToken(request.Msg.GetShareToken())
	if errors.Is(errSettings, db.ErrSevenDaysToDieMapShareNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("map share was not found"))
	}
	if errSettings != nil {
		return nil, internalErrf("failed to resolve map share")
	}
	gameServer, errServer := xs.sevenDaysToDieMapServer(settings.GameServerID)
	if errServer != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("map share was not found"))
	}
	view, errView := xs.buildSevenDaysToDieMapView(ctx, gameServer)
	if errView != nil {
		return nil, errView
	}
	response := connect.NewResponse(&xylona.GetPublicSevenDaysToDieMapResponse{Map: view})
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Referrer-Policy", "no-referrer")
	return response, nil
}

func (xs *XylonaService) buildSevenDaysToDieMapView(ctx context.Context, gameServer *models.GameServer) (*xylona.SevenDaysToDieMapView, error) {
	settings, errSettings := xs.db.GetGameServerSevenDaysToDieMap(gameServer.ID)
	if errSettings != nil {
		return nil, internalErrf("failed to load map settings")
	}
	notes, errNotes := decodeSevenDaysToDieMapNotes(settings.NotesJSON)
	if errNotes != nil {
		return nil, internalErrf("failed to decode map notes")
	}
	cached, cachedFound, errCached := decodeStoredSevenDaysToDieMapSnapshot(settings.SnapshotJSON)
	if errCached != nil {
		log.Warn().Err(errCached).Str("game_server_id", gameServer.ID).Msg("Ignoring invalid cached 7 Days to Die map snapshot")
		cached = nil
	}
	if !cachedFound {
		cached = nil
	}

	collectedAt := time.Time{}
	if settings.SnapshotAt.Valid {
		collectedAt = settings.SnapshotAt.Time.UTC()
	}
	statusMessage := ""
	stale := false
	liveSnapshot, errLive := xs.querySevenDaysToDieMap(ctx, gameServer)
	if errLive == nil && liveSnapshot != nil {
		collectedAt = time.Now().UTC()
		cached = mergeSevenDaysToDieMapSnapshot(cached, liveSnapshot, collectedAt)
		encoded, errEncode := json.Marshal(cached)
		if errEncode == nil {
			errStore := xs.db.StoreGameServerSevenDaysToDieMapSnapshot(gameServer.ID, string(encoded), collectedAt)
			if errStore != nil {
				log.Warn().Err(errStore).Str("game_server_id", gameServer.ID).Msg("Failed to cache 7 Days to Die map snapshot")
			}
		}
		if !liveSnapshot.Enabled {
			statusMessage = "Map rendering is disabled in serverconfig.xml."
		}
	} else {
		stale = true
		statusMessage = "Live map data is unavailable. Showing the last known snapshot when available."
		if cached != nil {
			for index := range cached.Players {
				cached.Players[index].Online = false
			}
		}
	}

	view := &xylona.SevenDaysToDieMapView{
		GameServerId:    gameServer.ID,
		GameServerName:  gameServer.Name,
		ShareEnabled:    settings.ShareEnabled,
		Stale:           stale,
		StatusMessage:   statusMessage,
		TileUrlTemplate: SevenDaysToDieMapTilePathPrefix + "/" + gameServer.ID + "/{z}/{x}/{y}.png",
	}
	if !collectedAt.IsZero() {
		view.CollectedAt = timestamppb.New(collectedAt)
	}
	if cached == nil {
		view.Markers = publicSevenDaysToDieMapNotes(notes)
		return view, nil
	}
	view.Enabled = cached.Enabled
	view.TileSize = cached.TileSize
	view.MaxZoom = cached.MaxZoom
	view.MapSize = publicSevenDaysToDieMapVector(cached.MapSize)
	view.SourceTime = cached.SourceTime
	view.Players = publicSevenDaysToDieMapPlayers(cached.Players)
	view.Markers = publicSevenDaysToDieMapNotes(notes)
	return view, nil
}

func (xs *XylonaService) querySevenDaysToDieMap(ctx context.Context, gameServer *models.GameServer) (*node.SevenDaysToDieMapSnapshot, error) {
	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}
	tokenName, tokenSecret, errCredentials := xs.actionsInst.SevenDaysToDieMapCredentials(gameServer)
	if errCredentials != nil {
		return nil, fmt.Errorf("resolve 7 Days to Die map credentials: %w", errCredentials)
	}
	snapshot, errQuery := client.QuerySevenDaysToDieMap(ctx, node.SevenDaysToDieMapQueryRequest{
		WorkingDirectory: gameServer.Directory,
		TokenName:        tokenName,
		TokenSecret:      tokenSecret,
	})
	if errQuery != nil {
		return nil, fmt.Errorf("query 7 Days to Die map: %w", errQuery)
	}
	return snapshot, nil
}

func (xs *XylonaService) sevenDaysToDieMapServer(gameServerID string) (*models.GameServer, error) {
	gameServer, errLookup := xs.db.GetGameServerByID(strings.TrimSpace(gameServerID))
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	if gameServer.GameID != sevenDaysToDieGameID {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("map is only available for 7 Days to Die servers"))
	}
	return gameServer, nil
}

func mergeSevenDaysToDieMapSnapshot(cached *storedSevenDaysToDieMapSnapshot, live *node.SevenDaysToDieMapSnapshot, now time.Time) *storedSevenDaysToDieMapSnapshot {
	playersByID := make(map[string]storedSevenDaysToDieMapPlayer)
	if cached != nil {
		for _, player := range cached.Players {
			player.Online = false
			playersByID[player.ID] = player
		}
	}
	for _, player := range live.Players {
		lastSeenAt := now
		existing, found := playersByID[player.ID]
		if !player.Online && found && !existing.LastSeenAt.IsZero() {
			lastSeenAt = existing.LastSeenAt
		}
		playersByID[player.ID] = storedSevenDaysToDieMapPlayer{
			ID: player.ID, Name: player.Name, Online: player.Online, Position: player.Position, LastSeenAt: lastSeenAt,
		}
	}
	players := make([]storedSevenDaysToDieMapPlayer, 0, len(playersByID))
	for _, player := range playersByID {
		players = append(players, player)
	}
	sort.Slice(players, func(i int, j int) bool {
		if players[i].Online != players[j].Online {
			return players[i].Online
		}
		return strings.ToLower(players[i].Name) < strings.ToLower(players[j].Name)
	})
	return &storedSevenDaysToDieMapSnapshot{
		Enabled: live.Enabled, TileSize: live.TileSize, MaxZoom: live.MaxZoom, MapSize: live.MapSize,
		SourceTime: live.SourceTime, Players: players,
	}
}

func decodeStoredSevenDaysToDieMapSnapshot(raw string) (*storedSevenDaysToDieMapSnapshot, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, false, nil
	}
	var snapshot storedSevenDaysToDieMapSnapshot
	errDecode := json.Unmarshal([]byte(raw), &snapshot)
	if errDecode != nil {
		return nil, false, fmt.Errorf("decode stored 7 Days to Die map snapshot: %w", errDecode)
	}
	return &snapshot, true, nil
}

func decodeSevenDaysToDieMapNotes(raw string) ([]sevenDaysToDieMapNoteConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return []sevenDaysToDieMapNoteConfig{}, nil
	}
	var notes []sevenDaysToDieMapNoteConfig
	errDecode := json.Unmarshal([]byte(raw), &notes)
	if errDecode != nil {
		return nil, fmt.Errorf("decode 7 Days to Die map notes: %w", errDecode)
	}
	return notes, nil
}

func validateSevenDaysToDieMapNotes(rawNotes []*xylona.SevenDaysToDieMapMarker) ([]sevenDaysToDieMapNoteConfig, error) {
	if len(rawNotes) > sevenDaysToDieMapNoteLimit {
		return nil, fmt.Errorf("at most %d map notes are allowed", sevenDaysToDieMapNoteLimit)
	}
	notes := make([]sevenDaysToDieMapNoteConfig, 0, len(rawNotes))
	seenIDs := make(map[string]struct{}, len(rawNotes))
	for _, rawNote := range rawNotes {
		if rawNote == nil || rawNote.GetNative() {
			return nil, errors.New("native server markers cannot be edited here")
		}
		id := strings.TrimSpace(rawNote.GetId())
		if id == "" {
			id = uuid.NewString()
		}
		if len(id) > 64 {
			return nil, errors.New("map note ID is too long")
		}
		_, duplicate := seenIDs[id]
		if duplicate {
			return nil, errors.New("map note IDs must be unique")
		}
		seenIDs[id] = struct{}{}
		name := strings.TrimSpace(rawNote.GetName())
		note := strings.TrimSpace(rawNote.GetNote())
		icon := strings.TrimSpace(rawNote.GetIcon())
		if name == "" || len(name) > 100 || len(note) > 500 || len(icon) > 64 {
			return nil, errors.New("map note title, note, or icon is invalid")
		}
		if !finiteMapCoordinate(rawNote.GetX()) || !finiteMapCoordinate(rawNote.GetZ()) {
			return nil, errors.New("map note coordinates are invalid")
		}
		notes = append(notes, sevenDaysToDieMapNoteConfig{
			ID: id, Name: name, Note: note, Icon: icon, X: rawNote.GetX(), Z: rawNote.GetZ(),
		})
	}
	return notes, nil
}

func finiteMapCoordinate(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= -1_000_000 && value <= 1_000_000
}

func publicSevenDaysToDieMapShare(share *db.GameServerSevenDaysToDieMapShare) *xylona.SevenDaysToDieMapShare {
	return &xylona.SevenDaysToDieMapShare{
		Id: share.ID, ShareToken: share.Token, CreatedAt: timestamppb.New(share.CreatedAt),
	}
}

func publicSevenDaysToDieMapVector(vector node.SevenDaysToDieMapVector) *xylona.SevenDaysToDieMapVector {
	return &xylona.SevenDaysToDieMapVector{X: vector.X, Y: vector.Y, Z: vector.Z}
}

func publicSevenDaysToDieMapPlayers(players []storedSevenDaysToDieMapPlayer) []*xylona.SevenDaysToDieMapPlayer {
	result := make([]*xylona.SevenDaysToDieMapPlayer, 0, len(players))
	for _, player := range players {
		item := &xylona.SevenDaysToDieMapPlayer{
			Id: player.ID, Name: player.Name, Online: player.Online, Position: publicSevenDaysToDieMapVector(player.Position),
		}
		if !player.LastSeenAt.IsZero() {
			item.LastSeenAt = timestamppb.New(player.LastSeenAt)
		}
		result = append(result, item)
	}
	return result
}

func publicSevenDaysToDieMapNotes(notes []sevenDaysToDieMapNoteConfig) []*xylona.SevenDaysToDieMapMarker {
	result := make([]*xylona.SevenDaysToDieMapMarker, 0, len(notes))
	for _, note := range notes {
		result = append(result, &xylona.SevenDaysToDieMapMarker{
			Id: note.ID, Name: note.Name, Note: note.Note, Icon: note.Icon, X: note.X, Z: note.Z,
		})
	}
	return result
}

// SevenDaysToDieMapTile serves one authenticated or capability-authorized tile.
func (xs *XylonaService) SevenDaysToDieMapTile(response http.ResponseWriter, request *http.Request) {
	gameServerID := strings.TrimSpace(chi.URLParam(request, "gameServerId"))
	gameServer, errServer := xs.sevenDaysToDieMapServer(gameServerID)
	if errServer != nil {
		http.NotFound(response, request)
		return
	}
	errAccess := xs.authorizeSevenDaysToDieMapTile(request, gameServer)
	if errAccess != nil {
		http.Error(response, "unauthorized", http.StatusUnauthorized)
		return
	}
	zoom, errZoom := strconv.ParseInt(chi.URLParam(request, "zoom"), 10, 32)
	x, errX := strconv.ParseInt(chi.URLParam(request, "x"), 10, 32)
	yText := strings.TrimSuffix(chi.URLParam(request, "y"), ".png")
	y, errY := strconv.ParseInt(yText, 10, 32)
	if errZoom != nil || errX != nil || errY != nil {
		http.NotFound(response, request)
		return
	}
	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		http.Error(response, "map tile unavailable", http.StatusServiceUnavailable)
		return
	}
	tokenName, tokenSecret, errCredentials := xs.actionsInst.SevenDaysToDieMapCredentials(gameServer)
	if errCredentials != nil {
		http.Error(response, "map tile unavailable", http.StatusServiceUnavailable)
		return
	}
	content, errTile := client.GetSevenDaysToDieMapTile(request.Context(), node.SevenDaysToDieMapTileRequest{
		WorkingDirectory: gameServer.Directory,
		TokenName:        tokenName,
		TokenSecret:      tokenSecret,
		Zoom:             int32(zoom), X: int32(x), Y: int32(y),
	})
	if errors.Is(errTile, fs.ErrNotExist) {
		http.NotFound(response, request)
		return
	}
	if errTile != nil {
		http.Error(response, "map tile unavailable", http.StatusServiceUnavailable)
		return
	}
	response.Header().Set("Content-Type", "image/png")
	response.Header().Set("Cache-Control", "private, max-age=15")
	response.Header().Set("Vary", "Cookie, "+sevenDaysToDieMapShareHeader)
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.WriteHeader(http.StatusOK)
	_, errWrite := response.Write(content) //nolint:gosec // The node validates the PNG signature and the controller fixes the content type.
	if errWrite != nil {
		log.Debug().Err(errWrite).Str("game_server_id", gameServer.ID).Msg("Map tile response closed early")
	}
}

func (xs *XylonaService) authorizeSevenDaysToDieMapTile(request *http.Request, gameServer *models.GameServer) error {
	shareToken := strings.TrimSpace(request.Header.Get(sevenDaysToDieMapShareHeader))
	if shareToken != "" {
		settings, errShare := xs.db.GetGameServerSevenDaysToDieMapByShareToken(shareToken)
		if errShare != nil || settings.GameServerID != gameServer.ID {
			return errors.New("map share is invalid")
		}
		return nil
	}
	user, errUser := xs.getUserFromHeader(request.Header)
	if errUser != nil {
		return errUser
	}
	return xs.ensureLocalServerPermission(user, gameServer, permissionGameServerView)
}
