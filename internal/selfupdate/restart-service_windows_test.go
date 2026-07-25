//go:build windows

package selfupdate

import (
	"errors"
	"slices"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

type fakeWindowsServiceRestarter struct {
	statuses     []svc.Status
	queryIndex   int
	queryErr     error
	controlCalls []svc.Cmd
	startCalls   int
}

func (f *fakeWindowsServiceRestarter) Query() (svc.Status, error) {
	if f.queryErr != nil {
		return svc.Status{}, f.queryErr
	}
	if f.queryIndex >= len(f.statuses) {
		return f.statuses[len(f.statuses)-1], nil
	}
	status := f.statuses[f.queryIndex]
	f.queryIndex++
	return status, nil
}

func (f *fakeWindowsServiceRestarter) Control(command svc.Cmd) (svc.Status, error) {
	f.controlCalls = append(f.controlCalls, command)
	return svc.Status{}, nil
}

func (f *fakeWindowsServiceRestarter) Start(...string) error {
	f.startCalls++
	return nil
}

func TestRestartOpenedWindowsService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		statuses     []svc.Status
		wantControls []svc.Cmd
	}{
		{
			name: "running service is stopped before starting updated binary",
			statuses: []svc.Status{
				{State: svc.Running},
				{State: svc.Stopped},
				{State: svc.Running},
			},
			wantControls: []svc.Cmd{svc.Stop},
		},
		{
			name: "stopped service is started directly",
			statuses: []svc.Status{
				{State: svc.Stopped},
				{State: svc.Running},
			},
		},
		{
			name: "recovery start is allowed to finish and then stopped",
			statuses: []svc.Status{
				{State: svc.StartPending},
				{State: svc.Running},
				{State: svc.Stopped},
				{State: svc.Running},
			},
			wantControls: []svc.Cmd{svc.Stop},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service := &fakeWindowsServiceRestarter{statuses: test.statuses}
			errRestart := restartOpenedWindowsService(
				service,
				"XylonaNode",
				time.Now().Add(5*time.Second),
			)
			if errRestart != nil {
				t.Fatalf("restartOpenedWindowsService() error = %v", errRestart)
			}
			if !slices.Equal(service.controlCalls, test.wantControls) {
				t.Fatalf("control calls = %v, want %v", service.controlCalls, test.wantControls)
			}
			if service.startCalls != 1 {
				t.Fatalf("start calls = %d, want 1", service.startCalls)
			}
		})
	}
}

func TestClassifyWindowsServiceRestartError(t *testing.T) {
	t.Parallel()

	errRestart := errors.New("restart failed")
	errQuery := errors.New("query failed")
	tests := []struct {
		name       string
		service    *fakeWindowsServiceRestarter
		wantUnsafe bool
	}{
		{
			name: "stopped service permits executable rollback",
			service: &fakeWindowsServiceRestarter{
				statuses: []svc.Status{{State: svc.Stopped}},
			},
		},
		{
			name: "running service prevents executable rollback",
			service: &fakeWindowsServiceRestarter{
				statuses: []svc.Status{{State: svc.Running}},
			},
			wantUnsafe: true,
		},
		{
			name: "unknown service state prevents executable rollback",
			service: &fakeWindowsServiceRestarter{
				queryErr: errQuery,
			},
			wantUnsafe: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := classifyWindowsServiceRestartError(test.service, errRestart)
			if !errors.Is(got, errRestart) {
				t.Fatalf("classified error = %v, want original restart error", got)
			}
			if errors.Is(got, errServiceRestartRollbackUnsafe) != test.wantUnsafe {
				t.Fatalf(
					"classified unsafe = %t, want %t: %v",
					errors.Is(got, errServiceRestartRollbackUnsafe),
					test.wantUnsafe,
					got,
				)
			}
		})
	}
}
