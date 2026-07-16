//go:build windows

package main

import (
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

type windowsServiceExecutionResult struct {
	serviceSpecificExitCode bool
	exitCode                uint32
}

func TestWindowsServiceHandler(t *testing.T) {
	t.Run("graceful control requests", func(t *testing.T) {
		cases := []struct {
			name    string
			command svc.Cmd
		}{
			{name: "stop", command: svc.Stop},
			{name: "system shutdown", command: svc.Shutdown},
		}

		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				shutdownReceived := make(chan os.Signal, 1)
				handler := &windowsServiceHandler{
					run: func(shutdownSignals <-chan os.Signal) int {
						shutdownReceived <- <-shutdownSignals
						return 0
					},
				}

				requests := make(chan svc.ChangeRequest, 1)
				changes := make(chan svc.Status, 4)
				result := make(chan windowsServiceExecutionResult, 1)
				go func() {
					serviceSpecificExitCode, exitCode := handler.Execute(nil, requests, changes)
					result <- windowsServiceExecutionResult{
						serviceSpecificExitCode: serviceSpecificExitCode,
						exitCode:                exitCode,
					}
				}()

				assertWindowsServiceStatus(t, changes, svc.StartPending, 0)
				assertWindowsServiceStatus(t, changes, svc.Running, svc.AcceptStop|svc.AcceptShutdown)

				requests <- svc.ChangeRequest{Cmd: testCase.command}
				assertWindowsServiceStatus(t, changes, svc.StopPending, 0)

				select {
				case signal := <-shutdownReceived:
					if signal != os.Interrupt {
						t.Fatalf("shutdown signal = %v, want %v", signal, os.Interrupt)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("timed out waiting for service shutdown signal")
				}

				select {
				case executionResult := <-result:
					if executionResult.serviceSpecificExitCode {
						t.Fatal("graceful service stop returned a service-specific exit code")
					}
					if executionResult.exitCode != 0 {
						t.Fatalf("graceful service stop exit code = %d, want 0", executionResult.exitCode)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("timed out waiting for service handler to stop")
				}
			})
		}
	})

	t.Run("unexpected application exits trigger recovery", func(t *testing.T) {
		cases := []struct {
			name         string
			application  int
			wantExitCode uint32
		}{
			{name: "clean application exit", application: 0, wantExitCode: 1},
			{name: "application error", application: 7, wantExitCode: 7},
		}

		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				handler := &windowsServiceHandler{
					run: func(<-chan os.Signal) int {
						return testCase.application
					},
				}

				requests := make(chan svc.ChangeRequest)
				changes := make(chan svc.Status, 2)
				result := make(chan windowsServiceExecutionResult, 1)
				go func() {
					serviceSpecificExitCode, exitCode := handler.Execute(nil, requests, changes)
					result <- windowsServiceExecutionResult{
						serviceSpecificExitCode: serviceSpecificExitCode,
						exitCode:                exitCode,
					}
				}()

				assertWindowsServiceStatus(t, changes, svc.StartPending, 0)
				assertWindowsServiceStatus(t, changes, svc.Running, svc.AcceptStop|svc.AcceptShutdown)

				select {
				case executionResult := <-result:
					if !executionResult.serviceSpecificExitCode {
						t.Fatal("unexpected service exit did not return a service-specific exit code")
					}
					if executionResult.exitCode != testCase.wantExitCode {
						t.Fatalf("service exit code = %d, want %d", executionResult.exitCode, testCase.wantExitCode)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("timed out waiting for unexpected service exit")
				}
			})
		}
	})
}

func assertWindowsServiceStatus(t *testing.T, changes <-chan svc.Status, wantState svc.State, wantAccepts svc.Accepted) {
	t.Helper()

	select {
	case status := <-changes:
		if status.State != wantState {
			t.Fatalf("service state = %s, want %s", windowsServiceStateName(status.State), windowsServiceStateName(wantState))
		}
		if status.Accepts != wantAccepts {
			t.Fatalf("service accepted controls = %d, want %d", status.Accepts, wantAccepts)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for service state %s", windowsServiceStateName(wantState))
	}
}

func TestWindowsServiceStateName(t *testing.T) {
	cases := []struct {
		name  string
		state svc.State
		want  string
	}{
		{name: "stopped", state: svc.Stopped, want: "stopped"},
		{name: "starting", state: svc.StartPending, want: "starting"},
		{name: "stopping", state: svc.StopPending, want: "stopping"},
		{name: "running", state: svc.Running, want: "running"},
		{name: "resuming", state: svc.ContinuePending, want: "resuming"},
		{name: "pausing", state: svc.PausePending, want: "pausing"},
		{name: "paused", state: svc.Paused, want: "paused"},
		{name: "unknown", state: svc.State(99), want: "unknown (99)"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := windowsServiceStateName(testCase.state)
			if got != testCase.want {
				t.Fatalf("windowsServiceStateName(%d) = %q, want %q", testCase.state, got, testCase.want)
			}
		})
	}
}
