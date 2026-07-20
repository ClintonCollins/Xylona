package rpc

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/controller/authz"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	minecraftGameID = "minecraft"
	// MinecraftMapViewerPathPrefix is the session-authenticated BlueMap asset route.
	MinecraftMapViewerPathPrefix = "/api/minecraft-map/view"
	// MinecraftMapSharedPathPrefix is the public capability-token BlueMap asset route.
	MinecraftMapSharedPathPrefix = "/api/minecraft-map/shared"
	minecraftMapNodeProtocol     = 8
	minecraftMapDefaultWorldName = "world"
	minecraftMapShareCookieName  = "xylona_minecraft_map_share"
	minecraftMapShareGrantName   = "minecraft-map-share"
	minecraftMapShareGrantTTL    = 15 * time.Minute
)

var minecraftWorldNamePattern = regexp.MustCompile(`^[A-Za-z0-9._ -]{1,80}$`)

type minecraftMapShareGrant struct {
	GameServerID   string
	ShareTokenHash string
	ExpiresAt      int64
}

// GetMinecraftMap returns BlueMap setup and viewer state to users who can view
// the server. Configuration and sharing remain settings-gated.
func (xs *XylonaService) GetMinecraftMap(
	ctx context.Context,
	request *connect.Request[xylona.GetMinecraftMapRequest],
) (*connect.Response[xylona.GetMinecraftMapResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errServer := xs.minecraftMapServer(request.Msg.GetGameServerId())
	if errServer != nil {
		return nil, errServer
	}
	errView := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerView)
	if errView != nil {
		return nil, errView
	}
	canManage, errManage := authz.HasPermission(xs.db, user, gameServer.ID, gameServer.UserID, permissionGameServerSettings)
	if errManage != nil {
		log.Error().Err(errManage).Str("game_server_id", gameServer.ID).Msg("Failed to check Minecraft map settings permission")
		return nil, internalErr()
	}
	view, errBuild := xs.buildMinecraftMapView(ctx, gameServer, canManage, false)
	if errBuild != nil {
		return nil, errBuild
	}
	return connect.NewResponse(&xylona.GetMinecraftMapResponse{Map: view}), nil
}

// UpdateMinecraftMapConfig persists map activation and explicit BlueMap
// resource-download acceptance. It reuses game_server.settings.
func (xs *XylonaService) UpdateMinecraftMapConfig(
	ctx context.Context,
	request *connect.Request[xylona.UpdateMinecraftMapConfigRequest],
) (*connect.Response[xylona.UpdateMinecraftMapConfigResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errServer := xs.minecraftMapServer(request.Msg.GetGameServerId())
	if errServer != nil {
		return nil, errServer
	}
	errSettingsPermission := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerSettings)
	if errSettingsPermission != nil {
		return nil, errSettingsPermission
	}

	worldName := strings.TrimSpace(request.Msg.GetWorldName())
	if worldName == "" {
		worldName = minecraftMapDefaultWorldName
	}
	if !validMinecraftWorldName(worldName) {
		return nil, invalidArg("world_name must be a local Minecraft world folder name")
	}
	current, errCurrent := xs.db.GetGameServerMinecraftMap(gameServer.ID)
	if errCurrent != nil {
		log.Error().Err(errCurrent).Str("game_server_id", gameServer.ID).Msg("Failed to load Minecraft map settings")
		return nil, internalErr()
	}
	if request.Msg.GetEnabled() && !current.AcceptedAt.Valid && !request.Msg.GetAcceptBluemapDownload() {
		return nil, invalidArg("accept_bluemap_download is required before Xylona can install BlueMap resources")
	}
	if request.Msg.GetEnabled() {
		errRCONPort := xs.ensureMinecraftMapRCONPortAvailable(gameServer)
		if errRCONPort != nil {
			return nil, errRCONPort
		}
	}
	errUpdate := xs.db.UpdateGameServerMinecraftMapConfig(
		gameServer.ID,
		request.Msg.GetEnabled(),
		worldName,
		request.Msg.GetAcceptBluemapDownload(),
		user.ID,
	)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("game_server_id", gameServer.ID).Msg("Failed to update Minecraft map settings")
		return nil, internalErr()
	}
	if !request.Msg.GetEnabled() {
		client, errClient := xs.resolveNodeClient(gameServer)
		if errClient != nil {
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("minecraft map was disabled, but its node is unavailable to stop the renderer"))
		}
		errStop := client.StopMinecraftMap(ctx, gameServer.ID)
		if errStop != nil {
			log.Warn().Err(errStop).Str("game_server_id", gameServer.ID).Msg("Failed to stop managed Minecraft map")
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("minecraft map was disabled, but the renderer did not stop cleanly"))
		}
	}
	view, errBuild := xs.buildMinecraftMapView(ctx, gameServer, true, false)
	if errBuild != nil {
		return nil, errBuild
	}
	return connect.NewResponse(&xylona.UpdateMinecraftMapConfigResponse{Map: view}), nil
}

func (xs *XylonaService) ensureMinecraftMapRCONPortAvailable(gameServer *models.GameServer) error {
	if gameServer.QueryPort >= 65535 {
		return invalidArg("Minecraft map requires query_port + 1 to be a valid RCON port")
	}
	rconPort := gameServer.QueryPort + 1
	servers, errServers := xs.db.GetGameServersByNodeID(gameServer.NodeID)
	if errServers != nil {
		log.Error().Err(errServers).Str("game_server_id", gameServer.ID).Msg("Failed to validate Minecraft map RCON port")
		return internalErr()
	}
	selectedIP := strings.TrimSpace(gameServer.IP)
	selectedBindsToAllIPs := gameServer.R.Game != nil && gameServer.R.Game.BindsToAllIps
	for _, existingServer := range servers {
		if existingServer.ID == gameServer.ID {
			continue
		}
		existingBindsToAllIPs := existingServer.R.Game != nil && existingServer.R.Game.BindsToAllIps
		sameIP := strings.TrimSpace(existingServer.IP) == selectedIP
		if !sameIP && !selectedBindsToAllIPs && !existingBindsToAllIPs {
			continue
		}
		existingPorts, errPorts := xs.gameServerPortFootprint(existingServer)
		if errPorts != nil {
			log.Error().Err(errPorts).Str("game_server_id", gameServer.ID).Msg("Failed to inspect Minecraft map RCON conflicts")
			return internalErr()
		}
		if slices.Contains(existingPorts, rconPort) {
			return connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("minecraft map RCON port %d is already in use on this node", rconPort))
		}
	}
	return nil
}

// RegenerateMinecraftMapShare rotates the public capability token.
func (xs *XylonaService) RegenerateMinecraftMapShare(
	_ context.Context,
	request *connect.Request[xylona.RegenerateMinecraftMapShareRequest],
) (*connect.Response[xylona.RegenerateMinecraftMapShareResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errServer := xs.minecraftMapServer(request.Msg.GetGameServerId())
	if errServer != nil {
		return nil, errServer
	}
	errSettingsPermission := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerSettings)
	if errSettingsPermission != nil {
		return nil, errSettingsPermission
	}
	settings, errSettings := xs.db.GetGameServerMinecraftMap(gameServer.ID)
	if errSettings != nil {
		return nil, internalErr()
	}
	if !settings.Enabled {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("enable the Minecraft map before sharing it"))
	}
	token, errGenerate := xs.db.RegenerateGameServerMinecraftMapShare(gameServer.ID, user.ID)
	if errGenerate != nil {
		log.Error().Err(errGenerate).Str("game_server_id", gameServer.ID).Msg("Failed to regenerate Minecraft map share")
		return nil, internalErr()
	}
	return connect.NewResponse(&xylona.RegenerateMinecraftMapShareResponse{ShareToken: token}), nil
}

// RevokeMinecraftMapShare disables the current public capability token.
func (xs *XylonaService) RevokeMinecraftMapShare(
	_ context.Context,
	request *connect.Request[xylona.RevokeMinecraftMapShareRequest],
) (*connect.Response[xylona.RevokeMinecraftMapShareResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServer, errServer := xs.minecraftMapServer(request.Msg.GetGameServerId())
	if errServer != nil {
		return nil, errServer
	}
	errSettingsPermission := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerSettings)
	if errSettingsPermission != nil {
		return nil, errSettingsPermission
	}
	errRevoke := xs.db.RevokeGameServerMinecraftMapShare(gameServer.ID, user.ID)
	if errRevoke != nil {
		log.Error().Err(errRevoke).Str("game_server_id", gameServer.ID).Msg("Failed to revoke Minecraft map share")
		return nil, internalErr()
	}
	return connect.NewResponse(&xylona.RevokeMinecraftMapShareResponse{}), nil
}

// GetPublicMinecraftMap resolves a public capability token without a session.
func (xs *XylonaService) GetPublicMinecraftMap(
	ctx context.Context,
	request *connect.Request[xylona.GetPublicMinecraftMapRequest],
) (*connect.Response[xylona.GetPublicMinecraftMapResponse], error) {
	token := strings.TrimSpace(request.Msg.GetShareToken())
	settings, errSettings := xs.db.GetGameServerMinecraftMapByShareToken(token)
	if errors.Is(errSettings, db.ErrMinecraftMapShareNotFound) || (errSettings == nil && !settings.Enabled) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("public map link is invalid or has been revoked"))
	}
	if errSettings != nil {
		log.Error().Err(errSettings).Msg("Failed to resolve public Minecraft map share")
		return nil, internalErr()
	}
	gameServer, errServer := xs.minecraftMapServer(settings.GameServerID)
	if errServer != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("public map link is invalid or has been revoked"))
	}
	view, errBuild := xs.buildMinecraftMapView(ctx, gameServer, false, true)
	if errBuild != nil {
		return nil, errBuild
	}
	response := connect.NewResponse(&xylona.GetPublicMinecraftMapResponse{Map: view})
	errCookie := xs.setMinecraftMapShareCookie(response.Header(), settings)
	if errCookie != nil {
		log.Error().Err(errCookie).Str("game_server_id", gameServer.ID).Msg("Failed to establish public Minecraft map viewer session")
		return nil, internalErr()
	}
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Referrer-Policy", "no-referrer")
	return response, nil
}

func (xs *XylonaService) buildMinecraftMapView(
	ctx context.Context,
	gameServer *models.GameServer,
	canManage bool,
	public bool,
) (*xylona.MinecraftMapView, error) {
	settings, errSettings := xs.db.GetGameServerMinecraftMap(gameServer.ID)
	if errSettings != nil {
		log.Error().Err(errSettings).Str("game_server_id", gameServer.ID).Msg("Failed to load Minecraft map settings")
		return nil, internalErr()
	}
	view := &xylona.MinecraftMapView{
		GameServerId:            gameServer.ID,
		GameServerName:          gameServer.Name,
		Enabled:                 settings.Enabled,
		CanManage:               canManage,
		ShareEnabled:            settings.ShareTokenHash.Valid,
		WorldName:               settings.WorldName,
		BluemapDownloadAccepted: settings.AcceptedAt.Valid,
		Status:                  "disabled",
		StatusMessage:           "Enable the live map to begin rendering this world.",
	}
	if !settings.Enabled {
		return view, nil
	}
	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		view.Status = "node_unavailable"
		view.StatusMessage = "The server's node is currently unavailable."
		return view, nil //nolint:nilerr // Node availability is intentionally represented in the map view.
	}
	caps, errCaps := client.GetRuntimeCapabilities(ctx)
	if errCaps != nil || caps.ProtocolVersion < minecraftMapNodeProtocol || !caps.MinecraftMap {
		view.Status = "node_upgrade_required"
		view.StatusMessage = "Update this node to use Minecraft live maps."
		return view, nil //nolint:nilerr // Capability availability is intentionally represented in the map view.
	}
	status, errEnsure := client.EnsureMinecraftMap(ctx, node.MinecraftMapEnsureRequest{
		ProcessID:        gameServer.ID,
		WorkingDirectory: gameServer.Directory,
		WorldName:        settings.WorldName,
		JavaExecutable:   xs.minecraftJavaExecutable(gameServer),
		MinecraftVersion: xs.minecraftInstalledVersion(gameServer),
	})
	if errEnsure != nil {
		log.Warn().Err(errEnsure).Str("game_server_id", gameServer.ID).Msg("Minecraft map is unavailable")
		view.Status = "unavailable"
		view.StatusMessage = "BlueMap could not be started. Check the node logs and retry."
		return view, nil
	}
	currentSettings, errCurrentSettings := xs.db.GetGameServerMinecraftMap(gameServer.ID)
	if errCurrentSettings != nil {
		log.Error().Err(errCurrentSettings).Str("game_server_id", gameServer.ID).Msg("Failed to recheck Minecraft map settings")
		return nil, internalErr()
	}
	if !currentSettings.Enabled {
		errStop := client.StopMinecraftMap(ctx, gameServer.ID)
		if errStop != nil {
			log.Warn().Err(errStop).Str("game_server_id", gameServer.ID).Msg("Failed to stop Minecraft map after concurrent disable")
		}
		view.Enabled = false
		view.Available = false
		view.Status = "disabled"
		view.StatusMessage = "Enable the live map to begin rendering this world."
		return view, nil
	}
	view.Available = status.Ready
	view.Provider = status.Provider
	view.Status = status.Status
	view.StatusMessage = status.StatusMessage
	view.BluemapVersion = status.BlueMapVersion
	view.LivePlayersAvailable = status.LivePlayersAvailable
	if status.Ready {
		if public {
			view.ViewerUrl = MinecraftMapSharedPathPrefix + "/" + gameServer.ID + "/"
		} else {
			view.ViewerUrl = MinecraftMapViewerPathPrefix + "/" + gameServer.ID + "/"
		}
	}
	return view, nil
}

func (xs *XylonaService) setMinecraftMapShareCookie(header http.Header, settings *db.GameServerMinecraftMap) error {
	if xs.secureCookie == nil {
		return errors.New("secure cookie codec is unavailable")
	}
	if settings == nil || !settings.ShareTokenHash.Valid {
		return errors.New("minecraft map share is unavailable")
	}
	expiresAt := time.Now().UTC().Add(minecraftMapShareGrantTTL)
	grant := minecraftMapShareGrant{
		GameServerID:   settings.GameServerID,
		ShareTokenHash: settings.ShareTokenHash.String,
		ExpiresAt:      expiresAt.Unix(),
	}
	encoded, errEncode := xs.secureCookie.Encode(minecraftMapShareGrantName, grant)
	if errEncode != nil {
		return fmt.Errorf("encode Minecraft map share grant: %w", errEncode)
	}
	cookie := &http.Cookie{
		Name:     minecraftMapShareCookieName,
		Value:    encoded,
		Path:     MinecraftMapSharedPathPrefix + "/" + settings.GameServerID + "/",
		Expires:  expiresAt,
		MaxAge:   int(minecraftMapShareGrantTTL.Seconds()),
		HttpOnly: true,
		Secure:   xs.secureCookies,
		SameSite: http.SameSiteStrictMode,
	}
	header.Add("Set-Cookie", cookie.String())
	return nil
}

func (xs *XylonaService) decodeMinecraftMapShareGrant(request *http.Request) (*minecraftMapShareGrant, error) {
	if xs.secureCookie == nil {
		return nil, errors.New("secure cookie codec is unavailable")
	}
	cookie, errCookie := request.Cookie(minecraftMapShareCookieName)
	if errCookie != nil {
		return nil, errors.New("map share viewer session is missing")
	}
	var grant minecraftMapShareGrant
	errDecode := xs.secureCookie.Decode(minecraftMapShareGrantName, cookie.Value, &grant)
	if errDecode != nil {
		return nil, errors.New("map share viewer session is invalid")
	}
	if grant.ExpiresAt <= time.Now().UTC().Unix() || strings.TrimSpace(grant.GameServerID) == "" || strings.TrimSpace(grant.ShareTokenHash) == "" {
		return nil, errors.New("map share viewer session has expired")
	}
	return &grant, nil
}

func (xs *XylonaService) minecraftMapServer(gameServerID string) (*models.GameServer, error) {
	gameServerID = strings.TrimSpace(gameServerID)
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}
	gameServer, errLookup := xs.db.GetGameServerByID(gameServerID)
	if errLookup != nil {
		return nil, dbLookup(errLookup)
	}
	if gameServer.GameID != minecraftGameID {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("live map is available only for Minecraft servers"))
	}
	return gameServer, nil
}

func (xs *XylonaService) minecraftJavaExecutable(gameServer *models.GameServer) string {
	if gameServer == nil || gameServer.R.Game == nil {
		return "java"
	}
	if xs.resolveNodeGOOS(gameServer.NodeID) == "windows" {
		return strings.TrimSpace(gameServer.R.Game.WindowsBaseCommand)
	}
	return strings.TrimSpace(gameServer.R.Game.LinuxBaseCommand)
}

func (xs *XylonaService) minecraftInstalledVersion(gameServer *models.GameServer) string {
	if xs.versionState != nil {
		state, found := xs.versionState.GetWithOK(gameServer.ID)
		if found && strings.TrimSpace(state.InstalledVersion) != "" {
			return state.InstalledVersion
		}
	}
	return gameServer.Version
}

func validMinecraftWorldName(worldName string) bool {
	worldName = strings.TrimSpace(worldName)
	return worldName != "." && worldName != ".." && minecraftWorldNamePattern.MatchString(worldName) && filepath.Base(worldName) == worldName
}

// MinecraftMapAsset serves authenticated and shared BlueMap web assets through
// the controller so nodes never expose another public port.
func (xs *XylonaService) MinecraftMapAsset(response http.ResponseWriter, request *http.Request) {
	if xs.minecraftMapAssetSlots != nil {
		select {
		case xs.minecraftMapAssetSlots <- struct{}{}:
			defer func() { <-xs.minecraftMapAssetSlots }()
		case <-request.Context().Done():
			return
		default:
			http.Error(response, "map is busy", http.StatusServiceUnavailable)
			return
		}
	}
	gameServer, assetPath, errAccess := xs.authorizeMinecraftMapAsset(request)
	if errAccess != nil {
		http.NotFound(response, request)
		return
	}
	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		http.Error(response, "map unavailable", http.StatusServiceUnavailable)
		return
	}
	asset, errAsset := client.GetMinecraftMapAsset(request.Context(), node.MinecraftMapAssetRequest{
		ProcessID:        gameServer.ID,
		WorkingDirectory: gameServer.Directory,
		AssetPath:        assetPath,
	})
	if errors.Is(errAsset, fs.ErrNotExist) || errors.Is(errAsset, node.ErrInvalidPath) {
		http.NotFound(response, request)
		return
	}
	if errAsset != nil {
		log.Debug().Err(errAsset).Str("game_server_id", gameServer.ID).Str("asset_path", assetPath).Msg("Minecraft map asset unavailable")
		http.Error(response, "map unavailable", http.StatusServiceUnavailable)
		return
	}
	response.Header().Set("Content-Type", asset.ContentType)
	cacheControl := asset.CacheControl
	if strings.HasPrefix(filepath.ToSlash(assetPath), "maps/") {
		cacheControl = "private, no-store"
	} else if strings.TrimSpace(chi.URLParam(request, "gameServerId")) != "" && cacheControl != "no-store" {
		cacheControl = "private, max-age=300"
	}
	response.Header().Set("Cache-Control", cacheControl)
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.Header().Set("X-Frame-Options", "SAMEORIGIN")
	response.Header().Set("Content-Security-Policy", "sandbox allow-same-origin allow-scripts allow-forms allow-popups allow-downloads; default-src 'self'; base-uri 'self'; frame-ancestors 'self'; object-src 'none'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self' blob:; connect-src 'self'")
	response.Header().Set("Referrer-Policy", "no-referrer")
	if asset.ContentEncoding != "" {
		response.Header().Set("Content-Encoding", asset.ContentEncoding)
	}
	response.WriteHeader(http.StatusOK)
	_, errWrite := response.Write(asset.Content)
	if errWrite != nil {
		log.Debug().Err(errWrite).Str("game_server_id", gameServer.ID).Msg("Minecraft map response closed early")
	}
}

func (xs *XylonaService) authorizeMinecraftMapAsset(request *http.Request) (*models.GameServer, string, error) {
	gameServerID := strings.TrimSpace(chi.URLParam(request, "gameServerId"))
	assetPath := strings.TrimPrefix(chi.URLParam(request, "*"), "/")
	gameServer, errServer := xs.minecraftMapServer(gameServerID)
	if errServer != nil {
		return nil, "", errServer
	}
	settings, errSettings := xs.db.GetGameServerMinecraftMap(gameServer.ID)
	if errSettings != nil || !settings.Enabled {
		return nil, "", errors.New("map is disabled")
	}
	if strings.HasPrefix(request.URL.Path, MinecraftMapSharedPathPrefix+"/") {
		grant, errGrant := xs.decodeMinecraftMapShareGrant(request)
		if errGrant != nil || grant.GameServerID != gameServer.ID || !settings.ShareTokenHash.Valid {
			return nil, "", errors.New("map share is invalid")
		}
		if subtle.ConstantTimeCompare([]byte(grant.ShareTokenHash), []byte(settings.ShareTokenHash.String)) != 1 {
			return nil, "", errors.New("map share has been revoked")
		}
		return gameServer, assetPath, nil
	}
	user, errUser := xs.getUserFromHeader(request.Header)
	if errUser != nil {
		return nil, "", errors.New("map viewer session is invalid")
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerView)
	if errPermission != nil {
		return nil, "", errPermission
	}
	return gameServer, assetPath, nil
}
