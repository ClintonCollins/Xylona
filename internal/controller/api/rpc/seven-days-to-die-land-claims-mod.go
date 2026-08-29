package rpc

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/sevendaystodiemod"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// InstallSevenDaysToDieLandClaimsMod installs or repairs the native land-claim WebAPI helper.
func (xs *XylonaService) InstallSevenDaysToDieLandClaimsMod(
	ctx context.Context,
	request *connect.Request[xylona.InstallSevenDaysToDieLandClaimsModRequest],
) (*connect.Response[xylona.InstallSevenDaysToDieLandClaimsModResponse], error) {
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

	errContext := ctx.Err()
	if errContext != nil {
		return nil, connect.NewError(contextConnectCode(errContext), fmt.Errorf("install land claim helper: %w", errContext))
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}

	policy := xs.buildFileMutationProtectionPolicy(user, gameServer)
	errInstall := sevendaystodiemod.Install(ctx, client, gameServer, policy)
	if errors.Is(errInstall, sevendaystodiemod.ErrAssetsUnavailable) {
		log.Error().Err(errInstall).Str("game_server_id", gameServer.ID).Msg("Embedded 7 Days to Die land claim helper is unavailable")
		return nil, internalErrf("land claim helper assets are unavailable")
	}
	if errInstall != nil {
		errContext = ctx.Err()
		if errContext != nil {
			return nil, connect.NewError(contextConnectCode(errContext), fmt.Errorf("install land claim helper: %w", errContext))
		}
		log.Error().Err(errInstall).Str("game_server_id", gameServer.ID).Msg("Failed to install 7 Days to Die land claim helper")
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("could not install the land claim helper"))
	}

	return connect.NewResponse(&xylona.InstallSevenDaysToDieLandClaimsModResponse{}), nil
}
