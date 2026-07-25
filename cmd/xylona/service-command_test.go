package main

import (
	"testing"

	"github.com/ClintonCollins/Xylona/internal/appservice"
	"github.com/ClintonCollins/Xylona/internal/selfupdate"
)

func TestControllerServiceExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		updateRequested bool
		restartMode     selfupdate.RestartMode
		want            int
	}{
		{
			name:            "planned Windows service update uses helper handoff",
			updateRequested: true,
			restartMode:     selfupdate.RestartModeWindowsService,
			want:            appservice.UpdateHandoffExitCode,
		},
		{
			name:        "ordinary Windows service exit remains normal",
			restartMode: selfupdate.RestartModeWindowsService,
		},
		{
			name:            "foreground update remains normal",
			updateRequested: true,
			restartMode:     selfupdate.RestartModeSelf,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := controllerServiceExitCode(test.updateRequested, test.restartMode)
			if got != test.want {
				t.Fatalf("controllerServiceExitCode() = %d, want %d", got, test.want)
			}
		})
	}
}
