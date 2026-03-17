package supervisor

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestStartMetricsPoller_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	inst := &Instance{
		ctx:             ctx,
		runningCommands: make(map[string]*Command),
		RWMutex:         &sync.RWMutex{},
	}

	inst.StartMetricsPoller(ctx)

	// Cancel the context and give goroutines time to exit.
	cancel()
	time.Sleep(50 * time.Millisecond)
	// No assertions needed — the test passes if it doesn't deadlock or panic.
}

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
