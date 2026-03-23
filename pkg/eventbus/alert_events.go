package eventbus

import "time"

// Alert event topics — server events
const (
	TopicGameServerCrashed         = "game_server.crashed"
	TopicGameServerStatusChanged   = "game_server.status_changed"
	TopicGameServerCPUThreshold    = "game_server.cpu_threshold"
	TopicGameServerMemoryThreshold = "game_server.memory_threshold"
	TopicGameServerDiskThreshold   = "game_server.disk_threshold"
	TopicGameServerPlayerThreshold = "game_server.player_threshold"
)

// Alert event topics — node events
const (
	TopicNodeCPUThreshold    = "node.cpu_threshold"
	TopicNodeMemoryThreshold = "node.memory_threshold"
	TopicNodeDiskThreshold   = "node.disk_threshold"
)

// ThresholdDirection indicates whether a threshold was entered or resolved.
type ThresholdDirection string

const (
	ThresholdEntered  ThresholdDirection = "entered"
	ThresholdResolved ThresholdDirection = "resolved"
)

// ServerCrashedEvent is published when a game server process exits unexpectedly.
type ServerCrashedEvent struct {
	ServerID     string
	ServerNodeID string
	ExitCode     int
	Timestamp    time.Time
}

// StatusChangedEvent is published when a game server's status transitions.
type StatusChangedEvent struct {
	ServerID     string
	ServerNodeID string
	OldStatus    string
	NewStatus    string
}

// ThresholdEvent is published when a server metric crosses a threshold boundary.
type ThresholdEvent struct {
	ServerID     string
	ServerNodeID string
	CurrentValue float64
	Threshold    float64
	Direction    ThresholdDirection
}

// NodeThresholdEvent is published when a node-level metric crosses a threshold.
type NodeThresholdEvent struct {
	NodeID       string
	CurrentValue float64
	Threshold    float64
	Direction    ThresholdDirection
}
