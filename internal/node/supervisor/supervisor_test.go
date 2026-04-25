package supervisor

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestNewInstance(t *testing.T) {
	ctx := context.Background()
	inst, err := New(ctx)
	if err != nil {
		t.Fatalf("Failed to create supervisor instance: %v", err)
	}
	if inst == nil {
		t.Fatal("Supervisor instance is nil")
	}
	if inst.runningCommands == nil {
		t.Fatal("runningCommands map is nil")
	}
}

func TestGetCommandByID(t *testing.T) {
	ctx := context.Background()
	inst, _ := New(ctx)

	// Test non-existent command
	cmd, err := inst.GetCommandByID("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent command, got nil")
	}
	if !errors.Is(err, ErrCommandDoesNotExist) {
		t.Errorf("Expected ErrCommandDoesNotExist, got %v", err)
	}
	if cmd != nil {
		t.Error("Expected nil command for non-existent ID")
	}

	// Add a command manually for testing
	inst.Lock()
	testCmd := &Command{ID: "test-1", RWMutex: &sync.RWMutex{}}
	inst.runningCommands["test-1"] = testCmd
	inst.Unlock()

	cmd, err = inst.GetCommandByID("test-1")
	if err != nil {
		t.Fatalf("Failed to get existing command: %v", err)
	}
	if cmd != testCmd {
		t.Errorf("Got wrong command: expected %v, got %v", testCmd, cmd)
	}
}

func TestPrepareCommandProcessRequiresBaseCommand(t *testing.T) {
	ctx := context.Background()

	inst, errNew := New(ctx)
	if errNew != nil {
		t.Fatalf("failed to create supervisor instance: %v", errNew)
	}

	_, errPrepare := inst.prepareCommandProcess(PreparedCommand{
		ID:     "missing-base-command",
		Status: xylona.Status_ONLINE,
	})
	if errPrepare == nil {
		t.Fatal("expected error for missing base command")
	}
}
