package helpers

import "github.com/ClintonCollins/Xylona/proto/go/xylona"

// WebsocketMessageType identifies an outbound websocket payload kind.
type WebsocketMessageType int

// WebsocketRequestType identifies an inbound websocket request kind.
type WebsocketRequestType int

const (
	// WebsocketOutputTypeUnknown represents an unknown outbound websocket payload.
	WebsocketOutputTypeUnknown WebsocketMessageType = iota
	// WebsocketOutputTypeGameServerConsole represents console output for a game server.
	WebsocketOutputTypeGameServerConsole
	// WebsocketOutputTypeGameServerStatus represents a game server status update.
	WebsocketOutputTypeGameServerStatus
	// WebsocketOutputTypeRaw represents an unstructured outbound websocket payload.
	WebsocketOutputTypeRaw
)

const (
	// RequestUnknown represents an unknown inbound websocket request.
	RequestUnknown WebsocketRequestType = iota
	// RequestGetGameServerConsole subscribes to game server console output.
	RequestGetGameServerConsole
	// RequestGetGameServerStatus subscribes to game server status updates.
	RequestGetGameServerStatus
	// RequestRemoveGameServerConsole unsubscribes from game server console output.
	RequestRemoveGameServerConsole
	// RequestRemoveGameServerStatus unsubscribes from game server status updates.
	RequestRemoveGameServerStatus
)

// GameServerConsoleOutput is a websocket payload containing console text.
type GameServerConsoleOutput struct {
	GameServerID string `json:"gameServerID"`
	Output       string `json:"output"`
}

// GameServerStatusUpdate is a websocket payload containing a status change.
type GameServerStatusUpdate struct {
	GameServerID string        `json:"gameServerID"`
	Status       xylona.Status `json:"status"`
}

// WebsocketRequest describes a typed websocket subscription request.
type WebsocketRequest struct {
	GameServerID *string
	Type         WebsocketRequestType `json:"type"`
}

// WebsocketMessage describes an outbound websocket payload envelope.
type WebsocketMessage struct {
	Type                    WebsocketMessageType     `json:"type"`
	GameServerConsoleOutput *GameServerConsoleOutput `json:"gameServerConsoleOutput,omitempty"`
	GameServerStatusUpdate  *GameServerStatusUpdate  `json:"gameServerStatusUpdate,omitempty"`
	RawData                 any                      `json:"rawData,omitempty"`
}

func (w WebsocketMessageType) String() string {
	switch w {
	case WebsocketOutputTypeGameServerConsole:
		return "GameServerConsole"
	case WebsocketOutputTypeGameServerStatus:
		return "GameServerStatus"
	default:
		return "Unknown"
	}
}

func (w WebsocketRequestType) String() string {
	switch w {
	case RequestGetGameServerConsole:
		return "GameServerConsole"
	case RequestGetGameServerStatus:
		return "GameServerStatus"
	default:
		return "Unknown"
	}
}
