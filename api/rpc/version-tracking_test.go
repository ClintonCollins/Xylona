package rpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestGetVersionInfoRequiresAuthentication(t *testing.T) {
	states := versiontracker.NewVersionStateMap()
	states.Set("server-1", versiontracker.VersionState{
		Status: versiontracker.VersionStatusChecked,
	})

	service := &XylonaService{
		versionState: states,
	}

	_, errGet := service.GetVersionInfo(
		context.Background(),
		connect.NewRequest(&xylona.GetVersionInfoRequest{GameServerId: "server-1"}),
	)
	if connect.CodeOf(errGet) != connect.CodeUnauthenticated {
		t.Fatalf("GetVersionInfo() code = %v, want %v", connect.CodeOf(errGet), connect.CodeUnauthenticated)
	}
}

func TestCheckForUpdateRequiresAuthentication(t *testing.T) {
	service := &XylonaService{}

	_, errCheck := service.CheckForUpdate(
		context.Background(),
		connect.NewRequest(&xylona.CheckForUpdateRequest{GameServerId: "server-1"}),
	)
	if connect.CodeOf(errCheck) != connect.CodeUnauthenticated {
		t.Fatalf("CheckForUpdate() code = %v, want %v", connect.CodeOf(errCheck), connect.CodeUnauthenticated)
	}
}

func TestGetVersionInfoRequiresViewPermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.service.versionState = versiontracker.NewVersionStateMap()
	fixture.service.versionState.Set("server-local-1", versiontracker.VersionState{
		Status: versiontracker.VersionStatusChecked,
	})

	request := connect.NewRequest(&xylona.GetVersionInfoRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-other")

	_, errGet := fixture.service.GetVersionInfo(context.Background(), request)
	if connect.CodeOf(errGet) != connect.CodePermissionDenied {
		t.Fatalf("GetVersionInfo() code = %v, want %v", connect.CodeOf(errGet), connect.CodePermissionDenied)
	}
}

func TestCheckForUpdateRequiresViewPermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.service.versionState = versiontracker.NewVersionStateMap()

	request := connect.NewRequest(&xylona.CheckForUpdateRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-other")

	_, errCheck := fixture.service.CheckForUpdate(context.Background(), request)
	if connect.CodeOf(errCheck) != connect.CodePermissionDenied {
		t.Fatalf("CheckForUpdate() code = %v, want %v", connect.CodeOf(errCheck), connect.CodePermissionDenied)
	}
}

func TestSetDummyUpdateFailureRequiresAuthentication(t *testing.T) {
	service := &XylonaService{
		dummyTracker: versiontracker.NewDummyTracker(),
		versionState: versiontracker.NewVersionStateMap(),
	}

	_, errSet := service.SetDummyUpdateFailure(
		context.Background(),
		connect.NewRequest(&xylona.SetDummyUpdateFailureRequest{
			SimulateFailure: true,
		}),
	)
	if connect.CodeOf(errSet) != connect.CodeUnauthenticated {
		t.Fatalf("SetDummyUpdateFailure() code = %v, want %v", connect.CodeOf(errSet), connect.CodeUnauthenticated)
	}
}

func TestClearDummyVersionStatesOnlyRemovesDummyEntries(t *testing.T) {
	tracker := versiontracker.NewDummyTracker()
	tracker.MarkUpdated()
	states := versiontracker.NewVersionStateMap()
	states.Set("server-1", versiontracker.VersionState{
		Status:           versiontracker.VersionStatusChecked,
		InstalledVersion: "2.0.0",
		LatestVersion:    "2.0.0",
		TrackerType:      "dummy",
	})
	states.Set("server-2", versiontracker.VersionState{
		Status:           versiontracker.VersionStatusChecked,
		InstalledVersion: "1.21.4",
		LatestVersion:    "1.21.4",
		TrackerType:      "minecraft",
	})

	tracker.Reset()
	tracker.SetSimulateFailure(true)
	clearDummyVersionStates(states)

	installedVersion, errInstalled := tracker.GetInstalledVersion(context.Background(), nil)
	if errInstalled != nil {
		t.Fatalf("GetInstalledVersion() error = %v", errInstalled)
	}
	if installedVersion != "1.0.0" {
		t.Errorf("installed version = %q, want %q", installedVersion, "1.0.0")
	}
	if !tracker.SimulateFailure() {
		t.Error("SimulateFailure() = false, want true")
	}
	if state := states.Get("server-1"); state.Status != versiontracker.VersionStatusNoTracker {
		t.Errorf("version state after reset = %+v, want deleted/no tracker", state)
	}
	if state := states.Get("server-2"); state.Status != versiontracker.VersionStatusChecked {
		t.Errorf("non-dummy state after reset = %+v, want preserved checked state", state)
	}
}
