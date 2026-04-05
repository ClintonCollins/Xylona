package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// SearchSteamApps returns an empty result because Steam app search is no longer supported.
func (xs *XylonaService) SearchSteamApps(_ context.Context, request *connect.Request[xylona.SearchSteamAppsRequest]) (*connect.Response[xylona.SearchSteamAppsResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	// Search is no longer supported — the Steam APIs that provided app list
	// search have been removed. Users enter AppIDs directly and we look them
	// up via GetSteamAppDetails instead.
	return &connect.Response[xylona.SearchSteamAppsResponse]{
		Msg: &xylona.SearchSteamAppsResponse{},
	}, nil
}

// GetSteamAppDetails returns cached Steam metadata for the requested AppID.
func (xs *XylonaService) GetSteamAppDetails(ctx context.Context, request *connect.Request[xylona.GetSteamAppDetailsRequest]) (*connect.Response[xylona.GetSteamAppDetailsResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	appID := request.Msg.GetAppId()
	details, errFetch := xs.steamCache.FetchDetails(ctx, appID)
	if errFetch != nil {
		log.Warn().Err(errFetch).Str("app_id", appID).Msg("Failed to fetch Steam app details")
		return &connect.Response[xylona.GetSteamAppDetailsResponse]{
			Msg: &xylona.GetSteamAppDetailsResponse{
				DetailsAvailable: false,
			},
		}, nil
	}

	launchConfigs := make([]*xylona.SteamLaunchConfig, len(details.LaunchConfigs))
	for i, lc := range details.LaunchConfigs {
		launchConfigs[i] = &xylona.SteamLaunchConfig{
			Executable:  lc.Executable,
			Arguments:   lc.Arguments,
			Os:          lc.OS,
			Description: lc.Description,
		}
	}

	return &connect.Response[xylona.GetSteamAppDetailsResponse]{
		Msg: &xylona.GetSteamAppDetailsResponse{
			Details: &xylona.SteamAppDetails{
				AppId:            details.AppID,
				Name:             details.Name,
				WindowsSupport:   details.WindowsSupport,
				LinuxSupport:     details.LinuxSupport,
				InstallDirectory: details.InstallDirectory,
				ParentAppId:      details.ParentAppID,
				LaunchConfigs:    launchConfigs,
			},
			DetailsAvailable: true,
		},
	}, nil
}
