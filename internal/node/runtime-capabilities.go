package node

const RuntimeProtocolVersion int64 = 1

// RuntimeCapabilities describes process-runtime features exposed by a node.
type RuntimeCapabilities struct {
	ProtocolVersion int64
	LaunchEnv       bool
}

// RuntimeCapabilities reports the runtime behavior supported by this node.
func (n *Node) RuntimeCapabilities() RuntimeCapabilities {
	return RuntimeCapabilities{
		ProtocolVersion: RuntimeProtocolVersion,
		LaunchEnv:       true,
	}
}
