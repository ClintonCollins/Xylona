package actions

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"

	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/pkg/query"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (inst *Instance) backgroundJobQueryAllGameServers() {
	throttle := time.NewTicker(time.Second * 5)
	defer throttle.Stop()

	for {
		select {
		case <-inst.ctx.Done():
			return
		case <-throttle.C:
			runBackgroundTask("backgroundJobQueryAllGameServers", "tick", nil, func() {
				gameServers, err := inst.db.GetAllGameServers()
				if err != nil {
					log.Error().Err(err).Msg("Failed to get game servers")
					return
				}
				inst.queryGameServers(inst.ctx, gameServers)
			})
		}
	}
}

func getQueryInfoType(game *models.Game) xylona.ServerQuery_Type {
	if game.ID == "minecraft" {
		return xylona.ServerQuery_Minecraft
	}
	if game.ID == palworldGameID {
		return xylona.ServerQuery_Palworld
	}
	if game.UsesSourceQuery {
		return xylona.ServerQuery_Source
	}
	return xylona.ServerQuery_Unknown
}

func gameServerQueryPort(gameServer *models.GameServer) int64 {
	if gameServer != nil && gameServer.GameID == "7_days_to_die" {
		return gameServer.Port
	}
	if gameServer == nil {
		return 0
	}
	return gameServer.QueryPort
}

func defaultServerQuery(gs *models.GameServer, queryType xylona.ServerQuery_Type) *xylona.ServerQuery {
	out := &xylona.ServerQuery{
		ServerId:   gs.ID,
		ServerName: gs.Name,
		Type:       queryType,
	}
	switch queryType {
	case xylona.ServerQuery_Minecraft:
		out.Minecraft = &xylona.MinecraftQueryInfo{MaxPlayers: helpers.ClampUint32FromInt64(gs.MaxPlayers)}
	case xylona.ServerQuery_Source:
		out.Source = &xylona.SourceQueryInfo{MaxPlayers: helpers.ClampUint32FromInt64(gs.MaxPlayers)}
	case xylona.ServerQuery_Palworld:
		out.Palworld = &xylona.PalworldQueryInfo{MaxPlayers: helpers.ClampUint32FromInt64(gs.MaxPlayers)}
	}
	return out
}

func nodeQueryKind(queryType xylona.ServerQuery_Type) node.GameServerQueryKind {
	switch queryType {
	case xylona.ServerQuery_Minecraft:
		return node.GameServerQueryKindMinecraft
	case xylona.ServerQuery_Source:
		return node.GameServerQueryKindSource
	case xylona.ServerQuery_Palworld:
		return node.GameServerQueryKindPalworld
	default:
		return node.GameServerQueryKindUnknown
	}
}

func queryResultComplete(result *xylona.ServerQuery) bool {
	if result == nil {
		return false
	}
	switch result.GetType() {
	case xylona.ServerQuery_Minecraft:
		return result.GetMinecraft() != nil
	case xylona.ServerQuery_Source:
		return result.GetSource() != nil
	case xylona.ServerQuery_Palworld:
		return result.GetPalworld() != nil
	default:
		return false
	}
}

func serverQueryFromNodeResult(gs *models.GameServer, result node.GameServerQueryResult) *xylona.ServerQuery {
	out := &xylona.ServerQuery{
		ServerId:   gs.ID,
		ServerName: gs.Name,
		Type:       xylona.ServerQuery_Unknown,
	}
	switch result.Kind {
	case node.GameServerQueryKindMinecraft:
		out.Type = xylona.ServerQuery_Minecraft
		if result.Minecraft != nil {
			out.Minecraft = &xylona.MinecraftQueryInfo{
				Motd:            result.Minecraft.MOTD,
				GameType:        result.Minecraft.GameType,
				Map:             result.Minecraft.Map,
				NumberOfPlayers: result.Minecraft.NumberOfPlayers,
				MaxPlayers:      result.Minecraft.MaxPlayers,
				PlayerList:      append([]string(nil), result.Minecraft.PlayerList...),
				ProtocolVersion: result.Minecraft.ProtocolVersion,
				ServerVersion:   result.Minecraft.ServerVersion,
			}
		}
	case node.GameServerQueryKindSource:
		out.Type = xylona.ServerQuery_Source
		if result.Source != nil {
			out.Source = &xylona.SourceQueryInfo{
				Name:                result.Source.Name,
				Map:                 result.Source.Map,
				Game:                result.Source.Game,
				AppId:               result.Source.AppID,
				SteamId:             result.Source.SteamID,
				GameId:              result.Source.GameID,
				Players:             result.Source.Players,
				MaxPlayers:          result.Source.MaxPlayers,
				Bots:                result.Source.Bots,
				ServerOs:            result.Source.ServerOS,
				Visibility:          result.Source.Visibility,
				Vac:                 result.Source.VAC,
				Version:             result.Source.Version,
				Protocol:            result.Source.Protocol,
				PlayerList:          append([]string(nil), result.Source.PlayerList...),
				PlayerListSupported: result.Source.PlayerListSupported,
			}
		}
	case node.GameServerQueryKindPalworld:
		out.Type = xylona.ServerQuery_Palworld
		if result.Palworld != nil {
			out.Palworld = &xylona.PalworldQueryInfo{
				Name:              result.Palworld.Name,
				Description:       result.Palworld.Description,
				Version:           result.Palworld.Version,
				WorldGuid:         result.Palworld.WorldGUID,
				Players:           result.Palworld.Players,
				MaxPlayers:        result.Palworld.MaxPlayers,
				PlayerList:        append([]string(nil), result.Palworld.PlayerList...),
				UptimeSeconds:     result.Palworld.UptimeSeconds,
				ServerFps:         result.Palworld.ServerFPS,
				ServerFrameTimeMs: result.Palworld.ServerFrameTimeMS,
				Days:              result.Palworld.Days,
				Responded:         result.Palworld.Responded,
			}
		}
	}
	return out
}

func (inst *Instance) storeServerQuery(result *xylona.ServerQuery) {
	inst.serverQueriesMutex.Lock()
	inst.serverQueriesInfoMap[result.GetServerId()] = result
	inst.serverQueriesMutex.Unlock()
}

func (inst *Instance) queryRemoteGameServer(ctx context.Context, gs *models.GameServer, queryType xylona.ServerQuery_Type) (*xylona.ServerQuery, error) {
	client, errClient := inst.resolveNodeClient(gs.NodeID)
	if errClient != nil {
		log.Debug().Err(errClient).Str("server", gs.Name).Str("node_id", gs.NodeID).Msg("Failed to resolve node client for game server query")
		return nil, fmt.Errorf("resolve game server query node client: %w", errClient)
	}
	queryRequest := node.GameServerQueryRequest{
		Kind:       nodeQueryKind(queryType),
		IP:         gs.IP,
		QueryPort:  gameServerQueryPort(gs),
		MaxPlayers: gs.MaxPlayers,
	}
	if queryType == xylona.ServerQuery_Palworld {
		username, password, errCredentials := inst.palworldQueryCredentials(gs)
		if errCredentials != nil {
			log.Debug().Err(errCredentials).Str("server", gs.Name).Msg("Failed to load Palworld query credentials")
			return nil, fmt.Errorf("load Palworld query credentials: %w", errCredentials)
		}
		queryRequest.Username = username
		queryRequest.Password = password
	}
	result, errQuery := client.QueryGameServer(ctx, queryRequest)
	if errQuery != nil {
		log.Debug().Err(errQuery).Str("server", gs.Name).Str("node_id", gs.NodeID).Msg("Failed to query game server through node")
		return nil, fmt.Errorf("query game server through node: %w", errQuery)
	}
	serverQuery := serverQueryFromNodeResult(gs, result)
	if !queryResultComplete(serverQuery) {
		return nil, fmt.Errorf("remote game server returned incomplete %s query", queryType.String())
	}
	return serverQuery, nil
}

func (inst *Instance) queryGameServers(ctx context.Context, gameServers []*models.GameServer) {
	errGroup, _ := errgroup.WithContext(ctx)
	errGroup.SetLimit(30)
	for _, gameServer := range gameServers {
		gs := gameServer
		errGroup.Go(func() error {
			runBackgroundTask("backgroundJobQueryAllGameServers", "query-server", map[string]string{
				"game_server_id": gs.ID,
			}, func() {
				gs.Status = inst.currentProcessStatus(gs).String()
				if gs.Status == "" {
					gs.Status = xylona.Status_OFFLINE.String()
				}
				queryType := getQueryInfoType(gs.R.Game)
				switch queryType {
				case xylona.ServerQuery_Unknown:
					inst.recordUnsupportedGameServerQuery(gs.ID)
					return
				case xylona.ServerQuery_Minecraft:
					if gs.Status != xylona.Status_ONLINE.String() {
						inst.recordUnavailableGameServerQuery(gs.ID, queryType)
						inst.storeServerQuery(defaultServerQuery(gs, queryType))
						return
					}
					startedAt := time.Now()
					if inst.isRemoteGameServer(gs) {
						serverQuery, errQuery := inst.queryRemoteGameServer(ctx, gs, queryType)
						if errQuery != nil {
							inst.recordFailedGameServerQuery(gs.ID, queryType, startedAt)
							inst.storeServerQuery(defaultServerQuery(gs, queryType))
							return
						}
						inst.recordSuccessfulGameServerQuery(gs.ID, queryType, startedAt, serverQuery)
						inst.storeServerQuery(serverQuery)
						return
					}
					info, err := query.Minecraft(gs.IP, int(gs.QueryPort))
					if err != nil {
						log.Debug().Err(err).Str("server", gs.Name).Msg("Failed to query minecraft server")
						inst.recordFailedGameServerQuery(gs.ID, queryType, startedAt)
						info = &xylona.MinecraftQueryInfo{MaxPlayers: helpers.ClampUint32FromInt64(gs.MaxPlayers)}
					} else {
						inst.recordSuccessfulGameServerQuery(gs.ID, queryType, startedAt, &xylona.ServerQuery{Type: queryType, Minecraft: info})
					}
					inst.storeServerQuery(&xylona.ServerQuery{
						ServerId:   gs.ID,
						ServerName: gs.Name,
						Type:       xylona.ServerQuery_Minecraft,
						Minecraft:  info,
					})
					return
				case xylona.ServerQuery_Source:
					if gs.Status != xylona.Status_ONLINE.String() {
						inst.recordUnavailableGameServerQuery(gs.ID, queryType)
						inst.storeServerQuery(defaultServerQuery(gs, queryType))
						return
					}
					startedAt := time.Now()
					if inst.isRemoteGameServer(gs) {
						serverQuery, errQuery := inst.queryRemoteGameServer(ctx, gs, queryType)
						if errQuery != nil {
							inst.recordFailedGameServerQuery(gs.ID, queryType, startedAt)
							inst.storeServerQuery(defaultServerQuery(gs, queryType))
							return
						}
						inst.recordSuccessfulGameServerQuery(gs.ID, queryType, startedAt, serverQuery)
						inst.storeServerQuery(serverQuery)
						return
					}
					info, err := query.Source(gs.IP, int(gameServerQueryPort(gs)))
					if err != nil {
						log.Debug().Err(err).Str("server", gs.Name).Msg("Failed to query source server")
						inst.recordFailedGameServerQuery(gs.ID, queryType, startedAt)
						info = &xylona.SourceQueryInfo{MaxPlayers: helpers.ClampUint32FromInt64(gs.MaxPlayers)}
					} else {
						inst.recordSuccessfulGameServerQuery(gs.ID, queryType, startedAt, &xylona.ServerQuery{Type: queryType, Source: info})
					}
					inst.storeServerQuery(&xylona.ServerQuery{
						ServerId:   gs.ID,
						ServerName: gs.Name,
						Type:       xylona.ServerQuery_Source,
						Source:     info,
					})
					return
				case xylona.ServerQuery_Palworld:
					if gs.Status != xylona.Status_ONLINE.String() {
						inst.recordUnavailableGameServerQuery(gs.ID, queryType)
						inst.storeServerQuery(defaultServerQuery(gs, queryType))
						return
					}
					startedAt := time.Now()
					if inst.isRemoteGameServer(gs) {
						serverQuery, errQuery := inst.queryRemoteGameServer(ctx, gs, queryType)
						if errQuery != nil {
							inst.recordFailedGameServerQuery(gs.ID, queryType, startedAt)
							inst.storeServerQuery(defaultServerQuery(gs, queryType))
							return
						}
						inst.recordSuccessfulGameServerQuery(gs.ID, queryType, startedAt, serverQuery)
						inst.storeServerQuery(serverQuery)
						return
					}
					username, password, errCredentials := inst.palworldQueryCredentials(gs)
					if errCredentials != nil {
						log.Debug().Err(errCredentials).Str("server", gs.Name).Msg("Failed to load Palworld query credentials")
						inst.recordFailedGameServerQuery(gs.ID, queryType, startedAt)
						inst.storeServerQuery(defaultServerQuery(gs, queryType))
						return
					}
					info, errQuery := query.Palworld(ctx, gs.IP, int(gs.QueryPort), username, password)
					if errQuery != nil {
						log.Debug().Err(errQuery).Str("server", gs.Name).Msg("Failed to query Palworld server")
						inst.recordFailedGameServerQuery(gs.ID, queryType, startedAt)
						info = &xylona.PalworldQueryInfo{MaxPlayers: helpers.ClampUint32FromInt64(gs.MaxPlayers)}
					} else {
						inst.recordSuccessfulGameServerQuery(gs.ID, queryType, startedAt, &xylona.ServerQuery{Type: queryType, Palworld: info})
					}
					inst.storeServerQuery(&xylona.ServerQuery{
						ServerId:   gs.ID,
						ServerName: gs.Name,
						Type:       xylona.ServerQuery_Palworld,
						Palworld:   info,
					})
					return
				}
			})
			return nil
		})
	}
	if errWait := errGroup.Wait(); errWait != nil {
		log.Warn().Err(errWait).Msg("Background task group returned error")
	}
}

func (inst *Instance) backgroundJobCheckVersionUpdates() {
	if inst.versionState == nil {
		return
	}

	interval := 4 * time.Hour
	if envInterval := os.Getenv("XYLONA_VERSION_CHECK_INTERVAL"); envInterval != "" {
		parsed, errParse := time.ParseDuration(envInterval)
		if errParse == nil && parsed > 0 {
			interval = parsed
		}
	}

	// Run an initial check shortly after startup so version info is available
	// without waiting for the first periodic tick.
	startupTimer := time.NewTimer(5 * time.Second)
	defer startupTimer.Stop()
	select {
	case <-inst.ctx.Done():
		return
	case <-startupTimer.C:
		runBackgroundTask("backgroundJobCheckVersionUpdates", "startup-check", nil, func() {
			inst.checkAllServerVersions()
		})
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-inst.ctx.Done():
			return
		case <-ticker.C:
			runBackgroundTask("backgroundJobCheckVersionUpdates", "tick", nil, func() {
				inst.checkAllServerVersions()
			})
		}
	}
}

func (inst *Instance) checkAllServerVersions() {
	gameServers, errServers := inst.db.GetAllGameServers()
	if errServers != nil {
		log.Error().Err(errServers).Msg("Version check: failed to get game servers")
		return
	}

	// Include all servers unless they are EXPLICITLY known to have no tracker
	// (i.e., their state was deliberately set to VersionStatusNoTracker by a
	// prior check). Servers with the default state (also VersionStatusNoTracker
	// but never written to the map) are treated as unchecked and included so
	// that trackers can be resolved on first run.
	allStates := inst.versionState.GetAll()
	var trackable []*models.GameServer
	for _, gs := range gameServers {
		state, explicitlySet := allStates[gs.ID]
		if !explicitlySet || state.Status != versiontracker.VersionStatusNoTracker {
			trackable = append(trackable, gs)
		}
	}

	if len(trackable) == 0 {
		return
	}

	stagger := time.Duration(0)
	if len(trackable) > 1 {
		stagger = 30 * time.Second
	}

	eb := eventbus.Get()

	for i, gs := range trackable {
		if i > 0 && stagger > 0 {
			staggerTimer := time.NewTimer(stagger)
			select {
			case <-inst.ctx.Done():
				if !staggerTimer.Stop() {
					<-staggerTimer.C
				}
				return
			case <-staggerTimer.C:
			}
		}
		gameServer := gs
		runBackgroundTask("backgroundJobCheckVersionUpdates", "check-server", map[string]string{
			"game_server_id": gameServer.ID,
		}, func() {
			inst.checkServerVersion(gameServer, eb)
		})
	}
}

func (inst *Instance) checkServerVersion(gs *models.GameServer, eb *eventbus.EventBus) {
	tracker := inst.resolveTrackerForServer(gs)
	if tracker == nil {
		inst.versionState.InitNoTracker(gs.ID)
		inst.publishVersionState(gs, inst.versionState.Get(gs.ID))
		return
	}
	trackerType := versiontracker.TrackerTypeName(tracker)
	_ = eb
	inst.refreshVersionState(inst.ctx, gs, tracker, trackerType, true, true)
}

// backgroundJobCheckModUpdates runs every 6 hours, checks all game servers for
// available mod updates, and publishes a "mod.update_available" event for each
// server that has at least one mod with a newer version available.
func (inst *Instance) backgroundJobCheckModUpdates() {
	if inst.modManager == nil {
		return
	}

	throttle := time.NewTicker(time.Hour * 6)
	defer throttle.Stop()

	for {
		select {
		case <-inst.ctx.Done():
			return
		case <-throttle.C:
			runBackgroundTask("backgroundJobCheckModUpdates", "tick", nil, func() {
				gameServers, errServers := inst.db.GetAllGameServers()
				if errServers != nil {
					log.Error().Err(errServers).Msg("Mod update check: failed to get game servers")
					return
				}

				eb := eventbus.Get()

				for _, gs := range gameServers {
					gameServer := gs
					runBackgroundTask("backgroundJobCheckModUpdates", "check-server", map[string]string{
						"game_server_id": gameServer.ID,
					}, func() {
						updates, errCheck := inst.modManager.CheckUpdates(inst.ctx, gameServer.ID, "")
						if errCheck != nil {
							log.Warn().Err(errCheck).Str("game_server_id", gameServer.ID).
								Msg("Mod update check: failed to check updates for game server")
							return
						}
						if len(updates) > 0 {
							eb.Publish("mod.update_available", gameServer.ID)
							log.Info().Str("game_server_id", gameServer.ID).Int("update_count", len(updates)).
								Msg("Mod update check: updates available")
						}
					})
				}
			})
		}
	}
}
