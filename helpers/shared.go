package helpers

import "github.com/ClintonCollins/Xylona/proto/go/xylona"

type WebsocketOutputType int

const (
	WebsocketOutputTypeGameServerConsole WebsocketOutputType = iota
	WebsocketOutputTypeGameServerStatus
)

func (w WebsocketOutputType) String() string {
	switch w {
	case WebsocketOutputTypeGameServerConsole:
		return "GameServerConsole"
	case WebsocketOutputTypeGameServerStatus:
		return "GameServerStatus"
	default:
		return "Unknown"
	}
}

type WebsocketOutputPayload struct {
	OutputType   WebsocketOutputType `json:"outputType"`
	GameServerID string              `json:"gameServerID"`
	Data         string              `json:"data"`
	Status       xylona.Status       `json:"status"`
}
