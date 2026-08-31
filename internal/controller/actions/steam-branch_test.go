package actions

import (
	"errors"
	"testing"

	"github.com/ClintonCollins/Xylona/sql/models"
)

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

	publicCmd, errPublic := appendSteamBranchToUpdateCommand(base, "public")
	if errPublic != nil {
		t.Fatalf("appendSteamBranchToUpdateCommand(public) error = %v", errPublic)
	}
	if publicCmd != base {
		t.Fatalf("appendSteamBranchToUpdateCommand(public) = %q, want unchanged %q", publicCmd, base)
	}

	experimentalCmd, errExperimental := appendSteamBranchToUpdateCommand(base, "latest_experimental")
	if errExperimental != nil {
		t.Fatalf("appendSteamBranchToUpdateCommand(latest_experimental) error = %v", errExperimental)
	}
	want := "steamcmd +login anonymous +app_update 294420 -beta latest_experimental validate +quit"
	if experimentalCmd != want {
		t.Fatalf("appendSteamBranchToUpdateCommand(latest_experimental) = %q, want %q", experimentalCmd, want)
	}
}

func TestAppendSteamBranchToUpdateCommandRejectsInjection(t *testing.T) {
	t.Parallel()

	base := "steamcmd +login anonymous +app_update 294420 validate +quit"
	testCases := []struct {
		name   string
		branch string
	}{
		{name: "new submission plus command", branch: "latest +force_install_dir /tmp/pwn"},
		{name: "already stored extra app update", branch: "public +app_update 0"},
		{name: "already stored quoted payload", branch: `latest"; +quit`},
		{name: "already stored control character", branch: "latest\n+quit"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, errAppend := appendSteamBranchToUpdateCommand(base, tc.branch)
			if !errors.Is(errAppend, ErrInvalidSteamBranch) {
				t.Fatalf("appendSteamBranchToUpdateCommand(%q) error = %v, want %v", tc.branch, errAppend, ErrInvalidSteamBranch)
			}
			if got != "" {
				t.Fatalf("appendSteamBranchToUpdateCommand(%q) = %q, want empty command on error", tc.branch, got)
			}
		})
	}
}

func TestPersistSteamBranchSelectionRejectsInjection(t *testing.T) {
	t.Parallel()

	inst := &Instance{}
	gameServer := &models.GameServer{ID: "server-1", Branch: "public"}
	errPersist := inst.PersistSteamBranchSelection(gameServer, "latest +force_install_dir /tmp/pwn")
	if !errors.Is(errPersist, ErrInvalidSteamBranch) {
		t.Fatalf("PersistSteamBranchSelection() error = %v, want %v", errPersist, ErrInvalidSteamBranch)
	}
	if gameServer.Branch != "public" {
		t.Fatalf("PersistSteamBranchSelection() Branch = %q, want unchanged %q", gameServer.Branch, "public")
	}
}

func TestPersistSteamBranchSelectionAcceptsSafeBranch(t *testing.T) {
	t.Parallel()

	inst := &Instance{}
	gameServer := &models.GameServer{ID: "server-1", Branch: "public"}
	errPersist := inst.PersistSteamBranchSelection(gameServer, "latest_experimental")
	if errPersist != nil {
		t.Fatalf("PersistSteamBranchSelection() error = %v", errPersist)
	}
	if gameServer.Branch != "latest_experimental" {
		t.Fatalf("PersistSteamBranchSelection() Branch = %q, want %q", gameServer.Branch, "latest_experimental")
	}
}
