package actions

import (
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/supervisor"
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

// handleServerExit is called from the StartGameServer callback after the
// process exits. It decides whether to auto-restart the server, applying
// exponential backoff and retry limits based on the server's current DB config.
func (inst *Instance) handleServerExit(cmd *supervisor.Command, serverID string) {
	if cmd.IntentionalStop() {
		log.Debug().Str("game_server_id", serverID).
			Msg("Auto-restart: stop was intentional, skipping")
		return
	}

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
		inst.supervisorInstance.SendConsoleOutput(serverID, msg)
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

	msg := fmt.Sprintf(
		"Server exited unexpectedly. Restarting in %s (attempt %d/%d)...",
		delay.Round(time.Second), attempt+1, maxRetries,
	)
	inst.supervisorInstance.SendConsoleOutput(serverID, msg)

	log.Warn().Str("game_server_id", serverID).
		Int("attempt", attempt+1).Int("max", maxRetries).
		Dur("delay", delay).
		Msg("Auto-restart: scheduling restart")

	// Run the restart in a goroutine so the callback returns promptly.
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
			inst.supervisorInstance.SendConsoleOutput(serverID,
				"Auto-restart was disabled during cooldown. Restart cancelled.")
			log.Info().Str("game_server_id", serverID).
				Msg("Auto-restart: disabled before restart fired, aborting")
			return
		}

		startMsg := fmt.Sprintf("Auto-restart: starting server (attempt %d/%d)",
			attempt+1, maxRetries)
		inst.supervisorInstance.SendConsoleOutput(serverID, startMsg)
		log.Info().Str("game_server_id", serverID).
			Int("attempt", attempt+1).
			Msg("Auto-restart: starting server")

		inst.StartGameServer(gs)
	}()
}
