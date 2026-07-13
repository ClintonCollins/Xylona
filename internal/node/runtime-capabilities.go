package node

// RuntimeProtocolVersion is the node process-runtime capability protocol version.
const RuntimeProtocolVersion int64 = 2

// RuntimeCapabilities describes process-runtime features exposed by a node.
type RuntimeCapabilities struct {
	ProtocolVersion          int64
	LaunchEnv                bool
	ReliableProcessLifecycle bool
	TelnetInput              bool
}

// RuntimeCapabilities reports the runtime behavior supported by this node.
func (n *Node) RuntimeCapabilities() RuntimeCapabilities {
	return RuntimeCapabilities{
		ProtocolVersion:          RuntimeProtocolVersion,
		LaunchEnv:                true,
		ReliableProcessLifecycle: true,
		TelnetInput:              true,
	}
}
