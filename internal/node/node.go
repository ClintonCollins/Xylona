package node

import (
	"context"
	"sync"

	"golang.org/x/sync/singleflight"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
)

// Node is the in-process node implementation used by the controller's embedded
// node and the node binary's Connect-RPC server. Method receivers are pointers
// so callers can reuse a single instance across goroutines.
type Node struct {
	ctx                        context.Context
	supervisor                 *supervisor.Instance
	db                         *db.Connection
	events                     *EventEmitter
	minecraftMapLifecycleLocks sync.Map
	minecraftMapLiveMu         sync.Mutex
	minecraftMapLiveCache      map[string]minecraftMapLiveCacheEntry
	minecraftMapLiveRequests   singleflight.Group
}

// New constructs a Node. The supervisor and database references are retained
// as-is; callers are expected to manage their lifetimes. The database may be
// nil when internal game-server lookups are unavailable.
func New(ctx context.Context, supervisorInst *supervisor.Instance, database *db.Connection) *Node {
	nodeInst := &Node{
		ctx:                   ctx,
		supervisor:            supervisorInst,
		db:                    database,
		events:                NewEventEmitter(),
		minecraftMapLiveCache: make(map[string]minecraftMapLiveCacheEntry),
	}
	nodeInst.startStatusEventBridge()
	return nodeInst
}

// Events returns the node's replayable event emitter.
func (n *Node) Events() *EventEmitter {
	return n.events
}
