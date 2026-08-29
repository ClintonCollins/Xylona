package node

import (
	"slices"

	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
)

// RuntimeProtocolVersion is the node process-runtime capability protocol version.
const RuntimeProtocolVersion int64 = 12

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
	GameOperations           []GameOperationSupport
}

// GameOperationSupport identifies the operation IDs compiled into a node for one game.
type GameOperationSupport struct {
	GameID       string
	OperationIDs []string
}

// SupportsGameOperation reports whether the node advertised an exact game operation.
func (capabilities RuntimeCapabilities) SupportsGameOperation(gameID string, operationID string) bool {
	for _, support := range capabilities.GameOperations {
		if support.GameID == gameID && slices.Contains(support.OperationIDs, operationID) {
			return true
		}
	}
	return false
}

// RuntimeCapabilities reports the runtime behavior supported by this node.
func (n *Node) RuntimeCapabilities() RuntimeCapabilities {
	operations := gameintegrations.OperationsForGame("7_days_to_die")
	operationIDs := make([]string, 0, len(operations))
	for _, operation := range operations {
		operationIDs = append(operationIDs, operation.ID)
	}
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
		GameOperations: []GameOperationSupport{
			{GameID: "7_days_to_die", OperationIDs: operationIDs},
		},
	}
}
