//go:build windows

package main

import (
	"context"
	"errors"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/appservice"
	"github.com/ClintonCollins/Xylona/internal/selfupdate"
)

func TestWindowsNodeServiceInstallChecksPrivilegesBeforePairing(t *testing.T) {
	originalRequireAccess := requireWindowsServiceManagerAccess
	originalPairNode := pairWindowsNodeForService
	t.Cleanup(func() {
		requireWindowsServiceManagerAccess = originalRequireAccess
		pairWindowsNodeForService = originalPairNode
	})

	errAccess := errors.New("administrator access required")
	requireWindowsServiceManagerAccess = func() error {
		return errAccess
	}
	pairCalled := false
	pairWindowsNodeForService = func(context.Context, *nodeServicePreparation) error {
		pairCalled = true
		return nil
	}

	command := newWindowsNodeServiceCommand()
	errRun := command.Run(t.Context(), []string{
		"service",
		"install",
		"--controller-url", "https://controller.test",
		"--join-token", "one-time-token",
	})
	if !errors.Is(errRun, errAccess) {
		t.Fatalf("service install error = %v, want privilege error", errRun)
	}
	if pairCalled {
		t.Fatal("service install paired the node before validating SCM access")
	}
}

func TestWindowsNodeServiceExitCode(t *testing.T) {
	t.Parallel()

	errRun := errors.New("node failed")
	tests := []struct {
		name          string
		errRun        error
		stopRequested bool
		restartMode   selfupdate.RestartMode
		want          int
	}{
		{
			name:        "planned Windows service update uses helper handoff",
			restartMode: selfupdate.RestartModeWindowsService,
			want:        appservice.UpdateHandoffExitCode,
		},
		{
			name:          "SCM stop remains a clean service stop",
			stopRequested: true,
			restartMode:   selfupdate.RestartModeWindowsService,
		},
		{
			name:        "non-service clean exit remains normal",
			restartMode: selfupdate.RestartModeSelf,
		},
		{
			name:        "runtime failure remains recovery eligible",
			errRun:      errRun,
			restartMode: selfupdate.RestartModeWindowsService,
			want:        1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := windowsNodeServiceExitCode(test.errRun, test.stopRequested, test.restartMode)
			if got != test.want {
				t.Fatalf("windowsNodeServiceExitCode() = %d, want %d", got, test.want)
			}
		})
	}
}
