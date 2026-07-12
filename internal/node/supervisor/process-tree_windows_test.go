//go:build windows

package supervisor

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const (
	windowsControlSignalRoleEnv   = "XYLONA_WINDOWS_CONTROL_SIGNAL_TEST"
	windowsControlSignalMarkerEnv = "XYLONA_WINDOWS_CONTROL_SIGNAL_MARKER"
)

func TestWindowsSupervisorGracefulInterrupt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Windows console control integration test in short mode")
	}
	executable, errExecutable := os.Executable()
	if errExecutable != nil {
		t.Fatalf("os.Executable() error = %v", errExecutable)
	}
	markerPath := filepath.Join(t.TempDir(), "control-signal.txt")
	instance, errNew := New(t.Context())
	if errNew != nil {
		t.Fatalf("New() error = %v", errNew)
	}
	command, errStart := instance.StartCommand(PreparedCommand{
		ID:          "windows-control-signal",
		BaseCommand: executable,
		Args:        []string{"-test.run=^TestWindowsControlSignalChild$"},
		Status:      xylona.Status_ONLINE,
		StopTimeout: 10 * time.Second,
		LaunchEnv: map[string]string{
			windowsControlSignalRoleEnv:   "child",
			windowsControlSignalMarkerEnv: markerPath,
		},
	})
	if errStart != nil {
		t.Fatalf("StartCommand() error = %v", errStart)
	}

	waitForWindowsControlMarker(t, markerPath, "ready", 5*time.Second)
	stopStarted := time.Now()
	command.Stop("")
	stopDuration := time.Since(stopStarted)
	if stopDuration >= 5*time.Second {
		t.Fatalf("Stop() took %v, want graceful return under 5s", stopDuration)
	}
	waitForWindowsControlMarker(t, markerPath, "stopped", 5*time.Second)
	if command.Status() != xylona.Status_OFFLINE {
		t.Fatalf("command status = %s, want OFFLINE", command.Status())
	}
}

func TestWindowsSupervisorConsolesAreIsolated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Windows console isolation integration test in short mode")
	}
	executable, errExecutable := os.Executable()
	if errExecutable != nil {
		t.Fatalf("os.Executable() error = %v", errExecutable)
	}
	instance, errNew := New(t.Context())
	if errNew != nil {
		t.Fatalf("New() error = %v", errNew)
	}

	startChild := func(id string, markerPath string) *Command {
		command, errStart := instance.StartCommand(PreparedCommand{
			ID:          id,
			BaseCommand: executable,
			Args:        []string{"-test.run=^TestWindowsControlSignalChild$"},
			Status:      xylona.Status_ONLINE,
			StopTimeout: 10 * time.Second,
			LaunchEnv: map[string]string{
				windowsControlSignalRoleEnv:   "child",
				windowsControlSignalMarkerEnv: markerPath,
			},
		})
		if errStart != nil {
			t.Fatalf("StartCommand(%q) error = %v", id, errStart)
		}
		return command
	}

	firstMarker := filepath.Join(t.TempDir(), "first.txt")
	secondMarker := filepath.Join(t.TempDir(), "second.txt")
	first := startChild("windows-console-isolation-first", firstMarker)
	second := startChild("windows-console-isolation-second", secondMarker)
	t.Cleanup(func() {
		if first.Status() != xylona.Status_OFFLINE {
			first.Stop("")
		}
		if second.Status() != xylona.Status_OFFLINE {
			second.Stop("")
		}
	})
	waitForWindowsControlMarker(t, firstMarker, "ready", 5*time.Second)
	waitForWindowsControlMarker(t, secondMarker, "ready", 5*time.Second)

	first.Stop("")
	waitForWindowsControlMarker(t, firstMarker, "stopped", 5*time.Second)
	time.Sleep(500 * time.Millisecond)
	if second.Status() != xylona.Status_ONLINE {
		t.Fatalf("second command status = %s after stopping first, want ONLINE", second.Status())
	}
	waitForWindowsControlMarker(t, secondMarker, "ready", time.Second)

	second.Stop("")
	waitForWindowsControlMarker(t, secondMarker, "stopped", 5*time.Second)
}

func TestWindowsSupervisorConsoleInput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Windows console input integration test in short mode")
	}
	executable, errExecutable := os.Executable()
	if errExecutable != nil {
		t.Fatalf("os.Executable() error = %v", errExecutable)
	}
	markerPath := filepath.Join(t.TempDir(), "console-input.txt")
	instance, errNew := New(t.Context())
	if errNew != nil {
		t.Fatalf("New() error = %v", errNew)
	}
	command, errStart := instance.StartCommand(PreparedCommand{
		ID:          "windows-console-input",
		BaseCommand: executable,
		Args:        []string{"-test.run=^TestWindowsConsoleInputChild$"},
		Status:      xylona.Status_ONLINE,
		StopTimeout: 10 * time.Second,
		LaunchEnv: map[string]string{
			windowsControlSignalRoleEnv:   "input-child",
			windowsControlSignalMarkerEnv: markerPath,
		},
	})
	if errStart != nil {
		t.Fatalf("StartCommand() error = %v", errStart)
	}

	waitForWindowsControlMarker(t, markerPath, "ready", 5*time.Second)
	errInput := command.SendInput("hello-console")
	if errInput != nil {
		t.Fatalf("SendInput() error = %v", errInput)
	}
	waitForWindowsControlMarker(t, markerPath, "input:hello-console", 5*time.Second)

	deadline := time.Now().Add(5 * time.Second)
	for command.Status() != xylona.Status_OFFLINE && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if command.Status() != xylona.Status_OFFLINE {
		t.Fatalf("command status = %s, want OFFLINE", command.Status())
	}
}

func TestPrepareProcessInvocation(t *testing.T) {
	tests := []struct {
		name        string
		baseCommand string
		args        []string
		wantCommand string
		wantArgs    []string
		wantErr     bool
	}{
		{
			name:        "native executable",
			baseCommand: `C:\server\game.exe`,
			args:        []string{"-port", "27015"},
			wantCommand: `C:\server\game.exe`,
			wantArgs:    []string{"-port", "27015"},
		},
		{
			name:        "batch launcher",
			baseCommand: `C:\server files\StartServer64.bat`,
			args:        []string{"-cachedir", "Zomboid"},
			wantCommand: "cmd.exe",
			wantArgs: []string{
				"/d",
				"/v:off",
				"/c",
				"call",
				`C:\server files\StartServer64.bat`,
				"-cachedir",
				"Zomboid",
			},
		},
		{
			name:        "unsafe batch argument",
			baseCommand: `C:\server\start.cmd`,
			args:        []string{"%PATH%"},
			wantErr:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotCommand, gotArgs, errPrepare := prepareProcessInvocation(test.baseCommand, test.args)
			if test.wantErr {
				if errPrepare == nil {
					t.Fatal("prepareProcessInvocation() error = nil, want error")
				}
				return
			}
			if errPrepare != nil {
				t.Fatalf("prepareProcessInvocation() error = %v", errPrepare)
			}
			if gotCommand != test.wantCommand {
				t.Errorf("prepareProcessInvocation() command = %q, want %q", gotCommand, test.wantCommand)
			}
			if !slices.Equal(gotArgs, test.wantArgs) {
				t.Errorf("prepareProcessInvocation() args = %q, want %q", gotArgs, test.wantArgs)
			}
		})
	}
}

func TestWindowsBatchInvocationRunsAndForwardsArguments(t *testing.T) {
	batchPath := filepath.Join(t.TempDir(), "echo argument.cmd")
	errWrite := os.WriteFile(batchPath, []byte("@echo off\r\necho batch-output:%~1\r\n"), 0o600)
	if errWrite != nil {
		t.Fatalf("write batch fixture: %v", errWrite)
	}
	baseCommand, args, errPrepare := prepareProcessInvocation(batchPath, []string{"hello world"})
	if errPrepare != nil {
		t.Fatalf("prepareProcessInvocation() error = %v", errPrepare)
	}
	command := exec.CommandContext(context.Background(), baseCommand, args...)
	output, errRun := command.CombinedOutput()
	if errRun != nil {
		t.Fatalf("batch command error = %v; output = %s", errRun, output)
	}
	if !strings.Contains(string(output), "batch-output:hello world") {
		t.Fatalf("batch output = %q, want forwarded argument", output)
	}
}

func TestWindowsControlSignalChild(t *testing.T) {
	if os.Getenv(windowsControlSignalRoleEnv) != "child" {
		return
	}
	markerPath := os.Getenv(windowsControlSignalMarkerEnv)
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)

	errReady := os.WriteFile(markerPath, []byte("ready"), 0o600) //nolint:gosec // Child test writes only to its parent-provided TempDir marker.
	if errReady != nil {
		t.Fatalf("write ready marker: %v", errReady)
	}
	select {
	case <-interrupts:
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for CTRL+C")
	}
	errStopped := os.WriteFile(markerPath, []byte("stopped"), 0o600) //nolint:gosec // Child test writes only to its parent-provided TempDir marker.
	if errStopped != nil {
		t.Fatalf("write stopped marker: %v", errStopped)
	}
}

func TestWindowsConsoleInputChild(t *testing.T) {
	if os.Getenv(windowsControlSignalRoleEnv) != "input-child" {
		return
	}
	markerPath := os.Getenv(windowsControlSignalMarkerEnv)
	consoleInput, errOpen := os.Open("CONIN$")
	if errOpen != nil {
		t.Fatalf("open CONIN$: %v", errOpen)
	}
	errReady := os.WriteFile(markerPath, []byte("ready"), 0o600) //nolint:gosec // Child test writes only to its parent-provided TempDir marker.
	if errReady != nil {
		errClose := consoleInput.Close()
		t.Fatalf("write ready marker: %v; close CONIN$: %v", errReady, errClose)
	}
	line, errRead := bufio.NewReader(consoleInput).ReadString('\n')
	errClose := consoleInput.Close()
	if errRead != nil || errClose != nil {
		t.Fatalf("read console input error = %v; close error = %v", errRead, errClose)
	}
	errInput := os.WriteFile(markerPath, []byte("input:"+strings.TrimSpace(line)), 0o600) //nolint:gosec // Child test writes only to its parent-provided TempDir marker.
	if errInput != nil {
		t.Fatalf("write input marker: %v", errInput)
	}
}

func waitForWindowsControlMarker(t *testing.T, markerPath string, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		contents, errRead := os.ReadFile(markerPath)
		if errRead == nil && strings.TrimSpace(string(contents)) == want {
			return
		}
		if errRead != nil && !os.IsNotExist(errRead) {
			t.Fatalf("read control marker: %v", errRead)
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("control marker did not become %q within %v", want, timeout)
}
