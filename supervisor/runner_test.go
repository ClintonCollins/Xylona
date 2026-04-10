package supervisor

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/pkg/eventbus"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

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
	pc := PreparedCommand{
		ID:          "status-test-server",
		NodeID:      "test-node-3",
		BaseCommand: baseCommand,
		Args:        args,
		Status:      xylona.Status_ONLINE,
	}

	_, errStart := inst.prepareCommandProcess(pc)
	if errStart != nil {
		t.Fatalf("failed to start command: %v", errStart)
	}

	// Collect status change events. We expect at least:
	// 1. OFFLINE -> ONLINE (server started)
	// 2. ONLINE -> OFFLINE (server stopped / output reader finished)
	events := make([]eventbus.StatusChangedEvent, 0)
	deadline := time.After(10 * time.Second)

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
			// After receiving the OFFLINE transition, we have enough.
			if statusEvt.NewStatus == xylona.Status_OFFLINE.String() {
				goto done
			}
		case <-deadline:
			t.Fatalf("timed out waiting for status events; received %d events: %+v", len(events), events)
		}
	}

done:
	if len(events) < 2 {
		t.Fatalf("expected at least 2 status change events, got %d: %+v", len(events), events)
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
