package supervisor

import (
	"context"
	"errors"
	"sync"
	"testing"
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

func TestGetCommandAndArgsSplit(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple command",
			input:    "ls -la",
			expected: []string{"ls", "-la"},
		},
		{
			name:     "command with quotes",
			input:    `echo "hello world"`,
			expected: []string{"echo", "hello world"},
		},
		{
			name:     "multiple spaces",
			input:    "ls    -la",
			expected: []string{"ls", "-la"},
		},
		{
			name:     "quoted path",
			input:    `"/path with spaces/bin" --arg`,
			expected: []string{"/path with spaces/bin", "--arg"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getCommandAndArgsSplit(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("got %d args, want %d", len(got), len(tt.expected))
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("arg %d: got %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}
