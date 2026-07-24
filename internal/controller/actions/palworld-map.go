package actions

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const palworldMapPollInterval = 5 * time.Second

// PalworldMapState is the latest controller-cached map state for one server.
// A failed or offline poll preserves the last snapshot and marks it stale at
// the API boundary instead of silently presenting it as current.
type PalworldMapState struct {
	ServerID          string
	ServerName        string
	ServerOnline      bool
	Snapshot          *node.PalworldMapSnapshot
	UnavailableReason string
}

// GetPalworldMapState returns a defensive copy of the latest cached state.
func (inst *Instance) GetPalworldMapState(gameServerID string) PalworldMapState {
	inst.palworldMapsMutex.RLock()
	state, exists := inst.palworldMaps[gameServerID]
	inst.palworldMapsMutex.RUnlock()
	if !exists {
		return PalworldMapState{
			ServerID:          gameServerID,
			UnavailableReason: "Waiting for the first live map snapshot.",
		}
	}
	state.Snapshot = clonePalworldMapSnapshot(state.Snapshot)
	return state
}

func (inst *Instance) backgroundJobQueryPalworldMaps() {
	startupTimer := time.NewTimer(5 * time.Second)
	defer startupTimer.Stop()
	select {
	case <-inst.ctx.Done():
		return
	case <-startupTimer.C:
		inst.queryAllPalworldMaps(inst.ctx)
	}

	ticker := time.NewTicker(palworldMapPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-inst.ctx.Done():
			return
		case <-ticker.C:
			inst.queryAllPalworldMaps(inst.ctx)
		}
	}
}

func (inst *Instance) queryAllPalworldMaps(ctx context.Context) {
	gameServers, errServers := inst.db.GetAllGameServers()
	if errServers != nil {
		log.Error().Err(errServers).Msg("Failed to list Palworld servers for live map polling")
		return
	}

	errGroup, groupContext := errgroup.WithContext(ctx)
	errGroup.SetLimit(10)
	for _, gameServer := range gameServers {
		gs := gameServer
		if gs == nil || gs.GameID != palworldGameID {
			continue
		}
		errGroup.Go(func() error {
			inst.queryOnePalworldMap(groupContext, gs)
			return nil
		})
	}
	errWait := errGroup.Wait()
	if errWait != nil {
		log.Warn().Err(errWait).Msg("Palworld live map polling group returned an error")
	}
}

func (inst *Instance) queryOnePalworldMap(ctx context.Context, gameServer *models.GameServer) {
	serverOnline := inst.currentProcessStatus(gameServer) == xylona.Status_ONLINE
	if !serverOnline {
		inst.storePalworldMapState(PalworldMapState{
			ServerID:          gameServer.ID,
			ServerName:        gameServer.Name,
			ServerOnline:      false,
			UnavailableReason: "The game server is offline. Showing the last snapshot when available.",
		})
		return
	}

	username, password, errCredentials := inst.palworldQueryCredentials(gameServer)
	if errCredentials != nil {
		log.Debug().Err(errCredentials).Str("game_server_id", gameServer.ID).Msg("Palworld live map credentials are unavailable")
		inst.storePalworldMapState(PalworldMapState{
			ServerID:          gameServer.ID,
			ServerName:        gameServer.Name,
			ServerOnline:      true,
			UnavailableReason: "Configure the Palworld REST API credentials to load the live map.",
		})
		return
	}

	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		log.Debug().Err(errClient).Str("game_server_id", gameServer.ID).Msg("Palworld live map node is unavailable")
		inst.storePalworldMapState(PalworldMapState{
			ServerID:          gameServer.ID,
			ServerName:        gameServer.Name,
			ServerOnline:      true,
			UnavailableReason: "The server node is unavailable. Showing the last snapshot when available.",
		})
		return
	}

	snapshot, errQuery := client.QueryPalworldMap(ctx, node.PalworldMapQueryRequest{
		IP:        gameServer.IP,
		QueryPort: gameServer.QueryPort,
		Username:  username,
		Password:  password,
	})
	if errQuery != nil {
		log.Debug().Err(errQuery).Str("game_server_id", gameServer.ID).Msg("Palworld live map query failed")
		inst.storePalworldMapState(PalworldMapState{
			ServerID:          gameServer.ID,
			ServerName:        gameServer.Name,
			ServerOnline:      true,
			UnavailableReason: "The live world snapshot is temporarily unavailable. Showing the last snapshot when available.",
		})
		return
	}
	if snapshot == nil {
		inst.storePalworldMapState(PalworldMapState{
			ServerID:          gameServer.ID,
			ServerName:        gameServer.Name,
			ServerOnline:      true,
			UnavailableReason: "The server node returned an empty live map snapshot.",
		})
		return
	}

	inst.storePalworldMapState(PalworldMapState{
		ServerID:     gameServer.ID,
		ServerName:   gameServer.Name,
		ServerOnline: true,
		Snapshot:     snapshot,
	})
}

func (inst *Instance) storePalworldMapState(next PalworldMapState) {
	inst.palworldMapsMutex.Lock()
	previous, exists := inst.palworldMaps[next.ServerID]
	if next.Snapshot == nil && exists {
		next.Snapshot = previous.Snapshot
	}
	inst.palworldMaps[next.ServerID] = PalworldMapState{
		ServerID:          next.ServerID,
		ServerName:        next.ServerName,
		ServerOnline:      next.ServerOnline,
		Snapshot:          clonePalworldMapSnapshot(next.Snapshot),
		UnavailableReason: next.UnavailableReason,
	}
	inst.palworldMapsMutex.Unlock()
}

func clonePalworldMapSnapshot(snapshot *node.PalworldMapSnapshot) *node.PalworldMapSnapshot {
	if snapshot == nil {
		return nil
	}
	cloned := *snapshot
	cloned.Actors = append([]node.PalworldMapActor(nil), snapshot.Actors...)
	if snapshot.Health != nil {
		health := *snapshot.Health
		cloned.Health = &health
	}
	return &cloned
}
