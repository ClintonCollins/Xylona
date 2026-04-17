// Package noderegistry holds the controller's set of NodeClients, keyed by
// node ID. It is the single place the rest of the controller reaches for to
// talk to a node — embedded (in-process) or remote (gRPC, later step).
//
// The registry is intentionally small: it is a concurrent map plus a notion
// of "self" (the node ID the controller's embedded node owns). Dial loops
// and reconnection logic live in the gRPC client implementation, not here.
package noderegistry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
)

// ErrNodeNotRegistered is returned by Get when no client exists for the
// requested node ID.
var ErrNodeNotRegistered = errors.New("noderegistry: node not registered")

// Observer receives notifications when clients are registered or removed.
// Implementations must be safe for concurrent invocation; the registry calls
// OnRegister and OnRemove while holding no registry lock, but observers can
// be invoked from multiple goroutines.
type Observer interface {
	OnRegister(client nodeclient.NodeClient)
	OnRemove(nodeID string)
}

// Registry is a concurrent map of nodeID to NodeClient. The zero value is
// not usable; construct with New.
type Registry struct {
	mu         sync.RWMutex
	clients    map[string]nodeclient.NodeClient
	selfNodeID string

	observersMu sync.RWMutex
	observers   []Observer
}

// New constructs a registry with the given self node ID and registers the
// embedded client under that ID. embedded may be nil in tests that do not
// need a self client; callers that need one must register it explicitly.
func New(selfNodeID string, embedded nodeclient.NodeClient) *Registry {
	registry := &Registry{
		clients:    make(map[string]nodeclient.NodeClient),
		selfNodeID: selfNodeID,
	}
	if embedded != nil {
		registry.clients[embedded.ID()] = embedded
	}
	return registry
}

// AddObserver subscribes fn to register/remove events. Observers must be
// registered before the first Register call if they want to see all clients.
// Safe to call from any goroutine.
func (r *Registry) AddObserver(obs Observer) {
	if obs == nil {
		return
	}
	r.observersMu.Lock()
	defer r.observersMu.Unlock()
	r.observers = append(r.observers, obs)
}

func (r *Registry) notifyRegister(client nodeclient.NodeClient) {
	r.observersMu.RLock()
	observers := append([]Observer(nil), r.observers...)
	r.observersMu.RUnlock()
	for _, obs := range observers {
		obs.OnRegister(client)
	}
}

func (r *Registry) notifyRemove(nodeID string) {
	r.observersMu.RLock()
	observers := append([]Observer(nil), r.observers...)
	r.observersMu.RUnlock()
	for _, obs := range observers {
		obs.OnRemove(nodeID)
	}
}

// Register adds or replaces the client for client.ID(). Registering over an
// existing client is allowed; callers wanting replace-with-close semantics
// should call Remove first. Observers are notified after the registration
// lands.
func (r *Registry) Register(client nodeclient.NodeClient) {
	if client == nil {
		return
	}
	r.mu.Lock()
	r.clients[client.ID()] = client
	r.mu.Unlock()
	r.notifyRegister(client)
}

// Remove unregisters the client for nodeID. If the client implements
// io.Closer, Close is invoked; any error is logged rather than returned to
// keep Remove's signature convenient for cleanup paths. Observers are
// notified after the removal lands.
func (r *Registry) Remove(nodeID string) {
	r.mu.Lock()
	client, ok := r.clients[nodeID]
	if ok {
		delete(r.clients, nodeID)
	}
	r.mu.Unlock()

	if !ok {
		return
	}

	r.notifyRemove(nodeID)

	closer, ok := client.(io.Closer)
	if !ok {
		return
	}
	errClose := closer.Close()
	if errClose != nil {
		log.Warn().Err(errClose).Str("node_id", nodeID).
			Msg("noderegistry: close client on remove")
	}
}

// Get returns the NodeClient for nodeID or ErrNodeNotRegistered if absent.
func (r *Registry) Get(nodeID string) (nodeclient.NodeClient, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	client, ok := r.clients[nodeID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNodeNotRegistered, nodeID)
	}
	return client, nil
}

// GetSelf returns the NodeClient registered under selfNodeID, or nil if the
// controller has no embedded client registered. Callers should nil-check
// the return value before using it.
func (r *Registry) GetSelf() nodeclient.NodeClient {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.clients[r.selfNodeID]
}

// SelfID returns the node ID configured as the controller's self.
func (r *Registry) SelfID() string {
	return r.selfNodeID
}

// List returns a snapshot of all currently registered clients. The order is
// not defined.
func (r *Registry) List() []nodeclient.NodeClient {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]nodeclient.NodeClient, 0, len(r.clients))
	for _, client := range r.clients {
		out = append(out, client)
	}
	return out
}

// Close tears down every registered client that implements io.Closer. Errors
// are joined and returned together so a single misbehaving client does not
// prevent the rest from closing.
func (r *Registry) Close(_ context.Context) error {
	r.mu.Lock()
	clients := r.clients
	r.clients = make(map[string]nodeclient.NodeClient)
	r.mu.Unlock()

	var closeErrors error
	for nodeID, client := range clients {
		closer, ok := client.(io.Closer)
		if !ok {
			continue
		}
		errClose := closer.Close()
		if errClose != nil {
			closeErrors = errors.Join(closeErrors, fmt.Errorf("noderegistry: close %s: %w", nodeID, errClose))
		}
	}
	return closeErrors
}
