package node

import (
	"context"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
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
// later migration steps that need access to node-local settings; internal/node
// itself does not currently issue queries against it.
func New(ctx context.Context, supervisorInst *supervisor.Instance, database *db.Connection) *Node {
	nodeInst := &Node{
		ctx:        ctx,
		supervisor: supervisorInst,
		db:         database,
		events:     NewEventEmitter(),
	}
	nodeInst.startStatusEventBridge()
	return nodeInst
}

// Events returns the node's replayable event emitter.
func (n *Node) Events() *EventEmitter {
	return n.events
}
