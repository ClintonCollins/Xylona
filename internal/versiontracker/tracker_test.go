package versiontracker

import "testing"

func TestTrackerTypeName(t *testing.T) {
	tests := []struct {
		name    string
		tracker VersionTracker
		want    string
	}{
		{name: "nil tracker", tracker: nil, want: ""},
		{name: "dummy tracker", tracker: NewDummyTracker(), want: "dummy"},
		{name: "minecraft tracker", tracker: NewMinecraftTracker(), want: "minecraft"},
		{name: "steam tracker", tracker: NewSteamTracker(), want: "steam"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := TrackerTypeName(tc.tracker); got != tc.want {
				t.Fatalf("TrackerTypeName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVersionsEqualIgnoresWhitespace(t *testing.T) {
	if !versionsEqual(" 1.21.4 \n", "\t1.21.4 ") {
		t.Fatal("versionsEqual() = false, want true for whitespace-only differences")
	}
}

func TestVersionsEqualDetectsDifferentVersions(t *testing.T) {
	if versionsEqual("1.21.3", "1.21.4") {
		t.Fatal("versionsEqual() = true, want false for different versions")
	}
}

func TestResolveTracker_SteamCMDUsesAppUpdateID(t *testing.T) {
	tracker := ResolveTrackerWithContext(ResolverConfig{}, TrackerContext{
		GameID:         "7_days_to_die",
		UpdateCommand:  "steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update 294420 validate +quit",
		ServerSoftware: "",
	})

	steamTracker, ok := tracker.(*SteamTracker)
	if !ok {
		t.Fatalf("ResolveTrackerWithContext() type = %T, want *SteamTracker", tracker)
	}
	if steamTracker.preferredAppID != "294420" {
		t.Fatalf("preferredAppID = %q, want %q", steamTracker.preferredAppID, "294420")
	}
}
