package helpers

import "github.com/ClintonCollins/Xylona/proto/go/xylona"

type WebsocketMessageType int
type WebsocketRequestType int

const (
	WebsocketOutputTypeUnknown WebsocketMessageType = iota
	WebsocketOutputTypeGameServerConsole
	WebsocketOutputTypeGameServerStatus
	WebsocketOutputTypeRaw
)

const (
	RequestUnknown WebsocketRequestType = iota
	RequestGetGameServerConsole
	RequestGetGameServerStatus
	RequestRemoveGameServerConsole
	RequestRemoveGameServerStatus
)

type GameServerConsoleOutput struct {
	GameServerID string `json:"gameServerID"`
	Output       string `json:"output"`
}

type GameServerStatusUpdate struct {
	GameServerID string        `json:"gameServerID"`
	Status       xylona.Status `json:"status"`
}

type WebsocketRequest struct {
	GameServerID *string
	Type         WebsocketRequestType `json:"type"`
}

type WebsocketMessage struct {
	Type                    WebsocketMessageType     `json:"type"`
	GameServerConsoleOutput *GameServerConsoleOutput `json:"gameServerConsoleOutput,omitempty"`
	GameServerStatusUpdate  *GameServerStatusUpdate  `json:"gameServerStatusUpdate,omitempty"`
	RawData                 interface{}              `json:"rawData,omitempty"`
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
