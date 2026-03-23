package versiontracker

import (
	"context"
	"sync"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// DummyTracker is a VersionTracker implementation for testing and development.
// It returns hardcoded version values and supports simulating updates and failures.
type DummyTracker struct {
	mu              sync.RWMutex
	simulateFailure bool
	updated         bool
}

// NewDummyTracker creates a new DummyTracker with default state.
func NewDummyTracker() *DummyTracker {
	return &DummyTracker{}
}

// GetInstalledVersion returns "1.0.0" by default, or "2.0.0" after MarkUpdated is called.
func (d *DummyTracker) GetInstalledVersion(_ context.Context, _ *models.GameServer) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.updated {
		return "2.0.0", nil
	}
	return "1.0.0", nil
}

// GetLatestVersion always returns "2.0.0".
func (d *DummyTracker) GetLatestVersion(_ context.Context, _ *models.GameServer) (string, error) {
	return "2.0.0", nil
}

// CheckForUpdate compares installed vs latest and returns update info.
// Returns nil if versions match (up-to-date).
func (d *DummyTracker) CheckForUpdate(ctx context.Context, gs *models.GameServer) (*UpdateInfo, error) {
	installed, errInstalled := d.GetInstalledVersion(ctx, gs)
	if errInstalled != nil {
		return nil, errInstalled
	}
	latest, errLatest := d.GetLatestVersion(ctx, gs)
	if errLatest != nil {
		return nil, errLatest
	}
	if installed == latest {
		return nil, nil
	}
	return &UpdateInfo{
		InstalledVersion: installed,
		LatestVersion:    latest,
		UpdateAvailable:  true,
	}, nil
}

// SetSimulateFailure sets whether the tracker should simulate failures.
func (d *DummyTracker) SetSimulateFailure(fail bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.simulateFailure = fail
}

// SimulateFailure returns whether failure simulation is enabled.
func (d *DummyTracker) SimulateFailure() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.simulateFailure
}

// MarkUpdated simulates a successful update, changing the installed version to match latest.
func (d *DummyTracker) MarkUpdated() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.updated = true
}

// Reset returns the tracker to its initial state.
func (d *DummyTracker) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.updated = false
	d.simulateFailure = false
}
