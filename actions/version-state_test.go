package actions

import (
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
)

func TestInstanceVersionStateReturnsStoredMap(t *testing.T) {
	vsm := versiontracker.NewVersionStateMap()
	inst := &Instance{
		versionState: vsm,
	}

	got := inst.VersionState()
	if got != vsm {
		t.Fatalf("VersionState() returned %p, want %p", got, vsm)
	}
}

func TestInstanceVersionStateReturnsNilWhenUnset(t *testing.T) {
	inst := &Instance{}

	got := inst.VersionState()
	if got != nil {
		t.Fatalf("VersionState() = %p, want nil", got)
	}
}
