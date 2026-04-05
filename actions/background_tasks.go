package actions

import (
	"context"
	"os"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/pkg/query"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
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
			gameServers, err := inst.db.GetAllGameServers()
			if err != nil {
				log.Error().Err(err).Msg("Failed to get game servers")
				continue
			}
			inst.queryGameServers(inst.ctx, gameServers)
		}
	}
}

func getQueryInfoType(game *models.Game) xylona.ServerQuery_Type {
	if game.ID == "minecraft" {
		return xylona.ServerQuery_Minecraft
	}
	if game.UsesSourceQuery {
		return xylona.ServerQuery_Source
	}
	return xylona.ServerQuery_Unknown
}

func (inst *Instance) queryGameServers(ctx context.Context, gameServers []*models.GameServer) {
	errGroup, _ := errgroup.WithContext(ctx)
	errGroup.SetLimit(30)
	for _, gameServer := range gameServers {
		gs := gameServer
		errGroup.Go(func() error {
			gameServerCmd, errGetCommand := inst.supervisorInstance.GetCommandByID(gs.ID)
			if errGetCommand != nil {
				gs.Status = xylona.Status_OFFLINE.String()
			} else {
				gs.Status = gameServerCmd.Status().String()
			}
			switch getQueryInfoType(gs.R.Game) {
			case xylona.ServerQuery_Minecraft:
				if gs.Status != xylona.Status_ONLINE.String() {
					inst.serverQueriesMutex.Lock()
					inst.serverQueriesInfoMap[gs.ID] = &xylona.ServerQuery{
						ServerId:   gs.ID,
						ServerName: gs.Name,
						Type:       xylona.ServerQuery_Minecraft,
						Minecraft:  &xylona.MinecraftQueryInfo{MaxPlayers: helpers.ClampUint32FromInt64(gs.MaxPlayers)},
					}
					inst.serverQueriesMutex.Unlock()
					return nil
				}
				info, err := query.Minecraft(gs.IP, int(gs.QueryPort))
				if err != nil {
					log.Debug().Err(err).Str("server", gs.Name).Msg("Failed to query minecraft server")
					info = &xylona.MinecraftQueryInfo{MaxPlayers: helpers.ClampUint32FromInt64(gs.MaxPlayers)}
				}
				inst.serverQueriesMutex.Lock()
				inst.serverQueriesInfoMap[gs.ID] = &xylona.ServerQuery{
					ServerId:   gs.ID,
					ServerName: gs.Name,
					Type:       xylona.ServerQuery_Minecraft,
					Minecraft:  info,
				}
				inst.serverQueriesMutex.Unlock()
				return nil
			case xylona.ServerQuery_Source:
				if gs.Status != xylona.Status_ONLINE.String() {
					inst.serverQueriesMutex.Lock()
					inst.serverQueriesInfoMap[gs.ID] = &xylona.ServerQuery{
						ServerId:   gs.ID,
						ServerName: gs.Name,
						Type:       xylona.ServerQuery_Source,
						Source:     &xylona.SourceQueryInfo{MaxPlayers: helpers.ClampUint32FromInt64(gs.MaxPlayers)},
					}
					inst.serverQueriesMutex.Unlock()
					return nil
				}
				info, err := query.Source(gs.IP, int(gs.QueryPort))
				if err != nil {
					log.Debug().Err(err).Str("server", gs.Name).Msg("Failed to query source server")
					info = &xylona.SourceQueryInfo{MaxPlayers: helpers.ClampUint32FromInt64(gs.MaxPlayers)}
				}
				inst.serverQueriesMutex.Lock()
				inst.serverQueriesInfoMap[gs.ID] = &xylona.ServerQuery{
					ServerId:   gs.ID,
					ServerName: gs.Name,
					Type:       xylona.ServerQuery_Source,
					Source:     info,
				}
				inst.serverQueriesMutex.Unlock()
				return nil
			}
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
		inst.checkAllServerVersions()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-inst.ctx.Done():
			return
		case <-ticker.C:
			inst.checkAllServerVersions()
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
		inst.checkServerVersion(gs, eb)
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
			gameServers, errServers := inst.db.GetAllGameServers()
			if errServers != nil {
				log.Error().Err(errServers).Msg("Mod update check: failed to get game servers")
				continue
			}

			eb := eventbus.Get()

			for _, gs := range gameServers {
				updates, errCheck := inst.modManager.CheckUpdates(inst.ctx, gs.ID, "")
				if errCheck != nil {
					log.Warn().Err(errCheck).Str("game_server_id", gs.ID).
						Msg("Mod update check: failed to check updates for game server")
					continue
				}
				if len(updates) > 0 {
					eb.Publish("mod.update_available", gs.ID)
					log.Info().Str("game_server_id", gs.ID).Int("update_count", len(updates)).
						Msg("Mod update check: updates available")
				}
			}
		}
	}
}
