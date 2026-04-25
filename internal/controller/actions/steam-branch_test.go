package actions

import "testing"

func TestNormalizeSteamBranch(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty defaults to public", input: "", want: "public"},
		{name: "whitespace defaults to public", input: "   ", want: "public"},
		{name: "public stays public", input: "public", want: "public"},
		{name: "custom branch preserved", input: "latest_experimental", want: "latest_experimental"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeSteamBranch(tc.input)
			if got != tc.want {
				t.Fatalf("normalizeSteamBranch(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestAppendSteamBranchToUpdateCommand(t *testing.T) {
	t.Parallel()

	base := "steamcmd +login anonymous +app_update 294420 validate +quit"

	publicCmd := appendSteamBranchToUpdateCommand(base, "public")
	if publicCmd != base {
		t.Fatalf("appendSteamBranchToUpdateCommand(public) = %q, want unchanged %q", publicCmd, base)
	}

	experimentalCmd := appendSteamBranchToUpdateCommand(base, "latest_experimental")
	want := "steamcmd +login anonymous +app_update 294420 -beta latest_experimental validate +quit"
	if experimentalCmd != want {
		t.Fatalf("appendSteamBranchToUpdateCommand(latest_experimental) = %q, want %q", experimentalCmd, want)
	}
}
