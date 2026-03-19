package actions

import (
	"context"
	"time"

	"github.com/ClintonCollins/Xylona/pkg/query"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
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
						Minecraft:  &xylona.MinecraftQueryInfo{MaxPlayers: uint32(gs.MaxPlayers)},
					}
					inst.serverQueriesMutex.Unlock()
					return nil
				}
				info, err := query.Minecraft(gs.IP, int(gs.QueryPort))
				if err != nil {
					log.Debug().Err(err).Str("server", gs.Name).Msg("Failed to query minecraft server")
					info = &xylona.MinecraftQueryInfo{MaxPlayers: uint32(gs.MaxPlayers)}
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
						Source:     &xylona.SourceQueryInfo{MaxPlayers: uint32(gs.MaxPlayers)},
					}
					inst.serverQueriesMutex.Unlock()
					return nil
				}
				info, err := query.Source(gs.IP, int(gs.QueryPort))
				if err != nil {
					log.Debug().Err(err).Str("server", gs.Name).Msg("Failed to query source server")
					info = &xylona.SourceQueryInfo{MaxPlayers: uint32(gs.MaxPlayers)}
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
