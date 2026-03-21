package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/steamcache"
)

func (xs *XylonaService) SearchSteamApps(_ context.Context, request *connect.Request[xylona.SearchSteamAppsRequest]) (*connect.Response[xylona.SearchSteamAppsResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	results := xs.steamCache.Search(request.Msg.GetQuery())
	apps := make([]*xylona.SteamApp, len(results))
	for i, app := range results {
		apps[i] = steamAppToProto(app)
	}

	resp := &connect.Response[xylona.SearchSteamAppsResponse]{
		Msg: &xylona.SearchSteamAppsResponse{
			Apps: apps,
		},
	}
	return resp, nil
}

func (xs *XylonaService) GetSteamAppDetails(ctx context.Context, request *connect.Request[xylona.GetSteamAppDetailsRequest]) (*connect.Response[xylona.GetSteamAppDetailsResponse], error) {
	_, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	appID := request.Msg.GetAppId()
	details, errFetch := xs.steamCache.FetchDetails(ctx, appID)
	if errFetch != nil {
		log.Warn().Err(errFetch).Str("app_id", appID).Msg("Failed to fetch Steam app details")

		// Fall back to cached app list for partial data.
		cached := xs.steamCache.FindByID(appID)
		if cached == nil {
			return &connect.Response[xylona.GetSteamAppDetailsResponse]{
				Msg: &xylona.GetSteamAppDetailsResponse{
					DetailsAvailable: false,
				},
			}, nil
		}

		return &connect.Response[xylona.GetSteamAppDetailsResponse]{
			Msg: &xylona.GetSteamAppDetailsResponse{
				Details: &xylona.SteamAppDetails{
					AppId: cached.AppID,
					Name:  cached.Name,
				},
				DetailsAvailable: false,
			},
		}, nil
	}

	resp := &connect.Response[xylona.GetSteamAppDetailsResponse]{
		Msg: &xylona.GetSteamAppDetailsResponse{
			Details:          steamAppDetailsToProto(details),
			DetailsAvailable: true,
		},
	}
	return resp, nil
}

func steamAppToProto(app steamcache.SteamApp) *xylona.SteamApp {
	return &xylona.SteamApp{
		AppId: app.AppID,
		Name:  app.Name,
	}
}

func steamAppDetailsToProto(details *steamcache.SteamAppDetails) *xylona.SteamAppDetails {
	launchConfigs := make([]*xylona.SteamLaunchConfig, len(details.LaunchConfigs))
	for i, lc := range details.LaunchConfigs {
		launchConfigs[i] = &xylona.SteamLaunchConfig{
			Executable:  lc.Executable,
			Arguments:   lc.Arguments,
			Os:          lc.OS,
			Description: lc.Description,
		}
	}

	return &xylona.SteamAppDetails{
		AppId:            details.AppID,
		Name:             details.Name,
		WindowsSupport:   details.WindowsSupport,
		LinuxSupport:     details.LinuxSupport,
		InstallDirectory: details.InstallDirectory,
		ParentAppId:      details.ParentAppID,
		LaunchConfigs:    launchConfigs,
	}
}
