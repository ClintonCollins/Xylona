package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/controller/actions"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/palworldmap"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	permissionGameServerView     = "game_server.view"
	permissionGameServerSettings = "game_server.settings"
	palworldGameID               = "palworld"
	palworldMapMaxLayers         = 4
	// palworldMapStaleAfter must clear the map query timeout plus a poll
	// interval. A populated world can take several seconds to serialize, and a
	// tighter window flagged those working-but-slow snapshots as stale.
	palworldMapStaleAfter         = 30 * time.Second
	palworldMapTileInstallTimeout = 10 * time.Minute
)

var palworldMapLayerIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type palworldMapLayerConfig struct {
	ID              string  `json:"id"`
	Label           string  `json:"label"`
	TileURLTemplate string  `json:"tile_url_template"`
	Attribution     string  `json:"attribution"`
	MinZoom         int32   `json:"min_zoom"`
	MaxZoom         int32   `json:"max_zoom"`
	TileSize        int32   `json:"tile_size"`
	TransformA      float64 `json:"transform_a"`
	TransformB      float64 `json:"transform_b"`
	TransformC      float64 `json:"transform_c"`
	TransformD      float64 `json:"transform_d"`
	MinX            float64 `json:"min_x"`
	MinY            float64 `json:"min_y"`
	MaxX            float64 `json:"max_x"`
	MaxY            float64 `json:"max_y"`
}

// GetPalworldMap returns the exact cached world state to any user who can
// view the server. Share and map-source controls remain settings-gated.
func (xs *XylonaService) GetPalworldMap(
	_ context.Context,
	request *connect.Request[xylona.GetPalworldMapRequest],
) (*connect.Response[xylona.GetPalworldMapResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errServer := xs.palworldMapServer(request.Msg.GetGameServerId())
	if errServer != nil {
		return nil, errServer
	}
	errView := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerView)
	if errView != nil {
		return nil, errView
	}
	if xs.actionsInst == nil {
		return nil, internalErr()
	}

	settings, errSettings := xs.db.GetGameServerPalworldMap(gameServer.ID)
	if errSettings != nil {
		log.Error().Err(errSettings).Str("game_server_id", gameServer.ID).Msg("Failed to load Palworld map settings")
		return nil, internalErr()
	}
	layers, errLayers := decodePalworldMapLayers(settings.LayersJSON)
	if errLayers != nil {
		log.Error().Err(errLayers).Str("game_server_id", gameServer.ID).Msg("Stored Palworld map layer configuration is invalid")
		return nil, internalErr()
	}
	canManage, errManage := db.HasPermission(
		xs.db,
		user,
		gameServer.ID,
		gameServer.UserID,
		permissionGameServerSettings,
	)
	if errManage != nil {
		log.Error().Err(errManage).Str("game_server_id", gameServer.ID).Msg("Failed to check Palworld map settings permission")
		return nil, internalErr()
	}

	state := xs.actionsInst.GetPalworldMapState(gameServer.ID)
	if state.ServerName == "" {
		state.ServerName = gameServer.Name
	}
	view := palworldMapView(
		state,
		layers,
		canManage,
		settings.ShareTokenHash.Valid,
		time.Now().UTC(),
	)
	return connect.NewResponse(&xylona.GetPalworldMapResponse{Map: view}), nil
}

// UpdatePalworldMapConfig stores administrator-supplied tile definitions.
func (xs *XylonaService) UpdatePalworldMapConfig(
	_ context.Context,
	request *connect.Request[xylona.UpdatePalworldMapConfigRequest],
) (*connect.Response[xylona.UpdatePalworldMapConfigResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errServer := xs.palworldMapServer(request.Msg.GetGameServerId())
	if errServer != nil {
		return nil, errServer
	}
	errSettings := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerSettings)
	if errSettings != nil {
		return nil, errSettings
	}

	layers, errLayers := validatePalworldMapLayers(request.Msg.GetLayers())
	if errLayers != nil {
		return nil, invalidArg(errLayers.Error())
	}
	layersJSON, errMarshal := json.Marshal(layers)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Str("game_server_id", gameServer.ID).Msg("Failed to encode Palworld map layers")
		return nil, internalErr()
	}
	errUpdate := xs.db.UpdateGameServerPalworldMapLayers(gameServer.ID, string(layersJSON), user.ID)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("game_server_id", gameServer.ID).Msg("Failed to update Palworld map layers")
		return nil, internalErr()
	}
	return connect.NewResponse(&xylona.UpdatePalworldMapConfigResponse{
		Layers: publicPalworldMapLayers(layers),
	}), nil
}

// InstallPalworldMapTiles downloads the supported Palworld 1.0 tile layers to
// controller-local storage and selects those same-origin layers for the server.
func (xs *XylonaService) InstallPalworldMapTiles(
	_ context.Context,
	request *connect.Request[xylona.InstallPalworldMapTilesRequest],
) (*connect.Response[xylona.InstallPalworldMapTilesResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errServer := xs.palworldMapServer(request.Msg.GetGameServerId())
	if errServer != nil {
		return nil, errServer
	}
	errSettings := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerSettings)
	if errSettings != nil {
		return nil, errSettings
	}
	if xs.palworldMapTiles == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("local Palworld map tile storage is unavailable"))
	}

	// Use the controller lifecycle rather than the default unary deadline so a
	// slow first-time install can finish, while shutdown still cancels it.
	downloadContext, cancelDownload := context.WithTimeout(xs.ctx, palworldMapTileInstallTimeout)
	defer cancelDownload()
	errInstall := xs.palworldMapTiles.Install(downloadContext)
	if errInstall != nil {
		log.Error().Err(errInstall).Str("game_server_id", gameServer.ID).Msg("Failed to install Palworld map tiles")
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("palworld map tiles could not be installed; check the controller logs and retry"))
	}

	layers := localPalworldMapLayerConfigs(xs.palworldMapTiles.Layers())
	layersJSON, errMarshal := json.Marshal(layers)
	if errMarshal != nil {
		log.Error().Err(errMarshal).Str("game_server_id", gameServer.ID).Msg("Failed to encode installed Palworld map layers")
		return nil, internalErr()
	}
	errUpdate := xs.db.UpdateGameServerPalworldMapLayers(gameServer.ID, string(layersJSON), user.ID)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("game_server_id", gameServer.ID).Msg("Failed to select installed Palworld map layers")
		return nil, internalErr()
	}

	return connect.NewResponse(&xylona.InstallPalworldMapTilesResponse{
		Layers: publicPalworldMapLayers(layers),
	}), nil
}

// RegeneratePalworldMapShare rotates the capability token and returns the new
// plaintext token once. Existing public links stop working immediately.
func (xs *XylonaService) RegeneratePalworldMapShare(
	_ context.Context,
	request *connect.Request[xylona.RegeneratePalworldMapShareRequest],
) (*connect.Response[xylona.RegeneratePalworldMapShareResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errServer := xs.palworldMapServer(request.Msg.GetGameServerId())
	if errServer != nil {
		return nil, errServer
	}
	errSettings := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerSettings)
	if errSettings != nil {
		return nil, errSettings
	}
	token, errGenerate := xs.db.RegenerateGameServerPalworldMapShare(gameServer.ID, user.ID)
	if errGenerate != nil {
		log.Error().Err(errGenerate).Str("game_server_id", gameServer.ID).Msg("Failed to regenerate Palworld map share")
		return nil, internalErr()
	}
	return connect.NewResponse(&xylona.RegeneratePalworldMapShareResponse{ShareToken: token}), nil
}

// RevokePalworldMapShare disables the current public map link.
func (xs *XylonaService) RevokePalworldMapShare(
	_ context.Context,
	request *connect.Request[xylona.RevokePalworldMapShareRequest],
) (*connect.Response[xylona.RevokePalworldMapShareResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errServer := xs.palworldMapServer(request.Msg.GetGameServerId())
	if errServer != nil {
		return nil, errServer
	}
	errSettings := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerSettings)
	if errSettings != nil {
		return nil, errSettings
	}
	errRevoke := xs.db.RevokeGameServerPalworldMapShare(gameServer.ID, user.ID)
	if errRevoke != nil {
		log.Error().Err(errRevoke).Str("game_server_id", gameServer.ID).Msg("Failed to revoke Palworld map share")
		return nil, internalErr()
	}
	return connect.NewResponse(&xylona.RevokePalworldMapShareResponse{}), nil
}

// GetPublicPalworldMap resolves a public capability token without requiring a
// session. The token is sent in the request body, not the request URL.
func (xs *XylonaService) GetPublicPalworldMap(
	_ context.Context,
	request *connect.Request[xylona.GetPublicPalworldMapRequest],
) (*connect.Response[xylona.GetPublicPalworldMapResponse], error) {
	settings, errSettings := xs.db.GetGameServerPalworldMapByShareToken(request.Msg.GetShareToken())
	if errors.Is(errSettings, db.ErrPalworldMapShareNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("public map link is invalid or has been revoked"))
	}
	if errSettings != nil {
		log.Error().Err(errSettings).Msg("Failed to resolve public Palworld map share")
		return nil, internalErr()
	}
	gameServer, errGameServer := xs.db.GetGameServerByID(settings.GameServerID)
	if errGameServer != nil || gameServer.GameID != palworldGameID {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("public map link is invalid or has been revoked"))
	}
	if xs.actionsInst == nil {
		return nil, internalErr()
	}
	layers, errLayers := decodePalworldMapLayers(settings.LayersJSON)
	if errLayers != nil {
		log.Error().Err(errLayers).Str("game_server_id", settings.GameServerID).Msg("Stored public Palworld map layer configuration is invalid")
		return nil, internalErr()
	}
	state := xs.actionsInst.GetPalworldMapState(settings.GameServerID)
	if state.ServerName == "" {
		state.ServerName = gameServer.Name
	}
	view := palworldMapView(
		state,
		layers,
		false,
		false,
		time.Now().UTC(),
	)
	response := connect.NewResponse(&xylona.GetPublicPalworldMapResponse{Map: view})
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Referrer-Policy", "no-referrer")
	return response, nil
}

func (xs *XylonaService) palworldMapServer(gameServerID string) (*models.GameServer, error) {
	gameServerID = strings.TrimSpace(gameServerID)
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}
	gameServer, errLookup := xs.db.GetGameServerByID(gameServerID)
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	if gameServer.GameID != palworldGameID {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("live map is available only for Palworld servers"))
	}
	return gameServer, nil
}

func palworldMapView(
	state actions.PalworldMapState,
	layers []palworldMapLayerConfig,
	canManage bool,
	shareEnabled bool,
	now time.Time,
) *xylona.PalworldMapView {
	view := &xylona.PalworldMapView{
		ServerName:        state.ServerName,
		ServerOnline:      state.ServerOnline,
		UnavailableReason: state.UnavailableReason,
		Layers:            publicPalworldMapLayers(layers),
		CanManageShare:    canManage,
		ShareEnabled:      shareEnabled,
	}
	if state.Snapshot == nil {
		return view
	}
	snapshot := state.Snapshot
	view.Available = true
	view.Stale = !state.ServerOnline || state.UnavailableReason != "" || snapshot.CollectedAt.IsZero() || now.Sub(snapshot.CollectedAt) > palworldMapStaleAfter
	view.Partial = snapshot.Partial
	view.PartialReason = snapshot.PartialReason
	view.Truncated = snapshot.Truncated
	view.Source = snapshot.Source
	view.SourceTime = snapshot.SourceTime
	if !snapshot.CollectedAt.IsZero() {
		view.CollectedAt = timestamppb.New(snapshot.CollectedAt)
	}
	view.Actors = publicPalworldMapActors(snapshot.Actors)
	view.Health = publicPalworldMapHealth(snapshot.Health)
	return view
}

func publicPalworldMapActors(actors []node.PalworldMapActor) []*xylona.PalworldMapActor {
	result := make([]*xylona.PalworldMapActor, 0, len(actors))
	for _, actor := range actors {
		result = append(result, &xylona.PalworldMapActor{
			Key:         actor.Key,
			Kind:        publicPalworldMapActorKind(actor.Kind),
			Name:        actor.Name,
			GuildKey:    actor.GuildKey,
			GuildName:   actor.GuildName,
			TrainerName: actor.TrainerName,
			ClassName:   actor.ClassName,
			LocationX:   actor.LocationX,
			LocationY:   actor.LocationY,
			LocationZ:   actor.LocationZ,
			RotationZ:   actor.RotationZ,
			Level:       actor.Level,
			Hp:          actor.HP,
			MaxHp:       actor.MaxHP,
			Action:      actor.Action,
			AiAction:    actor.AIAction,
			Active:      actor.Active,
		})
	}
	return result
}

func publicPalworldMapHealth(health *node.PalworldMapHealth) *xylona.PalworldMapHealth {
	if health == nil {
		return nil
	}
	return &xylona.PalworldMapHealth{
		ServerFps:         health.ServerFPS,
		ServerFrameTimeMs: health.ServerFrameTimeMS,
		CurrentPlayers:    health.CurrentPlayers,
		MaxPlayers:        health.MaxPlayers,
		UptimeSeconds:     health.UptimeSeconds,
		BaseCampCount:     health.BaseCampCount,
		Days:              health.Days,
	}
}

func publicPalworldMapActorKind(kind node.PalworldMapActorKind) xylona.PalworldMapActorKind {
	switch kind {
	case node.PalworldMapActorKindPlayer:
		return xylona.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_PLAYER
	case node.PalworldMapActorKindBase:
		return xylona.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_BASE
	case node.PalworldMapActorKindBaseWorker:
		return xylona.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_BASE_WORKER
	case node.PalworldMapActorKindCompanionPal:
		return xylona.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_COMPANION_PAL
	case node.PalworldMapActorKindWildPal:
		return xylona.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_WILD_PAL
	case node.PalworldMapActorKindNPC:
		return xylona.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_NPC
	case node.PalworldMapActorKindOther:
		return xylona.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_OTHER
	default:
		return xylona.PalworldMapActorKind_PALWORLD_MAP_ACTOR_KIND_UNSPECIFIED
	}
}

func validatePalworldMapLayers(rawLayers []*xylona.PalworldMapLayer) ([]palworldMapLayerConfig, error) {
	if len(rawLayers) > palworldMapMaxLayers {
		return nil, fmt.Errorf("no more than %d map layers are allowed", palworldMapMaxLayers)
	}
	seenIDs := make(map[string]struct{}, len(rawLayers))
	layers := make([]palworldMapLayerConfig, 0, len(rawLayers))
	for i, rawLayer := range rawLayers {
		if rawLayer == nil {
			return nil, fmt.Errorf("map layer %d is required", i+1)
		}
		layer := palworldMapLayerConfig{
			ID:              strings.TrimSpace(rawLayer.GetId()),
			Label:           strings.TrimSpace(rawLayer.GetLabel()),
			TileURLTemplate: strings.TrimSpace(rawLayer.GetTileUrlTemplate()),
			Attribution:     strings.TrimSpace(rawLayer.GetAttribution()),
			MinZoom:         rawLayer.GetMinZoom(),
			MaxZoom:         rawLayer.GetMaxZoom(),
			TileSize:        rawLayer.GetTileSize(),
			TransformA:      rawLayer.GetTransformA(),
			TransformB:      rawLayer.GetTransformB(),
			TransformC:      rawLayer.GetTransformC(),
			TransformD:      rawLayer.GetTransformD(),
			MinX:            rawLayer.GetMinX(),
			MinY:            rawLayer.GetMinY(),
			MaxX:            rawLayer.GetMaxX(),
			MaxY:            rawLayer.GetMaxY(),
		}
		errLayer := validatePalworldMapLayer(layer, seenIDs)
		if errLayer != nil {
			return nil, fmt.Errorf("map layer %d: %w", i+1, errLayer)
		}
		seenIDs[layer.ID] = struct{}{}
		layers = append(layers, layer)
	}
	return layers, nil
}

func validatePalworldMapLayer(layer palworldMapLayerConfig, seenIDs map[string]struct{}) error {
	if layer.ID == "" || len(layer.ID) > 32 || !palworldMapLayerIDPattern.MatchString(layer.ID) {
		return errors.New("ID must use 1-32 letters, numbers, underscores, or hyphens")
	}
	if _, exists := seenIDs[layer.ID]; exists {
		return fmt.Errorf("ID %q is duplicated", layer.ID)
	}
	if layer.Label == "" || len(layer.Label) > 64 {
		return errors.New("label must contain 1-64 characters")
	}
	if len(layer.TileURLTemplate) > 2048 || !strings.Contains(layer.TileURLTemplate, "{z}") || !strings.Contains(layer.TileURLTemplate, "{x}") || !strings.Contains(layer.TileURLTemplate, "{y}") {
		return errors.New("tile URL must contain {z}, {x}, and {y}")
	}
	resolvedTileURL := strings.NewReplacer("{z}", "0", "{x}", "0", "{y}", "0").Replace(layer.TileURLTemplate)
	parsedURL, errURL := url.Parse(resolvedTileURL)
	validAbsoluteURL := errURL == nil && parsedURL.Host != "" && (parsedURL.Scheme == "http" || parsedURL.Scheme == "https")
	validSameOriginURL := errURL == nil && parsedURL.Scheme == "" && parsedURL.Host == "" && strings.HasPrefix(resolvedTileURL, "/") && !strings.HasPrefix(resolvedTileURL, "//")
	if !validAbsoluteURL && !validSameOriginURL {
		return errors.New("tile URL must be a same-origin path or an absolute HTTP or HTTPS URL")
	}
	if layer.Attribution == "" || len(layer.Attribution) > 300 {
		return errors.New("attribution must contain 1-300 characters")
	}
	if layer.MinZoom < 0 || layer.MaxZoom < layer.MinZoom || layer.MaxZoom > 12 {
		return errors.New("zoom range must be between 0 and 12")
	}
	if layer.TileSize < 128 || layer.TileSize > 1024 {
		return errors.New("tile size must be between 128 and 1024 pixels")
	}
	numbers := []float64{
		layer.TransformA,
		layer.TransformB,
		layer.TransformC,
		layer.TransformD,
		layer.MinX,
		layer.MinY,
		layer.MaxX,
		layer.MaxY,
	}
	for _, number := range numbers {
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return errors.New("transform and bounds must contain finite numbers")
		}
	}
	if layer.TransformA == 0 || layer.TransformC == 0 {
		return errors.New("transform A and C cannot be zero")
	}
	if layer.MaxX <= layer.MinX || layer.MaxY <= layer.MinY {
		return errors.New("maximum bounds must be greater than minimum bounds")
	}
	return nil
}

func localPalworldMapLayerConfigs(layers []palworldmap.Layer) []palworldMapLayerConfig {
	configs := make([]palworldMapLayerConfig, 0, len(layers))
	for _, layer := range layers {
		configs = append(configs, palworldMapLayerConfig{
			ID:              layer.ID,
			Label:           layer.Label,
			TileURLTemplate: palworldmap.TileURLTemplate(layer.ID),
			Attribution:     layer.Attribution,
			MinZoom:         layer.MinZoom,
			MaxZoom:         layer.MaxZoom,
			TileSize:        layer.TileSize,
			TransformA:      layer.TransformA,
			TransformB:      layer.TransformB,
			TransformC:      layer.TransformC,
			TransformD:      layer.TransformD,
			MinX:            layer.MinX,
			MinY:            layer.MinY,
			MaxX:            layer.MaxX,
			MaxY:            layer.MaxY,
		})
	}
	return configs
}

func decodePalworldMapLayers(raw string) ([]palworldMapLayerConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var layers []palworldMapLayerConfig
	errDecode := json.Unmarshal([]byte(raw), &layers)
	if errDecode != nil {
		return nil, fmt.Errorf("decode palworld map layers: %w", errDecode)
	}
	return layers, nil
}

func publicPalworldMapLayers(layers []palworldMapLayerConfig) []*xylona.PalworldMapLayer {
	result := make([]*xylona.PalworldMapLayer, 0, len(layers))
	for _, layer := range layers {
		result = append(result, &xylona.PalworldMapLayer{
			Id:              layer.ID,
			Label:           layer.Label,
			TileUrlTemplate: layer.TileURLTemplate,
			Attribution:     layer.Attribution,
			MinZoom:         layer.MinZoom,
			MaxZoom:         layer.MaxZoom,
			TileSize:        layer.TileSize,
			TransformA:      layer.TransformA,
			TransformB:      layer.TransformB,
			TransformC:      layer.TransformC,
			TransformD:      layer.TransformD,
			MinX:            layer.MinX,
			MinY:            layer.MinY,
			MaxX:            layer.MaxX,
			MaxY:            layer.MaxY,
		})
	}
	return result
}
