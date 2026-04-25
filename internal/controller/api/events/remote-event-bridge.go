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
	ctx, cancel := context.WithCancel(b.parent) //nolint:gosec // G118: cancel is retained in b.active and invoked by OnRemove or the next startForClient call, not leaked.
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

		// Track per-server previous status so we can publish StatusChanged
		// with the correct OldStatus field.
		previousStatus := make(map[string]string)
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
				b.republish(client.ID(), ev, previousStatus)
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

// republish translates a node.Event into the controller's eventbus topics.
// For EventTypeProcessStatus: publishes StatusChangedEvent (with IntentionalStop
// inferred from the event's status text — remote nodes don't have a direct
// supervisor handle to query, so "OFFLINE with non-empty status" is treated
// as a process exit signal; auto-restart's own DB lookup disambiguates).
func (b *RemoteEventBridge) republish(nodeID string, ev node.Event, previousStatus map[string]string) {
	switch ev.Type {
	case node.EventTypeProcessStatus:
		newStatus := strings.ToUpper(strings.TrimSpace(ev.Status))
		if newStatus == "" {
			return
		}
		oldStatus, hasOld := previousStatus[ev.ProcessID]
		if !hasOld {
			oldStatus = xylona.Status_UNKNOWN.String()
		}
		previousStatus[ev.ProcessID] = newStatus

		serverName := b.resolveServerName(ev.ProcessID)

		b.bus.Publish(eventbus.TopicGameServerStatusChanged, eventbus.StatusChangedEvent{
			ServerID:     ev.ProcessID,
			ServerName:   serverName,
			ServerNodeID: nodeID,
			OldStatus:    oldStatus,
			NewStatus:    newStatus,
			// IntentionalStop cannot be inferred from the wire alone; the
			// auto-restart subscriber re-reads the DB to determine whether
			// a stop was intentional or an unexpected exit. ExitCode is
			// left zero because the node doesn't surface it in the status
			// event; crash events handle the non-zero case below.
		})

		// Status==crashed is a synthetic marker we add on the node side when
		// the process exits with a non-zero code (see ProcessCrashEvent); no
		// additional publish is needed here.

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
