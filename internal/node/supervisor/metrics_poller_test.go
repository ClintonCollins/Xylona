package supervisor

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestPollProcessMetrics_NoCommands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	inst := &Instance{
		ctx:             ctx,
		runningCommands: make(map[string]*Command),
		RWMutex:         &sync.RWMutex{},
	}

	// Should run and return cleanly when there are no commands.
	done := make(chan struct{})
	go func() {
		inst.pollProcessMetrics(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Error("pollProcessMetrics did not stop after context cancellation")
	}
}

func TestPollDiskMetrics_NoCommands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	inst := &Instance{
		ctx:             ctx,
		runningCommands: make(map[string]*Command),
		RWMutex:         &sync.RWMutex{},
	}

	done := make(chan struct{})
	go func() {
		inst.pollDiskMetrics(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Error("pollDiskMetrics did not stop after context cancellation")
	}
}
