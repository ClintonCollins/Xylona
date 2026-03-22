package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/modmanager"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetServerSoftwareOptions returns the available server software options for a game.
func (xs *XylonaService) GetServerSoftwareOptions(
	_ context.Context,
	request *connect.Request[xylona.GetServerSoftwareOptionsRequest],
) (*connect.Response[xylona.GetServerSoftwareOptionsResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	game, errGame := xs.db.GetGameByID(request.Msg.GetGameId())
	if errGame != nil {
		if errors.Is(errGame, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("game not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get game"))
	}

	softwareJSON := game.ServerSoftware.GetOr("")
	allSoftware, errParse := modmanager.ParseServerSoftware(softwareJSON)
	if errParse != nil {
		log.Error().Err(errParse).Str("game_id", game.ID).Msg("Failed to parse server software JSON")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to parse server software config"))
	}

	var options []*xylona.ServerSoftwareOption
	for _, sw := range allSoftware {
		options = append(options, &xylona.ServerSoftwareOption{
			Id:            sw.ID,
			Name:          sw.Name,
			JarSource:     sw.JarSource,
			HasModSupport: sw.ModConfig != nil,
		})
	}

	return &connect.Response[xylona.GetServerSoftwareOptionsResponse]{
		Msg: &xylona.GetServerSoftwareOptionsResponse{
			Options: options,
		},
	}, nil
}

// GetServerSoftwareVersions returns available versions for a server software option.
func (xs *XylonaService) GetServerSoftwareVersions(
	ctx context.Context,
	request *connect.Request[xylona.GetServerSoftwareVersionsRequest],
) (*connect.Response[xylona.GetServerSoftwareVersionsResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	game, errGame := xs.db.GetGameByID(request.Msg.GetGameId())
	if errGame != nil {
		if errors.Is(errGame, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("game not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get game"))
	}

	softwareJSON := game.ServerSoftware.GetOr("")
	allSoftware, errParse := modmanager.ParseServerSoftware(softwareJSON)
	if errParse != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to parse server software config"))
	}

	sw, found := modmanager.GetSoftwareByID(allSoftware, request.Msg.GetSoftwareId())
	if !found {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("software option not found: %s", request.Msg.GetSoftwareId()))
	}

	if sw.JarSource == "" {
		return &connect.Response[xylona.GetServerSoftwareVersionsResponse]{
			Msg: &xylona.GetServerSoftwareVersionsResponse{},
		}, nil
	}

	// Use the jar source as the provider ID to get available game versions.
	provider, ok := modproviders.GetProvider(sw.JarSource)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("jar source provider not found: %s", sw.JarSource))
	}

	// For jar source providers like PaperMC, GetModDetails returns the project
	// with its available game versions (e.g., 1.21.4, 1.21.3). We return these
	// as SoftwareVersion entries — each represents a game version the user can
	// select, not individual builds.
	details, errDetails := provider.GetModDetails(ctx, sw.ID, nil)
	if errDetails != nil {
		log.Error().Err(errDetails).Str("jar_source", sw.JarSource).Msg("Failed to get software versions")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get software versions"))
	}

	var protoVersions []*xylona.SoftwareVersion
	// The versions in ModDetails come from the project's version list.
	// For PaperMC this is game versions; for other providers it may differ.
	if details != nil {
		for _, v := range details.Versions {
			protoVersions = append(protoVersions, &xylona.SoftwareVersion{
				VersionId:     v.VersionID,
				VersionString: v.VersionString,
			})
		}
	}

	return &connect.Response[xylona.GetServerSoftwareVersionsResponse]{
		Msg: &xylona.GetServerSoftwareVersionsResponse{
			Versions: protoVersions,
		},
	}, nil
}

// SetServerSoftware sets the active server software for a game server.
func (xs *XylonaService) SetServerSoftware(
	_ context.Context,
	request *connect.Request[xylona.SetServerSoftwareRequest],
) (*connect.Response[xylona.SetServerSoftwareResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	gameServer, errGetServer := xs.getGameServerFromID(request.Msg.GetGameServerId())
	if errGetServer != nil {
		return nil, errGetServer
	}

	errPerm := xs.ensureLocalServerPermission(user, gameServer, PermissionGameServerMods)
	if errPerm != nil {
		return nil, errPerm
	}

	softwareID := request.Msg.GetSoftwareId()

	// Store only the software ID (e.g., "paper", "fabric", "vanilla").
	// The version is used for the JAR download but doesn't need to persist
	// separately — it's a property of the installed server files.
	setter := &models.GameServerSetter{
		ID:             omit.From(gameServer.ID),
		ServerSoftware: omitnull.From(softwareID),
	}

	updated, errUpdate := xs.db.UpdateGameServer(xs.db.DB, setter)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Msg("Failed to update game server software")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update server software"))
	}

	return &connect.Response[xylona.SetServerSoftwareResponse]{
		Msg: &xylona.SetServerSoftwareResponse{
			GameServer: helpers.GameServerModelToProto(updated),
		},
	}, nil
}
