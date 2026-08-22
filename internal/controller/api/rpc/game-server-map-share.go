package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

var errPublicGameServerMapUnavailable = errors.New("public game server map unavailable")

// GetOrCreateGameServerMapShareSettings returns the canonical map link settings,
// creating disabled settings on first use.
func (xs *XylonaService) GetOrCreateGameServerMapShareSettings(
	_ context.Context,
	request *connect.Request[xylona.GetOrCreateGameServerMapShareSettingsRequest],
) (*connect.Response[xylona.GetOrCreateGameServerMapShareSettingsResponse], error) {
	gameServer, errAccess := xs.manageableGameServerMap(request.Header(), request.Msg.GetGameServerId())
	if errAccess != nil {
		return nil, errAccess
	}
	share, errShare := xs.getOrCreateGameServerMapShare(gameServer.ID)
	if errShare != nil {
		return nil, internalErr()
	}
	return connect.NewResponse(&xylona.GetOrCreateGameServerMapShareSettingsResponse{
		Settings: publicGameServerMapShareSettings(share),
	}), nil
}

// UpdateGameServerMapShareSettings updates the canonical map link.
func (xs *XylonaService) UpdateGameServerMapShareSettings(
	_ context.Context,
	request *connect.Request[xylona.UpdateGameServerMapShareSettingsRequest],
) (*connect.Response[xylona.UpdateGameServerMapShareSettingsResponse], error) {
	gameServer, errAccess := xs.manageableGameServerMap(request.Header(), request.Msg.GetGameServerId())
	if errAccess != nil {
		return nil, errAccess
	}
	identifier := request.Msg.GetPublicIdentifier()
	errIdentifier := validateStatusPageIdentifier(identifier)
	if errIdentifier != nil {
		return nil, invalidArg("public_identifier: " + errIdentifier.Error())
	}
	if request.Msg.GetEnabled() {
		errEnable := xs.ensureGameServerMapShareCanEnable(gameServer)
		if errEnable != nil {
			return nil, errEnable
		}
	}
	_, errCreate := xs.getOrCreateGameServerMapShare(gameServer.ID)
	if errCreate != nil {
		return nil, internalErr()
	}
	share, errUpdate := xs.db.UpdateGameServerMapShare(gameServer.ID, identifier, request.Msg.GetEnabled())
	if errors.Is(errUpdate, db.ErrGameServerMapShareIdentifierConflict) {
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("public identifier is unavailable"))
	}
	if errors.Is(errUpdate, sql.ErrNoRows) {
		return nil, notFoundErr()
	}
	if errUpdate != nil {
		return nil, internalErr()
	}
	return connect.NewResponse(&xylona.UpdateGameServerMapShareSettingsResponse{
		Settings: publicGameServerMapShareSettings(share),
	}), nil
}

// ResolvePublicGameServerMap returns only the public map kind.
func (xs *XylonaService) ResolvePublicGameServerMap(
	_ context.Context,
	request *connect.Request[xylona.ResolvePublicGameServerMapRequest],
) (*connect.Response[xylona.ResolvePublicGameServerMapResponse], error) {
	kind, errResolve := xs.resolvePublicGameServerMapKind(request.Msg.GetPublicIdentifier())
	if errors.Is(errResolve, errPublicGameServerMapUnavailable) {
		return nil, publicGameServerMapNotFound()
	}
	if errResolve != nil {
		return nil, internalErr()
	}
	response := connect.NewResponse(&xylona.ResolvePublicGameServerMapResponse{Kind: kind})
	setPublicGameServerMapHeaders(response.Header())
	return response, nil
}

func (xs *XylonaService) manageableGameServerMap(header http.Header, gameServerID string) (*models.GameServer, error) {
	user, errUser := xs.getUserFromHeader(header)
	if errUser != nil {
		return nil, unauthenticated()
	}
	gameServerID = strings.TrimSpace(gameServerID)
	if gameServerID == "" {
		return nil, invalidArg("game_server_id is required")
	}
	gameServer, errServer := xs.db.GetGameServerByID(gameServerID)
	if errServer != nil {
		return nil, dbLookup(errServer)
	}
	_, supported := gameServerMapKind(gameServer.GameID)
	if !supported {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("this game does not support a public map"))
	}
	errPermission := xs.ensureLocalServerPermission(user, gameServer, permissionGameServerSettings)
	if errPermission != nil {
		return nil, errPermission
	}
	return gameServer, nil
}

func (xs *XylonaService) getOrCreateGameServerMapShare(gameServerID string) (*db.GameServerMapShare, error) {
	identifierGenerator := xs.statusPageIdentifier
	if identifierGenerator == nil {
		identifierGenerator = newStatusPageIdentifier
	}
	for range 10 {
		identifier, errIdentifier := identifierGenerator()
		if errIdentifier != nil {
			return nil, fmt.Errorf("generate game server map identifier: %w", errIdentifier)
		}
		share, errCreate := xs.db.GetOrCreateGameServerMapShare(gameServerID, identifier)
		if errors.Is(errCreate, db.ErrGameServerMapShareIdentifierConflict) {
			continue
		}
		if errCreate != nil {
			return nil, fmt.Errorf("get or create game server map share: %w", errCreate)
		}
		return share, nil
	}
	return nil, errors.New("could not allocate a game server map identifier")
}

func (xs *XylonaService) ensureGameServerMapShareCanEnable(gameServer *models.GameServer) error {
	if gameServer.GameID != minecraftGameID {
		return nil
	}
	settings, errSettings := xs.db.GetGameServerMinecraftMap(gameServer.ID)
	if errSettings != nil {
		return internalErr()
	}
	if !settings.Enabled {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("enable the Minecraft map before sharing it"))
	}
	return nil
}

func (xs *XylonaService) resolvePublicGameServerMapKind(identifier string) (xylona.GameServerMapKind, error) {
	_, _, kind, errResolve := xs.resolvePublicGameServerMapDetails(identifier)
	return kind, errResolve
}

func (xs *XylonaService) resolvePublicGameServerMapDetails(
	identifier string,
) (*db.GameServerMapShare, *models.GameServer, xylona.GameServerMapKind, error) {
	errIdentifier := validateStatusPageIdentifier(identifier)
	if errIdentifier != nil {
		return nil, nil, xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_UNSPECIFIED, errPublicGameServerMapUnavailable
	}
	share, errShare := xs.db.GetEnabledGameServerMapShareByIdentifier(identifier)
	if errors.Is(errShare, sql.ErrNoRows) {
		return nil, nil, xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_UNSPECIFIED, errPublicGameServerMapUnavailable
	}
	if errShare != nil {
		return nil, nil, xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_UNSPECIFIED, fmt.Errorf("get enabled game server map share: %w", errShare)
	}
	gameServer, errServer := xs.db.GetGameServerByID(share.GameServerID)
	if errors.Is(errServer, sql.ErrNoRows) {
		return nil, nil, xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_UNSPECIFIED, errPublicGameServerMapUnavailable
	}
	if errServer != nil {
		return nil, nil, xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_UNSPECIFIED, fmt.Errorf("get game server for map share: %w", errServer)
	}
	kind, supported := gameServerMapKind(gameServer.GameID)
	if !supported {
		return nil, nil, xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_UNSPECIFIED, errPublicGameServerMapUnavailable
	}
	if kind == xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_MINECRAFT {
		settings, errSettings := xs.db.GetGameServerMinecraftMap(gameServer.ID)
		if errSettings != nil {
			return nil, nil, xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_UNSPECIFIED, fmt.Errorf("get Minecraft map settings: %w", errSettings)
		}
		if !settings.Enabled {
			return nil, nil, xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_UNSPECIFIED, errPublicGameServerMapUnavailable
		}
	}
	return share, gameServer, kind, nil
}

func gameServerMapKind(gameID string) (xylona.GameServerMapKind, bool) {
	switch gameID {
	case palworldGameID:
		return xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_PALWORLD, true
	case sevenDaysToDieGameID:
		return xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_SEVEN_DAYS_TO_DIE, true
	case minecraftGameID:
		return xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_MINECRAFT, true
	default:
		return xylona.GameServerMapKind_GAME_SERVER_MAP_KIND_UNSPECIFIED, false
	}
}

func publicGameServerMapShareSettings(share *db.GameServerMapShare) *xylona.GameServerMapShareSettings {
	return &xylona.GameServerMapShareSettings{
		GameServerId:     share.GameServerID,
		PublicIdentifier: share.PublicIdentifier,
		Enabled:          share.Enabled,
		PublicPath:       "/maps/" + share.PublicIdentifier,
	}
}

func publicGameServerMapNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("public map is unavailable"))
}

func setPublicGameServerMapHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Robots-Tag", "noindex, nofollow")
}
