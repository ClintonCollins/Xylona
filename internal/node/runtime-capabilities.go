package node

// RuntimeProtocolVersion is the node process-runtime capability protocol version.
const RuntimeProtocolVersion int64 = 8

// RuntimeCapabilities describes process-runtime features exposed by a node.
type RuntimeCapabilities struct {
	ProtocolVersion          int64
	LaunchEnv                bool
	ReliableProcessLifecycle bool
	TelnetInput              bool
	RCONInput                bool
	RESTInput                bool
	PlayerActions            bool
	PalworldMap              bool
	SevenDaysToDieMap        bool
	MinecraftMap             bool
}

// RuntimeCapabilities reports the runtime behavior supported by this node.
func (n *Node) RuntimeCapabilities() RuntimeCapabilities {
	return RuntimeCapabilities{
		ProtocolVersion:          RuntimeProtocolVersion,
		LaunchEnv:                true,
		ReliableProcessLifecycle: true,
		TelnetInput:              true,
		RCONInput:                true,
		RESTInput:                true,
		PlayerActions:            true,
		PalworldMap:              true,
		SevenDaysToDieMap:        true,
		MinecraftMap:             true,
	}
}
