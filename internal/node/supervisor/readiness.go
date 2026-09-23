package supervisor

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// DefaultReadyTimeout is how long a start may report PRE_START before the
// supervisor gives up waiting for a readiness signal and reports ONLINE.
const DefaultReadyTimeout = 5 * time.Minute

// readyProbeInterval is a variable so tests can probe without waiting.
var readyProbeInterval = 5 * time.Second

// Readiness lists the signals that move a started game server from PRE_START
// to ONLINE. Whichever arrives first wins; Timeout is the fallback so a
// server never stays in PRE_START forever.
type Readiness struct {
	// LogPattern matches a console line the game prints once it accepts players.
	LogPattern *regexp.Regexp
	// Probe reports whether the game answers its query yet.
	Probe func(context.Context) bool
	// Timeout defaults to DefaultReadyTimeout.
	Timeout time.Duration
}

// readinessWatch carries one execution's matching console line to the
// goroutine waiting for readiness. The buffered channel keeps a match that
// arrives before that goroutine starts.
type readinessWatch struct {
	pattern *regexp.Regexp
	matched chan struct{}
}

func newReadinessWatch(pattern *regexp.Regexp) *readinessWatch {
	if pattern == nil {
		return nil
	}
	return &readinessWatch{pattern: pattern, matched: make(chan struct{}, 1)}
}

// observeReadyLine checks one console line against the current execution's
// readiness pattern.
func (c *Command) observeReadyLine(line string) {
	watch := c.readyWatch.Load()
	if watch == nil || !watch.pattern.MatchString(line) {
		return
	}
	select {
	case watch.matched <- struct{}{}:
	default:
	}
}

// awaitReadiness runs after the PRE_START transition is announced, so the
// ONLINE transition it reports can never be delivered ahead of it.
func (c *Command) awaitReadiness(
	processCtx context.Context,
	processGeneration uint64,
	readiness *Readiness,
	watch *readinessWatch,
) {
	timeout := readiness.Timeout
	if timeout <= 0 {
		timeout = DefaultReadyTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var probeTick <-chan time.Time
	if readiness.Probe != nil {
		ticker := time.NewTicker(readyProbeInterval)
		defer ticker.Stop()
		probeTick = ticker.C
	}
	var matched <-chan struct{}
	if watch != nil {
		matched = watch.matched
	}

	for {
		select {
		case <-processCtx.Done():
			return
		case <-matched:
			c.markReady(processGeneration, "Server is ready for players.")
			return
		case <-probeTick:
			probeCtx, cancelProbe := context.WithTimeout(processCtx, readyProbeInterval)
			answered := readiness.Probe(probeCtx)
			cancelProbe()
			if answered {
				c.markReady(processGeneration, "Server is ready for players.")
				return
			}
		case <-timer.C:
			c.markReady(processGeneration, fmt.Sprintf("No readiness signal after %s; reporting the server online.", timeout))
			return
		}
	}
}

// markReady moves a starting execution to ONLINE once. It shares
// executionMutex with finalizeExecution so a process that exits at the same
// moment can never end with a late ONLINE event.
func (c *Command) markReady(processGeneration uint64, notice string) {
	c.executionMutex.Lock()
	defer c.executionMutex.Unlock()
	c.Lock()
	if c.processGeneration != processGeneration || c.status != xylona.Status_PRE_START {
		c.Unlock()
		return
	}
	c.status = xylona.Status_ONLINE
	c.Unlock()
	c.readyWatch.Store(nil)
	c.sendJobNotification(formatXylonaMessage(notice))
	c.sendJobStatusNotification(xylona.Status_PRE_START, xylona.Status_ONLINE)
}
