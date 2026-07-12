package supervisor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const pseudoTerminalTestRoleEnv = "XYLONA_PSEUDO_TERMINAL_TEST_ROLE"

type internalGameStub struct {
	installErr error
	updateErr  error
}

func (g *internalGameStub) Install(_ *models.GameServer, _, _ io.Writer) error {
	return g.installErr
}

func (g *internalGameStub) Update(_ *models.GameServer, _, _ io.Writer) error {
	return g.updateErr
}

type blockingInternalGameStub struct {
	started chan struct{}
	release chan struct{}
}

func (g *blockingInternalGameStub) Install(_ *models.GameServer, _, _ io.Writer) error {
	return nil
}

func (g *blockingInternalGameStub) Update(_ *models.GameServer, _, _ io.Writer) error {
	close(g.started)
	<-g.release
	return nil
}

func echoCommandArgs(output string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "echo", output}
	}
	return "echo", []string{output}
}

func shellCommandArgs(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", command}
	}
	return "sh", []string{"-c", command}
}

func launchEnvProbeCommand(key string) (string, []string) {
	if runtime.GOOS == "windows" {
		return shellCommandArgs("echo %" + key + "% & ping -n 3 127.0.0.1 >NUL")
	}
	return shellCommandArgs("printf '%s\n' \"$" + key + "\"; sleep 2")
}

func TestStartCommand(t *testing.T) {
	ctx := t.Context()

	inst, _ := New(ctx)

	baseCommand, args := echoCommandArgs("hello xylona")

	pc := PreparedCommand{
		ID:          "test-cmd",
		BaseCommand: baseCommand,
		Args:        args,
		Status:      xylona.Status_ONLINE,
	}

	cmd, err := inst.StartCommand(pc)
	if err != nil {
		t.Fatalf("Failed to start command: %v", err)
	}

	// Wait for output
	timeout := time.After(5 * time.Second)
	found := false
	for !found {
		select {
		case <-timeout:
			t.Fatal("Timed out waiting for output")
		default:
			if strings.Contains(cmd.GetOutputBuffer(), "hello xylona") {
				found = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	cmd.Stop("")

	if cmd.Status() != xylona.Status_OFFLINE {
		t.Errorf("Expected status OFFLINE after stop, got %v", cmd.Status())
	}
}

func TestStartCommandReturnsProcessStartFailurePromptly(t *testing.T) {
	inst, errNew := New(t.Context())
	if errNew != nil {
		t.Fatalf("New() error = %v", errNew)
	}
	startedAt := time.Now()
	_, errStart := inst.StartCommand(PreparedCommand{
		ID:          "missing-executable",
		BaseCommand: filepath.Join(t.TempDir(), "missing-executable"),
		Status:      xylona.Status_ONLINE,
	})
	duration := time.Since(startedAt)
	if errStart == nil {
		t.Fatal("StartCommand() error = nil, want process start failure")
	}
	if duration >= 2*time.Second {
		t.Fatalf("StartCommand() took %v, want failure before output drain timeout", duration)
	}
}

func TestPseudoTerminalConsole(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping pseudo-terminal process integration test in short mode")
	}
	executable, errExecutable := os.Executable()
	if errExecutable != nil {
		t.Fatalf("os.Executable() error = %v", errExecutable)
	}
	inst, errNew := New(t.Context())
	if errNew != nil {
		t.Fatalf("New() error = %v", errNew)
	}
	command, errStart := inst.StartCommand(PreparedCommand{
		ID:          "pseudo-terminal-console",
		BaseCommand: executable,
		Args:        []string{"-test.run=^TestPseudoTerminalConsoleChild$"},
		ServiceID:   "terraria",
		Status:      xylona.Status_ONLINE,
		StopTimeout: 10 * time.Second,
		LaunchEnv: map[string]string{
			pseudoTerminalTestRoleEnv: "child",
		},
	})
	if errStart != nil {
		t.Fatalf("StartCommand() error = %v", errStart)
	}

	waitForCommandOutput(t, command, "pty-ready", 5*time.Second)
	stopStarted := time.Now()
	command.Stop("exit")
	stopDuration := time.Since(stopStarted)
	if stopDuration >= 5*time.Second {
		t.Fatalf("Stop() took %v, want graceful return under 5s", stopDuration)
	}
	if command.Status() != xylona.Status_OFFLINE {
		t.Fatalf("command status = %s, want OFFLINE", command.Status())
	}
	output := command.GetOutputBuffer()
	if !strings.Contains(output, "pty-input:exit") {
		t.Fatalf("output buffer = %q, want console input acknowledgement", output)
	}
	if !strings.Contains(output, "pty-final") {
		t.Fatalf("output buffer = %q, want final drained output", output)
	}
}

func TestPseudoTerminalConsoleChild(t *testing.T) {
	if os.Getenv(pseudoTerminalTestRoleEnv) != "child" {
		return
	}
	_, errReady := fmt.Fprintln(os.Stdout, "pty-ready")
	if errReady != nil {
		t.Fatalf("write ready output: %v", errReady)
	}
	line, errRead := bufio.NewReader(os.Stdin).ReadString('\n')
	if errRead != nil {
		t.Fatalf("read pseudo-terminal input: %v", errRead)
	}
	_, errInput := fmt.Fprintf(os.Stdout, "pty-input:%s\n", strings.TrimSpace(line))
	if errInput != nil {
		t.Fatalf("write input acknowledgement: %v", errInput)
	}
	_, errFinal := fmt.Fprintln(os.Stdout, "pty-final")
	if errFinal != nil {
		t.Fatalf("write final output: %v", errFinal)
	}
}

func waitForCommandOutput(t *testing.T, command *Command, expected string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(command.GetOutputBuffer(), expected) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("command output did not contain %q within %v; output = %q", expected, timeout, command.GetOutputBuffer())
}

func TestCommandOutputListener(t *testing.T) {
	ctx := t.Context()

	inst, _ := New(ctx)

	baseCommand, args := echoCommandArgs("test-output")

	pc := PreparedCommand{
		ID:          "test-listener",
		BaseCommand: baseCommand,
		Args:        args,
		Status:      xylona.Status_ONLINE,
	}

	cmd, _ := inst.StartCommand(pc)

	outChan := make(chan *xylona.Message, 10)
	cmd.AddOutputListener("test-l", outChan)
	defer cmd.RemoveOutputListener("test-l")

	timeout := time.After(5 * time.Second)
	found := false
	for !found {
		select {
		case msg := <-outChan:
			if msg.GetGameServerConsoleOutput() != nil && strings.Contains(msg.GetGameServerConsoleOutput().GetOutput(), "test-output") {
				found = true
			}
		case <-timeout:
			t.Fatal("Timed out waiting for listener message")
		}
	}
}

func TestLaunchEnvReachesChildAndIsClearedAfterStart(t *testing.T) {
	ctx := t.Context()

	inst, errNew := New(ctx)
	if errNew != nil {
		t.Fatalf("failed to create supervisor: %v", errNew)
	}

	const envKey = "XYLONA_LAUNCH_ENV_TEST"
	const envValue = "launch-value"
	baseCommand, args := launchEnvProbeCommand(envKey)

	cmd, errStart := inst.StartCommand(PreparedCommand{
		ID:          "launch-env-running-test",
		BaseCommand: baseCommand,
		Args:        args,
		Status:      xylona.Status_ONLINE,
		LaunchEnv: map[string]string{
			envKey: envValue,
		},
	})
	if errStart != nil {
		t.Fatalf("StartCommand() error = %v", errStart)
	}
	defer cmd.Stop("")

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(cmd.GetOutputBuffer(), envValue) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(cmd.GetOutputBuffer(), envValue) {
		t.Fatalf("output buffer = %q, want launch env value", cmd.GetOutputBuffer())
	}

	for time.Now().Before(deadline) {
		cmd.RLock()
		currentCMD := cmd.currentCMD
		envCleared := currentCMD != nil && currentCMD.Env == nil
		cmd.RUnlock()
		if envCleared {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("exec.Cmd.Env was not cleared while process was still tracked")
}

func TestStartCommandRejectsInvalidTelnetInput(t *testing.T) {
	baseCommand, args := echoCommandArgs("not-started")
	tests := []struct {
		name        string
		inputMethod InputMethod
		wantErr     error
	}{
		{
			name: "missing credentials",
			inputMethod: InputMethod{
				Type: InputTypeTelnet,
			},
			wantErr: ErrTelnetCredentialsRequired,
		},
		{
			name: "missing port",
			inputMethod: InputMethod{
				Type: InputTypeTelnet,
				TelnetCredentials: &TelnetCredentials{
					Port: 0,
				},
			},
			wantErr: ErrTelnetPortRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst, errNew := New(t.Context())
			if errNew != nil {
				t.Fatalf("failed to create supervisor: %v", errNew)
			}

			_, errStart := inst.StartCommand(PreparedCommand{
				ID:          "invalid-telnet-input",
				BaseCommand: baseCommand,
				Args:        args,
				Status:      xylona.Status_ONLINE,
				InputMethod: tc.inputMethod,
			})
			if !errors.Is(errStart, tc.wantErr) {
				t.Fatalf("StartCommand() error = %v, want %v", errStart, tc.wantErr)
			}

			_, errGet := inst.GetCommandByID("invalid-telnet-input")
			if !errors.Is(errGet, ErrCommandDoesNotExist) {
				t.Fatalf("GetCommandByID() error = %v, want %v", errGet, ErrCommandDoesNotExist)
			}
		})
	}
}

func TestSetupCmdClearsStaleTelnetStartupHookOnReuse(t *testing.T) {
	inst, errNew := New(t.Context())
	if errNew != nil {
		t.Fatalf("failed to create supervisor: %v", errNew)
	}

	baseCommand, args := echoCommandArgs("stdin-reuse")
	persistentCommand := inst.initNewCommand(PreparedCommand{
		ID:          "reused-input-method",
		BaseCommand: baseCommand,
		Args:        args,
		Status:      xylona.Status_ONLINE,
		InputMethod: InputMethod{
			Type: InputTypeTelnet,
			TelnetCredentials: &TelnetCredentials{
				Port: 1,
			},
		},
	}, nil)
	persistentCommand.runAfterStartup = connectTelnetAndSetAsStdinWriter

	preparedCommand := PreparedCommand{
		ID:          "reused-input-method",
		BaseCommand: baseCommand,
		Args:        args,
		Status:      xylona.Status_ONLINE,
	}
	reusedCommand := inst.initNewCommand(preparedCommand, persistentCommand)

	_, errSetup := inst.setupCmd(reusedCommand, preparedCommand)
	if errSetup != nil {
		t.Fatalf("setupCmd() error = %v", errSetup)
	}

	closer, ok := reusedCommand.stdInWriter.(io.Closer)
	if ok {
		t.Cleanup(func() {
			errClose := closer.Close()
			if errClose != nil {
				t.Errorf("close stdin pipe: %v", errClose)
			}
		})
	}

	if reusedCommand.runAfterStartup != nil {
		t.Fatal("expected stale telnet startup hook to be cleared for stdin reuse")
	}
}

func TestConnectTelnetAndSetAsStdinWriterSkipsNonTelnetInput(t *testing.T) {
	stdInWriter := &strings.Builder{}
	cmd := &Command{
		ID:               "non-telnet-connect",
		stdInWriter:      stdInWriter,
		instanceCtx:      t.Context(),
		processCtx:       t.Context(),
		RWMutex:          &sync.RWMutex{},
		toggleOutputType: make(chan struct{}),
	}

	connectTelnetAndSetAsStdinWriter(cmd)

	if cmd.stdInWriter != stdInWriter {
		t.Fatalf("stdInWriter = %T, want original writer", cmd.stdInWriter)
	}
}

func TestConnectTelnetAndSetAsStdinWriterDiscardsInvalidTelnetInput(t *testing.T) {
	tests := []struct {
		name        string
		inputMethod InputMethod
	}{
		{
			name: "missing credentials",
			inputMethod: InputMethod{
				Type: InputTypeTelnet,
			},
		},
		{
			name: "missing port",
			inputMethod: InputMethod{
				Type: InputTypeTelnet,
				TelnetCredentials: &TelnetCredentials{
					Port: 0,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &Command{
				ID:               "invalid-telnet-connect",
				inputMethod:      tc.inputMethod,
				stdInWriter:      &strings.Builder{},
				instanceCtx:      t.Context(),
				processCtx:       t.Context(),
				RWMutex:          &sync.RWMutex{},
				toggleOutputType: make(chan struct{}),
			}

			connectTelnetAndSetAsStdinWriter(cmd)

			if cmd.stdInWriter != io.Discard {
				t.Fatalf("stdInWriter = %T, want io.Discard", cmd.stdInWriter)
			}
		})
	}
}

func TestInitNewCommandPreservesInjectedOutputOnFirstReuse(t *testing.T) {
	ctx := context.Background()

	inst, errNew := New(ctx)
	if errNew != nil {
		t.Fatalf("failed to create supervisor: %v", errNew)
	}

	inst.SendConsoleOutput("buffered-server", "Stopping server")

	persistentCommand, errGet := inst.GetCommandByID("buffered-server")
	if errGet != nil {
		t.Fatalf("failed to get synthetic shell command: %v", errGet)
	}

	initialBuffer := persistentCommand.GetOutputBuffer()
	if !strings.Contains(initialBuffer, "Stopping server") {
		t.Fatalf("expected injected output to exist before reuse, got %q", initialBuffer)
	}
	if !persistentCommand.preserveBufferedOutputOnReuse {
		t.Fatal("expected preserve flag to be set after injecting offline console output")
	}

	reusedCommand := inst.initNewCommand(
		PreparedCommand{
			ID:          "buffered-server",
			BaseCommand: "echo",
			Args:        []string{"restarted"},
			Status:      xylona.Status_UPDATING,
		},
		persistentCommand,
	)

	if reusedCommand.GetOutputBuffer() != initialBuffer {
		t.Fatalf("expected output buffer to survive first reuse, got %q want %q", reusedCommand.GetOutputBuffer(), initialBuffer)
	}
	if reusedCommand.preserveBufferedOutputOnReuse {
		t.Fatal("expected preserve flag to be cleared after the first reuse")
	}
}

func TestInitNewCommandClearsBufferWhenReuseIsNotPreserved(t *testing.T) {
	ctx := context.Background()

	inst, errNew := New(ctx)
	if errNew != nil {
		t.Fatalf("failed to create supervisor: %v", errNew)
	}

	persistentCommand := inst.GetCommandByIDOrCreateShell("normal-reuse")
	persistentCommand.pushToOutputBuffer("stale output")

	reusedCommand := inst.initNewCommand(
		PreparedCommand{
			ID:          "normal-reuse",
			BaseCommand: "echo",
			Args:        []string{"restarted"},
			Status:      xylona.Status_ONLINE,
		},
		persistentCommand,
	)

	if reusedCommand.GetOutputBuffer() != "" {
		t.Fatalf("expected output buffer to be cleared for ordinary reuse, got %q", reusedCommand.GetOutputBuffer())
	}
}

func TestInitNewCommandUpdatesGameServerNameOnReuse(t *testing.T) {
	ctx := context.Background()

	inst, errNew := New(ctx)
	if errNew != nil {
		t.Fatalf("failed to create supervisor: %v", errNew)
	}

	persistentCommand := inst.GetCommandByIDOrCreateShell("named-reuse")

	reusedCommand := inst.initNewCommand(
		PreparedCommand{
			ID:             "named-reuse",
			GameServerName: "Alpha",
			BaseCommand:    "echo",
			Args:           []string{"restarted"},
			Status:         xylona.Status_ONLINE,
		},
		persistentCommand,
	)

	if reusedCommand.gameServerName != "Alpha" {
		t.Fatalf("gameServerName = %q, want %q", reusedCommand.gameServerName, "Alpha")
	}
}

func TestCrashEventPublished(t *testing.T) {
	ctx := t.Context()

	inst, errNew := New(ctx)
	if errNew != nil {
		t.Fatalf("failed to create supervisor: %v", errNew)
	}

	eb := eventbus.Get()
	crashCh := eb.SubscribeReliable(eventbus.TopicGameServerCrashed)
	defer eb.Unsubscribe(eventbus.TopicGameServerCrashed, crashCh)

	// Command that prints output then exits with non-zero exit code.
	baseCommand, args := shellCommandArgs("echo crashing && exit 42")

	// Use a callback to know when the process finishes, and use
	// prepareCommandProcess directly to avoid a pre-existing race
	// in StartCommand when very-short-lived processes exit before
	// StartCommand can read currentCMD.
	done := make(chan struct{})
	pc := PreparedCommand{
		ID:          "crash-test-server",
		NodeID:      "test-node-1",
		BaseCommand: baseCommand,
		Args:        args,
		Status:      xylona.Status_ONLINE,
		CallbackFunction: func(_ *Command) {
			close(done)
		},
	}

	_, errStart := inst.prepareCommandProcess(pc)
	if errStart != nil {
		t.Fatalf("failed to start command: %v", errStart)
	}

	// Drain events until we find one for our specific server, skipping
	// events from other tests that share the process-global event bus.
	deadline := time.After(10 * time.Second)
	for {
		select {
		case evt := <-crashCh:
			crashEvt, ok := evt.(eventbus.ServerCrashedEvent)
			if !ok {
				t.Fatalf("expected ServerCrashedEvent, got %T", evt)
			}
			if crashEvt.ServerID != "crash-test-server" {
				continue // Skip events from other tests.
			}
			if crashEvt.ServerNodeID != "test-node-1" {
				t.Errorf("expected ServerNodeID %q, got %q", "test-node-1", crashEvt.ServerNodeID)
			}
			if crashEvt.ExitCode == 0 {
				t.Errorf("expected non-zero exit code, got %d", crashEvt.ExitCode)
			}
			if crashEvt.Timestamp.IsZero() {
				t.Error("expected non-zero timestamp")
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for crash event")
		}
	}
}

func TestNoCrashEventOnCleanExit(t *testing.T) {
	ctx := t.Context()

	inst, errNew := New(ctx)
	if errNew != nil {
		t.Fatalf("failed to create supervisor: %v", errNew)
	}

	eb := eventbus.Get()
	crashCh := eb.SubscribeReliable(eventbus.TopicGameServerCrashed)
	defer eb.Unsubscribe(eventbus.TopicGameServerCrashed, crashCh)

	// Command that prints output then exits cleanly with exit code 0.
	baseCommand, args := shellCommandArgs("echo clean && exit 0")

	// Use prepareCommandProcess directly to avoid a pre-existing race
	// in StartCommand when very-short-lived processes exit before
	// StartCommand can read currentCMD.
	done := make(chan struct{})
	pc := PreparedCommand{
		ID:          "clean-exit-server",
		NodeID:      "test-node-2",
		BaseCommand: baseCommand,
		Args:        args,
		Status:      xylona.Status_ONLINE,
		CallbackFunction: func(_ *Command) {
			close(done)
		},
	}

	_, errStart := inst.prepareCommandProcess(pc)
	if errStart != nil {
		t.Fatalf("failed to start command: %v", errStart)
	}

	// Wait for the process to finish, then confirm no crash event was published.
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for process to finish")
	}

	// Drain any events for a brief window and verify none belong to our server.
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case evt := <-crashCh:
			crashEvt, ok := evt.(eventbus.ServerCrashedEvent)
			if ok && crashEvt.ServerID == "clean-exit-server" {
				t.Fatalf("unexpected crash event on clean exit: %+v", crashEvt)
			}
			// Skip events from other tests.
		case <-timer.C:
			return // Expected — no crash event for a clean exit.
		}
	}
}

func TestStatusChangeEventPublished(t *testing.T) {
	ctx := t.Context()

	inst, errNew := New(ctx)
	if errNew != nil {
		t.Fatalf("failed to create supervisor: %v", errNew)
	}

	eb := eventbus.Get()
	statusCh := eb.SubscribeReliable(eventbus.TopicGameServerStatusChanged)
	defer eb.Unsubscribe(eventbus.TopicGameServerStatusChanged, statusCh)

	// Command that exits quickly — we should see status transitions.
	baseCommand, args := echoCommandArgs("status-test")

	// Use prepareCommandProcess directly to avoid a pre-existing race
	// in StartCommand when very-short-lived processes exit before
	// StartCommand can read currentCMD.
	done := make(chan struct{})
	pc := PreparedCommand{
		ID:          "status-test-server",
		NodeID:      "test-node-3",
		BaseCommand: baseCommand,
		Args:        args,
		Status:      xylona.Status_ONLINE,
		CallbackFunction: func(_ *Command) {
			close(done)
		},
	}

	_, errStart := inst.prepareCommandProcess(pc)
	if errStart != nil {
		t.Fatalf("failed to start command: %v", errStart)
	}

	// Collect status change events. We expect exactly:
	// 1. OFFLINE -> ONLINE (server started)
	// 2. ONLINE -> OFFLINE (server stopped)
	events := make([]eventbus.StatusChangedEvent, 0)
	deadline := time.After(10 * time.Second)
	var drain <-chan time.Time
	var drainTimer *time.Timer
	defer func() {
		if drainTimer != nil {
			drainTimer.Stop()
		}
	}()

	for {
		select {
		case evt := <-statusCh:
			statusEvt, ok := evt.(eventbus.StatusChangedEvent)
			if !ok {
				t.Fatalf("expected StatusChangedEvent, got %T", evt)
			}
			if statusEvt.ServerID != "status-test-server" {
				// Skip events from other tests running in parallel.
				continue
			}
			events = append(events, statusEvt)
		case <-done:
			drainTimer = time.NewTimer(300 * time.Millisecond)
			drain = drainTimer.C
			done = nil
		case <-drain:
			goto done
		case <-deadline:
			t.Fatalf("timed out waiting for status events; received %d events: %+v", len(events), events)
		}
	}

done:
	if len(events) != 2 {
		t.Fatalf("expected exactly 2 status change events, got %d: %+v", len(events), events)
	}

	// Verify the startup transition.
	startEvt := events[0]
	if startEvt.OldStatus != xylona.Status_OFFLINE.String() {
		t.Errorf("expected first event OldStatus %q, got %q", xylona.Status_OFFLINE.String(), startEvt.OldStatus)
	}
	if startEvt.NewStatus != xylona.Status_ONLINE.String() {
		t.Errorf("expected first event NewStatus %q, got %q", xylona.Status_ONLINE.String(), startEvt.NewStatus)
	}
	if startEvt.ServerNodeID != "test-node-3" {
		t.Errorf("expected ServerNodeID %q, got %q", "test-node-3", startEvt.ServerNodeID)
	}

	// Verify the final event transitions to OFFLINE.
	lastEvt := events[len(events)-1]
	if lastEvt.NewStatus != xylona.Status_OFFLINE.String() {
		t.Errorf("expected last event NewStatus %q, got %q", xylona.Status_OFFLINE.String(), lastEvt.NewStatus)
	}
}

func TestInternalCommandExitStatus(t *testing.T) {
	tests := []struct {
		name          string
		status        xylona.Status
		game          *internalGameStub
		includeServer bool
		includeGameID bool
		wantExitCode  int
	}{
		{
			name:          "successful install",
			status:        xylona.Status_INSTALLING,
			game:          &internalGameStub{},
			includeServer: true,
			includeGameID: true,
			wantExitCode:  0,
		},
		{
			name:          "failed install",
			status:        xylona.Status_INSTALLING,
			game:          &internalGameStub{installErr: errors.New("install failed")},
			includeServer: true,
			includeGameID: true,
			wantExitCode:  1,
		},
		{
			name:          "failed update",
			status:        xylona.Status_UPDATING,
			game:          &internalGameStub{updateErr: errors.New("update failed")},
			includeServer: true,
			includeGameID: true,
			wantExitCode:  1,
		},
		{
			name:          "missing game server",
			status:        xylona.Status_INSTALLING,
			game:          &internalGameStub{},
			includeServer: false,
			includeGameID: true,
			wantExitCode:  1,
		},
		{
			name:          "missing game ID",
			status:        xylona.Status_INSTALLING,
			game:          &internalGameStub{},
			includeServer: true,
			includeGameID: false,
			wantExitCode:  1,
		},
		{
			name:          "unregistered game",
			status:        xylona.Status_INSTALLING,
			includeServer: true,
			includeGameID: true,
			wantExitCode:  1,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inst, errNew := New(t.Context())
			if errNew != nil {
				t.Fatalf("failed to create supervisor: %v", errNew)
			}

			serverID := fmt.Sprintf("internal-status-test-%d", index)
			gameID := serverID + "-game"
			if test.game != nil {
				gameintegrations.RegisterGame(gameID, test.game)
				t.Cleanup(func() {
					gameintegrations.UnregisterGameForTest(gameID)
				})
			}

			var gameServer *models.GameServer
			if test.includeServer {
				gameServer = &models.GameServer{ID: serverID, Name: test.name}
			}
			var internalGameID *string
			if test.includeGameID {
				internalGameID = &gameID
			}

			eb := eventbus.Get()
			statusCh := eb.SubscribeReliable(eventbus.TopicGameServerStatusChanged)
			defer eb.Unsubscribe(eventbus.TopicGameServerStatusChanged, statusCh)

			done := make(chan struct{})
			command, errStart := inst.StartCommand(PreparedCommand{
				ID:                 serverID,
				GameServerName:     test.name,
				InternalCommand:    true,
				InternalGameServer: gameServer,
				GameID:             internalGameID,
				Status:             test.status,
				CallbackFunction: func(_ *Command) {
					close(done)
				},
			})
			if errStart != nil {
				t.Fatalf("failed to start internal command: %v", errStart)
			}

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for internal command completion")
			}

			var finalEvent eventbus.StatusChangedEvent
			foundFinalEvent := false
			deadline := time.After(2 * time.Second)
			for !foundFinalEvent {
				select {
				case rawEvent := <-statusCh:
					statusEvent, ok := rawEvent.(eventbus.StatusChangedEvent)
					if !ok || statusEvent.ServerID != serverID {
						continue
					}
					if statusEvent.NewStatus == xylona.Status_OFFLINE.String() {
						finalEvent = statusEvent
						foundFinalEvent = true
					}
				case <-deadline:
					t.Fatal("timed out waiting for final internal command status event")
				}
			}

			if finalEvent.ExitCode != test.wantExitCode {
				t.Errorf("exit code = %d, want %d", finalEvent.ExitCode, test.wantExitCode)
			}
			if command.Status() != xylona.Status_OFFLINE {
				t.Errorf("command status = %s, want %s", command.Status(), xylona.Status_OFFLINE)
			}
		})
	}
}

func TestStartCommandRejectsOverlappingInternalCommand(t *testing.T) {
	inst, errNew := New(t.Context())
	if errNew != nil {
		t.Fatalf("failed to create supervisor: %v", errNew)
	}

	gameID := "blocking-internal-update"
	game := &blockingInternalGameStub{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	gameintegrations.RegisterGame(gameID, game)
	t.Cleanup(func() {
		gameintegrations.UnregisterGameForTest(gameID)
	})

	server := &models.GameServer{ID: "blocking-server", Name: "Blocking Server"}
	prepared := PreparedCommand{
		ID:                 server.ID,
		InternalCommand:    true,
		InternalGameServer: server,
		GameID:             &gameID,
		Status:             xylona.Status_UPDATING,
	}
	done := make(chan struct{})
	prepared.CallbackFunction = func(_ *Command) {
		close(done)
	}

	_, errStart := inst.StartCommand(prepared)
	if errStart != nil {
		t.Fatalf("first StartCommand() error = %v", errStart)
	}

	select {
	case <-game.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for internal update to start")
	}

	_, errSecondStart := inst.StartCommand(prepared)
	if !errors.Is(errSecondStart, ErrCommandAlreadyRunning) {
		t.Fatalf("second StartCommand() error = %v, want %v", errSecondStart, ErrCommandAlreadyRunning)
	}

	close(game.release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for internal update to finish")
	}
}

func TestExtractExitCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{
			name:     "nil error returns 0",
			err:      nil,
			expected: 0,
		},
		{
			name:     "non-ExitError with nil cmd returns -1",
			err:      context.DeadlineExceeded,
			expected: -1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractExitCode(nil, tc.err)
			if got != tc.expected {
				t.Errorf("extractExitCode(nil, %v) = %d, want %d", tc.err, got, tc.expected)
			}
		})
	}
}

func TestNodeIDThreadedThroughInitNewCommand(t *testing.T) {
	ctx := t.Context()

	inst, errNew := New(ctx)
	if errNew != nil {
		t.Fatalf("failed to create supervisor: %v", errNew)
	}

	// Test new command creation.
	newCmd := inst.initNewCommand(PreparedCommand{
		ID:          "node-id-test",
		NodeID:      "my-node-id",
		BaseCommand: "echo",
		Args:        []string{"test"},
		Status:      xylona.Status_ONLINE,
	}, nil)

	if newCmd.nodeID != "my-node-id" {
		t.Errorf("expected nodeID %q on new command, got %q", "my-node-id", newCmd.nodeID)
	}

	// Test reused persistent command gets nodeID updated.
	reusedCmd := inst.initNewCommand(PreparedCommand{
		ID:          "node-id-test",
		NodeID:      "updated-node-id",
		BaseCommand: "echo",
		Args:        []string{"test2"},
		Status:      xylona.Status_ONLINE,
	}, newCmd)

	if reusedCmd.nodeID != "updated-node-id" {
		t.Errorf("expected nodeID %q on reused command, got %q", "updated-node-id", reusedCmd.nodeID)
	}
}

func TestGetCommandByIDOrCreateShellReturnsSingleShellUnderConcurrentAccess(t *testing.T) {
	inst, errNew := New(t.Context())
	if errNew != nil {
		t.Fatalf("failed to create supervisor: %v", errNew)
	}

	const goroutineCount = 16
	results := make([]*Command, goroutineCount)

	var wg sync.WaitGroup
	wg.Add(goroutineCount)
	for i := range goroutineCount {
		go func(index int) {
			defer wg.Done()
			results[index] = inst.GetCommandByIDOrCreateShell("concurrent-shell")
		}(i)
	}
	wg.Wait()

	first := results[0]
	if first == nil {
		t.Fatal("expected first shell result to be non-nil")
	}

	for i, result := range results[1:] {
		if result != first {
			t.Fatalf("result %d returned a different shell pointer", i+1)
		}
	}

	stored, errGet := inst.GetCommandByID("concurrent-shell")
	if errGet != nil {
		t.Fatalf("GetCommandByID() error = %v", errGet)
	}
	if stored != first {
		t.Fatal("stored command pointer does not match concurrent shell result")
	}
}
