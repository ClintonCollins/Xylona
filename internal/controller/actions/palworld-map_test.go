package actions

import (
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/node"
)

func TestPalworldMapState(t *testing.T) {
	inst := newTestInstance(t)
	collectedAt := time.Now().UTC()
	inst.storePalworldMapState(PalworldMapState{
		ServerID:     "palworld-1",
		ServerName:   "Palpagos",
		ServerOnline: true,
		Snapshot: &node.PalworldMapSnapshot{
			CollectedAt: collectedAt,
			Health: &node.PalworldMapHealth{
				ServerFPS:     60,
				BaseCampCount: 3,
			},
			Actors: []node.PalworldMapActor{
				{
					Key:       "player-1",
					Kind:      node.PalworldMapActorKindPlayer,
					Name:      "Alex",
					LocationX: 123.5,
					LocationY: -456.25,
				},
			},
		},
	})

	state := inst.GetPalworldMapState("palworld-1")
	if state.Snapshot == nil || len(state.Snapshot.Actors) != 1 {
		t.Fatalf("GetPalworldMapState() = %+v", state)
	}
	actor := state.Snapshot.Actors[0]
	if actor.Name != "Alex" || actor.LocationX != 123.5 || actor.LocationY != -456.25 {
		t.Fatalf("GetPalworldMapState() actor = %+v", actor)
	}
	if state.Snapshot.Health == nil || state.Snapshot.Health.ServerFPS != 60 || state.Snapshot.Health.BaseCampCount != 3 {
		t.Fatalf("GetPalworldMapState() health = %+v", state.Snapshot.Health)
	}
	state.Snapshot.Actors[0].Name = "changed"
	state.Snapshot.Health.ServerFPS = 1
	unchanged := inst.GetPalworldMapState("palworld-1")
	if unchanged.Snapshot.Actors[0].Name != "Alex" {
		t.Fatal("GetPalworldMapState() returned mutable internal actor data")
	}
	if unchanged.Snapshot.Health == nil || unchanged.Snapshot.Health.ServerFPS != 60 {
		t.Fatal("GetPalworldMapState() returned mutable internal health data")
	}

	inst.storePalworldMapState(PalworldMapState{
		ServerID:          "palworld-1",
		ServerName:        "Palpagos",
		ServerOnline:      false,
		UnavailableReason: "offline",
	})
	stale := inst.GetPalworldMapState("palworld-1")
	if stale.Snapshot == nil || stale.Snapshot.Actors[0].Name != "Alex" || stale.UnavailableReason != "offline" {
		t.Fatalf("offline state = %+v", stale)
	}
}
