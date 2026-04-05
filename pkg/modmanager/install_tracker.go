package modmanager

import (
	"sync"
	"time"
)

// Install status values tracked for in-progress and recent installs.
const (
	InstallStatusIdle       = "idle"
	InstallStatusInstalling = "installing"
	InstallStatusComplete   = "complete"
	InstallStatusFailed     = "failed"
)

// InstallState represents the current state of a server software installation.
type InstallState struct {
	Status     string
	SoftwareID string
	Error      string
	UpdatedAt  time.Time
}

// InstallTracker tracks in-memory server software installation state.
type InstallTracker struct {
	mu    sync.RWMutex
	state map[string]InstallState
	ttl   time.Duration
}

// NewInstallTracker creates a new InstallTracker with a default 5-minute TTL
// for completed/failed states.
func NewInstallTracker() *InstallTracker {
	return &InstallTracker{
		state: make(map[string]InstallState),
		ttl:   5 * time.Minute,
	}
}

// SetInstalling marks a server as currently installing software.
func (t *InstallTracker) SetInstalling(gameServerID string, softwareID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state[gameServerID] = InstallState{
		Status:     InstallStatusInstalling,
		SoftwareID: softwareID,
		UpdatedAt:  time.Now(),
	}
}

// SetComplete marks a server software installation as complete.
func (t *InstallTracker) SetComplete(gameServerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	existing, ok := t.state[gameServerID]
	if !ok {
		return
	}
	existing.Status = InstallStatusComplete
	existing.UpdatedAt = time.Now()
	t.state[gameServerID] = existing
}

// SetFailed marks a server software installation as failed.
func (t *InstallTracker) SetFailed(gameServerID string, errMsg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	existing, ok := t.state[gameServerID]
	if !ok {
		return
	}
	existing.Status = InstallStatusFailed
	existing.Error = errMsg
	existing.UpdatedAt = time.Now()
	t.state[gameServerID] = existing
}

// IsInstalling returns whether a server currently has an install in progress.
func (t *InstallTracker) IsInstalling(gameServerID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	state, ok := t.state[gameServerID]
	return ok && state.Status == InstallStatusInstalling
}

// Get returns the install state for a server. If no state exists, returns
// an idle state with ok=false.
func (t *InstallTracker) Get(gameServerID string) (InstallState, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	state, ok := t.state[gameServerID]
	if !ok {
		return InstallState{Status: InstallStatusIdle}, false
	}
	return state, true
}

// Cleanup removes completed/failed states older than the TTL.
// In-progress states are never removed.
func (t *InstallTracker) Cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	for id, state := range t.state {
		if state.Status == InstallStatusInstalling {
			continue
		}
		if now.Sub(state.UpdatedAt) > t.ttl {
			delete(t.state, id)
		}
	}
}
