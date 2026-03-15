package supervisor

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestStartCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inst, _ := New(ctx)

	var fullCmd string
	if runtime.GOOS == "windows" {
		fullCmd = "cmd /c echo hello xylona"
	} else {
		fullCmd = "echo hello xylona"
	}

	pc := PreparedCommand{
		ID:                 "test-cmd",
		FullCommandAndArgs: fullCmd,
		Status:             xylona.Status_ONLINE,
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inst, _ := New(ctx)

	var fullCmd string
	if runtime.GOOS == "windows" {
		fullCmd = "cmd /c echo test-output"
	} else {
		fullCmd = "echo test-output"
	}

	pc := PreparedCommand{
		ID:                 "test-listener",
		FullCommandAndArgs: fullCmd,
		Status:             xylona.Status_ONLINE,
	}

	cmd, _ := inst.StartCommand(pc)

	outChan := make(chan xylona.Message, 10)
	cmd.AddOutputListener("test-l", outChan)
	defer cmd.RemoveOutputListener("test-l")

	timeout := time.After(5 * time.Second)
	found := false
	for !found {
		select {
		case msg := <-outChan:
			if msg.GameServerConsoleOutput != nil && strings.Contains(msg.GameServerConsoleOutput.Output, "test-output") {
				found = true
			}
		case <-timeout:
			t.Fatal("Timed out waiting for listener message")
		}
	}
}
