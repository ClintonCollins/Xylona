package node

import (
	"context"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/supervisor"
)

// Node is the in-process node implementation. The controller creates one of
// these for the embedded node and (in Step 4+) the node binary will create one
// of these to back its gRPC server. Method receivers are pointer to allow the
// caller to reuse a single instance across goroutines.
type Node struct {
	ctx        context.Context
	supervisor *supervisor.Instance
	db         *db.Connection
	events     *EventEmitter
}

// New constructs a Node. The supervisor and db references are retained as-is;
// callers are expected to manage their lifetimes.
//
// db is included on the constructor signature for forward compatibility with
// later migration steps that need access to node-local settings; pkg/node
// itself does not currently issue queries against it.
func New(ctx context.Context, supervisorInst *supervisor.Instance, database *db.Connection) *Node {
	return &Node{
		ctx:        ctx,
		supervisor: supervisorInst,
		db:         database,
		events:     NewEventEmitter(),
	}
}

// Supervisor returns the underlying supervisor instance. Exposed for callers
// that still need direct access during the transition; will be removed once
// all access flows through Node methods.
func (n *Node) Supervisor() *supervisor.Instance {
	return n.supervisor
}

// Events returns the node's EventEmitter. Step 9 will wire this up; for now
// the existing pkg/eventbus continues to drive event delivery.
func (n *Node) Events() *EventEmitter {
	return n.events
}
