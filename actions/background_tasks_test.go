package actions

import (
	"context"
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestCheckServerVersionSetsResolvedTrackerType(t *testing.T) {
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
