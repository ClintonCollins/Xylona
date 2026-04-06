package actions

import (
	"context"
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestCheckServerVersionSetsResolvedTrackerType(t *testing.T) {
	t.Parallel()

	inst := &Instance{
		ctx:          context.Background(),
		versionState: versiontracker.NewVersionStateMap(),
		resolverConfig: versiontracker.ResolverConfig{
			DummyTracker: versiontracker.NewDummyTracker(),
			DummyGameID:  "dummy-game",
		},
	}

	gs := &models.GameServer{
		ID:     "server-1",
		GameID: "dummy-game",
	}
	gs.R.Game = &models.Game{}

	inst.checkServerVersion(gs, eventbus.Get())

	state := inst.versionState.Get(gs.ID)
	if state.Status != versiontracker.VersionStatusChecked {
		t.Fatalf("status = %v, want %v", state.Status, versiontracker.VersionStatusChecked)
	}
	if state.TrackerType != "dummy" {
		t.Fatalf("tracker type = %q, want %q", state.TrackerType, "dummy")
	}
}

func TestCheckServerVersionPopulatesVersionsWhenUpdateAvailable(t *testing.T) {
	t.Parallel()

	dummy := versiontracker.NewDummyTracker()
	inst := &Instance{
		ctx:          context.Background(),
		versionState: versiontracker.NewVersionStateMap(),
		resolverConfig: versiontracker.ResolverConfig{
			DummyTracker: dummy,
			DummyGameID:  "dummy-game",
		},
	}

	gs := &models.GameServer{
		ID:     "server-1",
		GameID: "dummy-game",
	}
	gs.R.Game = &models.Game{}

	inst.checkServerVersion(gs, eventbus.Get())

	state := inst.versionState.Get(gs.ID)
	if state.InstalledVersion != "1.0.0" {
		t.Errorf("InstalledVersion = %q, want %q", state.InstalledVersion, "1.0.0")
	}
	if state.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", state.LatestVersion, "2.0.0")
	}
	if !state.UpdateAvailable {
		t.Errorf("UpdateAvailable = false, want true")
	}
}

func TestCheckServerVersionPopulatesVersionsWhenUpToDate(t *testing.T) {
	t.Parallel()

	dummy := versiontracker.NewDummyTracker()
	dummy.MarkUpdated() // installed=2.0.0, latest=2.0.0 — no update
	inst := &Instance{
		ctx:          context.Background(),
		versionState: versiontracker.NewVersionStateMap(),
		resolverConfig: versiontracker.ResolverConfig{
			DummyTracker: dummy,
			DummyGameID:  "dummy-game",
		},
	}

	gs := &models.GameServer{
		ID:     "server-1",
		GameID: "dummy-game",
	}
	gs.R.Game = &models.Game{}

	inst.checkServerVersion(gs, eventbus.Get())

	state := inst.versionState.Get(gs.ID)
	if state.Status != versiontracker.VersionStatusChecked {
		t.Fatalf("status = %v, want %v", state.Status, versiontracker.VersionStatusChecked)
	}
	if state.InstalledVersion != "2.0.0" {
		t.Errorf("InstalledVersion = %q, want %q", state.InstalledVersion, "2.0.0")
	}
	if state.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", state.LatestVersion, "2.0.0")
	}
	if state.UpdateAvailable {
		t.Errorf("UpdateAvailable = true, want false")
	}
}
