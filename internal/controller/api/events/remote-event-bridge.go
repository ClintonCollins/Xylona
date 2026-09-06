// Package events wires node-side event streams into the controller's
// eventbus so remote-node processes surface the same lifecycle signals as
// the controller's embedded supervisor. With the bridge in place, every
// websocket subscriber, auto-restart hook, and alert evaluator sees remote
// game-server events identically to embedded ones.
package events

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/eventbus"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// RemoteEventBridge owns a goroutine per non-self registered node that
// consumes NodeClient.StreamEvents(ctx) and republishes equivalent events
// into the shared eventbus. The self-node's supervisor already publishes
// directly so no bridge goroutine is needed for it.
//
// The bridge implements noderegistry.Observer so nodes auto-wire as they
// register (e.g. via bootstrap) and auto-unwire on removal.
type RemoteEventBridge struct {
	registry *noderegistry.Registry
	db       *db.Connection
	bus      *eventbus.EventBus
	parent   context.Context

	mu     sync.Mutex
	active map[string]context.CancelFunc
}

type processLifecycleCursor struct {
	executionID string
	sequence    uint64
}

// NewRemoteEventBridge constructs a bridge. It does NOT start any goroutines
// until Start is called. Pass the controller's long-lived context as parent;
// every per-node goroutine is tied to a child context derived from it.
func NewRemoteEventBridge(parent context.Context, registry *noderegistry.Registry, dbInst *db.Connection, bus *eventbus.EventBus) *RemoteEventBridge {
	return &RemoteEventBridge{
		registry: registry,
		db:       dbInst,
		bus:      bus,
		parent:   parent,
		active:   make(map[string]context.CancelFunc),
	}
}

// Start registers the bridge as a registry observer and spins up goroutines
// for every non-self node already registered (captures the startup state).
func (b *RemoteEventBridge) Start() {
	b.registry.AddObserver(b)
	// Cover nodes that were already registered before we observed.
	for _, client := range b.registry.List() {
		if client.ID() == b.registry.SelfID() {
			continue
		}
		b.startForClient(client)
	}
}

// OnRegister satisfies noderegistry.Observer. Starts a per-node goroutine
// for remote clients; no-op for the self node.
func (b *RemoteEventBridge) OnRegister(client nodeclient.NodeClient) {
	if client == nil {
		return
	}
	if client.ID() == b.registry.SelfID() {
		return
	}
	b.startForClient(client)
}

// OnRemove satisfies noderegistry.Observer. Cancels the per-node goroutine
// if one is running.
func (b *RemoteEventBridge) OnRemove(nodeID string) {
	b.mu.Lock()
	cancel, ok := b.active[nodeID]
	if ok {
		delete(b.active, nodeID)
	}
	b.mu.Unlock()
	if ok {
		cancel()
	}
}

func (b *RemoteEventBridge) startForClient(client nodeclient.NodeClient) {
	b.mu.Lock()
	if existing, ok := b.active[client.ID()]; ok {
		// Replace in-flight bridge if the registry registered over an old client.
		existing()
		delete(b.active, client.ID())
	}
	ctx, cancel := context.WithCancel(b.parent)
	b.active[client.ID()] = cancel
	b.mu.Unlock()

	go b.runForClient(ctx, client)
}

// runForClient subscribes to the client's event stream and republishes
// events into the eventbus. It retries on transport errors with a backoff
// until the parent context is canceled.
func (b *RemoteEventBridge) runForClient(ctx context.Context, client nodeclient.NodeClient) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	// These maps intentionally survive stream reconnects. Legacy nodes need the
	// previous-status cache, while v2 nodes use the cursor to deduplicate replay.
	previousStatus := make(map[string]string)
	lifecycleCursors := make(map[string]processLifecycleCursor)

	for {
		if ctx.Err() != nil {
			return
		}

		events, errStream := client.StreamEvents(ctx)
		if errStream != nil {
			log.Warn().
				Err(errStream).
				Str("node_id", client.ID()).
				Dur("retry_in", backoff).
				Msg("remote-event-bridge: stream events failed; will retry")
			if waitOrCancel(ctx, backoff) {
				return
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		// Reset backoff on a successful subscribe.
		backoff = time.Second

		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					// Stream closed; loop re-subscribes with backoff.
					log.Debug().Str("node_id", client.ID()).Msg("remote-event-bridge: event stream closed, re-subscribing")
					goto restream
				}
				b.republish(client.ID(), ev, previousStatus, lifecycleCursors)
			}
		}
	restream:
		if waitOrCancel(ctx, backoff) {
			return
		}
	}
}

func waitOrCancel(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return true
	case <-time.After(d):
		return false
	}
}

func currentNodeOwnsProcess(serverNodeID, eventNodeID string) bool {
	return serverNodeID != "" && serverNodeID == eventNodeID
}

// republish translates a node.Event into the controller's eventbus topics.
// For EventTypeProcessStatus, v2 events carry authoritative lifecycle metadata
// and are deduplicated across replay. Legacy events retain the previous-status
// reconstruction used by v1 nodes, with the cache surviving reconnects.
func (b *RemoteEventBridge) republish(
	nodeID string,
	ev node.Event,
	previousStatus map[string]string,
	lifecycleCursors map[string]processLifecycleCursor,
) {
	if ev.Type == node.EventTypeProcessStatus && b.db != nil {
		gameServer, errServer := b.db.GetGameServerByID(ev.ProcessID)
		if errServer != nil || !currentNodeOwnsProcess(gameServer.NodeID, nodeID) {
			return
		}
	}
	switch ev.Type {
	case node.EventTypeProcessStatus:
		newStatus := strings.ToUpper(strings.TrimSpace(ev.Status))
		if newStatus == "" {
			return
		}

		var oldStatus string
		reliableLifecycle := strings.TrimSpace(ev.ExecutionID) != "" && ev.TransitionSequence > 0
		if reliableLifecycle {
			cursor, hasCursor := lifecycleCursors[ev.ProcessID]
			if hasCursor && cursor.executionID == ev.ExecutionID && ev.TransitionSequence <= cursor.sequence {
				return
			}
			lifecycleCursors[ev.ProcessID] = processLifecycleCursor{
				executionID: ev.ExecutionID,
				sequence:    ev.TransitionSequence,
			}
			oldStatus = strings.ToUpper(strings.TrimSpace(ev.OldStatus))
			if oldStatus == "" {
				oldStatus = xylona.Status_UNKNOWN.String()
			}
		} else {
			var hasOld bool
			oldStatus, hasOld = previousStatus[ev.ProcessID]
			if !hasOld {
				oldStatus = xylona.Status_UNKNOWN.String()
			}
		}
		previousStatus[ev.ProcessID] = newStatus

		serverName := b.resolveServerName(ev.ProcessID)

		b.bus.Publish(eventbus.TopicGameServerStatusChanged, eventbus.StatusChangedEvent{
			Failure:            ev.Failure,
			ServerID:           ev.ProcessID,
			ServerName:         serverName,
			ServerNodeID:       nodeID,
			OldStatus:          oldStatus,
			NewStatus:          newStatus,
			ExecutionID:        ev.ExecutionID,
			TransitionSequence: ev.TransitionSequence,
			IntentionalStop:    ev.IntentionalStop,
			ExitCode:           ev.ExitCode,
			ExitCodeKnown:      ev.ExitCodeKnown,
			Replayed:           ev.Replayed,
			OccurredAt:         ev.Timestamp,
		})
		if oldStatus == xylona.Status_ONLINE.String() && newStatus == xylona.Status_OFFLINE.String() &&
			ev.ExitCodeKnown && ev.ExitCode != 0 && !ev.IntentionalStop {
			crashedAt := ev.Timestamp
			if crashedAt.IsZero() {
				crashedAt = time.Now()
			}
			b.bus.Publish(eventbus.TopicGameServerCrashed, eventbus.ServerCrashedEvent{
				ServerID:     ev.ProcessID,
				ServerNodeID: nodeID,
				ExitCode:     ev.ExitCode,
				Timestamp:    crashedAt,
			})
		}

	case node.EventTypeConsoleOutput:
		// Console output is consumed directly by the websocket console
		// subscriber via its own subscription path. The bridge doesn't need
		// to republish it into a generic topic.

	case node.EventTypeMetrics:
		// Metrics update — this is handled by the snapshot poller in
		// Phase 6, not by the event bridge. Drop the payload silently to
		// avoid unbounded memory growth in the bus.
	}
}

// resolveServerName looks up the game server's display name by ID. Returns
// empty string on miss so the status event still publishes.
func (b *RemoteEventBridge) resolveServerName(serverID string) string {
	if b.db == nil {
		return ""
	}
	gs, errGet := b.db.GetGameServerByID(serverID)
	if errGet != nil {
		return ""
	}
	return gs.Name
}
