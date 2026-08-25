package rpc

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const (
	sevenDaysToDieLandClaimsModDirectory = "Mods/Xylona_LandClaims"
	sevenDaysToDieWebServerDLLPath       = "Mods/TFP_WebServer/WebServer.dll"
)

//go:embed seven-days-to-die-land-claims/ModInfo.xml
var sevenDaysToDieLandClaimsModInfo []byte

//go:embed seven-days-to-die-land-claims/v2.6/XylonaLandClaims.dll
var sevenDaysToDieLandClaimsV26DLL []byte

//go:embed seven-days-to-die-land-claims/v3/XylonaLandClaims.dll
var sevenDaysToDieLandClaimsV3DLL []byte

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

	runtimeStatus := xs.getLocalGameServerStatus(ctx, gameServer)
	errContext := ctx.Err()
	if errContext != nil {
		return nil, connect.NewError(contextConnectCode(errContext), fmt.Errorf("install land claim helper: %w", errContext))
	}
	if runtimeStatus != xylona.Status_OFFLINE {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("server must be stopped before installing the land claim helper"))
	}

	client, errClient := xs.resolveNodeClient(gameServer)
	if errClient != nil {
		return nil, errClient
	}

	dll := sevenDaysToDieLandClaimsV26DLL
	_, errStat := client.StatFile(ctx, gameServer.Directory, sevenDaysToDieWebServerDLLPath)
	if errors.Is(errStat, os.ErrNotExist) {
		dll = sevenDaysToDieLandClaimsV3DLL
	} else if errStat != nil {
		log.Error().Err(errStat).Str("game_server_id", gameServer.ID).Msg("Failed to detect 7 Days to Die WebServer version")
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("could not determine the installed 7 Days to Die version"))
	}
	if len(sevenDaysToDieLandClaimsModInfo) == 0 || len(dll) == 0 {
		log.Error().Str("game_server_id", gameServer.ID).Msg("Embedded 7 Days to Die land claim helper is empty")
		return nil, internalErrf("land claim helper assets are unavailable")
	}

	policy := xs.buildProtectionPolicy(gameServer)
	errCreate := client.CreateFileOrDirectory(ctx, gameServer.Directory, sevenDaysToDieLandClaimsModDirectory, "", true, policy)
	if errCreate != nil {
		log.Error().Err(errCreate).Str("game_server_id", gameServer.ID).Msg("Failed to create 7 Days to Die land claim helper directory")
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("could not create the land claim helper directory"))
	}
	errWriteDLL := client.WriteFile(
		ctx,
		gameServer.Directory,
		sevenDaysToDieLandClaimsModDirectory+"/XylonaLandClaims.dll",
		dll,
		policy,
	)
	if errWriteDLL != nil {
		log.Error().Err(errWriteDLL).Str("game_server_id", gameServer.ID).Msg("Failed to write 7 Days to Die land claim helper DLL")
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("could not install the land claim helper"))
	}
	errWriteModInfo := client.WriteFile(
		ctx,
		gameServer.Directory,
		sevenDaysToDieLandClaimsModDirectory+"/ModInfo.xml",
		sevenDaysToDieLandClaimsModInfo,
		policy,
	)
	if errWriteModInfo != nil {
		log.Error().Err(errWriteModInfo).Str("game_server_id", gameServer.ID).Msg("Failed to write 7 Days to Die land claim helper metadata")
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("could not install the land claim helper"))
	}

	return connect.NewResponse(&xylona.InstallSevenDaysToDieLandClaimsModResponse{}), nil
}
