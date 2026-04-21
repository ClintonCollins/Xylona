package actions

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// autoRestartStableWindow is how long a server must run continuously before its
// retry counter resets.
const autoRestartStableWindow = 5 * time.Minute

// restartEntry tracks per-server auto-restart attempt state.
type restartEntry struct {
	mu            sync.Mutex
	attemptCount  int
	lastStartTime time.Time
}

// restartStateMap tracks per-server restart attempt state in memory.
// State is intentionally not persisted; it resets on Xylona restart so that a
// server that has been offline long enough for a service restart gets a fresh
// attempt window.
type restartStateMap struct {
	m sync.Map // map[serverID string]*restartEntry
}

func (r *restartStateMap) entry(serverID string) *restartEntry {
	val, _ := r.m.LoadOrStore(serverID, &restartEntry{})
	entry, _ := val.(*restartEntry)
	return entry
}

func (r *restartStateMap) recordStarted(serverID string) {
	e := r.entry(serverID)
	e.mu.Lock()
	e.lastStartTime = time.Now()
	e.mu.Unlock()
}

// startAutoRestartSubscriber spins up a goroutine that listens for game
// server status-change events on the eventbus and invokes handleServerExit
// when a server transitions into OFFLINE. Both embedded and remote nodes
// publish status-change events uniformly (the remote-event bridge
// republishes remote node events into the same bus), so the subscriber is
// the single place that reacts to a process exiting.
//
// Call this once during controller startup. The goroutine exits when ctx
// is canceled.
func (inst *Instance) startAutoRestartSubscriber(ctx context.Context) {
	bus := eventbus.Get()
	ch := bus.SubscribeReliable(eventbus.TopicGameServerStatusChanged)
	go func() {
		defer bus.Unsubscribe(eventbus.TopicGameServerStatusChanged, ch)
		for {
			select {
			case <-ctx.Done():
				return
			case raw, ok := <-ch:
				if !ok {
					return
				}
				event, castOK := raw.(eventbus.StatusChangedEvent)
				if !castOK {
					log.Warn().Msg("auto-restart: got non-StatusChangedEvent on status topic")
					continue
				}
				inst.onStatusChanged(event)
			}
		}
	}()
}

// onStatusChanged is the status-event sink. It filters for OFFLINE
// transitions (process exited), runs any registered one-shot exit hook
// (install post-install, update restart, etc.), then invokes the
// auto-restart logic. Intentional stops skip auto-restart but still fire
// hooks; non-OFFLINE transitions are informational only.
func (inst *Instance) onStatusChanged(event eventbus.StatusChangedEvent) {
	newStatus := strings.ToUpper(strings.TrimSpace(event.NewStatus))
	if newStatus != xylona.Status_OFFLINE.String() {
		return
	}

	// Fire one-shot exit hook first; install/update flows set these to run
	// post-install or chain a restart.
	if inst.exitHooks != nil {
		hook, ok := inst.exitHooks.take(event.ServerID)
		if ok {
			// Run in a goroutine so a slow hook doesn't block the event
			// subscriber from processing subsequent events.
			go hook(event)
		}
	}

	if event.IntentionalStop {
		inst.intentionalStops.clear(event.ServerID)
		log.Debug().Str("game_server_id", event.ServerID).
			Msg("Auto-restart: stop was intentional, skipping")
		return
	}

	if inst.intentionalStops.take(event.ServerID) {
		log.Debug().Str("game_server_id", event.ServerID).
			Msg("Auto-restart: stop was intentionally requested, skipping")
		return
	}
	inst.handleServerExit(event.ServerID)
}

// handleServerExit runs the auto-restart decision for a server that has
// just gone OFFLINE unexpectedly. Applies exponential backoff and retry
// limits based on the server's current DB config. Works for both embedded
// and remote nodes: the only differences flow through inst.StartGameServer
// (which routes via NodeClient).
func (inst *Instance) handleServerExit(serverID string) {
	// Load fresh config from DB to pick up any live changes.
	gameServer, errGet := inst.db.GetGameServerByID(serverID)
	if errGet != nil {
		log.Error().Err(errGet).Str("game_server_id", serverID).
			Msg("Auto-restart: failed to load game server")
		return
	}

	if !gameServer.AutoRestartEnabled {
		return
	}

	maxRetries := int(gameServer.AutoRestartMaxRetries)
	baseCooldown := time.Duration(gameServer.AutoRestartCooldownSeconds) * time.Second

	e := inst.restartState.entry(serverID)
	e.mu.Lock()
	defer e.mu.Unlock()

	// Reset attempt counter if the server ran stably before this exit.
	if !e.lastStartTime.IsZero() && time.Since(e.lastStartTime) >= autoRestartStableWindow {
		if e.attemptCount > 0 {
			log.Info().Str("game_server_id", serverID).
				Dur("uptime", time.Since(e.lastStartTime)).
				Msg("Auto-restart: stable run detected, resetting retry counter")
		}
		e.attemptCount = 0
	}

	if e.attemptCount >= maxRetries {
		msg := fmt.Sprintf(
			"Auto-restart limit reached (%d/%d). Server will not be restarted automatically.",
			e.attemptCount, maxRetries,
		)
		inst.sendConsoleLine(gameServer, msg)
		log.Warn().Str("game_server_id", serverID).
			Int("attempts", e.attemptCount).Int("max", maxRetries).
			Msg("Auto-restart: retry limit exhausted")
		return
	}

	attempt := e.attemptCount
	e.attemptCount++

	// Exponential backoff: baseCooldown * 2^attempt, capped to avoid overflow.
	shift := min(attempt, 6) // cap at 64x base cooldown
	delay := baseCooldown * (1 << shift)
	if attempt == 0 {
		delay = 1 * time.Second
	}

	msg := fmt.Sprintf(
		"Server exited unexpectedly. Restarting in %s (attempt %d/%d)...",
		delay.Round(time.Second), attempt+1, maxRetries,
	)
	inst.sendConsoleLine(gameServer, msg)

	log.Warn().Str("game_server_id", serverID).
		Int("attempt", attempt+1).Int("max", maxRetries).
		Dur("delay", delay).
		Msg("Auto-restart: scheduling restart")

	// Run the restart in a goroutine so the subscriber returns promptly.
	go func() {
		select {
		case <-inst.ctx.Done():
			log.Debug().Str("game_server_id", serverID).
				Msg("Auto-restart: Xylona shutting down, cancelling restart")
			return
		case <-time.After(delay):
		}

		// Re-load to catch any mid-cooldown config changes or deletion.
		gs, errReload := inst.db.GetGameServerByID(serverID)
		if errReload != nil {
			log.Error().Err(errReload).Str("game_server_id", serverID).
				Msg("Auto-restart: server gone before restart, aborting")
			return
		}
		if !gs.AutoRestartEnabled {
			inst.sendConsoleLine(gs, "Auto-restart was disabled during cooldown. Restart cancelled.")
			log.Info().Str("game_server_id", serverID).
				Msg("Auto-restart: disabled before restart fired, aborting")
			return
		}

		startMsg := fmt.Sprintf("Auto-restart: starting server (attempt %d/%d)",
			attempt+1, maxRetries)
		inst.sendConsoleLine(gs, startMsg)
		log.Info().Str("game_server_id", serverID).
			Int("attempt", attempt+1).
			Msg("Auto-restart: starting server")

		inst.StartGameServer(gs)
	}()
}

// sendConsoleLine pushes a controller-generated line into the console buffer
// for the game server, routing via the owning node's NodeClient so it works
// for both embedded and remote servers. Falls back to the supervisor path
// when the registry isn't configured (tests that bypass the node layer).
func (inst *Instance) sendConsoleLine(gs *models.GameServer, line string) {
	if gs == nil {
		return
	}
	if inst.nodeRegistry != nil {
		client, errGet := inst.nodeRegistry.Get(gs.NodeID)
		if errGet == nil {
			errSend := client.SendConsoleOutput(inst.ctx, gs.ID, line)
			if errSend == nil {
				return
			}
			log.Warn().Err(errSend).Str("game_server_id", gs.ID).
				Msg("auto-restart: send console line via node client failed; falling back")
		}
	}
	if inst.supervisorInstance != nil {
		inst.supervisorInstance.SendConsoleOutput(gs.ID, line)
	}
}
