package versiontracker

import (
	"sync"
	"time"
)

// VersionStatus represents the current state of version tracking for a server.
type VersionStatus int

const (
	// VersionStatusNoTracker indicates no version tracker is available for this server.
	VersionStatusNoTracker VersionStatus = iota
	// VersionStatusUnchecked indicates a tracker exists but has not checked yet.
	VersionStatusUnchecked
	// VersionStatusChecking indicates a version check is in progress.
	VersionStatusChecking
	// VersionStatusChecked indicates a version check has completed successfully.
	VersionStatusChecked
	// VersionStatusError indicates the last version check encountered an error.
	VersionStatusError
)

// VersionState holds the version tracking state for a single game server.
type VersionState struct {
	Status           VersionStatus
	InstalledVersion string
	LatestVersion    string
	UpdateAvailable  bool
	LastCheckTime    time.Time
	InstalledCheckTime time.Time
	LatestCheckTime    time.Time
	TrackerType      string
}

// VersionStateMap is a concurrent-safe map of server ID to VersionState.
type VersionStateMap struct {
	mu     sync.RWMutex
	states map[string]VersionState
}

// NewVersionStateMap creates a new empty VersionStateMap.
func NewVersionStateMap() *VersionStateMap {
	return &VersionStateMap{
		states: make(map[string]VersionState),
	}
}

// Get returns the VersionState for the given server ID.
// Returns a state with VersionStatusNoTracker if the server is not tracked.
func (m *VersionStateMap) Get(serverID string) VersionState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.states[serverID]
	if !ok {
		return VersionState{Status: VersionStatusNoTracker}
	}
	return state
}

// GetWithOK returns the VersionState and whether it was explicitly stored.
func (m *VersionStateMap) GetWithOK(serverID string) (VersionState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.states[serverID]
	return state, ok
}

// Set stores a VersionState for the given server ID.
func (m *VersionStateMap) Set(serverID string, state VersionState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[serverID] = state
}

// InitUnchecked initializes a server entry with Unchecked status and the given tracker type.
func (m *VersionStateMap) InitUnchecked(serverID string, trackerType string) {
	m.Set(serverID, VersionState{
		Status:      VersionStatusUnchecked,
		TrackerType: trackerType,
	})
}

// InitNoTracker initializes a server entry with NoTracker status.
func (m *VersionStateMap) InitNoTracker(serverID string) {
	m.Set(serverID, VersionState{
		Status: VersionStatusNoTracker,
	})
}

// GetAll returns a copy of all tracked server states.
func (m *VersionStateMap) GetAll() map[string]VersionState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]VersionState, len(m.states))
	for k, v := range m.states {
		result[k] = v
	}
	return result
}

// Delete removes the version state for the given server ID.
func (m *VersionStateMap) Delete(serverID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.states, serverID)
}
